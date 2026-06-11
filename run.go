package grove

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kusold/grove/db"
	"github.com/kusold/grove/health"
	"github.com/kusold/grove/httpx"
	"github.com/kusold/grove/lifecycle"
)

// Main is the primary entry point for a Grove service. It calls Run with the
// provided module and options. If Run returns an error, Main prints it to
// stderr and exits with code 1.
//
// Typical usage in a service's main.go:
//
//	func main() {
//	    grove.Main(mymodule.Module{})
//	}
func Main(module Module, opts ...Option) {
	ctx := context.Background()
	if err := Run(ctx, module, opts...); err != nil {
		fmt.Fprintf(os.Stderr, "grove: %v\n", err)
		os.Exit(1)
	}
}

// Run creates the App, applies options, validates capability dependencies,
// registers the module, and starts the service runtime. It returns an error if
// any option fails, dependency validation fails, module registration fails, or
// a lifecycle hook fails.
//
// When the HTTP capability is enabled, Run configures the HTTP transport, starts
// lifecycle hooks, starts the HTTP server, and blocks until SIGINT or SIGTERM is
// received. On signal, it shuts down the HTTP server with a configurable timeout
// and then runs lifecycle stop hooks in reverse order.
//
// Run does not call os.Exit, making it suitable for testing and programmatic
// use.
func Run(ctx context.Context, module Module, opts ...Option) error {
	app, err := NewApp(module.Name(), opts...)
	if err != nil {
		return fmt.Errorf("option error: %w", err)
	}

	// Wire Postgres lifecycle hooks before module registration so that the
	// postgres-connect hook runs before any module hooks during startup.
	// The pool is connected during lifecycle start and closed during lifecycle
	// stop, ensuring orderly startup and shutdown relative to other hooks.
	if app.hasCapability(capPostgres) {
		wirePostgresLifecycle(app)
	}

	if err := module.Register(ctx, app); err != nil {
		return fmt.Errorf("module %q registration failed: %w", module.Name(), err)
	}

	// If HTTP is not enabled, there is usually nothing to start. Postgres is
	// the exception because its capability owns a lifecycle-managed resource.
	if !app.hasCapability(capHTTP) {
		if !app.hasCapability(capPostgres) {
			return nil
		}
		if err := app.Lifecycle().Start(ctx); err != nil {
			return fmt.Errorf("lifecycle start: %w", err)
		}
		if err := app.Lifecycle().Stop(context.WithoutCancel(ctx)); err != nil {
			return fmt.Errorf("lifecycle stop: %w", err)
		}
		return nil
	}

	httpServer, err := httpx.NewServer(httpx.ServerOptions{
		Registry: app.HTTP(),
		Health:   app.Health(),
		Config:   app.Config().HTTP(),
		Logger:   app.Logger(),
	})
	if err != nil {
		return err
	}
	httpServer.RegisterLifecycle(app.Lifecycle())

	// Register signal handling before startup hooks so SIGINT/SIGTERM received
	// during startup are handled by Grove instead of the process default.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Run lifecycle start hooks (module registration is complete).
	if err := app.Lifecycle().Start(sigCtx); err != nil {
		return fmt.Errorf("lifecycle start: %w", err)
	}

	serverErr := httpServer.Run()

	select {
	case <-sigCtx.Done():
		// Restore default signal behavior before shutdown work so a second
		// SIGINT/SIGTERM can interrupt a stalled graceful stop.
		stop()
		app.Logger().Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			if stopErr := app.Lifecycle().Stop(context.WithoutCancel(ctx)); stopErr != nil {
				return errors.Join(fmt.Errorf("http server error: %w", err), fmt.Errorf("lifecycle stop: %w", stopErr))
			}
			return fmt.Errorf("http server error: %w", err)
		}
	}

	// Run lifecycle stop hooks in reverse order (includes http-server stop).
	if err := app.Lifecycle().Stop(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("lifecycle stop: %w", err)
	}

	app.Logger().Info("shutdown complete")
	return nil
}

// wirePostgresLifecycle registers lifecycle hooks and readiness checks for the
// Postgres capability. The pool is connected during startup and closed during
// shutdown. Readiness reflects Postgres connectivity so traffic can be held
// back when the database is unavailable without turning liveness into a
// dependency check.
func wirePostgresLifecycle(app *App) {
	app.Lifecycle().Append(lifecycle.Hook{
		Name: "postgres-connect",
		Start: func(ctx context.Context) error {
			dbConfig, err := db.ConfigFrom(app.Config().Database())
			if err != nil {
				return fmt.Errorf("postgres config: %w", err)
			}
			if err := app.db.Connect(ctx, dbConfig); err != nil {
				return fmt.Errorf("postgres connect: %w", err)
			}
			app.Logger().Info("postgres connected",
				"max_conns", dbConfig.MaxConns,
				"min_conns", dbConfig.MinConns,
			)
			return nil
		},
		Stop: func(ctx context.Context) error {
			app.db.Close()
			app.Logger().Info("postgres pool closed")
			return nil
		},
	})

	app.Health().RegisterReadiness(health.Check{
		Name: "postgres",
		Check: func(ctx context.Context) error {
			return app.db.Ping(ctx)
		},
	})
}

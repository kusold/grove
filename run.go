package grove

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
// When the HTTP capability is enabled, Run wires /healthz and /readyz routes,
// starts lifecycle hooks, starts the HTTP server, and blocks until SIGINT or
// SIGTERM is received. On signal, it shuts down the HTTP server with a
// configurable timeout and then runs lifecycle stop hooks in reverse order.
//
// Run does not call os.Exit, making it suitable for testing and programmatic
// use.
func Run(ctx context.Context, module Module, opts ...Option) error {
	app, err := newApp(module.Name(), opts...)
	if err != nil {
		return fmt.Errorf("option error: %w", err)
	}

	if err := module.Register(ctx, app); err != nil {
		return fmt.Errorf("module %q registration failed: %w", module.Name(), err)
	}

	// If HTTP is not enabled, there is nothing to start. Return immediately.
	if !app.hasCapability(capHTTP) {
		return nil
	}

	// Wire /healthz and /readyz routes.
	reg := app.HTTP()
	reg.Get("/healthz", app.Health().HealthzHandler())
	reg.Get("/readyz", app.Health().ReadyzHandler())

	// Configure shutdown timeout from config.
	shutdownTimeout, err := time.ParseDuration(app.Config().HTTP().ShutdownTimeout)
	if err != nil {
		return fmt.Errorf("invalid HTTP_SHUTDOWN_TIMEOUT %q: %w", app.Config().HTTP().ShutdownTimeout, err)
	}

	// Register the HTTP server stop as a lifecycle hook so it runs in the
	// correct order relative to other stop hooks (e.g., DB cleanup).
	httpServer := &http.Server{Addr: app.Config().HTTP().Addr, Handler: app.HTTP()}
	app.Lifecycle().Append(lifecycle.Hook{
		Name: "http-server",
		Stop: func(ctx context.Context) error {
			shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("http server shutdown: %w", err)
			}
			return nil
		},
	})

	// Run lifecycle start hooks (module registration is complete).
	if err := app.Lifecycle().Start(ctx); err != nil {
		return fmt.Errorf("lifecycle start: %w", err)
	}

	// Start listening in a goroutine. ListenAndServe always returns a non-nil
	// error; http.ErrServerClosed is expected during graceful shutdown.
	serverErr := make(chan error, 1)
	go func() {
		app.Logger().Info("http server starting", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Block until SIGINT, SIGTERM, or a server error.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-sigCtx.Done():
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

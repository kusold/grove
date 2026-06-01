package grove

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kusold/grove/httpx"
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

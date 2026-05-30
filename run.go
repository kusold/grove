package grove

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Main is the primary entrypoint for a Grove service. It calls Run with a
// context that cancels on SIGINT or SIGTERM. If Run returns an error, Main
// logs it and exits with code 1.
func Main(module Module, opts ...Option) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := Run(ctx, module, opts...); err != nil {
		slog.Error("grove: service exited with error", "error", err, "service", module.Name())
		os.Exit(1)
	}
}

// Run creates the App, applies options, registers the module, and manages
// the full service lifecycle. It returns any error encountered (it never
// calls os.Exit directly).
//
// The lifecycle is:
//  1. Create App and apply options.
//  2. Call module.Register to wire service capabilities.
//  3. Run lifecycle start hooks.
//  4. Wait for context cancellation or signal.
//  5. Run lifecycle stop hooks (in reverse order).
func Run(ctx context.Context, module Module, opts ...Option) error {
	if module == nil {
		return fmt.Errorf("grove: module must not be nil")
	}

	app, err := newApp(module.Name(), opts)
	if err != nil {
		return fmt.Errorf("grove: building app: %w", err)
	}

	app.Logger().Info("grove: registering module", "service", module.Name())

	// Register the module before starting anything.
	if err := module.Register(ctx, app); err != nil {
		return fmt.Errorf("grove: module registration failed: %w", err)
	}

	app.markStarted()
	app.Logger().Info("grove: module registered", "service", module.Name())

	// Start lifecycle hooks.
	if err := app.Lifecycle().Start(ctx); err != nil {
		return fmt.Errorf("grove: lifecycle start failed: %w", err)
	}
	app.Logger().Info("grove: service started", "service", module.Name())

	// Wait for shutdown signal.
	<-ctx.Done()
	app.Logger().Info("grove: shutting down", "service", module.Name())

	// Stop lifecycle hooks in reverse order.
	stopCtx, cancel := context.WithTimeout(context.Background(), app.Config().Service.ShutdownTimeout)
	defer cancel()

	if err := app.Lifecycle().Stop(stopCtx); err != nil {
		return fmt.Errorf("grove: lifecycle stop failed: %w", err)
	}

	app.Logger().Info("grove: service stopped", "service", module.Name())
	return nil
}

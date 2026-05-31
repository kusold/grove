package grove

import (
	"context"
	"fmt"
	"os"
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
// and registers the module. It returns an error if any option fails, dependency
// validation fails, or module registration fails. Run does not call os.Exit,
// making it suitable for testing and programmatic use.
//
// The HTTP server is not started until after module registration completes
// successfully.
func Run(ctx context.Context, module Module, opts ...Option) error {
	app, err := newApp(module.Name(), opts...)
	if err != nil {
		return fmt.Errorf("option error: %w", err)
	}

	if err := app.validateCapabilities(); err != nil {
		return err
	}

	if err := module.Register(ctx, app); err != nil {
		return fmt.Errorf("module %q registration failed: %w", module.Name(), err)
	}

	return nil
}

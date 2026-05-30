package grove

import "context"

// Module is the core interface that every Grove service must implement.
// A service's Register method wires its routes, lifecycle hooks, and other
// capabilities into the provided App.
type Module interface {
	// Name returns the service's identity. This value is used for logging,
	// process metadata, and config defaults. It should be a short, stable
	// identifier (e.g. "canopy").
	Name() string

	// Register is called once during startup. The module should use the
	// provided App to register HTTP routes, lifecycle hooks, health checks,
	// and any other capabilities the service needs.
	Register(ctx context.Context, app *App) error
}

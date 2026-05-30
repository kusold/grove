package grove

import "context"

// Module is the interface every Grove service must implement.
// A service's Register method wires routes, jobs, and other capabilities
// into the App.
type Module interface {
	// Name returns the service's identity (e.g. "canopy").
	// This value is used for logging, health checks, and observability.
	Name() string

	// Register is called once during startup. The module should use the
	// provided App to register HTTP routes, lifecycle hooks, health checks,
	// and any other capabilities the service needs.
	//
	// Register must complete before the HTTP server starts accepting traffic.
	// If Register returns an error, startup is aborted.
	Register(ctx context.Context, app *App) error
}

package grove

import (
	"fmt"
	"log/slog"

	"github.com/kusold/grove/config"
	"github.com/kusold/grove/db"
	"github.com/kusold/grove/health"
	"github.com/kusold/grove/httpx"
	"github.com/kusold/grove/lifecycle"
)

// App is the central runtime object for a Grove service. It holds private state
// and exposes public methods for registering capabilities during module
// registration. All fields are private to keep the public API stable and explicit.
type App struct {
	name         string
	capabilities map[capability]bool
	cfg          config.Provider
	logger       *slog.Logger
	lifecycle    *lifecycle.Manager
	healthReg    *health.Registry
	httpReg      *httpx.Registry
	db           *db.Database
}

// Name returns the service name, derived from Module.Name().
func (a *App) Name() string {
	return a.name
}

// Config returns the service configuration loaded from environment variables.
// The config is loaded during app construction and is available for the
// entire lifetime of the service.
func (a *App) Config() config.Provider {
	return a.cfg
}

// Logger returns the configured structured logger for the service. The logger
// includes service name, environment, and version as default attributes. Log
// format is controlled by the LOG_FORMAT env var ("text" or "json"), and
// colorization of text output by LOG_COLOR ("on", "off", or "auto").
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// Lifecycle returns the lifecycle manager for registering start/stop hooks.
// Hooks are run in registration order on startup and in reverse order on
// shutdown.
func (a *App) Lifecycle() *lifecycle.Manager {
	return a.lifecycle
}

// Health returns the health and readiness registry. Services can register
// health checks (for liveness) and readiness checks (for traffic readiness)
// through this registry. The HTTP capability wires /healthz and /readyz
// routes to these checks automatically.
func (a *App) Health() *health.Registry {
	return a.healthReg
}

// HTTP returns the HTTP registry for registering routes and middleware.
// It panics with a clear error message if the HTTP capability was not enabled
// via grove.WithHTTP(). This fail-fast behavior ensures missing capabilities
// are caught during module registration rather than at request time.
func (a *App) HTTP() *httpx.Registry {
	if err := a.requireCapability(capHTTP); err != nil {
		panic(err.Error())
	}
	return a.httpReg
}

// RequireDB returns the database handle when the Postgres capability is enabled.
// It returns a helpful error when Postgres was not enabled via grove.WithPostgres().
// This accessor should be used during module registration or inside handlers to
// obtain the database connection.
//
// Example:
//
//	db, err := app.RequireDB()
//	if err != nil {
//	    return err
//	}
//	db.Pool().QueryRow(ctx, "SELECT 1")
func (a *App) RequireDB() (*db.Database, error) {
	if err := a.requireCapability(capPostgres); err != nil {
		return nil, err
	}
	return a.db, nil
}

// hasCapability reports whether a capability is enabled.
func (a *App) hasCapability(c capability) bool {
	return a.capabilities[c]
}

// requireCapability returns an error if the given capability is not enabled.
// The error message guides the user toward the correct Option function.
func (a *App) requireCapability(c capability) error {
	if a.hasCapability(c) {
		return nil
	}
	return fmt.Errorf(
		"grove: %s capability is required but was not enabled; add grove.%s()",
		capabilityDisplayName[c],
		capabilityOptionName[c],
	)
}

// NewApp creates an App with the given name and applies the provided options.
// It is intended for use in tests that need to exercise Module.Register against
// a real App. Production code should use Main or Run instead.
//
// If any option returns an error, NewApp returns that error.
func NewApp(name string, opts ...Option) (*App, error) {
	b := newBuilder(name)
	if err := b.applyOptions(opts...); err != nil {
		return nil, err
	}
	if err := b.validateCapabilities(); err != nil {
		return nil, err
	}
	return b.buildApp(), nil
}

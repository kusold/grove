package grove

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/kusold/grove/config"
	"github.com/kusold/grove/health"
	"github.com/kusold/grove/lifecycle"
)

// App is the central runtime object passed to a service's Register method.
// It holds references to registries for HTTP, health, lifecycle, and other
// capabilities. All fields are private; access is through public methods.
type App struct {
	name    string
	cfg     *config.Config
	logger  *slog.Logger
	lm      *lifecycle.Manager
	hr      *health.Registry
	started bool
	mu      sync.Mutex
}

// newApp creates an App with the given name and applies all options.
func newApp(name string, opts []Option) (*App, error) {
	cfg := config.Default()
	cfg.Service.Name = name

	logger := slog.Default()

	app := &App{
		name:   name,
		cfg:    cfg,
		logger: logger,
		lm:     lifecycle.NewManager(),
		hr:     health.NewRegistry(),
	}

	for _, opt := range opts {
		if err := opt(app); err != nil {
			return nil, fmt.Errorf("grove: applying option: %w", err)
		}
	}

	return app, nil
}

// Name returns the service name.
func (a *App) Name() string {
	return a.name
}

// Config returns the service configuration.
func (a *App) Config() *config.Config {
	return a.cfg
}

// Logger returns the structured logger.
func (a *App) Logger() *slog.Logger {
	return a.logger
}

// Health returns the health check registry.
func (a *App) Health() *health.Registry {
	return a.hr
}

// Lifecycle returns the lifecycle hook manager.
func (a *App) Lifecycle() *lifecycle.Manager {
	return a.lm
}

// markStarted prevents further registration after startup begins.
func (a *App) markStarted() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.started = true
}

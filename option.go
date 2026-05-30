package grove

import (
	"fmt"
	"log/slog"

	"github.com/kusold/grove/config"
)

// Option configures an App during construction.
// Options are applied in order before the module is registered.
type Option func(app *App) error

// WithConfig replaces the default config with the provided one.
func WithConfig(cfg *config.Config) Option {
	return func(app *App) error {
		app.cfg = cfg
		return nil
	}
}

// WithLogger replaces the default logger with the provided one.
func WithLogger(logger *slog.Logger) Option {
	return func(app *App) error {
		if logger == nil {
			return fmt.Errorf("grove: logger must not be nil")
		}
		app.logger = logger
		return nil
	}
}

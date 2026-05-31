package grove

import (
	"fmt"

	"github.com/kusold/grove/config"
)

// App is the central runtime object for a Grove service. It holds private state
// and exposes public methods for registering capabilities during module
// registration. All fields are private to keep the public API stable and explicit.
type App struct {
	name         string
	capabilities map[capability]bool
	cfg          config.Provider
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

// newApp creates an App with the given name and applies the provided options.
// If any option returns an error, application stops and the error is returned.
func newApp(name string, opts ...Option) (*App, error) {
	b := newBuilder(name)
	if err := b.applyOptions(opts...); err != nil {
		return nil, err
	}
	if err := b.validateCapabilities(); err != nil {
		return nil, err
	}
	return b.buildApp(), nil
}

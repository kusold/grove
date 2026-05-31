package grove

import "fmt"

// App is the central runtime object for a Grove service. It holds private state
// and exposes public methods for registering capabilities during module
// registration. All fields are private to keep the public API stable and explicit.
type App struct {
	name         string
	capabilities map[capability]bool
}

// Name returns the service name, derived from Module.Name().
func (a *App) Name() string {
	return a.name
}

// enableCapability records that a capability has been enabled.
func (a *App) enableCapability(c capability) {
	if a.capabilities == nil {
		a.capabilities = make(map[capability]bool)
	}
	a.capabilities[c] = true
}

// hasCapability reports whether a capability is enabled.
func (a *App) hasCapability(c capability) bool {
	return a.capabilities[c]
}

// validateCapabilities checks that all enabled capabilities have their
// dependencies satisfied. It returns an error describing the first missing
// dependency found, iterating in the deterministic capability order so that
// error messages are consistent regardless of option registration order.
func (a *App) validateCapabilities() error {
	for _, c := range capabilityOrder {
		if !a.hasCapability(c) {
			continue
		}
		deps, hasDeps := capabilityDeps[c]
		if !hasDeps {
			continue
		}
		for _, dep := range deps {
			if !a.hasCapability(dep) {
				return fmt.Errorf(
					"grove: %s requires %s, but it was not enabled; add grove.%s()",
					capabilityDisplayName[c],
					capabilityDisplayName[dep],
					capabilityOptionName[dep],
				)
			}
		}
	}
	return nil
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
	a := &App{
		name: name,
	}
	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, err
		}
	}
	return a, nil
}

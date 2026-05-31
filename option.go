package grove

import (
	"fmt"

	"github.com/kusold/grove/config"
)

// Option configures a Grove app during construction. Options are applied in
// order before module registration runs. An Option may return an error if its
// configuration is invalid.
type Option func(*builder) error

type builder struct {
	name         string
	capabilities map[capability]bool
}

// capability represents a named capability that can be enabled on an App.
type capability string

const (
	capHTTP capability = "http"
)

// capabilityDeps maps each capability to its required dependencies.
// A capability cannot be enabled unless all of its dependencies are also
// enabled. Dependencies are validated after all options are applied, so the
// order of options does not matter.
var capabilityDeps = map[capability][]capability{}

// capabilityOrder defines the deterministic initialization order for
// capabilities. Capabilities are initialized in this order regardless of the
// order in which options are passed to Main or Run.
var capabilityOrder = []capability{
	capHTTP,
}

// capabilityOptionName maps each capability to the Option function name used
// in error messages to guide users toward the fix.
var capabilityOptionName = map[capability]string{
	capHTTP: "WithHTTP",
}

// capabilityDisplayName maps each capability to a human-readable name used in
// error messages.
var capabilityDisplayName = map[capability]string{
	capHTTP: "http",
}

func newBuilder(name string) *builder {
	return &builder{name: name}
}

func (b *builder) applyOptions(opts ...Option) error {
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return err
		}
	}
	return nil
}

// enableCapability records that a capability has been enabled.
func (b *builder) enableCapability(c capability) {
	if b.capabilities == nil {
		b.capabilities = make(map[capability]bool)
	}
	b.capabilities[c] = true
}

// hasCapability reports whether a capability is enabled.
func (b *builder) hasCapability(c capability) bool {
	return b.capabilities[c]
}

// validateCapabilities checks that all enabled capabilities have their
// dependencies satisfied. It returns an error describing the first missing
// dependency found, iterating in deterministic capability order so that error
// messages are consistent regardless of option registration order.
func (b *builder) validateCapabilities() error {
	for _, c := range capabilityOrder {
		if !b.hasCapability(c) {
			continue
		}
		deps, hasDeps := capabilityDeps[c]
		if !hasDeps {
			continue
		}
		for _, dep := range deps {
			if !b.hasCapability(dep) {
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

func (b *builder) buildApp() *App {
	return &App{
		name:         b.name,
		capabilities: b.capabilitySet(),
		cfg:          config.Load(b.name),
	}
}

func (b *builder) capabilitySet() map[capability]bool {
	if len(b.capabilities) == 0 {
		return nil
	}
	caps := make(map[capability]bool, len(b.capabilities))
	for _, c := range capabilityOrder {
		if b.hasCapability(c) {
			caps[c] = true
		}
	}
	return caps
}

// WithHTTP enables the HTTP capability, backed by chi.
func WithHTTP() Option {
	return func(b *builder) error {
		b.enableCapability(capHTTP)
		return nil
	}
}

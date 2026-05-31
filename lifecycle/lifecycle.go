// Package lifecycle provides start/stop hook management for Grove services.
// Hooks are run in registration order on startup and in reverse order on
// shutdown, giving each capability a chance to initialize and tear down
// deterministically.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Hook represents a named start/stop pair that can be registered with a
// Manager. Both Start and Stop may be nil if only one direction is needed.
type Hook struct {
	// Name identifies the hook in log messages and error output.
	Name string

	// Start is called during service startup. Hooks run in registration order.
	Start func(ctx context.Context) error

	// Stop is called during service shutdown. Hooks run in reverse registration
	// order.
	Stop func(ctx context.Context) error
}

// Manager manages a collection of lifecycle hooks. It is safe to call Append
// from multiple goroutines, but Start and Stop are intended to be called
// sequentially during service lifecycle transitions.
type Manager struct {
	mu    sync.Mutex
	hooks []Hook
}

// New creates a new Manager with no hooks.
func New() *Manager {
	return &Manager{}
}

// Append adds a hook to the manager. Hooks are started in append order and
// stopped in reverse append order.
func (m *Manager) Append(h Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, h)
}

// Start runs all registered hook Start functions in registration order. If a
// hook returns an error, Start stops any hooks that already started and returns
// the start error.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	hooks := make([]Hook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	var started []Hook
	for _, h := range hooks {
		if h.Start == nil {
			continue
		}
		if err := h.Start(ctx); err != nil {
			startErr := fmt.Errorf("lifecycle hook %q start: %w", h.Name, err)
			if stopErr := stopHooks(ctx, started); stopErr != nil {
				return errors.Join(startErr, fmt.Errorf("lifecycle startup rollback: %w", stopErr))
			}
			return startErr
		}
		started = append(started, h)
	}
	return nil
}

// Stop runs all registered hook Stop functions in reverse registration order.
// If a hook returns an error, Stop continues running remaining hooks and
// returns the first error encountered. This ensures all hooks get a chance to
// clean up even if one fails.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	hooks := make([]Hook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	return stopHooks(ctx, hooks)
}

func stopHooks(ctx context.Context, hooks []Hook) error {
	var firstErr error
	for i := len(hooks) - 1; i >= 0; i-- {
		h := hooks[i]
		if h.Stop == nil {
			continue
		}
		if err := h.Stop(ctx); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("lifecycle hook %q stop: %w", h.Name, err)
			}
		}
	}
	return firstErr
}

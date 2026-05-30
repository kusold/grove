package lifecycle

import "context"

// Hook represents a start/stop lifecycle hook.
type Hook struct {
	Name  string
	Start func(ctx context.Context) error
	Stop  func(ctx context.Context) error
}

// Manager manages ordered lifecycle hooks.
// Start hooks run in registration order.
// Stop hooks run in reverse registration order.
type Manager struct {
	hooks []Hook
}

// NewManager creates an empty lifecycle manager.
func NewManager() *Manager {
	return &Manager{}
}

// Append adds a lifecycle hook.
func (m *Manager) Append(hook Hook) {
	m.hooks = append(m.hooks, hook)
}

// Start runs all start hooks in registration order.
func (m *Manager) Start(ctx context.Context) error {
	for _, hook := range m.hooks {
		if hook.Start != nil {
			if err := hook.Start(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stop runs all stop hooks in reverse registration order.
func (m *Manager) Stop(ctx context.Context) error {
	var firstErr error
	for i := len(m.hooks) - 1; i >= 0; i-- {
		if m.hooks[i].Stop != nil {
			if err := m.hooks[i].Stop(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

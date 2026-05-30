package health

import "context"

// Check is a named health or readiness check.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// Registry holds health and readiness checks.
type Registry struct {
	healthChecks []Check
	readyChecks  []Check
}

// NewRegistry creates an empty health registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// AddHealthCheck registers a liveness check.
func (r *Registry) AddHealthCheck(check Check) {
	r.healthChecks = append(r.healthChecks, check)
}

// AddReadyCheck registers a readiness check.
func (r *Registry) AddReadyCheck(check Check) {
	r.readyChecks = append(r.readyChecks, check)
}

// IsHealthy returns true if all liveness checks pass.
func (r *Registry) IsHealthy(ctx context.Context) error {
	for _, check := range r.healthChecks {
		if err := check.Fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

// IsReady returns true if all readiness checks pass.
func (r *Registry) IsReady(ctx context.Context) error {
	for _, check := range r.readyChecks {
		if err := check.Fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

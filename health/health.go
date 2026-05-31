// Package health provides health and readiness check registries for Grove
// services. Health checks determine whether a service process is alive (liveness),
// while readiness checks determine whether it is prepared to accept traffic.
//
// In Phase 1, both /healthz and /readyz return 200 unconditionally. As
// capabilities like Postgres and migrations are added, checks can be registered
// to make readiness reflect real infrastructure state.
//
// The registry keeps separate evaluation paths for health and readiness so they
// can diverge independently. For example, a service might be healthy (alive)
// but not ready (waiting for DB connections).
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

// Check is a named health or readiness check. If the check function returns a
// non-nil error, the check is considered failing.
type Check struct {
	// Name identifies the check in structured responses and logs.
	Name string

	// Check runs the actual health or readiness probe. A nil return means
	// healthy/ready. A non-nil return means failing, and the error message
	// is included in the response body.
	Check func(ctx context.Context) error
}

// checkResult holds the result of evaluating a single check.
type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Registry manages separate lists of health (liveness) and readiness checks.
// Both lists are safe to append from multiple goroutines.
type Registry struct {
	mu        sync.Mutex
	health    []Check
	readiness []Check
}

// New creates a new Registry with no checks registered.
func New() *Registry {
	return &Registry{}
}

// RegisterHealth adds a health (liveness) check. Health checks should confirm
// that the service process is alive and not deadlocked. They should be fast and
// not depend on external infrastructure.
func (r *Registry) RegisterHealth(check Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.health = append(r.health, check)
}

// RegisterReadiness adds a readiness check. Readiness checks confirm that the
// service is prepared to accept traffic—for example, that database connections
// are established and migrations are current.
func (r *Registry) RegisterReadiness(check Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readiness = append(r.readiness, check)
}

// IsHealthy evaluates all registered health checks. It returns nil if every
// check passes, or an error describing the first failure. All checks are run
// even if one fails, so that all failures can be reported.
func (r *Registry) IsHealthy(ctx context.Context) (err error) {
	return runChecks(ctx, r.healthChecks())
}

// IsReady evaluates all registered readiness checks. It returns nil if every
// check passes, or an error describing the first failure. All checks are run
// even if one fails, so that all failures can be reported.
func (r *Registry) IsReady(ctx context.Context) (err error) {
	return runChecks(ctx, r.readinessChecks())
}

func (r *Registry) healthChecks() []Check {
	r.mu.Lock()
	defer r.mu.Unlock()

	checks := make([]Check, len(r.health))
	copy(checks, r.health)
	return checks
}

func (r *Registry) readinessChecks() []Check {
	r.mu.Lock()
	defer r.mu.Unlock()

	checks := make([]Check, len(r.readiness))
	copy(checks, r.readiness)
	return checks
}

// HealthzHandler returns an http.HandlerFunc that evaluates health checks and
// returns HTTP 200 when healthy or HTTP 503 when not. In Phase 1, with no
// checks registered, it always returns 200.
//
// Response body (healthy):
//
//	{"status": "ok"}
//
// Response body (unhealthy):
//
//	{"status": "unhealthy", "checks": [{"name": "db", "status": "failing", "error": "connection refused"}]}
func (r *Registry) HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.serveCheck(w, req, r.healthChecks(), "unhealthy")
	}
}

// ReadyzHandler returns an http.HandlerFunc that evaluates readiness checks and
// returns HTTP 200 when ready or HTTP 503 when not. In Phase 1, with no checks
// registered, it always returns 200.
//
// Response body (ready):
//
//	{"status": "ok"}
//
// Response body (not ready):
//
//	{"status": "not_ready", "checks": [{"name": "db", "status": "failing", "error": "connection refused"}]}
func (r *Registry) ReadyzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.serveCheck(w, req, r.readinessChecks(), "not_ready")
	}
}

// serveCheck is the shared logic for both /healthz and /readyz. It evaluates
// the given checks, writes a JSON response, and sets the appropriate
// HTTP status code.
func (r *Registry) serveCheck(w http.ResponseWriter, req *http.Request, checks []Check, failStatus string) {
	results, err := runChecksWithResults(req.Context(), checks)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"status": failStatus,
		"checks": results,
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

// runChecks executes all checks and returns the first error encountered. All
// checks are run regardless of failures so that every problem can be reported.
func runChecks(ctx context.Context, checks []Check) error {
	var firstErr error
	for _, c := range checks {
		if err := c.Check(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runChecksWithResults(ctx context.Context, checks []Check) ([]checkResult, error) {
	results := make([]checkResult, 0, len(checks))
	var firstErr error
	for _, c := range checks {
		if err := c.Check(ctx); err != nil {
			results = append(results, checkResult{
				Name:   c.Name,
				Status: "failing",
				Error:  err.Error(),
			})
			if firstErr == nil {
				firstErr = err
			}
		} else {
			results = append(results, checkResult{
				Name:   c.Name,
				Status: "passing",
			})
		}
	}
	return results, firstErr
}

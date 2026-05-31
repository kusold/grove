package grove

// Option configures an App during construction. Options are applied in order
// before module registration runs. An Option may return an error if the
// configuration is invalid (e.g., a capability dependency is missing).
type Option func(*App) error

// capability represents a named capability that can be enabled on an App.
type capability string

const (
	capHTTP          capability = "http"
	capPostgres      capability = "postgres"
	capMigrations    capability = "migrations"
	capTenancy       capability = "tenancy"
	capOpenAPI       capability = "openapi"
	capObservability capability = "observability"
	capJobs          capability = "jobs"
	capOIDC          capability = "oidc"
)

// capabilityDeps maps each capability to its required dependencies.
// A capability cannot be enabled unless all of its dependencies are also
// enabled. Dependencies are validated after all options are applied, so the
// order of options does not matter.
var capabilityDeps = map[capability][]capability{
	capMigrations: {capPostgres},
	capOpenAPI:    {capHTTP},
	capTenancy:    {capHTTP},
	capJobs:       {capPostgres},
	capOIDC:       {capHTTP},
}

// capabilityOrder defines the deterministic initialization order for
// capabilities. Capabilities are initialized in this order regardless of the
// order in which options are passed to Main or Run.
var capabilityOrder = []capability{
	capObservability,
	capPostgres,
	capMigrations,
	capHTTP,
	capTenancy,
	capOpenAPI,
	capJobs,
	capOIDC,
}

// capabilityOptionName maps each capability to the Option function name used
// in error messages to guide users toward the fix.
var capabilityOptionName = map[capability]string{
	capHTTP:          "WithHTTP",
	capPostgres:      "WithPostgres",
	capMigrations:    "WithMigrations",
	capTenancy:       "WithTenancy",
	capOpenAPI:       "WithOpenAPI",
	capObservability: "WithObservability",
	capJobs:          "WithJobs",
	capOIDC:          "WithOIDC",
}

// capabilityDisplayName maps each capability to a human-readable name used in
// error messages.
var capabilityDisplayName = map[capability]string{
	capHTTP:          "http",
	capPostgres:      "postgres",
	capMigrations:    "migrations",
	capTenancy:       "tenancy",
	capOpenAPI:       "openapi",
	capObservability: "observability",
	capJobs:          "jobs",
	capOIDC:          "oidc",
}

// WithHTTP enables the HTTP capability, backed by chi.
func WithHTTP() Option {
	return func(a *App) error {
		a.enableCapability(capHTTP)
		return nil
	}
}

// WithPostgres enables the Postgres capability.
func WithPostgres() Option {
	return func(a *App) error {
		a.enableCapability(capPostgres)
		return nil
	}
}

// WithMigrations enables the migrations capability.
// Requires WithPostgres.
func WithMigrations() Option {
	return func(a *App) error {
		a.enableCapability(capMigrations)
		return nil
	}
}

// WithTenancy enables the tenancy capability.
// Requires WithHTTP.
func WithTenancy() Option {
	return func(a *App) error {
		a.enableCapability(capTenancy)
		return nil
	}
}

// WithOpenAPI enables the OpenAPI capability.
// Requires WithHTTP.
func WithOpenAPI() Option {
	return func(a *App) error {
		a.enableCapability(capOpenAPI)
		return nil
	}
}

// WithObservability enables the observability capability.
func WithObservability() Option {
	return func(a *App) error {
		a.enableCapability(capObservability)
		return nil
	}
}

// WithJobs enables the background jobs capability.
// Requires WithPostgres.
func WithJobs() Option {
	return func(a *App) error {
		a.enableCapability(capJobs)
		return nil
	}
}

// WithOIDC enables the OIDC authentication capability.
// Requires WithHTTP.
func WithOIDC() Option {
	return func(a *App) error {
		a.enableCapability(capOIDC)
		return nil
	}
}

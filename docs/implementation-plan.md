# Grove Implementation Plan

This document is a detailed handoff plan for implementing Grove, an internal Go web
framework/service platform. It is written so it can be converted into GitHub issues
with minimal interpretation.

Grove is developed alongside Canopy, an example service in the same Go workspace.
Canopy is a framework demo, not a product service. Its job is to prove Grove's
APIs, defaults, testing story, and upgrade path.

## Current Workspace

The current workspace has this shape:

```text
grove-workspace/
  go.work
  grove/
    go.mod
  canopy/
    go.mod
```

Current module names:

```text
github.com/kusold/grove
github.com/kusold/canopy
```

The workspace already follows the desired early-development strategy: separate Go
modules developed together with `go.work`.

## Product Decisions Already Made

These decisions should be treated as settled unless explicitly revisited.

1. `chi` is foundational to Grove HTTP services.
2. `chi` may be exposed as public Grove API.
3. Database migrations must be configurable.
4. Canopy should use automatic startup migrations.
5. Canopy is primarily a framework demo.
6. Multitenancy is first-class for HTTP services.
7. Tenant isolation should rely on Postgres row-level security.
8. OpenAPI is strongly encouraged for JSON routes.
9. OpenAPI is not mandatory for all routes because services may also use HTML,
   `templ`, `htmx`, or similar server-rendered approaches.

## Core Design Principles

1. Grove is a dependency, not a copied template.
2. Services should contain business behavior and service-specific wiring.
3. Grove should own cross-cutting infrastructure.
4. Generated service code should stay minimal.
5. Public APIs should be small, explicit, and hard to misuse.
6. Missing capabilities should fail clearly at startup or registration time.
7. Tenancy should fail closed at the HTTP layer, DB helper layer, and Postgres RLS
   layer.
8. Canopy should drive the API design by exercising real framework paths.
9. Prefer boring, idiomatic Go over hidden magic.
10. Do not add dependencies until the phase that actually needs them.

## Target High-Level API

The eventual Canopy `main.go` should look approximately like this:

```go
package main

import (
	"github.com/kusold/canopy/internal/canopy"
	"github.com/kusold/grove"
)

func main() {
	grove.Main(
		canopy.Module{},
		grove.WithHTTP(),
		grove.WithPostgres(),
		grove.WithMigrations(),
		grove.WithTenancy(),
		grove.WithOpenAPI(),
	)
}
```

Avoid `platform.Main` naming in code and docs. The public package is Grove, so
examples should use `grove.Main`, `grove.WithHTTP`, etc.

## Target Module Interface

Start with one service module interface:

```go
type Module interface {
	Name() string
	Register(ctx context.Context, app *App) error
}
```

Avoid splitting into many optional interfaces too early. The service's
`Register` method can call explicit Grove registries:

```go
func (m Module) Register(ctx context.Context, app *grove.App) error {
	app.HTTP().Route("/example", func(r chi.Router) {
		r.Get("/hello", handlers.Hello)
	})

	return nil
}
```

Future optional interfaces may be useful, but should be introduced only after
Canopy demonstrates the need.

## Target App API

The `App` should have private fields only. Public methods should expose stable
registries or required capabilities.

Sketch:

```go
type App struct {
	// private fields only
}

func (a *App) Name() string
func (a *App) Config() config.Provider
func (a *App) Logger() *slog.Logger
func (a *App) HTTP() *httpx.Registry
func (a *App) Health() *health.Registry
func (a *App) Lifecycle() *lifecycle.Manager
func (a *App) Tenants() *tenancy.Manager

// Capability-backed accessors should fail clearly when the capability was not enabled.
func (a *App) RequireDB() (*db.Database, error)
func (a *App) RequireMigrations() (*migrate.Registry, error)
func (a *App) RequireOpenAPI() (*openapi.Registry, error)
```

Important decision: avoid accessors that silently return `nil`. If a capability is
not enabled, Grove should produce a helpful error such as:

```text
grove: postgres capability is required but was not enabled; add grove.WithPostgres()
```

## Capability Model

Capabilities should be registered with options. Add each public capability option
in the phase where the capability is implemented, rather than reserving stubs for
future features too early.

The Phase 1 skeleton starts with the first capability Grove will implement:

```go
grove.WithHTTP()
```

Later phases should add their own options, such as `WithPostgres`,
`WithMigrations`, `WithTenancy`, `WithOpenAPI`, `WithOIDC`, `WithJobs`,
`WithMail`, and `WithObservability`, when those capabilities are implemented.

Each capability may contribute:

1. Config schema/defaults.
2. Lifecycle hooks.
3. Health/readiness checks.
4. HTTP middleware.
5. Testkit fakes.
6. Observability hooks.
7. Capability dependencies.

The builder should validate dependencies before starting:

```text
WithMigrations requires WithPostgres
WithOpenAPI requires WithHTTP
WithTenancy on HTTP services requires WithHTTP
WithJobs requires WithPostgres
WithOIDC requires WithHTTP
```

Order should not be accidental. Capabilities may be registered in any order, but
Grove should initialize them in a deterministic internal order.

## Build Phases

The implementation should be incremental. Do not attempt the whole framework in
one pass. Each phase should update Canopy and include tests where practical.

## Phase 0: Planning, Repository Hygiene, and ADRs

### Goal

Capture the foundational choices before public APIs spread through Canopy.

### Dependencies

None.

### Deliverables

1. Add `docs/implementation-plan.md`.
2. Add `docs/adr/0001-grove-as-framework.md`.
3. Add `docs/adr/0002-chi-as-public-api.md`.
4. Add `docs/adr/0003-postgres-rls-tenancy.md`.
5. Add `docs/adr/0004-configurable-migrations.md`.
6. Add a short README section linking to the implementation plan and ADRs.

### Acceptance Criteria

1. A new contributor can understand the intended direction without reading a chat
   transcript.
2. The settled decisions above are represented in docs.
3. Any still-open decisions are listed explicitly.

### Pay Special Attention To

1. Do not over-spec every future capability.
2. Keep ADRs short and decisive.
3. Capture why the decision was made, not only what was chosen.

### Candidate GitHub Issues

1. Create Grove implementation plan document.
2. Add ADR for Grove as dependency-first framework.
3. Add ADR for chi as public HTTP API.
4. Add ADR for Postgres RLS-backed tenancy.
5. Add ADR for configurable migration behavior.

## Phase 1: Grove Core HTTP Skeleton

### Goal

Build the smallest usable Grove runtime that starts Canopy as an HTTP service.

### Dependencies

1. `github.com/go-chi/chi/v5`
2. Standard library `log/slog`

### Deliverables in Grove

1. `grove.Main(module Module, opts ...Option)`.
2. `grove.Run(ctx context.Context, module Module, opts ...Option) error`.
3. `Module` interface.
4. `Option` type and internal builder.
5. `App` object with private fields.
6. Config package with environment loading.
7. Logger initialization using `slog`.
8. Lifecycle manager with start/stop hooks.
9. Health registry.
10. HTTP registry backed by chi.
11. Initial `WithHTTP()` capability option.
12. Graceful shutdown for SIGINT and SIGTERM.
13. `/healthz` route.
14. `/readyz` route.
15. Capability dependency validation framework, even if simple.

### Deliverables in Canopy

1. `main.go` using `grove.Main`.
2. `internal/canopy/module.go`.
3. `GET /example/hello` returning:

```json
{
  "message": "hello from canopy"
}
```

### Proposed Grove Package Layout

```text
grove/
  app.go
  module.go
  option.go
  run.go
  config/
    config.go
  health/
    health.go
  httpx/
    httpx.go
    middleware.go
  lifecycle/
    lifecycle.go
```

Keep root package files small. Internal builder types can live in unexported root
package files unless they grow large enough to justify `internal/builder`.

### HTTP Registry Sketch

Since chi is public API, make chi easy to use:

```go
type Registry struct {
	router chi.Router
}

func New() *Registry
func (r *Registry) Router() chi.Router
func (r *Registry) Route(pattern string, fn func(r chi.Router))
func (r *Registry) Mount(pattern string, h http.Handler)
func (r *Registry) Use(middlewares ...func(http.Handler) http.Handler)
func (r *Registry) Get(pattern string, h http.HandlerFunc)
func (r *Registry) Post(pattern string, h http.HandlerFunc)
```

The `Router()` method intentionally exposes chi.

### Lifecycle Sketch

```go
type Hook struct {
	Name  string
	Start func(context.Context) error
	Stop  func(context.Context) error
}

type Manager struct {
	hooks []Hook
}

func (m *Manager) Append(h Hook)
func (m *Manager) Start(ctx context.Context) error
func (m *Manager) Stop(ctx context.Context) error
```

Startup should run hooks in registration order. Shutdown should run hooks in
reverse order.

### Config Sketch

Start simple:

```go
type Config struct {
	Service ServiceConfig
	HTTP    HTTPConfig
}

type ServiceConfig struct {
	Name        string
	Environment string
	Version     string
}

type HTTPConfig struct {
	Addr string
}
```

Initial env vars:

```text
SERVICE_NAME=canopy
SERVICE_ENV=development
SERVICE_VERSION=dev
HTTP_ADDR=:8080
```

If both `Module.Name()` and `SERVICE_NAME` exist, `SERVICE_NAME` should override
runtime config but not change the module identity. Document this clearly.

### Acceptance Criteria

1. `go test ./...` passes in `grove`.
2. `go test ./...` passes in `canopy`.
3. `go run ./canopy` from the workspace starts an HTTP server.
4. `GET /healthz` returns HTTP 200.
5. `GET /readyz` returns HTTP 200.
6. `GET /example/hello` returns JSON.
7. SIGINT/SIGTERM shut the service down gracefully.

### Pay Special Attention To

1. Avoid designing Postgres, OpenAPI, auth, and jobs in Phase 1.
2. Do not introduce a DI container.
3. Do not let the HTTP server start before module registration completes.
4. Do not let `Main` swallow errors silently. It may `os.Exit(1)`, but `Run`
   should return errors.
5. Add timeouts for HTTP server shutdown.
6. Health and readiness are different concepts. In Phase 1 they may behave the
   same, but keep separate registries or separate evaluation paths.
7. Keep logging safe by default. Do not log all environment variables.

### Candidate GitHub Issues

1. Implement Grove module interface and runtime entrypoint.
2. Implement option builder, initial `WithHTTP()` capability option, and
   capability validation skeleton.
3. Implement config loading for service and HTTP settings.
4. Implement slog logger setup.
5. Implement lifecycle manager.
6. Implement health and readiness registries.
7. Implement chi-backed HTTP registry.
8. Implement graceful HTTP server startup and shutdown.
9. Wire Canopy to Grove with `/example/hello`.
10. Add Phase 1 tests for Grove and Canopy.

## Phase 2: Tenancy Context and HTTP Tenant Middleware Shape

### Goal

Introduce first-class tenant concepts before Postgres so all later DB work has the
right security model.

### Dependencies

1. Phase 1.
2. No external dependency required.

### Deliverables in Grove

1. `tenancy` package.
2. `Tenant` type.
3. Tenant context helpers.
4. Tenant resolver interface.
5. Header tenant resolver for local/demo use.
6. Tenant middleware.
7. Tenant-required route middleware.
8. Consistent error response for missing tenant.
9. `WithTenancy()` capability option that depends on `WithHTTP()`.

### Tenant Type Sketch

```go
type Tenant struct {
	ID   string
	Slug string
}
```

The ID will likely become UUID-oriented once Postgres is introduced. Keep it as
string initially to avoid forcing a UUID dependency before it is needed.

### Context API Sketch

```go
func WithTenant(ctx context.Context, tenant Tenant) context.Context
func FromContext(ctx context.Context) (Tenant, bool)
func Require(ctx context.Context) (Tenant, error)
```

### Resolver Sketch

```go
type Resolver interface {
	ResolveTenant(r *http.Request) (Tenant, bool, error)
}
```

Header resolver for Canopy:

```text
X-Tenant-ID: 00000000-0000-0000-0000-000000000001
X-Tenant-Slug: acme
```

### Middleware Sketch

```go
func Middleware(resolver Resolver) func(http.Handler) http.Handler
func RequireMiddleware() func(http.Handler) http.Handler
```

`Middleware` resolves and attaches a tenant if present. `RequireMiddleware`
rejects requests when no tenant exists.

### Canopy Demo

Add a route group that requires a tenant:

```text
GET /example/whoami-tenant
```

Response:

```json
{
  "tenant_id": "...",
  "tenant_slug": "..."
}
```

### Acceptance Criteria

1. Requests without tenant headers still work for non-tenant routes.
2. Requests without tenant headers fail on tenant-required routes.
3. Requests with tenant headers work on tenant-required routes.
4. Error response includes request ID if request ID middleware exists by then.
5. Tests cover tenant present, tenant missing, and resolver error cases.

### Pay Special Attention To

1. Tenant middleware must fail closed when used in required mode.
2. Avoid global tenant state.
3. Tenant values must live in request context.
4. Do not assume auth exists yet.
5. Keep the resolver interface compatible with future auth-claim based tenancy.

### Candidate GitHub Issues

1. Add tenancy package with Tenant type and context helpers.
2. Add tenant resolver interface and header resolver.
3. Add tenant middleware and tenant-required middleware.
4. Add consistent JSON error response helper for missing tenant.
5. Add `WithTenancy()` capability option and HTTP dependency validation.
6. Add Canopy tenant demo route.
7. Add tenant middleware tests.

## Phase 3: Postgres, RLS Foundation, and Migration Modes

### Goal

Add Postgres support in a way that makes tenant isolation enforceable through RLS.

### Dependencies

1. Phase 1.
2. Phase 2.
3. `github.com/jackc/pgx/v5/pgxpool`.
4. Migration library decision: goose, golang-migrate, or Atlas.

### Important Open Decision

Choose migration engine before implementation.

Recommended starting choice: `goose`.

Reasoning:

1. Simple Go API.
2. Works well with embedded migrations.
3. Easy to understand in a small framework.
4. Less infrastructure-heavy than Atlas for MVP.

Alternative: `golang-migrate` if compatibility with existing migration workflows is
more important.

Atlas is powerful, but probably too much early unless schema-diff workflow is a
goal.

### Deliverables in Grove

1. `db` package with pgx pool setup.
2. DB config.
3. DB health/readiness check.
4. `TenantTx`.
5. `SystemTx`.
6. RLS session variable setup inside tenant transactions.
7. Migration registry.
8. Configurable migration mode.
9. Startup lifecycle integration.
10. Test helpers for Postgres if practical.
11. `WithPostgres()` and `WithMigrations()` capability options, with
    migrations depending on Postgres.

### DB Config

Environment variables:

```text
DATABASE_URL=postgres://...
DATABASE_MAX_CONNS=10
DATABASE_MIN_CONNS=0
DATABASE_CONNECT_TIMEOUT=5s
```

### Migration Config

Use explicit modes instead of booleans:

```text
GROVE_MIGRATIONS=off
GROVE_MIGRATIONS=validate
GROVE_MIGRATIONS=up
```

Meaning:

1. `off`: do nothing at startup.
2. `validate`: verify migrations are current; fail startup/readiness if not.
3. `up`: run migrations automatically during startup.

Canopy should use:

```text
GROVE_MIGRATIONS=up
```

When implementing DB and migration config, reconsider whether hand-rolled env
loading is still the right tradeoff. `github.com/caarlos0/env/v11` is a good
candidate if typed values, defaults, required settings, and enum validation make
manual parsing repetitive. Keep Grove's public config provider API independent
of the parsing library either way.

### Database API Sketch

```go
type Database struct {
	pool *pgxpool.Pool
}

func (d *Database) Pool() *pgxpool.Pool

func (d *Database) TenantTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error
func (d *Database) SystemTx(ctx context.Context, reason string, fn func(ctx context.Context, tx pgx.Tx) error) error
```

`TenantTx` should:

1. Require a tenant in context.
2. Begin a transaction.
3. Set the tenant ID with a transaction-local Postgres setting.
4. Execute the callback.
5. Commit on success.
6. Roll back on error or panic.

Tenant setting SQL:

```sql
select set_config('app.tenant_id', $1, true);
```

The third argument must be `true` so the setting is local to the transaction.

`SystemTx` should:

1. Be visibly different from tenant access.
2. Require a non-empty reason string.
3. Log the reason.
4. Never set `app.tenant_id`.

This makes tenant bypasses intentional and searchable.

### RLS Prelude

Provide a Grove migration prelude that services can include or that Grove can
register automatically:

```sql
create schema if not exists grove;

create or replace function grove.current_tenant_id()
returns uuid
language sql
stable
as $$
	select nullif(current_setting('app.tenant_id', true), '')::uuid
$$;
```

Service migration example:

```sql
create table example_widgets (
	id uuid primary key,
	tenant_id uuid not null,
	name text not null,
	created_at timestamptz not null default now()
);

alter table example_widgets enable row level security;
alter table example_widgets force row level security;

create policy example_widgets_tenant_isolation on example_widgets
	using (tenant_id = grove.current_tenant_id())
	with check (tenant_id = grove.current_tenant_id());
```

### Canopy Demo Resource

Add a tenant-scoped resource:

```text
POST /example/widgets
GET /example/widgets
GET /example/widgets/{id}
```

Keep fields minimal:

```json
{
  "id": "...",
  "name": "demo widget"
}
```

All widget routes should require a tenant.

### Acceptance Criteria

1. Grove connects to Postgres with `pgxpool`.
2. Readiness fails if Postgres is unavailable and Postgres capability is enabled.
3. Canopy can auto-run migrations.
4. Canopy can create a widget for tenant A.
5. Canopy can list tenant A widgets.
6. Tenant B cannot see tenant A widgets.
7. Missing tenant cannot access widget routes.
8. `TenantTx` fails when no tenant is present.
9. `SystemTx` requires a reason.
10. Tests prove RLS isolation, not only handler filtering.

### Pay Special Attention To

1. RLS should be the real isolation boundary.
2. Do not rely only on `where tenant_id = $1` in queries.
3. Always use `alter table ... force row level security`.
4. Ensure the DB user used by the app is subject to RLS. Table owners can bypass
   RLS unless forced.
5. Never set tenant state outside the transaction.
6. Do not use connection-level tenant settings without resetting them.
7. Transaction helper must roll back on panic.
8. Migration startup order matters: migrations must run after DB connect and
   before HTTP serving readiness.
9. Decide whether startup should fail if `GROVE_MIGRATIONS=validate` detects
   pending migrations.
10. Tests should use two tenants and attempt cross-tenant reads.

### Candidate GitHub Issues

1. Choose and document migration engine.
2. Add Postgres config and pgxpool connection.
3. Add `WithPostgres()` capability option.
4. Add DB health/readiness check.
5. Implement `TenantTx` with transaction-local tenant setting.
6. Implement `SystemTx` with required reason logging.
7. Add Grove RLS SQL prelude.
8. Implement migration registry.
9. Implement migration mode config and `WithMigrations()` capability option.
10. Wire Canopy automatic migrations.
11. Add Canopy tenant-scoped widgets table with RLS.
12. Add Canopy widget routes using `TenantTx`.
13. Add RLS isolation integration tests.

## Phase 4: Testkit Foundation

### Goal

Make service tests pleasant and security-sensitive.

### Dependencies

1. Phase 1.
2. Phase 2.
3. Phase 3 if DB test helpers are included.

### Deliverables in Grove

1. `grovetest` or `testkit` package name decision.
2. Test harness constructor.
3. In-memory HTTP server/test client.
4. Deterministic config injection.
5. Fake tenant resolver.
6. Fake auth placeholder.
7. Fake mailer placeholder.
8. Optional test Postgres setup.
9. Helpers for making tenant-scoped requests.

### Package Name Decision

Recommended package:

```go
package grovetest
```

Import path:

```go
github.com/kusold/grove/grovetest
```

This avoids overloading the term `testkit` in public examples while still making
the purpose obvious.

### API Sketch

```go
func New(t testing.TB, module grove.Module, opts ...Option) *Harness

type Harness struct {
	// private fields
}

func (h *Harness) Client() *http.Client
func (h *Harness) URL(path string) string
func (h *Harness) Request() *RequestBuilder
func (h *Harness) Tenant(id string) tenancy.Tenant
func (h *Harness) Close()
```

Request helper sketch:

```go
h.Request().
	Tenant(tenant).
	PostJSON("/example/widgets", map[string]any{"name": "demo"})
```

### DB Test Options

There are two viable approaches:

1. Testcontainers.
2. Docker Compose or external test database.

Recommended early approach: allow an external `TEST_DATABASE_URL` first, then add
Testcontainers later. This avoids a large dependency and keeps early CI simpler.

Environment:

```text
TEST_DATABASE_URL=postgres://...
```

### Acceptance Criteria

1. Canopy integration tests can start the app in-process.
2. Tests can issue HTTP requests without binding to `:8080`.
3. Tests can inject tenant context through headers or fake resolver.
4. Tests can run with deterministic config.
5. DB tests can run migrations.
6. Cross-tenant isolation tests are easy to write.

### Pay Special Attention To

1. Test helpers must never be enabled in production runtime by accident.
2. Avoid global mutable test state.
3. Make cleanup automatic through `t.Cleanup`.
4. Tests should not require port 8080.
5. Make request/response assertion helpers useful but not a custom testing DSL.
6. Keep testkit APIs small until Canopy demonstrates repeated patterns.

### Candidate GitHub Issues

1. Decide test package name and public shape.
2. Implement in-process Grove test harness.
3. Implement test HTTP client and request builder.
4. Implement fake tenant helper.
5. Add deterministic test config support.
6. Add test database support via `TEST_DATABASE_URL`.
7. Add Canopy integration tests using Grove test harness.

## Phase 5: OpenAPI for JSON Routes

### Goal

Strongly encourage OpenAPI-backed JSON routes while preserving support for HTML
and htmx-style services.

### Dependencies

1. Phase 1.
2. Canopy JSON route to document.
3. `github.com/oapi-codegen/oapi-codegen/v2` tooling.

### Deliverables in Grove

1. OpenAPI registry.
2. `/openapi.json` route when capability is enabled.
3. Optional `/docs` placeholder or disabled docs route.
4. Documentation for JSON API conventions.
5. Optional test helper to check JSON routes are documented.
6. `WithOpenAPI()` capability option that depends on `WithHTTP()`.

### Deliverables in Canopy

1. `internal/canopy/openapi/openapi.yaml`.
2. Generated Go types.
3. One OpenAPI-backed JSON handler.
4. `/openapi.json` serves the spec.

### Route Convention

Recommended service layout:

```text
internal/canopy/
  api/
    handlers.go
    routes.go
  web/
    handlers.go
    routes.go
  openapi/
    openapi.yaml
    generated.gen.go
```

`api` is for JSON/OpenAPI-backed routes. `web` is for HTML, `templ`, `htmx`, etc.

### OpenAPI Policy

Do not enforce OpenAPI globally. Instead:

1. Document that JSON routes should normally be OpenAPI-backed.
2. Provide helpers that make compliance easy.
3. Allow services to opt into stricter checks later.

Possible future option:

```go
grove.WithOpenAPI(openapi.WarnOnUndocumentedJSON())
```

Do not implement this warning mode until there is a concrete need.

### Acceptance Criteria

1. Canopy has an OpenAPI spec for at least one JSON route.
2. Generated types compile.
3. Handler implementation uses generated request/response types where practical.
4. `/openapi.json` returns the Canopy spec.
5. HTML route support remains unaffected.

### Pay Special Attention To

1. Do not require all routes to be OpenAPI-backed.
2. Avoid complicated spec merging in v1.
3. Serving one spec per service is enough.
4. Keep generated files out of Grove.
5. Make generation reproducible with `go generate` or `make generate`.
6. Do not make OpenAPI registration happen through hidden file discovery.

### Candidate GitHub Issues

1. Add Grove OpenAPI registry.
2. Add `WithOpenAPI()` capability option and HTTP dependency validation.
3. Add `/openapi.json` serving capability.
4. Add Canopy OpenAPI spec.
5. Add oapi-codegen generation setup.
6. Refactor one Canopy JSON handler to use generated types.
7. Document JSON API and HTML route conventions.

## Phase 6: Authentication and Identity

### Goal

Add authentication without overbuilding authorization.

### Dependencies

1. Phase 1.
2. Phase 2.
3. `github.com/coreos/go-oidc/v3/oidc` or equivalent.
4. `golang.org/x/oauth2` if login flows are needed.

### Deliverables in Grove

1. `auth` package.
2. Identity context.
3. Auth provider interface.
4. Fake auth provider for tests.
5. OIDC bearer-token validation provider.
6. Middleware that fails closed.
7. Hook for mapping claims to tenant resolution.
8. `WithOIDC()` capability option that depends on `WithHTTP()`.

### Identity Sketch

```go
type Identity struct {
	Subject   string
	Email     string
	Name      string
	Claims    map[string]any
	TenantIDs []string
}
```

### Provider Sketch

```go
type Provider interface {
	Middleware() func(http.Handler) http.Handler
	FromContext(ctx context.Context) (Identity, bool)
	Require(ctx context.Context) (Identity, error)
}
```

### Authorization Policy

Grove owns authentication. Services own domain authorization.

Provide a small optional interface later if needed:

```go
type Authorizer interface {
	Can(ctx context.Context, action string, resource any) error
}
```

Do not build RBAC in v1.

### Acceptance Criteria

1. Handlers can require identity.
2. Missing/invalid auth fails closed on auth-required routes.
3. OIDC tokens verify issuer, audience, expiry, and signature.
4. Tests can inject fake identity.
5. Tenant resolution can use identity claims where configured.

### Pay Special Attention To

1. Do not log tokens.
2. Do not log all claims by default.
3. Validate issuer and audience.
4. Cache OIDC provider metadata/JWKS safely through the library.
5. Auth middleware order matters. Usually request ID, recovery, logging, auth,
   tenant, handler.
6. Tenant claims should not be final authorization by themselves.

### Candidate GitHub Issues

1. Add auth identity type and context helpers.
2. Add auth provider interface.
3. Add fake auth provider for tests.
4. Add OIDC bearer-token provider.
5. Add `WithOIDC()` capability option and HTTP dependency validation.
6. Add auth-required middleware.
7. Add claim-based tenant resolver.
8. Add Canopy auth demo route.

## Phase 7: Jobs with River

### Goal

Add background jobs using River without building a custom queue.

### Dependencies

1. Phase 3.
2. `github.com/riverqueue/river`.

### Deliverables in Grove

1. Jobs registry.
2. River client and worker lifecycle integration.
3. Job registration API.
4. Enqueue helper.
5. Transactional enqueue support.
6. Tenant-aware job wrapper.
7. Test mode.
8. `WithJobs()` capability option that depends on `WithPostgres()`.

### API Sketch

```go
type Registry struct {
	// wraps River client/workers
}

func (r *Registry) Register(workers ...Worker) error
func (r *Registry) Enqueue(ctx context.Context, job Job) error
```

Exact API should align with River's actual worker and args model. Avoid wrapping
River so tightly that useful River features are hidden.

### Tenant-Aware Jobs

Tenant-scoped job args should include tenant ID:

```go
type TenantJob struct {
	TenantID string `json:"tenant_id"`
}
```

Before executing a tenant job, Grove should restore tenant context. Any DB work in
the job should then use `TenantTx`.

### Acceptance Criteria

1. Canopy registers one job.
2. Canopy can enqueue the job from an HTTP route.
3. Worker runs during local development.
4. Worker shuts down gracefully.
5. Tenant context is restored for tenant-scoped jobs.
6. Tests can execute jobs deterministically.

### Pay Special Attention To

1. Jobs require Postgres.
2. Transactional enqueue is valuable; do not lose it behind abstraction.
3. Tenant ID in job args is sensitive enough to avoid noisy logging.
4. Job retries and idempotency should be documented.
5. Worker shutdown should be part of lifecycle stop.

### Candidate GitHub Issues

1. Add jobs capability dependency on Postgres.
2. Add `WithJobs()` capability option and Postgres dependency validation.
3. Add River client setup.
4. Add River worker lifecycle integration.
5. Add Grove jobs registry.
6. Add tenant-aware job wrapper.
7. Add Canopy demo job.
8. Add deterministic job test support.

## Phase 8: Email

### Goal

Add email through a small provider abstraction.

### Dependencies

1. Phase 1.
2. Phase 4 for fake mailer test ergonomics.

### Deliverables in Grove

1. `mail` package.
2. Mailer interface.
3. Console mailer.
4. Fake mailer.
5. Optional SMTP provider.
6. Test assertions for sent email.
7. `WithMail()` capability option.

### API Sketch

```go
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

type Message struct {
	To      []Address
	From    *Address
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
}

type Address struct {
	Email string
	Name  string
}
```

### Acceptance Criteria

1. Canopy can send an email through Grove.
2. Local development can use console mailer.
3. Tests can use fake mailer and inspect messages.
4. Missing production provider config fails clearly.

### Pay Special Attention To

1. Do not build a template engine yet.
2. Do not pick a production vendor too early.
3. Avoid logging full email bodies in production.
4. Email may become tenant-branded later; keep context available.

### Candidate GitHub Issues

1. Add mail package and message types.
2. Add console mailer.
3. Add fake mailer.
4. Add `WithMail()` capability and config.
5. Add Canopy email demo path or job.
6. Add mailer tests.

## Phase 9: Observability

### Goal

Provide consistent logs, request IDs, traces, metrics, and instrumentation hooks.

### Dependencies

1. Phase 1.
2. Phase 3 for DB instrumentation.
3. Phase 7 for job instrumentation.
4. OpenTelemetry Go packages.

### Deliverables in Grove

1. Request ID middleware.
2. Panic recovery middleware.
3. Request logging middleware.
4. Structured log attributes for service, environment, version.
5. Tenant and user log attributes where safe.
6. OpenTelemetry setup.
7. HTTP tracing.
8. DB instrumentation where feasible.
9. Job instrumentation.
10. Metrics exporter or endpoint.
11. `WithObservability()` capability option.

### Log Fields

Default fields:

```text
service
environment
version
request_id
tenant_id
user_subject
```

Only include tenant/user fields when available. Never log secrets.

### Acceptance Criteria

1. Every request gets a request ID.
2. Error responses include request ID.
3. Request logs include service and request ID.
4. Panic recovery does not expose stack traces in production.
5. Traces can be exported locally.
6. Job logs include job type and outcome.

### Pay Special Attention To

1. Observability should not be all-or-nothing.
2. OTel exporter config should be optional.
3. Panic recovery should be safe by default.
4. Avoid high-cardinality metric labels.
5. Do not put raw path IDs into metric names.
6. Do not log request/response bodies by default.

### Candidate GitHub Issues

1. Add request ID middleware.
2. Add panic recovery middleware.
3. Add request logging middleware.
4. Add structured logger context helpers.
5. Add `WithObservability()` capability option.
6. Add OpenTelemetry config and setup.
7. Add HTTP tracing.
8. Add DB instrumentation.
9. Add job instrumentation.
10. Add local observability documentation.

## Phase 10: CLI and Service Generation

### Goal

Create a small CLI only after Grove and Canopy establish stable conventions.

### Dependencies

1. Phase 1.
2. Phase 3 if migrations are generated.
3. Phase 5 if OpenAPI files are generated.

### Deliverables

1. `svc new <name>`.
2. `svc doctor`.
3. `svc generate`.
4. `svc migrate up`.
5. `svc test`.

### Recommendation

Do not implement the CLI too early. Build Canopy by hand first. Once Canopy's
layout feels good, encode that into `svc new`.

### Generated Files for `svc new`

Keep this minimal:

```text
main.go
internal/<name>/module.go
internal/<name>/api/routes.go
internal/<name>/openapi/openapi.yaml
internal/<name>/migrations/001_init.sql
README.md
```

Do not generate Grove internals.

### Acceptance Criteria

1. `svc new demo` creates a minimal service.
2. Generated service imports Grove as dependency.
3. Generated service starts and serves health routes.
4. Generated code is small enough to review easily.

### Pay Special Attention To

1. Generators can freeze bad conventions.
2. Avoid large copied templates.
3. Make generated service code easy to delete or modify.
4. The CLI should not become required for normal Go development.

### Candidate GitHub Issues

1. Design service template after Canopy stabilizes.
2. Implement `svc new`.
3. Implement `svc doctor`.
4. Implement `svc generate`.
5. Implement migration command.
6. Document CLI workflow.

## Cross-Cutting Error Handling

Grove should have consistent JSON errors for framework-generated failures:

```json
{
  "error": {
    "code": "tenant_required",
    "message": "tenant is required",
    "request_id": "..."
  }
}
```

Suggested error package:

```go
package apperr

type Error struct {
	Code       string
	Message    string
	StatusCode int
	Cause      error
}
```

Helpers:

```go
func NotFound(message string) *Error
func InvalidArgument(message string) *Error
func Unauthenticated() *Error
func PermissionDenied() *Error
func TenantRequired() *Error
func Internal(err error) *Error
```

Pay attention to separation:

1. Grove-generated errors should be consistent.
2. Service domain errors should be easy to convert.
3. Internal causes should be logged, not exposed.
4. Production responses must not include stack traces.

## Open Decisions

These still need explicit decisions before or during implementation.

1. Go version:
   The workspace currently uses `go 1.25.8`. Decide whether to keep that for now
   or move to the current stable Go version before implementation.

2. Migration engine:
   Choose `goose`, `golang-migrate`, or Atlas. Recommendation: start with
   `goose`.

3. Test database strategy:
   Choose external `TEST_DATABASE_URL` first, Testcontainers first, or both.
   Recommendation: support `TEST_DATABASE_URL` first and add Testcontainers later.

4. Package naming:
   Decide between `grovetest`, `testkit`, or `platformtest`. Recommendation:
   `grovetest`.

5. Config library:
   Decide whether to hand-roll environment loading initially or use a library.
   Recommendation: hand-roll Phase 1 config, then reconsider after capability
   config gets repetitive.

6. SQL generation:
   Decide when to introduce `sqlc`. Recommendation: use hand-written pgx for the
   first Canopy widget resource, then introduce `sqlc` when query count grows.

7. UUID dependency:
   Decide whether to use `github.com/google/uuid`, `github.com/gofrs/uuid`, or
   strings at the Grove API boundary. Recommendation: strings at Grove boundaries,
   service/domain code may use UUID types.

8. Production migration behavior:
   Decide whether `GROVE_MIGRATIONS=validate` fails startup or only readiness.
   Recommendation: fail startup for simple deploy semantics.

9. Error package location:
   Decide whether errors live at `grove/apperr`, `grove/errors`, or root helpers.
   Recommendation: `apperr` to avoid collision with standard `errors`.

10. Internal package boundaries:
   Decide how much of Grove implementation should live under `internal/`.
   Recommendation: keep public packages explicit and put builder/runtime internals
   under `internal/` only when they are clearly not public.

## Dependency Introduction Order

Add dependencies only when needed:

1. Phase 1: `chi`.
2. Phase 3: `pgx`, migration engine.
3. Phase 5: `oapi-codegen` tooling.
4. Phase 6: OIDC library.
5. Phase 7: River.
6. Phase 9: OpenTelemetry packages.

Avoid adding River, OIDC, OTel, and OpenAPI dependencies during Phase 1.

## Suggested Issue Labels

Use labels like:

```text
area:docs
area:http
area:config
area:lifecycle
area:tenancy
area:postgres
area:migrations
area:testkit
area:openapi
area:auth
area:jobs
area:mail
area:observability
area:canopy
type:adr
type:feature
type:test
type:docs
priority:p0
priority:p1
priority:p2
```

## Suggested Epic Order

1. Documentation and ADR foundation.
2. Core Grove runtime.
3. Canopy HTTP demo.
4. Tenancy context and HTTP middleware.
5. Postgres and RLS.
6. Test harness.
7. OpenAPI.
8. Auth.
9. Jobs.
10. Email.
11. Observability.
12. CLI.

## Definition of Done for Early MVP

The early MVP is complete when:

1. Canopy imports Grove instead of copying framework code.
2. Canopy starts with `grove.Main`.
3. Canopy exposes `/healthz` and `/readyz`.
4. Canopy exposes a simple chi route.
5. Grove has tenant context and tenant-required middleware.
6. Canopy has a tenant-required route.
7. Grove connects to Postgres.
8. Grove can run Canopy migrations in `up` mode.
9. Canopy has a tenant-scoped table protected by RLS.
10. Tests prove cross-tenant isolation.
11. Grove has a basic test harness for Canopy integration tests.
12. Canopy serves at least one OpenAPI-backed JSON route.

## Implementation Guidance for Future Agents

1. Work phase by phase.
2. Do not skip Canopy updates.
3. Do not widen public API without a Canopy use case.
4. Prefer explicit startup errors over nil capability access.
5. Keep service `main.go` tiny.
6. Keep generated code minimal.
7. Write tests for behavior, especially tenancy and lifecycle.
8. Treat tenant isolation as a security feature, not convenience plumbing.
9. Use Postgres RLS to prove isolation.
10. Do not build an ORM.
11. Do not build a custom job queue.
12. Do not build custom OIDC validation.
13. Do not hide chi.
14. Avoid clever dependency injection.
15. Document any new public API as soon as it is introduced.

# ADR 0001: Grove as a Dependency-First Framework

## Status

Accepted.

## Context

Grove is an internal Go web framework and service platform. It is developed
alongside Canopy, an example service in the same Go workspace.

The earliest design choice is whether Grove should be a copied service template,
a CLI-generated scaffold, or a framework that services import as a normal Go
dependency. This choice shapes every later API decision: service layout, runtime
startup, cross-cutting infrastructure, testing, upgrades, and how much code each
service owns.

Grove needs to provide shared infrastructure for HTTP, configuration, logging,
lifecycle management, health checks, tenancy, database access, migrations,
OpenAPI integration, jobs, mail, observability, and test helpers. Services should
remain focused on business behavior and service-specific wiring.

## Decision

Grove will be a dependency-first framework.

Services import Grove and start through a small public runtime API, eventually
shaped like:

```go
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

Each service provides a module:

```go
type Module interface {
	Name() string
	Register(ctx context.Context, app *App) error
}
```

The module registers service behavior through explicit Grove registries exposed
from `*grove.App`, such as HTTP routes, health checks, migrations, tenancy, and
future framework capabilities.

Grove owns shared runtime concerns. Services own domain behavior.

Generated service code, when it exists, should stay minimal and should call into
Grove instead of copying Grove internals. A future CLI may create small service
entrypoints and conventional folders, but it must not become the primary
abstraction or a source of duplicated framework code.

Canopy will remain the proving service for Grove APIs. Public Grove APIs should
be introduced only when Canopy or another real service path demonstrates the
need.

## Consequences

Grove can evolve framework behavior in one place and services can receive fixes
through normal dependency updates.

Service `main.go` files stay small, reviewable, and focused on capability
selection.

Cross-cutting infrastructure has a clear owner. Grove owns lifecycle,
configuration, HTTP defaults, tenancy plumbing, database helpers, migrations,
observability hooks, and test support. Services own handlers, domain models,
service-specific routes, and domain authorization.

Capability access must be explicit and hard to misuse. If a service requires a
capability that was not enabled, Grove should fail with a clear startup or
registration error rather than returning `nil` or silently skipping behavior.

Framework API design must stay conservative. Grove should prefer small explicit
registries and boring Go interfaces over hidden magic, global state, or a
dependency injection container.

Canopy must exercise real Grove paths. If Canopy bypasses Grove for common
service concerns, that is evidence that Grove's API is missing or awkward.

## Alternatives Considered

### Copied Service Template

A copied template would make the first service quick to start, but every service
would then own its own copy of framework infrastructure. Fixes to lifecycle,
tenancy, migrations, observability, and security-sensitive behavior would have to
be manually propagated.

This is a poor fit for Grove because tenant isolation and runtime behavior should
be centrally maintained.

### CLI-First Code Generation

A CLI-generated scaffold can be useful later, once Grove and Canopy establish
stable conventions. It should not be the primary framework model.

If generation comes first, it can freeze weak early decisions and encourage large
generated files that services are afraid to edit. Grove should establish the
runtime and public APIs first, then encode proven conventions into a small CLI if
that remains useful.

### Application Platform Hidden Behind Configuration

Grove could hide most service behavior behind configuration and discovery. That
would reduce boilerplate, but it would also obscure startup order, capability
dependencies, routing, and failure modes.

Grove should instead use explicit Go registration and clear runtime errors.

## Open Questions

This ADR does not choose specific implementations for future capabilities such
as the migration engine, test database strategy, config library, SQL generation,
or CLI command shape. Those decisions should be captured separately when their
implementation phase begins.

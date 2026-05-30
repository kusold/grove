# ADR 0002: chi as Public HTTP API

## Status

Accepted.

## Context

Grove needs a small, idiomatic HTTP API for service route registration,
middleware, route groups, mounted handlers, health endpoints, tenant-required
routes, and future OpenAPI integration.

Go's standard `net/http` package is the right foundation for handlers, but it
does not provide enough routing structure by itself. Grove could hide a router
behind its own abstraction, but doing so would require copying common router
features, documenting Grove-specific equivalents, and adapting middleware from
the broader Go ecosystem.

The implementation plan already treats `chi` as foundational to Grove HTTP
services. Canopy should exercise the same routing path that real services will
use.

## Decision

Grove will use `github.com/go-chi/chi/v5` as the HTTP router for Grove HTTP
services, and `chi` may appear in Grove's public HTTP API.

The Grove HTTP registry will be backed by `chi`. It should make common route
registration convenient while intentionally exposing `chi.Router` where that is
the most direct and idiomatic API:

```go
func (m Module) Register(ctx context.Context, app *grove.App) error {
	app.HTTP().Route("/example", func(r chi.Router) {
		r.Get("/hello", handlers.Hello)
	})

	return nil
}
```

The registry may also provide narrow convenience methods for common operations,
such as `Get`, `Post`, `Use`, `Mount`, `Route`, and `Router`, but those methods
should remain thin wrappers over chi rather than a separate routing model.

Services should still register routes through Grove's HTTP registry. That gives
Grove one clear place to install framework middleware, health checks, tenancy
enforcement, observability hooks, OpenAPI-related routes, and test helpers.

## Consequences

Services can use familiar chi route groups, middleware, mounts, URL parameters,
and ecosystem integrations without waiting for Grove to mirror every feature.

Grove's HTTP API stays smaller because it does not need to invent a router
facade or maintain adapters for multiple routers.

`chi` becomes part of Grove's public API surface. Grove must treat chi version
changes as API-affecting changes and avoid surprising services with router
behavior changes.

Grove HTTP capabilities should compose with chi instead of bypassing it. Tenant
middleware, auth middleware, observability middleware, and route documentation
hooks should be designed to work naturally in chi route groups.

Exposing chi does not mean services should construct the whole Grove HTTP server
themselves. Grove still owns runtime startup, capability wiring, lifecycle,
health/readiness routes, framework defaults, and cross-cutting middleware.

## Alternatives Considered

### Hide chi Behind a Grove Router Interface

A Grove-owned router interface would reduce the visible dependency on chi, but
it would quickly become a partial copy of chi's API. It would also make common Go
middleware and route-group patterns harder to use.

This adds indirection without a clear benefit for Grove's early services.

### Use Only net/http

Using only `net/http` would avoid a router dependency, but it would push route
groups, middleware ordering, URL parameters, mounted handlers, and nested route
composition into Grove or individual services.

That would produce more framework code and less predictable service code.

### Support Multiple Routers

Grove could define an adapter layer for several routers, but there is no current
Canopy use case requiring router portability.

Supporting multiple routers now would widen the public API before Grove has
proven its first HTTP service path.

## Open Questions

The exact first-phase method set for the HTTP registry should be finalized while
implementing the Phase 1 HTTP skeleton.

OpenAPI conventions for documenting JSON routes should be captured separately
when the OpenAPI phase begins.

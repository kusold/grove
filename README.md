# grove

`grove` is a Go web framework and service platform. It is designed as a
**dependency, not a copied template** — services import Grove and compose
capabilities through options.

This repository contains the core framework code and APIs. See
[canopy](https://github.com/kusold/canopy) for the example service that drives
Grove's API design.

## Documentation

- [Implementation plan](docs/implementation-plan.md) — phased roadmap, target
  APIs, and acceptance criteria.
- [Environment variables](docs/environment.md) — generated configuration
  reference.

## Testing

Fast checks should run with:

```sh
go test -short ./...
```

Integration tests use Testcontainers and must call `integrationtest.Require(t)`
or a helper that calls it. They run during the full suite:

```sh
go test ./...
```

### Architecture Decision Records (ADRs)

Key design decisions are documented in `docs/adr/`:

| # | Decision | Summary |
|---|----------|--------|
| 1 | [Grove as a dependency-first framework](docs/adr/0001-grove-as-framework.md) | Grove is imported, not forked or templated |
| 2 | [chi as public HTTP API](docs/adr/0002-chi-as-public-api.md) | chi is exposed through Grove's HTTP registry |
| 3 | [Postgres RLS-backed tenancy](docs/adr/0003-postgres-rls-tenancy.md) | Row-level security enforces tenant isolation |
| 4 | [Configurable migrations](docs/adr/0004-configurable-migrations.md) | Migration behavior is opt-in with explicit modes |

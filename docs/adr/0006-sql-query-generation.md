# ADR 0006: SQL Query Generation

## Status

Accepted.

## Context

Grove is expected to support Postgres-backed services with tenant-aware database
access, migrations, tests, and service-owned application queries. ADR 0003
established Postgres RLS as the isolation boundary so that tenant safety does
not depend only on every query author remembering a `tenant_id` predicate.

Even with RLS, service query code still needs to be maintainable. Grove and
Canopy will need enough SQL for demo resources, auth, jobs, and other
Postgres-backed capabilities that hand-written row scanning would become noisy
and repetitive.

The implementation plan left SQL generation timing as an open Phase 3 decision.
The intended service query style is now settled.

## Decision

Grove services will use `sqlc` for service-owned application queries once
Postgres-backed resources are introduced.

`sqlc` configuration and generated query packages should live with the service
or module that owns the queries. Canopy should use this path for its
Postgres-backed resources so it proves the Grove service authoring workflow.

Grove's public APIs should not expose `sqlc` generated types. Services may use
their generated query packages internally, while Grove continues to expose
framework-level abstractions for tenancy, transactions, migrations, and other
cross-cutting database concerns.

Grove framework internals may still use small hand-written SQL when that is the
clearer fit, especially for infrastructure operations such as setting tenant
transaction context, health checks, migration bookkeeping, or other queries that
do not benefit from generated CRUD-style accessors.

## Consequences

Service query code gets compile-time checked SQL, typed parameters, and typed
row results without introducing an ORM or hiding the SQL that matters for RLS,
indexes, constraints, and query planning.

Migration files remain the schema source for database changes. `sqlc` should
consume the relevant schema and query files from the owning service or module,
but it does not replace Grove's migration registry or startup migration modes.

Generated code becomes part of the service implementation surface. Repositories
that use `sqlc` need a consistent generation command and review convention so
query changes, generated code, and migrations stay together.

Grove should keep `sqlc` out of framework-facing API contracts. This allows a
service to reorganize generated packages without forcing Grove API churn, and it
keeps the framework usable for services that only need Grove's database
capabilities at the boundary.

## Alternatives Considered

### Hand-Written `pgx` Queries Everywhere

Hand-written `pgx` is simple and appropriate for small framework plumbing, but
using it for all service resources would repeat scanning code and defer many SQL
shape errors until runtime.

### ORM

An ORM would reduce some boilerplate, but Grove's Postgres/RLS design benefits
from explicit SQL. Service authors should be able to see and tune the exact
queries they run.

### Query Builder

A query builder would keep more code in Go, but it is a weaker fit for Grove's
SQL-first migration and RLS model. It also does not provide the same direct
schema/query checking that `sqlc` provides.

## Open Questions

The first `sqlc` implementation should decide the exact package layout,
generation command, and whether generated files are committed for Canopy.

The Postgres implementation should decide how Grove-owned infrastructure queries
are organized when they grow beyond small hand-written statements.

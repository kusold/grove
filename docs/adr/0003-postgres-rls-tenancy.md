# ADR 0003: Postgres RLS-Backed Tenancy

## Status

Accepted.

## Context

Grove treats multitenancy as a first-class concern for HTTP services. Tenant
identity should be resolved at the edge, carried through request context, and
available to service code without global state.

Tenant isolation is security-sensitive. Handler filters such as
`where tenant_id = $1` are useful, but they are easy to forget, bypass, or apply
inconsistently as services grow. Grove needs a model where the database enforces
the isolation boundary even when service queries are imperfect.

Grove is expected to support Postgres-backed services, migrations, test helpers,
background jobs, auth-derived tenant resolution, and operational tools over
time. The tenancy model should be strong enough for those later capabilities
without requiring every service to invent its own tenant plumbing.

## Decision

Grove will use Postgres row-level security as the tenant isolation boundary for
tenant-scoped data.

HTTP tenancy starts in Grove middleware. A tenant resolver attaches a tenant to
the request context when one is present, and tenant-required routes fail closed
when no tenant is available.

Tenant-scoped database work must go through a Grove helper shaped like
`TenantTx`. That helper will require a tenant in context, begin a transaction,
set a transaction-local Postgres setting, run the service callback, and then
commit or roll back.

The tenant setting will use a Grove-owned key:

```sql
select set_config('app.tenant_id', $1, true);
```

The setting must be transaction-local. Grove must not rely on connection-level
tenant state that could leak through a pooled connection.

Grove will provide an RLS prelude that exposes the current tenant to policies:

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

Tenant-scoped service tables should include a `tenant_id` column, enable and
force row-level security, and define policies in terms of
`grove.current_tenant_id()`:

```sql
alter table example_widgets enable row level security;
alter table example_widgets force row level security;

create policy example_widgets_tenant_isolation on example_widgets
	using (tenant_id = grove.current_tenant_id())
	with check (tenant_id = grove.current_tenant_id());
```

Non-tenant system access must be explicit. Grove may provide a helper shaped
like `SystemTx`, but it must be visibly different from tenant access, require a
reason, and never set `app.tenant_id`.

Tests for tenant-scoped data should prove RLS behavior with at least two
tenants. It is not enough to test that handlers add tenant filters.

## Consequences

Tenant isolation becomes a database-enforced invariant instead of a convention
that every query author must remember.

Grove's tenancy API must fail closed in multiple places: missing tenants should
be rejected by tenant-required HTTP middleware, `TenantTx` should fail when no
tenant is present, and Postgres RLS should reject access when no transaction
tenant has been set.

Service migrations need to participate in the tenancy model. Tenant-scoped
tables must include a tenant key, enable RLS, force RLS, and define policies
that use Grove's current-tenant helper.

The application database role must be subject to RLS. Grove and service
documentation must call out role ownership and bypass behavior because table
owners and privileged roles can otherwise bypass RLS.

Database access patterns become more explicit. Most tenant-scoped application
work should use `TenantTx`; system or maintenance work should use a separate
path with an auditable reason.

Pooled connections require care. Tenant state may be set only for the current
transaction and must not persist beyond the transaction boundary.

## Alternatives Considered

### Query-Only Tenant Filtering

Services could rely on every query including a tenant predicate.

This is simple to start, but it makes tenant isolation depend on every future
query being written correctly. One missing predicate can become a cross-tenant
data leak.

### Application-Owned Repository Wrappers

Grove could require all database access to happen through generated or
hand-written repository wrappers that inject tenant filters.

This improves consistency, but it still keeps the final isolation guarantee in
application code. It also pushes Grove toward a heavier data-access framework
before Canopy has demonstrated that need.

### Separate Database or Schema per Tenant

A database or schema per tenant can provide a strong isolation story for some
products, but it adds provisioning, migration, connection-pool, and operational
complexity.

Grove's initial target is an internal service framework where shared Postgres
tables with RLS provide a better default tradeoff. Services that truly require
physical tenant separation should treat that as a separate architecture
decision.

### Connection-Level Tenant State

Grove could set tenant state when a connection is checked out and reset it later.

This is fragile with pooled connections because a missed reset can leak tenant
state into unrelated work. Transaction-local settings provide a clearer and
safer boundary.

## Open Questions

The exact first method signatures for `TenantTx`, `SystemTx`, and database test
helpers should be finalized during the Postgres implementation phase.

The migration engine and startup migration modes should be captured in the
separate migrations ADR.

Auth-derived tenant resolution, tenant membership checks, and admin/system data
access conventions should be documented when those capabilities are introduced.

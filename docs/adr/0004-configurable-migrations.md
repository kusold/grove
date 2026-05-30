# ADR 0004: Configurable Migrations

## Status

Accepted.

## Context

Grove is expected to own cross-cutting database infrastructure for services that
opt into Postgres. Schema migrations are part of that infrastructure, but
services do not all need the same startup behavior.

Some environments should run migrations automatically during startup. Canopy, as
the Grove demo service, should use this path so the framework exercises the
complete Postgres startup flow.

Other environments should avoid mutating the database from application startup.
Production deployments may run migrations as a separate release step, while the
application should still fail clearly if the deployed schema is not current.

Grove therefore needs migration behavior that is explicit, configurable, and
visible in startup and readiness semantics. A single hard-coded behavior would
either be too risky for production or too inconvenient for local development and
demo services.

## Decision

Grove will provide migrations as an explicit capability. `WithMigrations` will
require the Postgres capability, and migration startup work will run after
Postgres connects and before the HTTP server starts accepting traffic.

Migration behavior will be controlled by an explicit mode instead of booleans:

```text
GROVE_MIGRATIONS=off
GROVE_MIGRATIONS=validate
GROVE_MIGRATIONS=up
```

The modes mean:

1. `off`: do not inspect or run migrations at startup.
2. `validate`: verify that registered migrations are current and fail startup if
   they are not.
3. `up`: run registered migrations during startup and fail startup if they
   cannot be applied.

When the migrations capability is enabled and no mode is configured, Grove will
default to `validate`. This makes the safe behavior the default: application
startup does not mutate the database, but schema drift is still detected before
the service becomes available.

Canopy will configure:

```text
GROVE_MIGRATIONS=up
```

This keeps Canopy easy to run as a framework demo and proves Grove's automatic
startup migration path.

Grove's initial migration engine will be `goose`. Services should be able to
register migrations explicitly, including embedded SQL migrations. Grove should
not depend on hidden file discovery for service migrations.

The migration registry should support Grove-owned migrations, such as the RLS
prelude, and service-owned migrations, such as Canopy's demo tables and
policies. Migration ordering must be deterministic.

## Consequences

Migration behavior is visible in configuration and can be chosen per service or
environment.

The default mode is conservative. Enabling migrations does not automatically
grant the application permission to mutate production schema, but it does catch
pending migrations as an early startup failure.

Canopy remains frictionless for local and demo use because it can opt into
automatic `up` mode.

Startup order matters. Grove must connect to Postgres, initialize the migration
registry, apply or validate migrations according to the configured mode, and
only then report readiness for HTTP traffic.

Readiness should not report success after a failed migration check. For the
initial implementation, `validate` and `up` failures should fail startup rather
than only marking readiness unhealthy.

Grove takes a dependency on `goose` when the Postgres and migration phase begins.
That dependency becomes part of Grove's migration implementation, but services
should interact with Grove's registry rather than calling `goose` directly from
normal startup code.

## Alternatives Considered

### Always Run Migrations on Startup

Automatic startup migrations are convenient, especially for local development
and demo services, but they are too strong as the only behavior.

Some production environments require separate migration jobs, deploy gates, or
manual approval before schema changes. Grove should support those workflows
without forcing services to remove the migration capability entirely.

### Never Run Migrations on Startup

Only validating or ignoring migrations would avoid application-driven schema
changes, but it would make local development and Canopy less useful as a proof
of the complete framework path.

Grove should make automatic migrations easy when a service explicitly chooses
that behavior.

### Boolean Configuration

A setting such as `MIGRATIONS_ENABLED=true` does not distinguish "validate but do
not mutate" from "apply pending migrations."

Explicit modes are more verbose, but they make the operational behavior clear
and leave room for future modes if Canopy or production use reveals a need.

### Atlas

Atlas provides a powerful schema management workflow, especially for teams that
want schema diffing and richer migration planning.

That is more infrastructure than Grove needs for the MVP. It can be revisited if
Grove later needs schema-diff workflows.

### golang-migrate

`golang-migrate` is widely used and compatible with many existing migration
workflows.

For Grove's initial embedded-migration use case, `goose` offers a smaller and
more direct Go API. Grove can reconsider if compatibility with existing service
migration workflows becomes more important.

## Open Questions

The exact public registration API for service migrations should be finalized
during the Postgres implementation phase.

The first implementation should decide whether Grove-owned migrations and
service-owned migrations share one version table or use separate versioning
namespaces.

CLI migration commands, rollback conventions, and generated migration file
layout should be decided later if Grove adds a CLI.

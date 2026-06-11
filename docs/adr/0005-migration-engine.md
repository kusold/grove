# ADR 0005: Migration Engine

## Status

Accepted.

## Context

Grove needs a migration engine to manage database schema changes for services
that opt into the Postgres capability. ADR 0004 established that Grove will
provide configurable migration behavior (`off`, `validate`, `up`) and that
services should register migrations explicitly rather than through hidden file
discovery.

The migration engine is an implementation detail of Grove's migration registry.
Services interact with Grove's public API, not the engine directly. Even so, the
choice matters because it affects Grove's dependency footprint, how migrations
are embedded and ordered, and how easily services can adopt the framework.

Three candidates were evaluated: `goose`, `golang-migrate`, and `Atlas`.

## Decision

Grove will use `goose` as its migration engine.

`goose` will be added as a dependency of the `grove` module. Grove's migration
registry will wrap `goose` so that services register migrations through Grove's
public API rather than calling `goose` directly from normal startup code.

Grove will support embedded SQL migrations registered through the migration
registry. Migration ordering must be deterministic.

## Reasoning

### Why `goose`

1. **Simple Go API.** `goose` provides a straightforward Go API for running
   migrations programmatically. This aligns with Grove's goal of explicit
   registration over hidden file discovery.

2. **Embedded migrations.** `goose` works well with Go's `embed` package for
   embedding SQL migration files into the binary. Services can register embedded
   migration collections through Grove's registry.

3. **Small footprint.** `goose` has a minimal dependency tree. Grove should not
   carry heavy infrastructure dependencies for features it does not yet need.

4. **Clear migration semantics.** `goose` orders migrations by numeric version.
   Grove will use timestamp versions (`YYYYMMDDHHMMSS`) so parallel branches can
   add migrations without coordinating the next sequence number.

5. **SQL-first.** `goose` treats SQL migrations as first-class. Grove services
   will primarily write SQL migrations (e.g., creating tables, enabling RLS,
   adding policies), and `goose` handles this naturally.

### Why Not `golang-migrate`

`golang-migrate` is widely used and compatible with many existing migration
workflows. It supports multiple source drivers and database backends.

For Grove's use case, it is a reasonable alternative, but `goose` offers a more
direct Go API and smaller footprint for the embedded-migration pattern Grove
needs. Grove can reconsider if compatibility with existing `golang-migrate`
workflows becomes important for a consuming service.

### Why Not `Atlas`

Atlas provides a powerful schema management workflow with schema diffing, declarative
migrations, and richer migration planning.

That power comes with significant infrastructure complexity. Grove does not need
schema-diff workflows for the MVP. Atlas can be revisited if Grove later requires
more advanced schema management capabilities.

## Consequences

Grove takes a dependency on `goose`. This dependency is confined to Grove's
migration implementation. Services should not import `goose` directly for
migration registration during normal startup; they should use Grove's migration
registry.

Grove must decide how to expose migration registration through its public API.
The initial API should support:

1. Registering embedded SQL migrations from service modules.
2. Registering Grove-owned migrations such as the RLS prelude.
3. Deterministic ordering of all registered migrations.

The implementation must support the three modes from ADR 0004: `off`, `validate`,
and `up`.

## Open Questions

Whether Grove-owned migrations and service-owned migrations share one `goose`
database table or use separate versioning namespaces should be decided during
implementation.

CLI migration commands, rollback conventions, and generated migration file layout
should be decided later if Grove adds a CLI (Phase 10).

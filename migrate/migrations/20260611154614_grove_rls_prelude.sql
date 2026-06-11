-- +goose Up
-- +goose StatementBegin
-- Grove RLS Setup
-- Creates the grove schema and the current_tenant_id() helper function used by
-- Row-Level Security policies. This is the foundation for tenant-scoped data
-- isolation in all Grove services.
--
-- The function safely returns NULL when no tenant is set, which means RLS
-- policies using it will match zero rows by default (fail-closed).

create schema if not exists grove;

create or replace function grove.current_tenant_id()
returns uuid
language sql
stable
as $$
	select nullif(current_setting('app.tenant_id', true), '')::uuid
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop function if exists grove.current_tenant_id();
drop schema if exists grove;
-- +goose StatementEnd

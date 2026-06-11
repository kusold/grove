// Package migrate provides Grove's database migration registry and RLS prelude.
//
// Grove uses goose as its migration engine. Services register migrations through
// Grove rather than calling goose directly.
//
// The RLS prelude (PreludeSQL) creates the grove schema and the
// grove.current_tenant_id() helper function used by Row-Level Security policies.
// Services should apply the prelude before running their own migrations so that
// tenant-scoped tables can reference grove.current_tenant_id() in their RLS
// policies.
package migrate

import _ "github.com/pressly/goose/v3"

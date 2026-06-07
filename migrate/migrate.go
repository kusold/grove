// Package migrate provides Grove's database migration registry.
//
// Grove uses goose as its migration engine. Services will register migrations
// through Grove rather than calling goose directly.
//
// This package will be expanded in Phase 3 (Postgres, RLS Foundation, and
// Migration Modes) when the Postgres capability is implemented. For now it
// establishes the package structure and keeps the goose dependency in go.mod.
package migrate

import _ "github.com/pressly/goose/v3"

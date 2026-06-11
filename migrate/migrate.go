// Package migrate provides Grove's database migration registry.
//
// Grove uses goose as its migration engine. Services register migrations through
// Grove rather than calling goose directly.
//
// Grove-owned migrations, such as the RLS prelude, are registered before
// service-owned migrations so tenant-scoped tables can reference Grove database
// helpers in their own migration files.
//
// Each migration source (Grove-owned and service-owned) uses its own goose
// version table named <source>_db_version to avoid version number collisions
// between sources. This allows independent timestamp-based versioning per source.
package migrate

import _ "github.com/pressly/goose/v3"

// Package migrate provides Grove's database migration registry.
//
// Grove uses goose as its migration engine. Services register migrations through
// Grove rather than calling goose directly.
//
// Grove-owned migrations, such as the RLS prelude, are registered before
// service-owned migrations so tenant-scoped tables can reference Grove database
// helpers in their own migration files.
package migrate

import _ "github.com/pressly/goose/v3"

// Package migrate provides Grove's database migration registry and RLS prelude.
//
// The RLS prelude (PreludeSQL) creates the grove schema and the
// grove.current_tenant_id() helper function used by Row-Level Security policies.
// Services should apply the prelude before running their own migrations so that
// tenant-scoped tables can reference grove.current_tenant_id() in their RLS
// policies.
package migrate

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed prelude.sql
var preludeFS embed.FS

// PreludeSQL returns the raw SQL for the Grove RLS prelude. This creates the
// grove schema and the grove.current_tenant_id() helper function.
//
// Use this when you need to apply the prelude programmatically outside of
// goose's migration system.
func PreludeSQL() string {
	b, err := fs.ReadFile(preludeFS, "prelude.sql")
	if err != nil {
		// This should never happen since prelude.sql is embedded at compile time.
		panic("migrate: failed to read embedded prelude.sql: " + err.Error())
	}
	return string(b)
}

// PreludeStatements returns the prelude SQL split into individual semicolon-
// terminated statements. This is useful for drivers that do not support
// executing multiple statements in a single call.
func PreludeStatements() []string {
	return splitStatements(PreludeSQL())
}

// splitStatements splits SQL text into individual statements, stripping
// comments and empty lines. Each returned string is a single SQL statement
// without a trailing semicolon.
func splitStatements(sql string) []string {
	var stmts []string
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and comments.
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		stmts = append(stmts, trimmed)
	}
	return stmts
}

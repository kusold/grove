package migrate

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed rls_setup.sql
var rlsSetupFS embed.FS

// RLSSetupSQL returns the raw SQL that creates the grove schema and the
// grove.current_tenant_id() helper function used by Row-Level Security policies.
//
// Use this when you need to apply the RLS setup programmatically outside of
// goose's migration system.
func RLSSetupSQL() string {
	b, err := fs.ReadFile(rlsSetupFS, "rls_setup.sql")
	if err != nil {
		// This should never happen since rls_setup.sql is embedded at compile time.
		panic("migrate: failed to read embedded rls_setup.sql: " + err.Error())
	}
	return string(b)
}

// RLSSetupStatements returns the RLS setup SQL split into individual
// statements. This is useful for drivers that do not support executing
// multiple statements in a single call.
func RLSSetupStatements() []string {
	return splitStatements(RLSSetupSQL())
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

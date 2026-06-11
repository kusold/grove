package migrate

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

const groveMigrationDir = "migrations"

//go:embed migrations/*.sql
var groveMigrationFS embed.FS

// GroveMigrations returns Grove-owned migrations, including the RLS prelude.
//
// Service-owned migrations should be registered separately and run after these
// migrations so tenant-scoped tables can reference grove.current_tenant_id().
func GroveMigrations() Source {
	return Source{
		Name: "grove",
		FS:   groveMigrationFS,
		Dir:  groveMigrationDir,
	}
}

// RLSSetupSQL returns the SQL applied by Grove's RLS prelude migration.
//
// This helper exists for tests and direct setup paths until the migration runner
// is wired. Startup migration code should use GroveMigrations instead.
func RLSSetupSQL() string {
	b, err := fs.ReadFile(groveMigrationFS, "migrations/20260611154614_grove_rls_prelude.sql")
	if err != nil {
		// This should never happen since the migration is embedded at compile time.
		panic("migrate: failed to read embedded RLS prelude migration: " + err.Error())
	}
	sql, err := gooseSection(string(b), "-- +goose Up")
	if err != nil {
		panic("migrate: failed to read RLS prelude up migration: " + err.Error())
	}
	return sql
}

func gooseSection(sql, marker string) (string, error) {
	lines := strings.Split(sql, "\n")
	started := false
	inStatement := false
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == marker:
			started = true
		case !started:
			continue
		case strings.HasPrefix(trimmed, "-- +goose Down"):
			return strings.TrimSpace(strings.Join(out, "\n")) + "\n", nil
		case strings.HasPrefix(trimmed, "-- +goose StatementBegin"):
			inStatement = true
		case strings.HasPrefix(trimmed, "-- +goose StatementEnd"):
			inStatement = false
		case strings.HasPrefix(trimmed, "-- +goose"):
			return "", fmt.Errorf("unexpected goose annotation %q", trimmed)
		case inStatement:
			out = append(out, line)
		}
	}
	if !started {
		return "", fmt.Errorf("missing %s section", marker)
	}
	return "", fmt.Errorf("missing -- +goose Down section")
}

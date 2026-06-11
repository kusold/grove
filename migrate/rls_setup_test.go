package migrate

import (
	"io/fs"
	"strings"
	"testing"
)

func TestGroveMigrationsReturnsEmbeddedMigrationCollection(t *testing.T) {
	source := GroveMigrations()
	if source.Name != "grove" {
		t.Fatalf("GroveMigrations().Name = %q, want grove", source.Name)
	}
	if source.Dir != "migrations" {
		t.Fatalf("GroveMigrations().Dir = %q, want migrations", source.Dir)
	}

	matches, err := fs.Glob(source.FS, source.Dir+"/*.sql")
	if err != nil {
		t.Fatalf("glob Grove migrations: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("GroveMigrations() found %d migration files, want 1", len(matches))
	}
	if matches[0] != "migrations/20260611154614_grove_rls_prelude.sql" {
		t.Fatalf("Grove migration file = %q, want migrations/20260611154614_grove_rls_prelude.sql", matches[0])
	}
}

func TestGroveRLSMigrationUsesGooseAnnotations(t *testing.T) {
	b, err := fs.ReadFile(GroveMigrations().FS, "migrations/20260611154614_grove_rls_prelude.sql")
	if err != nil {
		t.Fatalf("read Grove RLS migration: %v", err)
	}
	sql := string(b)
	for _, want := range []string{
		"-- +goose Up",
		"-- +goose StatementBegin",
		"-- +goose StatementEnd",
		"-- +goose Down",
		"drop function if exists grove.current_tenant_id();",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("Grove RLS migration missing %q", want)
		}
	}
}

func TestRLSSetupSQLContainsSchemaCreation(t *testing.T) {
	sql := RLSSetupSQL()
	if !strings.Contains(sql, "create schema if not exists grove") {
		t.Error("RLSSetupSQL() missing 'create schema if not exists grove'")
	}
}

func TestRLSSetupSQLContainsCurrentTenantIDFunction(t *testing.T) {
	sql := RLSSetupSQL()
	if !strings.Contains(sql, "create or replace function grove.current_tenant_id()") {
		t.Error("RLSSetupSQL() missing 'create or replace function grove.current_tenant_id()'")
	}
}

func TestRLSSetupSQLContainsSetConfigReference(t *testing.T) {
	sql := RLSSetupSQL()
	if !strings.Contains(sql, "current_setting('app.tenant_id', true)") {
		t.Error("RLSSetupSQL() missing reference to current_setting('app.tenant_id', true)")
	}
}

func TestRLSSetupSQLReturnsUUID(t *testing.T) {
	sql := RLSSetupSQL()
	if !strings.Contains(sql, "returns uuid") {
		t.Error("RLSSetupSQL() missing 'returns uuid'")
	}
}

func TestRLSSetupSQLUsesStable(t *testing.T) {
	sql := RLSSetupSQL()
	if !strings.Contains(sql, "stable") {
		t.Error("RLSSetupSQL() missing 'stable' keyword")
	}
}

func TestRLSSetupSQLUsesNullifForSafeDefault(t *testing.T) {
	sql := RLSSetupSQL()
	if !strings.Contains(sql, "nullif") {
		t.Error("RLSSetupSQL() missing 'nullif' for safe NULL default when no tenant is set")
	}
}

func TestRLSSetupSQLIsNotEmpty(t *testing.T) {
	if RLSSetupSQL() == "" {
		t.Error("RLSSetupSQL() returned empty string")
	}
}

func TestRLSSetupSQLExtractsOnlyUpMigrationBody(t *testing.T) {
	sql := RLSSetupSQL()
	for _, unwanted := range []string{"+goose", "drop function", "drop schema"} {
		if strings.Contains(sql, unwanted) {
			t.Errorf("RLSSetupSQL() should not include %q", unwanted)
		}
	}
}

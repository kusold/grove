package migrate

import (
	"strings"
	"testing"
)

func TestPreludeSQLContainsSchemaCreation(t *testing.T) {
	sql := PreludeSQL()
	if !strings.Contains(sql, "create schema if not exists grove") {
		t.Error("PreludeSQL() missing 'create schema if not exists grove'")
	}
}

func TestPreludeSQLContainsCurrentTenantIDFunction(t *testing.T) {
	sql := PreludeSQL()
	if !strings.Contains(sql, "create or replace function grove.current_tenant_id()") {
		t.Error("PreludeSQL() missing 'create or replace function grove.current_tenant_id()'")
	}
}

func TestPreludeSQLContainsSetConfigReference(t *testing.T) {
	sql := PreludeSQL()
	if !strings.Contains(sql, "current_setting('app.tenant_id', true)") {
		t.Error("PreludeSQL() missing reference to current_setting('app.tenant_id', true)")
	}
}

func TestPreludeSQLReturnsUUID(t *testing.T) {
	sql := PreludeSQL()
	if !strings.Contains(sql, "returns uuid") {
		t.Error("PreludeSQL() missing 'returns uuid'")
	}
}

func TestPreludeSQLUsesStable(t *testing.T) {
	sql := PreludeSQL()
	if !strings.Contains(sql, "stable") {
		t.Error("PreludeSQL() missing 'stable' keyword")
	}
}

func TestPreludeSQLUsesNullifForSafeDefault(t *testing.T) {
	sql := PreludeSQL()
	if !strings.Contains(sql, "nullif") {
		t.Error("PreludeSQL() missing 'nullif' for safe NULL default when no tenant is set")
	}
}

func TestPreludeSQLIsNotEmpty(t *testing.T) {
	if PreludeSQL() == "" {
		t.Error("PreludeSQL() returned empty string")
	}
}

func TestPreludeSQLDoesNotContainGooseMarker(t *testing.T) {
	// The prelude is raw SQL, not a goose migration file.
	// It should not contain goose's +goose Up/Down annotations
	// since it may be applied outside of goose.
	sql := PreludeSQL()
	if strings.Contains(sql, "+goose") {
		t.Error("PreludeSQL() should not contain goose markers; it is raw SQL")
	}
}

func TestPreludeStatements(t *testing.T) {
	stmts := PreludeStatements()
	if len(stmts) < 2 {
		t.Fatalf("PreludeStatements() returned %d statements, want at least 2", len(stmts))
	}

	foundSchema := false
	foundFunction := false
	for _, s := range stmts {
		if strings.Contains(s, "create schema") {
			foundSchema = true
		}
		if strings.Contains(s, "create or replace function") {
			foundFunction = true
		}
	}
	if !foundSchema {
		t.Error("PreludeStatements() missing schema creation statement")
	}
	if !foundFunction {
		t.Error("PreludeStatements() missing function creation statement")
	}
}

func TestPreludeStatementsExcludesComments(t *testing.T) {
	for _, s := range PreludeStatements() {
		if strings.HasPrefix(s, "--") {
			t.Errorf("PreludeStatements() included comment: %q", s)
		}
	}
}

func TestPreludeStatementsExcludesEmptyStrings(t *testing.T) {
	for _, s := range PreludeStatements() {
		if s == "" {
			t.Error("PreludeStatements() included empty string")
		}
	}
}

package migrate

import (
	"strings"
	"testing"
)

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

func TestRLSSetupSQLDoesNotContainGooseMarker(t *testing.T) {
	// The setup SQL is raw SQL, not a goose migration file.
	// It should not contain goose's +goose Up/Down annotations
	// since it may be applied outside of goose.
	sql := RLSSetupSQL()
	if strings.Contains(sql, "+goose") {
		t.Error("RLSSetupSQL() should not contain goose markers; it is raw SQL")
	}
}

func TestRLSSetupStatements(t *testing.T) {
	stmts := RLSSetupStatements()
	if len(stmts) < 2 {
		t.Fatalf("RLSSetupStatements() returned %d statements, want at least 2", len(stmts))
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
		t.Error("RLSSetupStatements() missing schema creation statement")
	}
	if !foundFunction {
		t.Error("RLSSetupStatements() missing function creation statement")
	}
}

func TestRLSSetupStatementsExcludesComments(t *testing.T) {
	for _, s := range RLSSetupStatements() {
		if strings.HasPrefix(s, "--") {
			t.Errorf("RLSSetupStatements() included comment: %q", s)
		}
	}
}

func TestRLSSetupStatementsExcludesEmptyStrings(t *testing.T) {
	for _, s := range RLSSetupStatements() {
		if s == "" {
			t.Error("RLSSetupStatements() included empty string")
		}
	}
}

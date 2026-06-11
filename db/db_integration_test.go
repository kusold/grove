package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kusold/grove/internal/integrationtest"
	"github.com/kusold/grove/tenancy"
)

func TestOpenConnectsToPostgres18(t *testing.T) {
	databaseURL := integrationtest.Postgres18(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	database, err := Open(ctx, Config{
		URL:            databaseURL,
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open() returned unexpected error: %v", err)
	}
	t.Cleanup(database.Close)

	if err := database.Ping(ctx); err != nil {
		t.Fatalf("Ping() returned unexpected error: %v", err)
	}

	var versionNum string
	if err := database.Pool().QueryRow(ctx, "show server_version_num").Scan(&versionNum); err != nil {
		t.Fatalf("query server version: %v", err)
	}
	if !strings.HasPrefix(versionNum, "18") {
		t.Fatalf("server_version_num = %q, want Postgres 18", versionNum)
	}
}

// setupTestDB creates a database connection as the superuser (who owns all
// objects) and ensures the grove schema and helper function exist.
func setupTestDB(t *testing.T) *Database {
	t.Helper()

	databaseURL := integrationtest.Postgres18(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	database, err := Open(ctx, Config{
		URL:            databaseURL,
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open() returned unexpected error: %v", err)
	}
	t.Cleanup(database.Close)

	// Create grove schema and current_tenant_id helper function.
	_, err = database.Pool().Exec(ctx, `
		create schema if not exists grove;

		create or replace function grove.current_tenant_id()
		returns text
		language sql
		stable
		as $$
			select nullif(current_setting('app.tenant_id', true), '')
		$$;
	`)
	if err != nil {
		t.Fatalf("create grove schema and function: %v", err)
	}

	return database
}

// setupRLSTestDB creates a database connection as a non-superuser role
// (grove_app) that is subject to RLS policies. The superuser (grove) owns the
// tables and creates the grove_app role with appropriate grants. This setup is
// necessary because the default testcontainers user is a superuser, and
// superusers bypass RLS even with FORCE ROW LEVEL SECURITY.
//
// Returns two values:
//   - ownerDB: a Database connected as the superuser (for DDL)
//   - appDB:   a Database connected as grove_app (subject to RLS)
//   - appURL:  the connection URL for the app user
func setupRLSTestDB(t *testing.T) (ownerDB, appDB *Database) {
	t.Helper()

	databaseURL := integrationtest.Postgres18(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect as the superuser to set up the role and schema.
	ownerDB, err := Open(ctx, Config{
		URL:            databaseURL,
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open() owner: %v", err)
	}
	t.Cleanup(ownerDB.Close)

	// Create a non-superuser role that will be subject to RLS.
	_, err = ownerDB.Pool().Exec(ctx, `
		do $$
		begin
			if not exists (select from pg_roles where rolname = 'grove_app') then
				create role grove_app login password 'grove_app';
			end if;
		end
		$$;

		grant all on schema public to grove_app;
	`)
	if err != nil {
		t.Fatalf("create grove_app role: %v", err)
	}

	// Create grove schema and function (owned by superuser).
	_, err = ownerDB.Pool().Exec(ctx, `
		create schema if not exists grove;

		create or replace function grove.current_tenant_id()
		returns text
		language sql
		stable
		as $$
			select nullif(current_setting('app.tenant_id', true), '')
		$$;

		grant usage on schema grove to grove_app;
		grant usage on schema public to grove_app;
	`)
	if err != nil {
		t.Fatalf("setup grove schema: %v", err)
	}

	// Build the app user connection URL by replacing the user/password.
	appURL := strings.Replace(databaseURL, "grove:grove@", "grove_app:grove_app@", 1)

	// Grant connect on the database to the app user.
	_, err = ownerDB.Pool().Exec(ctx, "grant connect on database grove_test to grove_app")
	if err != nil {
		t.Fatalf("grant connect: %v", err)
	}

	appDB, err = Open(ctx, Config{
		URL:            appURL,
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open() app: %v", err)
	}
	t.Cleanup(appDB.Close)

	return ownerDB, appDB
}

func TestTenantTxIntegration(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	t.Run("fails when no tenant is in context", func(t *testing.T) {
		err := database.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should fail without tenant in context")
		}
		if !strings.Contains(err.Error(), "tenant") {
			t.Errorf("error = %q, want tenant-related error", err.Error())
		}
	})

	t.Run("sets app.tenant_id within transaction", func(t *testing.T) {
		tenant := tenancy.Tenant{ID: "tenant-abc", Slug: "acme"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		var tenantID string
		err := database.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "select current_setting('app.tenant_id', true)").Scan(&tenantID)
		})
		if err != nil {
			t.Fatalf("TenantTx() returned unexpected error: %v", err)
		}
		if tenantID != "tenant-abc" {
			t.Errorf("app.tenant_id = %q, want %q", tenantID, "tenant-abc")
		}
	})

	t.Run("setting does not leak outside transaction", func(t *testing.T) {
		tenant := tenancy.Tenant{ID: "tenant-xyz", Slug: "xyz"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := database.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return nil // just set the tenant and commit
		})
		if err != nil {
			t.Fatalf("TenantTx() returned unexpected error: %v", err)
		}

		// Verify the setting is no longer set on a subsequent query.
		var val string
		err = database.Pool().QueryRow(ctx, "select current_setting('app.tenant_id', true)").Scan(&val)
		if err != nil {
			t.Fatalf("query current_setting: %v", err)
		}
		if val != "" {
			t.Errorf("app.tenant_id leaked = %q, want empty string", val)
		}
	})

	t.Run("commits on success", func(t *testing.T) {
		// Create a test table, insert a row inside TenantTx, verify it persists.
		_, err := database.Pool().Exec(ctx, `
			create table test_commit (
				id text primary key,
				value text not null
			);
			alter table test_commit enable row level security;
			alter table test_commit force row level security;

			create policy test_commit_tenant on test_commit
				using (id = grove.current_tenant_id())
				with check (id = grove.current_tenant_id());
		`)
		if err != nil {
			t.Fatalf("create test_commit table: %v", err)
		}

		tenant := tenancy.Tenant{ID: "commit-tenant", Slug: "commit"}
		tctx := tenancy.WithTenant(context.Background(), tenant)

		err = database.TenantTx(tctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into test_commit (id, value) values ($1, $2)", "commit-tenant", "hello")
			return err
		})
		if err != nil {
			t.Fatalf("TenantTx() commit: %v", err)
		}

		// Verify the row exists by querying with the same tenant context.
		err = database.TenantTx(tctx, func(ctx context.Context, tx pgx.Tx) error {
			var value string
			if err := tx.QueryRow(ctx, "select value from test_commit where id = $1", "commit-tenant").Scan(&value); err != nil {
				return fmt.Errorf("query committed row: %w", err)
			}
			if value != "hello" {
				return fmt.Errorf("value = %q, want %q", value, "hello")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() verify commit: %v", err)
		}
	})

	t.Run("rolls back on error", func(t *testing.T) {
		_, err := database.Pool().Exec(ctx, `
			create table test_rollback (
				id text primary key,
				value text not null
			);
			alter table test_rollback enable row level security;
			alter table test_rollback force row level security;

			create policy test_rollback_tenant on test_rollback
				using (id = grove.current_tenant_id())
				with check (id = grove.current_tenant_id());
		`)
		if err != nil {
			t.Fatalf("create test_rollback table: %v", err)
		}

		tenant := tenancy.Tenant{ID: "rollback-tenant", Slug: "rollback"}
		tctx := tenancy.WithTenant(context.Background(), tenant)

		err = database.TenantTx(tctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into test_rollback (id, value) values ($1, $2)", "rollback-tenant", "should not persist")
			if err != nil {
				return err
			}
			return fmt.Errorf("deliberate error")
		})
		if err == nil {
			t.Fatal("TenantTx() should return the callback error")
		}
		if !strings.Contains(err.Error(), "deliberate error") {
			t.Errorf("error = %q, want to contain 'deliberate error'", err.Error())
		}

		// Verify the row does not exist.
		err = database.TenantTx(tctx, func(ctx context.Context, tx pgx.Tx) error {
			var value string
			err := tx.QueryRow(ctx, "select value from test_rollback where id = $1", "rollback-tenant").Scan(&value)
			if err == nil {
				return fmt.Errorf("expected no row, got value = %q", value)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() verify rollback: %v", err)
		}
	})

	t.Run("rolls back on panic", func(t *testing.T) {
		_, err := database.Pool().Exec(ctx, `
			create table test_panic (
				id text primary key,
				value text not null
			);
			alter table test_panic enable row level security;
			alter table test_panic force row level security;

			create policy test_panic_tenant on test_panic
				using (id = grove.current_tenant_id())
				with check (id = grove.current_tenant_id());
		`)
		if err != nil {
			t.Fatalf("create test_panic table: %v", err)
		}

		tenant := tenancy.Tenant{ID: "panic-tenant", Slug: "panic"}
		tctx := tenancy.WithTenant(context.Background(), tenant)

		recovered := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					recovered = true
				}
			}()
			_ = database.TenantTx(tctx, func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "insert into test_panic (id, value) values ($1, $2)", "panic-tenant", "should not persist")
				if err != nil {
					return err
				}
				panic("deliberate panic")
			})
		}()

		if !recovered {
			t.Fatal("expected panic to propagate")
		}

		// Verify the row does not exist.
		err = database.TenantTx(tctx, func(ctx context.Context, tx pgx.Tx) error {
			var value string
			err := tx.QueryRow(ctx, "select value from test_panic where id = $1", "panic-tenant").Scan(&value)
			if err == nil {
				return fmt.Errorf("expected no row after panic, got value = %q", value)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() verify panic rollback: %v", err)
		}
	})

	t.Run("RLS isolates tenant data", func(t *testing.T) {
		ownerDB, appDB := setupRLSTestDB(t)
		_ = ownerDB

		// Create table as owner (superuser). Use public schema explicitly
		// to avoid the table being created in the grove schema (which is in
		// the owner's search_path).
		_, err := ownerDB.Pool().Exec(context.Background(), `
			create table public.test_rls (
				id text primary key,
				tenant_id text not null,
				name text not null
			);
			alter table public.test_rls enable row level security;
			alter table public.test_rls force row level security;

			create policy test_rls_isolation on public.test_rls
				using (tenant_id = grove.current_tenant_id())
				with check (tenant_id = grove.current_tenant_id());

			grant select, insert, update, delete on public.test_rls to grove_app;
		`)
		if err != nil {
			t.Fatalf("create test_rls table: %v", err)
		}

		tenantA := tenancy.Tenant{ID: "tenant-a", Slug: "a"}
		tenantB := tenancy.Tenant{ID: "tenant-b", Slug: "b"}
		ctxA := tenancy.WithTenant(context.Background(), tenantA)
		ctxB := tenancy.WithTenant(context.Background(), tenantB)

		// Insert a row for tenant A using the app user (subject to RLS).
		err = appDB.TenantTx(ctxA, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into test_rls (id, tenant_id, name) values ($1, $2, $3)", "widget-1", "tenant-a", "Tenant A Widget")
			return err
		})
		if err != nil {
			t.Fatalf("TenantTx() insert tenant A: %v", err)
		}

		// Tenant B should not see tenant A's data (RLS enforced).
		err = appDB.TenantTx(ctxB, func(ctx context.Context, tx pgx.Tx) error {
			var name string
			err := tx.QueryRow(ctx, "select name from test_rls where id = $1", "widget-1").Scan(&name)
			if err == nil {
				return fmt.Errorf("tenant B should not see tenant A's widget, got name = %q", name)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() tenant B should not see tenant A data: %v", err)
		}

		// Tenant A should see their own data.
		err = appDB.TenantTx(ctxA, func(ctx context.Context, tx pgx.Tx) error {
			var name string
			if err := tx.QueryRow(ctx, "select name from test_rls where id = $1", "widget-1").Scan(&name); err != nil {
				return fmt.Errorf("tenant A should see their widget: %w", err)
			}
			if name != "Tenant A Widget" {
				return fmt.Errorf("name = %q, want %q", name, "Tenant A Widget")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() tenant A should see their own data: %v", err)
		}
	})
}

func TestSystemTxIntegration(t *testing.T) {
	database := setupTestDB(t)
	ctx := context.Background()

	t.Run("executes callback without tenant context", func(t *testing.T) {
		// SystemTx should work without any tenant in context.
		err := database.SystemTx(ctx, "administrative task", func(ctx context.Context, tx pgx.Tx) error {
			var result int
			if err := tx.QueryRow(ctx, "select 1").Scan(&result); err != nil {
				return err
			}
			if result != 1 {
				return fmt.Errorf("result = %d, want 1", result)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("SystemTx() returned unexpected error: %v", err)
		}
	})

	t.Run("never sets app.tenant_id", func(t *testing.T) {
		err := database.SystemTx(ctx, "check no tenant", func(ctx context.Context, tx pgx.Tx) error {
			var val *string
			if err := tx.QueryRow(ctx, "select nullif(current_setting('app.tenant_id', true), '')").Scan(&val); err != nil {
				return err
			}
			if val != nil && *val != "" {
				return fmt.Errorf("app.tenant_id = %q, want empty/null", *val)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("SystemTx() returned unexpected error: %v", err)
		}
	})

	t.Run("can insert data visible to tenant transactions", func(t *testing.T) {
		_, err := database.Pool().Exec(ctx, `
			create table test_system_insert (
				id text primary key,
				tenant_id text not null,
				name text not null
			);
			alter table test_system_insert enable row level security;
			alter table test_system_insert force row level security;

			create policy test_system_insert_isolation on test_system_insert
				using (tenant_id = grove.current_tenant_id())
				with check (tenant_id = grove.current_tenant_id());
		`)
		if err != nil {
			t.Fatalf("create test_system_insert table: %v", err)
		}

		// SystemTx can insert rows because the superuser bypasses RLS.
		err = database.SystemTx(ctx, "seed test data", func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into test_system_insert (id, tenant_id, name) values ($1, $2, $3)", "w1", "tenant-a", "Widget A")
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "insert into test_system_insert (id, tenant_id, name) values ($1, $2, $3)", "w2", "tenant-b", "Widget B")
			return err
		})
		if err != nil {
			t.Fatalf("SystemTx() insert test data: %v", err)
		}

		// Verify the rows are visible to their respective tenant transactions.
		tenantA := tenancy.Tenant{ID: "tenant-a", Slug: "a"}
		ctxA := tenancy.WithTenant(context.Background(), tenantA)

		err = database.TenantTx(ctxA, func(ctx context.Context, tx pgx.Tx) error {
			var name string
			if err := tx.QueryRow(ctx, "select name from test_system_insert where id = $1", "w1").Scan(&name); err != nil {
				return fmt.Errorf("tenant A should see their widget: %w", err)
			}
			if name != "Widget A" {
				return fmt.Errorf("name = %q, want %q", name, "Widget A")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() verify SystemTx insert: %v", err)
		}
	})

	t.Run("requires non-empty reason", func(t *testing.T) {
		err := database.SystemTx(ctx, "", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should require non-empty reason")
		}
		if !strings.Contains(err.Error(), "non-empty reason") {
			t.Errorf("error = %q, want non-empty reason error", err.Error())
		}
	})

	t.Run("rolls back on error", func(t *testing.T) {
		_, err := database.Pool().Exec(ctx, `
			create table test_system_rollback (
				id text primary key,
				value text not null
			);
		`)
		if err != nil {
			t.Fatalf("create test_system_rollback table: %v", err)
		}

		err = database.SystemTx(ctx, "test rollback", func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into test_system_rollback (id, value) values ($1, $2)", "sys-1", "should not persist")
			if err != nil {
				return err
			}
			return fmt.Errorf("deliberate error")
		})
		if err == nil {
			t.Fatal("SystemTx() should return callback error")
		}

		// Verify the row was rolled back.
		err = database.SystemTx(ctx, "verify rollback", func(ctx context.Context, tx pgx.Tx) error {
			var value string
			err := tx.QueryRow(ctx, "select value from test_system_rollback where id = $1", "sys-1").Scan(&value)
			if err == nil {
				return fmt.Errorf("expected no row after rollback, got value = %q", value)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("SystemTx() verify rollback: %v", err)
		}
	})

	t.Run("rolls back on panic", func(t *testing.T) {
		_, err := database.Pool().Exec(ctx, `
			create table test_system_panic (
				id text primary key,
				value text not null
			);
		`)
		if err != nil {
			t.Fatalf("create test_system_panic table: %v", err)
		}

		recovered := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					recovered = true
				}
			}()
			_ = database.SystemTx(ctx, "test panic", func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "insert into test_system_panic (id, value) values ($1, $2)", "sys-panic", "should not persist")
				if err != nil {
					return err
				}
				panic("deliberate panic")
			})
		}()

		if !recovered {
			t.Fatal("expected panic to propagate")
		}

		// Verify the row was rolled back.
		err = database.SystemTx(ctx, "verify panic rollback", func(ctx context.Context, tx pgx.Tx) error {
			var value string
			err := tx.QueryRow(ctx, "select value from test_system_panic where id = $1", "sys-panic").Scan(&value)
			if err == nil {
				return fmt.Errorf("expected no row after panic rollback, got value = %q", value)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("SystemTx() verify panic rollback: %v", err)
		}
	})
}

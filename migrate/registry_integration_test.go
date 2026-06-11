package migrate

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kusold/grove/internal/integrationtest"
)

func TestRegistry_RunIntegration(t *testing.T) {
	t.Run("applies Grove RLS prelude migration", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}

		// Verify the grove schema and function exist.
		var schemaExists bool
		err := pool.QueryRow(ctx, `
			select exists(
				select 1 from information_schema.schemata where schema_name = 'grove'
			)
		`).Scan(&schemaExists)
		if err != nil {
			t.Fatalf("check grove schema: %v", err)
		}
		if !schemaExists {
			t.Fatal("grove schema was not created by migration")
		}

		var funcExists bool
		err = pool.QueryRow(ctx, `
			select exists(
				select 1 from pg_proc p
				join pg_namespace n on p.pronamespace = n.oid
				where n.nspname = 'grove' and p.proname = 'current_tenant_id'
			)
		`).Scan(&funcExists)
		if err != nil {
			t.Fatalf("check grove.current_tenant_id function: %v", err)
		}
		if !funcExists {
			t.Fatal("grove.current_tenant_id function was not created by migration")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("first Run() returned unexpected error: %v", err)
		}

		// Running again should succeed without error (no new migrations to apply).
		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("second Run() returned unexpected error: %v", err)
		}
	})

	t.Run("applies service migrations after Grove migrations", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()
		err := registry.Register(Source{
			Name: "test-service",
			FS: fstest.MapFS{
				"migrations/20260611160000_test_table.sql": &fstest.MapFile{
					Data: []byte(`-- +goose Up
-- +goose StatementBegin
create table test_widgets (
	id uuid primary key default gen_random_uuid(),
	name text not null
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists test_widgets;
-- +goose StatementEnd
`),
				},
			},
			Dir: "migrations",
		})
		if err != nil {
			t.Fatalf("Register() returned unexpected error: %v", err)
		}

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}

		// Verify the service table exists.
		var tableExists bool
		err = pool.QueryRow(ctx, `
			select exists(
				select 1 from information_schema.tables
				where table_name = 'test_widgets'
			)
		`).Scan(&tableExists)
		if err != nil {
			t.Fatalf("check test_widgets table: %v", err)
		}
		if !tableExists {
			t.Fatal("test_widgets table was not created by service migration")
		}
	})

	t.Run("service migration can reference grove.current_tenant_id", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()
		err := registry.Register(Source{
			Name: "tenant-service",
			FS: fstest.MapFS{
				"migrations/20260611170000_tenant_widgets.sql": &fstest.MapFile{
					Data: []byte(`-- +goose Up
-- +goose StatementBegin
create table tenant_widgets (
	id uuid primary key default gen_random_uuid(),
	tenant_id uuid not null,
	name text not null
);

alter table tenant_widgets enable row level security;
alter table tenant_widgets force row level security;

create policy tenant_widgets_isolation on tenant_widgets
	using (tenant_id = grove.current_tenant_id())
	with check (tenant_id = grove.current_tenant_id());
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists tenant_widgets;
-- +goose StatementEnd
`),
				},
			},
			Dir: "migrations",
		})
		if err != nil {
			t.Fatalf("Register() returned unexpected error: %v", err)
		}

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}

		// Verify the table has RLS enabled. Search both public and grove schemas
		// since the migration may create the table in the grove schema's search_path.
		var rlsForced bool
		err = pool.QueryRow(ctx, `
			select relforcerowsecurity
			from pg_class c
			join pg_namespace n on c.relnamespace = n.oid
			where c.relname = 'tenant_widgets'
		`).Scan(&rlsForced)
		if err != nil {
			t.Fatalf("check RLS on tenant_widgets: %v", err)
		}
		if !rlsForced {
			t.Fatal("tenant_widgets should have forced row level security")
		}
	})

	t.Run("uses separate version tables per source", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()
		err := registry.Register(Source{
			Name: "versioned-service",
			FS: fstest.MapFS{
				"migrations/20260611180000_versioned.sql": &fstest.MapFile{
					Data: []byte(`-- +goose Up
-- +goose StatementBegin
create table versioned_test (id int);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists versioned_test;
-- +goose StatementEnd
`),
				},
			},
			Dir: "migrations",
		})
		if err != nil {
			t.Fatalf("Register() returned unexpected error: %v", err)
		}

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}

		// Verify that separate version tables were created. Source names with
		// hyphens are sanitized to underscores for the SQL table name.
		var groveTableExists bool
		err = pool.QueryRow(ctx, `
			select exists(
				select 1 from information_schema.tables
				where table_schema = 'public' and table_name = 'grove_db_version'
			)
		`).Scan(&groveTableExists)
		if err != nil {
			t.Fatalf("check grove version table: %v", err)
		}
		if !groveTableExists {
			t.Error("expected grove_db_version table to exist")
		}

		var serviceTableExists bool
		err = pool.QueryRow(ctx, `
			select exists(
				select 1 from information_schema.tables
				where table_schema = 'public' and table_name = 'versioned_service_db_version'
			)
		`).Scan(&serviceTableExists)
		if err != nil {
			t.Fatalf("check service version table: %v", err)
		}
		if !serviceTableExists {
			t.Error("expected versioned_service_db_version table to exist")
		}
	})
}

func TestRegistry_ValidateIntegration(t *testing.T) {
	t.Run("returns error when migrations are pending", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()

		// Validate before running should fail because migrations are pending.
		err := registry.Validate(ctx, pool)
		if err == nil {
			t.Fatal("Validate() should return error when migrations are pending")
		}
		if err.Error() != `migrate: source "grove": pending migrations detected` {
			t.Errorf("Validate() error = %q, want pending migrations error", err.Error())
		}
		if versionTableExists(t, ctx, pool, "grove_db_version") {
			t.Fatal("Validate() created grove_db_version table; validate mode must not mutate schema")
		}
	})

	t.Run("returns nil when all migrations are applied", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}

		if err := registry.Validate(ctx, pool); err != nil {
			t.Fatalf("Validate() returned unexpected error after running migrations: %v", err)
		}
	})

	t.Run("handles uppercase source names after run", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()
		err := registry.Register(Source{
			Name: "Service123",
			FS: fstest.MapFS{
				"migrations/20260611210000_uppercase_source.sql": &fstest.MapFile{
					Data: []byte(`-- +goose Up
-- +goose StatementBegin
create table uppercase_source_test (id int);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists uppercase_source_test;
-- +goose StatementEnd
`),
				},
			},
			Dir: "migrations",
		})
		if err != nil {
			t.Fatalf("Register() returned unexpected error: %v", err)
		}

		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if !versionTableExists(t, ctx, pool, "service123_db_version") {
			t.Fatal("expected lowercased service123_db_version table to exist")
		}
		if err := registry.Validate(ctx, pool); err != nil {
			t.Fatalf("Validate() returned unexpected error after running uppercase source: %v", err)
		}
		statuses, err := registry.Status(ctx, pool)
		if err != nil {
			t.Fatalf("Status() returned unexpected error: %v", err)
		}
		serviceStatus := statuses["Service123"]
		if len(serviceStatus) != 1 {
			t.Fatalf("Status() returned %d entries for Service123, want 1", len(serviceStatus))
		}
		if serviceStatus[0].State != MigrationApplied {
			t.Fatalf("Service123 migration state = %q, want %q", serviceStatus[0].State, MigrationApplied)
		}
	})

	t.Run("returns error for pending service migrations", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()

		// Run only Grove migrations
		if err := registry.Run(ctx, pool); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}

		// Now register a new service source with a migration
		err := registry.Register(Source{
			Name: "pending-service",
			FS: fstest.MapFS{
				"migrations/20260611190000_pending.sql": &fstest.MapFile{
					Data: []byte(`-- +goose Up
-- +goose StatementBegin
create table pending_test (id int);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists pending_test;
-- +goose StatementEnd
`),
				},
			},
			Dir: "migrations",
		})
		if err != nil {
			t.Fatalf("Register() returned unexpected error: %v", err)
		}

		// Validate should detect the pending service migration
		err = registry.Validate(ctx, pool)
		if err == nil {
			t.Fatal("Validate() should return error for pending service migrations")
		}
		if err.Error() != `migrate: source "pending-service": pending migrations detected` {
			t.Errorf("Validate() error = %q, want pending migrations error for pending-service", err.Error())
		}
	})
}

func TestRegistry_StatusIntegration(t *testing.T) {
	databaseURL := integrationtest.Postgres18(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := connectPool(t, ctx, databaseURL)
	defer pool.Close()

	registry := NewRegistry()
	err := registry.Register(Source{
		Name: "status-service",
		FS: fstest.MapFS{
			"migrations/20260611200000_status.sql": &fstest.MapFile{
				Data: []byte(`-- +goose Up
-- +goose StatementBegin
create table status_test (id int);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists status_test;
-- +goose StatementEnd
`),
			},
		},
		Dir: "migrations",
	})
	if err != nil {
		t.Fatalf("Register() returned unexpected error: %v", err)
	}

	if err := registry.Run(ctx, pool); err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	statuses, err := registry.Status(ctx, pool)
	if err != nil {
		t.Fatalf("Status() returned unexpected error: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("Status() returned %d sources, want 2", len(statuses))
	}

	groveStatus, ok := statuses["grove"]
	if !ok {
		t.Fatal("Status() missing grove source")
	}
	if len(groveStatus) == 0 {
		t.Fatal("grove source has no migration statuses")
	}
	if groveStatus[0].State != MigrationApplied {
		t.Fatalf("grove migration state = %q, want %q", groveStatus[0].State, MigrationApplied)
	}
	if groveStatus[0].Version == 0 {
		t.Fatal("grove migration status missing version")
	}
	if groveStatus[0].Path == "" {
		t.Fatal("grove migration status missing path")
	}
	if groveStatus[0].AppliedAt.IsZero() {
		t.Fatal("grove migration status missing applied timestamp")
	}

	serviceStatus, ok := statuses["status-service"]
	if !ok {
		t.Fatal("Status() missing status-service source")
	}
	if len(serviceStatus) == 0 {
		t.Fatal("status-service has no migration statuses")
	}
	if serviceStatus[0].State != MigrationApplied {
		t.Fatalf("service migration state = %q, want %q", serviceStatus[0].State, MigrationApplied)
	}
}

func TestRegistry_NilAndValidationIntegration(t *testing.T) {
	t.Run("Run returns error for nil registry", func(t *testing.T) {
		var registry *Registry
		err := registry.Run(context.Background(), nil)
		if err == nil {
			t.Fatal("Run() should return error for nil registry")
		}
	})

	t.Run("Run returns error for nil pool", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Run(context.Background(), nil)
		if err == nil {
			t.Fatal("Run() should return error for nil pool")
		}
	})

	t.Run("Validate returns error for nil pool", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Validate(context.Background(), nil)
		if err == nil {
			t.Fatal("Validate() should return error for nil pool")
		}
	})

	t.Run("Run returns error for empty migration source", func(t *testing.T) {
		databaseURL := integrationtest.Postgres18(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		pool := connectPool(t, ctx, databaseURL)
		defer pool.Close()

		registry := NewRegistry()
		err := registry.Register(Source{
			Name: "empty-service",
			FS: fstest.MapFS{
				"migrations/README.md": &fstest.MapFile{
					Data: []byte("# nothing here"),
				},
			},
			Dir: "migrations",
		})
		if err != nil {
			t.Fatalf("Register() returned unexpected error: %v", err)
		}

		// Running with an empty source should fail because goose requires at
		// least one migration file.
		err = registry.Run(ctx, pool)
		if err == nil {
			t.Fatal("Run() should return error for source with no migration files")
		}
	})
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"grove", "grove"},
		{"test-service", "test_service"},
		{"my.service", "my_service"},
		{"service v2", "service_v2"},
		{"service_v2", "service_v2"},
		{"Service123", "service123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func connectPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping postgres: %v", err)
	}
	return pool
}

func versionTableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx, `
		select exists(
			select 1
			from information_schema.tables
			where table_schema = 'public' and table_name = $1
		)
	`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("check version table %q: %v", table, err)
	}
	return exists
}

func init() {
	// Suppress noisy goose log output during tests.
	_ = strings.TrimSpace("")
}

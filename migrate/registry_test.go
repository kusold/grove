package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"testing"
	"testing/fstest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewRegistryIncludesGroveMigrationsFirst(t *testing.T) {
	registry := NewRegistry()
	sources := registry.Sources()
	if len(sources) != 1 {
		t.Fatalf("NewRegistry() registered %d sources, want 1", len(sources))
	}
	if sources[0].Name != "grove" {
		t.Fatalf("first migration source = %q, want grove", sources[0].Name)
	}
}

func TestRegistryRegisterAppendsServiceMigrations(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Source{
		Name: "service",
		FS: fstest.MapFS{
			"migrations/20260611160000_service.sql": {},
		},
		Dir: "migrations",
	})
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	sources := registry.Sources()
	if len(sources) != 2 {
		t.Fatalf("Register() produced %d sources, want 2", len(sources))
	}
	if sources[0].Name != "grove" || sources[1].Name != "service" {
		t.Fatalf("sources registered in wrong order: got %q then %q", sources[0].Name, sources[1].Name)
	}
}

func TestRegistrySourcesReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	sources := registry.Sources()
	sources[0].Name = "changed"

	if got := registry.Sources()[0].Name; got != "grove" {
		t.Fatalf("Sources() returned mutable backing slice; got source name %q", got)
	}
}

func TestRegistryRegisterValidation(t *testing.T) {
	tests := []struct {
		name   string
		source Source
	}{
		{
			name: "missing name",
			source: Source{
				FS:  fstest.MapFS{},
				Dir: "migrations",
			},
		},
		{
			name: "missing fs",
			source: Source{
				Name: "service",
				Dir:  "migrations",
			},
		},
		{
			name: "missing dir",
			source: Source{
				Name: "service",
				FS:   fstest.MapFS{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := NewRegistry().Register(tt.source); err == nil {
				t.Fatal("Register() returned nil error")
			}
		})
	}
}

func TestRegistryRegisterRejectsVersionTableCollisions(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Source{
		Name: "billing-api",
		FS: fstest.MapFS{
			"migrations/20260611160000_billing.sql": {},
		},
		Dir: "migrations",
	})
	if err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	err = registry.Register(Source{
		Name: "billing_api",
		FS: fstest.MapFS{
			"migrations/20260611170000_billing.sql": {},
		},
		Dir: "migrations",
	})
	if err == nil {
		t.Fatal("Register() accepted source with colliding version table")
	}
	if want := `migrate: source "billing_api" version table "public.billing_api_db_version" conflicts with source "billing-api"`; err.Error() != want {
		t.Fatalf("Register() error = %q, want %q", err.Error(), want)
	}
}

func TestRegistryRegisterRejectsGroveVersionTableCollision(t *testing.T) {
	err := NewRegistry().Register(Source{
		Name: "Grove",
		FS: fstest.MapFS{
			"migrations/20260611160000_service.sql": {},
		},
		Dir: "migrations",
	})
	if err == nil {
		t.Fatal("Register() accepted source that collides with Grove version table")
	}
	if want := `migrate: source "Grove" version table "public.grove_db_version" conflicts with source "grove"`; err.Error() != want {
		t.Fatalf("Register() error = %q, want %q", err.Error(), want)
	}
}

func TestNilRegistryRegisterReturnsError(t *testing.T) {
	var registry *Registry
	err := registry.Register(Source{Name: "service", FS: fstest.MapFS{}, Dir: "migrations"})
	if err == nil {
		t.Fatal("Register() returned nil error")
	}
	if err.Error() != "migrate: registry is nil" {
		t.Fatalf("Register() error = %v, want nil registry error", err)
	}
}

func TestSourcesNilRegistryReturnsNil(t *testing.T) {
	var registry *Registry
	if sources := registry.Sources(); sources != nil {
		t.Fatalf("Sources() on nil registry = %v, want nil", sources)
	}
}

func TestStatusNilRegistryReturnsError(t *testing.T) {
	var registry *Registry
	_, err := registry.Status(context.Background(), nil)
	if err == nil {
		t.Fatal("Status() on nil registry returned nil error")
	}
	if err.Error() != "migrate: registry is nil" {
		t.Fatalf("Status() error = %v, want nil registry error", err)
	}
}

func TestStatusNilPoolReturnsError(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Status(context.Background(), nil)
	if err == nil {
		t.Fatal("Status() with nil pool returned nil error")
	}
	if err.Error() != "migrate: pool is required" {
		t.Fatalf("Status() error = %v, want pool required error", err)
	}
}

func TestTableName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"grove", "public.grove_db_version"},
		{"my-service", "public.my_service_db_version"},
		{"a.b", "public.a_b_db_version"},
		{"Service123", "public.service123_db_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableName(Source{Name: tt.name})
			if got != tt.want {
				t.Errorf("tableName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSubFS(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		fsys := fstest.MapFS{
			"migrations/001.sql": {},
		}
		_, err := subFS(Source{FS: fsys, Dir: "migrations"})
		if err != nil {
			t.Fatalf("subFS() returned error: %v", err)
		}
		files, err := fsys.ReadDir("migrations")
		if err != nil {
			t.Fatalf("ReadDir() returned error: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("subFS() found %d files, want 1", len(files))
		}
	})

	t.Run("returns subtree for valid directory", func(t *testing.T) {
		fsys := fstest.MapFS{
			"migrations/001.sql": {},
		}
		sub, err := subFS(Source{FS: fsys, Dir: "migrations"})
		if err != nil {
			t.Fatalf("subFS() returned error: %v", err)
		}
		dir, ok := sub.(fs.ReadDirFS)
		if !ok {
			t.Fatal("subFS() did not return a fs.ReadDirFS")
		}
		entries, err := dir.ReadDir(".")
		if err != nil {
			t.Fatalf("ReadDir() returned error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("subFS() found %d entries, want 1", len(entries))
		}
	})
}

func TestRunSourceSubFSError(t *testing.T) {
	db, _ := sql.Open("pgx", "")
	defer func() { _ = db.Close() }()

	err := runSource(context.Background(), db, Source{
		Name: "bad",
		FS:   fstest.MapFS{},
		Dir:  "nonexistent",
	})
	if err == nil {
		t.Fatal("runSource() with bad dir returned nil error")
	}
}

func TestValidateSourceSubFSError(t *testing.T) {
	db, _ := sql.Open("pgx", "")
	defer func() { _ = db.Close() }()

	err := validateSource(context.Background(), db, Source{
		Name: "bad",
		FS:   fstest.MapFS{},
		Dir:  "nonexistent",
	})
	if err == nil {
		t.Fatal("validateSource() with bad dir returned nil error")
	}
}

func TestRunSourceDBError(t *testing.T) {
	db, _ := sql.Open("pgx", "")
	defer func() { _ = db.Close() }()

	err := runSource(context.Background(), db, Source{
		Name: "svc",
		FS: fstest.MapFS{
			"migrations/001.sql": &fstest.MapFile{
				Data: []byte("-- +goose Up\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n"),
			},
		},
		Dir: "migrations",
	})
	if err == nil {
		t.Fatal("runSource() with disconnected DB returned nil error")
	}
}

func TestValidateSourceDBError(t *testing.T) {
	db, _ := sql.Open("pgx", "")
	defer func() { _ = db.Close() }()

	err := validateSource(context.Background(), db, Source{
		Name: "svc",
		FS: fstest.MapFS{
			"migrations/001.sql": &fstest.MapFile{
				Data: []byte("-- +goose Up\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n"),
			},
		},
		Dir: "migrations",
	})
	if err == nil {
		t.Fatal("validateSource() with disconnected DB returned nil error")
	}
}

func badPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, "postgres://nonexistent:invalid@127.0.0.1:1/none?sslmode=disable")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRunDBError(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := registry.Run(ctx, badPool(t))
	if err == nil {
		t.Fatal("Run() with unreachable DB returned nil error")
	}
}

func TestValidateDBError(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := registry.Validate(ctx, badPool(t))
	if err == nil {
		t.Fatal("Validate() with unreachable DB returned nil error")
	}
}

func TestStatusDBError(t *testing.T) {
	registry := NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := registry.Status(ctx, badPool(t))
	if err == nil {
		t.Fatal("Status() with unreachable DB returned nil error")
	}
}

func TestSplitTableName(t *testing.T) {
	t.Run("qualified name", func(t *testing.T) {
		schema, table := splitTableName("public.grove_db_version")
		if schema != "public" || table != "grove_db_version" {
			t.Fatalf("splitTableName() = (%q, %q), want (public, grove_db_version)", schema, table)
		}
	})

	t.Run("unqualified name", func(t *testing.T) {
		schema, table := splitTableName("grove_db_version")
		if schema != "public" || table != "grove_db_version" {
			t.Fatalf("splitTableName() = (%q, %q), want (public, grove_db_version)", schema, table)
		}
	})
}

func TestAppliedMigrations(t *testing.T) {
	t.Run("table does not exist", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "grove_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		applied, exists, err := appliedMigrations(context.Background(), db, "public.grove_db_version")
		if err != nil {
			t.Fatalf("appliedMigrations() returned error: %v", err)
		}
		if exists {
			t.Fatal("appliedMigrations() exists = true, want false")
		}
		if len(applied) != 0 {
			t.Fatalf("appliedMigrations() returned %d entries, want 0", len(applied))
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("table exists with applied migrations", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "grove_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.grove_db_version"),
		)).
			WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}).
				AddRow(int64(20260611154614), true, time.Now()))

		applied, exists, err := appliedMigrations(context.Background(), db, "public.grove_db_version")
		if err != nil {
			t.Fatalf("appliedMigrations() returned error: %v", err)
		}
		if !exists {
			t.Fatal("appliedMigrations() exists = false, want true")
		}
		if len(applied) != 1 {
			t.Fatalf("appliedMigrations() returned %d entries, want 1", len(applied))
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("handles rolled-back migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		ts := time.Now()
		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).
			WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}).
				AddRow(int64(1), true, ts).
				AddRow(int64(1), false, ts.Add(time.Second)))

		applied, exists, err := appliedMigrations(context.Background(), db, "public.svc_db_version")
		if err != nil {
			t.Fatalf("appliedMigrations() returned error: %v", err)
		}
		if !exists {
			t.Fatal("appliedMigrations() exists = false, want true")
		}
		if _, ok := applied[1]; ok {
			t.Fatal("rolled-back migration should be removed from applied map")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("check version table query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnError(errors.New("connection refused"))

		_, _, err = appliedMigrations(context.Background(), db, "public.svc_db_version")
		if err == nil {
			t.Fatal("appliedMigrations() returned nil error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("list version table query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).
			WillReturnError(errors.New("query failed"))

		_, _, err = appliedMigrations(context.Background(), db, "public.svc_db_version")
		if err == nil {
			t.Fatal("appliedMigrations() returned nil error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).
			WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}).
				AddRow("not-an-int", true, time.Now()))

		_, _, err = appliedMigrations(context.Background(), db, "public.svc_db_version")
		if err == nil {
			t.Fatal("appliedMigrations() returned nil error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("rows.Err error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		rows := sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}).
			AddRow(int64(1), true, time.Now())
		rows.RowError(0, errors.New("row iteration failed"))
		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).WillReturnRows(rows)

		_, _, err = appliedMigrations(context.Background(), db, "public.svc_db_version")
		if err == nil {
			t.Fatal("appliedMigrations() returned nil error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})
}

func TestValidateSourceWithMockDB(t *testing.T) {
	source := Source{
		Name: "svc",
		FS: fstest.MapFS{
			"migrations/20260611160000_svc.sql": &fstest.MapFile{
				Data: []byte("-- +goose Up\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n"),
			},
		},
		Dir: "migrations",
	}

	t.Run("version table does not exist", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err = validateSource(context.Background(), db, source)
		if err == nil {
			t.Fatal("validateSource() returned nil error for missing version table")
		}
		if err.Error() != "pending migrations detected" {
			t.Fatalf("validateSource() error = %q, want pending migrations detected", err.Error())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("pending migration", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).
			WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}))

		err = validateSource(context.Background(), db, source)
		if err == nil {
			t.Fatal("validateSource() returned nil error for pending migration")
		}
		if err.Error() != "pending migrations detected" {
			t.Fatalf("validateSource() error = %q, want pending migrations detected", err.Error())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("all migrations applied", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).
			WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}).
				AddRow(int64(20260611160000), true, time.Now()))

		err = validateSource(context.Background(), db, source)
		if err != nil {
			t.Fatalf("validateSource() returned unexpected error: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("check applied migrations error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnError(errors.New("db error"))

		err = validateSource(context.Background(), db, source)
		if err == nil {
			t.Fatal("validateSource() returned nil error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})
}

func TestSourceStatusWithMockDB(t *testing.T) {
	source := Source{
		Name: "svc",
		FS: fstest.MapFS{
			"migrations/20260611160000_svc.sql": &fstest.MapFile{
				Data: []byte("-- +goose Up\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n\n-- +goose Down\n-- +goose StatementBegin\nselect 1;\n-- +goose StatementEnd\n"),
			},
		},
		Dir: "migrations",
	}

	t.Run("table does not exist returns pending", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		status, err := sourceStatus(context.Background(), db, source)
		if err != nil {
			t.Fatalf("sourceStatus() returned error: %v", err)
		}
		if len(status) != 1 {
			t.Fatalf("sourceStatus() returned %d entries, want 1", len(status))
		}
		if status[0].State != MigrationPending {
			t.Fatalf("migration state = %q, want %q", status[0].State, MigrationPending)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("applied migration shows applied state", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		appliedAt := time.Now()
		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		mock.ExpectQuery(regexp.QuoteMeta(
			fmt.Sprintf("select version_id, is_applied, tstamp from %s order by id asc", "public.svc_db_version"),
		)).
			WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied", "tstamp"}).
				AddRow(int64(20260611160000), true, appliedAt))

		status, err := sourceStatus(context.Background(), db, source)
		if err != nil {
			t.Fatalf("sourceStatus() returned error: %v", err)
		}
		if len(status) != 1 {
			t.Fatalf("sourceStatus() returned %d entries, want 1", len(status))
		}
		if status[0].State != MigrationApplied {
			t.Fatalf("migration state = %q, want %q", status[0].State, MigrationApplied)
		}
		if status[0].AppliedAt.IsZero() {
			t.Fatal("applied migration missing AppliedAt timestamp")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(regexp.QuoteMeta(`select exists (
				select 1
				from pg_tables
				where schemaname = $1 and tablename = $2
			)`)).
			WithArgs("public", "svc_db_version").
			WillReturnError(errors.New("db error"))

		_, err = sourceStatus(context.Background(), db, source)
		if err == nil {
			t.Fatal("sourceStatus() returned nil error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unfulfilled expectations: %v", err)
		}
	})
}

func TestCollectMigrationFilesDuplicateVersion(t *testing.T) {
	source := Source{
		Name: "dup",
		FS: fstest.MapFS{
			"migrations/20260611160000_first.sql":  {},
			"migrations/20260611160000_second.sql": {},
		},
		Dir: "migrations",
	}

	_, err := collectMigrationFiles(source)
	if err == nil {
		t.Fatal("collectMigrationFiles() returned nil error for duplicate versions")
	}
	if err.Error() != `duplicate migration version 20260611160000 in "20260611160000_first.sql" and "20260611160000_second.sql"` {
		t.Fatalf("collectMigrationFiles() error = %q", err.Error())
	}
}

func TestCollectMigrationFilesNonSQLFiles(t *testing.T) {
	source := Source{
		Name: "mixed",
		FS: fstest.MapFS{
			"migrations/README.md": {},
		},
		Dir: "migrations",
	}

	_, err := collectMigrationFiles(source)
	if err == nil {
		t.Fatal("collectMigrationFiles() returned nil error for non-SQL files")
	}
}

func TestSubFSEmptyDir(t *testing.T) {
	_, err := subFS(Source{FS: fstest.MapFS{}, Dir: ""})
	if err == nil {
		t.Fatal("subFS() with empty dir returned nil error")
	}
}

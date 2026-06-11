package migrate

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

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

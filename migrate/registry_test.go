package migrate

import (
	"testing"
	"testing/fstest"
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

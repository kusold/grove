package migrate

import (
	"errors"
	"io/fs"
)

// Source identifies an embedded migration collection.
type Source struct {
	// Name is a stable label for logging and diagnostics.
	Name string

	// FS contains the migration files.
	FS fs.FS

	// Dir is the directory inside FS that contains migration files.
	Dir string
}

// Registry collects migration sources in deterministic registration order.
type Registry struct {
	sources []Source
}

// NewRegistry returns a registry initialized with Grove-owned migrations.
func NewRegistry() *Registry {
	return &Registry{
		sources: []Source{GroveMigrations()},
	}
}

// Register appends a service-owned migration source after Grove-owned sources.
func (r *Registry) Register(source Source) error {
	if r == nil {
		return errors.New("migrate: registry is nil")
	}
	if source.Name == "" {
		return errors.New("migrate: source name is required")
	}
	if source.FS == nil {
		return errors.New("migrate: source filesystem is required")
	}
	if source.Dir == "" {
		return errors.New("migrate: source directory is required")
	}
	r.sources = append(r.sources, source)
	return nil
}

// Sources returns the registered migration sources in execution order.
func (r *Registry) Sources() []Source {
	if r == nil {
		return nil
	}
	sources := make([]Source, len(r.sources))
	copy(sources, r.sources)
	return sources
}

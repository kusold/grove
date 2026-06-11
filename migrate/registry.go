package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	stdlib "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
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

// Run applies all pending migrations from all registered sources against the
// database. Sources are applied in registration order (Grove-owned first, then
// service-owned), and each source uses its own goose version table to avoid
// cross-source version collisions.
//
// The pgxpool.Pool is used to open a database/sql connection via pgx/stdlib
// for goose compatibility. The caller is responsible for closing the pool.
func (r *Registry) Run(ctx context.Context, pool *pgxpool.Pool) error {
	if r == nil {
		return errors.New("migrate: registry is nil")
	}
	if pool == nil {
		return errors.New("migrate: pool is required")
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	for _, source := range r.sources {
		if err := runSource(ctx, db, source); err != nil {
			return fmt.Errorf("migrate: source %q: %w", source.Name, err)
		}
	}
	return nil
}

// Validate checks whether all registered migration sources are up to date
// without applying any changes. It returns an error if any source has pending
// (unapplied) migrations.
func (r *Registry) Validate(ctx context.Context, pool *pgxpool.Pool) error {
	if r == nil {
		return errors.New("migrate: registry is nil")
	}
	if pool == nil {
		return errors.New("migrate: pool is required")
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	for _, source := range r.sources {
		if err := validateSource(ctx, db, source); err != nil {
			return fmt.Errorf("migrate: source %q: %w", source.Name, err)
		}
	}
	return nil
}

// tableName returns the goose version table name for a source. Each source
// gets its own table to avoid version number collisions between Grove-owned
// and service-owned migrations. Non-alphanumeric characters in the source
// name are replaced with underscores to produce a valid SQL identifier.
//
// The table name is schema-qualified with "public" to ensure the table is
// created in a predictable location regardless of the database user name or
// search_path. Without qualification, Postgres uses current_schema() which
// can change if the user name matches a schema name (e.g., a user named
// "grove" when a "grove" schema exists from the RLS prelude).
func tableName(source Source) string {
	return "public." + sanitizeName(source.Name) + "_db_version"
}

// sanitizeName replaces non-alphanumeric characters with underscores to
// produce a valid SQL identifier.
func sanitizeName(name string) string {
	var b []byte
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b = append(b, byte(r))
		} else {
			b = append(b, '_')
		}
	}
	return string(b)
}

// subFS returns the subtree of the source filesystem rooted at the migration
// directory. Goose expects the FS to be rooted at the directory containing the
// migration files.
func subFS(source Source) (fs.FS, error) {
	sub, err := fs.Sub(source.FS, source.Dir)
	if err != nil {
		return nil, fmt.Errorf("sub filesystem: %w", err)
	}
	return sub, nil
}

// runSource applies all pending migrations for a single source.
func runSource(ctx context.Context, db *sql.DB, source Source) error {
	migrationFS, err := subFS(source)
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrationFS,
		goose.WithTableName(tableName(source)),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	_, err = provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// validateSource checks whether a single source has pending migrations.
func validateSource(ctx context.Context, db *sql.DB, source Source) error {
	migrationFS, err := subFS(source)
	if err != nil {
		return err
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrationFS,
		goose.WithTableName(tableName(source)),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	pending, err := provider.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("check pending migrations: %w", err)
	}
	if pending {
		return errors.New("pending migrations detected")
	}
	return nil
}

// Status returns the migration status for all registered sources. Each source
// is checked independently against its own goose version table.
func (r *Registry) Status(ctx context.Context, pool *pgxpool.Pool) (map[string][]*goose.MigrationStatus, error) {
	if r == nil {
		return nil, errors.New("migrate: registry is nil")
	}
	if pool == nil {
		return nil, errors.New("migrate: pool is required")
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	statuses := make(map[string][]*goose.MigrationStatus, len(r.sources))
	for _, source := range r.sources {
		migrationFS, err := subFS(source)
		if err != nil {
			return nil, fmt.Errorf("migrate: source %q: %w", source.Name, err)
		}

		provider, err := goose.NewProvider(
			goose.DialectPostgres,
			db,
			migrationFS,
			goose.WithTableName(tableName(source)),
		)
		if err != nil {
			return nil, fmt.Errorf("migrate: source %q: create provider: %w", source.Name, err)
		}

		status, err := provider.Status(ctx)
		if err != nil {
			return nil, fmt.Errorf("migrate: source %q: status: %w", source.Name, err)
		}
		statuses[source.Name] = status
	}
	return statuses, nil
}

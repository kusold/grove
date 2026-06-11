package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

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

// MigrationState describes whether a registered migration has been applied.
type MigrationState string

const (
	// MigrationPending means the migration exists in a registered source but has
	// not been applied to the database.
	MigrationPending MigrationState = "pending"

	// MigrationApplied means the migration exists in a registered source and is
	// recorded as applied in the source version table.
	MigrationApplied MigrationState = "applied"
)

// MigrationStatus describes one registered migration without exposing the
// underlying migration engine's types.
type MigrationStatus struct {
	Path      string
	Version   int64
	State     MigrationState
	AppliedAt time.Time
}

type migrationFile struct {
	path    string
	version int64
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
	migrations, err := collectMigrationFiles(source)
	if err != nil {
		return err
	}

	applied, exists, err := appliedMigrations(ctx, db, tableName(source))
	if err != nil {
		return fmt.Errorf("check applied migrations: %w", err)
	}
	if !exists {
		return errors.New("pending migrations detected")
	}
	for _, migration := range migrations {
		if _, ok := applied[migration.version]; !ok {
			return errors.New("pending migrations detected")
		}
	}
	return nil
}

// Status returns the migration status for all registered sources. Each source
// is checked independently against its own goose version table.
func (r *Registry) Status(ctx context.Context, pool *pgxpool.Pool) (map[string][]MigrationStatus, error) {
	if r == nil {
		return nil, errors.New("migrate: registry is nil")
	}
	if pool == nil {
		return nil, errors.New("migrate: pool is required")
	}

	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

	statuses := make(map[string][]MigrationStatus, len(r.sources))
	for _, source := range r.sources {
		status, err := sourceStatus(ctx, db, source)
		if err != nil {
			return nil, fmt.Errorf("migrate: source %q: %w", source.Name, err)
		}
		statuses[source.Name] = status
	}
	return statuses, nil
}

func sourceStatus(ctx context.Context, db *sql.DB, source Source) ([]MigrationStatus, error) {
	migrations, err := collectMigrationFiles(source)
	if err != nil {
		return nil, err
	}
	applied, exists, err := appliedMigrations(ctx, db, tableName(source))
	if err != nil {
		return nil, fmt.Errorf("check applied migrations: %w", err)
	}

	status := make([]MigrationStatus, 0, len(migrations))
	for _, migration := range migrations {
		entry := MigrationStatus{
			Path:    migration.path,
			Version: migration.version,
			State:   MigrationPending,
		}
		if exists {
			if appliedAt, ok := applied[migration.version]; ok {
				entry.State = MigrationApplied
				entry.AppliedAt = appliedAt
			}
		}
		status = append(status, entry)
	}
	return status, nil
}

func collectMigrationFiles(source Source) ([]migrationFile, error) {
	migrationFS, err := subFS(source)
	if err != nil {
		return nil, err
	}
	paths, err := fs.Glob(migrationFS, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("find migration files: %w", err)
	}
	if len(paths) == 0 {
		return nil, goose.ErrNoMigrationFiles
	}

	migrations := make([]migrationFile, 0, len(paths))
	seen := make(map[int64]string, len(paths))
	for _, path := range paths {
		version, err := goose.NumericComponent(path)
		if err != nil {
			continue
		}
		if previous, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, path)
		}
		seen[version] = path
		migrations = append(migrations, migrationFile{path: path, version: version})
	}
	if len(migrations) == 0 {
		return nil, goose.ErrNoMigrationFiles
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}

func appliedMigrations(ctx context.Context, db *sql.DB, qualifiedTable string) (map[int64]time.Time, bool, error) {
	schema, table := splitTableName(qualifiedTable)
	var exists bool
	if err := db.QueryRowContext(ctx, `
		select exists (
			select 1
			from pg_tables
			where schemaname = $1 and tablename = $2
		)
	`, schema, table).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("check version table: %w", err)
	}
	if !exists {
		return map[int64]time.Time{}, false, nil
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		"select version_id, is_applied, tstamp from %s order by id asc",
		qualifiedTable,
	))
	if err != nil {
		return nil, false, fmt.Errorf("list version table: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]time.Time)
	for rows.Next() {
		var version int64
		var isApplied bool
		var appliedAt time.Time
		if err := rows.Scan(&version, &isApplied, &appliedAt); err != nil {
			return nil, false, fmt.Errorf("scan version table: %w", err)
		}
		if isApplied {
			applied[version] = appliedAt
		} else {
			delete(applied, version)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return applied, true, nil
}

func splitTableName(qualifiedTable string) (string, string) {
	schema, table, ok := strings.Cut(qualifiedTable, ".")
	if !ok {
		return "public", qualifiedTable
	}
	return schema, table
}

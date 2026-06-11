// Package db provides Grove's Postgres database primitives.
package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kusold/grove/config"
	"github.com/kusold/grove/tenancy"
)

// Config holds typed Postgres pool configuration for pgx.
type Config struct {
	// URL is the Postgres application connection URL.
	URL string

	// AdminURL is the privileged Postgres connection URL used for SystemTx.
	// It is optional at startup, but SystemTx requires it to be configured.
	AdminURL string

	// MaxConns is the maximum number of connections in the pgx pool.
	MaxConns int32

	// MinConns is the minimum number of connections in the pgx pool.
	MinConns int32

	// ConnectTimeout is the timeout for establishing a Postgres connection.
	ConnectTimeout time.Duration
}

// pool abstracts the pgxpool operations used by Database.
// *pgxpool.Pool satisfies this interface.
type pool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

// Database owns the pgx connection pool for a Grove service.
type Database struct {
	pool      pool
	adminPool pool
}

// ConfigFrom parses Grove's environment-backed database config into typed pgx
// settings.
func ConfigFrom(cfg config.DatabaseConfig) (Config, error) {
	maxConns, err := parseConns("DATABASE_MAX_CONNS", cfg.MaxConns)
	if err != nil {
		return Config{}, err
	}
	minConns, err := parseConns("DATABASE_MIN_CONNS", cfg.MinConns)
	if err != nil {
		return Config{}, err
	}
	connectTimeout, err := time.ParseDuration(cfg.ConnectTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_CONNECT_TIMEOUT %q: %w", cfg.ConnectTimeout, err)
	}

	parsed := Config{
		URL:            cfg.URL,
		AdminURL:       cfg.AdminURL,
		MaxConns:       maxConns,
		MinConns:       minConns,
		ConnectTimeout: connectTimeout,
	}
	if err := parsed.Validate(); err != nil {
		return Config{}, err
	}
	return parsed, nil
}

// Validate checks whether the config can be used to create a pgx pool.
func (c Config) Validate() error {
	if c.URL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.MaxConns < 1 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be at least 1, got %d", c.MaxConns)
	}
	if c.MinConns < 0 {
		return fmt.Errorf("DATABASE_MIN_CONNS must be at least 0, got %d", c.MinConns)
	}
	if c.MinConns > c.MaxConns {
		return fmt.Errorf("DATABASE_MIN_CONNS must be less than or equal to DATABASE_MAX_CONNS, got %d > %d", c.MinConns, c.MaxConns)
	}
	if c.ConnectTimeout <= 0 {
		return fmt.Errorf("DATABASE_CONNECT_TIMEOUT must be greater than 0, got %s", c.ConnectTimeout)
	}
	return nil
}

// PoolConfig converts Grove database config into a pgxpool config.
func (c Config) PoolConfig() (*pgxpool.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	poolConfig, err := pgxpool.ParseConfig(c.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolConfig.MaxConns = c.MaxConns
	poolConfig.MinConns = c.MinConns
	poolConfig.ConnConfig.ConnectTimeout = c.ConnectTimeout
	return poolConfig, nil
}

// AdminPoolConfig converts Grove database config into a pgxpool config for the
// privileged system transaction pool. It returns nil when no admin URL is
// configured.
func (c Config) AdminPoolConfig() (*pgxpool.Config, error) {
	if c.AdminURL == "" {
		return nil, nil
	}
	adminConfig := c
	adminConfig.URL = c.AdminURL
	return adminConfig.PoolConfig()
}

// Open creates a Database, opens its pgx connection pool, and verifies that
// Postgres is reachable.
func Open(ctx context.Context, cfg Config) (*Database, error) {
	database := &Database{}
	if err := database.Connect(ctx, cfg); err != nil {
		return nil, err
	}
	return database, nil
}

// Connect opens the pgx connection pool for this Database and verifies that
// Postgres is reachable.
func (d *Database) Connect(ctx context.Context, cfg Config) error {
	if d == nil {
		return errors.New("db: database is nil")
	}
	poolConfig, err := cfg.PoolConfig()
	if err != nil {
		return err
	}
	appPool, err := connectPool(ctx, poolConfig)
	if err != nil {
		return err
	}

	var adminPool pool
	adminPoolConfig, err := cfg.AdminPoolConfig()
	if err != nil {
		appPool.Close()
		return fmt.Errorf("admin postgres config: %w", err)
	}
	if adminPoolConfig != nil {
		adminPool, err = connectPool(ctx, adminPoolConfig)
		if err != nil {
			appPool.Close()
			return fmt.Errorf("admin postgres connect: %w", err)
		}
	}

	if d.pool != nil {
		d.pool.Close()
	}
	if d.adminPool != nil {
		d.adminPool.Close()
	}
	d.pool = appPool
	d.adminPool = adminPool
	return nil
}

// Pool returns the underlying pgx connection pool.
func (d *Database) Pool() *pgxpool.Pool {
	if d == nil {
		return nil
	}
	if p, ok := d.pool.(*pgxpool.Pool); ok {
		return p
	}
	return nil
}

// Ping verifies that the database is reachable.
func (d *Database) Ping(ctx context.Context) error {
	if d == nil || d.pool == nil {
		return errors.New("db: pool is not initialized")
	}
	return d.pool.Ping(ctx)
}

// Close releases the underlying pgx connection pool.
func (d *Database) Close() {
	if d == nil || d.pool == nil {
		if d != nil && d.adminPool != nil {
			d.adminPool.Close()
		}
		return
	}
	d.pool.Close()
	if d.adminPool != nil {
		d.adminPool.Close()
	}
}

// TenantTx executes a callback inside a Postgres transaction with the tenant
// ID set as a transaction-local session variable. This enables RLS policies
// that filter rows by the current tenant.
//
// The function:
//  1. Requires a tenant in context (fails with a clear error otherwise).
//  2. Begins a transaction.
//  3. Sets app.tenant_id via set_config(..., true) so the setting is local to
//     the transaction and never leaks to other queries on the same connection.
//  4. Executes the callback with the transaction.
//  5. Commits on success, rolls back on error or panic.
//
// The setting does not persist beyond the transaction scope.
func (d *Database) TenantTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if d == nil || d.pool == nil {
		return errors.New("db: pool is not initialized")
	}

	tenant, err := tenancy.Require(ctx)
	if err != nil {
		return fmt.Errorf("db: tenant transaction requires a tenant in context: %w", err)
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin tenant transaction: %w", err)
	}

	// Set the tenant ID as a transaction-local Postgres setting. The third
	// argument (true) ensures the setting is local to this transaction and
	// does not leak to other queries on the same connection.
	_, err = tx.Exec(ctx, "select set_config('app.tenant_id', $1, true)", tenant.ID)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("db: set tenant config: %w", err)
	}

	// Handle panics by rolling back before the panic propagates.
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("db: transaction error (%v), rollback error (%v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit tenant transaction: %w", err)
	}
	return nil
}

// SystemTx executes a callback inside a Postgres transaction on the privileged
// admin pool without setting a tenant context. This is for administrative or
// cross-tenant operations that must explicitly bypass RLS.
//
// The reason parameter is required and is logged to make tenant bypasses
// intentional and searchable. SystemTx never sets app.tenant_id.
func (d *Database) SystemTx(ctx context.Context, reason string, fn func(ctx context.Context, tx pgx.Tx) error) error {
	if d == nil || d.pool == nil {
		return errors.New("db: pool is not initialized")
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("db: system transaction requires a non-empty reason")
	}
	if d.adminPool == nil {
		return errors.New("db: system transaction requires DATABASE_ADMIN_URL")
	}

	slog.InfoContext(ctx, "db: system transaction started", "reason", reason)

	tx, err := d.adminPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin system transaction: %w", err)
	}

	// Handle panics by rolling back before the panic propagates.
	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(ctx)
			panic(r)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("db: system transaction error (%v), rollback error (%v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit system transaction: %w", err)
	}
	return nil
}

func connectPool(ctx context.Context, poolConfig *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return pool, nil
}

func parseConns(name, value string) (int32, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, value, err)
	}
	return int32(parsed), nil
}

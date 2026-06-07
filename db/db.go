// Package db provides Grove's Postgres database primitives.
package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kusold/grove/config"
)

// Config holds typed Postgres pool configuration for pgx.
type Config struct {
	// URL is the Postgres connection URL.
	URL string

	// MaxConns is the maximum number of connections in the pgx pool.
	MaxConns int32

	// MinConns is the minimum number of connections in the pgx pool.
	MinConns int32

	// ConnectTimeout is the timeout for establishing a Postgres connection.
	ConnectTimeout time.Duration
}

// Database owns the pgx connection pool for a Grove service.
type Database struct {
	pool *pgxpool.Pool
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

// Open creates a pgx connection pool and verifies that Postgres is reachable.
func Open(ctx context.Context, cfg Config) (*Database, error) {
	poolConfig, err := cfg.PoolConfig()
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	database := &Database{pool: pool}
	if err := database.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return database, nil
}

// Pool returns the underlying pgx connection pool.
func (d *Database) Pool() *pgxpool.Pool {
	if d == nil {
		return nil
	}
	return d.pool
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
		return
	}
	d.pool.Close()
}

func parseConns(name, value string) (int32, error) {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, value, err)
	}
	return int32(parsed), nil
}

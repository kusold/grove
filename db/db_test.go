package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kusold/grove/config"
)

func TestConfigFrom(t *testing.T) {
	t.Run("parses database config", func(t *testing.T) {
		cfg, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://user:pass@localhost:5432/app",
			MaxConns:       "12",
			MinConns:       "2",
			ConnectTimeout: "3s",
		})
		if err != nil {
			t.Fatalf("ConfigFrom() returned unexpected error: %v", err)
		}
		if cfg.URL != "postgres://user:pass@localhost:5432/app" {
			t.Errorf("URL = %q, want database URL", cfg.URL)
		}
		if cfg.MaxConns != 12 {
			t.Errorf("MaxConns = %d, want 12", cfg.MaxConns)
		}
		if cfg.MinConns != 2 {
			t.Errorf("MinConns = %d, want 2", cfg.MinConns)
		}
		if cfg.ConnectTimeout != 3*time.Second {
			t.Errorf("ConnectTimeout = %s, want 3s", cfg.ConnectTimeout)
		}
	})

	t.Run("requires database URL", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			MaxConns:       "10",
			MinConns:       "0",
			ConnectTimeout: "5s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should require DATABASE_URL")
		}
		if !strings.Contains(err.Error(), "DATABASE_URL is required") {
			t.Errorf("error = %q, want DATABASE_URL requirement", err.Error())
		}
	})

	t.Run("validates max connections", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "0",
			MinConns:       "0",
			ConnectTimeout: "5s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject zero max connections")
		}
		if !strings.Contains(err.Error(), "DATABASE_MAX_CONNS must be at least 1") {
			t.Errorf("error = %q, want max connection validation", err.Error())
		}
	})

	t.Run("validates min connections", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "10",
			MinConns:       "11",
			ConnectTimeout: "5s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject min connections above max")
		}
		if !strings.Contains(err.Error(), "DATABASE_MIN_CONNS must be less than or equal to DATABASE_MAX_CONNS") {
			t.Errorf("error = %q, want min connection validation", err.Error())
		}
	})

	t.Run("validates negative min connections", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "10",
			MinConns:       "-1",
			ConnectTimeout: "5s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject negative min connections")
		}
		if !strings.Contains(err.Error(), "DATABASE_MIN_CONNS must be at least 0") {
			t.Errorf("error = %q, want min connection validation", err.Error())
		}
	})

	t.Run("validates connect timeout", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "10",
			MinConns:       "0",
			ConnectTimeout: "sometimes",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject invalid connect timeout")
		}
		if !strings.Contains(err.Error(), "invalid DATABASE_CONNECT_TIMEOUT") {
			t.Errorf("error = %q, want timeout validation", err.Error())
		}
	})

	t.Run("validates positive connect timeout", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "10",
			MinConns:       "0",
			ConnectTimeout: "0s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject zero connect timeout")
		}
		if !strings.Contains(err.Error(), "DATABASE_CONNECT_TIMEOUT must be greater than 0") {
			t.Errorf("error = %q, want positive timeout validation", err.Error())
		}
	})

	t.Run("reports invalid min connection format", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "10",
			MinConns:       "many",
			ConnectTimeout: "5s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject invalid min connection format")
		}
		if !strings.Contains(err.Error(), `invalid DATABASE_MIN_CONNS "many"`) {
			t.Errorf("error = %q, want min connection parse error", err.Error())
		}
	})
}

func TestPoolConfig(t *testing.T) {
	t.Run("converts config to pgx pool config", func(t *testing.T) {
		cfg := Config{
			URL:            "postgres://user:pass@localhost:5432/app?sslmode=disable",
			MaxConns:       10,
			MinConns:       1,
			ConnectTimeout: 5 * time.Second,
		}

		poolConfig, err := cfg.PoolConfig()
		if err != nil {
			t.Fatalf("PoolConfig() returned unexpected error: %v", err)
		}
		if poolConfig.ConnConfig.User != "user" {
			t.Errorf("ConnConfig.User = %q, want user", poolConfig.ConnConfig.User)
		}
		if poolConfig.MaxConns != 10 {
			t.Errorf("MaxConns = %d, want 10", poolConfig.MaxConns)
		}
		if poolConfig.MinConns != 1 {
			t.Errorf("MinConns = %d, want 1", poolConfig.MinConns)
		}
		if poolConfig.ConnConfig.ConnectTimeout != 5*time.Second {
			t.Errorf("ConnectTimeout = %s, want 5s", poolConfig.ConnConfig.ConnectTimeout)
		}
	})

	t.Run("returns validation error", func(t *testing.T) {
		_, err := (Config{}).PoolConfig()
		if err == nil {
			t.Fatal("PoolConfig() should validate config")
		}
		if !strings.Contains(err.Error(), "DATABASE_URL is required") {
			t.Errorf("error = %q, want DATABASE_URL validation", err.Error())
		}
	})
}

func TestOpen(t *testing.T) {
	t.Run("rejects invalid database URL", func(t *testing.T) {
		_, err := Open(context.Background(), Config{
			URL:            "not a postgres url",
			MaxConns:       10,
			MinConns:       0,
			ConnectTimeout: 5 * time.Second,
		})
		if err == nil {
			t.Fatal("Open() should reject invalid DATABASE_URL")
		}
		if !strings.Contains(err.Error(), "parse DATABASE_URL") {
			t.Errorf("error = %q, want parse DATABASE_URL", err.Error())
		}
	})

	t.Run("closes pool when ping fails", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Open(ctx, Config{
			URL:            "postgres://localhost/app",
			MaxConns:       10,
			MinConns:       0,
			ConnectTimeout: 5 * time.Second,
		})
		if err == nil {
			t.Fatal("Open() should fail when ping context is canceled")
		}
		if !strings.Contains(err.Error(), "connect postgres") {
			t.Errorf("error = %q, want connect postgres error", err.Error())
		}
	})
}

func TestTenantTx(t *testing.T) {
	t.Run("returns error when pool is not initialized", func(t *testing.T) {
		database := &Database{}
		err := database.TenantTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should error when pool is nil")
		}
		if !strings.Contains(err.Error(), "pool is not initialized") {
			t.Errorf("error = %q, want pool initialization error", err.Error())
		}
	})

	t.Run("returns error when database is nil", func(t *testing.T) {
		var database *Database
		err := database.TenantTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should error when database is nil")
		}
		if !strings.Contains(err.Error(), "pool is not initialized") {
			t.Errorf("error = %q, want pool initialization error", err.Error())
		}
	})

	t.Run("returns error when no tenant in context", func(t *testing.T) {
		database := &Database{pool: &pgxpool.Pool{}}
		err := database.TenantTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should error without tenant")
		}
		if !strings.Contains(err.Error(), "tenant") {
			t.Errorf("error = %q, want tenant-related error", err.Error())
		}
	})
}

func TestSystemTx(t *testing.T) {
	t.Run("returns error when pool is not initialized", func(t *testing.T) {
		database := &Database{}
		err := database.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should error when pool is nil")
		}
		if !strings.Contains(err.Error(), "pool is not initialized") {
			t.Errorf("error = %q, want pool initialization error", err.Error())
		}
	})

	t.Run("returns error when database is nil", func(t *testing.T) {
		var database *Database
		err := database.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should error when database is nil")
		}
		if !strings.Contains(err.Error(), "pool is not initialized") {
			t.Errorf("error = %q, want pool initialization error", err.Error())
		}
	})

	t.Run("returns error when reason is empty", func(t *testing.T) {
		database := &Database{pool: &pgxpool.Pool{}}
		err := database.SystemTx(context.Background(), "", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should require non-empty reason")
		}
		if !strings.Contains(err.Error(), "non-empty reason") {
			t.Errorf("error = %q, want non-empty reason error", err.Error())
		}
	})
}

func TestDatabase(t *testing.T) {
	t.Run("nil safety", func(t *testing.T) {
		var database *Database
		if database.Pool() != nil {
			t.Fatal("nil Database Pool() should return nil")
		}
		if err := database.Ping(context.Background()); err == nil {
			t.Fatal("nil Database Ping() should return an error")
		} else if !strings.Contains(err.Error(), "pool is not initialized") {
			t.Errorf("Ping() error = %q, want pool initialization error", err.Error())
		}
		database.Close()
	})

	t.Run("empty database safety", func(t *testing.T) {
		database := &Database{}
		if database.Pool() != nil {
			t.Fatal("empty Database Pool() should return nil")
		}
		if err := database.Ping(context.Background()); err == nil {
			t.Fatal("empty Database Ping() should return an error")
		} else if !strings.Contains(err.Error(), "pool is not initialized") {
			t.Errorf("Ping() error = %q, want pool initialization error", err.Error())
		}
		database.Close()
	})

	t.Run("returns and closes pool", func(t *testing.T) {
		cfg := Config{
			URL:            "postgres://localhost/app",
			MaxConns:       10,
			MinConns:       0,
			ConnectTimeout: 5 * time.Second,
		}
		poolConfig, err := cfg.PoolConfig()
		if err != nil {
			t.Fatalf("PoolConfig() returned unexpected error: %v", err)
		}
		pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err != nil {
			t.Fatalf("NewWithConfig() returned unexpected error: %v", err)
		}
		database := &Database{pool: pool}
		if database.Pool() != pool {
			t.Fatal("Pool() should return the underlying pool")
		}
		database.Close()
	})
}

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kusold/grove/config"
	"github.com/kusold/grove/tenancy"
)

// mockPool implements the pool interface for unit testing.
type mockPool struct {
	beginFn func(ctx context.Context) (pgx.Tx, error)
	pingFn  func(ctx context.Context) error
	closed  bool
}

func (m *mockPool) Begin(ctx context.Context) (pgx.Tx, error) {
	return m.beginFn(ctx)
}

func (m *mockPool) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}

func (m *mockPool) Close() {
	m.closed = true
}

// mockTx implements pgx.Tx for unit testing. Unimplemented methods
// delegate to the embedded interface (which is nil; only methods we
// override are called in tests).
type mockTx struct {
	pgx.Tx
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	rollbackFn func(ctx context.Context) error
	commitFn   func(ctx context.Context) error
}

func (m *mockTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockTx) Rollback(ctx context.Context) error {
	if m.rollbackFn != nil {
		return m.rollbackFn(ctx)
	}
	return nil
}

func (m *mockTx) Commit(ctx context.Context) error {
	if m.commitFn != nil {
		return m.commitFn(ctx)
	}
	return nil
}

func TestConfigFrom(t *testing.T) {
	t.Run("parses database config", func(t *testing.T) {
		cfg, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://user:pass@localhost:5432/app",
			AdminURL:       "postgres://admin:pass@localhost:5432/app",
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
		if cfg.AdminURL != "postgres://admin:pass@localhost:5432/app" {
			t.Errorf("AdminURL = %q, want admin database URL", cfg.AdminURL)
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

	t.Run("reports invalid max connection format", func(t *testing.T) {
		_, err := ConfigFrom(config.DatabaseConfig{
			URL:            "postgres://localhost/app",
			MaxConns:       "many",
			MinConns:       "0",
			ConnectTimeout: "5s",
		})
		if err == nil {
			t.Fatal("ConfigFrom() should reject invalid max connection format")
		}
		if !strings.Contains(err.Error(), `invalid DATABASE_MAX_CONNS "many"`) {
			t.Errorf("error = %q, want max connection parse error", err.Error())
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

func TestAdminPoolConfig(t *testing.T) {
	t.Run("returns nil when admin URL is not configured", func(t *testing.T) {
		cfg := Config{
			URL:            "postgres://user:pass@localhost:5432/app?sslmode=disable",
			MaxConns:       10,
			MinConns:       1,
			ConnectTimeout: 5 * time.Second,
		}

		poolConfig, err := cfg.AdminPoolConfig()
		if err != nil {
			t.Fatalf("AdminPoolConfig() returned unexpected error: %v", err)
		}
		if poolConfig != nil {
			t.Fatal("AdminPoolConfig() should return nil when AdminURL is empty")
		}
	})

	t.Run("converts admin URL to pgx pool config", func(t *testing.T) {
		cfg := Config{
			URL:            "postgres://user:pass@localhost:5432/app?sslmode=disable",
			AdminURL:       "postgres://admin:pass@localhost:5432/app?sslmode=disable",
			MaxConns:       10,
			MinConns:       1,
			ConnectTimeout: 5 * time.Second,
		}

		poolConfig, err := cfg.AdminPoolConfig()
		if err != nil {
			t.Fatalf("AdminPoolConfig() returned unexpected error: %v", err)
		}
		if poolConfig == nil {
			t.Fatal("AdminPoolConfig() should return an admin pool config")
		}
		if poolConfig.ConnConfig.User != "admin" {
			t.Errorf("ConnConfig.User = %q, want admin", poolConfig.ConnConfig.User)
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

	t.Run("returns error when begin fails", func(t *testing.T) {
		beginErr := errors.New("connection refused")
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return nil, beginErr
			},
		}}
		tenant := tenancy.Tenant{ID: "begin-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should return error when begin fails")
		}
		if !strings.Contains(err.Error(), "begin tenant transaction") {
			t.Errorf("error = %q, want begin error", err.Error())
		}
	})

	t.Run("returns error when set_config fails", func(t *testing.T) {
		setConfigErr := errors.New("set_config failed")
		var rollbackCalled bool
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return &mockTx{
					execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, setConfigErr
					},
					rollbackFn: func(ctx context.Context) error {
						rollbackCalled = true
						return nil
					},
				}, nil
			},
		}}
		tenant := tenancy.Tenant{ID: "setconfig-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should return error when set_config fails")
		}
		if !strings.Contains(err.Error(), "set tenant config") {
			t.Errorf("error = %q, want set tenant config error", err.Error())
		}
		if !rollbackCalled {
			t.Error("expected rollback to be called")
		}
	})

	t.Run("commits on success", func(t *testing.T) {
		var commitCalled bool
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return &mockTx{
					execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, nil
					},
					commitFn: func(ctx context.Context) error {
						commitCalled = true
						return nil
					},
				}, nil
			},
		}}
		tenant := tenancy.Tenant{ID: "commit-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err != nil {
			t.Fatalf("TenantTx() returned unexpected error: %v", err)
		}
		if !commitCalled {
			t.Error("expected commit to be called")
		}
	})

	t.Run("rolls back on callback error", func(t *testing.T) {
		var rollbackCalled bool
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return &mockTx{
					execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, nil
					},
					rollbackFn: func(ctx context.Context) error {
						rollbackCalled = true
						return nil
					},
				}, nil
			},
		}}
		tenant := tenancy.Tenant{ID: "rollback-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return fmt.Errorf("callback error")
		})
		if err == nil {
			t.Fatal("TenantTx() should return callback error")
		}
		if !strings.Contains(err.Error(), "callback error") {
			t.Errorf("error = %q, want callback error", err.Error())
		}
		if !rollbackCalled {
			t.Error("expected rollback to be called")
		}
	})

	t.Run("returns combined error when callback and rollback both fail", func(t *testing.T) {
		callbackErr := errors.New("callback error")
		rollbackErr := errors.New("rollback error")
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return &mockTx{
					execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, nil
					},
					rollbackFn: func(ctx context.Context) error {
						return rollbackErr
					},
				}, nil
			},
		}}
		tenant := tenancy.Tenant{ID: "combined-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return callbackErr
		})
		if err == nil {
			t.Fatal("TenantTx() should return error")
		}
		if !strings.Contains(err.Error(), "callback error") || !strings.Contains(err.Error(), "rollback error") {
			t.Errorf("error = %q, want both callback and rollback errors", err.Error())
		}
	})

	t.Run("rolls back and re-panics on callback panic", func(t *testing.T) {
		var rollbackCalled bool
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return &mockTx{
					execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, nil
					},
					rollbackFn: func(ctx context.Context) error {
						rollbackCalled = true
						return nil
					},
				}, nil
			},
		}}
		tenant := tenancy.Tenant{ID: "panic-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		recovered := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					recovered = true
				}
			}()
			_ = d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
				panic("deliberate panic")
			})
		}()

		if !recovered {
			t.Fatal("expected panic to propagate")
		}
		if !rollbackCalled {
			t.Error("expected rollback to be called")
		}
	})

	t.Run("returns error when commit fails", func(t *testing.T) {
		commitErr := errors.New("commit failed")
		d := &Database{pool: &mockPool{
			beginFn: func(ctx context.Context) (pgx.Tx, error) {
				return &mockTx{
					execFn: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
						return pgconn.CommandTag{}, nil
					},
					commitFn: func(ctx context.Context) error {
						return commitErr
					},
				}, nil
			},
		}}
		tenant := tenancy.Tenant{ID: "commit-err-test", Slug: "test"}
		ctx := tenancy.WithTenant(context.Background(), tenant)

		err := d.TenantTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("TenantTx() should return error when commit fails")
		}
		if !strings.Contains(err.Error(), "commit tenant transaction") {
			t.Errorf("error = %q, want commit error", err.Error())
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

	t.Run("returns error when reason is whitespace", func(t *testing.T) {
		database := &Database{pool: &pgxpool.Pool{}}
		err := database.SystemTx(context.Background(), " \t\n ", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should require non-empty reason")
		}
		if !strings.Contains(err.Error(), "non-empty reason") {
			t.Errorf("error = %q, want non-empty reason error", err.Error())
		}
	})

	t.Run("returns error when begin fails", func(t *testing.T) {
		beginErr := errors.New("connection refused")
		d := &Database{
			pool: &mockPool{},
			adminPool: &mockPool{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return nil, beginErr
				},
			},
		}

		err := d.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should return error when begin fails")
		}
		if !strings.Contains(err.Error(), "begin system transaction") {
			t.Errorf("error = %q, want begin error", err.Error())
		}
	})

	t.Run("commits on success", func(t *testing.T) {
		var commitCalled bool
		d := &Database{
			pool: &mockPool{},
			adminPool: &mockPool{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return &mockTx{
						commitFn: func(ctx context.Context) error {
							commitCalled = true
							return nil
						},
					}, nil
				},
			},
		}

		err := d.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err != nil {
			t.Fatalf("SystemTx() returned unexpected error: %v", err)
		}
		if !commitCalled {
			t.Error("expected commit to be called")
		}
	})

	t.Run("rolls back on callback error", func(t *testing.T) {
		var rollbackCalled bool
		d := &Database{
			pool: &mockPool{},
			adminPool: &mockPool{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return &mockTx{
						rollbackFn: func(ctx context.Context) error {
							rollbackCalled = true
							return nil
						},
					}, nil
				},
			},
		}

		err := d.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return fmt.Errorf("callback error")
		})
		if err == nil {
			t.Fatal("SystemTx() should return callback error")
		}
		if !strings.Contains(err.Error(), "callback error") {
			t.Errorf("error = %q, want callback error", err.Error())
		}
		if !rollbackCalled {
			t.Error("expected rollback to be called")
		}
	})

	t.Run("returns combined error when callback and rollback both fail", func(t *testing.T) {
		callbackErr := errors.New("callback error")
		rollbackErr := errors.New("rollback error")
		d := &Database{
			pool: &mockPool{},
			adminPool: &mockPool{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return &mockTx{
						rollbackFn: func(ctx context.Context) error {
							return rollbackErr
						},
					}, nil
				},
			},
		}

		err := d.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return callbackErr
		})
		if err == nil {
			t.Fatal("SystemTx() should return error")
		}
		if !strings.Contains(err.Error(), "callback error") || !strings.Contains(err.Error(), "rollback error") {
			t.Errorf("error = %q, want both callback and rollback errors", err.Error())
		}
	})

	t.Run("rolls back and re-panics on callback panic", func(t *testing.T) {
		var rollbackCalled bool
		d := &Database{
			pool: &mockPool{},
			adminPool: &mockPool{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return &mockTx{
						rollbackFn: func(ctx context.Context) error {
							rollbackCalled = true
							return nil
						},
					}, nil
				},
			},
		}

		recovered := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					recovered = true
				}
			}()
			_ = d.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
				panic("deliberate panic")
			})
		}()

		if !recovered {
			t.Fatal("expected panic to propagate")
		}
		if !rollbackCalled {
			t.Error("expected rollback to be called")
		}
	})

	t.Run("returns error when commit fails", func(t *testing.T) {
		commitErr := errors.New("commit failed")
		d := &Database{
			pool: &mockPool{},
			adminPool: &mockPool{
				beginFn: func(ctx context.Context) (pgx.Tx, error) {
					return &mockTx{
						commitFn: func(ctx context.Context) error {
							return commitErr
						},
					}, nil
				},
			},
		}

		err := d.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should return error when commit fails")
		}
		if !strings.Contains(err.Error(), "commit system transaction") {
			t.Errorf("error = %q, want commit error", err.Error())
		}
	})

	t.Run("requires admin pool", func(t *testing.T) {
		database := &Database{pool: &mockPool{}}
		err := database.SystemTx(context.Background(), "admin task", func(ctx context.Context, tx pgx.Tx) error {
			return nil
		})
		if err == nil {
			t.Fatal("SystemTx() should require admin pool")
		}
		if !strings.Contains(err.Error(), "DATABASE_ADMIN_URL") {
			t.Errorf("error = %q, want DATABASE_ADMIN_URL error", err.Error())
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

	t.Run("pings successfully with mock pool", func(t *testing.T) {
		d := &Database{pool: &mockPool{}}
		if err := d.Ping(context.Background()); err != nil {
			t.Fatalf("Ping() returned unexpected error: %v", err)
		}
	})

	t.Run("Pool returns nil for mock pool", func(t *testing.T) {
		d := &Database{pool: &mockPool{}}
		if d.Pool() != nil {
			t.Fatal("Pool() should return nil for non-pgxpool.Pool underlying pool")
		}
	})

	t.Run("closes app and admin pools", func(t *testing.T) {
		appPool := &mockPool{}
		adminPool := &mockPool{}
		d := &Database{pool: appPool, adminPool: adminPool}

		d.Close()

		if !appPool.closed {
			t.Fatal("Close() should close app pool")
		}
		if !adminPool.closed {
			t.Fatal("Close() should close admin pool")
		}
	})
}

func TestConnect(t *testing.T) {
	t.Run("returns error when database is nil", func(t *testing.T) {
		var d *Database
		err := d.Connect(context.Background(), Config{
			URL:            "postgres://localhost/app",
			MaxConns:       10,
			MinConns:       0,
			ConnectTimeout: 5 * time.Second,
		})
		if err == nil {
			t.Fatal("Connect() should error when database is nil")
		}
		if !strings.Contains(err.Error(), "database is nil") {
			t.Errorf("error = %q, want database nil error", err.Error())
		}
	})
}

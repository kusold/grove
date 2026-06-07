package db

import (
	"context"
	"strings"
	"testing"
	"time"

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
}

func TestPoolConfig(t *testing.T) {
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
}

func TestOpen(t *testing.T) {
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
}

func TestDatabaseNilSafety(t *testing.T) {
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
}

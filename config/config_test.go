package config

import "testing"

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SERVICE_NAME",
		"SERVICE_ENV",
		"SERVICE_VERSION",
		"HTTP_ADDR",
		"HTTP_SHUTDOWN_TIMEOUT",
		"DATABASE_URL",
		"DATABASE_ADMIN_URL",
		"DATABASE_MAX_CONNS",
		"DATABASE_MIN_CONNS",
		"DATABASE_CONNECT_TIMEOUT",
		"LOG_FORMAT",
		"LOG_COLOR",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad(t *testing.T) {
	t.Run("uses module name as default service name", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("canopy")
		if cfg.Service().Name != "canopy" {
			t.Errorf("Service.Name = %q, want %q", cfg.Service().Name, "canopy")
		}
	})

	t.Run("SERVICE_NAME overrides module name", func(t *testing.T) {
		t.Setenv("SERVICE_NAME", "production-canopy")
		cfg := Load("canopy")
		if cfg.Service().Name != "production-canopy" {
			t.Errorf("Service.Name = %q, want %q", cfg.Service().Name, "production-canopy")
		}
	})

	t.Run("default environment is development", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("test")
		if cfg.Service().Environment != "development" {
			t.Errorf("Service.Environment = %q, want %q", cfg.Service().Environment, "development")
		}
	})

	t.Run("SERVICE_ENV overrides default environment", func(t *testing.T) {
		t.Setenv("SERVICE_ENV", "production")
		cfg := Load("test")
		if cfg.Service().Environment != "production" {
			t.Errorf("Service.Environment = %q, want %q", cfg.Service().Environment, "production")
		}
	})

	t.Run("default version is dev", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("test")
		if cfg.Service().Version != "dev" {
			t.Errorf("Service.Version = %q, want %q", cfg.Service().Version, "dev")
		}
	})

	t.Run("SERVICE_VERSION overrides default version", func(t *testing.T) {
		t.Setenv("SERVICE_VERSION", "v1.2.3")
		cfg := Load("test")
		if cfg.Service().Version != "v1.2.3" {
			t.Errorf("Service.Version = %q, want %q", cfg.Service().Version, "v1.2.3")
		}
	})

	t.Run("default HTTP addr is :8080", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("test")
		if cfg.HTTP().Addr != ":8080" {
			t.Errorf("HTTP.Addr = %q, want %q", cfg.HTTP().Addr, ":8080")
		}
	})

	t.Run("HTTP_ADDR overrides default addr", func(t *testing.T) {
		t.Setenv("HTTP_ADDR", ":9090")
		cfg := Load("test")
		if cfg.HTTP().Addr != ":9090" {
			t.Errorf("HTTP.Addr = %q, want %q", cfg.HTTP().Addr, ":9090")
		}
	})

	t.Run("empty env var is treated as unset", func(t *testing.T) {
		t.Setenv("SERVICE_NAME", "")
		cfg := Load("canopy")
		if cfg.Service().Name != "canopy" {
			t.Errorf("Service.Name = %q, want %q when env var is empty", cfg.Service().Name, "canopy")
		}
	})

	t.Run("all values can be set simultaneously", func(t *testing.T) {
		t.Setenv("SERVICE_NAME", "my-service")
		t.Setenv("SERVICE_ENV", "staging")
		t.Setenv("SERVICE_VERSION", "v2.0.0")
		t.Setenv("HTTP_ADDR", ":3000")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "20s")
		t.Setenv("DATABASE_URL", "postgres://localhost/app")
		t.Setenv("DATABASE_ADMIN_URL", "postgres://admin@localhost/app")
		t.Setenv("DATABASE_MAX_CONNS", "20")
		t.Setenv("DATABASE_MIN_CONNS", "2")
		t.Setenv("DATABASE_CONNECT_TIMEOUT", "7s")
		t.Setenv("LOG_FORMAT", "json")

		cfg := Load("unused-module-name")
		if cfg.Service().Name != "my-service" {
			t.Errorf("Service.Name = %q, want %q", cfg.Service().Name, "my-service")
		}
		if cfg.Service().Environment != "staging" {
			t.Errorf("Service.Environment = %q, want %q", cfg.Service().Environment, "staging")
		}
		if cfg.Service().Version != "v2.0.0" {
			t.Errorf("Service.Version = %q, want %q", cfg.Service().Version, "v2.0.0")
		}
		if cfg.HTTP().Addr != ":3000" {
			t.Errorf("HTTP.Addr = %q, want %q", cfg.HTTP().Addr, ":3000")
		}
		if cfg.HTTP().ShutdownTimeout != "20s" {
			t.Errorf("HTTP.ShutdownTimeout = %q, want %q", cfg.HTTP().ShutdownTimeout, "20s")
		}
		if cfg.Database().URL != "postgres://localhost/app" {
			t.Errorf("Database.URL = %q, want %q", cfg.Database().URL, "postgres://localhost/app")
		}
		if cfg.Database().AdminURL != "postgres://admin@localhost/app" {
			t.Errorf("Database.AdminURL = %q, want %q", cfg.Database().AdminURL, "postgres://admin@localhost/app")
		}
		if cfg.Database().MaxConns != "20" {
			t.Errorf("Database.MaxConns = %q, want %q", cfg.Database().MaxConns, "20")
		}
		if cfg.Database().MinConns != "2" {
			t.Errorf("Database.MinConns = %q, want %q", cfg.Database().MinConns, "2")
		}
		if cfg.Database().ConnectTimeout != "7s" {
			t.Errorf("Database.ConnectTimeout = %q, want %q", cfg.Database().ConnectTimeout, "7s")
		}
		if cfg.Logger().Format != "json" {
			t.Errorf("Logger.Format = %q, want %q", cfg.Logger().Format, "json")
		}
	})
}

func TestLoggerConfig(t *testing.T) {
	t.Run("default log format is text", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("test")
		if cfg.Logger().Format != "text" {
			t.Errorf("Logger.Format = %q, want %q", cfg.Logger().Format, "text")
		}
	})

	t.Run("LOG_FORMAT=json overrides default", func(t *testing.T) {
		t.Setenv("LOG_FORMAT", "json")
		cfg := Load("test")
		if cfg.Logger().Format != "json" {
			t.Errorf("Logger.Format = %q, want %q", cfg.Logger().Format, "json")
		}
	})

	t.Run("empty LOG_FORMAT is treated as unset", func(t *testing.T) {
		t.Setenv("LOG_FORMAT", "")
		cfg := Load("test")
		if cfg.Logger().Format != "text" {
			t.Errorf("Logger.Format = %q, want %q when env var is empty", cfg.Logger().Format, "text")
		}
	})

	t.Run("default log color is auto", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("test")
		if cfg.Logger().Color != "auto" {
			t.Errorf("Logger.Color = %q, want %q", cfg.Logger().Color, "auto")
		}
	})

	t.Run("LOG_COLOR=on overrides default", func(t *testing.T) {
		t.Setenv("LOG_COLOR", "on")
		cfg := Load("test")
		if cfg.Logger().Color != "on" {
			t.Errorf("Logger.Color = %q, want %q", cfg.Logger().Color, "on")
		}
	})

	t.Run("LOG_COLOR=off overrides default", func(t *testing.T) {
		t.Setenv("LOG_COLOR", "off")
		cfg := Load("test")
		if cfg.Logger().Color != "off" {
			t.Errorf("Logger.Color = %q, want %q", cfg.Logger().Color, "off")
		}
	})

	t.Run("empty LOG_COLOR is treated as unset", func(t *testing.T) {
		t.Setenv("LOG_COLOR", "")
		cfg := Load("test")
		if cfg.Logger().Color != "auto" {
			t.Errorf("Logger.Color = %q, want %q when env var is empty", cfg.Logger().Color, "auto")
		}
	})
}

func TestDatabaseConfig(t *testing.T) {
	t.Run("defaults database pool settings", func(t *testing.T) {
		clearConfigEnv(t)
		cfg := Load("test")
		if cfg.Database().URL != "" {
			t.Errorf("Database.URL = %q, want empty default", cfg.Database().URL)
		}
		if cfg.Database().AdminURL != "" {
			t.Errorf("Database.AdminURL = %q, want empty default", cfg.Database().AdminURL)
		}
		if cfg.Database().MaxConns != "10" {
			t.Errorf("Database.MaxConns = %q, want %q", cfg.Database().MaxConns, "10")
		}
		if cfg.Database().MinConns != "0" {
			t.Errorf("Database.MinConns = %q, want %q", cfg.Database().MinConns, "0")
		}
		if cfg.Database().ConnectTimeout != "5s" {
			t.Errorf("Database.ConnectTimeout = %q, want %q", cfg.Database().ConnectTimeout, "5s")
		}
	})

	t.Run("empty database env vars are treated as unset", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("DATABASE_MAX_CONNS", "")
		t.Setenv("DATABASE_MIN_CONNS", "")
		t.Setenv("DATABASE_CONNECT_TIMEOUT", "")
		cfg := Load("test")
		if cfg.Database().MaxConns != "10" {
			t.Errorf("Database.MaxConns = %q, want %q", cfg.Database().MaxConns, "10")
		}
		if cfg.Database().MinConns != "0" {
			t.Errorf("Database.MinConns = %q, want %q", cfg.Database().MinConns, "0")
		}
		if cfg.Database().ConnectTimeout != "5s" {
			t.Errorf("Database.ConnectTimeout = %q, want %q", cfg.Database().ConnectTimeout, "5s")
		}
	})
}

func TestLoadDoesNotReadAllEnvVars(t *testing.T) {
	t.Run("only reads specific known variables", func(t *testing.T) {
		clearConfigEnv(t)
		// Ensure that adding random env vars doesn't affect Load
		t.Setenv("RANDOM_VAR_12345", "should-be-ignored")
		cfg := Load("test")
		if cfg.Service().Name != "test" {
			t.Errorf("unexpected change from random env var")
		}
	})
}

package config

import "testing"

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SERVICE_NAME", "SERVICE_ENV", "SERVICE_VERSION", "HTTP_ADDR", "LOG_FORMAT"} {
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
}

func TestEnvOr(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_KEY", "value")
		if got := envOr("TEST_KEY", "fallback"); got != "value" {
			t.Errorf("envOr() = %q, want %q", got, "value")
		}
	})

	t.Run("returns fallback when not set", func(t *testing.T) {
		if got := envOr("UNSET_TEST_KEY_XYZ", "fallback"); got != "fallback" {
			t.Errorf("envOr() = %q, want %q", got, "fallback")
		}
	})

	t.Run("returns fallback when set to empty string", func(t *testing.T) {
		t.Setenv("TEST_EMPTY_KEY", "")
		if got := envOr("TEST_EMPTY_KEY", "fallback"); got != "fallback" {
			t.Errorf("envOr() = %q, want %q for empty env var", got, "fallback")
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

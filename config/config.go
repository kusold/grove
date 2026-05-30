package config

import (
	"os"
	"time"
)

// ServiceConfig holds service identity configuration.
type ServiceConfig struct {
	Name            string
	Environment     string
	Version         string
	ShutdownTimeout time.Duration
}

// HTTPConfig holds HTTP server configuration.
type HTTPConfig struct {
	Addr string
}

// Config holds all configuration for a Grove service.
type Config struct {
	Service ServiceConfig
	HTTP    HTTPConfig
}

// Default returns a Config populated with sensible defaults and
// environment variable overrides.
func Default() *Config {
	cfg := &Config{
		Service: ServiceConfig{
			Name:            envOr("SERVICE_NAME", "grove"),
			Environment:     envOr("SERVICE_ENV", "development"),
			Version:         envOr("SERVICE_VERSION", "dev"),
			ShutdownTimeout: 30 * time.Second,
		},
		HTTP: HTTPConfig{
			Addr: envOr("HTTP_ADDR", ":8080"),
		},
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

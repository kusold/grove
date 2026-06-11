// Package config provides environment-variable-based configuration for Grove
// services. It loads service identity and HTTP settings from the process
// environment with sensible defaults.
//
// Environment variables:
//
//	SERVICE_NAME     — overrides the runtime service name (default: none)
//	SERVICE_ENV      — deployment environment (default: "development")
//	SERVICE_VERSION  — service version string (default: "dev")
//	HTTP_ADDR        — listen address for the HTTP server (default: ":8080")
//	DATABASE_URL     — Postgres application connection URL (default: none)
//	DATABASE_ADMIN_URL — privileged Postgres connection URL for system transactions (default: none)
//	DATABASE_MAX_CONNS — maximum pgx pool connections (default: "10")
//	DATABASE_MIN_CONNS — minimum pgx pool connections (default: "0")
//	DATABASE_CONNECT_TIMEOUT — Postgres connection timeout (default: "5s")
//	LOG_FORMAT       — log output format: "text" or "json" (default: "text")
//	LOG_COLOR        — colorize text output: "on", "off", or "auto" (default: "auto")
//
// If both Module.Name() and SERVICE_NAME are set, SERVICE_NAME overrides the
// runtime config but does not change module identity. This distinction is
// important: Module.Name() is the stable identifier used for logging and
// wiring, while Config.Service().Name is the runtime name that may differ per
// deployment.
package config

import (
	"os"
	"strings"

	env "github.com/caarlos0/env/v11"
)

// Provider exposes Grove service configuration without requiring callers to
// depend on a concrete config source.
type Provider interface {
	Service() ServiceConfig
	HTTP() HTTPConfig
	Database() DatabaseConfig
	Logger() LoggerConfig
}

// Config holds all configuration for a Grove service, grouped by subsystem.
type Config struct {
	service ServiceConfig
	http    HTTPConfig
	db      DatabaseConfig
	logger  LoggerConfig
}

var _ Provider = (*Config)(nil)

// ServiceConfig holds service identity configuration.
type ServiceConfig struct {
	// Runtime service name. If SERVICE_NAME is set, it overrides the name
	// derived from Module.Name(). If SERVICE_NAME is empty, the module name is
	// used as the runtime name.
	Name string `env:"NAME"`

	// Deployment environment, such as "development", "staging", or
	// "production".
	Environment string `env:"ENV" envDefault:"development"`

	// Service version string, typically set by the build pipeline.
	Version string `env:"VERSION" envDefault:"dev"`
}

// HTTPConfig holds HTTP server configuration.
type HTTPConfig struct {
	// Listen address for the HTTP server, such as ":8080".
	Addr string `env:"ADDR" envDefault:":8080"`

	// Maximum duration to wait for the HTTP server to complete in-flight
	// requests during graceful shutdown.
	ShutdownTimeout string `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}

// DatabaseConfig holds Postgres database configuration as loaded from the
// environment. The db package parses and validates these values for pgx.
type DatabaseConfig struct {
	// Postgres application connection URL. It is required when the Postgres
	// capability connects to the database.
	URL string `env:"URL"`

	// Privileged Postgres connection URL for system transactions that need to
	// bypass tenant RLS. It is optional at startup, but required when SystemTx is
	// used.
	AdminURL string `env:"ADMIN_URL"`

	// Maximum number of connections in the pgx pool.
	MaxConns string `env:"MAX_CONNS" envDefault:"10"`

	// Minimum number of connections in the pgx pool.
	MinConns string `env:"MIN_CONNS" envDefault:"0"`

	// Timeout for establishing a Postgres connection.
	ConnectTimeout string `env:"CONNECT_TIMEOUT" envDefault:"5s"`
}

// LoggerConfig holds logger configuration.
type LoggerConfig struct {
	// Log output format. Valid values are "text" and "json".
	Format string `env:"FORMAT" envDefault:"text"`

	// ANSI colorization for text log output. Valid values are "on" (always
	// colorize), "off" (never colorize), and "auto" (colorize only when output
	// is a terminal). Colorization is only applied when Format is "text".
	Color string `env:"COLOR" envDefault:"auto"`
}

// Load reads configuration from environment variables. It applies sensible
// defaults for any values that are not explicitly set.
//
// The moduleName parameter comes from Module.Name() and is used as the
// default service name when SERVICE_NAME is not set in the environment.
func Load(moduleName string) *Config {
	loaded := envConfig{
		Service: ServiceConfig{Name: moduleName},
	}
	if err := env.ParseWithOptions(&loaded, env.Options{Environment: nonEmptyEnvironment()}); err != nil {
		panic(err)
	}
	return &Config{
		service: loaded.Service,
		http:    loaded.HTTP,
		db:      loaded.Database,
		logger:  loaded.Logger,
	}
}

// Service returns service identity configuration.
func (c *Config) Service() ServiceConfig {
	return c.service
}

// HTTP returns HTTP server configuration.
func (c *Config) HTTP() HTTPConfig {
	return c.http
}

// Database returns Postgres database configuration.
func (c *Config) Database() DatabaseConfig {
	return c.db
}

// Logger returns logger configuration.
func (c *Config) Logger() LoggerConfig {
	return c.logger
}

//go:generate sh -c "go tool github.com/g4s8/envdoc -types=envConfig -output ../docs/environment.md && perl -0pi -e 's/\\n*\\z/\\n/' ../docs/environment.md"
type envConfig struct {
	// Service identity configuration.
	Service ServiceConfig `envPrefix:"SERVICE_"`

	// HTTP server configuration.
	HTTP HTTPConfig `envPrefix:"HTTP_"`

	// Postgres database configuration.
	Database DatabaseConfig `envPrefix:"DATABASE_"`

	// Logger configuration.
	Logger LoggerConfig `envPrefix:"LOG_"`
}

func nonEmptyEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || value == "" {
			continue
		}
		environment[key] = value
	}
	return environment
}

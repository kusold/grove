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
//	LOG_FORMAT       — log output format: "text" or "json" (default: "text")
//	LOG_COLOR        — colorize text output: "on", "off", or "auto" (default: "auto")
//
// If both Module.Name() and SERVICE_NAME are set, SERVICE_NAME overrides the
// runtime config but does not change module identity. This distinction is
// important: Module.Name() is the stable identifier used for logging and
// wiring, while Config.Service().Name is the runtime name that may differ per
// deployment.
package config

import "os"

// Provider exposes Grove service configuration without requiring callers to
// depend on a concrete config source.
type Provider interface {
	Service() ServiceConfig
	HTTP() HTTPConfig
	Logger() LoggerConfig
}

// Config holds all configuration for a Grove service, grouped by subsystem.
type Config struct {
	service ServiceConfig
	http    HTTPConfig
	logger  LoggerConfig
}

var _ Provider = (*Config)(nil)

// ServiceConfig holds service identity configuration.
type ServiceConfig struct {
	// Name is the runtime service name. If SERVICE_NAME is set, it overrides
	// the name derived from Module.Name(). If SERVICE_NAME is empty, the
	// module name is used as the runtime name.
	Name string

	// Environment is the deployment environment (e.g. "development",
	// "staging", "production").
	Environment string

	// Version is the service version string, typically set by the build
	// pipeline.
	Version string
}

// HTTPConfig holds HTTP server configuration.
type HTTPConfig struct {
	// Addr is the listen address for the HTTP server (e.g. ":8080").
	Addr string
}

// LoggerConfig holds logger configuration.
type LoggerConfig struct {
	// Format controls the log output format. Valid values are "text" and "json".
	// Defaults to "text".
	Format string

	// Color controls ANSI colorization of text log output. Valid values are
	// "on" (always colorize), "off" (never colorize), and "auto" (colorize only
	// when output is a terminal). Defaults to "auto". Colorization is only
	// applied when Format is "text".
	Color string
}

// Load reads configuration from environment variables. It applies sensible
// defaults for any values that are not explicitly set.
//
// The moduleName parameter comes from Module.Name() and is used as the
// default service name when SERVICE_NAME is not set in the environment.
func Load(moduleName string) *Config {
	return &Config{
		service: ServiceConfig{
			Name:        envOr("SERVICE_NAME", moduleName),
			Environment: envOr("SERVICE_ENV", "development"),
			Version:     envOr("SERVICE_VERSION", "dev"),
		},
		http: HTTPConfig{
			Addr: envOr("HTTP_ADDR", ":8080"),
		},
		logger: LoggerConfig{
			Format: envOr("LOG_FORMAT", "text"),
			Color:  envOr("LOG_COLOR", "auto"),
		},
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

// Logger returns logger configuration.
func (c *Config) Logger() LoggerConfig {
	return c.logger
}

// envOr returns the value of the environment variable named by the key, or
// the provided fallback value if the variable is not set or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

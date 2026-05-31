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
//
// If both Module.Name() and SERVICE_NAME are set, SERVICE_NAME overrides the
// runtime config but does not change module identity. This distinction is
// important: Module.Name() is the stable identifier used for logging and
// wiring, while Config.Service.Name is the runtime name that may differ per
// deployment.
package config

import "os"

// Config holds all configuration for a Grove service, grouped by subsystem.
type Config struct {
	Service ServiceConfig
	HTTP    HTTPConfig
}

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

// Load reads configuration from environment variables. It applies sensible
// defaults for any values that are not explicitly set.
//
// The moduleName parameter comes from Module.Name() and is used as the
// default service name when SERVICE_NAME is not set in the environment.
func Load(moduleName string) *Config {
	return &Config{
		Service: ServiceConfig{
			Name:        envOr("SERVICE_NAME", moduleName),
			Environment: envOr("SERVICE_ENV", "development"),
			Version:     envOr("SERVICE_VERSION", "dev"),
		},
		HTTP: HTTPConfig{
			Addr: envOr("HTTP_ADDR", ":8080"),
		},
	}
}

// envOr returns the value of the environment variable named by the key, or
// the provided fallback value if the variable is not set or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

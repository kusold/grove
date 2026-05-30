package config

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Service.Name == "" {
		t.Error("expected non-empty service name")
	}
	if cfg.Service.Environment == "" {
		t.Error("expected non-empty service environment")
	}
	if cfg.Service.Version == "" {
		t.Error("expected non-empty service version")
	}
	if cfg.HTTP.Addr == "" {
		t.Error("expected non-empty HTTP addr")
	}
}

func TestDefaultWithEnvOverride(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("HTTP_ADDR", ":9090")

	cfg := Default()
	if cfg.Service.Name != "test-service" {
		t.Errorf("expected service name 'test-service', got %q", cfg.Service.Name)
	}
	if cfg.HTTP.Addr != ":9090" {
		t.Errorf("expected HTTP addr ':9090', got %q", cfg.HTTP.Addr)
	}
}

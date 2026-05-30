package grove

import (
	"log/slog"
	"testing"

	"github.com/kusold/grove/config"
)

func TestApp_Name(t *testing.T) {
	app, err := newApp("test-svc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Name() != "test-svc" {
		t.Errorf("expected 'test-svc', got %q", app.Name())
	}
}

func TestApp_ConfigDefaults(t *testing.T) {
	app, err := newApp("test-svc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := app.Config()
	if cfg.Service.Name != "test-svc" {
		t.Errorf("expected service name 'test-svc', got %q", cfg.Service.Name)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("expected default addr ':8080', got %q", cfg.HTTP.Addr)
	}
}

func TestApp_Logger(t *testing.T) {
	app, err := newApp("test-svc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Logger() == nil {
		t.Error("expected non-nil default logger")
	}
}

func TestApp_HealthRegistry(t *testing.T) {
	app, err := newApp("test-svc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Health() == nil {
		t.Error("expected non-nil health registry")
	}
}

func TestApp_LifecycleManager(t *testing.T) {
	app, err := newApp("test-svc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Lifecycle() == nil {
		t.Error("expected non-nil lifecycle manager")
	}
}

func TestApp_WithConfig(t *testing.T) {
	cfg := &config.Config{
		Service: config.ServiceConfig{
			Name:            "overridden",
			Environment:     "production",
			Version:         "2.0",
			ShutdownTimeout: 10,
		},
		HTTP: config.HTTPConfig{Addr: ":3000"},
	}
	app, err := newApp("test-svc", []Option{WithConfig(cfg)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Config().HTTP.Addr != ":3000" {
		t.Errorf("expected ':3000', got %q", app.Config().HTTP.Addr)
	}
}

func TestApp_WithLogger(t *testing.T) {
	logger := slog.Default()
	app, err := newApp("test-svc", []Option{WithLogger(logger)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.Logger() != logger {
		t.Error("expected custom logger to be set")
	}
}

func TestApp_WithNilLogger(t *testing.T) {
	_, err := newApp("test-svc", []Option{WithLogger(nil)})
	if err == nil {
		t.Fatal("expected error for nil logger")
	}
}

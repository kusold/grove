package health

import (
	"context"
	"errors"
	"testing"
)

func TestRegistry_IsHealthy_NoChecks(t *testing.T) {
	r := NewRegistry()
	if err := r.IsHealthy(context.Background()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRegistry_IsHealthy_AllPass(t *testing.T) {
	r := NewRegistry()
	r.AddHealthCheck(Check{
		Name: "test",
		Fn:   func(ctx context.Context) error { return nil },
	})
	if err := r.IsHealthy(context.Background()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRegistry_IsHealthy_CheckFails(t *testing.T) {
	r := NewRegistry()
	r.AddHealthCheck(Check{
		Name: "failing",
		Fn:   func(ctx context.Context) error { return errors.New("broken") },
	})
	if err := r.IsHealthy(context.Background()); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestRegistry_IsReady_NoChecks(t *testing.T) {
	r := NewRegistry()
	if err := r.IsReady(context.Background()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRegistry_IsReady_CheckFails(t *testing.T) {
	r := NewRegistry()
	r.AddReadyCheck(Check{
		Name: "db",
		Fn:   func(ctx context.Context) error { return errors.New("db down") },
	})
	if err := r.IsReady(context.Background()); err == nil {
		t.Error("expected error, got nil")
	}
}

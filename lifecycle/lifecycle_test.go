package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestManager_StartStop_Order(t *testing.T) {
	m := NewManager()
	var order []string

	m.Append(Hook{
		Name: "first",
		Start: func(ctx context.Context) error {
			order = append(order, "start-first")
			return nil
		},
		Stop: func(ctx context.Context) error {
			order = append(order, "stop-first")
			return nil
		},
	})
	m.Append(Hook{
		Name: "second",
		Start: func(ctx context.Context) error {
			order = append(order, "start-second")
			return nil
		},
		Stop: func(ctx context.Context) error {
			order = append(order, "stop-second")
			return nil
		},
	})

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	expected := []string{"start-first", "start-second", "stop-second", "stop-first"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("index %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestManager_Start_Error(t *testing.T) {
	m := NewManager()
	m.Append(Hook{
		Name: "good",
		Start: func(ctx context.Context) error {
			return nil
		},
	})
	m.Append(Hook{
		Name: "bad",
		Start: func(ctx context.Context) error {
			return errors.New("boom")
		},
	})

	err := m.Start(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "boom" {
		t.Errorf("expected 'boom', got %q", err.Error())
	}
}

func TestManager_Stop_ContinuesOnError(t *testing.T) {
	m := NewManager()
	var stopped []string

	m.Append(Hook{
		Name: "a",
		Stop: func(ctx context.Context) error {
			stopped = append(stopped, "a")
			return errors.New("a failed")
		},
	})
	m.Append(Hook{
		Name: "b",
		Stop: func(ctx context.Context) error {
			stopped = append(stopped, "b")
			return nil
		},
	})

	err := m.Stop(context.Background())
	if err == nil {
		t.Fatal("expected error from first stop hook")
	}
	if len(stopped) != 2 {
		t.Errorf("expected both stops to run, got %v", stopped)
	}
}

func TestManager_NilHooks(t *testing.T) {
	m := NewManager()
	m.Append(Hook{Name: "empty"})
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

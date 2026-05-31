package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("returns non-nil manager", func(t *testing.T) {
		m := New()
		if m == nil {
			t.Fatal("New() returned nil")
		}
	})
}

func TestAppend(t *testing.T) {
	t.Run("hooks accumulate in order", func(t *testing.T) {
		m := New()
		m.Append(Hook{Name: "first"})
		m.Append(Hook{Name: "second"})
		m.Append(Hook{Name: "third"})

		if len(m.hooks) != 3 {
			t.Fatalf("expected 3 hooks, got %d", len(m.hooks))
		}
		names := []string{m.hooks[0].Name, m.hooks[1].Name, m.hooks[2].Name}
		want := []string{"first", "second", "third"}
		for i, n := range names {
			if n != want[i] {
				t.Errorf("hook[%d].Name = %q, want %q", i, n, want[i])
			}
		}
	})
}

func TestStart(t *testing.T) {
	t.Run("runs hooks in registration order", func(t *testing.T) {
		m := New()
		var order []string
		m.Append(Hook{Name: "a", Start: func(ctx context.Context) error {
			order = append(order, "a")
			return nil
		}})
		m.Append(Hook{Name: "b", Start: func(ctx context.Context) error {
			order = append(order, "b")
			return nil
		}})
		m.Append(Hook{Name: "c", Start: func(ctx context.Context) error {
			order = append(order, "c")
			return nil
		}})

		if err := m.Start(context.Background()); err != nil {
			t.Fatalf("Start() returned unexpected error: %v", err)
		}
		if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
			t.Errorf("hooks ran in wrong order: %v", order)
		}
	})

	t.Run("stops on first error", func(t *testing.T) {
		m := New()
		var ran []string
		hookErr := errors.New("boom")

		m.Append(Hook{Name: "a", Start: func(ctx context.Context) error {
			ran = append(ran, "a")
			return nil
		}})
		m.Append(Hook{Name: "b", Start: func(ctx context.Context) error {
			ran = append(ran, "b")
			return hookErr
		}})
		m.Append(Hook{Name: "c", Start: func(ctx context.Context) error {
			ran = append(ran, "c")
			return nil
		}})

		err := m.Start(context.Background())
		if err == nil {
			t.Fatal("expected error from Start()")
		}
		if !errors.Is(err, hookErr) {
			t.Errorf("error = %v, want to wrap %v", err, hookErr)
		}
		if !containsStr(ran, "a") || !containsStr(ran, "b") {
			t.Errorf("expected a and b to run, got %v", ran)
		}
		if containsStr(ran, "c") {
			t.Error("c should not have run after b failed")
		}
		if err.Error() != `lifecycle hook "b" start: boom` {
			t.Errorf("error message = %q, want hook name in error", err.Error())
		}
	})

	t.Run("rolls back started hooks on error", func(t *testing.T) {
		m := New()
		var order []string
		hookErr := errors.New("boom")

		m.Append(Hook{
			Name: "a",
			Start: func(ctx context.Context) error {
				order = append(order, "start-a")
				return nil
			},
			Stop: func(ctx context.Context) error {
				order = append(order, "stop-a")
				return nil
			},
		})
		m.Append(Hook{
			Name: "b",
			Start: func(ctx context.Context) error {
				order = append(order, "start-b")
				return nil
			},
			Stop: func(ctx context.Context) error {
				order = append(order, "stop-b")
				return nil
			},
		})
		m.Append(Hook{
			Name: "c",
			Start: func(ctx context.Context) error {
				order = append(order, "start-c")
				return hookErr
			},
			Stop: func(ctx context.Context) error {
				order = append(order, "stop-c")
				return nil
			},
		})
		m.Append(Hook{
			Name: "d",
			Start: func(ctx context.Context) error {
				order = append(order, "start-d")
				return nil
			},
			Stop: func(ctx context.Context) error {
				order = append(order, "stop-d")
				return nil
			},
		})

		err := m.Start(context.Background())
		if err == nil {
			t.Fatal("expected error from Start()")
		}
		if !errors.Is(err, hookErr) {
			t.Errorf("error = %v, want to wrap %v", err, hookErr)
		}
		want := []string{"start-a", "start-b", "start-c", "stop-b", "stop-a"}
		if len(order) != len(want) {
			t.Fatalf("order = %v, want %v", order, want)
		}
		for i, v := range order {
			if v != want[i] {
				t.Errorf("order[%d] = %q, want %q", i, v, want[i])
			}
		}
	})

	t.Run("returns rollback error with start error", func(t *testing.T) {
		m := New()
		startErr := errors.New("start boom")
		stopErr := errors.New("stop boom")

		m.Append(Hook{
			Name:  "a",
			Start: func(ctx context.Context) error { return nil },
			Stop:  func(ctx context.Context) error { return stopErr },
		})
		m.Append(Hook{
			Name:  "b",
			Start: func(ctx context.Context) error { return startErr },
		})

		err := m.Start(context.Background())
		if err == nil {
			t.Fatal("expected error from Start()")
		}
		if !errors.Is(err, startErr) {
			t.Errorf("error = %v, want to wrap start error %v", err, startErr)
		}
		if !errors.Is(err, stopErr) {
			t.Errorf("error = %v, want to wrap rollback error %v", err, stopErr)
		}
	})

	t.Run("skips nil Start functions", func(t *testing.T) {
		m := New()
		var ran []string

		m.Append(Hook{Name: "a", Start: func(ctx context.Context) error {
			ran = append(ran, "a")
			return nil
		}})
		m.Append(Hook{Name: "b"}) // no Start
		m.Append(Hook{Name: "c", Start: func(ctx context.Context) error {
			ran = append(ran, "c")
			return nil
		}})

		if err := m.Start(context.Background()); err != nil {
			t.Fatalf("Start() returned unexpected error: %v", err)
		}
		if len(ran) != 2 || ran[0] != "a" || ran[1] != "c" {
			t.Errorf("expected [a c], got %v", ran)
		}
	})

	t.Run("returns nil with no hooks", func(t *testing.T) {
		m := New()
		if err := m.Start(context.Background()); err != nil {
			t.Fatalf("Start() returned unexpected error: %v", err)
		}
	})

	t.Run("passes context to hooks", func(t *testing.T) {
		m := New()
		var receivedCtx context.Context
		m.Append(Hook{Name: "ctx-check", Start: func(ctx context.Context) error {
			receivedCtx = ctx
			return nil
		}})

		ctx := context.Background()
		if err := m.Start(ctx); err != nil {
			t.Fatalf("Start() returned unexpected error: %v", err)
		}
		if receivedCtx != ctx {
			t.Error("context not passed to hook")
		}
	})
}

func TestStop(t *testing.T) {
	t.Run("runs hooks in reverse order", func(t *testing.T) {
		m := New()
		var order []string
		m.Append(Hook{Name: "a", Stop: func(ctx context.Context) error {
			order = append(order, "a")
			return nil
		}})
		m.Append(Hook{Name: "b", Stop: func(ctx context.Context) error {
			order = append(order, "b")
			return nil
		}})
		m.Append(Hook{Name: "c", Stop: func(ctx context.Context) error {
			order = append(order, "c")
			return nil
		}})

		if err := m.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() returned unexpected error: %v", err)
		}
		if len(order) != 3 || order[0] != "c" || order[1] != "b" || order[2] != "a" {
			t.Errorf("hooks ran in wrong order: %v, want [c b a]", order)
		}
	})

	t.Run("continues on error and returns first error", func(t *testing.T) {
		m := New()
		var ran []string
		err1 := errors.New("boom1")
		err2 := errors.New("boom2")

		m.Append(Hook{Name: "a", Stop: func(ctx context.Context) error {
			ran = append(ran, "a")
			return err1
		}})
		m.Append(Hook{Name: "b", Stop: func(ctx context.Context) error {
			ran = append(ran, "b")
			return err2
		}})
		m.Append(Hook{Name: "c", Stop: func(ctx context.Context) error {
			ran = append(ran, "c")
			return nil
		}})

		err := m.Stop(context.Background())
		if err == nil {
			t.Fatal("expected error from Stop()")
		}
		// Should return the first error encountered (hooks stop in reverse: c→b→a,
		// c returns nil, b returns err2, so first error is err2 from hook "b")
		if !errors.Is(err, err2) {
			t.Errorf("error = %v, want to wrap first error %v", err, err2)
		}
		// All hooks should still have run
		if len(ran) != 3 {
			t.Errorf("expected all 3 hooks to run, got %v", ran)
		}
		if ran[0] != "c" || ran[1] != "b" || ran[2] != "a" {
			t.Errorf("hooks ran in wrong order: %v, want [c b a]", ran)
		}
	})

	t.Run("skips nil Stop functions", func(t *testing.T) {
		m := New()
		var ran []string

		m.Append(Hook{Name: "a", Stop: func(ctx context.Context) error {
			ran = append(ran, "a")
			return nil
		}})
		m.Append(Hook{Name: "b"}) // no Stop
		m.Append(Hook{Name: "c", Stop: func(ctx context.Context) error {
			ran = append(ran, "c")
			return nil
		}})

		if err := m.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() returned unexpected error: %v", err)
		}
		if len(ran) != 2 || ran[0] != "c" || ran[1] != "a" {
			t.Errorf("expected [c a], got %v", ran)
		}
	})

	t.Run("returns nil with no hooks", func(t *testing.T) {
		m := New()
		if err := m.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() returned unexpected error: %v", err)
		}
	})

	t.Run("passes context to hooks", func(t *testing.T) {
		m := New()
		var receivedCtx context.Context
		m.Append(Hook{Name: "ctx-check", Stop: func(ctx context.Context) error {
			receivedCtx = ctx
			return nil
		}})

		ctx := context.Background()
		if err := m.Stop(ctx); err != nil {
			t.Fatalf("Stop() returned unexpected error: %v", err)
		}
		if receivedCtx != ctx {
			t.Error("context not passed to hook")
		}
	})
}

func TestStartAndStop(t *testing.T) {
	t.Run("full lifecycle runs in correct order", func(t *testing.T) {
		m := New()
		var order []string

		m.Append(Hook{
			Name: "db",
			Start: func(ctx context.Context) error {
				order = append(order, "start-db")
				return nil
			},
			Stop: func(ctx context.Context) error {
				order = append(order, "stop-db")
				return nil
			},
		})
		m.Append(Hook{
			Name: "http",
			Start: func(ctx context.Context) error {
				order = append(order, "start-http")
				return nil
			},
			Stop: func(ctx context.Context) error {
				order = append(order, "stop-http")
				return nil
			},
		})

		ctx := context.Background()
		if err := m.Start(ctx); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		if err := m.Stop(ctx); err != nil {
			t.Fatalf("Stop() error: %v", err)
		}

		want := []string{"start-db", "start-http", "stop-http", "stop-db"}
		if len(order) != len(want) {
			t.Fatalf("order = %v, want %v", order, want)
		}
		for i, v := range order {
			if v != want[i] {
				t.Errorf("order[%d] = %q, want %q", i, v, want[i])
			}
		}
	})
}

func TestConcurrency(t *testing.T) {
	t.Run("Append is safe from multiple goroutines", func(t *testing.T) {
		m := New()
		done := make(chan struct{})

		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 100; i++ {
				m.Append(Hook{Name: "goroutine-1"})
			}
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 100; i++ {
				m.Append(Hook{Name: "goroutine-2"})
			}
		}()

		<-done
		<-done

		if len(m.hooks) != 200 {
			t.Errorf("expected 200 hooks, got %d", len(m.hooks))
		}
	})
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

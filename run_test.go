package grove

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kusold/grove/config"
	"github.com/kusold/grove/lifecycle"
	"github.com/kusold/grove/tenancy"
)

// testModule is a minimal Module implementation for testing.
type testModule struct {
	name     string
	register func(ctx context.Context, app *App) error
}

func (m testModule) Name() string { return m.name }

func (m testModule) Register(ctx context.Context, app *App) error {
	if m.register != nil {
		return m.register(ctx, app)
	}
	return nil
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SERVICE_NAME",
		"SERVICE_ENV",
		"SERVICE_VERSION",
		"HTTP_ADDR",
		"HTTP_SHUTDOWN_TIMEOUT",
		"DATABASE_URL",
		"DATABASE_MAX_CONNS",
		"DATABASE_MIN_CONNS",
		"DATABASE_CONNECT_TIMEOUT",
		"LOG_FORMAT",
		"LOG_COLOR",
		"GROVE_MIGRATIONS",
	} {
		t.Setenv(key, "")
	}
}

func TestModuleInterface(t *testing.T) {
	t.Run("Name returns the module name", func(t *testing.T) {
		m := testModule{name: "test-service"}
		if got := m.Name(); got != "test-service" {
			t.Errorf("Name() = %q, want %q", got, "test-service")
		}
	})

	t.Run("Register is called with context and app", func(t *testing.T) {
		called := false
		m := testModule{
			name: "test-service",
			register: func(ctx context.Context, app *App) error {
				called = true
				if app == nil {
					t.Error("app should not be nil")
				}
				if ctx == nil {
					t.Error("ctx should not be nil")
				}
				return nil
			},
		}

		err := Run(context.Background(), m)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if !called {
			t.Error("Register was not called")
		}
	})
}

func TestRun(t *testing.T) {
	t.Run("creates app with module name", func(t *testing.T) {
		m := testModule{
			name: "my-service",
			register: func(ctx context.Context, app *App) error {
				if got := app.Name(); got != "my-service" {
					t.Errorf("app.Name() = %q, want %q", got, "my-service")
				}
				return nil
			},
		}

		if err := Run(context.Background(), m); err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("applies options before registration", func(t *testing.T) {
		var capturedName string
		m := testModule{
			name: "original",
			register: func(ctx context.Context, app *App) error {
				capturedName = app.Name()
				return nil
			},
		}

		overrideName := func(b *builder) error {
			b.name = "overridden"
			return nil
		}

		err := Run(context.Background(), m, overrideName)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
		if capturedName != "overridden" {
			t.Errorf("app.Name() = %q, want %q after option applied", capturedName, "overridden")
		}
	})

	t.Run("returns error when registration fails", func(t *testing.T) {
		regErr := errors.New("registration boom")
		m := testModule{
			name: "failing-service",
			register: func(ctx context.Context, app *App) error {
				return regErr
			},
		}

		err := Run(context.Background(), m)
		if err == nil {
			t.Fatal("Run() should return an error when registration fails")
		}
		if !errors.Is(err, regErr) {
			t.Errorf("Run() error = %v, want to wrap %v", err, regErr)
		}
		if !strings.Contains(err.Error(), "failing-service") {
			t.Errorf("Run() error = %q, want to contain module name %q", err.Error(), "failing-service")
		}
		if !strings.Contains(err.Error(), "registration failed") {
			t.Errorf("Run() error = %q, want to contain 'registration failed'", err.Error())
		}
	})

	t.Run("accepts multiple options", func(t *testing.T) {
		m := testModule{name: "multi-opts"}
		opt1 := func(b *builder) error { return nil }
		opt2 := func(b *builder) error { return nil }

		err := Run(context.Background(), m, opt1, opt2)
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	})

	t.Run("returns error when option fails", func(t *testing.T) {
		optErr := errors.New("option boom")
		failOpt := func(b *builder) error { return optErr }

		err := Run(context.Background(), testModule{name: "opt-fail"}, failOpt)
		if err == nil {
			t.Fatal("Run() should return an error when an option fails")
		}
		if !errors.Is(err, optErr) {
			t.Errorf("Run() error = %v, want to wrap %v", err, optErr)
		}
		if !strings.Contains(err.Error(), "option error") {
			t.Errorf("Run() error = %q, want to contain 'option error'", err.Error())
		}
	})
}

func TestNewApp(t *testing.T) {
	t.Run("sets name from argument", func(t *testing.T) {
		app, err := NewApp("test-app")
		if err != nil {
			t.Fatalf("NewApp() returned unexpected error: %v", err)
		}
		if got := app.Name(); got != "test-app" {
			t.Errorf("app.Name() = %q, want %q", got, "test-app")
		}
	})

	t.Run("applies options in order", func(t *testing.T) {
		var order []string
		opt1 := func(b *builder) error { order = append(order, "opt1"); return nil }
		opt2 := func(b *builder) error { order = append(order, "opt2"); return nil }

		_, err := NewApp("test", opt1, opt2)
		if err != nil {
			t.Fatalf("NewApp() returned unexpected error: %v", err)
		}

		if len(order) != 2 || order[0] != "opt1" || order[1] != "opt2" {
			t.Errorf("options applied in wrong order: %v", order)
		}
	})

	t.Run("returns non-nil app with no options", func(t *testing.T) {
		app, err := NewApp("bare")
		if err != nil {
			t.Fatalf("NewApp() returned unexpected error: %v", err)
		}
		if app == nil {
			t.Error("NewApp() returned nil")
		}
	})

	t.Run("stops applying options on first error", func(t *testing.T) {
		var applied []string
		opt1 := func(b *builder) error { applied = append(applied, "opt1"); return nil }
		optErr := errors.New("boom")
		opt2 := func(b *builder) error { return optErr }
		opt3 := func(b *builder) error { applied = append(applied, "opt3"); return nil }

		_, err := NewApp("test", opt1, opt2, opt3)
		if err == nil {
			t.Fatal("NewApp() should return an error")
		}
		if !errors.Is(err, optErr) {
			t.Errorf("NewApp() error = %v, want to wrap %v", err, optErr)
		}
		if len(applied) != 1 || applied[0] != "opt1" {
			t.Errorf("expected only opt1 applied, got %v", applied)
		}
	})
}

func TestApp(t *testing.T) {
	t.Run("Name returns the app name", func(t *testing.T) {
		app, _ := NewApp("svc-name")
		if got := app.Name(); got != "svc-name" {
			t.Errorf("app.Name() = %q, want %q", got, "svc-name")
		}
	})

	t.Run("Config returns non-nil config", func(t *testing.T) {
		app, err := NewApp("test-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg == nil {
			t.Fatal("app.Config() returned nil")
		}
	})

	t.Run("Config uses module name as default service name", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := NewApp("my-service")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg.Service().Name != "my-service" {
			t.Errorf("Config().Service().Name = %q, want %q", cfg.Service().Name, "my-service")
		}
	})

	t.Run("Config applies environment defaults", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := NewApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg.Service().Environment != "development" {
			t.Errorf("Config().Service().Environment = %q, want %q", cfg.Service().Environment, "development")
		}
		if cfg.Service().Version != "dev" {
			t.Errorf("Config().Service().Version = %q, want %q", cfg.Service().Version, "dev")
		}
		if cfg.HTTP().Addr != ":8080" {
			t.Errorf("Config().HTTP().Addr = %q, want %q", cfg.HTTP().Addr, ":8080")
		}
	})

	t.Run("Config SERVICE_NAME overrides module name", func(t *testing.T) {
		t.Setenv("SERVICE_NAME", "overridden")
		app, err := NewApp("module-name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cfg := app.Config()
		if cfg.Service().Name != "overridden" {
			t.Errorf("Config().Service().Name = %q, want %q", cfg.Service().Name, "overridden")
		}
		// Module identity is unchanged
		if app.Name() != "module-name" {
			t.Errorf("app.Name() = %q, want %q — module identity should not change", app.Name(), "module-name")
		}
	})
}

func TestCapabilityOptions(t *testing.T) {
	t.Run("WithHTTP enables http capability", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capHTTP) {
			t.Error("expected http capability to be enabled")
		}
	})

	t.Run("options are idempotent", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capHTTP) {
			t.Error("expected http capability to be enabled")
		}
	})
}

func TestCapabilityDependencyValidation(t *testing.T) {
	t.Run("no capabilities is valid", func(t *testing.T) {
		err := Run(context.Background(), testModule{name: "test"})
		if err != nil {
			t.Fatalf("expected no error with no capabilities, got: %v", err)
		}
	})

	t.Run("http capability without deps is valid", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, testModule{name: "test"}, WithHTTP())
		}()

		// Give the server time to start
		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("missing dependencies fail with helpful errors", func(t *testing.T) {
		capDependency := capability("dependency")
		capDependent := capability("dependent")
		withCapabilityMetadata(t,
			[]capability{capDependency, capDependent},
			map[capability][]capability{capDependent: {capDependency}},
			map[capability]string{
				capDependency: "WithDependency",
				capDependent:  "WithDependent",
			},
			map[capability]string{
				capDependency: "dependency",
				capDependent:  "dependent",
			},
		)

		err := Run(context.Background(), testModule{name: "test"}, func(b *builder) error {
			b.enableCapability(capDependent)
			return nil
		})
		if err == nil {
			t.Fatal("expected dependency validation error")
		}
		if !strings.Contains(err.Error(), "dependent requires dependency") {
			t.Errorf("error = %q, want to contain 'dependent requires dependency'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithDependency()") {
			t.Errorf("error = %q, want to contain 'grove.WithDependency()'", err.Error())
		}
	})

	t.Run("dependencies may be provided after dependents", func(t *testing.T) {
		capDependency := capability("dependency")
		capDependent := capability("dependent")
		withCapabilityMetadata(t,
			[]capability{capDependency, capDependent},
			map[capability][]capability{capDependent: {capDependency}},
			map[capability]string{
				capDependency: "WithDependency",
				capDependent:  "WithDependent",
			},
			map[capability]string{
				capDependency: "dependency",
				capDependent:  "dependent",
			},
		)

		err := Run(context.Background(), testModule{name: "test"},
			func(b *builder) error {
				b.enableCapability(capDependent)
				return nil
			},
			func(b *builder) error {
				b.enableCapability(capDependency)
				return nil
			},
		)
		if err != nil {
			t.Fatalf("expected no error when deps are provided in reverse order, got: %v", err)
		}
	})
}

func TestCapabilityDeterministicOrder(t *testing.T) {
	t.Run("validation error is consistent regardless of option order", func(t *testing.T) {
		capFirst := capability("first")
		capSecond := capability("second")
		capFirstDep := capability("first-dep")
		capSecondDep := capability("second-dep")
		withCapabilityMetadata(t,
			[]capability{capFirstDep, capFirst, capSecondDep, capSecond},
			map[capability][]capability{
				capFirst:  {capFirstDep},
				capSecond: {capSecondDep},
			},
			map[capability]string{
				capFirstDep:  "WithFirstDependency",
				capFirst:     "WithFirst",
				capSecondDep: "WithSecondDependency",
				capSecond:    "WithSecond",
			},
			map[capability]string{
				capFirstDep:  "first dependency",
				capFirst:     "first",
				capSecondDep: "second dependency",
				capSecond:    "second",
			},
		)

		err2 := Run(context.Background(), testModule{name: "test"},
			func(b *builder) error {
				b.enableCapability(capSecond)
				b.enableCapability(capFirst)
				return nil
			},
		)
		err1 := Run(context.Background(), testModule{name: "test"},
			func(b *builder) error {
				b.enableCapability(capFirst)
				b.enableCapability(capSecond)
				return nil
			},
		)

		if err1 == nil || err2 == nil {
			t.Fatal("expected both to fail with dependency errors")
		}

		if err1.Error() != err2.Error() {
			t.Errorf("errors should be identical regardless of option order:\n  err1: %s\n  err2: %s", err1, err2)
		}
		if !strings.Contains(err1.Error(), "first requires first dependency") {
			t.Errorf("expected first error to be about the first capability, got: %s", err1)
		}
	})
}

func TestAppLifecycle(t *testing.T) {
	t.Run("Lifecycle returns non-nil manager", func(t *testing.T) {
		app, err := NewApp("test-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lc := app.Lifecycle()
		if lc == nil {
			t.Fatal("app.Lifecycle() returned nil")
		}
	})

	t.Run("Lifecycle returns same manager on each call", func(t *testing.T) {
		app, err := NewApp("test-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lc1 := app.Lifecycle()
		lc2 := app.Lifecycle()
		if lc1 != lc2 {
			t.Error("app.Lifecycle() should return the same manager each time")
		}
	})

	t.Run("hooks can be registered and run through app lifecycle", func(t *testing.T) {
		var order []string
		m := testModule{
			name: "lifecycle-svc",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
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
				app.Lifecycle().Append(lifecycle.Hook{
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
				return nil
			},
		}

		app, err := NewApp(m.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := m.Register(context.Background(), app); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := app.Lifecycle().Start(context.Background()); err != nil {
			t.Fatalf("Start() error: %v", err)
		}
		if err := app.Lifecycle().Stop(context.Background()); err != nil {
			t.Fatalf("Stop() error: %v", err)
		}

		want := []string{"start-first", "start-second", "stop-second", "stop-first"}
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

func TestAppHTTP(t *testing.T) {
	t.Run("HTTP returns registry when WithHTTP is enabled", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg := app.HTTP()
		if reg == nil {
			t.Fatal("app.HTTP() returned nil")
		}
	})

	t.Run("HTTP returns same registry on each call", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg1 := app.HTTP()
		reg2 := app.HTTP()
		if reg1 != reg2 {
			t.Error("app.HTTP() should return the same registry each time")
		}
	})

	t.Run("HTTP panics when capability not enabled", func(t *testing.T) {
		app, err := NewApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected HTTP() to panic when capability not enabled")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("panic value = %v, want string", r)
			}
			if !strings.Contains(msg, "http capability is required but was not enabled") {
				t.Errorf("panic = %q, want to contain 'http capability is required but was not enabled'", msg)
			}
			if !strings.Contains(msg, "grove.WithHTTP()") {
				t.Errorf("panic = %q, want to contain 'grove.WithHTTP()'", msg)
			}
		}()

		app.HTTP()
	})

	t.Run("registry routes work through app", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		reg := app.HTTP()
		reg.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "ok" {
			t.Errorf("body = %q, want %q", body, "ok")
		}
	})
}

func TestRequireCapability(t *testing.T) {
	t.Run("returns error when capability not enabled", func(t *testing.T) {
		app, err := NewApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = app.requireCapability(capHTTP)
		if err == nil {
			t.Fatal("expected error when http capability not enabled")
		}
		if !strings.Contains(err.Error(), "http capability is required but was not enabled") {
			t.Errorf("error = %q, want to contain 'http capability is required but was not enabled'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("returns nil when capability is enabled", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := app.requireCapability(capHTTP); err != nil {
			t.Errorf("expected nil error when capability is enabled, got: %v", err)
		}
	})
}

func TestAppLogger(t *testing.T) {
	t.Run("Logger returns non-nil slog.Logger", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := NewApp("test-svc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if app.Logger() == nil {
			t.Fatal("app.Logger() returned nil")
		}
	})

	t.Run("Logger defaults to text format", func(t *testing.T) {
		clearConfigEnv(t)

		var buf bytes.Buffer
		cfg := config.Load("dev-svc")
		logger := newLogger("dev-svc", cfg, &buf)
		logger.Info("test message")

		output := buf.String()
		if strings.HasPrefix(output, "{") {
			t.Error("expected text output by default, got JSON")
		}
		if !strings.Contains(output, "service=dev-svc") {
			t.Errorf("output should contain service attribute, got: %s", output)
		}
		if !strings.Contains(output, "environment=development") {
			t.Errorf("output should contain environment attribute, got: %s", output)
		}
		if !strings.Contains(output, "version=dev") {
			t.Errorf("output should contain version attribute, got: %s", output)
		}
	})

	t.Run("Logger uses JSON format when LOG_FORMAT=json", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_FORMAT", "json")
		t.Setenv("SERVICE_VERSION", "v1.0.0")

		var buf bytes.Buffer
		cfg := config.Load("json-svc")
		logger := newLogger("json-svc", cfg, &buf)
		logger.Info("test message")

		var record map[string]any
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("output should be valid JSON: %v, got: %s", err, buf.String())
		}
		if record["service"] != "json-svc" {
			t.Errorf("service = %v, want %q", record["service"], "json-svc")
		}
		if record["environment"] != "development" {
			t.Errorf("environment = %v, want %q", record["environment"], "development")
		}
		if record["version"] != "v1.0.0" {
			t.Errorf("version = %v, want %q", record["version"], "v1.0.0")
		}
	})

	t.Run("Logger service attribute uses module identity when SERVICE_NAME overrides runtime name", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_FORMAT", "json")
		t.Setenv("SERVICE_NAME", "production-canopy")

		var buf bytes.Buffer
		cfg := config.Load("canopy")
		logger := newLogger("canopy", cfg, &buf)
		logger.Info("test message")

		var record map[string]any
		if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
			t.Fatalf("output should be valid JSON: %v, got: %s", err, buf.String())
		}
		if cfg.Service().Name != "production-canopy" {
			t.Fatalf("Config().Service().Name = %q, want %q", cfg.Service().Name, "production-canopy")
		}
		if record["service"] != "canopy" {
			t.Errorf("service = %v, want stable module identity %q", record["service"], "canopy")
		}
	})

	t.Run("Logger uses text format when LOG_FORMAT=text", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_FORMAT", "text")

		var buf bytes.Buffer
		cfg := config.Load("text-svc")
		logger := newLogger("text-svc", cfg, &buf)
		logger.Info("test message")

		output := buf.String()
		if strings.HasPrefix(output, "{") {
			t.Error("expected text output, got JSON")
		}
		if !strings.Contains(output, "service=text-svc") {
			t.Errorf("output should contain service attribute, got: %s", output)
		}
	})

	t.Run("Logger colorizes levels when LOG_COLOR=on with text format", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_COLOR", "on")

		var buf bytes.Buffer
		cfg := config.Load("color-svc")
		logger := newLogger("color-svc", cfg, &buf)
		logger.Info("test message")

		got := buf.String()
		if !strings.Contains(got, "\x1b[32mlevel=INFO\x1b[0m") {
			t.Fatalf("expected green info level, got %q", got)
		}
	})

	t.Run("Logger does not colorize when LOG_COLOR=off", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_COLOR", "off")

		var buf bytes.Buffer
		cfg := config.Load("nocolor-svc")
		logger := newLogger("nocolor-svc", cfg, &buf)
		logger.Info("test message")

		got := buf.String()
		if strings.Contains(got, "\x1b[") {
			t.Fatalf("expected no ANSI escapes, got %q", got)
		}
		if !strings.Contains(got, "level=INFO") {
			t.Fatalf("expected level=INFO, got %q", got)
		}
	})

	t.Run("Logger auto colorize skips non-terminal writers", func(t *testing.T) {
		clearConfigEnv(t)
		// default LOG_COLOR is "auto", buf is not a terminal

		var buf bytes.Buffer
		cfg := config.Load("auto-svc")
		logger := newLogger("auto-svc", cfg, &buf)
		logger.Info("test message")

		got := buf.String()
		if strings.Contains(got, "\x1b[") {
			t.Fatalf("expected no ANSI escapes for non-terminal writer, got %q", got)
		}
	})

	t.Run("Logger does not colorize JSON output even with LOG_COLOR=on", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("LOG_FORMAT", "json")
		t.Setenv("LOG_COLOR", "on")

		var buf bytes.Buffer
		cfg := config.Load("json-color-svc")
		logger := newLogger("json-color-svc", cfg, &buf)
		logger.Info("test message")

		got := buf.String()
		if strings.Contains(got, "\x1b[") {
			t.Fatalf("expected no ANSI escapes in JSON, got %q", got)
		}
	})
}

func TestColorLevels(t *testing.T) {
	t.Run("colorizes all slog levels", func(t *testing.T) {
		tests := []struct {
			level string
			color string
		}{
			{"ERROR", "\x1b[31m"},
			{"WARN", "\x1b[33m"},
			{"INFO", "\x1b[32m"},
			{"DEBUG", "\x1b[34m"},
		}
		for _, tt := range tests {
			t.Run(tt.level, func(t *testing.T) {
				input := []byte("level=" + tt.level + " msg=test\n")
				got := colorLevels(input)
				want := tt.color + "level=" + tt.level + "\x1b[0m msg=test\n"
				if string(got) != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	})

	t.Run("only matches level field, not level in attribute values", func(t *testing.T) {
		input := []byte("level=INFO msg=test note=\"level=INFO\"\n")
		got := colorLevels(input)

		// The actual level= field should be colored
		if !strings.Contains(string(got), "\x1b[32mlevel=INFO\x1b[0m msg=test") {
			t.Errorf("expected level field to be colored, got %q", got)
		}
		// The level= inside the quoted value should NOT be colored
		if strings.Count(string(got), "\x1b[32mlevel=INFO\x1b[0m") != 1 {
			t.Errorf("expected exactly one colored level, got %q", got)
		}
		if !strings.Contains(string(got), `note="level=INFO"`) {
			t.Errorf("expected note value to remain intact, got %q", got)
		}
	})

	t.Run("does not color level text in quoted messages", func(t *testing.T) {
		input := []byte(`level=INFO msg="saw level=INFO token" note="level=WARN"` + "\n")
		got := string(colorLevels(input))

		if strings.Count(got, "\x1b[32mlevel=INFO\x1b[0m") != 1 {
			t.Errorf("expected exactly one colored level field, got %q", got)
		}
		if !strings.Contains(got, `msg="saw level=INFO token"`) {
			t.Errorf("expected message value to remain intact, got %q", got)
		}
		if !strings.Contains(got, `note="level=WARN"`) {
			t.Errorf("expected note value to remain intact, got %q", got)
		}
	})

	t.Run("colors only the first level field per line", func(t *testing.T) {
		input := []byte("level=INFO msg=test level=ERROR\n")
		got := string(colorLevels(input))

		if strings.Count(got, "\x1b[32mlevel=INFO\x1b[0m") != 1 {
			t.Errorf("expected builtin level to be colored once, got %q", got)
		}
		if strings.Contains(got, "\x1b[31mlevel=ERROR\x1b[0m") {
			t.Errorf("expected later level attribute to remain uncolored, got %q", got)
		}
		if !strings.Contains(got, " level=ERROR") {
			t.Errorf("expected later level attribute to remain present, got %q", got)
		}
	})

	t.Run("colors one level field on each line", func(t *testing.T) {
		input := []byte("level=INFO msg=one\nlevel=ERROR msg=two\n")
		got := string(colorLevels(input))

		if !strings.Contains(got, "\x1b[32mlevel=INFO\x1b[0m msg=one") {
			t.Errorf("expected info level to be colored, got %q", got)
		}
		if !strings.Contains(got, "\x1b[31mlevel=ERROR\x1b[0m msg=two") {
			t.Errorf("expected error level to be colored, got %q", got)
		}
	})

	t.Run("colorizes custom slog level names by level family", func(t *testing.T) {
		input := []byte("level=INFO+2 msg=custom\nlevel=ERROR+4 msg=custom\n")
		got := string(colorLevels(input))

		if !strings.Contains(got, "\x1b[32mlevel=INFO+2\x1b[0m msg=custom") {
			t.Errorf("expected custom info level to be colored, got %q", got)
		}
		if !strings.Contains(got, "\x1b[31mlevel=ERROR+4\x1b[0m msg=custom") {
			t.Errorf("expected custom error level to be colored, got %q", got)
		}
	})

	t.Run("returns input unchanged when no level present", func(t *testing.T) {
		input := []byte("msg=test something=else\n")
		got := colorLevels(input)
		if string(got) != string(input) {
			t.Errorf("got %q, want %q", got, input)
		}
	})
}

// --- Graceful HTTP Server Tests ---

func TestRun_HTTPServer(t *testing.T) {
	t.Run("starts HTTP server after module registration", func(t *testing.T) {
		clearConfigEnv(t)
		// Use a specific port for testing
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		registrationComplete := make(chan struct{})
		m := testModule{
			name: "order-test",
			register: func(ctx context.Context, app *App) error {
				close(registrationComplete)
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP())
		}()

		select {
		case <-registrationComplete:
		case err := <-runDone:
			t.Fatalf("Run() completed before module registration signal: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("module registration did not complete")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("lifecycle start hooks run before server", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		hookRan := make(chan struct{})
		m := testModule{
			name: "hook-order-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "test-hook",
					Start: func(ctx context.Context) error {
						close(hookRan)
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP())
		}()

		select {
		case <-hookRan:
		case err := <-runDone:
			t.Fatalf("Run() completed before lifecycle start hook signal: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("lifecycle start hook did not run")
		}

		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}
	})

	t.Run("returns error when lifecycle start hook fails", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")

		hookErr := errors.New("start boom")
		m := testModule{
			name: "hook-fail",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "failing-hook",
					Start: func(ctx context.Context) error {
						return hookErr
					},
				})
				return nil
			},
		}

		err := Run(context.Background(), m, WithHTTP())
		if err == nil {
			t.Fatal("expected error when lifecycle start hook fails")
		}
		if !errors.Is(err, hookErr) {
			t.Errorf("error = %v, want to wrap %v", err, hookErr)
		}
		if !strings.Contains(err.Error(), "lifecycle start") {
			t.Errorf("error = %q, want to contain 'lifecycle start'", err.Error())
		}
	})

	t.Run("stops lifecycle hooks when server fails to start", func(t *testing.T) {
		clearConfigEnv(t)

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to reserve listener: %v", err)
		}
		defer func() { _ = ln.Close() }()

		t.Setenv("HTTP_ADDR", ln.Addr().String())
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		var stopCalled bool
		m := testModule{
			name: "server-start-fail",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "started-resource",
					Start: func(ctx context.Context) error {
						return nil
					},
					Stop: func(ctx context.Context) error {
						stopCalled = true
						return nil
					},
				})
				return nil
			},
		}

		err = Run(context.Background(), m, WithHTTP())
		if err == nil {
			t.Fatal("expected server startup error")
		}
		if !strings.Contains(err.Error(), "http server error") {
			t.Errorf("error = %q, want to contain 'http server error'", err.Error())
		}
		if !stopCalled {
			t.Error("expected lifecycle stop hook to run after server startup failure")
		}
	})
}

func TestRun_GracefulShutdown(t *testing.T) {
	t.Run("SIGINT triggers graceful shutdown", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		started := make(chan struct{})
		var stopOrder []string
		m := testModule{
			name: "sigint-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "module-cleanup",
					Start: func(ctx context.Context) error {
						close(started)
						return nil
					},
					Stop: func(ctx context.Context) error {
						stopOrder = append(stopOrder, "module-cleanup")
						return nil
					},
				})
				return nil
			},
		}

		// Run in a goroutine; send SIGINT to ourselves
		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(context.Background(), m, WithHTTP())
		}()

		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("lifecycle start hook did not run")
		}

		// Send SIGINT to ourselves
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGINT)

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout after SIGINT")
		}

		// Verify lifecycle stop hooks ran
		if len(stopOrder) != 1 || stopOrder[0] != "module-cleanup" {
			t.Errorf("stop order = %v, want [module-cleanup]", stopOrder)
		}
	})

	t.Run("SIGTERM triggers graceful shutdown", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		started := make(chan struct{})
		var stopCalled bool
		m := testModule{
			name: "sigterm-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "module-cleanup",
					Start: func(ctx context.Context) error {
						close(started)
						return nil
					},
					Stop: func(ctx context.Context) error {
						stopCalled = true
						return nil
					},
				})
				return nil
			},
		}

		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(context.Background(), m, WithHTTP())
		}()

		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("lifecycle start hook did not run")
		}

		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGTERM)

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout after SIGTERM")
		}

		if !stopCalled {
			t.Error("expected lifecycle stop hook to run")
		}
	})

	t.Run("lifecycle stop hooks run in reverse order during shutdown", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		var stopOrder []string
		m := testModule{
			name: "reverse-order-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "first",
					Stop: func(ctx context.Context) error {
						stopOrder = append(stopOrder, "first")
						return nil
					},
				})
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "second",
					Stop: func(ctx context.Context) error {
						stopOrder = append(stopOrder, "second")
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP())
		}()

		time.Sleep(200 * time.Millisecond)
		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}

		// http-server stop is registered last, so it should stop first,
		// then "second", then "first"
		want := []string{"second", "first"}
		if len(stopOrder) != len(want) {
			t.Fatalf("stop order = %v, want %v", stopOrder, want)
		}
		for i, v := range stopOrder {
			if v != want[i] {
				t.Errorf("stopOrder[%d] = %q, want %q", i, v, want[i])
			}
		}
	})

	t.Run("parent cancellation does not cancel lifecycle stop context", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("HTTP_ADDR", "127.0.0.1:0")
		t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "5s")

		stopCtxErr := make(chan error, 1)
		m := testModule{
			name: "cancel-stop-context-test",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "inspect-stop-context",
					Stop: func(ctx context.Context) error {
						stopCtxErr <- ctx.Err()
						return nil
					},
				})
				return nil
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		runDone := make(chan error, 1)
		go func() {
			runDone <- Run(ctx, m, WithHTTP())
		}()

		time.Sleep(200 * time.Millisecond)
		cancel()

		select {
		case err := <-runDone:
			if err != nil {
				t.Fatalf("Run() returned unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Run() did not complete within timeout")
		}

		select {
		case err := <-stopCtxErr:
			if err != nil {
				t.Fatalf("stop hook context was canceled: %v", err)
			}
		default:
			t.Fatal("expected stop hook to receive a context")
		}
	})
}

func TestRun_NoHTTP(t *testing.T) {
	t.Run("returns nil immediately without HTTP capability", func(t *testing.T) {
		clearConfigEnv(t)
		m := testModule{name: "no-http"}
		err := Run(context.Background(), m)
		if err != nil {
			t.Fatalf("Run() without HTTP returned unexpected error: %v", err)
		}
	})

	t.Run("does not run lifecycle hooks without HTTP or Postgres capability", func(t *testing.T) {
		clearConfigEnv(t)
		started := false
		stopped := false
		m := testModule{
			name: "no-http-lifecycle",
			register: func(ctx context.Context, app *App) error {
				app.Lifecycle().Append(lifecycle.Hook{
					Name: "should-not-run",
					Start: func(ctx context.Context) error {
						started = true
						return nil
					},
					Stop: func(ctx context.Context) error {
						stopped = true
						return nil
					},
				})
				return nil
			},
		}

		err := Run(context.Background(), m)
		if err != nil {
			t.Fatalf("Run() without HTTP returned unexpected error: %v", err)
		}
		if started {
			t.Fatal("lifecycle start hook ran without HTTP or Postgres capability")
		}
		if stopped {
			t.Fatal("lifecycle stop hook ran without HTTP or Postgres capability")
		}
	})
}

func TestWithTenancy(t *testing.T) {
	t.Run("enables tenancy capability", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capTenancy) {
			t.Error("expected tenancy capability to be enabled")
		}
	})

	t.Run("succeeds when HTTP is also enabled", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy())
		if err != nil {
			t.Fatalf("expected no error when HTTP is enabled, got: %v", err)
		}
		if app == nil {
			t.Fatal("expected non-nil app")
		}
	})

	t.Run("fails when HTTP is not enabled", func(t *testing.T) {
		_, err := NewApp("test", WithTenancy())
		if err == nil {
			t.Fatal("expected error when WithTenancy is used without WithHTTP")
		}
		if !strings.Contains(err.Error(), "tenancy requires http") {
			t.Errorf("error = %q, want to contain 'tenancy requires http'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("fails with clear error via Run", func(t *testing.T) {
		m := testModule{name: "tenancy-no-http"}
		err := Run(context.Background(), m, WithTenancy())
		if err == nil {
			t.Fatal("expected error when WithTenancy is used without WithHTTP")
		}
		if !strings.Contains(err.Error(), "tenancy requires http") {
			t.Errorf("error = %q, want to contain 'tenancy requires http'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithHTTP()") {
			t.Errorf("error = %q, want to contain 'grove.WithHTTP()'", err.Error())
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		app, err := NewApp("test",
			WithHTTP(),
			WithTenancy(),
			WithTenancy(),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capTenancy) {
			t.Error("expected tenancy capability to be enabled")
		}
	})
}

func TestWithTenancy_MiddlewareWired(t *testing.T) {
	t.Run("tenant middleware is wired into HTTP stack", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var capturedID, capturedSlug string
		app.HTTP().Get("/test-tenant", func(w http.ResponseWriter, r *http.Request) {
			tenant, ok := tenancy.FromContext(r.Context())
			if ok {
				capturedID = tenant.ID
				capturedSlug = tenant.Slug
			}
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test-tenant", nil)
		req.Header.Set("X-Tenant-ID", "t-123")
		req.Header.Set("X-Tenant-Slug", "acme")
		rec := httptest.NewRecorder()

		app.HTTP().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if capturedID != "t-123" {
			t.Errorf("tenant ID = %q, want %q", capturedID, "t-123")
		}
		if capturedSlug != "acme" {
			t.Errorf("tenant slug = %q, want %q", capturedSlug, "acme")
		}
	})

	t.Run("tenant middleware works with RequireMiddleware on route groups", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		app.HTTP().Route("/api", func(r chi.Router) {
			r.Use(tenancy.RequireMiddleware())
			r.Get("/whoami", func(w http.ResponseWriter, r *http.Request) {
				tenant, _ := tenancy.FromContext(r.Context())
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]string{
					"tenant_id":   tenant.ID,
					"tenant_slug": tenant.Slug,
				}); err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
			})
		})

		t.Run("allows request with tenant headers", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
			req.Header.Set("X-Tenant-ID", "t-456")
			req.Header.Set("X-Tenant-Slug", "beta")
			rec := httptest.NewRecorder()

			app.HTTP().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["tenant_id"] != "t-456" {
				t.Errorf("tenant_id = %q, want %q", body["tenant_id"], "t-456")
			}
			if body["tenant_slug"] != "beta" {
				t.Errorf("tenant_slug = %q, want %q", body["tenant_slug"], "beta")
			}
		})

		t.Run("rejects request without tenant headers with 422", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/whoami", nil)
			rec := httptest.NewRecorder()

			app.HTTP().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}

			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			errObj, _ := body["error"].(map[string]any)
			if errObj["code"] != "tenant_required" {
				t.Errorf("code = %v, want %q", errObj["code"], "tenant_required")
			}
		})
	})

	t.Run("non-tenant routes still work", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		app.HTTP().Get("/public", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "public")
		})

		req := httptest.NewRequest(http.MethodGet, "/public", nil)
		rec := httptest.NewRecorder()
		app.HTTP().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Body.String() != "public" {
			t.Errorf("body = %q, want %q", rec.Body.String(), "public")
		}
	})

	t.Run("tenant middleware is not wired when tenancy is not enabled", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		app.HTTP().Get("/check", func(w http.ResponseWriter, r *http.Request) {
			_, ok := tenancy.FromContext(r.Context())
			if ok {
				t.Error("did not expect tenant in context when tenancy is not enabled")
			}
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/check", nil)
		req.Header.Set("X-Tenant-ID", "t-999")
		req.Header.Set("X-Tenant-Slug", "sneaky")
		rec := httptest.NewRecorder()
		app.HTTP().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

func TestWithPostgres(t *testing.T) {
	t.Run("enables postgres capability", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capPostgres) {
			t.Error("expected postgres capability to be enabled")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres(), WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capPostgres) {
			t.Error("expected postgres capability to be enabled")
		}
	})

	t.Run("RequireDB returns error when not enabled", func(t *testing.T) {
		app, err := NewApp("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		database, err := app.RequireDB()
		if err == nil {
			t.Fatal("expected error when RequireDB called without WithPostgres")
		}
		if database != nil {
			t.Error("expected nil database when capability not enabled")
		}
		if !strings.Contains(err.Error(), "postgres capability is required but was not enabled") {
			t.Errorf("error = %q, want to contain 'postgres capability is required but was not enabled'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithPostgres()") {
			t.Errorf("error = %q, want to contain 'grove.WithPostgres()'", err.Error())
		}
	})

	t.Run("RequireDB returns database when enabled", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		database, err := app.RequireDB()
		if err != nil {
			t.Fatalf("unexpected error from RequireDB: %v", err)
		}
		if database == nil {
			t.Fatal("expected non-nil database")
		}
	})

	t.Run("RequireDB returns same database on each call", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		db1, err := app.RequireDB()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		db2, err := app.RequireDB()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if db1 != db2 {
			t.Error("RequireDB should return the same database each time")
		}
	})

	t.Run("works alongside HTTP and tenancy", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy(), WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capPostgres) {
			t.Error("expected postgres capability to be enabled")
		}
		database, err := app.RequireDB()
		if err != nil {
			t.Fatalf("unexpected error from RequireDB: %v", err)
		}
		if database == nil {
			t.Fatal("expected non-nil database")
		}
	})

	t.Run("registers Postgres readiness but not liveness", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wirePostgresLifecycle(app)

		if err := app.Health().IsHealthy(context.Background()); err != nil {
			t.Fatalf("health should not depend on Postgres: %v", err)
		}
		if err := app.Health().IsReady(context.Background()); err == nil {
			t.Fatal("readiness should fail before Postgres connects")
		}
	})
}

func TestWithMigrations(t *testing.T) {
	t.Run("enables migrations capability", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capMigrations) {
			t.Error("expected migrations capability to be enabled")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres(), WithMigrations(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capMigrations) {
			t.Error("expected migrations capability to be enabled")
		}
	})

	t.Run("fails when Postgres is not enabled", func(t *testing.T) {
		_, err := NewApp("test", WithMigrations())
		if err == nil {
			t.Fatal("expected error when WithMigrations is used without WithPostgres")
		}
		if !strings.Contains(err.Error(), "migrations requires postgres") {
			t.Errorf("error = %q, want to contain 'migrations requires postgres'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithPostgres()") {
			t.Errorf("error = %q, want to contain 'grove.WithPostgres()'", err.Error())
		}
	})

	t.Run("fails with clear error via Run", func(t *testing.T) {
		m := testModule{name: "migrations-no-pg"}
		err := Run(context.Background(), m, WithMigrations())
		if err == nil {
			t.Fatal("expected error when WithMigrations is used without WithPostgres")
		}
		if !strings.Contains(err.Error(), "migrations requires postgres") {
			t.Errorf("error = %q, want to contain 'migrations requires postgres'", err.Error())
		}
	})

	t.Run("RequireMigrations returns error when not enabled", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg, err := app.RequireMigrations()
		if err == nil {
			t.Fatal("expected error when RequireMigrations called without WithMigrations")
		}
		if reg != nil {
			t.Error("expected nil registry when capability not enabled")
		}
		if !strings.Contains(err.Error(), "migrations capability is required but was not enabled") {
			t.Errorf("error = %q, want to contain 'migrations capability is required but was not enabled'", err.Error())
		}
		if !strings.Contains(err.Error(), "grove.WithMigrations()") {
			t.Errorf("error = %q, want to contain 'grove.WithMigrations()'", err.Error())
		}
	})

	t.Run("RequireMigrations returns registry when enabled", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg, err := app.RequireMigrations()
		if err != nil {
			t.Fatalf("unexpected error from RequireMigrations: %v", err)
		}
		if reg == nil {
			t.Fatal("expected non-nil registry")
		}
	})

	t.Run("RequireMigrations returns same registry on each call", func(t *testing.T) {
		app, err := NewApp("test", WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg1, err := app.RequireMigrations()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		reg2, err := app.RequireMigrations()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if reg1 != reg2 {
			t.Error("RequireMigrations should return the same registry each time")
		}
	})

	t.Run("works alongside HTTP, tenancy, and Postgres", func(t *testing.T) {
		app, err := NewApp("test", WithHTTP(), WithTenancy(), WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !app.hasCapability(capMigrations) {
			t.Error("expected migrations capability to be enabled")
		}
		reg, err := app.RequireMigrations()
		if err != nil {
			t.Fatalf("unexpected error from RequireMigrations: %v", err)
		}
		if reg == nil {
			t.Fatal("expected non-nil registry")
		}
	})

	t.Run("dependency order does not matter", func(t *testing.T) {
		app, err := NewApp("test", WithMigrations(), WithPostgres())
		if err != nil {
			t.Fatalf("expected no error when Postgres provided after migrations, got: %v", err)
		}
		if !app.hasCapability(capMigrations) {
			t.Error("expected migrations capability to be enabled")
		}
	})
}

func TestWireMigrationsLifecycle(t *testing.T) {
	t.Run("off mode does not register lifecycle hooks", func(t *testing.T) {
		clearConfigEnv(t)
		t.Setenv("GROVE_MIGRATIONS", "off")
		app, err := NewApp("test", WithHTTP(), WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wirePostgresLifecycle(app)
		wireMigrationsLifecycle(app)

		// Health should only have postgres readiness (no migrations check)
		ctx := context.Background()
		if err := app.Health().IsHealthy(ctx); err != nil {
			t.Fatalf("health should not depend on migrations: %v", err)
		}
		// Readiness will fail because postgres isn't connected, but migrations
		// readiness check should not be registered in off mode
		err = app.Health().IsReady(ctx)
		if err == nil {
			t.Fatal("expected readiness to fail before Postgres connects")
		}
		// The error should only mention postgres, not migrations
		if strings.Contains(err.Error(), "migrations") {
			t.Errorf("readiness error should not mention migrations in off mode: %v", err)
		}
	})

	t.Run("default mode is off", func(t *testing.T) {
		clearConfigEnv(t)
		app, err := NewApp("test", WithHTTP(), WithPostgres(), WithMigrations())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wirePostgresLifecycle(app)
		wireMigrationsLifecycle(app)

		err = app.Health().IsReady(context.Background())
		if err == nil {
			t.Fatal("expected readiness to fail before Postgres connects")
		}
		if strings.Contains(err.Error(), "migrations") {
			t.Errorf("readiness error should not mention migrations in default mode: %v", err)
		}
	})
}

func withCapabilityMetadata(
	t *testing.T,
	order []capability,
	deps map[capability][]capability,
	optionNames map[capability]string,
	displayNames map[capability]string,
) {
	t.Helper()

	oldOrder := capabilityOrder
	oldDeps := capabilityDeps
	oldOptionNames := capabilityOptionName
	oldDisplayNames := capabilityDisplayName

	capabilityOrder = order
	capabilityDeps = deps
	capabilityOptionName = optionNames
	capabilityDisplayName = displayNames

	t.Cleanup(func() {
		capabilityOrder = oldOrder
		capabilityDeps = oldDeps
		capabilityOptionName = oldOptionNames
		capabilityDisplayName = oldDisplayNames
	})
}

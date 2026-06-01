package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kusold/grove/config"
	"github.com/kusold/grove/health"
	"github.com/kusold/grove/lifecycle"
)

func TestNewServer(t *testing.T) {
	t.Run("wires healthz and readyz routes", func(t *testing.T) {
		server := newTestServer(t, New(), config.HTTPConfig{
			Addr:            "127.0.0.1:0",
			ShutdownTimeout: "5s",
		})

		for _, path := range []string{"/healthz", "/readyz"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
			}
		}
	})

	t.Run("serves custom routes through registry", func(t *testing.T) {
		reg := New()
		reg.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"message":"hello"}`)
		})

		server := newTestServer(t, reg, config.HTTPConfig{
			Addr:            "127.0.0.1:0",
			ShutdownTimeout: "5s",
		})

		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if body["message"] != "hello" {
			t.Errorf("message = %q, want %q", body["message"], "hello")
		}
	})

	t.Run("returns error for invalid shutdown timeout", func(t *testing.T) {
		_, err := NewServer(ServerOptions{
			Registry: New(),
			Health:   health.New(),
			Config: config.HTTPConfig{
				Addr:            "127.0.0.1:0",
				ShutdownTimeout: "not-a-duration",
			},
			Logger: discardLogger(),
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid HTTP_SHUTDOWN_TIMEOUT") {
			t.Errorf("error = %q, want to contain 'invalid HTTP_SHUTDOWN_TIMEOUT'", err.Error())
		}
	})
}

func TestServerRun(t *testing.T) {
	t.Run("reports startup errors", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("failed to reserve listener: %v", err)
		}
		defer func() { _ = ln.Close() }()

		server := newTestServer(t, New(), config.HTTPConfig{
			Addr:            ln.Addr().String(),
			ShutdownTimeout: "5s",
		})

		select {
		case err := <-server.Run():
			if err == nil {
				t.Fatal("expected startup error")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server did not report startup error")
		}
	})

	t.Run("shutdown closes run channel without error", func(t *testing.T) {
		addr := unusedTCPAddr(t)
		server := newTestServer(t, New(), config.HTTPConfig{
			Addr:            addr,
			ShutdownTimeout: "5s",
		})
		lc := lifecycle.New()
		server.RegisterLifecycle(lc)

		serverErr := server.Run()
		waitForHTTP(t, addr, "/healthz")

		if err := lc.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() error: %v", err)
		}

		select {
		case err := <-serverErr:
			if err != nil {
				t.Fatalf("Run() returned unexpected error after shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server did not stop")
		}
	})

	t.Run("shutdown hook respects timeout", func(t *testing.T) {
		addr := unusedTCPAddr(t)
		reg := New()
		handlerStarted := make(chan struct{})
		handlerDone := make(chan struct{})
		reg.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
			close(handlerStarted)
			defer close(handlerDone)
			<-r.Context().Done()
		})

		server := newTestServer(t, reg, config.HTTPConfig{
			Addr:            addr,
			ShutdownTimeout: "10ms",
		})
		lc := lifecycle.New()
		server.RegisterLifecycle(lc)

		serverErr := server.Run()
		waitForHTTP(t, addr, "/healthz")

		requestStarted := make(chan struct{})
		requestDone := make(chan struct{})
		go func() {
			close(requestStarted)
			resp, err := http.Get("http://" + addr + "/slow")
			if err == nil {
				_ = resp.Body.Close()
			}
			close(requestDone)
		}()
		<-requestStarted
		select {
		case <-handlerStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("slow handler did not start")
		}

		err := lc.Stop(context.Background())
		if err == nil {
			t.Fatal("expected shutdown timeout error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop() error = %v, want to wrap context deadline exceeded", err)
		}

		select {
		case <-serverErr:
		case <-time.After(5 * time.Second):
			t.Fatal("server did not stop after shutdown timeout")
		}
		select {
		case <-handlerDone:
		case <-time.After(5 * time.Second):
			t.Fatal("slow handler was not canceled after shutdown timeout")
		}
		select {
		case <-requestDone:
		case <-time.After(5 * time.Second):
			t.Fatal("slow request did not complete")
		}
	})
}

func newTestServer(t *testing.T, reg *Registry, cfg config.HTTPConfig) *Server {
	t.Helper()
	server, err := NewServer(ServerOptions{
		Registry: reg,
		Health:   health.New(),
		Config:   cfg,
		Logger:   discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}
	return server
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func unusedTCPAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate test listener: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release test listener: %v", err)
	}
	return addr
}

func waitForHTTP(t *testing.T, addr, path string) {
	t.Helper()

	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + path)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server did not serve %s: %v", path, lastErr)
}

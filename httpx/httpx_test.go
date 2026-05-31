package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestNew(t *testing.T) {
	t.Run("returns non-nil registry", func(t *testing.T) {
		reg := New()
		if reg == nil {
			t.Fatal("New() returned nil")
		}
	})

	t.Run("exposes underlying chi router", func(t *testing.T) {
		reg := New()
		r := reg.Router()
		if r == nil {
			t.Fatal("Router() returned nil")
		}
	})
}

func TestRegistryGet(t *testing.T) {
	t.Run("registers and handles GET route", func(t *testing.T) {
		reg := New()
		reg.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "hello")
		})

		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "hello" {
			t.Errorf("body = %q, want %q", body, "hello")
		}
	})

	t.Run("returns 405 for wrong method", func(t *testing.T) {
		reg := New()
		reg.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodPost, "/hello", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})
}

func TestRegistryPost(t *testing.T) {
	t.Run("registers and handles POST route", func(t *testing.T) {
		reg := New()
		reg.Post("/items", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, "created")
		})

		req := httptest.NewRequest(http.MethodPost, "/items", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		if body := rec.Body.String(); body != "created" {
			t.Errorf("body = %q, want %q", body, "created")
		}
	})
}

func TestRegistryRoute(t *testing.T) {
	t.Run("creates route group", func(t *testing.T) {
		reg := New()
		reg.Route("/api", func(r chi.Router) {
			r.Get("/users", func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, "users")
			})
		})

		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "users" {
			t.Errorf("body = %q, want %q", body, "users")
		}
	})
}

func TestRegistryMount(t *testing.T) {
	t.Run("mounts a sub-handler", func(t *testing.T) {
		reg := New()
		sub := chi.NewRouter()
		sub.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "pong")
		})
		reg.Mount("/sub", sub)

		req := httptest.NewRequest(http.MethodGet, "/sub/ping", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "pong" {
			t.Errorf("body = %q, want %q", body, "pong")
		}
	})
}

func TestRegistryUse(t *testing.T) {
	t.Run("applies middleware to routes", func(t *testing.T) {
		reg := New()

		var middlewareCalled bool
		reg.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				middlewareCalled = true
				next.ServeHTTP(w, r)
			})
		})

		reg.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if !middlewareCalled {
			t.Error("middleware was not called")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("applies multiple middleware in order", func(t *testing.T) {
		reg := New()

		var order []string
		reg.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "first")
				next.ServeHTTP(w, r)
			})
		})
		reg.Use(func(next http.Handler) http.Handler {
			return http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "second")
				next.ServeHTTP(w, r)
			}))
		})

		reg.Get("/test", func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		want := []string{"first", "second", "handler"}
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

func TestRegistryServeHTTP(t *testing.T) {
	t.Run("implements http.Handler", func(t *testing.T) {
		reg := New()
		reg.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "pong")
		})

		var _ http.Handler = reg

		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

func TestRegistryChiInterop(t *testing.T) {
	t.Run("routes registered via Router() are served", func(t *testing.T) {
		reg := New()
		reg.Router().Get("/direct", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "direct")
		})

		req := httptest.NewRequest(http.MethodGet, "/direct", nil)
		rec := httptest.NewRecorder()
		reg.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != "direct" {
			t.Errorf("body = %q, want %q", body, "direct")
		}
	})
}

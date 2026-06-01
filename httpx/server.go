package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/kusold/grove/config"
	"github.com/kusold/grove/health"
	"github.com/kusold/grove/lifecycle"
)

// ServerOptions are the explicit Grove dependencies needed to run the HTTP
// transport. The server is a runtime helper; service routes should still be
// registered through Registry.
type ServerOptions struct {
	Registry *Registry
	Health   *health.Registry
	Config   config.HTTPConfig
	Logger   *slog.Logger
}

// Server owns HTTP transport startup and graceful shutdown for a Grove service.
// It deliberately does not own capability ordering or middleware installation;
// those remain Grove runtime responsibilities.
type Server struct {
	server          *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

// NewServer wires Grove framework routes and constructs an HTTP transport.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Registry == nil {
		return nil, errors.New("httpx: registry is required")
	}
	if opts.Health == nil {
		return nil, errors.New("httpx: health registry is required")
	}

	shutdownTimeout, err := time.ParseDuration(opts.Config.ShutdownTimeout)
	if err != nil {
		return nil, fmt.Errorf("invalid HTTP_SHUTDOWN_TIMEOUT %q: %w", opts.Config.ShutdownTimeout, err)
	}

	opts.Registry.Get("/healthz", opts.Health.HealthzHandler())
	opts.Registry.Get("/readyz", opts.Health.ReadyzHandler())

	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &Server{
		server: &http.Server{
			Addr:    opts.Config.Addr,
			Handler: opts.Registry,
		},
		shutdownTimeout: shutdownTimeout,
		logger:          logger,
	}, nil
}

// RegisterLifecycle adds HTTP shutdown as a lifecycle stop hook. Because hooks
// stop in reverse order, callers should register this after service/module hooks
// so the HTTP server drains before dependent resources are torn down.
func (s *Server) RegisterLifecycle(lc *lifecycle.Manager) {
	lc.Append(lifecycle.Hook{
		Name: "http-server",
		Stop: func(ctx context.Context) error {
			shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
			defer cancel()
			if err := s.server.Shutdown(shutdownCtx); err != nil {
				if closeErr := s.server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
					return errors.Join(
						fmt.Errorf("http server shutdown: %w", err),
						fmt.Errorf("http server close: %w", closeErr),
					)
				}
				return fmt.Errorf("http server shutdown: %w", err)
			}
			return nil
		},
	})
}

// Run starts the HTTP server in a goroutine and returns a channel that receives
// unexpected server errors. A graceful shutdown closes the channel without an
// error.
func (s *Server) Run() <-chan error {
	serverErr := make(chan error, 1)
	go func() {
		s.logger.Info("http server starting", "addr", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()
	return serverErr
}

// Handler returns the HTTP handler configured for this server. Test harnesses
// can use it to exercise the same route stack without binding a network port.
func (s *Server) Handler() http.Handler {
	return s.server.Handler
}

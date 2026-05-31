// Package httpx provides the HTTP registry for Grove services, backed by chi.
// The registry wraps a chi.Router and exposes convenience methods for common
// routing operations while also allowing direct chi access through Router()
// for more advanced use cases.
package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Registry wraps a chi.Router and provides convenience methods for registering
// HTTP routes and middleware. It is the primary HTTP entry point for Grove
// services.
//
// The Router() method intentionally exposes the underlying chi.Router so that
// services can use chi directly when the convenience methods are insufficient.
type Registry struct {
	router chi.Router
}

// New creates a new Registry backed by a fresh chi.Router.
func New() *Registry {
	return &Registry{
		router: chi.NewRouter(),
	}
}

// Router returns the underlying chi.Router. This allows services to use chi
// directly for operations not covered by the convenience methods, such as
// route groups, inline middleware, or pattern matching features.
func (r *Registry) Router() chi.Router {
	return r.router
}

// Route creates a sub-router along the given pattern and calls fn with the
// sub-router. This is the primary way to organize routes into groups.
func (r *Registry) Route(pattern string, fn func(r chi.Router)) {
	r.router.Route(pattern, fn)
}

// Mount attaches another http.Handler at the given pattern. This is useful
// for mounting sub-applications or composing multiple routers.
func (r *Registry) Mount(pattern string, h http.Handler) {
	r.router.Mount(pattern, h)
}

// Use appends middleware to the router. Middleware are applied to all routes
// registered on this router.
func (r *Registry) Use(middlewares ...func(http.Handler) http.Handler) {
	r.router.Use(middlewares...)
}

// Get registers a handler for GET requests at the given pattern.
func (r *Registry) Get(pattern string, h http.HandlerFunc) {
	r.router.Get(pattern, h)
}

// Post registers a handler for POST requests at the given pattern.
func (r *Registry) Post(pattern string, h http.HandlerFunc) {
	r.router.Post(pattern, h)
}

// ServeHTTP makes the registry an http.Handler by delegating to the underlying
// chi.Router.
func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.router.ServeHTTP(w, req)
}

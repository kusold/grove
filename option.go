package grove

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/kusold/grove/config"
	"github.com/kusold/grove/db"
	"github.com/kusold/grove/health"
	"github.com/kusold/grove/httpx"
	"github.com/kusold/grove/lifecycle"
	"github.com/kusold/grove/migrate"
	"github.com/kusold/grove/tenancy"
)

// Option configures a Grove app during construction. Options are applied in
// order before module registration runs. An Option may return an error if its
// configuration is invalid.
type Option func(*builder) error

type builder struct {
	name            string
	capabilities    map[capability]bool
	tenancyResolver tenancy.Resolver
}

// capability represents a named capability that can be enabled on an App.
type capability string

const (
	capHTTP       capability = "http"
	capTenancy    capability = "tenancy"
	capPostgres   capability = "postgres"
	capMigrations capability = "migrations"
)

// capabilityDeps maps each capability to its required dependencies.
// A capability cannot be enabled unless all of its dependencies are also
// enabled. Dependencies are validated after all options are applied, so the
// order of options does not matter.
var capabilityDeps = map[capability][]capability{
	capTenancy:    {capHTTP},
	capMigrations: {capPostgres},
}

// capabilityOrder defines the deterministic initialization order for
// capabilities. Capabilities are initialized in this order regardless of the
// order in which options are passed to Main or Run.
var capabilityOrder = []capability{
	capHTTP,
	capTenancy,
	capPostgres,
	capMigrations,
}

// capabilityOptionName maps each capability to the Option function name used
// in error messages to guide users toward the fix.
var capabilityOptionName = map[capability]string{
	capHTTP:       "WithHTTP",
	capTenancy:    "WithTenancy",
	capPostgres:   "WithPostgres",
	capMigrations: "WithMigrations",
}

// capabilityDisplayName maps each capability to a human-readable name used in
// error messages.
var capabilityDisplayName = map[capability]string{
	capHTTP:       "http",
	capTenancy:    "tenancy",
	capPostgres:   "postgres",
	capMigrations: "migrations",
}

func newBuilder(name string) *builder {
	return &builder{name: name}
}

func (b *builder) applyOptions(opts ...Option) error {
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return err
		}
	}
	return nil
}

// enableCapability records that a capability has been enabled.
func (b *builder) enableCapability(c capability) {
	if b.capabilities == nil {
		b.capabilities = make(map[capability]bool)
	}
	b.capabilities[c] = true
}

// hasCapability reports whether a capability is enabled.
func (b *builder) hasCapability(c capability) bool {
	return b.capabilities[c]
}

// validateCapabilities checks that all enabled capabilities have their
// dependencies satisfied. It returns an error describing the first missing
// dependency found, iterating in deterministic capability order so that error
// messages are consistent regardless of option registration order.
func (b *builder) validateCapabilities() error {
	for _, c := range capabilityOrder {
		if !b.hasCapability(c) {
			continue
		}
		deps, hasDeps := capabilityDeps[c]
		if !hasDeps {
			continue
		}
		for _, dep := range deps {
			if !b.hasCapability(dep) {
				return fmt.Errorf(
					"grove: %s requires %s, but it was not enabled; add grove.%s()",
					capabilityDisplayName[c],
					capabilityDisplayName[dep],
					capabilityOptionName[dep],
				)
			}
		}
	}
	return nil
}

func (b *builder) buildApp() *App {
	cfg := config.Load(b.name)

	var httpReg *httpx.Registry
	if b.hasCapability(capHTTP) {
		httpReg = httpx.New()
	}

	// Wire tenant resolution middleware into the HTTP stack when the tenancy
	// capability is enabled. This runs for every request so that tenant context
	// is available to all handlers. Individual route groups can use
	// tenancy.RequireMiddleware() to reject requests without a tenant.
	if b.hasCapability(capTenancy) && httpReg != nil {
		httpReg.Use(tenancy.Middleware(b.tenancyResolver))
	}

	var database *db.Database
	if b.hasCapability(capPostgres) {
		database = &db.Database{}
	}

	var migrateReg *migrate.Registry
	if b.hasCapability(capMigrations) {
		migrateReg = migrate.NewRegistry()
	}

	return &App{
		name:         b.name,
		capabilities: b.capabilitySet(),
		cfg:          cfg,
		logger:       newLogger(b.name, cfg, os.Stdout),
		lifecycle:    lifecycle.New(),
		healthReg:    health.New(),
		httpReg:      httpReg,
		db:           database,
		migrateReg:   migrateReg,
	}
}

// newLogger creates a slog.Logger configured with structured attributes for
// service identity. serviceName is the stable module identity; runtime service
// naming overrides remain available through config.Service().Name.
//
// The handler format is determined by cfg.Logger().Format. When using text
// format, ANSI color codes are applied to level tags based on cfg.Logger().Color.
func newLogger(serviceName string, cfg config.Provider, w io.Writer) *slog.Logger {
	svc := cfg.Service()
	attrs := []slog.Attr{
		slog.String("service", serviceName),
		slog.String("environment", svc.Environment),
		slog.String("version", svc.Version),
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{AddSource: false}
	logCfg := cfg.Logger()

	switch logCfg.Format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		if shouldColorize(logCfg.Color, w) {
			w = &colorLevelWriter{writer: w}
		}
		handler = slog.NewTextHandler(w, opts)
	}

	handler = handler.WithAttrs(attrs)
	return slog.New(handler)
}

// shouldColorize determines whether log output should be colorized based on
// the config value and whether the writer is a terminal.
//   - "on": always colorize
//   - "off": never colorize
//   - "auto" (default): colorize only when the writer is a character device
func shouldColorize(colorCfg string, w io.Writer) bool {
	switch colorCfg {
	case "on":
		return true
	case "off":
		return false
	default: // "auto"
		f, ok := w.(*os.File)
		if !ok {
			return false
		}
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}
}

const colorReset = "\x1b[0m"

// levelColorRules maps slog levels to ANSI color codes.
var levelColorRules = []struct {
	level []byte
	color string
}{
	{level: []byte("level=" + slog.LevelError.String()), color: "\x1b[31m"},
	{level: []byte("level=" + slog.LevelWarn.String()), color: "\x1b[33m"},
	{level: []byte("level=" + slog.LevelInfo.String()), color: "\x1b[32m"},
	{level: []byte("level=" + slog.LevelDebug.String()), color: "\x1b[34m"},
}

// colorLevelWriter wraps an io.Writer and injects ANSI color codes around
// slog level fields in text output. It only colors the actual level= token,
// not occurrences that appear inside quoted attribute values.
type colorLevelWriter struct {
	writer io.Writer
}

func (w *colorLevelWriter) Write(p []byte) (int, error) {
	colored := colorLevels(p)
	_, err := w.writer.Write(colored)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func colorLevels(p []byte) []byte {
	var out []byte
	last := 0

	for lineStart := 0; lineStart < len(p); {
		lineEnd := bytes.IndexByte(p[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(p)
		} else {
			lineEnd += lineStart
		}

		line, changed := colorLineLevel(p[lineStart:lineEnd])
		if changed {
			if out == nil {
				out = make([]byte, 0, len(p)+len(colorReset)+5)
			}
			out = append(out, p[last:lineStart]...)
			out = append(out, line...)
			last = lineEnd
		}

		if lineEnd == len(p) {
			break
		}
		lineStart = lineEnd + 1
	}

	if out == nil {
		return p
	}
	out = append(out, p[last:]...)
	return out
}

func colorLineLevel(line []byte) ([]byte, bool) {
	for tokenStart := 0; tokenStart < len(line); {
		for tokenStart < len(line) && line[tokenStart] == ' ' {
			tokenStart++
		}
		tokenEnd := findTokenEnd(line, tokenStart)
		token := line[tokenStart:tokenEnd]
		if level, ok := bytes.CutPrefix(token, []byte("level=")); ok {
			color := colorForLevel(level)
			if color == "" {
				return line, false
			}

			out := make([]byte, 0, len(line)+len(color)+len(colorReset))
			out = append(out, line[:tokenStart]...)
			out = append(out, color...)
			out = append(out, token...)
			out = append(out, colorReset...)
			out = append(out, line[tokenEnd:]...)
			return out, true
		}
		tokenStart = tokenEnd
	}
	return line, false
}

func findTokenEnd(line []byte, tokenStart int) int {
	inQuote := false
	escaped := false
	for i := tokenStart; i < len(line); i++ {
		switch c := line[i]; {
		case escaped:
			escaped = false
		case c == '\\':
			escaped = inQuote
		case c == '"':
			inQuote = !inQuote
		case c == ' ' && !inQuote:
			return i
		}
	}
	return len(line)
}

func colorForLevel(level []byte) string {
	for _, rule := range levelColorRules {
		levelName := bytes.TrimPrefix(rule.level, []byte("level="))
		if bytes.Equal(level, levelName) {
			return rule.color
		}
		if bytes.HasPrefix(level, levelName) && isLevelDelta(level[len(levelName):]) {
			return rule.color
		}
	}
	return ""
}

func isLevelDelta(suffix []byte) bool {
	if len(suffix) <= 1 || (suffix[0] != '+' && suffix[0] != '-') {
		return false
	}
	for _, c := range suffix[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (b *builder) capabilitySet() map[capability]bool {
	if len(b.capabilities) == 0 {
		return nil
	}
	caps := make(map[capability]bool, len(b.capabilities))
	for _, c := range capabilityOrder {
		if b.hasCapability(c) {
			caps[c] = true
		}
	}
	return caps
}

// WithHTTP enables the HTTP capability, backed by chi.
func WithHTTP() Option {
	return func(b *builder) error {
		b.enableCapability(capHTTP)
		return nil
	}
}

// WithTenancy enables the tenancy capability and wires tenant resolution
// middleware into the HTTP stack. It uses tenancy.HeaderResolver, which extracts
// tenant identity from X-Tenant-ID and X-Tenant-Slug headers for local/demo use.
// When enabled, tenancy.Middleware is applied globally so that tenant context
// is available to all handlers.
//
// Tenancy requires the HTTP capability. If WithHTTP() is not also provided,
// Grove will fail at startup with a clear error:
//
//	grove: tenancy requires http, but it was not enabled; add grove.WithHTTP()
func WithTenancy() Option {
	return func(b *builder) error {
		b.enableCapability(capTenancy)
		b.tenancyResolver = tenancy.HeaderResolver{}
		return nil
	}
}

// WithPostgres enables the Postgres capability, backed by pgxpool. When
// enabled, Grove connects to Postgres during startup and closes the pool
// during shutdown. The database is accessible via app.RequireDB().
//
// Connection settings are loaded from environment variables:
//
//	DATABASE_URL            — Postgres application connection URL (required)
//	DATABASE_ADMIN_URL      — privileged URL for SystemTx (optional until used)
//	DATABASE_MAX_CONNS      — maximum pool connections (default: 10)
//	DATABASE_MIN_CONNS      — minimum pool connections (default: 0)
//	DATABASE_CONNECT_TIMEOUT — connection timeout (default: 5s)
func WithPostgres() Option {
	return func(b *builder) error {
		b.enableCapability(capPostgres)
		return nil
	}
}

// WithMigrations enables the migration capability, backed by goose. When
// enabled, Grove creates a migration registry that services can register
// migration sources against. The migration behavior at startup is controlled by
// the GROVE_MIGRATIONS environment variable:
//
//	GROVE_MIGRATIONS=off       — skip migrations at startup (default)
//	GROVE_MIGRATIONS=validate  — verify migrations are current; fail startup if not
//	GROVE_MIGRATIONS=up        — run pending migrations automatically during startup
//
// Migrations require the Postgres capability. If WithPostgres() is not also
// provided, Grove will fail at startup with a clear error:
//
//	grove: migrations requires postgres, but it was not enabled; add grove.WithPostgres()
func WithMigrations() Option {
	return func(b *builder) error {
		b.enableCapability(capMigrations)
		return nil
	}
}

package grove

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/kusold/grove/config"
)

// Option configures a Grove app during construction. Options are applied in
// order before module registration runs. An Option may return an error if its
// configuration is invalid.
type Option func(*builder) error

type builder struct {
	name         string
	capabilities map[capability]bool
}

// capability represents a named capability that can be enabled on an App.
type capability string

const (
	capHTTP capability = "http"
)

// capabilityDeps maps each capability to its required dependencies.
// A capability cannot be enabled unless all of its dependencies are also
// enabled. Dependencies are validated after all options are applied, so the
// order of options does not matter.
var capabilityDeps = map[capability][]capability{}

// capabilityOrder defines the deterministic initialization order for
// capabilities. Capabilities are initialized in this order regardless of the
// order in which options are passed to Main or Run.
var capabilityOrder = []capability{
	capHTTP,
}

// capabilityOptionName maps each capability to the Option function name used
// in error messages to guide users toward the fix.
var capabilityOptionName = map[capability]string{
	capHTTP: "WithHTTP",
}

// capabilityDisplayName maps each capability to a human-readable name used in
// error messages.
var capabilityDisplayName = map[capability]string{
	capHTTP: "http",
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
	return &App{
		name:         b.name,
		capabilities: b.capabilitySet(),
		cfg:          cfg,
		logger:       newLogger(cfg, os.Stdout),
	}
}

// newLogger creates a slog.Logger configured with structured attributes for
// service identity. The handler format is determined by cfg.Logger().Format.
// When using text format, ANSI color codes are applied to level tags based on
// cfg.Logger().Color.
func newLogger(cfg config.Provider, w io.Writer) *slog.Logger {
	svc := cfg.Service()
	attrs := []slog.Attr{
		slog.String("service", svc.Name),
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
	out := p
	for _, rule := range levelColorRules {
		colored := []byte(rule.color + string(rule.level) + colorReset)
		out = colorLevel(out, rule.level, colored)
	}
	return out
}

func colorLevel(p, target, colored []byte) []byte {
	var out []byte
	last := 0
	start := 0

	for {
		idx := bytes.Index(p[start:], target)
		if idx < 0 {
			if out == nil {
				return p
			}
			out = append(out, p[last:]...)
			return out
		}
		idx += start

		start = idx + len(target)
		if !isLevelToken(p, idx, len(target)) {
			continue
		}

		if out == nil {
			out = make([]byte, 0, len(p)+len(colored)-len(target))
		}
		out = append(out, p[last:idx]...)
		out = append(out, colored...)
		last = start
	}
}

// isLevelToken checks that the match at idx is a standalone token: preceded
// by space (or start of line) and followed by space, newline, or end of line.
// This avoids coloring level= inside quoted attribute values.
func isLevelToken(p []byte, idx, length int) bool {
	if idx > 0 && p[idx-1] != ' ' {
		return false
	}
	end := idx + length
	return end == len(p) || p[end] == ' ' || p[end] == '\n'
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

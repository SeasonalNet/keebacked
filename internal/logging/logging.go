// Package logging configures structured and human-readable keebacked logs.
package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	ansiReset   = "\x1b[0m"
	ansiDim     = "\x1b[2m"
	ansiCyan    = "\x1b[36m"
	ansiYellow  = "\x1b[33m"
	ansiRed     = "\x1b[31m"
	defaultTerm = "dumb"
)

// Format selects the log serialization format.
type Format string

const (
	// FormatAuto selects text for terminals and JSON otherwise.
	FormatAuto Format = "auto"
	// FormatText always selects human-readable text.
	FormatText Format = "text"
	// FormatJSON always selects structured JSON.
	FormatJSON Format = "json"
)

// ColorMode controls ANSI styling in text logs.
type ColorMode string

const (
	// ColorAuto enables ANSI styling only for a capable terminal.
	ColorAuto ColorMode = "auto"
	// ColorAlways enables ANSI styling whenever text output is selected.
	ColorAlways ColorMode = "always"
	// ColorNever disables ANSI styling.
	ColorNever ColorMode = "never"
)

// Config defines the runtime logging policy.
type Config struct {
	Format Format    `toml:"format"`
	Color  ColorMode `toml:"color"`
}

// DefaultConfig returns the documented logging defaults.
func DefaultConfig() Config {
	return Config{Format: FormatAuto, Color: ColorAuto}
}

// Validate rejects unsupported logging modes.
func (c Config) Validate() error {
	var errs []error
	if c.Format != FormatAuto && c.Format != FormatText && c.Format != FormatJSON {
		errs = append(errs, fmt.Errorf("logging.format must be one of auto, text, or json, got %q", c.Format))
	}
	if c.Color != ColorAuto && c.Color != ColorAlways && c.Color != ColorNever {
		errs = append(errs, fmt.Errorf("logging.color must be one of auto, always, or never, got %q", c.Color))
	}
	return errors.Join(errs...)
}

type outputState struct {
	terminal bool
	term     string
	noColor  bool
}

// New creates a logger whose automatic behavior is derived from the actual
// output file descriptor and current terminal environment.
func New(output *os.File, cfg Config) (*slog.Logger, error) {
	return newLogger(output, cfg, outputState{
		terminal: isTerminal(output),
		term:     os.Getenv("TERM"),
		noColor:  envDisablesColor(),
	})
}

func newLogger(output io.Writer, cfg Config, state outputState) (*slog.Logger, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	format := cfg.Format
	if format == FormatAuto {
		if state.terminal {
			format = FormatText
		} else {
			format = FormatJSON
		}
	}

	options := &slog.HandlerOptions{Level: slog.LevelInfo}
	if format == FormatJSON {
		return slog.New(slog.NewJSONHandler(output, options)), nil
	}

	color := cfg.Color == ColorAlways ||
		(cfg.Color == ColorAuto && state.terminal && !state.noColor &&
			!strings.EqualFold(strings.TrimSpace(state.term), defaultTerm))

	return slog.New(newTextHandler(output, options, color)), nil
}

func envDisablesColor() bool {
	value, present := os.LookupEnv("NO_COLOR")
	return present && value != ""
}

type storedAttr struct {
	groups []string
	attr   slog.Attr
}

type textHandler struct {
	output io.Writer
	level  slog.Leveler
	color  bool
	attrs  []storedAttr
	groups []string
	mu     *sync.Mutex
}

func newTextHandler(output io.Writer, options *slog.HandlerOptions, color bool) *textHandler {
	level := slog.Leveler(slog.LevelInfo)
	if options != nil && options.Level != nil {
		level = options.Level
	}
	return &textHandler{
		output: output,
		level:  level,
		color:  color,
		mu:     &sync.Mutex{},
	}
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *textHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	if !record.Time.IsZero() {
		line.WriteString(record.Time.Format(time.RFC3339Nano))
		line.WriteByte(' ')
	}

	level := paddedLevel(record.Level)
	if h.color {
		line.WriteString(levelColor(record.Level))
		line.WriteString(level)
		line.WriteString(ansiReset)
	} else {
		line.WriteString(level)
	}
	line.WriteByte(' ')
	line.WriteString(formatMessage(record.Message))

	for _, stored := range h.attrs {
		appendAttr(&line, stored.groups, stored.attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendAttr(&line, h.groups, attr)
		return true
	})
	line.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.output, line.String())
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	for _, attr := range attrs {
		clone.attrs = append(clone.attrs, storedAttr{
			groups: append([]string(nil), h.groups...),
			attr:   attr,
		})
	}
	return clone
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

func (h *textHandler) clone() *textHandler {
	clone := *h
	clone.attrs = append([]storedAttr(nil), h.attrs...)
	clone.groups = append([]string(nil), h.groups...)
	return &clone
}

func appendAttr(line *strings.Builder, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		nestedGroups := groups
		if attr.Key != "" {
			nestedGroups = make([]string, 0, len(groups)+1)
			nestedGroups = append(nestedGroups, groups...)
			nestedGroups = append(nestedGroups, attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			appendAttr(line, nestedGroups, nested)
		}
		return
	}

	key := attr.Key
	if len(groups) != 0 {
		parts := make([]string, 0, len(groups)+1)
		parts = append(parts, groups...)
		parts = append(parts, key)
		key = strings.Join(parts, ".")
	}
	line.WriteByte(' ')
	line.WriteString(key)
	line.WriteByte('=')
	line.WriteString(formatValue(attr.Value))
}

func formatMessage(message string) string {
	if strings.IndexFunc(message, unicode.IsControl) >= 0 {
		return strconv.Quote(message)
	}
	return message
}

func formatValue(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return quoteIfNeeded(value.String())
	case slog.KindBool:
		return strconv.FormatBool(value.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(value.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(value.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(value.Float64(), 'g', -1, 64)
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		return formatAny(value.Any())
	case slog.KindGroup:
		return "{}"
	case slog.KindLogValuer:
		return formatValue(value.Resolve())
	default:
		return quoteIfNeeded(value.String())
	}
}

func formatAny(value any) string {
	if err, ok := value.(error); ok {
		return quoteIfNeeded(err.Error())
	}
	if stringer, ok := value.(fmt.Stringer); ok {
		return quoteIfNeeded(stringer.String())
	}
	encoded, err := json.Marshal(value)
	if err == nil {
		return string(encoded)
	}
	return quoteIfNeeded(fmt.Sprint(value))
}

func quoteIfNeeded(value string) string {
	if value == "" || strings.ContainsAny(value, " =\"\\") || strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return strconv.Quote(value)
	}
	return value
}

func paddedLevel(level slog.Level) string {
	name := strings.ToUpper(level.String())
	if len(name) >= 5 {
		return name
	}
	return name + strings.Repeat(" ", 5-len(name))
}

func levelColor(level slog.Level) string {
	if level < slog.LevelInfo {
		return ansiDim
	}
	if level < slog.LevelWarn {
		return ansiCyan
	}
	if level < slog.LevelError {
		return ansiYellow
	}
	return ansiRed
}

var _ slog.Handler = (*textHandler)(nil)

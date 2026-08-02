package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type staticLogValuer struct {
	value string
}

func (v staticLogValuer) LogValue() slog.Value {
	return slog.StringValue(v.value)
}

func TestAutomaticFormatUsesTextForTerminal(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(&output, DefaultConfig(), outputState{terminal: true, term: "xterm-256color"})
	if err != nil {
		t.Fatal(err)
	}

	logger.Warn("target unavailable", "target", "offline")
	text := output.String()
	if !strings.Contains(text, ansiYellow+"WARN "+ansiReset) {
		t.Fatalf("output missing colored warning level: %q", text)
	}
	if !strings.Contains(text, "target unavailable target=offline") {
		t.Fatalf("output missing human-readable fields: %q", text)
	}
}

func TestAutomaticFormatUsesJSONWithoutTerminal(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(&output, DefaultConfig(), outputState{terminal: false, term: "xterm-256color"})
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("backup complete", "succeeded", 2)
	text := output.String()
	if strings.Contains(text, "\x1b[") {
		t.Fatalf("JSON output contains ANSI escapes: %q", text)
	}

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("output is not valid JSON: %v: %q", err, text)
	}
	if entry["msg"] != "backup complete" || entry["succeeded"] != float64(2) {
		t.Fatalf("unexpected JSON entry: %#v", entry)
	}
}

func TestAutomaticColorHonorsEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		state   outputState
		colored bool
	}{
		{name: "capable terminal", state: outputState{terminal: true, term: "xterm-256color"}, colored: true},
		{name: "NO_COLOR", state: outputState{terminal: true, term: "xterm-256color", noColor: true}, colored: false},
		{name: "dumb terminal", state: outputState{terminal: true, term: "dumb"}, colored: false},
		{name: "not terminal", state: outputState{terminal: false, term: "xterm-256color"}, colored: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			cfg := Config{Format: FormatText, Color: ColorAuto}
			logger, err := newLogger(&output, cfg, test.state)
			if err != nil {
				t.Fatal(err)
			}
			logger.Error("failed")
			got := strings.Contains(output.String(), ansiRed)
			if got != test.colored {
				t.Fatalf("colored = %t, output = %q", got, output.String())
			}
		})
	}
}

func TestExplicitColorModes(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		state   outputState
		colored bool
	}{
		{
			name:    "always colors redirected text",
			cfg:     Config{Format: FormatText, Color: ColorAlways},
			state:   outputState{terminal: false, term: "dumb", noColor: true},
			colored: true,
		},
		{
			name:    "never suppresses terminal color",
			cfg:     Config{Format: FormatText, Color: ColorNever},
			state:   outputState{terminal: true, term: "xterm-256color"},
			colored: false,
		},
		{
			name:    "JSON never contains color",
			cfg:     Config{Format: FormatJSON, Color: ColorAlways},
			state:   outputState{terminal: true, term: "xterm-256color"},
			colored: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := newLogger(&output, test.cfg, test.state)
			if err != nil {
				t.Fatal(err)
			}
			logger.Warn("check")
			got := strings.Contains(output.String(), "\x1b[")
			if got != test.colored {
				t.Fatalf("colored = %t, output = %q", got, output.String())
			}
		})
	}
}

func TestTextHandlerPreservesAttrsAndGroups(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(
		&output,
		Config{Format: FormatText, Color: ColorNever},
		outputState{},
	)
	if err != nil {
		t.Fatal(err)
	}

	logger.With("component", "daemon").WithGroup("target").Info(
		"reconciled",
		"name", "primary backup",
		"bytes", int64(42),
		"error", errors.New("example failure"),
	)

	text := output.String()
	for _, expected := range []string{
		" component=daemon",
		`target.name="primary backup"`,
		"target.bytes=42",
		`target.error="example failure"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output missing %q: %q", expected, text)
		}
	}
	if strings.Contains(text, "target.component=") {
		t.Fatalf("bound attribute was incorrectly moved into a later group: %q", text)
	}
}

func TestTextHandlerEscapesControlCharacters(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(
		&output,
		Config{Format: FormatText, Color: ColorNever},
		outputState{},
	)
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("message\x1b[31m", "path", "target\x1b[2J")
	text := output.String()
	if strings.Contains(text, "\x1b[31m") || strings.Contains(text, "\x1b[2J") {
		t.Fatalf("raw control sequence leaked into text output: %q", text)
	}
	if !strings.Contains(text, `"message\x1b[31m"`) || !strings.Contains(text, `path="target\x1b[2J"`) {
		t.Fatalf("control characters were not visibly escaped: %q", text)
	}
}

func TestNoColorEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if envDisablesColor() {
		t.Fatal("empty NO_COLOR unexpectedly disabled color")
	}
	t.Setenv("NO_COLOR", "1")
	if !envDisablesColor() {
		t.Fatal("non-empty NO_COLOR did not disable color")
	}
}

func TestTextHandlerSerializesConcurrentWrites(t *testing.T) {
	var output bytes.Buffer
	logger, err := newLogger(
		&output,
		Config{Format: FormatText, Color: ColorNever},
		outputState{},
	)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 16
	const messages = 8
	var wait sync.WaitGroup
	wait.Add(workers)
	for worker := range workers {
		go func() {
			defer wait.Done()
			for message := range messages {
				logger.Info("entry", "worker", worker, "message", message)
			}
		}()
	}
	wait.Wait()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != workers*messages {
		t.Fatalf("line count = %d, want %d", len(lines), workers*messages)
	}
	for _, line := range lines {
		if strings.Count(line, "entry") != 1 || !strings.Contains(line, " worker=") || !strings.Contains(line, " message=") {
			t.Fatalf("interleaved or malformed line: %q", line)
		}
	}
}

func TestFormatValueResolvesLogValuer(t *testing.T) {
	got := formatValue(slog.AnyValue(staticLogValuer{value: "resolved value"}))
	if got != `"resolved value"` {
		t.Fatalf("formatValue() = %q, want quoted resolved value", got)
	}
}

func TestValidateRejectsUnknownModes(t *testing.T) {
	tests := []Config{
		{Format: "pretty", Color: ColorAuto},
		{Format: FormatAuto, Color: "sometimes"},
	}
	for _, cfg := range tests {
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%#v) expected an error", cfg)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	var output bytes.Buffer
	handler := newTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelWarn}, false)
	logger := slog.New(handler)
	logger.Info("hidden")
	logger.Warn("visible")
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "visible") {
		t.Fatalf("unexpected filtered output: %q", output.String())
	}
}

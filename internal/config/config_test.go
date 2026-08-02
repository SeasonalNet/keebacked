package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.seasonalnet.org/SeasonalNet/keebacked/internal/logging"
)

func TestTrackedConfigurationMatchesDefaults(t *testing.T) {
	path := filepath.Join("..", "..", "config", "keebacked.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	defaults := Default()
	if cfg.Source != defaults.Source || cfg.Marker != defaults.Marker {
		t.Fatalf("tracked configuration differs from defaults: %#v", cfg)
	}
	if len(cfg.Targets) != len(defaults.Targets) {
		t.Fatalf("Targets = %#v", cfg.Targets)
	}
	for i := range defaults.Targets {
		if cfg.Targets[i] != defaults.Targets[i] {
			t.Fatalf("Targets = %#v", cfg.Targets)
		}
	}
	if cfg.Debounce != defaults.Debounce || cfg.Reconcile != defaults.Reconcile || cfg.Keep != defaults.Keep {
		t.Fatalf("tracked configuration differs from defaults: %#v", cfg)
	}
	if cfg.StabilityInterval != defaults.StabilityInterval || cfg.StabilityAttempts != defaults.StabilityAttempts {
		t.Fatalf("tracked configuration differs from defaults: %#v", cfg)
	}
	if cfg.Logging != defaults.Logging {
		t.Fatalf("tracked logging configuration differs from defaults: %#v", cfg.Logging)
	}
}

func TestLoadTOML(t *testing.T) {
	path := writeConfig(t, `
source = "/vault/vault.kdbx"
targets = ["/backup/primary", " /backup/offline ", "/backup/primary"]
marker = ".vault-target"
debounce = "5s"
reconcile = "30m"
keep = 42
stability_interval = "100ms"
stability_attempts = 4

[logging]
format = "text"
color = "never"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Source != "/vault/vault.kdbx" {
		t.Fatalf("Source = %q", cfg.Source)
	}
	if len(cfg.Targets) != 2 || cfg.Targets[0] != "/backup/primary" || cfg.Targets[1] != "/backup/offline" {
		t.Fatalf("Targets = %#v", cfg.Targets)
	}
	if cfg.Marker != ".vault-target" {
		t.Fatalf("Marker = %q", cfg.Marker)
	}
	if cfg.Debounce != 5*time.Second || cfg.Reconcile != 30*time.Minute {
		t.Fatalf("durations = %s, %s", cfg.Debounce, cfg.Reconcile)
	}
	if cfg.Keep != 42 || cfg.StabilityAttempts != 4 {
		t.Fatalf("numeric configuration = keep %d, attempts %d", cfg.Keep, cfg.StabilityAttempts)
	}
	if cfg.StabilityInterval != 100*time.Millisecond {
		t.Fatalf("StabilityInterval = %s", cfg.StabilityInterval)
	}
	if cfg.Logging.Format != logging.FormatText || cfg.Logging.Color != logging.ColorNever {
		t.Fatalf("Logging = %#v", cfg.Logging)
	}
}

func TestLoadAppliesDefaultsForOmittedKeys(t *testing.T) {
	path := writeConfig(t, `
source = "/vault/vault.kdbx"
targets = ["/backup/primary"]
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	defaults := Default()
	if cfg.Marker != defaults.Marker || cfg.Debounce != defaults.Debounce || cfg.Reconcile != defaults.Reconcile {
		t.Fatalf("defaults not applied: %#v", cfg)
	}
	if cfg.Keep != defaults.Keep || cfg.StabilityInterval != defaults.StabilityInterval || cfg.StabilityAttempts != defaults.StabilityAttempts {
		t.Fatalf("numeric defaults not applied: %#v", cfg)
	}
	if cfg.Logging != defaults.Logging {
		t.Fatalf("logging defaults not applied: %#v", cfg.Logging)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, `
source = "/vault/vault.kdbx"
targets = ["/backup/primary"]
unknown_setting = "1m"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected an error")
	}
	if !strings.Contains(err.Error(), "unknown keys: unknown_setting") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsUnknownLoggingKeys(t *testing.T) {
	path := writeConfig(t, `
source = "/vault/vault.kdbx"
targets = ["/backup/primary"]

[logging]
format = "auto"
unknown_option = true
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected an error")
	}
	if !strings.Contains(err.Error(), "unknown keys: logging.unknown_option") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsInvalidLoggingModes(t *testing.T) {
	tests := []struct {
		name    string
		setting string
		want    string
	}{
		{name: "format", setting: `format = "pretty"`, want: "logging.format"},
		{name: "color", setting: `color = "sometimes"`, want: "logging.color"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeConfig(t, `
source = "/vault/vault.kdbx"
targets = ["/backup/primary"]

[logging]
`+test.setting+"\n")

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load() expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadRejectsMalformedTOML(t *testing.T) {
	path := writeConfig(t, `source = [`) // Intentionally incomplete TOML.

	if _, err := Load(path); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	if _, err := Load(path); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestLoadRejectsEmptyPath(t *testing.T) {
	if _, err := Load("   "); err == nil {
		t.Fatal("Load() expected an error")
	}
}

func TestValidateRejectsRelativePaths(t *testing.T) {
	cfg := Config{
		Source:            "vault.kdbx",
		Targets:           []string{"backup"},
		Marker:            ".keebacked-target",
		Debounce:          time.Second,
		Reconcile:         time.Minute,
		Keep:              1,
		StabilityInterval: time.Millisecond,
		StabilityAttempts: 1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected an error")
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keebacked.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

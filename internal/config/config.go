// Package config loads and validates keebacked runtime configuration.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"git.seasonalnet.org/SeasonalNet/keebacked/internal/logging"
	"github.com/BurntSushi/toml"
)

const (
	// DefaultPath is the default runtime configuration file.
	DefaultPath = "/etc/keebacked/keebacked.toml"

	defaultSource            = "/source/vault.kdbx"
	defaultMarker            = ".keebacked-target"
	defaultDebounce          = 3 * time.Second
	defaultReconcile         = 15 * time.Minute
	defaultKeep              = 100
	defaultStabilityInterval = 250 * time.Millisecond
	defaultStabilityAttempts = 8
)

var defaultTargets = []string{"/targets/primary", "/targets/offline"}

// Config defines the complete runtime configuration for keebacked.
type Config struct {
	Source            string         `toml:"source"`
	Targets           []string       `toml:"targets"`
	Marker            string         `toml:"marker"`
	Debounce          time.Duration  `toml:"debounce"`
	Reconcile         time.Duration  `toml:"reconcile"`
	Keep              int            `toml:"keep"`
	StabilityInterval time.Duration  `toml:"stability_interval"`
	StabilityAttempts int            `toml:"stability_attempts"`
	Logging           logging.Config `toml:"logging"`
}

// Default returns the documented baseline configuration.
func Default() Config {
	return Config{
		Source:            defaultSource,
		Targets:           append([]string(nil), defaultTargets...),
		Marker:            defaultMarker,
		Debounce:          defaultDebounce,
		Reconcile:         defaultReconcile,
		Keep:              defaultKeep,
		StabilityInterval: defaultStabilityInterval,
		StabilityAttempts: defaultStabilityAttempts,
		Logging:           logging.DefaultConfig(),
	}
}

// Load decodes a TOML configuration file, rejects unknown keys, normalizes
// targets, and validates the result.
func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, errors.New("configuration path must not be empty")
	}

	cfg := Default()
	// #nosec G304 -- the configuration path is an explicit command-line selection.
	metadata, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, fmt.Errorf("decode configuration %q: %w", path, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) != 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return Config{}, fmt.Errorf("decode configuration %q: unknown keys: %s", path, strings.Join(keys, ", "))
	}

	cfg.Targets = normalizeTargets(cfg.Targets)
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate configuration %q: %w", path, err)
	}
	return cfg, nil
}

// Validate rejects ambiguous or unsafe configuration.
func (c Config) Validate() error {
	var errs []error

	if !filepath.IsAbs(c.Source) {
		errs = append(errs, errors.New("source must be an absolute path"))
	}
	if len(c.Targets) == 0 {
		errs = append(errs, errors.New("targets must contain at least one target"))
	}
	for _, target := range c.Targets {
		if !filepath.IsAbs(target) {
			errs = append(errs, fmt.Errorf("target %q must be an absolute path", target))
		}
		if filepath.Clean(target) == filepath.Clean(filepath.Dir(c.Source)) {
			errs = append(errs, fmt.Errorf("target %q must not be the source directory", target))
		}
	}
	if c.Marker == "" || filepath.Base(c.Marker) != c.Marker || c.Marker == "." || c.Marker == ".." {
		errs = append(errs, errors.New("marker must be a plain filename"))
	}
	if c.Debounce <= 0 {
		errs = append(errs, errors.New("debounce must be positive"))
	}
	if c.Reconcile <= 0 {
		errs = append(errs, errors.New("reconcile must be positive"))
	}
	if c.Keep < 0 {
		errs = append(errs, errors.New("keep must be zero or greater"))
	}
	if c.StabilityInterval <= 0 {
		errs = append(errs, errors.New("stability_interval must be positive"))
	}
	if c.StabilityAttempts < 1 {
		errs = append(errs, errors.New("stability_attempts must be at least one"))
	}
	if err := c.Logging.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func normalizeTargets(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	targets := make([]string, 0, len(values))
	for _, raw := range values {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		target := filepath.Clean(strings.TrimSpace(raw))
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

// Package main provides the keebacked daemon command.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"git.seasonalnet.org/SeasonalNet/keebacked/internal/backup"
	"git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo"
	"git.seasonalnet.org/SeasonalNet/keebacked/internal/config"
	"git.seasonalnet.org/SeasonalNet/keebacked/internal/daemon"
	"git.seasonalnet.org/SeasonalNet/keebacked/internal/logging"
)

func main() {
	os.Exit(run())
}

func run() int {
	var configPath string
	var once bool
	var showVersion bool
	flag.StringVar(&configPath, "config", config.DefaultPath, "path to the TOML configuration file")
	flag.BoolVar(&once, "once", false, "perform one reconciliation and exit")
	flag.BoolVar(&showVersion, "version", false, "print version information and exit")
	flag.Parse()

	build := buildinfo.Current()
	if showVersion {
		if _, err := fmt.Fprint(os.Stdout, build.String()); err != nil {
			return 1
		}
		return 0
	}

	logger, err := logging.New(os.Stdout, logging.DefaultConfig())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "initialize bootstrap logger: %v\n", err)
		return 1
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		return 2
	}

	logger, err = logging.New(os.Stdout, cfg.Logging)
	if err != nil {
		logger.Error("invalid logging configuration", "error", err)
		return 2
	}
	slog.SetDefault(logger)

	engine := &backup.Engine{
		Source:            cfg.Source,
		Targets:           cfg.Targets,
		Marker:            cfg.Marker,
		Keep:              cfg.Keep,
		StabilityInterval: cfg.StabilityInterval,
		StabilityAttempts: cfg.StabilityAttempts,
		Logger:            logger,
	}

	if once {
		result, runErr := engine.Run(context.Background())
		if runErr != nil {
			logger.Error("backup failed", "error", runErr)
			return 1
		}
		logger.Info(
			"backup complete",
			"sha256", result.Digest,
			"bytes", result.Size,
			"succeeded", result.Succeeded,
			"skipped", result.Skipped,
			"failed", result.Failed,
		)
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"keebacked starting",
		"version", build.Version,
		"commit", build.Commit,
		"modified", build.Modified,
		"built", build.BuildDate,
		"target", build.Target,
		"go", build.GoVersion,
		"config", configPath,
		"source", cfg.Source,
		"targets", cfg.Targets,
		"debounce", cfg.Debounce.String(),
		"reconcile", cfg.Reconcile.String(),
	)
	if err := daemon.Run(ctx, engine, cfg.Debounce, cfg.Reconcile, logger); err != nil {
		logger.Error("daemon stopped with error", "error", err)
		return 1
	}
	logger.Info("keebacked stopped")
	return 0
}

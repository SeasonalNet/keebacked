// Package daemon coordinates event-driven and periodic backup reconciliation.
package daemon

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"git.seasonalnet.org/SeasonalNet/keebacked/internal/backup"
	"git.seasonalnet.org/SeasonalNet/keebacked/internal/watch"
)

type directoryWatcher interface {
	Events() <-chan struct{}
	Errors() <-chan error
	Close() error
}

type watcherFactory func(string) (directoryWatcher, error)

var errEventChannelClosed = errors.New("filesystem watcher event channel closed")

// Run watches the source directory, debounces events, and periodically reconciles targets.
func Run(ctx context.Context, engine *backup.Engine, debounce, reconcile time.Duration, logger *slog.Logger) error {
	return run(ctx, engine, debounce, reconcile, logger, func(path string) (directoryWatcher, error) {
		return watch.NewDirectory(path)
	})
}

func run(
	ctx context.Context,
	engine *backup.Engine,
	debounce time.Duration,
	reconcile time.Duration,
	logger *slog.Logger,
	newWatcher watcherFactory,
) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	watcher, err := newWatcher(filepath.Dir(engine.Source))
	if err != nil {
		return err
	}
	defer func() {
		_ = watcher.Close()
	}()

	go func() {
		<-runCtx.Done()
		_ = watcher.Close()
	}()

	trigger := make(chan string, 1)
	enqueue := func(reason string) {
		select {
		case trigger <- reason:
		default:
		}
	}

	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case <-runCtx.Done():
				return
			case reason := <-trigger:
				result, runErr := engine.Run(runCtx)
				if runErr != nil {
					if !errors.Is(runErr, context.Canceled) {
						logger.Error("reconciliation failed", "reason", reason, "error", runErr)
					}
					continue
				}
				logger.Info(
					"reconciliation complete",
					"reason", reason,
					"sha256", result.Digest,
					"bytes", result.Size,
					"succeeded", result.Succeeded,
					"skipped", result.Skipped,
					"failed", result.Failed,
				)
			}
		}
	}()

	shutdown := func(runErr error) error {
		cancel()
		<-workerDone
		return runErr
	}

	reconcileTicker := time.NewTicker(reconcile)
	defer reconcileTicker.Stop()

	var debounceTimer *time.Timer
	var debounceC <-chan time.Time
	resetDebounce := func() {
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(debounce)
			debounceC = debounceTimer.C
			return
		}
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(debounce)
		debounceC = debounceTimer.C
	}
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	events := watcher.Events()
	watchErrors := watcher.Errors()
	enqueue("startup")
	for {
		select {
		case <-runCtx.Done():
			return shutdown(nil)
		case _, ok := <-events:
			if !ok {
				if ctx.Err() != nil {
					return shutdown(nil)
				}
				return shutdown(errEventChannelClosed)
			}
			resetDebounce()
		case watchErr, ok := <-watchErrors:
			if !ok {
				watchErrors = nil
				continue
			}
			logger.Warn("filesystem watcher error", "error", watchErr)
		case <-debounceC:
			debounceC = nil
			enqueue("filesystem-event")
		case <-reconcileTicker.C:
			enqueue("periodic")
		}
	}
}

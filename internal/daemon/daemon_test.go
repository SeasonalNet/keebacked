package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.seasonalnet.org/SeasonalNet/keebacked/internal/backup"
)

func TestRunStopsCleanlyWhenContextIsCanceled(t *testing.T) {
	engine, latest := daemonTestEngine(t)
	watcher := newFakeWatcher()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, engine, time.Millisecond, time.Hour, discardLogger(), watcher.factory)
	}()

	waitForFile(t, latest)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not stop after context cancellation")
	}

	watcher.assertClosed(t)
}

func TestRunStopsWorkerWhenEventChannelCloses(t *testing.T) {
	engine, _ := daemonTestEngine(t)
	watcher := newFakeWatcher()
	close(watcher.events)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), engine, time.Millisecond, time.Hour, discardLogger(), watcher.factory)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, errEventChannelClosed) {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not stop after the event channel closed")
	}

	watcher.assertClosed(t)
}

func TestRunContinuesAfterWatcherErrorChannelCloses(t *testing.T) {
	engine, latest := daemonTestEngine(t)
	watcher := newFakeWatcher()
	close(watcher.errs)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, engine, time.Millisecond, time.Hour, discardLogger(), watcher.factory)
	}()

	waitForFile(t, latest)
	select {
	case err := <-done:
		t.Fatalf("run() returned before cancellation: %v", err)
	default:
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run() did not stop after context cancellation")
	}
}

type fakeWatcher struct {
	events    chan struct{}
	errs      chan error
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{
		events: make(chan struct{}, 1),
		errs:   make(chan error, 1),
		closed: make(chan struct{}),
	}
}

func (f *fakeWatcher) factory(string) (directoryWatcher, error) {
	return f, nil
}

func (f *fakeWatcher) Events() <-chan struct{} {
	return f.events
}

func (f *fakeWatcher) Errors() <-chan error {
	return f.errs
}

func (f *fakeWatcher) Close() error {
	f.closeOnce.Do(func() {
		close(f.closed)
	})
	return nil
}

func (f *fakeWatcher) assertClosed(t *testing.T) {
	t.Helper()
	select {
	case <-f.closed:
	case <-time.After(time.Second):
		t.Fatal("watcher was not closed")
	}
}

func daemonTestEngine(t *testing.T) (*backup.Engine, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	target := filepath.Join(root, "target")
	if err := os.WriteFile(source, []byte("vault"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".keebacked-target"), []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return &backup.Engine{
		Source:            source,
		Targets:           []string{target},
		Marker:            ".keebacked-target",
		Keep:              5,
		StabilityInterval: time.Millisecond,
		StabilityAttempts: 3,
		Logger:            discardLogger(),
	}, filepath.Join(target, "latest", "vault.kdbx")
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

//go:build linux

package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirectoryEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	watcher, err := NewDirectory(dir)
	if err != nil {
		t.Fatalf("NewDirectory() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := watcher.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	if err := os.WriteFile(filepath.Join(dir, "vault.kdbx"), []byte("vault"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-watcher.Events():
	case err := <-watcher.Errors():
		t.Fatalf("watcher error = %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inotify event")
	}
}

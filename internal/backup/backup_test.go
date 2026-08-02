package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEngineCreatesVersionedAndLatestCopies(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	target := createTarget(t, root, "target")
	mustWriteFile(t, source, []byte("encrypted vault bytes"), 0o600)

	engine := testEngine(source, []string{target})
	engine.Now = func() time.Time {
		return time.Date(2026, 8, 2, 4, 12, 34, 123456789, time.UTC)
	}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("Succeeded = %d", result.Succeeded)
	}

	latest := filepath.Join(target, "latest", "vault.kdbx")
	assertFileContents(t, latest, "encrypted vault bytes")

	history, err := filepath.Glob(filepath.Join(target, "history", "*.kdbx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history files = %#v", history)
	}
	assertFileContents(t, history[0], "encrypted vault bytes")

	digest := sha256.Sum256([]byte("encrypted vault bytes"))
	assertFileContents(t, latest+".sha256", hex.EncodeToString(digest[:])+"\n")
}

func TestEngineDoesNotDuplicateUnchangedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	target := createTarget(t, root, "target")
	mustWriteFile(t, source, []byte("vault"), 0o600)
	engine := testEngine(source, []string{target})

	for range 2 {
		if _, err := engine.Run(context.Background()); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}

	history, err := filepath.Glob(filepath.Join(target, "history", "*.kdbx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history count = %d, files = %#v", len(history), history)
	}
}

func TestEngineReportsWhenNoTargetSucceeds(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	target := filepath.Join(root, "target")
	mustWriteFile(t, source, []byte("vault"), 0o600)
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := testEngine(source, []string{target}).Run(context.Background())
	if !errors.Is(err, ErrNoTargetSucceeded) {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Skipped != 1 || result.Succeeded != 0 {
		t.Fatalf("result = %+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(target, "history")); !os.IsNotExist(statErr) {
		t.Fatalf("history directory unexpectedly exists: %v", statErr)
	}
}

func TestEngineHandlesTargetsIndependently(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	good := createTarget(t, root, "good")
	missingMarker := filepath.Join(root, "missing-marker")
	mustWriteFile(t, source, []byte("vault"), 0o600)
	if err := os.Mkdir(missingMarker, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := testEngine(source, []string{missingMarker, good}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Succeeded != 1 || result.Skipped != 1 {
		t.Fatalf("result = %+v", result)
	}
	assertFileContents(t, filepath.Join(good, "latest", "vault.kdbx"), "vault")
}

func TestEngineRetainsNewestHistoryFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	target := createTarget(t, root, "target")
	mustWriteFile(t, source, []byte("v1"), 0o600)

	engine := testEngine(source, []string{target})
	engine.Keep = 2
	clock := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	engine.Now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}

	for _, contents := range []string{"v1", "v2", "v3"} {
		mustWriteFile(t, source, []byte(contents), 0o600)
		if _, err := engine.Run(context.Background()); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	}

	history, err := filepath.Glob(filepath.Join(target, "history", "*.kdbx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history count = %d, files = %#v", len(history), history)
	}
	assertFileContents(t, filepath.Join(target, "latest", "vault.kdbx"), "v3")
}

func TestValidateTargetRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	realTarget := createTarget(t, root, "real")
	link := filepath.Join(root, "link")
	if err := os.Symlink(realTarget, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateTarget(link, ".keebacked-target"); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("validateTarget() error = %v", err)
	}
}

func testEngine(source string, targets []string) *Engine {
	return &Engine{
		Source:            source,
		Targets:           targets,
		Marker:            ".keebacked-target",
		Keep:              100,
		StabilityInterval: time.Millisecond,
		StabilityAttempts: 3,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func createTarget(t *testing.T, root, name string) string {
	t.Helper()
	target := filepath.Join(root, name)
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(target, ".keebacked-target"), []byte(name+"\n"), 0o600)
	return target
}

func mustWriteFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFileContents(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("%s contents = %q, expected %q", path, data, expected)
	}
}

func TestEnsurePrivateDirectoryCreatesAndRepairsOwnerOnlyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private")
	if err := ensurePrivateDirectory(path); err != nil {
		t.Fatalf("ensurePrivateDirectory() create error = %v", err)
	}
	assertPermissions(t, path, 0o700)

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(path); err != nil {
		t.Fatalf("ensurePrivateDirectory() repair error = %v", err)
	}
	assertPermissions(t, path, 0o700)
}

func TestEnsurePrivateDirectoryRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDirectory, link); err != nil {
		t.Fatal(err)
	}

	if err := ensurePrivateDirectory(link); err == nil {
		t.Fatal("ensurePrivateDirectory() expected an error for a symlink")
	}
}

func TestEnginePublishesPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "vault.kdbx")
	target := createTarget(t, root, "target")
	mustWriteFile(t, source, []byte("vault"), 0o600)

	if _, err := testEngine(source, []string{target}).Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	historyDir := filepath.Join(target, "history")
	latestDir := filepath.Join(target, "latest")
	assertPermissions(t, historyDir, 0o700)
	assertPermissions(t, latestDir, 0o700)
	assertPermissions(t, filepath.Join(latestDir, "vault.kdbx"), 0o600)
	assertPermissions(t, filepath.Join(latestDir, "vault.kdbx.sha256"), 0o600)

	history, err := filepath.Glob(filepath.Join(historyDir, "*.kdbx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history files = %#v", history)
	}
	assertPermissions(t, history[0], 0o600)
}

func assertPermissions(t *testing.T, path string, expected os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := info.Mode().Perm(); actual != expected {
		t.Fatalf("%s permissions = %#o, expected %#o", path, actual, expected)
	}
}

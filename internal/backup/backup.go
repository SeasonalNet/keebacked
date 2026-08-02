// Package backup creates verified, atomic, versioned copies of opaque files.
package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var (
	// ErrTargetUnavailable indicates that a destination is intentionally skipped.
	ErrTargetUnavailable = errors.New("backup target unavailable")
	// ErrSourceUnstable indicates that no consistent source snapshot could be made.
	ErrSourceUnstable = errors.New("source did not become stable")
	// ErrNoTargetSucceeded indicates that a pass could not update any destination.
	ErrNoTargetSucceeded = errors.New("no backup target succeeded")
)

// Engine creates verified copies of one opaque source file.
type Engine struct {
	Source            string
	Targets           []string
	Marker            string
	Keep              int
	StabilityInterval time.Duration
	StabilityAttempts int
	Logger            *slog.Logger
	Now               func() time.Time
}

// Result summarizes one reconciliation pass.
type Result struct {
	Digest    string
	Size      int64
	Succeeded int
	Skipped   int
	Failed    int
}

type snapshot struct {
	path   string
	digest string
	size   int64
}

type targetStatus uint8

const (
	targetSucceeded targetStatus = iota
	targetSkipped
	targetFailed
)

// Run creates one stable snapshot and reconciles every configured target.
func (e *Engine) Run(ctx context.Context) (Result, error) {
	if e.Logger == nil {
		e.Logger = slog.Default()
	}
	if e.Now == nil {
		e.Now = time.Now
	}

	snap, err := e.makeSnapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if removeErr := os.Remove(snap.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			e.Logger.Warn("failed to remove temporary snapshot", "path", snap.path, "error", removeErr)
		}
	}()

	result := Result{Digest: snap.digest, Size: snap.size}
	var targetErrs []error
	for _, target := range e.Targets {
		name, status, targetErr := e.reconcileTarget(ctx, snap, target)
		switch status {
		case targetSucceeded:
			result.Succeeded++
			e.Logger.Info("backup target reconciled", "target", name, "path", target, "sha256", snap.digest, "bytes", snap.size)
		case targetSkipped:
			result.Skipped++
			e.Logger.Warn("backup target skipped", "target", name, "path", target, "reason", targetErr)
		case targetFailed:
			result.Failed++
			e.Logger.Error("backup target failed", "target", name, "path", target, "error", targetErr)
			targetErrs = append(targetErrs, fmt.Errorf("target %s: %w", target, targetErr))
		}
	}

	if result.Succeeded == 0 {
		return result, errors.Join(append([]error{ErrNoTargetSucceeded}, targetErrs...)...)
	}
	return result, nil
}

func (e *Engine) makeSnapshot(ctx context.Context) (snapshot, error) {
	for range e.StabilityAttempts {
		snap, stable, err := e.trySnapshot(ctx)
		if err != nil {
			return snapshot{}, err
		}
		if stable {
			return snap, nil
		}
	}
	return snapshot{}, ErrSourceUnstable
}

func (e *Engine) trySnapshot(ctx context.Context) (snapshot, bool, error) {
	if err := ctx.Err(); err != nil {
		return snapshot{}, false, err
	}

	before, err := os.Stat(e.Source)
	if err != nil {
		return snapshot{}, false, fmt.Errorf("stat source: %w", err)
	}
	if !before.Mode().IsRegular() {
		return snapshot{}, false, fmt.Errorf("source %q is not a regular file", e.Source)
	}
	if err := sleepContext(ctx, e.StabilityInterval); err != nil {
		return snapshot{}, false, err
	}

	afterDelay, err := os.Stat(e.Source)
	if err != nil || !sameState(before, afterDelay) {
		return snapshot{}, false, nil
	}

	tmp, err := os.CreateTemp("", "keebacked-snapshot-*")
	if err != nil {
		return snapshot{}, false, fmt.Errorf("create snapshot: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	// #nosec G304 -- the source path is explicit validated configuration and is intentionally opened.
	source, err := os.Open(e.Source)
	if err != nil {
		cleanup()
		return snapshot{}, false, nil
	}

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), source)
	closeSourceErr := source.Close()
	syncErr := tmp.Sync()
	closeTempErr := tmp.Close()
	if err := errors.Join(copyErr, closeSourceErr, syncErr, closeTempErr); err != nil {
		_ = os.Remove(tmpPath)
		return snapshot{}, false, fmt.Errorf("create source snapshot: %w", err)
	}

	afterCopy, err := os.Stat(e.Source)
	if err != nil || !sameState(afterDelay, afterCopy) || size != afterCopy.Size() {
		_ = os.Remove(tmpPath)
		return snapshot{}, false, nil
	}

	return snapshot{
		path:   tmpPath,
		digest: hex.EncodeToString(hash.Sum(nil)),
		size:   size,
	}, true, nil
}

func (e *Engine) reconcileTarget(ctx context.Context, snap snapshot, target string) (string, targetStatus, error) {
	name, err := validateTarget(target, e.Marker)
	if err != nil {
		return filepath.Base(target), targetSkipped, err
	}

	historyDir := filepath.Join(target, "history")
	latestDir := filepath.Join(target, "latest")
	if err := ensurePrivateDirectory(historyDir); err != nil {
		return name, targetFailed, fmt.Errorf("prepare history directory: %w", err)
	}
	if err := ensurePrivateDirectory(latestDir); err != nil {
		return name, targetFailed, fmt.Errorf("prepare latest directory: %w", err)
	}

	base := filepath.Base(e.Source)
	latestPath := filepath.Join(latestDir, base)
	latestHashPath := latestPath + ".sha256"
	currentDigest, _ := readDigest(latestHashPath)
	if currentDigest == snap.digest {
		actualDigest, digestErr := fileDigest(latestPath)
		if digestErr == nil && actualDigest == snap.digest {
			return name, targetSucceeded, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return name, targetFailed, err
	}
	if _, err := validateTarget(target, e.Marker); err != nil {
		return name, targetSkipped, err
	}

	historyName := versionedName(base, e.Now().UTC(), snap.digest)
	historyPath := filepath.Join(historyDir, historyName)
	if err := atomicVerifiedCopy(snap.path, historyPath, snap.digest); err != nil {
		return name, targetFailed, fmt.Errorf("publish history copy: %w", err)
	}
	if _, err := validateTarget(target, e.Marker); err != nil {
		return name, targetSkipped, err
	}
	if err := atomicVerifiedCopy(snap.path, latestPath, snap.digest); err != nil {
		return name, targetFailed, fmt.Errorf("publish latest copy: %w", err)
	}
	if err := atomicWrite(latestHashPath, []byte(snap.digest+"\n"), 0o600); err != nil {
		return name, targetFailed, fmt.Errorf("publish latest digest: %w", err)
	}
	if err := pruneHistory(historyDir, base, e.Keep); err != nil {
		return name, targetFailed, fmt.Errorf("prune history: %w", err)
	}

	return name, targetSucceeded, nil
}

func validateTarget(target, markerName string) (string, error) {
	info, err := os.Lstat(target)
	if err != nil {
		return "", fmt.Errorf("%w: stat target: %w", ErrTargetUnavailable, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%w: target is not a real directory", ErrTargetUnavailable)
	}

	markerPath := filepath.Join(target, markerName)
	markerInfo, err := os.Lstat(markerPath)
	if err != nil {
		return "", fmt.Errorf("%w: marker missing: %w", ErrTargetUnavailable, err)
	}
	if !markerInfo.Mode().IsRegular() {
		return "", fmt.Errorf("%w: marker is not a regular file", ErrTargetUnavailable)
	}
	if markerInfo.Size() > 256 {
		return "", fmt.Errorf("%w: marker exceeds 256 bytes", ErrTargetUnavailable)
	}

	// #nosec G304 -- the marker resides under a validated configured target.
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		return "", fmt.Errorf("%w: read marker: %w", ErrTargetUnavailable, err)
	}
	name := strings.TrimSpace(string(marker))
	if name == "" {
		name = filepath.Base(target)
	}
	return name, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%q is not a real directory", path)
	}
	// #nosec G302 -- this path is a directory and requires owner execute permission.
	return os.Chmod(path, 0o700)
}

func atomicVerifiedCopy(source, destination, expectedDigest string) error {
	dir := filepath.Dir(destination)
	tmp, err := os.CreateTemp(dir, ".keebacked-copy-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	// #nosec G304 -- source is an internal temporary snapshot created by this process.
	input, err := os.Open(source)
	if err != nil {
		_ = tmp.Close()
		return err
	}

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, hash), input)
	closeInputErr := input.Close()
	chmodErr := tmp.Chmod(0o600)
	syncErr := tmp.Sync()
	closeTempErr := tmp.Close()
	if err := errors.Join(copyErr, closeInputErr, chmodErr, syncErr, closeTempErr); err != nil {
		return err
	}

	actualDigest := hex.EncodeToString(hash.Sum(nil))
	if actualDigest != expectedDigest {
		return fmt.Errorf("destination digest mismatch: got %s, expected %s", actualDigest, expectedDigest)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func atomicWrite(destination string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(destination)
	tmp, err := os.CreateTemp(dir, ".keebacked-data-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func pruneHistory(historyDir, sourceBase string, keep int) error {
	if keep == 0 {
		return nil
	}

	entries, err := os.ReadDir(historyDir)
	if err != nil {
		return err
	}

	stem, ext := splitName(sourceBase)
	prefix := stem + "-"
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ext) {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	slices.Reverse(names)
	if len(names) <= keep {
		return nil
	}
	for _, name := range names[keep:] {
		if err := os.Remove(filepath.Join(historyDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return syncDirectory(historyDir)
}

func versionedName(sourceBase string, timestamp time.Time, digest string) string {
	stem, ext := splitName(sourceBase)
	return fmt.Sprintf("%s-%s-%s%s", stem, timestamp.Format("20060102T150405.000000000Z"), digest[:12], ext)
}

func splitName(base string) (string, string) {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "backup"
	}
	return stem, ext
}

func readDigest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > 256 {
		return "", errors.New("digest file exceeds 256 bytes")
	}
	// #nosec G304 -- digest path is derived from a validated target and source basename.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func fileDigest(path string) (string, error) {
	// #nosec G304 -- path is derived from a validated target and source basename.
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncDirectory(path string) error {
	// #nosec G304 -- path is an internally derived destination directory.
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}

func sameState(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

//go:build linux

//lint:file-ignore SA1019 The standard-library syscall inotify API is intentionally used to keep the daemon dependency-free.

// Package watch provides the minimal Linux filesystem notification layer used by keebacked.
package watch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"syscall"
)

const watchMask = syscall.IN_CLOSE_WRITE |
	syscall.IN_CREATE |
	syscall.IN_DELETE |
	syscall.IN_MOVED_FROM |
	syscall.IN_MOVED_TO |
	syscall.IN_ATTRIB |
	syscall.IN_DELETE_SELF |
	syscall.IN_MOVE_SELF

// Directory emits a notification whenever inotify reports relevant activity in one directory.
type Directory struct {
	fd        int
	events    chan struct{}
	errors    chan error
	closeOnce sync.Once
}

// NewDirectory starts watching path. The caller must close the watcher.
func NewDirectory(path string) (*Directory, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("initialize inotify: %w", err)
	}
	if _, err := syscall.InotifyAddWatch(fd, path, watchMask); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("watch directory %q: %w", path, err)
	}

	watcher := &Directory{
		fd:     fd,
		events: make(chan struct{}, 1),
		errors: make(chan error, 1),
	}
	go watcher.readLoop()
	return watcher, nil
}

// Events returns a coalescing channel of filesystem activity notifications.
func (d *Directory) Events() <-chan struct{} {
	return d.events
}

// Errors returns terminal watcher errors.
func (d *Directory) Errors() <-chan error {
	return d.errors
}

// Close stops the watcher and releases its file descriptor.
func (d *Directory) Close() error {
	var closeErr error
	d.closeOnce.Do(func() {
		closeErr = syscall.Close(d.fd)
	})
	return closeErr
}

func (d *Directory) readLoop() {
	defer close(d.events)
	defer close(d.errors)

	buffer := make([]byte, 16*1024)
	for {
		count, err := syscall.Read(d.fd, buffer)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if errors.Is(err, syscall.EBADF) || errors.Is(err, syscall.EINVAL) {
				return
			}
			d.errors <- fmt.Errorf("read inotify events: %w", err)
			return
		}
		if count == 0 {
			continue
		}
		select {
		case d.events <- struct{}{}:
		default:
		}
	}
}

// CloseOnContext closes watcher when ctx is cancelled.
func CloseOnContext(ctx context.Context, watcher *Directory) {
	go func() {
		<-ctx.Done()
		_ = watcher.Close()
	}()
}

//go:build linux

//lint:file-ignore SA1019 The standard-library syscall ioctl API keeps terminal detection dependency-free.

package logging

import (
	"os"
	"syscall"
	"unsafe"
)

func isTerminal(file *os.File) bool {
	var state syscall.Termios
	// #nosec G103 -- ioctl requires passing a pointer to the termios result buffer.
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&state)),
	)
	return errno == 0
}

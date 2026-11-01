//go:build linux

package schemaintel

import (
	"os"
	"syscall"
)

// openRegularOpenAPIDocument opens the final pathname component without
// following symlinks and without blocking on a FIFO/device before fstat.
// ReadFileLimited verifies that the opened descriptor is a regular file.
func openRegularOpenAPIDocument(path string) (*os.File, error) {
	fd, err := syscall.Open(path,
		syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

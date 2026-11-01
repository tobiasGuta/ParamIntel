//go:build !linux

package schemaintel

import (
	"fmt"
	"os"
)

// Non-Linux platforms do not support -openapi-untrusted. Reject special
// files and symbolic links before opening a trusted, operator-selected file.
// Do not claim that this precheck is an atomic defense against path races.
func openRegularOpenAPIDocument(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("openapi document must be a regular file (not a symlink or special file)")
	}
	return os.Open(path)
}

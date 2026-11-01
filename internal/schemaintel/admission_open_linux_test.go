//go:build linux

package schemaintel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadFileLimitedRejectsSymlinkWithoutFollowingIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "source.yaml")
	if err := os.WriteFile(target, []byte("openapi: 3.1.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFileLimited(link, DefaultMaxDocumentBytes)
	if err == nil {
		t.Fatal("symbolic link unexpectedly accepted")
	}
	if !errors.Is(err, syscall.ELOOP) && !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("unexpected symlink error: %v", err)
	}
}

func TestReadFileLimitedRejectsFIFOWithoutBlocking(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "malicious.yaml")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := ReadFileLimited(pipe, DefaultMaxDocumentBytes)
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("named pipe should fail closed, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("named pipe blocked opening before regular-file admission")
	}
}

//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// A subprocess that spawns a child and retains stdout/stderr must not
// prevent cancellation from returning within a bounded wall-clock interval.
func TestOpenAPIWorkerProcessGroupCancellationBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "sleep 10 & wait")
	configureOpenAPIWorkerCommand(cmd)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("worker must run in its own Linux process group")
	}
	start := time.Now()
	err := cmd.Run()
	if err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected process-group cancellation, got err=%v ctx=%v", err, ctx.Err())
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("cancellation waited too long for subprocess descendants: %s", elapsed)
	}
}

// WaitDelay also bounds a normally-exited parent whose descendant keeps
// an inherited stdout/stderr pipe open; clean up the test subprocess group.
func TestOpenAPIWorkerInheritedPipeWaitBounded(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 10 &")
	configureOpenAPIWorkerCommand(cmd)
	cmd.WaitDelay = 200 * time.Millisecond
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	start := time.Now()
	err := cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("expected bounded-pipe error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("orphaned inherited pipe blocked subprocess Wait: %s", elapsed)
	}
}

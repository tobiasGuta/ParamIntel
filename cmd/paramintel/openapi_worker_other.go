//go:build !linux

package main

import (
	"os/exec"
	"time"
)

func applyOpenAPIWorkerLimits(memoryMiB int, cpuSeconds int64) error {
	return ErrIsolatedOpenAPIUnavailable
}

// Non-Linux opt-in worker execution is disabled, but preserve a bounded
// fallback for cross-platform command construction.
func configureOpenAPIWorkerCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}

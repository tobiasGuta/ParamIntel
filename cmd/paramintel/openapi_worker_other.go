//go:build !linux

package main

func applyOpenAPIWorkerLimits(memoryMiB int, cpuSeconds int64) error {
	return ErrIsolatedOpenAPIUnavailable
}

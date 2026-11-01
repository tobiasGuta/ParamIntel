//go:build linux

package main

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
)

func requireMmapDataLimitSupport() error {
	var u syscall.Utsname
	if err := syscall.Uname(&u); err != nil {
		return fmt.Errorf("read Linux kernel version: %w", err)
	}
	var release []byte
	for _, c := range u.Release {
		if c == 0 {
			break
		}
		release = append(release, byte(c))
	}
	parts := strings.SplitN(string(release), ".", 3)
	if len(parts) < 2 {
		return fmt.Errorf("unrecognized Linux version %q", string(release))
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || major < 4 || (major == 4 && minor < 7) {
		return fmt.Errorf("Linux 4.7+ is required for RLIMIT_DATA mmap protection (running %q)", string(release))
	}
	return nil
}

func limitWorkerResource(kind int, requested uint64) error {
	var original syscall.Rlimit
	if err := syscall.Getrlimit(kind, &original); err != nil {
		return fmt.Errorf("inspect kernel resource limit: %w", err)
	}
	effective := requested
	if original.Cur < effective {
		effective = original.Cur
	}
	if original.Max < effective {
		effective = original.Max
	}
	if effective == 0 {
		return fmt.Errorf("kernel resource limit is zero")
	}
	if err := syscall.Setrlimit(kind, &syscall.Rlimit{Cur: effective, Max: effective}); err != nil {
		return fmt.Errorf("apply kernel resource limit: %w", err)
	}
	var actual syscall.Rlimit
	if err := syscall.Getrlimit(kind, &actual); err != nil {
		return fmt.Errorf("verify kernel resource limit: %w", err)
	}
	if actual.Cur > requested || actual.Max > requested {
		return fmt.Errorf("kernel resource ceiling was not enforced")
	}
	return nil
}

func applyOpenAPIWorkerLimits(memoryMiB int, cpuSeconds int64) error {
	if memoryMiB < minOpenAPIMemoryMiB || memoryMiB > maxOpenAPIMemoryMiB || cpuSeconds < 1 || cpuSeconds > 65 {
		return fmt.Errorf("invalid parser worker resource configuration")
	}
	if err := requireMmapDataLimitSupport(); err != nil {
		return err
	}
	if err := limitWorkerResource(syscall.RLIMIT_DATA, uint64(memoryMiB)<<20); err != nil {
		return fmt.Errorf("data/mmap limit: %w", err)
	}
	if err := limitWorkerResource(syscall.RLIMIT_CPU, uint64(cpuSeconds)); err != nil {
		return fmt.Errorf("CPU limit: %w", err)
	}
	return nil
}

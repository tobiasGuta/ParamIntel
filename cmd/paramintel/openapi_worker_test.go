package main

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

func TestOpenAPIIsolationPolicyRejectsUnsafeConfiguration(t *testing.T) {
	ok := openAPIIsolationPolicy{
		MaxBytes: schemaintel.DefaultMaxDocumentBytes,
		MemoryMiB: defaultOpenAPIMemoryMiB,
		Timeout: defaultOpenAPITimeout,
	}
	if err := ok.validate(); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
	for _, candidate := range []openAPIIsolationPolicy{
		{MaxBytes: 0, MemoryMiB: 768, Timeout: 15*time.Second},
		{MaxBytes: 65<<20, MemoryMiB: 768, Timeout: 15*time.Second},
		{MaxBytes: 1024, MemoryMiB: 255, Timeout: 15*time.Second},
		{MaxBytes: 1024, MemoryMiB: 2049, Timeout: 15*time.Second},
		{MaxBytes: 1024, MemoryMiB: 768, Timeout: time.Millisecond},
		{MaxBytes: 1024, MemoryMiB: 768, Timeout: 61*time.Second},
	} {
		if err := candidate.validate(); err == nil {
			t.Fatalf("invalid untrusted parser policy was accepted: %+v", candidate)
		}
	}
}

func TestUntrustedOpenAPIWorkerFailsClosedBeforeLaunching(t *testing.T) {
	policy := openAPIIsolationPolicy{
		MaxBytes: 100,
		MemoryMiB: 768,
		Timeout: 1*time.Second,
	}
	spec := []byte("openapi: 3.1.0\ninfo: {title: Test, version: '1'}\npaths: {}\n")
	_, err := runIsolatedOpenAPI(context.Background(), openAPIWorkerRequest{
		Phase: "bogus", Spec: spec, Limit: 100,
	}, policy)
	if runtime.GOOS != "linux" {
		if !errors.Is(err, ErrIsolatedOpenAPIUnavailable) {
			t.Fatalf("non-Linux untrusted isolation must fail closed: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "phase") {
		t.Fatalf("invalid phase should be rejected: %v", err)
	}
	_, err = runIsolatedOpenAPI(context.Background(), openAPIWorkerRequest{
		Phase: "validate", Spec: spec, Limit: 101,
	}, policy)
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("unmatched limit should be rejected: %v", err)
	}
	_, err = runIsolatedOpenAPI(context.Background(), openAPIWorkerRequest{
		Phase: "validate", Spec: make([]byte, 101), Limit: 100,
	}, policy)
	if !errors.Is(err, schemaintel.ErrDocumentTooLarge) {
		t.Fatalf("oversized request should be rejected without a worker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = runIsolatedOpenAPI(ctx, openAPIWorkerRequest{
		Phase: "validate", Spec: spec, Limit: 100,
	}, policy)
	if !errors.Is(err, ErrIsolatedOpenAPIFailed) {
		t.Fatalf("canceled worker must fail closed: %v", err)
	}
}

func TestOpenAPIWorkerOutputCap(t *testing.T) {
	b := &cappedOutput{max: 4}
	if n, err := b.Write([]byte("abcd")); n != 4 || err != nil {
		t.Fatalf("cap boundary n=%d err=%v", n, err)
	}
	if n, err := b.Write([]byte("x")); n != 0 || err == nil {
		t.Fatalf("worker over-cap n=%d err=%v", n, err)
	}
	if b.String() != "abcd" {
		t.Fatalf("output buffer mutated after oversized write: %q", b.String())
	}
}

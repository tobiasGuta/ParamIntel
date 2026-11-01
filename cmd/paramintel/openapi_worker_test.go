package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

func TestOpenAPIIsolationPolicyRejectsUnsafeConfiguration(t *testing.T) {
	ok := openAPIIsolationPolicy{
		MaxBytes:  schemaintel.DefaultMaxDocumentBytes,
		MemoryMiB: defaultOpenAPIMemoryMiB,
		Timeout:   defaultOpenAPITimeout,
	}
	if err := ok.validate(); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
	for _, candidate := range []openAPIIsolationPolicy{
		{MaxBytes: 0, MemoryMiB: 768, Timeout: 15 * time.Second},
		{MaxBytes: 65 << 20, MemoryMiB: 768, Timeout: 15 * time.Second},
		{MaxBytes: 1024, MemoryMiB: 255, Timeout: 15 * time.Second},
		{MaxBytes: 1024, MemoryMiB: 2049, Timeout: 15 * time.Second},
		{MaxBytes: 1024, MemoryMiB: 768, Timeout: time.Millisecond},
		{MaxBytes: 1024, MemoryMiB: 768, Timeout: 61 * time.Second},
	} {
		if err := candidate.validate(); err == nil {
			t.Fatalf("invalid untrusted parser policy was accepted: %+v", candidate)
		}
	}
}

func TestUntrustedOpenAPIWorkerFailsClosedBeforeLaunching(t *testing.T) {
	policy := openAPIIsolationPolicy{
		MaxBytes:  100,
		MemoryMiB: 768,
		Timeout:   1 * time.Second,
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

func TestUntrustedWorkerReceivesOnlyStructuralData(t *testing.T) {
	request := openAPIWorkerRequest{
		Phase: "analyze",
		Template: model.RequestTemplate{
			Method: "POST",
			URL:    "https://private.example.test/users/42?session=private-query-token",
			Headers: http.Header{
				"Content-Type":  {"application/json"},
				"Authorization": {"Bearer secret-auth-token"},
				"Cookie":        {"sid=secret-cookie"},
			},
			Body: []byte(`{"profile":{"name":"sensitive-name","settings":{"enabled":true}},"secret_scalar":91234,"items":["secret-array-value"]}`),
		},
		Baseline: model.BaselineProfile{
			StatusCode: 200, StatusStable: true,
			ContentType: "application/json", ContentTypeStable: true,
		},
	}
	if err := minimizeWorkerAnalysisContext(&request); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(out)
	for _, secret := range []string{"secret-auth-token", "secret-cookie", "private-query-token", "private.example.test", "sensitive-name", "secret-array-value", "91234"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("worker input contains sensitive value %q", secret)
		}
	}
	if request.Template.URL != "https://schema.invalid/users/42" {
		t.Fatalf("worker URL not minimized: %q", request.Template.URL)
	}
	if request.Template.Headers.Get("Content-Type") != "application/json" || request.Template.Headers.Get("Authorization") != "" {
		t.Fatalf("worker request headers contain unnecessary data: %+v", request.Template.Headers)
	}
	var result map[string]any
	if err := json.Unmarshal(request.Template.Body, &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["profile"].(map[string]any); !ok {
		t.Fatalf("JSON object nesting not preserved: %+v", result)
	}
	if _, ok := result["items"]; !ok {
		t.Fatalf("property paths not preserved: %+v", result)
	}
}

func TestIsolatedOpenAPIWorkerEnvironmentDoesNotInheritCredentials(t *testing.T) {
	t.Setenv("PARAMINTEL_FAKE_SECRET_TOKEN", "sensitive-scanner-token")
	policy := openAPIIsolationPolicy{
		MaxBytes:  schemaintel.DefaultMaxDocumentBytes,
		MemoryMiB: 768,
		Timeout:   15 * time.Second,
	}
	values := isolatedOpenAPIWorkerEnvironment(policy, 17)
	if len(values) != 4 {
		t.Fatalf("worker env contains unexpected entries: %+v", values)
	}
	for _, entry := range values {
		if strings.Contains(entry, "sensitive-scanner-token") ||
			strings.Contains(entry, "PARAMINTEL_FAKE_SECRET_TOKEN") {
			t.Fatalf("parser child inherited scanner secret: %q", entry)
		}
	}
	for _, want := range []string{
		"PARAMINTEL_INTERNAL_OPENAPI_MEMORY_MIB=768",
		"PARAMINTEL_INTERNAL_OPENAPI_CPU_SECONDS=17",
		"GOMEMLIMIT=384MiB", "GOGC=50",
	} {
		found := false
		for _, entry := range values {
			if entry == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("worker resource setting missing: %q", want)
		}
	}
}

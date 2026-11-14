package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestValidateExpectedBaselineStatus(t *testing.T) {
	for _, status := range []int{0, 200, 201, 204, 401, 429, 599} {
		if err := validateExpectedBaselineStatus(status); err != nil {
			t.Fatalf("status %d rejected: %v", status, err)
		}
	}
	for _, status := range []int{-1, 1, 99, 600} {
		if err := validateExpectedBaselineStatus(status); err == nil {
			t.Fatalf("status %d should be rejected before network requests", status)
		}
	}
}

func TestCheckExpectedBaselineStatus(t *testing.T) {
	stableUnauthorized := model.BaselineProfile{Samples: 3, StatusCode: http.StatusUnauthorized, StatusStable: true}
	if err := checkExpectedBaselineStatus(0, stableUnauthorized); err != nil {
		t.Fatalf("disabled guard must preserve existing baseline behavior: %v", err)
	}
	if err := checkExpectedBaselineStatus(http.StatusUnauthorized, stableUnauthorized); err != nil {
		t.Fatalf("explicitly expected 401 should be allowed: %v", err)
	}
	if err := checkExpectedBaselineStatus(http.StatusOK, stableUnauthorized); err == nil || !strings.Contains(err.Error(), "expected HTTP 200, observed stable HTTP 401") {
		t.Fatalf("expected an actionable auth-state mismatch diagnostic, got %v", err)
	}
	unstable := model.BaselineProfile{Samples: 3, StatusCode: http.StatusOK, StatusStable: false}
	if err := checkExpectedBaselineStatus(http.StatusOK, unstable); err == nil || !strings.Contains(err.Error(), "unstable") {
		t.Fatalf("an unstable baseline must not satisfy an expected status: %v", err)
	}
	stableOK := model.BaselineProfile{Samples: 3, StatusCode: http.StatusOK, StatusStable: true}
	if err := checkExpectedBaselineStatus(http.StatusOK, stableOK); err != nil {
		t.Fatalf("expected stable 200 should pass: %v", err)
	}
}

func TestCLIExpectedStatusMismatchDoesNotProbeOrWriteReport(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.txt")
	reportPath := filepath.Join(dir, "findings.json")
	raw := "GET " + srv.URL + " HTTP/1.1\r\nHost: " + target.Host + "\r\n\r\n"
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "-request", requestPath, "-baseline", "3", "-expect-status", "200", "-output", reportPath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected status mismatch failure; output:\n%s", output)
	}
	if !strings.Contains(string(output), "expected HTTP 200, observed stable HTTP 401") {
		t.Fatalf("missing explicit auth-state mismatch diagnostic:\n%s", output)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("expected exactly 3 baseline requests and no discovery probes, got %d", got)
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatalf("normal findings report must not be written after a status mismatch: %v", err)
	}
}

package main

import (
	"encoding/json"
	"fmt"
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

func TestBaselineStatusEvidenceSerialization(t *testing.T) {
	profile := model.BaselineProfile{
		Samples:         3,
		StatusCode:      http.StatusUnauthorized,
		StatusStable:    true,
		BodyLenMin:      25,
		BodyLenMax:      25,
		StableJSONPaths: map[string]string{"$.error": "s:unauthorized"},
	}
	tests := []struct {
		name       string
		expected   int
		wantResult string
		wantCode   bool
	}{
		{"no operator expectation", 0, statusExpectationNotConfigured, false},
		{"intentional 401", http.StatusUnauthorized, statusExpectationMatched, true},
		{"unexpected 401", http.StatusOK, statusExpectationMismatched, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := buildBaselineSummary(profile, tt.expected)
			raw, err := json.Marshal(summary)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["samples"] != float64(3) || decoded["stable_json_paths"] != float64(1) ||
				decoded["body_len_min"] != float64(25) || decoded["body_len_max"] != float64(25) {
				t.Fatalf("existing baseline report fields changed: %s", raw)
			}
			status, ok := decoded["status_evidence"].(map[string]any)
			if !ok {
				t.Fatalf("status evidence not an object: %s", raw)
			}
			if status["first_observed_status_code"] != float64(401) || status["status_stable"] != true ||
				status["expectation"] != tt.wantResult {
				t.Fatalf("unexpected status provenance: %s", raw)
			}
			value, present := status["expected_status_code"]
			if present != tt.wantCode {
				t.Fatalf("expected_status_code presence=%t want=%t: %s", present, tt.wantCode, raw)
			}
			if present && value != float64(tt.expected) {
				t.Fatalf("expected_status_code=%v want=%d", value, tt.expected)
			}
		})
	}
	unstable := profile
	unstable.StatusStable = false
	evidence := buildBaselineSummary(unstable, http.StatusOK).StatusEvidence
	if evidence.Expectation != statusExpectationUnstable || evidence.StatusStable {
		t.Fatalf("unstable status incorrectly admitted: %+v", evidence)
	}
	// Even though the guard rejects this case before producing a normal report,
	// the summary helper must not claim a status match.
}

func TestCLIBaselineStatusEvidence(t *testing.T) {
	tests := []struct {
		name             string
		token            string
		expected         int
		wantObserved     int
		wantExpectation  string
		wantFailure      bool
	}{
		{"valid token and expected 200", "lab-valid", 200, 200, statusExpectationMatched, false},
		{"expired token and expected 401", "lab-expired", 401, 401, statusExpectationMatched, false},
		{"expired token without expectation", "lab-expired", 0, 401, statusExpectationNotConfigured, false},
		{"expired token but expected 200", "lab-expired", 200, 401, statusExpectationMismatched, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				if r.Header.Get("Authorization") != "Bearer lab-valid" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
					return
				}
				_, _ = w.Write([]byte(`{"user":"lab-user","role":"member"}`))
			}))
			defer srv.Close()
			target, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			requestPath := filepath.Join(dir, "request.txt")
			reportPath := filepath.Join(dir, "report.json")
			raw := fmt.Sprintf("GET /account HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nAccept: application/json\r\n\r\n", target.Host, tt.token)
			if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{
				"run", ".",
				"-request", requestPath,
				"-scheme", "http",
				"-locations", "query",
				"-baseline", "3",
				"-trials", "2",
				"-characterize=false",
				"-value-aware=false",
				"-output", reportPath,
			}
			// Exercise the CLI's actual default when no expectation is set.
			if tt.expected != 0 {
				args = append(args, "-expect-baseline-status", fmt.Sprint(tt.expected))
			}
			cmd := exec.Command("go", args...)
			combined, runErr := cmd.CombinedOutput()
			if tt.wantFailure {
				if runErr == nil || !strings.Contains(string(combined), "stopping before discovery") ||
					!strings.Contains(string(combined), "HTTP 401 does not match expected HTTP 200") {
					t.Fatalf("expected baseline mismatch, got err=%v output=%s", runErr, combined)
				}
				if got := requests.Load(); got != 3 {
					t.Fatalf("expected only 3 baseline requests before failure, got %d", got)
				}
				if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
					t.Fatalf("failed scan must not emit normal findings JSON: %v", err)
				}
				return
			}
			if runErr != nil {
				t.Fatalf("CLI returned %v: %s", runErr, combined)
			}
			reportBytes, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatal(err)
			}
			var report model.ScanReport
			if err := json.Unmarshal(reportBytes, &report); err != nil {
				t.Fatalf("decode report: %v\n%s", err, reportBytes)
			}
			if report.Baseline.StatusEvidence.FirstObservedStatusCode != tt.wantObserved ||
				!report.Baseline.StatusEvidence.StatusStable ||
				report.Baseline.StatusEvidence.Expectation != tt.wantExpectation ||
				report.Baseline.Samples != 3 {
				t.Fatalf("incorrect baseline audit: %+v", report.Baseline)
			}
			gotExpected := report.Baseline.StatusEvidence.ExpectedStatusCode
			if tt.expected == 0 && gotExpected != nil {
				t.Fatalf("unexpected declared status: %v", *gotExpected)
			}
			if tt.expected != 0 && (gotExpected == nil || *gotExpected != tt.expected) {
				t.Fatalf("missing explicit status expectation in report: %+v", report.Baseline)
			}
			if len(report.Parameters) != 0 {
				t.Fatalf("fixed-response API must not produce parameter discoveries: %+v", report.Parameters)
			}
			if got := requests.Load(); got <= 3 {
				t.Fatalf("discovery did not proceed after accepted baseline, requests=%d", got)
			}
			// The audit must not copy bearer credentials into report status data.
			statusBytes, err := json.Marshal(report.Baseline.StatusEvidence)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(statusBytes), "Bearer") || strings.Contains(string(statusBytes), tt.token) {
				t.Fatalf("sensitive request metadata in status audit: %s", statusBytes)
			}
		})
	}
}

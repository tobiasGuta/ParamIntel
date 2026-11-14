package main

import (
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
)

// An authenticated baseline must not turn into a normal findings report
// when the server begins rejecting the unchanged captured request mid-scan.
func TestCLIConfirmedAuthDriftStopsBeforeReport(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 {
			_, _ = w.Write([]byte(`{"user":"lab-member"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"session no longer valid"}`))
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil { t.Fatal(err) }
	dir := t.TempDir()
	reqPath := filepath.Join(dir, "request.txt")
	reportPath := filepath.Join(dir, "report.json")
	raw := fmt.Sprintf("GET /account HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer lab-secret-not-for-logs\r\nAccept: application/json\r\n\r\n", target.Host)
	if err := os.WriteFile(reqPath, []byte(raw), 0600); err != nil { t.Fatal(err) }

	cmd := exec.Command("go", "run", ".",
		"-request", reqPath,
		"-scheme", "http",
		"-locations", "query",
		"-baseline", "3",
		"-expect-baseline-status", "200",
		"-characterize=false",
		"-value-aware=false",
		"-output", reportPath,
	)
	output, err := cmd.CombinedOutput()
	if err == nil { t.Fatalf("expected fail-closed status drift, output=%s", output) }
	text := string(output)
	if !strings.Contains(text, "baseline status drift (confirmed)") ||
		!strings.Contains(text, "clean baseline rechecks [401 401]") {
		t.Fatalf("missing typed diagnostic for confirmed drift: %s", text)
	}
	if strings.Contains(text, "lab-secret-not-for-logs") {
		t.Fatalf("operator bearer token leaked into error: %s", text)
	}
	if got := requests.Load(); got != 6 {
		t.Fatalf("expected baseline3 + probe1 + clean2, got %d", got)
	}
	if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("invalid session must not produce normal findings report: %v", statErr)
	}
}

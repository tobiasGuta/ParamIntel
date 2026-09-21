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
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestCLIEvidenceGuidedRescueUsesContextWithoutAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("format") == "json" {
			fmt.Fprint(w, `{"items":[],"supported_formats":["json","csv"],"selected_format":"json"}`)
			return
		}
		fmt.Fprint(w, `{"items":[],"supported_formats":["json","csv"]}`)
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	outputPath := filepath.Join(tmp, "report.json")
	raw := fmt.Sprintf("GET %s/api/items HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", srv.URL, target.Host)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-scheme", "http",
		"-locations", "query",
		"-baseline", "3",
		"-trials", "3",
		"-chunk", "64",
		"-characterize=false",
		"-value-aware=true",
		"-value-aware-budget", "8",
		"-output", outputPath,
	)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ParamIntel CLI failed: %v\n%s", err, combined)
	}

	rawReport, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var report model.ScanReport
	if err := json.Unmarshal(rawReport, &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, rawReport)
	}

	if len(report.Parameters) != 1 || report.Parameters[0].Name != "format" {
		t.Fatalf("parameters=%+v\nCLI:\n%s", report.Parameters, combined)
	}
	if report.Parameters[0].DiscoveryMode != "value_aware" || report.Parameters[0].DiscoveryValue != "json" {
		t.Fatalf("finding=%+v", report.Parameters[0])
	}
	if report.ValueAware == nil {
		t.Fatal("missing value-aware summary")
	}
	if report.ValueAware.Budget != 8 || report.ValueAware.RequestsUsed != 8 {
		t.Fatalf("value-aware summary=%+v", report.ValueAware)
	}
	if report.ValueAware.VerifiedRequests != 8 || report.ValueAware.MissRequests != 0 || report.ValueAware.BudgetExhaustedRequests != 0 {
		t.Fatalf("request outcome accounting=%+v", report.ValueAware)
	}
	if report.ValueAware.CandidatesAttempted != 1 {
		t.Fatalf("attempted=%d want=1 audit=%+v", report.ValueAware.CandidatesAttempted, report.ValueAware.CandidateAudit)
	}
	if report.ValueAware.EligibleCandidates <= 1 || report.ValueAware.CandidatesDeferred != report.ValueAware.EligibleCandidates-1 {
		t.Fatalf("eligibility/deferred summary=%+v", report.ValueAware)
	}
	if len(report.ValueAware.CandidateAudit) != 1 {
		t.Fatalf("audit=%+v", report.ValueAware.CandidateAudit)
	}
	audit := report.ValueAware.CandidateAudit[0]
	if audit.Name != "format" || audit.EvidenceTier != "B" || audit.ContextRelevance != 80 {
		t.Fatalf("unexpected first rescue audit=%+v", audit)
	}
	if audit.AIQueried || audit.RequestsUsed != 8 || audit.Outcome != "verified" {
		t.Fatalf("unexpected rescue accounting=%+v", audit)
	}
}

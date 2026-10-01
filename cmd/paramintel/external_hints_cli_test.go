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
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/externalhints"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestCLIExternalValueHintIsVerifiedWithoutInternalAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("deployment_lane") == "internal" {
			fmt.Fprint(w, `{"state":"internal"}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	hintsPath := filepath.Join(tmp, "hints.json")
	outputPath := filepath.Join(tmp, "report.json")

	rawRequest := "GET " + srv.URL + "/users HTTP/1.1\r\nHost: " + target.Host + "\r\n\r\n"
	if err := os.WriteFile(requestPath, []byte(rawRequest), 0o600); err != nil {
		t.Fatal(err)
	}
	hints := externalhints.Document{
		Context: []string{"user administration"},
		Candidates: []externalhints.CandidateHint{{
			Name:     "deployment_lane",
			Location: model.LocationQuery,
			Priority: 100,
			Reason:   "application deployment state",
			Values: []externalhints.ValueHint{{
				Value:    "internal",
				Kind:     "string",
				Priority: 100,
				Reason:   "plausible restricted deployment state",
			}},
		}},
	}
	hintsRaw, err := json.Marshal(hints)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hintsPath, hintsRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-hints", hintsPath,
		"-baseline", "2",
		"-trials", "2",
		"-value-aware-budget", "12",
		"-characterize=false",
		"-output", outputPath,
	)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI failed: %v\n%s", err, combined)
	}
	if strings.Contains(string(combined), "AI Candidate Advisor") || strings.Contains(string(combined), "AI Semantic Value Advisor") {
		t.Fatalf("external-hint run unexpectedly used internal AI:\n%s", combined)
	}

	reportRaw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var report model.ScanReport
	if err := json.Unmarshal(reportRaw, &report); err != nil {
		t.Fatal(err)
	}
	if report.AIAdvisor != nil || report.AIValueAdvisor != nil {
		t.Fatalf("internal AI summaries must be absent: %+v", report)
	}
	if len(report.Parameters) != 1 {
		t.Fatalf("parameters=%+v want one externally rescued finding\nCLI:\n%s", report.Parameters, combined)
	}
	finding := report.Parameters[0]
	if finding.Name != "deployment_lane" || finding.Location != model.LocationQuery {
		t.Fatalf("finding=%+v", finding)
	}
	if finding.DiscoveryMode != "external_value_hint" || finding.DiscoveryValue != "internal" || finding.DiscoveryValueKind != "string" {
		t.Fatalf("external value provenance=%+v", finding)
	}
	if finding.CandidateChanged != 2 || finding.RandomControlChanged != 0 {
		t.Fatalf("verification=%d/%d control=%d/%d", finding.CandidateChanged, finding.CandidateTrials, finding.RandomControlChanged, finding.RandomControlTrials)
	}
	if !hasCandidateSource(finding.CandidateSources, externalhints.SourceExternalSemanticHint) {
		t.Fatalf("external candidate provenance missing: %+v", finding.CandidateSources)
	}
	if report.ValueAware == nil || len(report.ValueAware.CandidateAudit) == 0 {
		t.Fatalf("value-aware audit missing: %+v", report.ValueAware)
	}
	foundExternalAudit := false
	for _, audit := range report.ValueAware.CandidateAudit {
		if audit.Name == "deployment_lane" {
			foundExternalAudit = true
			if audit.SemanticSource != "external_hint" || audit.AIQueried {
				t.Fatalf("external audit=%+v", audit)
			}
		}
	}
	if !foundExternalAudit {
		t.Fatalf("external rescue audit not found: %+v", report.ValueAware.CandidateAudit)
	}
}

func hasCandidateSource(sources []model.CandidateSource, want string) bool {
	for _, source := range sources {
		if source.Source == want {
			return true
		}
	}
	return false
}

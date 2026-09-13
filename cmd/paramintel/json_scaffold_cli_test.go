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

func TestCLIJSONScaffoldFindsResponseDerivedNestedField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		settings, _ := profile["settings"].(map[string]any)
		if _, ok := settings["beta_access"]; ok {
			fmt.Fprint(w, `{"state":"beta"}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	}))
	defer srv.Close()

	report, combined := runJSONScaffoldCLI(t, srv, `{"profile":{"name":"tobias","settings":{"beta_access":false}}}`)
	if len(report.Parameters) != 1 {
		t.Fatalf("parameters=%+v want exactly one scaffold finding\nCLI:\n%s", report.Parameters, combined)
	}
	r := report.Parameters[0]
	if r.Name != "beta_access" || r.JSONPath != "$.profile.settings.beta_access" {
		t.Fatalf("unexpected scaffold finding: %+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 {
		t.Fatalf("verification=%d/%d control=%d/%d", r.CandidateChanged, r.CandidateTrials, r.RandomControlChanged, r.RandomControlTrials)
	}
	if len(r.CandidateSources) != 1 || r.CandidateSources[0].Source != "context_response_scaffoldable_json_property" {
		t.Fatalf("candidate sources=%+v", r.CandidateSources)
	}
}

func TestCLIJSONScaffoldRejectsSharedParentBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		if settings, ok := profile["settings"].(map[string]any); ok && len(settings) > 0 {
			fmt.Fprint(w, `{"state":"settings-seen"}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	}))
	defer srv.Close()

	report, combined := runJSONScaffoldCLI(t, srv, `{"profile":{"name":"tobias","settings":{"beta_access":false}}}`)
	if len(report.Parameters) != 0 {
		t.Fatalf("shared scaffold behavior must be rejected by paired random-name control: %+v\nCLI:\n%s", report.Parameters, combined)
	}
}

func runJSONScaffoldCLI(t *testing.T, srv *httptest.Server, contextBody string) (model.ScanReport, string) {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	contextPath := filepath.Join(tmp, "context.json")
	outputPath := filepath.Join(tmp, "findings.json")
	body := `{"profile":{"name":"tobias"}}`
	raw := fmt.Sprintf("POST %s/api/profile HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", srv.URL, target.Host, len(body), body)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contextPath, []byte(contextBody), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-scheme", "http",
		"-locations", "json",
		"-context-response", contextPath,
		"-json-scaffold",
		"-allow-state-changing",
		"-baseline", "3",
		"-trials", "3",
		"-chunk", "8",
		"-characterize=false",
		"-value-aware=false",
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
	return report, string(combined)
}

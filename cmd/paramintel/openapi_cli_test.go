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
	"github.com/tobiasGuta/ParamIntel/internal/schemaintel"
)

func TestCLIOpenAPIFindsExistingParentResponseOnlyField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		if _, ok := profile["beta_access"]; ok {
			fmt.Fprint(w, `{"state":"beta"}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	}))
	defer srv.Close()

	report, combined := runOpenAPICLI(t, srv, openAPISpecExistingParent)
	if len(report.Parameters) != 1 {
		t.Fatalf("parameters=%+v want exactly one OpenAPI finding\nCLI:\n%s", report.Parameters, combined)
	}
	finding := report.Parameters[0]
	if finding.Name != "beta_access" || finding.JSONPath != "$.profile.beta_access" {
		t.Fatalf("unexpected OpenAPI finding: %+v", finding)
	}
	if finding.CandidateChanged != 3 || finding.RandomControlChanged != 0 {
		t.Fatalf("verification=%d/%d control=%d/%d", finding.CandidateChanged, finding.CandidateTrials, finding.RandomControlChanged, finding.RandomControlTrials)
	}
	if len(finding.CandidateSources) != 1 {
		t.Fatalf("candidate sources=%+v", finding.CandidateSources)
	}
	source := finding.CandidateSources[0]
	if source.Source != schemaintel.SourceResponseOnlyJSONProperty || source.Path != "$.profile.beta_access" {
		t.Fatalf("unexpected source=%+v", source)
	}
	if !source.ReadOnly || len(source.DeclaredTypes) != 1 || source.DeclaredTypes[0] != "boolean" {
		t.Fatalf("schema metadata not preserved: %+v", source)
	}
}

func TestCLIOpenAPIWithholdsScaffoldDescriptor(t *testing.T) {
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

	report, combined := runOpenAPICLI(t, srv, openAPISpecScaffold)
	if len(report.Parameters) != 0 {
		t.Fatalf("OpenAPI scaffold descriptor must remain inactive in Slice 2: %+v\nCLI:\n%s", report.Parameters, combined)
	}
}

func runOpenAPICLI(t *testing.T, srv *httptest.Server, specTemplate string) (model.ScanReport, string) {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	openAPIPath := filepath.Join(tmp, "openapi.yaml")
	outputPath := filepath.Join(tmp, "findings.json")
	body := `{"profile":{"name":"tobias"}}`
	raw := fmt.Sprintf("POST %s/api/profile HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", srv.URL, target.Host, len(body), body)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openAPIPath, []byte(specTemplate), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-scheme", "http",
		"-locations", "json",
		"-openapi", openAPIPath,
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

const openAPISpecExistingParent = `openapi: 3.1.0
info:
  title: ParamIntel Slice 2 test
  version: 1.0.0
paths:
  /api/profile:
    post:
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                profile:
                  type: object
                  properties:
                    name:
                      type: string
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  profile:
                    type: object
                    properties:
                      name:
                        type: string
                      beta_access:
                        type: boolean
                        readOnly: true
`

const openAPISpecScaffold = `openapi: 3.1.0
info:
  title: ParamIntel Slice 2 scaffold boundary test
  version: 1.0.0
paths:
  /api/profile:
    post:
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                profile:
                  type: object
                  properties:
                    name:
                      type: string
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  profile:
                    type: object
                    properties:
                      name:
                        type: string
                      settings:
                        type: object
                        properties:
                          beta_access:
                            type: boolean
                            readOnly: true
`

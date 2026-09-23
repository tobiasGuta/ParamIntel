package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestCLIOpenAPIBodylessGETUsesResponseIntelligenceOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"role":"ROLE_USER","available_credit":155}`)
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	openAPIPath := filepath.Join(tmp, "openapi.yaml")
	outputPath := filepath.Join(tmp, "findings.json")

	raw := fmt.Sprintf("GET %s/dashboard HTTP/1.1\r\nHost: %s\r\nAccept: application/json\r\nConnection: close\r\n\r\n", srv.URL, target.Host)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openAPIPath, []byte(openAPISpecBodylessGET), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-scheme", "http",
		"-locations", "query",
		"-openapi", openAPIPath,
		"-baseline", "3",
		"-trials", "3",
		"-chunk", "8",
		"-characterize=false",
		"-value-aware=false",
		"-verbose",
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
	if len(report.Parameters) != 0 {
		t.Fatalf("bodyless GET must not admit JSON findings: %+v\nCLI:\n%s", report.Parameters, combined)
	}
	output := string(combined)
	if !strings.Contains(output, "operation: GET /dashboard") {
		t.Fatalf("missing matched operation in CLI:\n%s", output)
	}
	if !strings.Contains(output, "request media type: <none> (bodyless request)") {
		t.Fatalf("missing bodyless request marker in CLI:\n%s", output)
	}
	if !strings.Contains(output, "response-only descriptors: 0") {
		t.Fatalf("bodyless GET should not admit response-only JSON descriptors as candidates:\n%s", output)
	}
}

const openAPISpecBodylessGET = `openapi: 3.0.1
info:
  title: bodyless get
  version: 1.0.0
paths:
  /dashboard:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  role:
                    type: string
                  available_credit:
                    type: number
`

const openAPISpec321Query = `openapi: 3.2.1
info:
  title: Search API Query Integration Test
  version: 1.0.0
paths:
  /api/search:
    query:
      operationId: executeSearch
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                search:
                  type: object
                  properties:
                    term:
                      type: string
              required:
                - search
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                properties:
                  search:
                    type: object
                    properties:
                      term:
                        type: string
                      highlight:
                        type: boolean
                        readOnly: true
                  results:
                    type: array
                    items:
                      type: object
                  total:
                    type: integer
`

// TestCLIOpenAPI_QueryMethod_RequiresAuthorizationGate verifies that a QUERY request
// is conservatively treated as state-changing by default, requiring -allow-state-changing.
func TestCLIOpenAPI_QueryMethod_RequiresAuthorizationGate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	openAPIPath := filepath.Join(tmp, "openapi.yaml")
	outputPath := filepath.Join(tmp, "findings.json")

	body := `{"search":{"term":"antigravity"}}`
	raw := fmt.Sprintf("QUERY %s/api/search HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		srv.URL, target.Host, len(body), body)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openAPIPath, []byte(openAPISpec321Query), 0600); err != nil {
		t.Fatal(err)
	}

	// Run WITHOUT -allow-state-changing
	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-scheme", "http",
		"-locations", "json",
		"-openapi", openAPIPath,
		"-output", outputPath,
	)
	combined, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("CLI unexpectedly succeeded without -allow-state-changing for QUERY method\nOutput:\n%s", combined)
	}
	output := string(combined)
	if !strings.Contains(output, "request method QUERY may be state-changing") {
		t.Fatalf("expected state-changing authorization error message, got:\n%s", output)
	}
	if !strings.Contains(output, "-allow-state-changing") {
		t.Fatalf("expected -allow-state-changing hint in error message, got:\n%s", output)
	}
}

// TestCLIOpenAPI_QueryMethod_FindsResponseOnlyCandidate_EndToEnd verifies end-to-end
// discovery with QUERY method, OpenAPI 3.2.1 candidate derivation, request property exclusion,
// and candidate/control verification.
func TestCLIOpenAPI_QueryMethod_FindsResponseOnlyCandidate_EndToEnd(t *testing.T) {
	var queryCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "QUERY" {
			http.Error(w, fmt.Sprintf("unexpected method: %s", r.Method), http.StatusMethodNotAllowed)
			return
		}
		atomic.AddInt64(&queryCount, 1)
		w.Header().Set("Content-Type", "application/json")

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		search, _ := body["search"].(map[string]any)
		// Signal response when the response-only candidate "highlight" is present
		if _, ok := search["highlight"]; ok {
			fmt.Fprint(w, `{"state":"highlight_active","results":[],"total":0}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal","results":[],"total":0}`)
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	openAPIPath := filepath.Join(tmp, "openapi.yaml")
	outputPath := filepath.Join(tmp, "findings.json")

	body := `{"search":{"term":"antigravity"}}`
	raw := fmt.Sprintf("QUERY %s/api/search HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		srv.URL, target.Host, len(body), body)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openAPIPath, []byte(openAPISpec321Query), 0600); err != nil {
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

	if report.Method != "QUERY" {
		t.Fatalf("report.Method=%q want QUERY", report.Method)
	}
	if len(report.Parameters) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v\nOutput:\n%s", len(report.Parameters), report.Parameters, combined)
	}

	finding := report.Parameters[0]
	if finding.Name != "highlight" || finding.JSONPath != "$.search.highlight" {
		t.Fatalf("unexpected finding: %+v", finding)
	}
	if finding.CandidateChanged != 3 || finding.RandomControlChanged != 0 {
		t.Fatalf("verification=%d/%d control=%d/%d", finding.CandidateChanged, finding.CandidateTrials, finding.RandomControlChanged, finding.RandomControlTrials)
	}
	if len(finding.CandidateSources) != 1 {
		t.Fatalf("candidate sources=%+v", finding.CandidateSources)
	}
	src := finding.CandidateSources[0]
	if src.Source != schemaintel.SourceResponseOnlyJSONProperty || src.Path != "$.search.highlight" {
		t.Fatalf("unexpected candidate source: %+v", src)
	}
	if !src.ReadOnly {
		t.Fatalf("readOnly metadata must be preserved: %+v", src)
	}
	if atomic.LoadInt64(&queryCount) == 0 {
		t.Fatal("server received no QUERY requests")
	}
}

// TestProductionClient_QueryRedirectPreservesMethodAndDoesNotForward verifies that
// ParamIntel's production client (using CheckRedirect with http.ErrUseLastResponse)
// halts on 3xx redirects without automatically rewriting QUERY to GET or following the redirect.
func TestProductionClient_QueryRedirectPreservesMethodAndDoesNotForward(t *testing.T) {
	var destinationHit int64
	destSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&destinationHit, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"destination":"reached"}`))
	}))
	defer destSrv.Close()

	var redirectServerMethod string
	var redirectServerBody string
	var redirectServerCT string

	redirectSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectServerMethod = r.Method
		redirectServerCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		redirectServerBody = string(b)
		http.Redirect(w, r, destSrv.URL, http.StatusMovedPermanently) // 301
	}))
	defer redirectSrv.Close()

	// Use the same constructor invoked by main, so production policy changes are tested.
	client := newTargetHTTPClient(15*time.Second, 0)

	bodyStr := `{"filter":"test_term"}`
	req, err := http.NewRequest("QUERY", redirectSrv.URL+"/search", strings.NewReader(bodyStr))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	defer resp.Body.Close()

	// 1. Response status must be the redirect status code (301), NOT 200
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("response status=%d; want 301 (CheckRedirect must return ErrUseLastResponse)", resp.StatusCode)
	}

	// 2. Destination server must NEVER be contacted
	if hits := atomic.LoadInt64(&destinationHit); hits != 0 {
		t.Fatalf("destination server was hit %d times; redirect must not be followed", hits)
	}

	// 3. The server that issued the redirect must have seen method QUERY with intact body and Content-Type
	if redirectServerMethod != "QUERY" {
		t.Fatalf("redirect server saw method=%q; want QUERY", redirectServerMethod)
	}
	if redirectServerCT != "application/json" {
		t.Fatalf("redirect server saw Content-Type=%q; want application/json", redirectServerCT)
	}
	if redirectServerBody != bodyStr {
		t.Fatalf("redirect server saw body=%q; want %q", redirectServerBody, bodyStr)
	}
}

// TestProductionClient_QueryPacingEnforced verifies that NewPacedTransport enforces
// pacing delays for QUERY requests.
func TestProductionClient_QueryPacingEnforced(t *testing.T) {
	var requestTimes []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestTimes = append(requestTimes, time.Now())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	delay := 30 * time.Millisecond
	client := newTargetHTTPClient(15*time.Second, delay)

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("QUERY", srv.URL+"/search", strings.NewReader(`{"filter":"pacing"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	if len(requestTimes) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requestTimes))
	}
	diff1 := requestTimes[1].Sub(requestTimes[0])
	if diff1 < delay-5*time.Millisecond {
		t.Fatalf("interval between req 0 and 1 was %v; want at least %v", diff1, delay)
	}
	diff2 := requestTimes[2].Sub(requestTimes[1])
	if diff2 < delay-5*time.Millisecond {
		t.Fatalf("interval between req 1 and 2 was %v; want at least %v", diff2, delay)
	}
}

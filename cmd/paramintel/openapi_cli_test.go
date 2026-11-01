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
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
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

func runOpenAPICLI(t *testing.T, srv *httptest.Server, specTemplate string, extraArgs ...string) (model.ScanReport, string) {
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
	cmd.Args = append(cmd.Args, extraArgs...)
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

func TestCLIOpenAPIUntrustedPreservesEvidence(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("untrusted parser subprocess uses Linux kernel limits")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		if _, ok := profile["beta_access"]; ok {
			fmt.Fprint(w, `{"state":"beta"}`)
		} else {
			fmt.Fprint(w, `{"state":"normal"}`)
		}
	}))
	defer srv.Close()

	trusted, _ := runOpenAPICLI(t, srv, openAPISpecExistingParent)
	untrusted, _ := runOpenAPICLI(t, srv, openAPISpecExistingParent,
		"-openapi-untrusted", "-openapi-parse-timeout=20s", "-openapi-memory-mib=768")
	if !reflect.DeepEqual(trusted.Parameters, untrusted.Parameters) {
		t.Fatalf("isolated parsing changed scan evidence:\ntrusted=%+v\nuntrusted=%+v", trusted.Parameters, untrusted.Parameters)
	}
}

func TestCLIOversizedOpenAPIFailsBeforeRequest(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	tmp := t.TempDir()
	rawRequest := fmt.Sprintf("GET %s/api/profile HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n", srv.URL)
	if err := os.WriteFile(filepath.Join(tmp, "request.txt"), []byte(rawRequest), 0600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(tmp, "too-large.yaml")
	if err := os.WriteFile(specPath, []byte(strings.Repeat("x", 1025)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"trusted", "untrusted"} {
		args := []string{"run", ".", "-request", filepath.Join(tmp, "request.txt"),
			"-openapi", specPath, "-openapi-max-bytes=1024"}
		if mode == "untrusted" {
			args = append(args, "-openapi-untrusted")
		}
		cmd := exec.Command("go", args...)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "exceeds configured byte limit") {
			t.Fatalf("mode=%s expected bounded rejection, got err=%v output=%s", mode, err, out)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("network requests=%d despite failing input admission", calls.Load())
	}
}

func TestCLIUntrustedMalformedOpenAPIFailsBeforeNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only isolated parser")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.txt")
	specPath := filepath.Join(dir, "invalid.yaml")
	raw := fmt.Sprintf("GET %s/resource HTTP/1.1\r\nHost: api.example.test\r\n\r\n", server.URL)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("openapi: 3.1.0\npaths: [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "-request", requestPath,
		"-openapi", specPath, "-openapi-untrusted", "-openapi-parse-timeout=15s")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "isolated openapi parser:") {
		t.Fatalf("invalid document should fail in isolated validator: err=%v output=%s", err, out)
	}
	if calls.Load() != 0 {
		t.Fatalf("malformed untrusted OpenAPI triggered %d HTTP requests", calls.Load())
	}
}

func TestCLIUntrustedRemoteReferenceDoesNotFetchBeforeNetwork(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only isolated parser")
	}
	var specServerCalls atomic.Int32
	remoteRef := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		specServerCalls.Add(1)
		fmt.Fprint(w, `{"type":"object"}`)
	}))
	defer remoteRef.Close()
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer target.Close()

	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.txt")
	specPath := filepath.Join(dir, "untrusted.yaml")
	rawRequest := fmt.Sprintf("GET %s/remote HTTP/1.1\r\nHost: api.example.test\r\n\r\n", target.URL)
	if err := os.WriteFile(requestPath, []byte(rawRequest), 0600); err != nil {
		t.Fatal(err)
	}
	spec := fmt.Sprintf(`openapi: 3.1.0
info:
  title: Remote reference must not be fetched
  version: "1"
paths:
  /remote:
    get:
      responses:
        "200":
          description: remote
          content:
            application/json:
              schema:
                $ref: "%s/remote.yaml#/components/schemas/Private"
`, remoteRef.URL)
	if err := os.WriteFile(specPath, []byte(spec), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".", "-request", requestPath, "-openapi", specPath, "-openapi-untrusted")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("remote schema reference unexpectedly accepted: %s", out)
	}
	if specServerCalls.Load() != 0 || targetCalls.Load() != 0 {
		t.Fatalf("unexpected network traffic: remote-spec=%d target=%d", specServerCalls.Load(), targetCalls.Load())
	}
}

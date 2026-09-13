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

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestCLINegativeControlRejectsSharedUnknownParameterNoise(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if len(r.URL.Query()) > 0 {
			_, _ = io.WriteString(w, "unknown parameter changed the response")
			return
		}
		_, _ = io.WriteString(w, "baseline response")
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	outputPath := filepath.Join(tmp, "findings.json")
	raw := "GET " + srv.URL + " HTTP/1.1\r\nHost: " + target.Host + "\r\n\r\n"
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
		t.Fatalf("shared unknown-parameter behavior must be rejected by the random-name control: %+v\nCLI:\n%s", report.Parameters, combined)
	}
	if !strings.Contains(string(combined), "random negative control reproduced candidate behavior") {
		t.Fatalf("expected explicit negative-control rejection diagnostics\nCLI:\n%s", combined)
	}
}

func TestCLIJSONSemanticEvidenceRemainsAuthoritative(t *testing.T) {
	var requestNumber uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddUint64(&requestNumber, 1)
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		role := "member"
		if _, ok := requestBody["role"]; ok {
			role = "admin"
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"request_id": fmt.Sprintf("request-%08d", n),
			"user": map[string]any{
				"role": role,
			},
		})
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	outputPath := filepath.Join(tmp, "findings.json")
	body := `{"name":"tobias"}`
	raw := fmt.Sprintf(
		"POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		srv.URL,
		target.Host,
		len(body),
		body,
	)
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		"go", "run", ".",
		"-request", requestPath,
		"-scheme", "http",
		"-locations", "json",
		"-baseline", "3",
		"-trials", "3",
		"-chunk", "8",
		"-characterize=false",
		"-value-aware=false",
		"-allow-state-changing",
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
	if len(report.Parameters) != 1 {
		t.Fatalf("parameters=%+v want exactly one JSON finding for role\nCLI:\n%s", report.Parameters, combined)
	}

	role, ok := findParameter(report.Parameters, "role")
	if !ok {
		t.Fatalf("role finding missing: %+v", report.Parameters)
	}
	if role.Location != model.LocationJSON || role.JSONPath != "$.role" {
		t.Fatalf("role placement=%s %s want json $.role", role.Location, role.JSONPath)
	}
	if role.CandidateChanged != 3 || role.RandomControlChanged != 0 {
		t.Fatalf("role verification=%d/%d control=%d/%d", role.CandidateChanged, role.CandidateTrials, role.RandomControlChanged, role.RandomControlTrials)
	}
	if !parameterHasEvidence(role, "json_value_changed", "$.user.role") {
		t.Fatalf("role missing JSON semantic evidence: %+v", role.Evidence)
	}
	for _, evidence := range role.Evidence {
		if strings.Contains(evidence.Path, "request_id") {
			t.Fatalf("dynamic request_id must not become semantic evidence: %+v", role.Evidence)
		}
		if evidence.Kind == "line_count" || evidence.Kind == "word_count" || evidence.Kind == "html_structure_changed" {
			t.Fatalf("v0.7 non-JSON evidence must not replace JSON semantics: %+v", role.Evidence)
		}
	}
}

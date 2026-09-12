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
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestCLIEvidenceFidelityFindsHTMLAndHeaderOnlyBehavior(t *testing.T) {
	var requestNumber uint64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddUint64(&requestNumber, 1)
		requestID := fmt.Sprintf("request-%08d", n)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// This header rotates on every response and must stay outside the evidence
		// model. It makes the fixture look more like a real dynamic application.
		w.Header().Set("X-Request-ID", requestID)
		if r.URL.Query().Has("verbose") {
			w.Header().Set("X-Debug-Mode", "enabled")
		}

		tag := "p"
		if r.URL.Query().Has("preview") {
			tag = "b"
		}
		// p and b are the same length, and requestID is fixed-width, so preview
		// changes HTML structure without changing response size, line count, or
		// word count. The body is dynamic even when no candidate is present.
		_, _ = fmt.Fprintf(w, "<html><body><main><%s>%s</%s></main></body></html>", tag, requestID, tag)
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

	if len(report.Parameters) != 2 {
		var names []string
		for _, parameter := range report.Parameters {
			names = append(names, parameter.Name)
		}
		sort.Strings(names)
		t.Fatalf("parameters=%v want exactly [preview verbose]\nCLI:\n%s", names, combined)
	}

	preview, ok := findParameter(report.Parameters, "preview")
	if !ok {
		t.Fatalf("preview finding missing: %+v", report.Parameters)
	}
	if preview.CandidateChanged != 3 || preview.RandomControlChanged != 0 {
		t.Fatalf("preview verification=%d/%d control=%d/%d", preview.CandidateChanged, preview.CandidateTrials, preview.RandomControlChanged, preview.RandomControlTrials)
	}
	if !parameterHasEvidence(preview, "html_structure_changed", "") {
		t.Fatalf("preview missing structural evidence: %+v", preview.Evidence)
	}
	if parameterHasEvidence(preview, "body_length", "") || parameterHasEvidence(preview, "line_count", "") || parameterHasEvidence(preview, "word_count", "") {
		t.Fatalf("preview must be proven by structure, not size/count drift: %+v", preview.Evidence)
	}

	verbose, ok := findParameter(report.Parameters, "verbose")
	if !ok {
		t.Fatalf("verbose finding missing: %+v", report.Parameters)
	}
	if verbose.CandidateChanged != 3 || verbose.RandomControlChanged != 0 {
		t.Fatalf("verbose verification=%d/%d control=%d/%d", verbose.CandidateChanged, verbose.CandidateTrials, verbose.RandomControlChanged, verbose.RandomControlTrials)
	}
	if !parameterHasEvidence(verbose, "header_added", "X-Debug-Mode") {
		t.Fatalf("verbose missing header-only evidence: %+v", verbose.Evidence)
	}

	serialized := string(rawReport)
	if strings.Contains(serialized, requestIDPrefixForTest()) {
		t.Fatal("rotating request IDs must not appear in findings evidence")
	}
	if strings.Contains(serialized, "enabled") {
		t.Fatal("raw custom response-header values must not appear in findings evidence")
	}
}

func findParameter(parameters []model.ParameterResult, name string) (model.ParameterResult, bool) {
	for _, parameter := range parameters {
		if parameter.Name == name {
			return parameter, true
		}
	}
	return model.ParameterResult{}, false
}

func parameterHasEvidence(parameter model.ParameterResult, kind, path string) bool {
	for _, evidence := range parameter.Evidence {
		if evidence.Kind == kind && (path == "" || evidence.Path == path) {
			return true
		}
	}
	return false
}

func requestIDPrefixForTest() string {
	return "request-000000"
}

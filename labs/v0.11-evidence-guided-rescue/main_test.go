package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/httpraw"
)

func decodeLabJSON(t *testing.T, handler http.HandlerFunc, target string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestItemsGroundTruth(t *testing.T) {
	baseline := decodeLabJSON(t, handleItems, "/items")
	if _, ok := baseline["selected_format"]; ok {
		t.Fatal("baseline unexpectedly exposes selected_format")
	}
	changed := decodeLabJSON(t, handleItems, "/items?format=json")
	if changed["selected_format"] != "json" {
		t.Fatalf("changed=%v", changed)
	}
}

func TestSearchGroundTruth(t *testing.T) {
	baseline := decodeLabJSON(t, handleSearch, "/search?debug=true")
	if _, ok := baseline["ordered"]; ok {
		t.Fatal("debug must remain a clean miss")
	}
	changed := decodeLabJSON(t, handleSearch, "/search?order=asc")
	if changed["ordered"] != true {
		t.Fatalf("changed=%v", changed)
	}
}

func TestNoSignalGroundTruth(t *testing.T) {
	baseline := decodeLabJSON(t, handleNoSignal, "/no-signal")
	probed := decodeLabJSON(t, handleNoSignal, "/no-signal?debug=true&format=json&limit=10")
	if baseline["status"] != probed["status"] || baseline["ok"] != probed["ok"] {
		t.Fatalf("baseline=%v probed=%v", baseline, probed)
	}
}

func TestProjectsGroundTruth(t *testing.T) {
	baseline := decodeLabJSON(t, handleProjects, "/projects?visibility=public")
	if _, ok := baseline["internal_projects"]; ok {
		t.Fatal("public value must remain baseline-like")
	}
	changed := decodeLabJSON(t, handleProjects, "/projects?visibility=internal")
	if changed["mode"] != "internal" {
		t.Fatalf("changed=%v", changed)
	}
}


func TestRequestFixturesAreValidRawHTTP(t *testing.T) {
	fixtures := []string{
		"request-items.txt",
		"request-search.txt",
		"request-no-signal.txt",
		"request-projects.txt",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte(`\n`)) {
				t.Fatalf("%s contains literal \\n escapes instead of HTTP line breaks", name)
			}
			tmpl, err := httpraw.Parse(raw, "http")
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			if tmpl.Method != http.MethodGet {
				t.Fatalf("%s method=%q want GET", name, tmpl.Method)
			}
		})
	}
}


func TestWordlistFixturesUseRealLineBreaks(t *testing.T) {
	fixtures := []string{
		"wordlist-items.txt",
		"wordlist-search.txt",
		"wordlist-no-signal.txt",
		"wordlist-projects.txt",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte(`\n`)) {
				t.Fatalf("%s contains literal \\n escapes instead of line breaks", name)
			}
			lines := bytes.Split(bytes.TrimSpace(raw), []byte{'
'})
			if len(lines) == 0 {
				t.Fatalf("%s contains no candidates", name)
			}
			for _, line := range lines {
				if bytes.ContainsAny(line, "\r\\") {
					t.Fatalf("%s contains malformed candidate %q", name, line)
				}
			}
		})
	}
}

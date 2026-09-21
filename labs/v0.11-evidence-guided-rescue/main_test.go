package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	if _, ok := baseline["sorted"]; ok {
		t.Fatal("debug must remain a clean miss")
	}
	changed := decodeLabJSON(t, handleSearch, "/search?sort=asc")
	if changed["sorted"] != true {
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

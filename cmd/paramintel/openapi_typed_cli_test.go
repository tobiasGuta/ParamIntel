package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCLIOpenAPIBooleanUsesSchemaTypedProbeFromFirstPass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		if raw, exists := profile["beta_access"]; exists {
			if enabled, ok := raw.(bool); ok && enabled {
				fmt.Fprint(w, `{"state":"beta"}`)
				return
			}
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	}))
	defer srv.Close()

	report, combined := runOpenAPICLI(t, srv, openAPISpecExistingParent)
	if len(report.Parameters) != 1 {
		t.Fatalf("parameters=%+v want one schema-typed OpenAPI finding\nCLI:\n%s", report.Parameters, combined)
	}
	finding := report.Parameters[0]
	if finding.Name != "beta_access" || finding.JSONPath != "$.profile.beta_access" {
		t.Fatalf("unexpected finding: %+v", finding)
	}
	if finding.DiscoveryMode != "schema_typed" {
		t.Fatalf("discovery mode=%q want schema_typed", finding.DiscoveryMode)
	}
	if finding.DiscoveryValueKind != "boolean" || finding.DiscoveryValue != "true" {
		t.Fatalf("typed value=%q kind=%q", finding.DiscoveryValue, finding.DiscoveryValueKind)
	}
	if finding.CandidateChanged != 3 || finding.RandomControlChanged != 0 {
		t.Fatalf("verification=%d/%d control=%d/%d", finding.CandidateChanged, finding.CandidateTrials, finding.RandomControlChanged, finding.RandomControlTrials)
	}
}

func TestCLIOpenAPIBooleanRejectsGenericTypedBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		for name, raw := range profile {
			if name == "name" {
				continue
			}
			if enabled, ok := raw.(bool); ok && enabled {
				fmt.Fprint(w, `{"state":"boolean-seen"}`)
				return
			}
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	}))
	defer srv.Close()

	report, combined := runOpenAPICLI(t, srv, openAPISpecExistingParent)
	if len(report.Parameters) != 0 {
		t.Fatalf("generic boolean behavior must be rejected by typed random-name control: %+v\nCLI:\n%s", report.Parameters, combined)
	}
}

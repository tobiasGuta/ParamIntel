package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestSemanticRescueFindsValueSensitiveDebug(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"users": []string{"alice", "bob"}}
		if r.URL.Query().Get("debug") == "true" {
			resp["debug"] = map[string]any{"database_ms": 12, "cache": "miss"}
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/users", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{Trials: 3, MinConfidence: .60}}
	candidate := model.Candidate{Name: "debug", Location: model.LocationQuery}

	// v0.3's generic random-string verifier cannot see this parameter.
	generic, err := engine.verify(context.Background(), tmpl, profile, candidate, 3)
	if err != nil {
		t.Fatal(err)
	}
	if generic.CandidateChanged != 0 || float64(generic.Confidence) != 0 {
		t.Fatalf("generic verification unexpectedly detected debug: %+v", generic)
	}

	rescued, value, ok, err := engine.semanticRescueCandidate(context.Background(), tmpl, profile, candidate, 3, .60)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected debug to be rescued by a semantic value")
	}
	if value.Kind != "string" || value.Raw != "true" {
		t.Fatalf("rescue value=%+v", value)
	}
	if rescued.Name != "debug" || rescued.Location != model.LocationQuery {
		t.Fatalf("result=%+v", rescued)
	}
	if rescued.CandidateChanged != 3 || rescued.RandomControlChanged != 0 {
		t.Fatalf("verification=%+v", rescued)
	}
	if float64(rescued.Confidence) != 1.0 || rescued.ConfidenceLabel != "high" {
		t.Fatalf("confidence=%+v", rescued)
	}
}

func TestSemanticRescueRejectsGenericSameValueBehavior(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"status": "baseline"}
		// Deliberately generic behavior: any query parameter causes the same
		// change, regardless of its name or value.
		if len(r.URL.Query()) > 0 {
			resp["query_seen"] = true
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/noisy", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{Trials: 3, MinConfidence: .60}}
	candidate := model.Candidate{Name: "preview", Location: model.LocationQuery}

	result, value, ok, err := engine.semanticRescueCandidate(context.Background(), tmpl, profile, candidate, 3, .60)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("generic behavior must not rescue candidate: value=%+v result=%+v", value, result)
	}
}

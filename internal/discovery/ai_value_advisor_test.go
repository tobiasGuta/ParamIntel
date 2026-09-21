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

func TestAIValueAdvisorRescuesApplicationSpecificEnum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"projects": 2}
		if r.URL.Query().Get("visibility") == "internal" {
			resp["internal_projects"] = 7
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/projects", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	advisorCalls := 0
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		Characterize:     false,
		ValueAware:       true,
		ValueAwareBudget: 8,
		SemanticValueAdvisor: func(ctx context.Context, candidate model.Candidate, deterministic []model.ProbeValue) ([]model.ProbeValue, error) {
			advisorCalls++
			if candidate.Name != "visibility" {
				t.Fatalf("candidate=%+v", candidate)
			}
			if len(deterministic) != 0 {
				t.Fatalf("visibility should have no built-in semantic profile: %+v", deterministic)
			}
			return []model.ProbeValue{model.StringValue("internal")}, nil
		},
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"visibility"})
	if err != nil {
		t.Fatal(err)
	}
	if advisorCalls != 1 {
		t.Fatalf("advisorCalls=%d want=1", advisorCalls)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.DiscoveryMode != "ai_value_aware" || r.DiscoveryValue != "internal" || r.DiscoveryValueKind != "string" {
		t.Fatalf("discovery=%+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 || float64(r.Confidence) != 1 {
		t.Fatalf("verification=%+v", r)
	}
}

func TestAIValueAdvisorNotCalledWhenDeterministicValueSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"ok": true}
		if r.URL.Query().Get("debug") == "true" {
			resp["debug"] = true
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	advisorCalls := 0
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		ValueAware:       true,
		ValueAwareBudget: 8,
		SemanticValueAdvisor: func(context.Context, model.Candidate, []model.ProbeValue) ([]model.ProbeValue, error) {
			advisorCalls++
			return []model.ProbeValue{model.StringValue("internal")}, nil
		},
	}}
	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"debug"})
	if err != nil {
		t.Fatal(err)
	}
	if advisorCalls != 0 {
		t.Fatalf("AI should not run after deterministic semantic success; calls=%d", advisorCalls)
	}
	if len(results) != 1 || results[0].DiscoveryMode != "value_aware" {
		t.Fatalf("results=%+v", results)
	}
}

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
		SemanticValueAdvisor: func(ctx context.Context, candidate model.Candidate, deterministic []model.ProbeValue) (SemanticValueAdvice, error) {
			advisorCalls++
			if candidate.Name != "visibility" {
				t.Fatalf("candidate=%+v", candidate)
			}
			if len(deterministic) != 0 {
				t.Fatalf("visibility should have no built-in semantic profile: %+v", deterministic)
			}
			return SemanticValueAdvice{Values: []model.ProbeValue{model.StringValue("internal")}, Queried: true}, nil
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
		SemanticValueAdvisor: func(context.Context, model.Candidate, []model.ProbeValue) (SemanticValueAdvice, error) {
			advisorCalls++
			return SemanticValueAdvice{Values: []model.ProbeValue{model.StringValue("internal")}, Queried: true}, nil
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


func TestAIValueAdvisorPrioritizesHighSignalCandidateBeforeBudgetIsSpent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"projects": 2, "available_visibilities": []string{"public", "private", "internal"}}
		if r.URL.Query().Get("visibility") == "internal" {
			resp["internal_projects"] = 1
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/projects", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	var called []string
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		ValueAware:           true,
		ValueAwareBudget:     8,
		EvidenceGuidedRescue: true,
		SemanticValuePriority: func(candidate model.Candidate) int {
			if candidate.Name == "visibility" {
				return 80
			}
			return 0
		},
		SemanticValueAdvisor: func(ctx context.Context, candidate model.Candidate, deterministic []model.ProbeValue) (SemanticValueAdvice, error) {
			called = append(called, candidate.Name)
			if len(called) > 1 {
				return SemanticValueAdvice{}, nil
			}
			if candidate.Name != "visibility" {
				t.Fatalf("first AI value query spent on %q, want visibility", candidate.Name)
			}
			return SemanticValueAdvice{Values: []model.ProbeValue{model.StringValue("internal")}, Queried: true}, nil
		},
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"account_id", "visibility"})
	if err != nil {
		t.Fatal(err)
	}
	if len(called) == 0 || called[0] != "visibility" {
		t.Fatalf("called=%v", called)
	}
	if len(results) != 1 || results[0].Name != "visibility" || results[0].DiscoveryMode != "ai_value_aware" {
		t.Fatalf("results=%+v", results)
	}
}


func TestAIValueAuditReportsOnlyActualProviderQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"role": "ROLE_USER"})
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	providerCalls := 0
	invocations := 0
	var audits []model.RescueCandidateAudit
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:             4,
		Trials:                3,
		MinConfidence:         .60,
		Locations:             []string{model.LocationQuery},
		ValueAware:            true,
		ValueAwareBudget:      16,
		EvidenceGuidedRescue:  true,
		SemanticValuePriority: func(candidate model.Candidate) int {
			if candidate.Name == "role" {
				return 100
			}
			return 0
		},
		SemanticValueAdvisor: func(ctx context.Context, candidate model.Candidate, deterministic []model.ProbeValue) (SemanticValueAdvice, error) {
			invocations++
			if providerCalls >= 1 {
				return SemanticValueAdvice{}, nil
			}
			providerCalls++
			return SemanticValueAdvice{
				Values:  []model.ProbeValue{model.StringValue("admin")},
				Queried: true,
			}, nil
		},
		RescueAuditObserver: func(audit model.RescueCandidateAudit) {
			audits = append(audits, audit)
		},
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"order", "role"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("results=%+v", results)
	}
	if providerCalls != 1 || invocations < 2 {
		t.Fatalf("providerCalls=%d invocations=%d", providerCalls, invocations)
	}
	if len(audits) < 2 {
		t.Fatalf("audits=%+v", audits)
	}
	if audits[0].Name != "role" || !audits[0].AIQueried || audits[0].AIValues != 1 {
		t.Fatalf("role audit=%+v", audits[0])
	}
	if audits[1].Name != "order" || audits[1].AIQueried || audits[1].AIValues != 0 {
		t.Fatalf("order audit=%+v", audits[1])
	}
}

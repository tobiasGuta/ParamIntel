package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

type rescueEvalResult struct {
	Found         []string
	RequestsUsed  int
	MissRequests  int
	VerifiedCost  int
	Attempted     []string
	Deferred      int
}

func runRescueEval(
	t *testing.T,
	handler http.HandlerFunc,
	words []string,
	seeds []model.Candidate,
	budget int,
	evidenceGuided bool,
	priority SemanticValuePriority,
) rescueEvalResult {
	t.Helper()

	srv := httptest.NewServer(handler)
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	var audits []model.RescueCandidateAudit
	eligible := 0
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:             64,
		Trials:                3,
		MinConfidence:         .60,
		Locations:             []string{model.LocationQuery},
		Characterize:          false,
		ValueAware:            true,
		ValueAwareBudget:      budget,
		EvidenceGuidedRescue:  evidenceGuided,
		SemanticValuePriority: priority,
		RescuePlanObserver: func(n, _ int) {
			eligible = n
		},
		RescueAuditObserver: func(audit model.RescueCandidateAudit) {
			audits = append(audits, audit)
		},
	}}

	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, words, seeds)
	if err != nil {
		t.Fatal(err)
	}

	out := rescueEvalResult{Deferred: eligible - len(audits)}
	for _, result := range results {
		out.Found = append(out.Found, result.Name)
	}
	for _, audit := range audits {
		out.RequestsUsed += audit.RequestsUsed
		out.Attempted = append(out.Attempted, audit.Name)
		switch audit.Outcome {
		case "verified":
			out.VerifiedCost += audit.RequestsUsed
		case "miss", "budget_exhausted":
			out.MissRequests += audit.RequestsUsed
		}
	}
	return out
}

func TestV011RescueEvaluationContextRelevanceUnderTightBudget(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("format") == "json" {
			fmt.Fprint(w, `{"items":[],"supported_formats":["json","csv"],"selected_format":"json"}`)
			return
		}
		fmt.Fprint(w, `{"items":[],"supported_formats":["json","csv"]}`)
	}
	priority := func(candidate model.Candidate) int {
		if candidate.Name == "format" {
			return 80
		}
		return 0
	}

	legacy := runRescueEval(t, handler, []string{"debug", "format"}, nil, 8, false, priority)
	guided := runRescueEval(t, handler, []string{"debug", "format"}, nil, 8, true, priority)

	if len(legacy.Found) != 0 {
		t.Fatalf("legacy unexpectedly found=%v metrics=%+v", legacy.Found, legacy)
	}
	if len(guided.Found) != 1 || guided.Found[0] != "format" {
		t.Fatalf("guided failed to recover format: %+v", guided)
	}
	if legacy.MissRequests != 8 || guided.MissRequests != 0 || guided.VerifiedCost != 8 {
		t.Fatalf("legacy=%+v guided=%+v", legacy, guided)
	}
	t.Logf("context relevance: legacy=%+v guided=%+v", legacy, guided)
}

func TestV011RescueEvaluationCostTieBreakerUnderTightBudget(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("sort") == "asc" {
			fmt.Fprint(w, `{"ok":true,"sorted":true}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}

	legacy := runRescueEval(t, handler, []string{"debug", "sort"}, nil, 8, false, nil)
	guided := runRescueEval(t, handler, []string{"debug", "sort"}, nil, 8, true, nil)

	if len(legacy.Found) != 0 {
		t.Fatalf("legacy unexpectedly found=%v metrics=%+v", legacy.Found, legacy)
	}
	if len(guided.Found) != 1 || guided.Found[0] != "sort" {
		t.Fatalf("guided failed to recover sort: %+v", guided)
	}
	if legacy.MissRequests != 8 || guided.VerifiedCost != 8 {
		t.Fatalf("legacy=%+v guided=%+v", legacy, guided)
	}
	t.Logf("cost tie-breaker: legacy=%+v guided=%+v", legacy, guided)
}

func TestV011RescueEvaluationNoRegressionWhenStrongCandidateAlreadyFirst(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("format") == "json" {
			fmt.Fprint(w, `{"ok":true,"format":"json"}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}

	legacy := runRescueEval(t, handler, []string{"format", "debug"}, nil, 8, false, nil)
	guided := runRescueEval(t, handler, []string{"format", "debug"}, nil, 8, true, nil)

	if len(legacy.Found) != 1 || legacy.Found[0] != "format" {
		t.Fatalf("legacy=%+v", legacy)
	}
	if len(guided.Found) != 1 || guided.Found[0] != "format" {
		t.Fatalf("guided=%+v", guided)
	}
	if legacy.RequestsUsed != guided.RequestsUsed || legacy.VerifiedCost != guided.VerifiedCost {
		t.Fatalf("unexpected cost regression: legacy=%+v guided=%+v", legacy, guided)
	}
	t.Logf("already-first no regression: legacy=%+v guided=%+v", legacy, guided)
}

func TestV011RescueEvaluationApplicationEvidenceRemainsAheadOfAI(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("format") == "json" {
			fmt.Fprint(w, `{"ok":true,"format":"json"}`)
			return
		}
		if r.URL.Query().Get("debug") == "true" {
			fmt.Fprint(w, `{"ok":true,"debug":true}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}

	seeds := []model.Candidate{
		{Name: "debug", Location: model.LocationQuery, Sources: []model.CandidateSource{{
			Source: "ai_semantic_hypothesis", Priority: 100,
		}}},
		{Name: "format", Location: model.LocationQuery, Sources: []model.CandidateSource{{
			Source: "context_response_only_json_property", Priority: 100,
		}}},
	}

	legacy := runRescueEval(t, handler, nil, seeds, 8, false, nil)
	guided := runRescueEval(t, handler, nil, seeds, 8, true, nil)

	if len(legacy.Found) != 1 || legacy.Found[0] != "debug" {
		t.Fatalf("legacy expected first-listed AI seed under v0.10 ordering: %+v", legacy)
	}
	if len(guided.Found) != 1 || guided.Found[0] != "format" {
		t.Fatalf("guided should prioritize application evidence: %+v", guided)
	}
	if guided.VerifiedCost != 8 {
		t.Fatalf("guided=%+v", guided)
	}
	t.Logf("application vs AI: legacy=%+v guided=%+v", legacy, guided)
}

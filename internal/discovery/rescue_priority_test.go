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

func TestRescueEvidenceTierOrdering(t *testing.T) {
	cases := []struct {
		name      string
		candidate model.Candidate
		wantTier  rescueEvidenceTier
	}{
		{
			name: "application",
			candidate: model.Candidate{Name: "access_level", Location: model.LocationJSON, Sources: []model.CandidateSource{{
				Source: "openapi_response_only_json_property", Priority: 110,
			}}},
			wantTier: rescueTierApplication,
		},
		{
			name: "context",
			candidate: model.Candidate{Name: "feature", Location: model.LocationJSON, Sources: []model.CandidateSource{{
				Source: "context_response_scaffoldable_json_property", Priority: 90,
			}}},
			wantTier: rescueTierContext,
		},
		{
			name: "ai",
			candidate: model.Candidate{Name: "visibility", Location: model.LocationQuery, Sources: []model.CandidateSource{{
				Source: "ai_semantic_hypothesis", Priority: 95,
			}}},
			wantTier: rescueTierAI,
		},
		{
			name:      "heuristic",
			candidate: model.Candidate{Name: "debug", Location: model.LocationQuery},
			wantTier:  rescueTierHeuristic,
		},
		{
			name:      "generic",
			candidate: model.Candidate{Name: "cursor", Location: model.LocationQuery},
			wantTier:  rescueTierGeneric,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rankRescueCandidate(tc.candidate, nil)
			if got.Tier != tc.wantTier {
				t.Fatalf("tier=%v want=%v rank=%+v", got.Tier, tc.wantTier, got)
			}
		})
	}

	for i := 0; i < len(cases)-1; i++ {
		left := rankRescueCandidate(cases[i].candidate, nil)
		right := rankRescueCandidate(cases[i+1].candidate, nil)
		if !rescueRankLess(left, right) {
			t.Fatalf("%s should outrank %s: left=%+v right=%+v", cases[i].name, cases[i+1].name, left, right)
		}
	}
}

func TestRescueContextRelevanceElevatesGenericCandidate(t *testing.T) {
	candidate := model.Candidate{Name: "visibility", Location: model.LocationQuery}
	rank := rankRescueCandidate(candidate, func(c model.Candidate) int {
		if c.Name == "visibility" {
			return 80
		}
		return 0
	})
	if rank.Tier != rescueTierContext || rank.ContextRelevance != 80 {
		t.Fatalf("rank=%+v", rank)
	}
}

func TestEvidenceGuidedRescueSpendsBudgetOnApplicationEvidenceBeforeAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Query().Get("format") == "json":
			fmt.Fprint(w, `{"ok":true,"format":"json"}`)
		case r.URL.Query().Get("debug") == "true":
			fmt.Fprint(w, `{"ok":true,"debug":true}`)
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{
		Method:  http.MethodGet,
		URL:     srv.URL + "/api",
		Headers: make(http.Header),
	}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	// Deliberately put the AI candidate first. With an eight-request rescue
	// budget, whichever candidate runs first consumes the complete successful
	// verification budget. Evidence-guided ordering must choose format.
	seeds := []model.Candidate{
		{
			Name:     "debug",
			Location: model.LocationQuery,
			Sources: []model.CandidateSource{{
				Source: "ai_semantic_hypothesis", Priority: 100,
			}},
		},
		{
			Name:     "format",
			Location: model.LocationQuery,
			Sources: []model.CandidateSource{{
				Source: "context_response_only_json_property", Priority: 100,
			}},
		},
	}

	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		Characterize:     false,
		ValueAware:           true,
		ValueAwareBudget:     8,
		EvidenceGuidedRescue: true,
	}}

	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, nil, seeds)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	if results[0].Name != "format" || results[0].DiscoveryMode != "value_aware" {
		t.Fatalf("application-backed candidate did not receive rescue budget first: %+v", results[0])
	}
}


func TestRescueRankPrefersCheaperScreenWithinEqualEvidence(t *testing.T) {
	sortCandidate := model.Candidate{Name: "sort", Location: model.LocationQuery}
	debugCandidate := model.Candidate{Name: "debug", Location: model.LocationQuery}

	sortRank := rankRescueCandidate(sortCandidate, nil)
	debugRank := rankRescueCandidate(debugCandidate, nil)

	if sortRank.Tier != rescueTierHeuristic || debugRank.Tier != rescueTierHeuristic {
		t.Fatalf("unexpected tiers: sort=%+v debug=%+v", sortRank, debugRank)
	}
	if sortRank.EstimatedScreenCost != 2 || debugRank.EstimatedScreenCost != 4 {
		t.Fatalf("unexpected costs: sort=%+v debug=%+v", sortRank, debugRank)
	}
	if !rescueRankLess(sortRank, debugRank) {
		t.Fatalf("cheaper equal-evidence candidate should run first: sort=%+v debug=%+v", sortRank, debugRank)
	}
}

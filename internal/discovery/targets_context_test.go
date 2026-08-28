package discovery

import (
	"net/http"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestBuildTargetsPrioritizesSeedAndPreservesItsProvenance(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method:  "POST",
		URL:     "https://example.test/api",
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"filters":{}}`),
	}
	seed := model.Candidate{
		Name:       "limit",
		Location:   model.LocationJSON,
		JSONParent: "$.filters",
		Sources: []model.CandidateSource{{
			Source:       "context_response_only_json_property",
			Path:         "$.filters.limit",
			ObservedType: "integer",
			Priority:     100,
		}},
	}

	targets, err := buildTargetsWithSeeds(tmpl, []string{"debug", "limit"}, []string{model.LocationJSON}, 3, []model.Candidate{seed})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 4 {
		t.Fatalf("targets=%+v", targets)
	}
	if targets[0].JSONPath() != "$.filters.limit" || len(targets[0].Sources) != 1 {
		t.Fatalf("first target should be contextual seed: %+v", targets[0])
	}
	count := 0
	for _, target := range targets {
		if target.JSONPath() == "$.filters.limit" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("context seed should suppress duplicate generic placement; count=%d targets=%+v", count, targets)
	}
}

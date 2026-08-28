package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/contextintel"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestContextCandidateComposesWithValueAwareJSONDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		resp := map[string]any{"items": []string{"active-a", "active-b"}}
		options, _ := body["options"].(map[string]any)
		if v, ok := options["include_deleted"].(bool); ok && v {
			resp["items"] = []string{"active-a", "active-b", "deleted-c"}
			resp["deleted_included"] = true
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{
		Method:  http.MethodPost,
		URL:     srv.URL + "/api/items",
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"options":{"page_size":10},"items":[]}`),
	}
	contextResponse := []byte(`{"options":{"page_size":10,"include_deleted":false},"items":[]}`)
	intel, err := contextintel.HarvestJSONResponse(tmpl.Body, contextResponse, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(intel.Actionable) != 1 {
		t.Fatalf("context candidates=%+v", intel.Actionable)
	}
	candidate := intel.Actionable[0]
	if candidate.Name != "include_deleted" || candidate.JSONParent != "$.options" || candidate.JSONPath() != "$.options.include_deleted" {
		t.Fatalf("candidate=%+v", candidate)
	}
	if len(candidate.Sources) != 1 || candidate.Sources[0].Source != "context_response_only_json_property" || candidate.Sources[0].ObservedType != "boolean" {
		t.Fatalf("candidate sources=%+v", candidate.Sources)
	}

	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationJSON},
		MaxJSONDepth:     3,
		Characterize:     false,
		ValueAware:       true,
		ValueAwareBudget: 16,
	}}

	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, []string{"admin", "debug"}, intel.Actionable)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.Name != "include_deleted" || r.JSONPath != "$.options.include_deleted" {
		t.Fatalf("result=%+v", r)
	}
	if r.DiscoveryMode != "value_aware" || r.DiscoveryValue != "true" || r.DiscoveryValueKind != "boolean" {
		t.Fatalf("discovery provenance=%+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 || float64(r.Confidence) != 1.0 {
		t.Fatalf("verification=%+v", r)
	}
	if len(r.CandidateSources) != 1 || r.CandidateSources[0].Source != "context_response_only_json_property" || r.CandidateSources[0].ObservedType != "boolean" {
		t.Fatalf("candidate sources=%+v", r.CandidateSources)
	}
}

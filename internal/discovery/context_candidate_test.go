package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/contextintel"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestContextCandidateFindsMassAssignmentFieldWithoutWordlistName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if _, ok := body["chosen_discount"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"chosen_discount must be an object"}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{
		Method:  "POST",
		URL:     srv.URL + "/api/checkout",
		Headers: http.Header{"Content-Type": []string{"text/plain;charset=UTF-8"}},
		Body:    []byte(`{"chosen_products":[{"product_id":"1","quantity":1}]}`),
	}
	contextResponse := []byte(`{"chosen_products":[],"chosen_discount":{"percentage":0}}`)
	intel, err := contextintel.HarvestJSONResponse(tmpl.Body, contextResponse, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(intel.Actionable) != 1 || intel.Actionable[0].Name != "chosen_discount" {
		t.Fatalf("context candidates=%+v", intel.Actionable)
	}

	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:     4,
		Trials:        3,
		MinConfidence: .60,
		Locations:     []string{model.LocationJSON},
		MaxJSONDepth:  3,
	}}

	// The generic list intentionally does not contain chosen_discount.
	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, []string{"admin", "debug", "preview"}, intel.Actionable)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.Name != "chosen_discount" || r.JSONPath != "$.chosen_discount" {
		t.Fatalf("result=%+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 || float64(r.Confidence) != 1.0 {
		t.Fatalf("verification=%+v", r)
	}
	if len(r.CandidateSources) != 1 || r.CandidateSources[0].Source != "context_response_only_json_property" {
		t.Fatalf("candidate sources=%+v", r.CandidateSources)
	}
}

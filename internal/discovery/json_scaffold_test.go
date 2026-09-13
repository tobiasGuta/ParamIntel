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

func scaffoldFixture(t *testing.T, handler http.HandlerFunc) (*httptest.Server, model.RequestTemplate, []model.Candidate) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	tmpl := model.RequestTemplate{
		Method:  http.MethodPost,
		URL:     srv.URL + "/api/profile",
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"profile":{"name":"tobias"}}`),
	}
	contextResponse := []byte(`{"profile":{"name":"tobias","settings":{"beta_access":false}}}`)
	intel, err := contextintel.HarvestJSONResponse(tmpl.Body, contextResponse, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(intel.Scaffoldable) != 1 || intel.Scaffoldable[0].JSONPath() != "$.profile.settings.beta_access" {
		t.Fatalf("scaffoldable=%+v", intel.Scaffoldable)
	}
	return srv, tmpl, intel.Scaffoldable
}

func TestJSONScaffoldFindsResponseDerivedNestedField(t *testing.T) {
	srv, tmpl, scaffoldable := scaffoldFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		settings, _ := profile["settings"].(map[string]any)
		if _, ok := settings["beta_access"]; ok {
			fmt.Fprint(w, `{"state":"beta"}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	})

	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:     8,
		Trials:        3,
		MinConfidence: .60,
		Locations:     []string{model.LocationJSON},
		MaxJSONDepth:  3,
		Characterize:  false,
		ValueAware:    false,
		JSONScaffold:  true,
	}}

	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, nil, scaffoldable)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.JSONPath != "$.profile.settings.beta_access" || r.CandidateChanged != 3 || r.RandomControlChanged != 0 {
		t.Fatalf("result=%+v", r)
	}
	if len(r.CandidateSources) != 1 || r.CandidateSources[0].Source != "context_response_scaffoldable_json_property" {
		t.Fatalf("candidate sources=%+v", r.CandidateSources)
	}
}

func TestJSONScaffoldDisabledDoesNotSendScaffoldCandidate(t *testing.T) {
	requests := 0
	srv, tmpl, scaffoldable := scaffoldFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"state":"normal"}`)
	})

	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	baselineRequests := requests
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:     8,
		Trials:        3,
		MinConfidence: .60,
		Locations:     []string{model.LocationJSON},
		MaxJSONDepth:  3,
		JSONScaffold:  false,
	}}
	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, nil, scaffoldable)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("results=%+v", results)
	}
	if requests != baselineRequests {
		t.Fatalf("scaffold-disabled scan sent %d unexpected requests", requests-baselineRequests)
	}
}

func TestJSONScaffoldRandomControlRejectsGenericParentBehavior(t *testing.T) {
	srv, tmpl, scaffoldable := scaffoldFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		profile, _ := body["profile"].(map[string]any)
		if settings, ok := profile["settings"].(map[string]any); ok && len(settings) > 0 {
			fmt.Fprint(w, `{"state":"settings-seen"}`)
			return
		}
		fmt.Fprint(w, `{"state":"normal"}`)
	})

	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:     8,
		Trials:        3,
		MinConfidence: .60,
		Locations:     []string{model.LocationJSON},
		MaxJSONDepth:  3,
		Characterize:  false,
		ValueAware:    false,
		JSONScaffold:  true,
	}}

	results, err := engine.ScanWithCandidates(context.Background(), tmpl, profile, nil, scaffoldable)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("generic scaffold behavior must be rejected by random-name control: %+v", results)
	}
}

func TestGroupTargetsIsolatesScaffoldCandidates(t *testing.T) {
	targets := []model.Candidate{
		{Name: "one", Location: model.LocationJSON, JSONParent: "$.settings", JSONScaffoldParent: "$.settings", Sources: []model.CandidateSource{{Source: "context"}}},
		{Name: "two", Location: model.LocationJSON, JSONParent: "$.settings", JSONScaffoldParent: "$.settings", Sources: []model.CandidateSource{{Source: "context"}}},
		{Name: "regular", Location: model.LocationJSON, JSONParent: "$", Sources: []model.CandidateSource{{Source: "context"}}},
	}
	groups := groupTargets(targets, 8)
	if len(groups) != 3 {
		t.Fatalf("groups=%+v", groups)
	}
	for _, group := range groups {
		if group[0].RequiresJSONScaffold() && len(group) != 1 {
			t.Fatalf("scaffold group was batched: %+v", group)
		}
	}
}

package main

import (
	"net/http"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestBuildRescuePriorityUsesBaselineResponseWithoutAI(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method:  http.MethodGet,
		URL:     "https://example.test/api/projects",
		Headers: make(http.Header),
	}
	snapshot := model.Snapshot{
		Body: []byte(`{"projects":[],"available_visibilities":["public","internal"]}`),
	}

	priority, source, err := buildRescuePriority(tmpl, snapshot, nil, []string{"auto"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if source != rescueContextBaselineResponse {
		t.Fatalf("source=%q", source)
	}
	if got := priority(model.Candidate{Name: "visibility", Location: model.LocationQuery}); got != 80 {
		t.Fatalf("visibility relevance=%d want=80", got)
	}
	if got := priority(model.Candidate{Name: "debug", Location: model.LocationQuery}); got != 0 {
		t.Fatalf("debug relevance=%d want=0", got)
	}
}

func TestBuildRescuePriorityPrefersExplicitContextResponse(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method:  http.MethodGet,
		URL:     "https://example.test/api/projects",
		Headers: make(http.Header),
	}
	snapshot := model.Snapshot{
		Body: []byte(`{"supported_formats":["json","csv"]}`),
	}
	contextRaw := []byte(`{"allowed_roles":["member","owner"]}`)

	priority, source, err := buildRescuePriority(tmpl, snapshot, contextRaw, []string{"auto"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if source != rescueContextExplicitResponse {
		t.Fatalf("source=%q", source)
	}
	if got := priority(model.Candidate{Name: "role", Location: model.LocationQuery}); got != 80 {
		t.Fatalf("role relevance=%d want=80", got)
	}
	if got := priority(model.Candidate{Name: "format", Location: model.LocationQuery}); got != 0 {
		t.Fatalf("format relevance=%d want=0 after explicit context override", got)
	}
}

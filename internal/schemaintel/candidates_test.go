package schemaintel

import (
	"reflect"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestExistingParentCandidatesAdmitsOnlyExistingParentDescriptors(t *testing.T) {
	report := Report{Candidates: []CandidateDescriptor{
		{
			Name:          "beta_access",
			Path:          "$.profile.beta_access",
			Parent:        "$.profile",
			DeclaredTypes: []string{"boolean"},
			ReadOnly:      true,
			SchemaRef:     "#/components/schemas/Profile",
			Source:        SourceResponseOnlyJSONProperty,
			Reason:        "response-only schema property",
			Placement:     PlacementExistingParent,
		},
		{
			Name:               "admin_mode",
			Path:               "$.profile.settings.admin_mode",
			Parent:             "$.profile.settings",
			DeclaredTypes:      []string{"boolean"},
			Source:             SourceResponseOnlyJSONProperty,
			Placement:          PlacementOneLevelScaffold,
			JSONScaffoldParent: "$.profile.settings",
		},
	}}

	got := ExistingParentCandidates(report)
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(got), got)
	}
	candidate := got[0]
	if candidate.Name != "beta_access" || candidate.Location != model.LocationJSON || candidate.JSONParent != "$.profile" {
		t.Fatalf("unexpected candidate: %+v", candidate)
	}
	if candidate.JSONScaffoldParent != "" {
		t.Fatalf("Slice 2 candidate must not carry scaffold authorization metadata: %+v", candidate)
	}
	if len(candidate.Sources) != 1 {
		t.Fatalf("sources=%+v", candidate.Sources)
	}
	source := candidate.Sources[0]
	if source.Source != SourceResponseOnlyJSONProperty || source.Path != "$.profile.beta_access" || source.Priority != existingParentCandidatePriority {
		t.Fatalf("unexpected provenance: %+v", source)
	}
	if !source.ReadOnly || source.SchemaRef != "#/components/schemas/Profile" || !reflect.DeepEqual(source.DeclaredTypes, []string{"boolean"}) {
		t.Fatalf("schema metadata not preserved: %+v", source)
	}
}

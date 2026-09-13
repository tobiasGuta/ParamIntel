package discovery

import (
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestSchemaTypedProbeValueSupportsOnlyUnambiguousOpenAPIScalars(t *testing.T) {
	tests := []struct {
		name      string
		candidate model.Candidate
		wantKind  string
		wantRaw   string
		wantOK    bool
	}{
		{
			name: "boolean",
			candidate: model.Candidate{
				Name: "beta_access", Location: model.LocationJSON, JSONParent: "$.profile",
				Sources: []model.CandidateSource{{Source: openAPIResponseOnlySource, DeclaredTypes: []string{"boolean"}}},
			},
			wantKind: "boolean", wantRaw: "true", wantOK: true,
		},
		{
			name: "integer",
			candidate: model.Candidate{
				Name: "tier", Location: model.LocationJSON, JSONParent: "$.profile",
				Sources: []model.CandidateSource{{Source: openAPIResponseOnlySource, DeclaredTypes: []string{"integer"}}},
			},
			wantKind: "integer", wantRaw: "1", wantOK: true,
		},
		{
			name: "union rejected",
			candidate: model.Candidate{
				Name: "beta_access", Location: model.LocationJSON, JSONParent: "$.profile",
				Sources: []model.CandidateSource{{Source: openAPIResponseOnlySource, DeclaredTypes: []string{"boolean", "null"}}},
			},
			wantOK: false,
		},
		{
			name: "number rejected",
			candidate: model.Candidate{
				Name: "ratio", Location: model.LocationJSON, JSONParent: "$.profile",
				Sources: []model.CandidateSource{{Source: openAPIResponseOnlySource, DeclaredTypes: []string{"number"}}},
			},
			wantOK: false,
		},
		{
			name: "context observed boolean does not authorize typed probe",
			candidate: model.Candidate{
				Name: "beta_access", Location: model.LocationJSON, JSONParent: "$.profile",
				Sources: []model.CandidateSource{{Source: "context_response_only_json_property", ObservedType: "boolean"}},
			},
			wantOK: false,
		},
		{
			name: "generic candidate rejected",
			candidate: model.Candidate{Name: "beta_access", Location: model.LocationJSON, JSONParent: "$.profile"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := schemaTypedProbeValue(tt.candidate)
			if ok != tt.wantOK {
				t.Fatalf("ok=%t want %t value=%+v", ok, tt.wantOK, value)
			}
			if !ok {
				return
			}
			if value.Kind != tt.wantKind || value.Raw != tt.wantRaw {
				t.Fatalf("value=%+v want kind=%s raw=%s", value, tt.wantKind, tt.wantRaw)
			}
		})
	}
}

func TestGroupTargetsIsolatesSchemaTypedCandidate(t *testing.T) {
	typed := model.Candidate{
		Name: "beta_access", Location: model.LocationJSON, JSONParent: "$.profile",
		Sources: []model.CandidateSource{{Source: openAPIResponseOnlySource, DeclaredTypes: []string{"boolean"}}},
	}
	other := model.Candidate{
		Name: "role", Location: model.LocationJSON, JSONParent: "$.profile",
		Sources: []model.CandidateSource{{Source: openAPIResponseOnlySource, DeclaredTypes: []string{"string"}}},
	}
	groups := groupTargets([]model.Candidate{typed, other}, 8)
	if len(groups) != 2 {
		t.Fatalf("groups=%+v want 2 isolated placement groups", groups)
	}
	if len(groups[0]) != 1 || groups[0][0].Name != "beta_access" {
		t.Fatalf("typed group=%+v", groups[0])
	}
}

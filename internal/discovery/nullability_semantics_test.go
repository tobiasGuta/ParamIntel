package discovery

import (
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestSchemaTypedProbeRejectsNullableScalarSources(t *testing.T) {
	tests := []struct {
		name      string
		types     []string
		nullable  bool
		wantProbe bool
		wantKind  string
		wantRaw   string
	}{
		{name: "non-nullable boolean", types: []string{"boolean"}, wantProbe: true, wantKind: "boolean", wantRaw: "true"},
		{name: "nullable boolean", types: []string{"boolean"}, nullable: true},
		{name: "non-nullable integer", types: []string{"integer"}, wantProbe: true, wantKind: "integer", wantRaw: "1"},
		{name: "nullable integer", types: []string{"integer"}, nullable: true},
		{name: "boolean null union", types: []string{"boolean", "null"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := model.Candidate{
				Name:     "enabled",
				Location: model.LocationJSON,
				Sources: []model.CandidateSource{{
					Source:        openAPIResponseOnlySource,
					DeclaredTypes: tt.types,
					Nullable:      tt.nullable,
				}},
			}

			kind, kindOK := schemaTypedProbeKind(candidate)
			value, valueOK := schemaTypedProbeValue(candidate)
			if kindOK != tt.wantProbe || valueOK != tt.wantProbe {
				t.Fatalf("typed probe kindOK=%t valueOK=%t want=%t", kindOK, valueOK, tt.wantProbe)
			}
			if !tt.wantProbe {
				if kind != "" || value.Kind != "" || value.Raw != "" {
					t.Fatalf("unexpected rejected probe kind=%q value=%+v", kind, value)
				}
				return
			}
			if kind != tt.wantKind || value.Kind != tt.wantKind || value.Raw != tt.wantRaw {
				t.Fatalf("kind=%q value=%+v want kind=%q raw=%q", kind, value, tt.wantKind, tt.wantRaw)
			}
		})
	}
}

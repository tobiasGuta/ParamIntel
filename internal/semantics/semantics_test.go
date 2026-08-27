package semantics

import (
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"testing"
)

func TestInferFromValidationResponse(t *testing.T) {
	for body, want := range map[string]string{`{"error":"limit must be an integer"}`: "integer", `{"error":"include_deleted must be boolean"}`: "boolean", `invalid uuid`: "uuid"} {
		got := InferFromResponse([]byte(body))
		if got.Name != want || got.Confidence < .95 {
			t.Fatalf("body=%q got=%+v", body, got)
		}
	}
}

func TestProfileValuesUseJSONTypes(t *testing.T) {
	bools := ProfileValues("include_deleted", model.LocationJSON)
	if len(bools) < 2 || bools[0].Kind != "boolean" || bools[1].Kind != "boolean" {
		t.Fatalf("bool profile=%+v", bools)
	}
	ints := ProfileValues("limit", model.LocationJSON)
	if len(ints) != 4 || ints[0].Kind != "integer" {
		t.Fatalf("int profile=%+v", ints)
	}
}

func TestProfileValuesAvoidsBusinessStateEnums(t *testing.T) {
	for _, name := range []string{"role", "status", "environment"} {
		if got := ProfileValues(name, model.LocationJSON); len(got) != 0 {
			t.Fatalf("%s should not be automatically value-profiled: %+v", name, got)
		}
	}
}

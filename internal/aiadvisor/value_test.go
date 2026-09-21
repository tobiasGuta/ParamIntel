package aiadvisor

import (
	"context"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

type valueTestProvider struct{}

func (valueTestProvider) Name() string  { return "test" }
func (valueTestProvider) Model() string { return "test-model" }
func (valueTestProvider) Suggest(context.Context, Input, int) ([]Suggestion, error) {
	return nil, nil
}
func (valueTestProvider) SuggestValues(context.Context, ValueInput, int) ([]ValueSuggestion, error) {
	return []ValueSuggestion{
		{Value: "internal", Kind: "string", Reason: "visibility state", Priority: 95},
		{Value: "true", Kind: "boolean", Reason: "typed JSON flag", Priority: 90},
		{Value: "internal", Kind: "string", Reason: "duplicate", Priority: 80},
		{Value: "ignored", Kind: "object", Reason: "unsupported", Priority: 70},
	}, nil
}

func TestGenerateValuesAdmitsBoundedTypedHypotheses(t *testing.T) {
	input := ValueInput{
		Candidate: ValueCandidate{Name: "visibility", Location: model.LocationJSON, JSONParent: "$"},
		ExcludedValues: []ValueIdentity{{Kind: "string", Raw: "public"}},
	}
	result, err := GenerateValues(context.Background(), valueTestProvider{}, input, 4)
	if err != nil {
		t.Fatal(err)
	}
	if result.SuggestedCount != 4 || result.AcceptedCount != 2 {
		t.Fatalf("result=%+v", result)
	}
	if result.Values[0] != model.StringValue("internal") {
		t.Fatalf("first=%+v", result.Values[0])
	}
	if result.Values[1] != model.BoolValue(true) {
		t.Fatalf("second=%+v", result.Values[1])
	}
}

func TestValueSuggestionsDoNotRepeatDeterministicValues(t *testing.T) {
	input := ValueInput{
		Candidate:      ValueCandidate{Name: "mode", Location: model.LocationQuery},
		ExcludedValues: []ValueIdentity{{Kind: "string", Raw: "internal"}},
	}
	values, audit := evaluateValueSuggestions(input, []ValueSuggestion{
		{Value: "internal", Kind: "string", Priority: 100},
		{Value: "preview", Kind: "boolean", Priority: 90},
	}, 4)
	if len(values) != 1 || values[0] != model.StringValue("preview") {
		t.Fatalf("values=%+v", values)
	}
	if audit[0].RejectionReason != "deterministic_value" {
		t.Fatalf("audit=%+v", audit)
	}
	if audit[1].Kind != "string" {
		t.Fatalf("query/form values must normalize to strings: %+v", audit[1])
	}
}

func TestJSONValueValidationRejectsUnsafeKindsAndInvalidTypedValues(t *testing.T) {
	input := ValueInput{Candidate: ValueCandidate{Name: "mode", Location: model.LocationJSON}}
	values, audit := evaluateValueSuggestions(input, []ValueSuggestion{
		{Value: "yes", Kind: "boolean", Priority: 100},
		{Value: "1.5", Kind: "integer", Priority: 90},
		{Value: "{}", Kind: "object", Priority: 80},
	}, 4)
	if len(values) != 0 {
		t.Fatalf("values=%+v", values)
	}
	want := []string{"invalid_boolean", "invalid_integer", "unsupported_kind"}
	for i, reason := range want {
		if audit[i].RejectionReason != reason {
			t.Fatalf("audit[%d]=%+v want=%s", i, audit[i], reason)
		}
	}
}


func TestSemanticStringAllowlistRejectsPayloadSyntax(t *testing.T) {
	input := ValueInput{Candidate: ValueCandidate{Name: "mode", Location: model.LocationQuery}}
	values, audit := evaluateValueSuggestions(input, []ValueSuggestion{
		{Value: "../admin", Kind: "string", Priority: 100},
		{Value: "<script>", Kind: "string", Priority: 90},
		{Value: "internal", Kind: "string", Priority: 80},
	}, 4)
	if len(values) != 1 || values[0] != model.StringValue("internal") {
		t.Fatalf("values=%+v", values)
	}
	if audit[0].RejectionReason != "unsafe_string" || audit[1].RejectionReason != "unsafe_string" {
		t.Fatalf("audit=%+v", audit)
	}
}

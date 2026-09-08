package aiadvisor

import (
	"context"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

type fakeProvider struct{ suggestions []Suggestion }

func (f fakeProvider) Name() string  { return "fake" }
func (f fakeProvider) Model() string { return "fake-model" }
func (f fakeProvider) Suggest(context.Context, Input, int) ([]Suggestion, error) {
	return f.suggestions, nil
}

func TestAcceptSuggestionsValidatesAndPrioritizes(t *testing.T) {
	input := Input{
		ActiveLocations:  []string{model.LocationQuery, model.LocationJSON},
		QueryKeys:        []string{"existing"},
		JSONParents:      []string{"$", "$.options"},
		RequestJSONShape: map[string]any{"options": map[string]any{"page_size": "integer"}},
	}
	suggestions := []Suggestion{
		{Name: "existing", Location: "query", Priority: 100, Reason: "already present"},
		{Name: "include_deleted", Location: "json", JSONParent: "$.missing", Priority: 99, Reason: "bad parent"},
		{Name: "role", Location: "header", Priority: 98, Reason: "bad location"},
		{Name: "include_archived", Location: "json", JSONParent: "$.options", Priority: 90, Reason: "response exposes archive semantics"},
		{Name: "expand", Location: "query", Priority: 80, Reason: "likely expansion control"},
		{Name: "bad name!", Location: "query", Priority: 100},
		{Name: "expand", Location: "query", Priority: 10, Reason: "duplicate"},
	}
	got := AcceptSuggestions(input, suggestions, 2)
	if len(got) != 2 {
		t.Fatalf("len=%d want=2: %#v", len(got), got)
	}
	if got[0].Name != "include_archived" || got[0].JSONParent != "$.options" {
		t.Fatalf("first=%+v", got[0])
	}
	if got[1].Name != "expand" || got[1].Location != model.LocationQuery {
		t.Fatalf("second=%+v", got[1])
	}
	if got[0].Sources[0].Source != SourceAISemanticHypothesis || got[0].Sources[0].Reason == "" {
		t.Fatalf("source=%+v", got[0].Sources[0])
	}
}

func TestGenerateKeepsProviderMetadataAndAdmissionAudit(t *testing.T) {
	p := fakeProvider{suggestions: []Suggestion{
		{Name: "debug", Location: "query", Priority: 70, Reason: "test"},
		{Name: "bad name!", Location: "query", Priority: 90, Reason: "invalid"},
	}}
	result, err := Generate(context.Background(), p, Input{ActiveLocations: []string{"query"}}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "fake" || result.Model != "fake-model" || result.SuggestedCount != 2 || result.AcceptedCount != 1 {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Audit) != 2 {
		t.Fatalf("audit=%+v", result.Audit)
	}
	var admitted, rejected *SuggestionAudit
	for i := range result.Audit {
		a := &result.Audit[i]
		switch a.Admission {
		case AdmissionAdmitted:
			admitted = a
		case AdmissionRejected:
			rejected = a
		}
	}
	if admitted == nil || admitted.Name != "debug" {
		t.Fatalf("admitted=%+v", admitted)
	}
	if rejected == nil || rejected.Name != "bad name!" || rejected.RejectionReason != "invalid_name" {
		t.Fatalf("rejected=%+v", rejected)
	}
}

func TestAdmissionAuditExplainsLocalRejections(t *testing.T) {
	input := Input{
		ActiveLocations: []string{model.LocationQuery, model.LocationJSON},
		QueryKeys:       []string{"existing"},
		JSONParents:     []string{"$"},
	}
	_, audit := evaluateSuggestions(input, []Suggestion{
		{Name: "existing", Location: "query", Priority: 100},
		{Name: "wrong_parent", Location: "json", JSONParent: "$.missing", Priority: 90},
		{Name: "header_candidate", Location: "header", Priority: 80},
		{Name: "good", Location: "query", Priority: 70},
		{Name: "good", Location: "query", Priority: 60},
	}, 5)

	reasons := map[string]bool{}
	for _, item := range audit {
		if item.RejectionReason != "" {
			reasons[item.RejectionReason] = true
		}
	}
	for _, want := range []string{"already_present", "invalid_json_parent", "inactive_location", "duplicate"} {
		if !reasons[want] {
			t.Fatalf("missing rejection reason %q in %+v", want, audit)
		}
	}
}

func TestBoundedReason(t *testing.T) {
	r := boundedReason("  hello \n world  ")
	if r != "hello world" {
		t.Fatalf("reason=%q", r)
	}
}

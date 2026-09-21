package aiadvisor

import (
	"reflect"
	"testing"
)

func TestBuildSemanticValueHintsExtractsRelevantEnumVocabulary(t *testing.T) {
	raw := []byte(`HTTP/1.1 200 OK
Content-Type: application/json

{"available_visibilities":["public","private","internal"],"projects":[{"id":"p1","secret":"token-123"}],"unrelated":["alpha","beta"]}`)
	got := BuildSemanticValueHints(raw, ValueCandidate{Name: "visibility", Location: "query"}, 12)
	want := []string{"internal", "private", "public"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hints=%v want=%v", got, want)
	}
}

func TestBuildSemanticValueHintsRejectsPayloadLikeAndUnrelatedValues(t *testing.T) {
	raw := []byte(`{"available_modes":["safe","../admin","<script>"],"notes":["internal"],"status":"private"}`)
	got := BuildSemanticValueHints(raw, ValueCandidate{Name: "mode", Location: "query"}, 12)
	want := []string{"safe"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hints=%v want=%v", got, want)
	}
}

func TestValueCandidateRelevancePrefersResponseSupportedCandidate(t *testing.T) {
	input := Input{
		ResponseJSONShape: map[string]any{
			"available_visibilities": "array",
			"projects": "array",
		},
	}
	visibility := ValueCandidateRelevance(input, ValueCandidate{Name: "visibility", Location: "query"})
	accountID := ValueCandidateRelevance(input, ValueCandidate{Name: "account_id", Location: "query"})
	if visibility <= accountID || visibility == 0 {
		t.Fatalf("visibility=%d account_id=%d", visibility, accountID)
	}
}

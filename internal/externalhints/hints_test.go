package externalhints

import (
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestParseRejectsUnknownFieldsAndOversizedInput(t *testing.T) {
	if _, err := Parse([]byte(`{"candidates":[{"name":"debug","location":"query","wat":true}]}`)); err == nil {
		t.Fatal("expected unknown field rejection")
	}
	if _, err := Parse([]byte(strings.Repeat("x", MaxDocumentBytes+1))); err == nil {
		t.Fatal("expected oversized document rejection")
	}
}

func TestAdmitCandidatesUsesExistingLocalPolicyAndExternalProvenance(t *testing.T) {
	doc, err := Parse([]byte(`{
	  "context":["admin user creation"],
	  "candidates":[
	    {"name":"role","location":"json","json_parent":"$","priority":90,"reason":"authorization-related field"},
	    {"name":"existing","location":"query","priority":100}
	  ]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	input := aiadvisor.Input{
		ActiveLocations:  []string{model.LocationQuery, model.LocationJSON},
		QueryKeys:        []string{"existing"},
		JSONParents:      []string{"$"},
		RequestJSONShape: map[string]any{},
	}
	admission := doc.AdmitCandidates(input, nil, 10)
	if len(admission.Candidates) != 1 {
		t.Fatalf("candidates=%+v", admission.Candidates)
	}
	got := admission.Candidates[0]
	if got.Name != "role" || got.Location != model.LocationJSON {
		t.Fatalf("candidate=%+v", got)
	}
	if len(got.Sources) != 1 || got.Sources[0].Source != SourceExternalSemanticHint {
		t.Fatalf("sources=%+v", got.Sources)
	}
	if len(admission.Audit) != 2 {
		t.Fatalf("audit=%+v", admission.Audit)
	}
}

func TestValuesForUsesExistingTypedAdmissionAndExclusions(t *testing.T) {
	doc, err := Parse([]byte(`{
	  "candidates":[{
	    "name":"role",
	    "location":"json",
	    "json_parent":"$",
	    "values":[
	      {"value":"admin","kind":"string","priority":90},
	      {"value":"true","kind":"boolean","priority":80},
	      {"value":"{}","kind":"object","priority":70}
	    ]
	  }]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	candidate := model.Candidate{Name: "role", Location: model.LocationJSON, JSONParent: "$"}
	admission := doc.ValuesFor(candidate, []model.ProbeValue{model.StringValue("admin")}, 12)
	if len(admission.Values) != 1 || admission.Values[0] != model.BoolValue(true) {
		t.Fatalf("values=%+v", admission.Values)
	}
	if len(admission.Audit) != 3 {
		t.Fatalf("audit=%+v", admission.Audit)
	}
	if admission.Audit[0].RejectionReason != "deterministic_value" {
		t.Fatalf("first audit=%+v", admission.Audit[0])
	}
	if admission.Audit[2].RejectionReason != "unsupported_kind" {
		t.Fatalf("third audit=%+v", admission.Audit[2])
	}
}

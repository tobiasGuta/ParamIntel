package decision

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestStateFromParameterResultSanitizesEvidenceValues(t *testing.T) {
	state := StateFromParameterResult(model.ParameterResult{
		Name:                 "locale",
		Location:             model.LocationQuery,
		DiscoveryMode:        "value_aware",
		DiscoveryValue:       "SECRET-DISCOVERY-VALUE",
		DiscoveryValueKind:   "string",
		Confidence:           model.ConfidenceScore(0.91),
		CandidateChanged:     3,
		CandidateTrials:      3,
		RandomControlChanged: 0,
		RandomControlTrials:  3,
		Evidence: []model.Difference{
			{Kind: "json_path_added", Path: "$.supported_locales", Before: "SECRET-BEFORE", After: "SECRET-AFTER"},
			{Kind: "json_path_added", Path: "$.supported_locales", Before: "OTHER", After: "VALUE"},
			{Kind: "body_length", Before: "100", After: "120"},
		},
	}, 8)

	if state.Candidate.Name != "locale" || state.Candidate.Location != model.LocationQuery {
		t.Fatalf("candidate=%+v", state.Candidate)
	}
	if state.Candidate.DiscoveryMode != "value_aware" || state.Candidate.ValueKind != "string" {
		t.Fatalf("candidate metadata=%+v", state.Candidate)
	}
	if state.RemainingRequestBudget != 8 {
		t.Fatalf("remaining budget=%d want=8", state.RemainingRequestBudget)
	}
	if len(state.Evidence.Kinds) != 2 || state.Evidence.Kinds[0] != "body_length" || state.Evidence.Kinds[1] != "json_path_added" {
		t.Fatalf("evidence kinds=%v", state.Evidence.Kinds)
	}
	if len(state.Evidence.Paths) != 1 || state.Evidence.Paths[0] != "$.supported_locales" {
		t.Fatalf("evidence paths=%v", state.Evidence.Paths)
	}

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET-DISCOVERY-VALUE", "SECRET-BEFORE", "SECRET-AFTER", "OTHER", "VALUE"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("sanitized state leaked %q: %s", secret, raw)
		}
	}
}

func TestStateFromParameterResultDefaultsGenericProbeKindToString(t *testing.T) {
	state := StateFromParameterResult(model.ParameterResult{
		Name:     "theme",
		Location: model.LocationJSON,
	}, 4)
	if state.Candidate.ValueKind != "string" {
		t.Fatalf("value kind=%q want=string", state.Candidate.ValueKind)
	}
}

func TestAppendShadowCaptureJSONLWritesSanitizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shadow.jsonl")
	state := State{
		Candidate: CandidateState{Name: "sandbox", Location: "query", ValueKind: "string"},
		Verification: VerificationState{
			CandidateChanged: 3,
			CandidateTrials:  3,
			ControlChanged:   0,
			ControlTrials:    3,
			Confidence:       1,
		},
		Evidence:               EvidenceState{Kinds: []string{"json_path_added"}, Paths: []string{"$.features.sandbox_flag"}},
		RemainingRequestBudget: 8,
	}
	if err := AppendShadowCaptureJSONL(path, state); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines=%d want=1", len(lines))
	}
	var record ShadowCaptureRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != ShadowCaptureSchemaVersion {
		t.Fatalf("schema version=%d", record.SchemaVersion)
	}
	if record.ID == "" {
		t.Fatal("record ID is empty")
	}
	if record.Source != "paramintel_verified_parameter_pre_characterization" {
		t.Fatalf("source=%q", record.Source)
	}
	if record.State.Candidate.Name != "sandbox" {
		t.Fatalf("state=%+v", record.State)
	}
}

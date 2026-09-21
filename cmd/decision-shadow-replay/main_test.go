package main

import (
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/decision"
)

func testDatasetCase(t *testing.T, action decision.Action) datasetCase {
	t.Helper()
	state := decision.State{
		Candidate: decision.CandidateState{Name: "region", Location: "query", ValueKind: "string"},
		Evidence: decision.EvidenceState{Paths: []string{"$.aliases.region"}},
		RemainingRequestBudget: 8,
	}
	record, err := decision.NewShadowCaptureRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	return datasetCase{
		ID:             record.ID,
		Name:           "shadow-" + record.ID,
		State:          state,
		ExpectedAction: action,
	}
}

func TestValidateDatasetRejectsUnlabeledCase(t *testing.T) {
	c := testDatasetCase(t, "")
	d := dataset{
		SchemaVersion: 1,
		Source:        "paramintel_decision_shadow_frozen",
		CapturedRecords: 1,
		UniqueCases:   1,
		Cases:         []datasetCase{c},
	}
	if err := validateDataset(d); err == nil {
		t.Fatal("expected unlabeled dataset to fail")
	}
}

func TestValidateDatasetAcceptsCatalogAction(t *testing.T) {
	c := testDatasetCase(t, decision.ActionRelatedValueProfile)
	d := dataset{
		SchemaVersion: 1,
		Source:        "paramintel_decision_shadow_frozen",
		CapturedRecords: 1,
		UniqueCases:   1,
		Cases:         []datasetCase{c},
	}
	if err := validateDataset(d); err != nil {
		t.Fatal(err)
	}
}

func TestValidateDatasetRejectsUnknownAction(t *testing.T) {
	c := testDatasetCase(t, decision.Action("invented_action"))
	d := dataset{
		SchemaVersion: 1,
		Source:        "paramintel_decision_shadow_frozen",
		CapturedRecords: 1,
		UniqueCases:   1,
		Cases:         []datasetCase{c},
	}
	if err := validateDataset(d); err == nil {
		t.Fatal("expected unknown action to fail")
	}
}

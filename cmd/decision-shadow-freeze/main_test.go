package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/decision"
)

func TestFreezeDatasetDeduplicatesAndVerifiesIDs(t *testing.T) {
	state := decision.State{
		Candidate: decision.CandidateState{Name: "region", Location: "query", ValueKind: "string"},
		Evidence: decision.EvidenceState{Paths: []string{"$.aliases.region"}},
		RemainingRequestBudget: 8,
	}
	record, err := decision.NewShadowCaptureRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "shadow.jsonl")
	raw := append(append([]byte{}, line...), '\n')
	raw = append(raw, line...)
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	got, err := freezeDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CapturedRecords != 2 {
		t.Fatalf("captured=%d want=2", got.CapturedRecords)
	}
	if got.UniqueCases != 1 || len(got.Cases) != 1 {
		t.Fatalf("unique=%d cases=%d", got.UniqueCases, len(got.Cases))
	}
	if got.Cases[0].ID != record.ID {
		t.Fatalf("id=%q want=%q", got.Cases[0].ID, record.ID)
	}
	if got.Cases[0].ExpectedAction != "" {
		t.Fatalf("expected action=%q want blank", got.Cases[0].ExpectedAction)
	}
}

func TestFreezeDatasetRejectsTamperedID(t *testing.T) {
	state := decision.State{
		Candidate: decision.CandidateState{Name: "region", Location: "query", ValueKind: "string"},
		RemainingRequestBudget: 8,
	}
	record, err := decision.NewShadowCaptureRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	record.ID = "tampered"
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "shadow.jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := freezeDataset(path); err == nil {
		t.Fatal("expected tampered id to fail")
	}
}

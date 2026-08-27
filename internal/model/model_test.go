package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfidenceScoreJSONKeepsDecimalAndLabel(t *testing.T) {
	r := ParameterResult{Name: "query", Location: "query", Confidence: ConfidenceScore(1), ConfidenceLabel: "high"}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"confidence":1.00`) {
		t.Fatalf("confidence did not retain decimal formatting: %s", s)
	}
	if !strings.Contains(s, `"confidence_label":"high"`) {
		t.Fatalf("confidence label missing: %s", s)
	}
}

func TestCandidateJSONPath(t *testing.T) {
	root := Candidate{Name: "debug", Location: LocationJSON, JSONParent: "$"}
	if root.JSONPath() != "$.debug" {
		t.Fatalf("root path=%q", root.JSONPath())
	}
	nested := Candidate{Name: "limit", Location: LocationJSON, JSONParent: "$.filters"}
	if nested.JSONPath() != "$.filters.limit" {
		t.Fatalf("nested path=%q", nested.JSONPath())
	}
}

func TestScanReportEmptyParametersSerializesArray(t *testing.T) {
	r := ScanReport{Version: "0.2.0", Parameters: make([]ParameterResult, 0)}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"version":"0.2.0"`) || !strings.Contains(s, `"parameters":[]`) {
		t.Fatalf("unexpected report JSON: %s", s)
	}
}

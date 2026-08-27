package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfidenceScoreJSONKeepsDecimalAndLabel(t *testing.T) {
	r := ParameterResult{
		Name:            "query",
		Location:        "query",
		Confidence:      ConfidenceScore(1),
		ConfidenceLabel: "high",
	}
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

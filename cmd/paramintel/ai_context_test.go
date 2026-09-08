package main

import (
	"bytes"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestSelectAIContextDefaultsToBaselineResponse(t *testing.T) {
	baselineBody := []byte(`{"filters":{"include_archived":false}}`)
	got, source := selectAIContext(model.Snapshot{Body: baselineBody}, nil, false)
	if source != aiContextBaselineResponse {
		t.Fatalf("source=%q", source)
	}
	if !bytes.Equal(got, baselineBody) {
		t.Fatalf("context=%q want=%q", got, baselineBody)
	}
}

func TestSelectAIContextExplicitOverrideWinsEvenWhenEmpty(t *testing.T) {
	got, source := selectAIContext(model.Snapshot{Body: []byte(`{"baseline":true}`)}, []byte{}, true)
	if source != aiContextExplicitOverride {
		t.Fatalf("source=%q", source)
	}
	if len(got) != 0 {
		t.Fatalf("context=%q want empty explicit override", got)
	}
}

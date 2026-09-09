package main

import "github.com/tobiasGuta/ParamIntel/internal/model"

const (
	aiContextBaselineResponse = "baseline_response"
	aiContextExplicitOverride = "ai_context_response"
)

// selectAIContext reuses an already-collected baseline response by default.
// An explicit -ai-context-response file wins when present. This selection does
// not perform target I/O; callers pass in data they have already collected or
// read locally.
func selectAIContext(snapshot model.Snapshot, override []byte, hasOverride bool) ([]byte, string) {
	if hasOverride {
		return override, aiContextExplicitOverride
	}
	return snapshot.Body, aiContextBaselineResponse
}

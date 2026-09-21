package main

import (
	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/discovery"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	rescueContextBaselineResponse = "baseline_response"
	rescueContextExplicitResponse = "context_response"
)

// buildRescuePriority derives a local, deterministic semantic relevance
// function from sanitized request/response structure. It never constructs an
// AI provider and never performs network I/O.
func buildRescuePriority(
	tmpl model.RequestTemplate,
	snapshot model.Snapshot,
	contextRaw []byte,
	locations []string,
	jsonDepth int,
) (discovery.SemanticValuePriority, string, error) {
	raw := snapshot.Body
	source := rescueContextBaselineResponse
	if len(contextRaw) > 0 {
		raw = contextRaw
		source = rescueContextExplicitResponse
	}

	input, err := aiadvisor.BuildInput(tmpl, raw, locations, jsonDepth)
	if err != nil {
		return nil, "", err
	}
	priority := func(candidate model.Candidate) int {
		return aiadvisor.ValueCandidateRelevance(input, aiadvisor.ValueCandidate{
			Name:       candidate.Name,
			Location:   candidate.Location,
			JSONParent: candidate.JSONParent,
		})
	}
	return priority, source, nil
}

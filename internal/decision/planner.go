package decision

import (
	"context"
	"fmt"
)

const DefaultMinConfidence = 0.80

type Plan struct {
	SuggestedAction Action             `json:"suggested_action"`
	AppliedAction   Action             `json:"applied_action"`
	Confidence      float64            `json:"confidence"`
	Probabilities   map[Action]float64 `json:"probabilities,omitempty"`
	Provider        string             `json:"provider,omitempty"`
	Model           string             `json:"model,omitempty"`
	Gated           bool               `json:"gated"`
	GateReason      string             `json:"gate_reason,omitempty"`
	Usage           Usage              `json:"usage,omitempty"`
}

type Planner struct {
	Provider      Provider
	MinConfidence float64
}

func DefaultExperimentCatalog() []Option {
	return []Option{
		{Action: ActionStop, Description: "Stop characterization because the current evidence is sufficient or no bounded experiment is worth the remaining request budget."},
		{Action: ActionEnumProfile, Description: "Test a small bounded set of enum-like application states already suggested by observed structure or local vocabulary."},
		{Action: ActionBooleanProfile, Description: "Test true/false semantics when the candidate plausibly represents a boolean feature or switch."},
		{Action: ActionNullabilityProfile, Description: "Test null or absence semantics when the candidate may distinguish unset from explicitly empty state."},
		{Action: ActionIntegerBoundaryProfile, Description: "Test a bounded integer boundary set such as zero, one, and a small positive value when the candidate appears numeric."},
		{Action: ActionEmptyValueProfile, Description: "Test an empty string or equivalent empty representation when that distinction could be meaningful."},
		{Action: ActionCaseVariationProfile, Description: "Test bounded case variants of an already-known semantic token when the application may be case-sensitive."},
		{Action: ActionRelatedValueProfile, Description: "Test a small bounded set of semantically related values derived from already observed application vocabulary."},
	}
}

func (p Planner) PlanNext(ctx context.Context, state State) (Plan, error) {
	if state.RemainingRequestBudget <= 0 {
		return Plan{
			SuggestedAction: ActionStop,
			AppliedAction:   ActionStop,
			Confidence:      1,
			Gated:           true,
			GateReason:      "request budget exhausted",
		}, nil
	}
	if p.Provider == nil {
		return Plan{}, fmt.Errorf("decision provider is required")
	}
	minConfidence := p.MinConfidence
	if minConfidence == 0 {
		minConfidence = DefaultMinConfidence
	}
	if minConfidence < 0 || minConfidence > 1 {
		return Plan{}, fmt.Errorf("minimum confidence must be between 0 and 1")
	}
	req := Request{
		State:   state,
		Options: DefaultExperimentCatalog(),
		Instructions: "Choose the single next bounded characterization experiment that is most informative given the deterministic ParamIntel state. " +
			"Prefer STOP when the evidence is already sufficient, the remaining request budget is too small, or none of the permitted experiments is justified. " +
			"Do not invent payloads, values, parameters, or actions outside the listed choices.",
	}
	result, err := p.Provider.Choose(ctx, req)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		SuggestedAction: result.Action,
		AppliedAction:   result.Action,
		Confidence:      result.Confidence,
		Probabilities:   result.Probabilities,
		Provider:        result.Provider,
		Model:           result.Model,
		Usage:           result.Usage,
	}
	if result.Action != ActionStop && result.Confidence < minConfidence {
		plan.AppliedAction = ActionStop
		plan.Gated = true
		plan.GateReason = fmt.Sprintf("provider confidence %.3f below %.3f threshold", result.Confidence, minConfidence)
	}
	return plan, nil
}

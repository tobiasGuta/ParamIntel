package decision

import (
	"context"
	"fmt"
)

const DefaultMinChoiceProbability = 0.0

type Plan struct {
	SuggestedAction Action             `json:"suggested_action"`
	AppliedAction   Action             `json:"applied_action"`
	Confidence          float64            `json:"confidence"`
	SelectedProbability float64            `json:"selected_probability"`
	Probabilities       map[Action]float64 `json:"probabilities,omitempty"`
	Provider        string             `json:"provider,omitempty"`
	Model           string             `json:"model,omitempty"`
	Gated           bool               `json:"gated"`
	GateReason      string             `json:"gate_reason,omitempty"`
	DecisionReason  string             `json:"decision_reason,omitempty"`
	Usage           Usage              `json:"usage,omitempty"`
}

type Planner struct {
	Provider      Provider
	MinChoiceProbability float64
}

func DefaultExperimentCatalog() []Option {
	return []Option{
		{Action: ActionStop, Description: "Choose when existing evidence is already sufficient, the signal is too weak or noisy to justify another experiment, the remaining request budget is too small, or no listed experiment is supported."},
		{Action: ActionEnumProfile, Description: "Choose when evidence suggests the parameter accepts one value from a finite application-defined set, such as allowed/supported/available states, modes, languages, currencies, providers, or similar closed choices."},
		{Action: ActionBooleanProfile, Description: "Choose when evidence suggests the parameter is a binary flag or capability with true/false, on/off, enabled/disabled, can/has/is, or equivalent two-state semantics."},
		{Action: ActionNullabilityProfile, Description: "Choose when evidence specifically suggests null, nullable, optional, unset, missing, or explicit-null semantics. Do not use merely because an empty string might matter."},
		{Action: ActionIntegerBoundaryProfile, Description: "Choose when evidence suggests a numeric quantity, count, limit, retry count, timeout, size, offset, minimum, maximum, or other bounded integer semantics."},
		{Action: ActionEmptyValueProfile, Description: "Choose when evidence specifically suggests blank, empty-string, empty-value, or present-but-empty semantics. Distinguish this from null/missing and from boolean false."},
		{Action: ActionCaseVariationProfile, Description: "Choose when evidence specifically suggests case sensitivity or case normalization for an already-known token, such as upper/lower/mixed-case handling."},
		{Action: ActionRelatedValueProfile, Description: "Choose when evidence suggests aliases, synonyms, sibling values, or nearby application vocabulary worth testing, but does not imply a closed allowed-value set. Prefer enum_profile for finite allowed/supported/available choices."},
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
			DecisionReason:  "request budget exhausted",
		}, nil
	}
	if p.Provider == nil {
		return Plan{}, fmt.Errorf("decision provider is required")
	}
	minChoiceProbability := p.MinChoiceProbability
	if minChoiceProbability < 0 || minChoiceProbability > 1 {
		return Plan{}, fmt.Errorf("minimum choice probability must be between 0 and 1")
	}
	req := Request{
		State:   state,
		Options: DefaultExperimentCatalog(),
		Instructions: "Choose the single next bounded characterization experiment that is best supported by the deterministic ParamIntel state. " +
			"Use the action definitions precisely: closed finite choices map to enum_profile; aliases or sibling vocabulary without a closed set map to related_value_profile; " +
			"null/missing semantics map to nullability_profile; blank/empty-string semantics map to empty_value_profile; binary capability semantics map to boolean_profile. " +
			"Prefer STOP when evidence is already sufficient, the signal is too weak/noisy, the remaining request budget is too small, or no permitted experiment is justified. " +
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
	selectedProbability, ok := result.Probabilities[result.Action]
	if !ok {
		return Plan{}, fmt.Errorf("provider response missing probability for selected action %q", result.Action)
	}
	plan.SelectedProbability = selectedProbability
	if result.Action != ActionStop && minChoiceProbability > 0 {
		if selectedProbability < minChoiceProbability {
			plan.AppliedAction = ActionStop
			plan.Gated = true
			plan.GateReason = fmt.Sprintf("selected action probability %.3f below %.3f threshold", selectedProbability, minChoiceProbability)
		}
	}
	return plan, nil
}

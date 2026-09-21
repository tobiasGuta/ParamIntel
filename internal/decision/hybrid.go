package decision

import (
	"context"
	"fmt"
)

type HybridPlanner struct {
	Local    HeuristicPlanner
	Provider Provider
}

func (p HybridPlanner) PlanNext(ctx context.Context, state State) (Plan, error) {
	local := p.Local.Decide(state)
	if local.Decided {
		return Plan{
			SuggestedAction: local.Action,
			AppliedAction:   local.Action,
			Confidence:      1,
			Provider:        "deterministic",
			Model:           "heuristic",
			Gated:           false,
			GateReason:      local.Reason,
		}, nil
	}

	if p.Provider == nil {
		return Plan{
			SuggestedAction: ActionStop,
			AppliedAction:   ActionStop,
			Provider:        "deterministic",
			Model:           "fail-closed",
			Gated:           true,
			GateReason:      "decision provider unavailable after deterministic abstain",
		}, nil
	}

	plan, err := (Planner{Provider: p.Provider}).PlanNext(ctx, state)
	if err != nil {
		return Plan{
			SuggestedAction: ActionStop,
			AppliedAction:   ActionStop,
			Provider:        p.Provider.Name(),
			Model:           p.Provider.Model(),
			Gated:           true,
			GateReason:      "decision provider failed closed",
		}, nil
	}

	if plan.AppliedAction == "" {
		return Plan{}, fmt.Errorf("hybrid planner produced empty applied action")
	}
	return plan, nil
}

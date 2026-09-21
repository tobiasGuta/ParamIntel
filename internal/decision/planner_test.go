package decision

import (
	"context"
	"testing"
)

type stubProvider struct {
	result Result
	calls  int
}

func (s *stubProvider) Name() string  { return "stub" }
func (s *stubProvider) Model() string { return "stub-model" }
func (s *stubProvider) Choose(context.Context, Request) (Result, error) {
	s.calls++
	return s.result, nil
}

func TestPlannerAppliesHighConfidenceChoice(t *testing.T) {
	provider := &stubProvider{result: Result{
		Action:     ActionRelatedValueProfile,
		Confidence: 0.92,
		Provider:   "stub",
		Model:      "stub-model",
		Probabilities: map[Action]float64{
			ActionRelatedValueProfile: 0.92,
			ActionStop:                0.08,
		},
	}}
	plan, err := (Planner{Provider: provider, MinChoiceProbability: 0.80}).PlanNext(context.Background(), State{
		RemainingRequestBudget: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AppliedAction != ActionRelatedValueProfile || plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestPlannerGatesLowProbabilityChoiceToStop(t *testing.T) {
	provider := &stubProvider{result: Result{
		Action:     ActionRelatedValueProfile,
		Confidence: 0.91,
		Provider:   "stub",
		Model:      "stub-model",
		Probabilities: map[Action]float64{
			ActionRelatedValueProfile: 0.61,
			ActionStop:                0.39,
		},
	}}
	plan, err := (Planner{Provider: provider, MinChoiceProbability: 0.80}).PlanNext(context.Background(), State{
		RemainingRequestBudget: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SuggestedAction != ActionRelatedValueProfile || plan.AppliedAction != ActionStop || !plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestPlannerStopsWithoutProviderWhenBudgetExhausted(t *testing.T) {
	provider := &stubProvider{}
	plan, err := (Planner{Provider: provider}).PlanNext(context.Background(), State{
		RemainingRequestBudget: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls=%d want=0", provider.calls)
	}
	if plan.AppliedAction != ActionStop || !plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
}


func TestPlannerPreservesLowConfidenceProviderStopWithoutGate(t *testing.T) {
	provider := &stubProvider{result: Result{
		Action:     ActionStop,
		Confidence: 0.49,
		Provider:   "stub",
		Model:      "stub-model",
		Probabilities: map[Action]float64{
			ActionStop:        0.50,
			ActionEnumProfile: 0.18,
		},
	}}
	plan, err := (Planner{Provider: provider, MinChoiceProbability: 0.80}).PlanNext(context.Background(), State{
		RemainingRequestBudget: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SuggestedAction != ActionStop || plan.AppliedAction != ActionStop {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Gated || plan.GateReason != "" {
		t.Fatalf("provider-selected STOP should not be labeled as gated: %+v", plan)
	}
}


func TestPlannerAppliesValidChoiceWhenNumericGateDisabled(t *testing.T) {
	provider := &stubProvider{result: Result{
		Action:     ActionEnumProfile,
		Confidence: 0.35,
		Provider:   "stub",
		Model:      "stub-model",
		Probabilities: map[Action]float64{
			ActionEnumProfile: 0.41,
			ActionStop:        0.33,
		},
	}}
	plan, err := (Planner{Provider: provider}).PlanNext(context.Background(), State{
		RemainingRequestBudget: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SuggestedAction != ActionEnumProfile || plan.AppliedAction != ActionEnumProfile {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Gated || plan.GateReason != "" {
		t.Fatalf("numeric gate should be disabled by default: %+v", plan)
	}
}

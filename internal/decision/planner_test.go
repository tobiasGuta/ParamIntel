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
	}}
	plan, err := (Planner{Provider: provider, MinConfidence: 0.80}).PlanNext(context.Background(), State{
		RemainingRequestBudget: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AppliedAction != ActionRelatedValueProfile || plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestPlannerGatesLowConfidenceChoiceToStop(t *testing.T) {
	provider := &stubProvider{result: Result{
		Action:     ActionRelatedValueProfile,
		Confidence: 0.61,
		Provider:   "stub",
		Model:      "stub-model",
	}}
	plan, err := (Planner{Provider: provider, MinConfidence: 0.80}).PlanNext(context.Background(), State{
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

package decision

import (
	"context"
	"errors"
	"testing"
)

type hybridStubProvider struct {
	result Result
	err    error
	calls  int
}

func (s *hybridStubProvider) Name() string  { return "stub" }
func (s *hybridStubProvider) Model() string { return "stub-model" }
func (s *hybridStubProvider) Choose(context.Context, Request) (Result, error) {
	s.calls++
	return s.result, s.err
}

func TestHybridPlannerUsesDeterministicDecisionFirst(t *testing.T) {
	provider := &hybridStubProvider{}
	state := State{
		Candidate:              CandidateState{Name: "notifications_enabled", ValueKind: "boolean"},
		RemainingRequestBudget: 10,
	}

	plan, err := (HybridPlanner{Provider: provider}).PlanNext(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls=%d want=0", provider.calls)
	}
	if plan.AppliedAction != ActionBooleanProfile {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Provider != "deterministic" {
		t.Fatalf("provider=%q want=deterministic", plan.Provider)
	}
}

func TestHybridPlannerUsesJevAfterDeterministicAbstainWithoutNumericGate(t *testing.T) {
	provider := &hybridStubProvider{result: Result{
		Action:     ActionRelatedValueProfile,
		Confidence: 0.35,
		Provider:   "stub",
		Model:      "stub-model",
		Probabilities: map[Action]float64{
			ActionRelatedValueProfile: 0.41,
			ActionStop:                0.33,
		},
	}}
	state := State{
		Candidate:              CandidateState{Name: "region", ValueKind: "string"},
		Evidence:               EvidenceState{Paths: []string{"$.aliases.region"}},
		RemainingRequestBudget: 10,
	}

	plan, err := (HybridPlanner{Provider: provider}).PlanNext(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d want=1", provider.calls)
	}
	if plan.SuggestedAction != ActionRelatedValueProfile || plan.AppliedAction != ActionRelatedValueProfile {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Gated {
		t.Fatalf("valid catalog choice should not be numerically gated: %+v", plan)
	}
}

func TestHybridPlannerFailsClosedOnProviderError(t *testing.T) {
	provider := &hybridStubProvider{err: errors.New("provider unavailable")}
	state := State{
		Candidate:              CandidateState{Name: "delivery", ValueKind: "string"},
		RemainingRequestBudget: 10,
	}

	plan, err := (HybridPlanner{Provider: provider}).PlanNext(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AppliedAction != ActionStop || !plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.GateReason != "decision provider failed closed" {
		t.Fatalf("gate reason=%q", plan.GateReason)
	}
}

func TestHybridPlannerFailsClosedWhenProviderMissing(t *testing.T) {
	state := State{
		Candidate:              CandidateState{Name: "delivery", ValueKind: "string"},
		RemainingRequestBudget: 10,
	}

	plan, err := (HybridPlanner{}).PlanNext(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AppliedAction != ActionStop || !plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
}

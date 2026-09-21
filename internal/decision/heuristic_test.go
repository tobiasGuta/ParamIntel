package decision

import "testing"

func TestHeuristicPlannerEasyCalibrationSet(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  Action
	}{
		{
			name: "sufficient evidence stops",
			state: State{
				Candidate: CandidateState{Name: "visibility", ValueKind: "string"},
				Verification: VerificationState{
					CandidateChanged: 3,
					CandidateTrials:  3,
					ControlChanged:   0,
					ControlTrials:    3,
					Confidence:       1,
				},
				RemainingRequestBudget: 18,
			},
			want: ActionStop,
		},
		{
			name: "boolean kind",
			state: State{
				Candidate:              CandidateState{Name: "notifications_enabled", ValueKind: "boolean"},
				RemainingRequestBudget: 18,
			},
			want: ActionBooleanProfile,
		},
		{
			name: "integer kind",
			state: State{
				Candidate:              CandidateState{Name: "page_limit", ValueKind: "integer"},
				RemainingRequestBudget: 18,
			},
			want: ActionIntegerBoundaryProfile,
		},
		{
			name: "enum status",
			state: State{
				Candidate: CandidateState{Name: "status", ValueKind: "string"},
				Evidence: EvidenceState{
					Paths: []string{"$.allowed_statuses"},
				},
				RemainingRequestBudget: 18,
			},
			want: ActionEnumProfile,
		},
		{
			name: "noisy control stops",
			state: State{
				Candidate: CandidateState{Name: "mode", ValueKind: "string"},
				Verification: VerificationState{
					CandidateChanged: 1,
					CandidateTrials:  3,
					ControlChanged:   1,
					ControlTrials:    3,
				},
				RemainingRequestBudget: 12,
			},
			want: ActionStop,
		},
	}

	planner := HeuristicPlanner{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := planner.Plan(tt.state); got != tt.want {
				t.Fatalf("Plan()=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestHeuristicPlannerStopsWhenBudgetExhausted(t *testing.T) {
	state := State{
		Candidate:              CandidateState{Name: "status", ValueKind: "string"},
		RemainingRequestBudget: 0,
	}
	if got := (HeuristicPlanner{}).Plan(state); got != ActionStop {
		t.Fatalf("Plan()=%q want=%q", got, ActionStop)
	}
}


func TestHeuristicPlannerAbstainsWhenNoRuleMatches(t *testing.T) {
	state := State{
		Candidate:              CandidateState{Name: "region", ValueKind: "string"},
		Evidence:               EvidenceState{Paths: []string{"$.aliases.region"}},
		RemainingRequestBudget: 18,
	}
	decision := (HeuristicPlanner{}).Decide(state)
	if decision.Decided {
		t.Fatalf("expected abstain, got %+v", decision)
	}
	if decision.Action != ActionStop {
		t.Fatalf("abstain action=%q want=%q", decision.Action, ActionStop)
	}
	if decision.Reason == "" {
		t.Fatal("abstain reason is empty")
	}
}


func TestHeuristicPlannerStructuralEvidenceRules(t *testing.T) {
	tests := []struct {
		name      string
		candidate CandidateState
		evidence  EvidenceState
		want      Action
	}{
		{
			name:      "supported plural enum",
			candidate: CandidateState{Name: "locale", ValueKind: "string"},
			evidence:  EvidenceState{Paths: []string{"$.supported_locales"}},
			want:      ActionEnumProfile,
		},
		{
			name:      "available plural enum",
			candidate: CandidateState{Name: "theme", ValueKind: "string"},
			evidence:  EvidenceState{Paths: []string{"$.available_themes"}},
			want:      ActionEnumProfile,
		},
		{
			name:      "supported compound enum",
			candidate: CandidateState{Name: "delivery", ValueKind: "string"},
			evidence:  EvidenceState{Paths: []string{"$.supported_delivery_methods"}},
			want:      ActionEnumProfile,
		},
		{
			name:      "can prefix boolean",
			candidate: CandidateState{Name: "archive", ValueKind: "string"},
			evidence:  EvidenceState{Paths: []string{"$.capabilities.can_archive"}},
			want:      ActionBooleanProfile,
		},
		{
			name:      "supported suffix boolean",
			candidate: CandidateState{Name: "read_only", ValueKind: "string"},
			evidence:  EvidenceState{Paths: []string{"$.capabilities.read_only_supported"}},
			want:      ActionBooleanProfile,
		},
		{
			name:      "max prefix integer",
			candidate: CandidateState{Name: "timeout", ValueKind: "string"},
			evidence:  EvidenceState{Paths: []string{"$.limits.max_timeout_seconds"}},
			want:      ActionIntegerBoundaryProfile,
		},
	}

	planner := HeuristicPlanner{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := planner.Decide(State{
				Candidate:              tt.candidate,
				Evidence:               tt.evidence,
				RemainingRequestBudget: 18,
			})
			if !decision.Decided {
				t.Fatalf("expected deterministic decision, got abstain: %+v", decision)
			}
			if decision.Action != tt.want {
				t.Fatalf("action=%q want=%q reason=%q", decision.Action, tt.want, decision.Reason)
			}
		})
	}
}

func TestHeuristicPlannerStructuralEvidenceDoesNotGuessWithoutRelationMarker(t *testing.T) {
	decision := (HeuristicPlanner{}).Decide(State{
		Candidate:              CandidateState{Name: "next_token", ValueKind: "string"},
		Evidence:               EvidenceState{Paths: []string{"$.pagination.next_token"}},
		RemainingRequestBudget: 18,
	})
	if decision.Decided {
		t.Fatalf("expected abstain, got %+v", decision)
	}
}

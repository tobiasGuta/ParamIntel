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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/decision"
)

type manifest struct {
	Cases []benchmarkCase `json:"cases"`
}

type benchmarkCase struct {
	Name           string          `json:"name"`
	StatePath      string          `json:"state"`
	ExpectedAction decision.Action `json:"expected_action"`
}

type caseResult struct {
	Name                    string          `json:"name"`
	ExpectedAction          decision.Action `json:"expected_action"`
	HeuristicAction         decision.Action `json:"heuristic_action"`
	HeuristicDecided        bool            `json:"heuristic_decided"`
	HeuristicReason         string          `json:"heuristic_reason"`
	HeuristicCorrect        bool            `json:"heuristic_correct"`
	JevModalAction          decision.Action `json:"jev_modal_action"`
	JevModalRate            float64         `json:"jev_modal_rate"`
	JevExpectedRate         float64         `json:"jev_expected_rate"`
	JevAppliedModalAction   decision.Action `json:"jev_applied_modal_action"`
	JevAppliedModalRate     float64         `json:"jev_applied_modal_rate"`
	JevAppliedExpectedRate  float64         `json:"jev_applied_expected_rate"`
	JevGatedRate            float64         `json:"jev_gated_rate"`
	SelectedProbabilityMean float64         `json:"selected_probability_mean"`
	DecisionMarginMean      float64         `json:"decision_margin_mean"`
	LatencyMSMean           float64         `json:"latency_ms_mean"`
	HybridAction            decision.Action `json:"hybrid_action"`
	HybridExpectedRate      float64         `json:"hybrid_expected_rate"`
	HybridAppliedAction     decision.Action `json:"hybrid_applied_action"`
	HybridAppliedExpectedRate float64       `json:"hybrid_applied_expected_rate"`
	HybridUsesJev           bool            `json:"hybrid_uses_jev"`
}

type gatePolicyResult struct {
	Name                    string  `json:"name"`
	MinDecisionMargin       float64 `json:"min_decision_margin"`
	JevExpectedDecisions    int     `json:"jev_expected_decisions"`
	JevExpectedRate         float64 `json:"jev_expected_rate"`
	HybridExpectedDecisions int     `json:"hybrid_expected_decisions"`
	HybridExpectedRate      float64 `json:"hybrid_expected_rate"`
	GatedNonStopDecisions   int     `json:"gated_non_stop_decisions"`
}

type report struct {
	Cases                  int          `json:"cases"`
	JevRunsPerCase         int          `json:"jev_runs_per_case"`
	HeuristicCorrectCases        int          `json:"heuristic_correct_cases"`
	HeuristicAccuracy            float64      `json:"heuristic_accuracy"`
	HeuristicDecidedCases        int          `json:"heuristic_decided_cases"`
	HeuristicCoverage            float64      `json:"heuristic_coverage"`
	HeuristicAccuracyWhenDecided float64      `json:"heuristic_accuracy_when_decided"`
	JevExpectedDecisions         int          `json:"jev_expected_decisions"`
	JevAppliedExpectedDecisions  int          `json:"jev_applied_expected_decisions"`
	JevTotalDecisions            int          `json:"jev_total_decisions"`
	JevExpectedRate              float64      `json:"jev_expected_rate"`
	JevAppliedExpectedRate       float64      `json:"jev_applied_expected_rate"`
	JevGatedDecisions            int          `json:"jev_gated_decisions"`
	JevGatedRate                 float64      `json:"jev_gated_rate"`
	HybridExpectedDecisions      int          `json:"hybrid_expected_decisions"`
	HybridAppliedExpectedDecisions int        `json:"hybrid_applied_expected_decisions"`
	HybridTotalDecisions         int          `json:"hybrid_total_decisions"`
	HybridExpectedRate           float64      `json:"hybrid_expected_rate"`
	HybridAppliedExpectedRate    float64      `json:"hybrid_applied_expected_rate"`
	HybridJevFallbackCalls       int          `json:"hybrid_jev_fallback_calls"`
	HybridJevFallbackRate        float64      `json:"hybrid_jev_fallback_rate"`
	ShadowGatePolicies           []gatePolicyResult `json:"shadow_gate_policies"`
	CaseResults                  []caseResult      `json:"case_results"`
}

func main() {
	var manifestPath, model string
	var runs int
	var timeout time.Duration

	flag.StringVar(&manifestPath, "manifest", `./labs/typesafe-decision-provider/benchmark.json`, "benchmark manifest")
	flag.StringVar(&model, "model", decision.DefaultTypeSafeModel, "TypeSafe model")
	flag.IntVar(&runs, "runs", 5, "Jev runs per case (1-20)")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "TypeSafe API timeout per run")
	flag.Parse()

	if runs < 1 || runs > 20 {
		fatal(fmt.Errorf("-runs must be between 1 and 20"))
	}
	if timeout <= 0 {
		fatal(fmt.Errorf("-timeout must be greater than zero"))
	}
	apiKey := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	if apiKey == "" {
		fatal(fmt.Errorf("TYPESAFE_API_KEY is required"))
	}

	raw, err := os.ReadFile(manifestPath)
	fatal(err)
	var m manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	fatal(dec.Decode(&m))
	if len(m.Cases) == 0 {
		fatal(fmt.Errorf("benchmark manifest has no cases"))
	}

	provider, err := decision.NewTypeSafeProvider(decision.TypeSafeConfig{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: timeout},
	})
	fatal(err)
	planner := decision.Planner{Provider: provider}
	heuristic := decision.HeuristicPlanner{}

	results := make([]caseResult, 0, len(m.Cases))
	heuristicCorrect := 0
	heuristicDecided := 0
	jevExpected := 0
	jevAppliedExpected := 0
	jevGated := 0
	jevTotal := 0
	hybridExpected := 0
	hybridAppliedExpected := 0
	hybridTotal := 0
	hybridJevCalls := 0

	marginThresholds := []float64{0, 0.03, 0.05, 0.08, 0.10, 0.15, 0.20, 0.30}
	type gateAccumulator struct {
		jevExpected    int
		hybridExpected int
		gatedNonStop   int
	}
	gateAcc := make(map[float64]*gateAccumulator, len(marginThresholds))
	for _, threshold := range marginThresholds {
		gateAcc[threshold] = &gateAccumulator{}
	}

	for _, bc := range m.Cases {
		stateRaw, err := os.ReadFile(filepath.Clean(bc.StatePath))
		if err != nil {
			fatal(fmt.Errorf("%s: read state: %w", bc.Name, err))
		}

		var state decision.State
		stateDecoder := json.NewDecoder(strings.NewReader(string(stateRaw)))
		stateDecoder.DisallowUnknownFields()
		if err := stateDecoder.Decode(&state); err != nil {
			fatal(fmt.Errorf("%s: decode state: %w", bc.Name, err))
		}

		hDecision := heuristic.Decide(state)
		hAction := hDecision.Action
		hCorrect := hDecision.Decided && hAction == bc.ExpectedAction
		if hDecision.Decided {
			heuristicDecided++
		}
		if hCorrect {
			heuristicCorrect++
		}

		counts := map[decision.Action]int{}
		appliedCounts := map[decision.Action]int{}
		var selectedProbabilitySum, marginSum float64
		var latencySum int64
		caseGated := 0

		for i := 0; i < runs; i++ {
			started := time.Now()
			plan, err := planner.PlanNext(context.Background(), state)
			if err != nil {
				fatal(fmt.Errorf("%s: Jev run %d: %w", bc.Name, i+1, err))
			}
			latencySum += time.Since(started).Milliseconds()

			counts[plan.SuggestedAction]++
			appliedCounts[plan.AppliedAction]++
			if plan.SuggestedAction == bc.ExpectedAction {
				jevExpected++
			}
			if plan.AppliedAction == bc.ExpectedAction {
				jevAppliedExpected++
			}
			if plan.Gated {
				jevGated++
				caseGated++
			}
			jevTotal++
			selectedProbabilitySum += plan.SelectedProbability

			runnerUp := 0.0
			for action, probability := range plan.Probabilities {
				if action == plan.SuggestedAction {
					continue
				}
				if probability > runnerUp {
					runnerUp = probability
				}
			}
			margin := plan.SelectedProbability - runnerUp
			marginSum += margin

			for _, threshold := range marginThresholds {
				acc := gateAcc[threshold]
				shadowAction := plan.SuggestedAction
				if shadowAction != decision.ActionStop && margin < threshold {
					shadowAction = decision.ActionStop
					acc.gatedNonStop++
				}
				if shadowAction == bc.ExpectedAction {
					acc.jevExpected++
				}
				if hDecision.Decided {
					if hDecision.Action == bc.ExpectedAction {
						acc.hybridExpected++
					}
				} else if shadowAction == bc.ExpectedAction {
					acc.hybridExpected++
				}
			}
		}

		modalAction, modalCount := modal(counts)
		appliedModalAction, appliedModalCount := modal(appliedCounts)
		jevExpectedRate := float64(counts[bc.ExpectedAction]) / float64(runs)
		jevAppliedExpectedRate := float64(appliedCounts[bc.ExpectedAction]) / float64(runs)
		jevGatedRate := float64(caseGated) / float64(runs)
		hybridAction := modalAction
		hybridExpectedRate := jevExpectedRate
		hybridAppliedAction := appliedModalAction
		hybridAppliedExpectedRate := jevAppliedExpectedRate
		hybridUsesJev := true
		if hDecision.Decided {
			hybridAction = hAction
			hybridUsesJev = false
			if hAction == bc.ExpectedAction {
				hybridExpectedRate = 1
				hybridAppliedExpectedRate = 1
				hybridExpected += runs
				hybridAppliedExpected += runs
			} else {
				hybridExpectedRate = 0
				hybridAppliedExpectedRate = 0
			}
			hybridAppliedAction = hAction
		} else {
			hybridExpected += counts[bc.ExpectedAction]
			hybridAppliedExpected += appliedCounts[bc.ExpectedAction]
			hybridJevCalls += runs
		}
		hybridTotal += runs

		results = append(results, caseResult{
			Name:                    bc.Name,
			ExpectedAction:          bc.ExpectedAction,
			HeuristicAction:         hAction,
			HeuristicDecided:        hDecision.Decided,
			HeuristicReason:         hDecision.Reason,
			HeuristicCorrect:        hCorrect,
			JevModalAction:          modalAction,
			JevModalRate:            float64(modalCount) / float64(runs),
			JevExpectedRate:         jevExpectedRate,
			JevAppliedModalAction:   appliedModalAction,
			JevAppliedModalRate:     float64(appliedModalCount) / float64(runs),
			JevAppliedExpectedRate:  jevAppliedExpectedRate,
			JevGatedRate:            jevGatedRate,
			SelectedProbabilityMean: selectedProbabilitySum / float64(runs),
			DecisionMarginMean:      marginSum / float64(runs),
			LatencyMSMean:           float64(latencySum) / float64(runs),
			HybridAction:            hybridAction,
			HybridExpectedRate:      hybridExpectedRate,
			HybridAppliedAction:     hybridAppliedAction,
			HybridAppliedExpectedRate: hybridAppliedExpectedRate,
			HybridUsesJev:           hybridUsesJev,
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })

	heuristicAccuracyWhenDecided := 0.0
	if heuristicDecided > 0 {
		heuristicAccuracyWhenDecided = float64(heuristicCorrect) / float64(heuristicDecided)
	}
	shadowPolicies := make([]gatePolicyResult, 0, len(marginThresholds))
	for _, threshold := range marginThresholds {
		acc := gateAcc[threshold]
		name := fmt.Sprintf("margin_%.2f", threshold)
		if threshold == 0 {
			name = "no_margin_gate"
		}
		shadowPolicies = append(shadowPolicies, gatePolicyResult{
			Name:                    name,
			MinDecisionMargin:       threshold,
			JevExpectedDecisions:    acc.jevExpected,
			JevExpectedRate:         float64(acc.jevExpected) / float64(jevTotal),
			HybridExpectedDecisions: acc.hybridExpected,
			HybridExpectedRate:      float64(acc.hybridExpected) / float64(hybridTotal),
			GatedNonStopDecisions:   acc.gatedNonStop,
		})
	}

	out := report{
		Cases:                        len(m.Cases),
		JevRunsPerCase:               runs,
		HeuristicCorrectCases:        heuristicCorrect,
		HeuristicAccuracy:            float64(heuristicCorrect) / float64(len(m.Cases)),
		HeuristicDecidedCases:        heuristicDecided,
		HeuristicCoverage:            float64(heuristicDecided) / float64(len(m.Cases)),
		HeuristicAccuracyWhenDecided: heuristicAccuracyWhenDecided,
		JevExpectedDecisions:         jevExpected,
		JevAppliedExpectedDecisions:  jevAppliedExpected,
		JevTotalDecisions:            jevTotal,
		JevExpectedRate:              float64(jevExpected) / float64(jevTotal),
		JevAppliedExpectedRate:       float64(jevAppliedExpected) / float64(jevTotal),
		JevGatedDecisions:            jevGated,
		JevGatedRate:                 float64(jevGated) / float64(jevTotal),
		HybridExpectedDecisions:      hybridExpected,
		HybridAppliedExpectedDecisions: hybridAppliedExpected,
		HybridTotalDecisions:         hybridTotal,
		HybridExpectedRate:           float64(hybridExpected) / float64(hybridTotal),
		HybridAppliedExpectedRate:    float64(hybridAppliedExpected) / float64(hybridTotal),
		HybridJevFallbackCalls:       hybridJevCalls,
		HybridJevFallbackRate:        float64(hybridJevCalls) / float64(hybridTotal),
		ShadowGatePolicies:           shadowPolicies,
		CaseResults:                  results,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	fatal(err)
	fmt.Println(string(b))
}

func modal(counts map[decision.Action]int) (decision.Action, int) {
	var best decision.Action
	bestCount := -1
	for action, count := range counts {
		if count > bestCount || (count == bestCount && string(action) < string(best)) {
			best = action
			bestCount = count
		}
	}
	return best, bestCount
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

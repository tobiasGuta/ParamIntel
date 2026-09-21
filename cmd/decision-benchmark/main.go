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
	HeuristicCorrect        bool            `json:"heuristic_correct"`
	JevModalAction          decision.Action `json:"jev_modal_action"`
	JevModalRate            float64         `json:"jev_modal_rate"`
	JevExpectedRate         float64         `json:"jev_expected_rate"`
	SelectedProbabilityMean float64         `json:"selected_probability_mean"`
	DecisionMarginMean      float64         `json:"decision_margin_mean"`
	LatencyMSMean           float64         `json:"latency_ms_mean"`
}

type report struct {
	Cases                  int          `json:"cases"`
	JevRunsPerCase         int          `json:"jev_runs_per_case"`
	HeuristicCorrectCases  int          `json:"heuristic_correct_cases"`
	HeuristicAccuracy      float64      `json:"heuristic_accuracy"`
	JevExpectedDecisions   int          `json:"jev_expected_decisions"`
	JevTotalDecisions      int          `json:"jev_total_decisions"`
	JevExpectedRate        float64      `json:"jev_expected_rate"`
	CaseResults            []caseResult `json:"case_results"`
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
	jevExpected := 0
	jevTotal := 0

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

		hAction := heuristic.Plan(state)
		hCorrect := hAction == bc.ExpectedAction
		if hCorrect {
			heuristicCorrect++
		}

		counts := map[decision.Action]int{}
		var selectedProbabilitySum, marginSum float64
		var latencySum int64

		for i := 0; i < runs; i++ {
			started := time.Now()
			plan, err := planner.PlanNext(context.Background(), state)
			if err != nil {
				fatal(fmt.Errorf("%s: Jev run %d: %w", bc.Name, i+1, err))
			}
			latencySum += time.Since(started).Milliseconds()

			counts[plan.SuggestedAction]++
			if plan.SuggestedAction == bc.ExpectedAction {
				jevExpected++
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
			marginSum += plan.SelectedProbability - runnerUp
		}

		modalAction, modalCount := modal(counts)
		results = append(results, caseResult{
			Name:                    bc.Name,
			ExpectedAction:          bc.ExpectedAction,
			HeuristicAction:         hAction,
			HeuristicCorrect:        hCorrect,
			JevModalAction:          modalAction,
			JevModalRate:            float64(modalCount) / float64(runs),
			JevExpectedRate:         float64(counts[bc.ExpectedAction]) / float64(runs),
			SelectedProbabilityMean: selectedProbabilitySum / float64(runs),
			DecisionMarginMean:      marginSum / float64(runs),
			LatencyMSMean:           float64(latencySum) / float64(runs),
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })

	out := report{
		Cases:                 len(m.Cases),
		JevRunsPerCase:        runs,
		HeuristicCorrectCases: heuristicCorrect,
		HeuristicAccuracy:     float64(heuristicCorrect) / float64(len(m.Cases)),
		JevExpectedDecisions:  jevExpected,
		JevTotalDecisions:     jevTotal,
		JevExpectedRate:       float64(jevExpected) / float64(jevTotal),
		CaseResults:           results,
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

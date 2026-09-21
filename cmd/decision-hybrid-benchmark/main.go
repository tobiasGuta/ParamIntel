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
	Name              string          `json:"name"`
	ExpectedAction    decision.Action `json:"expected_action"`
	ModalAction       decision.Action `json:"modal_action"`
	ModalRate         float64         `json:"modal_rate"`
	ModalCorrect      bool            `json:"modal_correct"`
	ExpectedRate      float64         `json:"expected_rate"`
	DeterministicRate float64         `json:"deterministic_rate"`
	ProviderRate      float64         `json:"provider_rate"`
	FailClosedRate    float64         `json:"fail_closed_rate"`
	LatencyMSMean     float64         `json:"latency_ms_mean"`
}

type report struct {
	Cases                  int          `json:"cases"`
	RunsPerCase            int          `json:"runs_per_case"`
	ExpectedDecisions      int          `json:"expected_decisions"`
	TotalDecisions         int          `json:"total_decisions"`
	ExpectedRate           float64      `json:"expected_rate"`
	ModalCorrectCases      int          `json:"modal_correct_cases"`
	ModalAccuracy          float64      `json:"modal_accuracy"`
	DeterministicDecisions int          `json:"deterministic_decisions"`
	ProviderDecisions      int          `json:"provider_decisions"`
	FailClosedDecisions    int          `json:"fail_closed_decisions"`
	CaseResults            []caseResult `json:"case_results"`
}

func main() {
	var manifestPath, model string
	var runs int
	var timeout time.Duration

	flag.StringVar(&manifestPath, "manifest", "./labs/typesafe-decision-provider/benchmark-holdout-v2.json", "benchmark manifest")
	flag.StringVar(&model, "model", decision.DefaultTypeSafeModel, "TypeSafe model")
	flag.IntVar(&runs, "runs", 5, "hybrid runs per case (1-20)")
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

	planner := decision.HybridPlanner{
		Local:    decision.HeuristicPlanner{},
		Provider: provider,
	}

	results := make([]caseResult, 0, len(m.Cases))
	expectedDecisions := 0
	totalDecisions := 0
	modalCorrectCases := 0
	deterministicDecisions := 0
	providerDecisions := 0
	failClosedDecisions := 0

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

		counts := map[decision.Action]int{}
		caseExpected := 0
		caseDeterministic := 0
		caseProvider := 0
		caseFailClosed := 0
		var latencySum int64

		for i := 0; i < runs; i++ {
			started := time.Now()
			plan, err := planner.PlanNext(context.Background(), state)
			if err != nil {
				fatal(fmt.Errorf("%s: hybrid run %d: %w", bc.Name, i+1, err))
			}
			latencySum += time.Since(started).Milliseconds()

			counts[plan.AppliedAction]++
			totalDecisions++
			if plan.AppliedAction == bc.ExpectedAction {
				expectedDecisions++
				caseExpected++
			}

			switch plan.Provider {
			case "deterministic":
				if plan.Model == "heuristic" {
					deterministicDecisions++
					caseDeterministic++
				} else {
					failClosedDecisions++
					caseFailClosed++
				}
			default:
				if plan.Gated && strings.Contains(plan.GateReason, "failed closed") {
					failClosedDecisions++
					caseFailClosed++
				} else {
					providerDecisions++
					caseProvider++
				}
			}
		}

		modalAction, modalCount := modal(counts)
		modalCorrect := modalAction == bc.ExpectedAction
		if modalCorrect {
			modalCorrectCases++
		}

		results = append(results, caseResult{
			Name:              bc.Name,
			ExpectedAction:    bc.ExpectedAction,
			ModalAction:       modalAction,
			ModalRate:         float64(modalCount) / float64(runs),
			ModalCorrect:      modalCorrect,
			ExpectedRate:      float64(caseExpected) / float64(runs),
			DeterministicRate: float64(caseDeterministic) / float64(runs),
			ProviderRate:      float64(caseProvider) / float64(runs),
			FailClosedRate:    float64(caseFailClosed) / float64(runs),
			LatencyMSMean:     float64(latencySum) / float64(runs),
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })

	out := report{
		Cases:                  len(m.Cases),
		RunsPerCase:            runs,
		ExpectedDecisions:      expectedDecisions,
		TotalDecisions:         totalDecisions,
		ExpectedRate:           float64(expectedDecisions) / float64(totalDecisions),
		ModalCorrectCases:      modalCorrectCases,
		ModalAccuracy:          float64(modalCorrectCases) / float64(len(m.Cases)),
		DeterministicDecisions: deterministicDecisions,
		ProviderDecisions:      providerDecisions,
		FailClosedDecisions:    failClosedDecisions,
		CaseResults:            results,
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

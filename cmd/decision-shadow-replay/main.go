package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/decision"
)

type dataset struct {
	SchemaVersion   int           `json:"schema_version"`
	Source          string        `json:"source"`
	CapturedRecords int           `json:"captured_records"`
	UniqueCases     int           `json:"unique_cases"`
	Cases           []datasetCase `json:"cases"`
}

type datasetCase struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	State          decision.State  `json:"state"`
	ExpectedAction decision.Action `json:"expected_action"`
	LabelNotes     string          `json:"label_notes,omitempty"`
}

type caseResult struct {
	ID                string          `json:"id"`
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
	InputTokens            int          `json:"input_tokens"`
	OutputTokens           int          `json:"output_tokens"`
	LatencyMSMean          float64      `json:"latency_ms_mean"`
	CaseResults            []caseResult `json:"case_results"`
}

func main() {
	var datasetPath, model, outputPath string
	var runs int
	var timeout time.Duration
	var validateOnly bool

	flag.StringVar(&datasetPath, "dataset", "./.paramintel/decision-shadow-dataset.json", "frozen labeled shadow dataset")
	flag.StringVar(&model, "model", decision.DefaultTypeSafeModel, "TypeSafe model")
	flag.StringVar(&outputPath, "output", "", "optional JSON report output; stdout if empty")
	flag.IntVar(&runs, "runs", 5, "hybrid runs per case (1-20)")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "TypeSafe API timeout per run")
	flag.BoolVar(&validateOnly, "validate-only", false, "validate frozen IDs and expert labels without calling TypeSafe")
	flag.Parse()

	if runs < 1 || runs > 20 {
		fatal(fmt.Errorf("-runs must be between 1 and 20"))
	}
	if timeout <= 0 {
		fatal(fmt.Errorf("-timeout must be greater than zero"))
	}

	raw, err := os.ReadFile(datasetPath)
	fatal(err)

	var d dataset
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	fatal(dec.Decode(&d))
	fatal(validateDataset(d))

	if validateOnly {
		fmt.Printf("dataset valid: %d labeled cases\n", len(d.Cases))
		return
	}

	apiKey := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	if apiKey == "" {
		fatal(fmt.Errorf("TYPESAFE_API_KEY is required unless -validate-only is used"))
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

	results := make([]caseResult, 0, len(d.Cases))
	expectedDecisions := 0
	totalDecisions := 0
	modalCorrectCases := 0
	deterministicDecisions := 0
	providerDecisions := 0
	failClosedDecisions := 0
	inputTokens := 0
	outputTokens := 0
	var latencyTotalMS int64

	for _, bc := range d.Cases {
		counts := map[decision.Action]int{}
		caseExpected := 0
		caseDeterministic := 0
		caseProvider := 0
		caseFailClosed := 0
		var caseLatencyMS int64

		for i := 0; i < runs; i++ {
			started := time.Now()
			plan, err := planner.PlanNext(context.Background(), bc.State)
			if err != nil {
				fatal(fmt.Errorf("%s: hybrid run %d: %w", bc.Name, i+1, err))
			}
			elapsed := time.Since(started).Milliseconds()
			caseLatencyMS += elapsed
			latencyTotalMS += elapsed

			counts[plan.AppliedAction]++
			totalDecisions++
			inputTokens += plan.Usage.InputTokens
			outputTokens += plan.Usage.OutputTokens

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
			ID:                bc.ID,
			Name:              bc.Name,
			ExpectedAction:    bc.ExpectedAction,
			ModalAction:       modalAction,
			ModalRate:         float64(modalCount) / float64(runs),
			ModalCorrect:      modalCorrect,
			ExpectedRate:      float64(caseExpected) / float64(runs),
			DeterministicRate: float64(caseDeterministic) / float64(runs),
			ProviderRate:      float64(caseProvider) / float64(runs),
			FailClosedRate:    float64(caseFailClosed) / float64(runs),
			LatencyMSMean:     float64(caseLatencyMS) / float64(runs),
		})
	}

	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })

	out := report{
		Cases:                  len(d.Cases),
		RunsPerCase:            runs,
		ExpectedDecisions:      expectedDecisions,
		TotalDecisions:         totalDecisions,
		ExpectedRate:           float64(expectedDecisions) / float64(totalDecisions),
		ModalCorrectCases:      modalCorrectCases,
		ModalAccuracy:          float64(modalCorrectCases) / float64(len(d.Cases)),
		DeterministicDecisions: deterministicDecisions,
		ProviderDecisions:      providerDecisions,
		FailClosedDecisions:    failClosedDecisions,
		InputTokens:            inputTokens,
		OutputTokens:           outputTokens,
		LatencyMSMean:          float64(latencyTotalMS) / float64(totalDecisions),
		CaseResults:            results,
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	fatal(err)
	encoded = append(encoded, '\n')
	if strings.TrimSpace(outputPath) != "" {
		fatal(os.WriteFile(outputPath, encoded, 0600))
		fmt.Printf("wrote %s\n", outputPath)
		return
	}
	_, _ = os.Stdout.Write(encoded)
}

func validateDataset(d dataset) error {
	if d.SchemaVersion != 1 {
		return fmt.Errorf("unsupported dataset schema version %d", d.SchemaVersion)
	}
	if strings.TrimSpace(d.Source) != "paramintel_decision_shadow_frozen" {
		return fmt.Errorf("unexpected dataset source %q", d.Source)
	}
	if len(d.Cases) == 0 {
		return fmt.Errorf("dataset has no cases")
	}
	if d.UniqueCases != len(d.Cases) {
		return fmt.Errorf("unique_cases=%d does not match cases=%d", d.UniqueCases, len(d.Cases))
	}

	allowed := map[decision.Action]struct{}{}
	for _, option := range decision.DefaultExperimentCatalog() {
		allowed[option.Action] = struct{}{}
	}

	seen := map[string]struct{}{}
	for i, bc := range d.Cases {
		if strings.TrimSpace(bc.ID) == "" {
			return fmt.Errorf("case %d: id is required", i+1)
		}
		if _, ok := seen[bc.ID]; ok {
			return fmt.Errorf("case %d: duplicate id %q", i+1, bc.ID)
		}
		seen[bc.ID] = struct{}{}

		recomputed, err := decision.NewShadowCaptureRecord(bc.State)
		if err != nil {
			return fmt.Errorf("case %d: recompute id: %w", i+1, err)
		}
		if recomputed.ID != bc.ID {
			return fmt.Errorf("case %d: id mismatch: got %q want %q", i+1, bc.ID, recomputed.ID)
		}
		if _, ok := allowed[bc.ExpectedAction]; !ok {
			if bc.ExpectedAction == "" {
				return fmt.Errorf("case %d (%s): expected_action is unlabeled", i+1, bc.ID)
			}
			return fmt.Errorf("case %d (%s): unknown expected_action %q", i+1, bc.ID, bc.ExpectedAction)
		}
	}
	return nil
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

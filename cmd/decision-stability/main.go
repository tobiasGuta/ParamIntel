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

type observation struct {
	Run                 int             `json:"run"`
	SuggestedAction     decision.Action `json:"suggested_action"`
	AppliedAction       decision.Action `json:"applied_action"`
	Confidence          float64         `json:"confidence"`
	SelectedProbability float64         `json:"selected_probability"`
	RunnerUpAction      decision.Action `json:"runner_up_action,omitempty"`
	RunnerUpProbability float64         `json:"runner_up_probability"`
	DecisionMargin      float64         `json:"decision_margin"`
	Gated               bool            `json:"gated"`
	LatencyMS           int64           `json:"latency_ms"`
}

type actionCount struct {
	Action decision.Action `json:"action"`
	Count  int             `json:"count"`
	Rate   float64         `json:"rate"`
}

type summary struct {
	Runs                       int           `json:"runs"`
	Choices                    []actionCount `json:"choices"`
	ModalAction                decision.Action `json:"modal_action"`
	ModalRate                  float64       `json:"modal_rate"`
	StopRate                   float64       `json:"stop_rate"`
	GatedRate                  float64       `json:"gated_rate"`
	SelectedProbabilityMin     float64       `json:"selected_probability_min"`
	SelectedProbabilityMean    float64       `json:"selected_probability_mean"`
	SelectedProbabilityMax     float64       `json:"selected_probability_max"`
	DecisionMarginMin          float64       `json:"decision_margin_min"`
	DecisionMarginMean         float64       `json:"decision_margin_mean"`
	DecisionMarginMax          float64       `json:"decision_margin_max"`
	ConfidenceMean             float64       `json:"confidence_mean"`
	LatencyMSMin               int64         `json:"latency_ms_min"`
	LatencyMSMean              float64       `json:"latency_ms_mean"`
	LatencyMSMax               int64         `json:"latency_ms_max"`
	Observations               []observation `json:"observations"`
}

func main() {
	var statePath, model string
	var runs int
	var minChoiceProbability float64
	var timeout time.Duration

	flag.StringVar(&statePath, "state", "", "sanitized decision-state JSON file")
	flag.StringVar(&model, "model", decision.DefaultTypeSafeModel, "TypeSafe model")
	flag.IntVar(&runs, "runs", 10, "number of sequential Jev decisions to sample (1-50)")
	flag.Float64Var(&minChoiceProbability, "min-choice-probability", decision.DefaultMinChoiceProbability, "optional selected-action probability gate for non-STOP decisions; 0 disables numeric gating")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "TypeSafe API timeout per run")
	flag.Parse()

	if strings.TrimSpace(statePath) == "" {
		fatal(fmt.Errorf("-state is required"))
	}
	if runs < 1 || runs > 50 {
		fatal(fmt.Errorf("-runs must be between 1 and 50"))
	}
	if minChoiceProbability < 0 || minChoiceProbability > 1 {
		fatal(fmt.Errorf("-min-choice-probability must be between 0 and 1"))
	}
	if timeout <= 0 {
		fatal(fmt.Errorf("-timeout must be greater than zero"))
	}

	apiKey := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
	if apiKey == "" {
		fatal(fmt.Errorf("TYPESAFE_API_KEY is required"))
	}

	raw, err := os.ReadFile(statePath)
	fatal(err)
	var state decision.State
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	fatal(dec.Decode(&state))

	provider, err := decision.NewTypeSafeProvider(decision.TypeSafeConfig{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: timeout},
	})
	fatal(err)

	planner := decision.Planner{
		Provider:              provider,
		MinChoiceProbability: minChoiceProbability,
	}

	observations := make([]observation, 0, runs)
	for i := 1; i <= runs; i++ {
		started := time.Now()
		plan, err := planner.PlanNext(context.Background(), state)
		fatal(err)
		runnerUpAction, runnerUpProbability := runnerUp(plan.Probabilities, plan.SuggestedAction)
		observations = append(observations, observation{
			Run:                 i,
			SuggestedAction:     plan.SuggestedAction,
			AppliedAction:       plan.AppliedAction,
			Confidence:          plan.Confidence,
			SelectedProbability: plan.SelectedProbability,
			RunnerUpAction:      runnerUpAction,
			RunnerUpProbability: runnerUpProbability,
			DecisionMargin:      plan.SelectedProbability - runnerUpProbability,
			Gated:               plan.Gated,
			LatencyMS:           time.Since(started).Milliseconds(),
		})
	}

	out, err := json.MarshalIndent(summarize(observations), "", "  ")
	fatal(err)
	fmt.Println(string(out))
}

func summarize(observations []observation) summary {
	counts := map[decision.Action]int{}
	var stopCount, gatedCount int
	var probabilitySum, marginSum, confidenceSum float64
	var latencySum int64

	minProbability := observations[0].SelectedProbability
	maxProbability := minProbability
	minMargin := observations[0].DecisionMargin
	maxMargin := minMargin
	minLatency := observations[0].LatencyMS
	maxLatency := minLatency

	for _, o := range observations {
		counts[o.SuggestedAction]++
		if o.SuggestedAction == decision.ActionStop {
			stopCount++
		}
		if o.Gated {
			gatedCount++
		}
		probabilitySum += o.SelectedProbability
		marginSum += o.DecisionMargin
		confidenceSum += o.Confidence
		latencySum += o.LatencyMS
		if o.SelectedProbability < minProbability {
			minProbability = o.SelectedProbability
		}
		if o.SelectedProbability > maxProbability {
			maxProbability = o.SelectedProbability
		}
		if o.DecisionMargin < minMargin {
			minMargin = o.DecisionMargin
		}
		if o.DecisionMargin > maxMargin {
			maxMargin = o.DecisionMargin
		}
		if o.LatencyMS < minLatency {
			minLatency = o.LatencyMS
		}
		if o.LatencyMS > maxLatency {
			maxLatency = o.LatencyMS
		}
	}

	choices := make([]actionCount, 0, len(counts))
	var modalAction decision.Action
	var modalCount int
	for action, count := range counts {
		if count > modalCount || (count == modalCount && string(action) < string(modalAction)) {
			modalAction = action
			modalCount = count
		}
		choices = append(choices, actionCount{
			Action: action,
			Count:  count,
			Rate:   float64(count) / float64(len(observations)),
		})
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].Count != choices[j].Count {
			return choices[i].Count > choices[j].Count
		}
		return choices[i].Action < choices[j].Action
	})

	n := float64(len(observations))
	return summary{
		Runs:                    len(observations),
		Choices:                 choices,
		ModalAction:             modalAction,
		ModalRate:               float64(modalCount) / n,
		StopRate:                float64(stopCount) / n,
		GatedRate:               float64(gatedCount) / n,
		SelectedProbabilityMin:  minProbability,
		SelectedProbabilityMean: probabilitySum / n,
		SelectedProbabilityMax:  maxProbability,
		DecisionMarginMin:       minMargin,
		DecisionMarginMean:      marginSum / n,
		DecisionMarginMax:       maxMargin,
		ConfidenceMean:          confidenceSum / n,
		LatencyMSMin:            minLatency,
		LatencyMSMean:           float64(latencySum) / n,
		LatencyMSMax:            maxLatency,
		Observations:            observations,
	}
}

func runnerUp(probabilities map[decision.Action]float64, selected decision.Action) (decision.Action, float64) {
	var action decision.Action
	var probability float64
	for candidate, value := range probabilities {
		if candidate == selected {
			continue
		}
		if value > probability || (value == probability && string(candidate) < string(action)) {
			action = candidate
			probability = value
		}
	}
	return action, probability
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/decision"
)

func main() {
	var statePath, model string
	var minChoiceProbability float64
	var timeout time.Duration
	flag.StringVar(&statePath, "state", "", "sanitized decision-state JSON file")
	flag.StringVar(&model, "model", decision.DefaultTypeSafeModel, "TypeSafe model")
	flag.Float64Var(&minChoiceProbability, "min-choice-probability", decision.DefaultMinChoiceProbability, "minimum selected-action probability required to apply a non-STOP decision")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "TypeSafe API timeout")
	flag.Parse()

	if strings.TrimSpace(statePath) == "" {
		fatal(fmt.Errorf("-state is required"))
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

	plan, err := (decision.Planner{
		Provider:             provider,
		MinChoiceProbability: minChoiceProbability,
	}).PlanNext(context.Background(), state)
	fatal(err)

	out, err := json.MarshalIndent(plan, "", "  ")
	fatal(err)
	fmt.Println(string(out))
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

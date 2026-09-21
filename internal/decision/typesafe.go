package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DefaultTypeSafeBaseURL = "https://api.typesafe.ai"
	DefaultTypeSafeModel   = "jev-latest"
)

type TypeSafeConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

type TypeSafeProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewTypeSafeProvider(cfg TypeSafeConfig) (*TypeSafeProvider, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("TypeSafe API key is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultTypeSafeBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultTypeSafeModel
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &TypeSafeProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  client,
	}, nil
}

func (p *TypeSafeProvider) Name() string { return "typesafe" }
func (p *TypeSafeProvider) Model() string { return p.model }

type typeSafeChoiceQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type typeSafeRequest struct {
	State     State                             `json:"state"`
	Questions map[string]typeSafeChoiceQuestion `json:"questions"`
	Model     string                            `json:"model"`
}

type typeSafeChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type typeSafeResponse struct {
	Model   string                          `json:"model"`
	Answers map[string]typeSafeChoiceAnswer `json:"answers"`
	Usage   Usage                           `json:"usage"`
}

func (p *TypeSafeProvider) Choose(ctx context.Context, req Request) (Result, error) {
	if err := ValidateRequest(req); err != nil {
		return Result{}, err
	}
	criteria := make(map[string]string, len(req.Options))
	allowed := make(map[Action]struct{}, len(req.Options))
	for _, option := range req.Options {
		criteria[string(option.Action)] = option.Description
		allowed[option.Action] = struct{}{}
	}
	payload := typeSafeRequest{
		State: req.State,
		Questions: map[string]typeSafeChoiceQuestion{
			"next_experiment": {
				Type:         "choice",
				Instructions: req.Instructions,
				Criteria:     criteria,
			},
		},
		Model: p.model,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, fmt.Errorf("encode TypeSafe request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("build TypeSafe request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "paramintel-typesafe-decision-spike")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("TypeSafe request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("read TypeSafe response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 2048 {
			msg = msg[:2048]
		}
		return Result{}, fmt.Errorf("TypeSafe API returned HTTP %d: %s", resp.StatusCode, msg)
	}
	var decoded typeSafeResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Result{}, fmt.Errorf("decode TypeSafe response: %w", err)
	}
	answer, ok := decoded.Answers["next_experiment"]
	if !ok {
		return Result{}, fmt.Errorf("TypeSafe response missing next_experiment answer")
	}
	if answer.Type != "choice" {
		return Result{}, fmt.Errorf("TypeSafe next_experiment answer has type %q, want choice", answer.Type)
	}
	action := Action(answer.Choice)
	if _, ok := allowed[action]; !ok {
		return Result{}, fmt.Errorf("TypeSafe returned unknown action %q", answer.Choice)
	}
	if answer.Confidence < 0 || answer.Confidence > 1 {
		return Result{}, fmt.Errorf("TypeSafe returned invalid confidence %v", answer.Confidence)
	}
	probabilities := make(map[Action]float64, len(answer.Probabilities))
	for label, probability := range answer.Probabilities {
		a := Action(label)
		if _, ok := allowed[a]; !ok {
			return Result{}, fmt.Errorf("TypeSafe returned probability for unknown action %q", label)
		}
		if probability < 0 || probability > 1 {
			return Result{}, fmt.Errorf("TypeSafe returned invalid probability %v for %q", probability, label)
		}
		probabilities[a] = probability
	}
	model := decoded.Model
	if strings.TrimSpace(model) == "" {
		model = p.model
	}
	return Result{
		Action:        action,
		Confidence:    answer.Confidence,
		Probabilities: probabilities,
		Provider:      p.Name(),
		Model:         model,
		Usage:         decoded.Usage,
	}, nil
}

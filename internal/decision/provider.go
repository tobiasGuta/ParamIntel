package decision

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type Action string

const (
	ActionStop                   Action = "stop"
	ActionEnumProfile            Action = "enum_profile"
	ActionBooleanProfile         Action = "boolean_profile"
	ActionNullabilityProfile     Action = "nullability_profile"
	ActionIntegerBoundaryProfile Action = "integer_boundary_profile"
	ActionEmptyValueProfile      Action = "empty_value_profile"
	ActionCaseVariationProfile   Action = "case_variation_profile"
	ActionRelatedValueProfile    Action = "related_value_profile"
)

type Option struct {
	Action      Action `json:"action"`
	Description string `json:"description"`
}

type CandidateState struct {
	Name          string `json:"name"`
	Location      string `json:"location"`
	DiscoveryMode string `json:"discovery_mode,omitempty"`
	ValueKind     string `json:"value_kind,omitempty"`
}

type VerificationState struct {
	CandidateChanged int     `json:"candidate_changed"`
	CandidateTrials  int     `json:"candidate_trials"`
	ControlChanged   int     `json:"control_changed"`
	ControlTrials    int     `json:"control_trials"`
	Confidence       float64 `json:"confidence"`
}

type EvidenceState struct {
	Kinds []string `json:"kinds,omitempty"`
	Paths []string `json:"paths,omitempty"`
}

type State struct {
	Candidate              CandidateState    `json:"candidate"`
	Verification           VerificationState `json:"verification"`
	Evidence               EvidenceState     `json:"evidence,omitempty"`
	RemainingRequestBudget int               `json:"remaining_request_budget"`
}

type Request struct {
	State        State    `json:"state"`
	Options      []Option `json:"options"`
	Instructions string   `json:"instructions"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Result struct {
	Action        Action             `json:"action"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[Action]float64 `json:"probabilities"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	Usage         Usage              `json:"usage"`
}

type Provider interface {
	Name() string
	Model() string
	Choose(ctx context.Context, req Request) (Result, error)
}

var actionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func ValidateRequest(req Request) error {
	if strings.TrimSpace(req.Instructions) == "" {
		return fmt.Errorf("decision instructions are required")
	}
	if len(req.Options) < 2 {
		return fmt.Errorf("at least two decision options are required")
	}
	if len(req.Options) > 16 {
		return fmt.Errorf("decision options are capped at 16 for this spike")
	}
	seen := map[Action]struct{}{}
	for _, option := range req.Options {
		if !actionIDPattern.MatchString(string(option.Action)) {
			return fmt.Errorf("invalid decision action %q", option.Action)
		}
		if _, ok := seen[option.Action]; ok {
			return fmt.Errorf("duplicate decision action %q", option.Action)
		}
		seen[option.Action] = struct{}{}
		if strings.TrimSpace(option.Description) == "" {
			return fmt.Errorf("decision action %q requires a description", option.Action)
		}
	}
	return nil
}

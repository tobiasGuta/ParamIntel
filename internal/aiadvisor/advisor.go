package aiadvisor

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const SourceAISemanticHypothesis = "ai_semantic_hypothesis"

var candidateNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,63}$`)

type Input struct {
	Method            string         `json:"method"`
	Path              string         `json:"path"`
	ActiveLocations   []string       `json:"active_locations"`
	QueryKeys         []string       `json:"query_keys,omitempty"`
	FormKeys          []string       `json:"form_keys,omitempty"`
	JSONParents       []string       `json:"json_parents,omitempty"`
	RequestJSONShape  map[string]any `json:"request_json_shape,omitempty"`
	ResponseJSONShape map[string]any `json:"response_json_shape,omitempty"`
}

type Suggestion struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	JSONParent string `json:"json_parent"`
	Reason     string `json:"reason"`
	Priority   int    `json:"priority"`
}

type Result struct {
	Provider       string
	Model          string
	SuggestedCount int
	AcceptedCount  int
	Candidates     []model.Candidate
}

func Generate(ctx context.Context, provider Provider, input Input, limit int) (Result, error) {
	if provider == nil {
		return Result{}, fmt.Errorf("AI provider is nil")
	}
	if limit <= 0 {
		return Result{}, fmt.Errorf("AI candidate limit must be greater than zero")
	}
	suggestions, err := provider.Suggest(ctx, input, limit)
	if err != nil {
		return Result{}, err
	}
	candidates := AcceptSuggestions(input, suggestions, limit)
	return Result{
		Provider:       provider.Name(),
		Model:          provider.Model(),
		SuggestedCount: len(suggestions),
		AcceptedCount:  len(candidates),
		Candidates:     candidates,
	}, nil
}

// AcceptSuggestions validates all model output against deterministic local
// constraints before it is allowed to enter the discovery candidate queue.
func AcceptSuggestions(input Input, suggestions []Suggestion, limit int) []model.Candidate {
	if limit <= 0 {
		return nil
	}
	items := append([]Suggestion(nil), suggestions...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].Name < items[j].Name
		}
		return items[i].Priority > items[j].Priority
	})

	active := stringSet(input.ActiveLocations)
	parents := stringSet(input.JSONParents)
	existing := existingCandidateKeys(input)
	seen := map[string]struct{}{}
	out := make([]model.Candidate, 0, min(limit, len(items)))

	for _, suggestion := range items {
		name := strings.TrimSpace(suggestion.Name)
		location := strings.ToLower(strings.TrimSpace(suggestion.Location))
		if !candidateNamePattern.MatchString(name) {
			continue
		}
		if _, ok := active[location]; !ok {
			continue
		}

		parent := ""
		if location == model.LocationJSON {
			parent = strings.TrimSpace(suggestion.JSONParent)
			if parent == "" {
				parent = "$"
			}
			if _, ok := parents[parent]; !ok {
				continue
			}
		}
		key := candidateKey(location, parent, name)
		if _, ok := existing[key]; ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		priority := suggestion.Priority
		if priority < 1 {
			priority = 1
		}
		if priority > 100 {
			priority = 100
		}
		reason := boundedReason(suggestion.Reason)
		out = append(out, model.Candidate{
			Name:       name,
			Location:   location,
			JSONParent: parent,
			Sources: []model.CandidateSource{{
				Source:   SourceAISemanticHypothesis,
				Priority: priority,
				Reason:   reason,
			}},
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func existingCandidateKeys(input Input) map[string]struct{} {
	out := map[string]struct{}{}
	for _, name := range input.QueryKeys {
		out[candidateKey(model.LocationQuery, "", name)] = struct{}{}
	}
	for _, name := range input.FormKeys {
		out[candidateKey(model.LocationForm, "", name)] = struct{}{}
	}
	collectExistingJSONKeys(input.RequestJSONShape, "$", out)
	return out
}

func collectExistingJSONKeys(shape map[string]any, parent string, out map[string]struct{}) {
	for name, value := range shape {
		out[candidateKey(model.LocationJSON, parent, name)] = struct{}{}
		if child, ok := value.(map[string]any); ok {
			collectExistingJSONKeys(child, joinJSONPath(parent, name), out)
		}
	}
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func candidateKey(location, parent, name string) string {
	return location + "|" + parent + "|" + name
}

func joinJSONPath(parent, name string) string {
	if parent == "$" {
		return "$." + name
	}
	return parent + "." + name
}

func boundedReason(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	const maxRunes = 300
	runes := []rune(reason)
	if len(runes) > maxRunes {
		reason = string(runes[:maxRunes])
	}
	return reason
}

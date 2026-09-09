package aiadvisor

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	SourceAISemanticHypothesis = "ai_semantic_hypothesis"
	AdmissionAdmitted          = "admitted"
	AdmissionRejected          = "rejected"
)

var candidateNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,63}$`)

type Input struct {
	Method                 string         `json:"method"`
	Path                   string         `json:"path"`
	ActiveLocations        []string       `json:"active_locations"`
	QueryKeys              []string       `json:"query_keys,omitempty"`
	FormKeys               []string       `json:"form_keys,omitempty"`
	JSONParents            []string       `json:"json_parents,omitempty"`
	RequestJSONShape       map[string]any `json:"request_json_shape,omitempty"`
	ResponseJSONShape      map[string]any `json:"response_json_shape,omitempty"`
	ExcludedCandidateNames []string       `json:"excluded_candidate_names,omitempty"`
	LocalCoveredNames      []string       `json:"-"`
}

type Suggestion struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	JSONParent string `json:"json_parent"`
	Reason     string `json:"reason"`
	Priority   int    `json:"priority"`
}

type SuggestionAudit struct {
	Name            string
	Location        string
	JSONParent      string
	Reason          string
	Priority        int
	Admission       string
	RejectionReason string
}

type Result struct {
	Provider       string
	Model          string
	SuggestedCount int
	AcceptedCount  int
	Candidates     []model.Candidate
	Audit          []SuggestionAudit
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
	candidates, audit := evaluateSuggestions(input, suggestions, limit)
	return Result{
		Provider:       provider.Name(),
		Model:          provider.Model(),
		SuggestedCount: len(suggestions),
		AcceptedCount:  len(candidates),
		Candidates:     candidates,
		Audit:          audit,
	}, nil
}

// AcceptSuggestions preserves the original candidate-only helper for callers
// that do not need admission audit details.
func AcceptSuggestions(input Input, suggestions []Suggestion, limit int) []model.Candidate {
	candidates, _ := evaluateSuggestions(input, suggestions, limit)
	return candidates
}

// evaluateSuggestions validates all model output against deterministic local
// constraints before it is allowed to enter the discovery candidate queue. It
// retains one bounded audit record per provider suggestion so rejected model
// output remains observable without becoming discovery evidence.
func evaluateSuggestions(input Input, suggestions []Suggestion, limit int) ([]model.Candidate, []SuggestionAudit) {
	if limit <= 0 {
		return nil, nil
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
	covered := stringSet(input.LocalCoveredNames)
	seen := map[string]struct{}{}
	out := make([]model.Candidate, 0, min(limit, len(items)))
	audit := make([]SuggestionAudit, 0, len(items))

	for _, suggestion := range items {
		name := strings.TrimSpace(suggestion.Name)
		location := strings.ToLower(strings.TrimSpace(suggestion.Location))
		parent := ""
		if location == model.LocationJSON {
			parent = strings.TrimSpace(suggestion.JSONParent)
			if parent == "" {
				parent = "$"
			}
		}
		priority := suggestion.Priority
		if priority < 1 {
			priority = 1
		}
		if priority > 100 {
			priority = 100
		}
		record := SuggestionAudit{
			Name:       name,
			Location:   location,
			JSONParent: parent,
			Reason:     boundedReason(suggestion.Reason),
			Priority:   priority,
			Admission:  AdmissionRejected,
		}

		switch {
		case !candidateNamePattern.MatchString(name):
			record.RejectionReason = "invalid_name"
			audit = append(audit, record)
			continue
		case !contains(active, location):
			record.RejectionReason = "inactive_location"
			audit = append(audit, record)
			continue
		case location == model.LocationJSON && !contains(parents, parent):
			record.RejectionReason = "invalid_json_parent"
			audit = append(audit, record)
			continue
		}

		key := candidateKey(location, parent, name)
		if _, ok := existing[key]; ok {
			record.RejectionReason = "already_present"
			audit = append(audit, record)
			continue
		}
		if _, ok := covered[name]; ok {
			record.RejectionReason = "deterministic_coverage"
			audit = append(audit, record)
			continue
		}
		if _, ok := seen[key]; ok {
			record.RejectionReason = "duplicate"
			audit = append(audit, record)
			continue
		}
		seen[key] = struct{}{}
		if len(out) >= limit {
			record.RejectionReason = "candidate_budget_exhausted"
			audit = append(audit, record)
			continue
		}

		record.Admission = AdmissionAdmitted
		audit = append(audit, record)
		out = append(out, model.Candidate{
			Name:       name,
			Location:   location,
			JSONParent: parent,
			Sources: []model.CandidateSource{{
				Source:   SourceAISemanticHypothesis,
				Priority: priority,
				Reason:   record.Reason,
			}},
		})
	}
	return out, audit
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

func contains(set map[string]struct{}, value string) bool {
	_, ok := set[value]
	return ok
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

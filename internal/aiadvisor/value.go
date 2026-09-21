package aiadvisor

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	ValueAdmissionAdmitted = "admitted"
	ValueAdmissionRejected = "rejected"
)

var safeSemanticString = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9 _.:+-]{0,79}$")

type ValueInput struct {
	Application    Input           `json:"application"`
	Candidate      ValueCandidate  `json:"candidate"`
	SemanticHints  []string        `json:"semantic_hints,omitempty"`
	ExcludedValues []ValueIdentity `json:"excluded_values,omitempty"`
}

type ValueCandidate struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	JSONParent string `json:"json_parent,omitempty"`
}

type ValueIdentity struct {
	Kind string `json:"kind"`
	Raw  string `json:"raw"`
}

type ValueSuggestion struct {
	Value    string `json:"value"`
	Kind     string `json:"kind"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

type ValueSuggestionAudit struct {
	Value           string
	Kind            string
	Reason          string
	Priority        int
	Admission       string
	RejectionReason string
}

type ValueResult struct {
	Provider       string
	Model          string
	SuggestedCount int
	AcceptedCount  int
	Values         []model.ProbeValue
	Audit          []ValueSuggestionAudit
}

type ValueProvider interface {
	Provider
	SuggestValues(ctx context.Context, input ValueInput, limit int) ([]ValueSuggestion, error)
}

func GenerateValues(ctx context.Context, provider Provider, input ValueInput, limit int) (ValueResult, error) {
	if provider == nil {
		return ValueResult{}, fmt.Errorf("AI provider is nil")
	}
	if limit <= 0 {
		return ValueResult{}, fmt.Errorf("AI value limit must be greater than zero")
	}
	valueProvider, ok := provider.(ValueProvider)
	if !ok {
		return ValueResult{}, fmt.Errorf("AI provider %q does not support semantic value suggestions", provider.Name())
	}
	suggestions, err := valueProvider.SuggestValues(ctx, input, limit)
	if err != nil {
		return ValueResult{}, err
	}
	values, audit := evaluateValueSuggestions(input, suggestions, limit)
	return ValueResult{
		Provider:       provider.Name(),
		Model:          provider.Model(),
		SuggestedCount: len(suggestions),
		AcceptedCount:  len(values),
		Values:         values,
		Audit:          audit,
	}, nil
}

func evaluateValueSuggestions(input ValueInput, suggestions []ValueSuggestion, limit int) ([]model.ProbeValue, []ValueSuggestionAudit) {
	if limit <= 0 {
		return nil, nil
	}
	items := append([]ValueSuggestion(nil), suggestions...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			if items[i].Kind == items[j].Kind {
				return items[i].Value < items[j].Value
			}
			return items[i].Kind < items[j].Kind
		}
		return items[i].Priority > items[j].Priority
	})

	excluded := map[string]struct{}{}
	for _, value := range input.ExcludedValues {
		excluded[valueKey(value.Kind, value.Raw)] = struct{}{}
	}
	seen := map[string]struct{}{}
	values := make([]model.ProbeValue, 0, min(limit, len(items)))
	audit := make([]ValueSuggestionAudit, 0, len(items))

	for _, suggestion := range items {
		raw := strings.TrimSpace(suggestion.Value)
		kind := strings.ToLower(strings.TrimSpace(suggestion.Kind))
		priority := suggestion.Priority
		if priority < 1 {
			priority = 1
		}
		if priority > 100 {
			priority = 100
		}
		record := ValueSuggestionAudit{
			Value:     raw,
			Kind:      kind,
			Reason:    boundedReason(suggestion.Reason),
			Priority:  priority,
			Admission: ValueAdmissionRejected,
		}

		value, reason, ok := normalizeSuggestedValue(input.Candidate.Location, kind, raw)
		if !ok {
			record.RejectionReason = reason
			audit = append(audit, record)
			continue
		}
		key := valueKey(value.Kind, value.Raw)
		if _, ok := excluded[key]; ok {
			record.RejectionReason = "deterministic_value"
			audit = append(audit, record)
			continue
		}
		if _, ok := seen[key]; ok {
			record.RejectionReason = "duplicate"
			audit = append(audit, record)
			continue
		}
		seen[key] = struct{}{}
		if len(values) >= limit {
			record.RejectionReason = "value_budget_exhausted"
			audit = append(audit, record)
			continue
		}

		record.Admission = ValueAdmissionAdmitted
		record.Kind = value.Kind
		record.Value = value.Raw
		audit = append(audit, record)
		values = append(values, value)
	}
	return values, audit
}

func normalizeSuggestedValue(location, kind, raw string) (model.ProbeValue, string, bool) {
	if len([]rune(raw)) > 80 {
		return model.ProbeValue{}, "value_too_long", false
	}

	if location != model.LocationJSON {
		if raw == "" {
			return model.ProbeValue{}, "empty_value", false
		}
		if !safeSemanticString.MatchString(raw) {
			return model.ProbeValue{}, "unsafe_string", false
		}
		return model.StringValue(raw), "", true
	}

	switch kind {
	case "string":
		if raw == "" {
			return model.ProbeValue{}, "empty_value", false
		}
		if !safeSemanticString.MatchString(raw) {
			return model.ProbeValue{}, "unsafe_string", false
		}
		return model.StringValue(raw), "", true
	case "boolean":
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return model.ProbeValue{}, "invalid_boolean", false
		}
		return model.BoolValue(b), "", true
	case "integer":
		i, err := strconv.Atoi(raw)
		if err != nil {
			return model.ProbeValue{}, "invalid_integer", false
		}
		return model.IntegerValue(i), "", true
	case "null":
		if raw != "" && !strings.EqualFold(raw, "null") {
			return model.ProbeValue{}, "invalid_null", false
		}
		return model.NullValue(), "", true
	default:
		return model.ProbeValue{}, "unsupported_kind", false
	}
}

func valueKey(kind, raw string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "|" + strings.TrimSpace(raw)
}

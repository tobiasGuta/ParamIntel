package externalhints

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/aiadvisor"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

const (
	SourceExternalSemanticHint = "external_semantic_hint"
	SemanticValueSource        = "external_hint"

	MaxDocumentBytes      = 128 * 1024
	MaxContextItems       = 20
	MaxCandidates         = 50
	MaxValuesPerCandidate = 12
)

type Document struct {
	Context    []string        `json:"context,omitempty"`
	Candidates []CandidateHint `json:"candidates"`
}

type CandidateHint struct {
	Name       string      `json:"name"`
	Location   string      `json:"location"`
	JSONParent string      `json:"json_parent,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Priority   int         `json:"priority,omitempty"`
	Values     []ValueHint `json:"values,omitempty"`
}

type ValueHint struct {
	Value    string `json:"value"`
	Kind     string `json:"kind,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

type CandidateAdmission struct {
	Candidates []model.Candidate
	Audit      []aiadvisor.SuggestionAudit
}

type ValueAdmission struct {
	Values []model.ProbeValue
	Audit  []aiadvisor.ValueSuggestionAudit
}

func Load(path string) (Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if err != nil {
		return Document{}, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (Document, error) {
	if len(raw) == 0 {
		return Document{}, fmt.Errorf("external hints document is empty")
	}
	if len(raw) > MaxDocumentBytes {
		return Document{}, fmt.Errorf("external hints document exceeds %d bytes", MaxDocumentBytes)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("decode external hints: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Document{}, fmt.Errorf("external hints document must contain exactly one JSON value")
		}
		return Document{}, fmt.Errorf("decode external hints trailing data: %w", err)
	}

	if len(doc.Context) > MaxContextItems {
		return Document{}, fmt.Errorf("external hints context exceeds %d items", MaxContextItems)
	}
	for i, item := range doc.Context {
		item = strings.Join(strings.Fields(item), " ")
		if item == "" {
			return Document{}, fmt.Errorf("external hints context[%d] is empty", i)
		}
		if len([]rune(item)) > 120 {
			return Document{}, fmt.Errorf("external hints context[%d] exceeds 120 characters", i)
		}
		doc.Context[i] = item
	}

	if len(doc.Candidates) == 0 {
		return Document{}, fmt.Errorf("external hints document contains no candidates")
	}
	if len(doc.Candidates) > MaxCandidates {
		return Document{}, fmt.Errorf("external hints candidates exceed %d", MaxCandidates)
	}
	for i := range doc.Candidates {
		if len(doc.Candidates[i].Values) > MaxValuesPerCandidate {
			return Document{}, fmt.Errorf("external hints candidate[%d] values exceed %d", i, MaxValuesPerCandidate)
		}
	}
	return doc, nil
}

func (d Document) AdmitCandidates(input aiadvisor.Input, deterministicNames []string, limit int) CandidateAdmission {
	if limit <= 0 || limit > MaxCandidates {
		limit = MaxCandidates
	}
	input.LocalCoveredNames = append([]string(nil), deterministicNames...)
	suggestions := make([]aiadvisor.Suggestion, 0, len(d.Candidates))
	for _, hint := range d.Candidates {
		suggestions = append(suggestions, aiadvisor.Suggestion{
			Name:       hint.Name,
			Location:   hint.Location,
			JSONParent: hint.JSONParent,
			Reason:     hint.Reason,
			Priority:   hint.Priority,
		})
	}
	candidates, audit := aiadvisor.AcceptSuggestionsWithSource(input, suggestions, limit, SourceExternalSemanticHint)
	return CandidateAdmission{Candidates: candidates, Audit: audit}
}

func (d Document) HasValuesFor(candidate model.Candidate) bool {
	hint, ok := d.findCandidate(candidate)
	return ok && len(hint.Values) > 0
}

func (d Document) ValuesFor(candidate model.Candidate, deterministic []model.ProbeValue, limit int) ValueAdmission {
	if limit <= 0 || limit > MaxValuesPerCandidate {
		limit = MaxValuesPerCandidate
	}
	hint, ok := d.findCandidate(candidate)
	if !ok || len(hint.Values) == 0 {
		return ValueAdmission{}
	}

	excluded := make([]aiadvisor.ValueIdentity, 0, len(deterministic))
	for _, value := range deterministic {
		excluded = append(excluded, aiadvisor.ValueIdentity{Kind: value.Kind, Raw: value.Raw})
	}
	suggestions := make([]aiadvisor.ValueSuggestion, 0, len(hint.Values))
	for _, value := range hint.Values {
		suggestions = append(suggestions, aiadvisor.ValueSuggestion{
			Value:    value.Value,
			Kind:     value.Kind,
			Reason:   value.Reason,
			Priority: value.Priority,
		})
	}
	values, audit := aiadvisor.AcceptValueSuggestions(aiadvisor.ValueInput{
		Candidate: aiadvisor.ValueCandidate{
			Name:       candidate.Name,
			Location:   candidate.Location,
			JSONParent: normalizeParent(candidate.Location, candidate.JSONParent),
		},
		ExcludedValues: excluded,
	}, suggestions, limit)
	return ValueAdmission{Values: values, Audit: audit}
}

func (d Document) findCandidate(candidate model.Candidate) (CandidateHint, bool) {
	wantParent := normalizeParent(candidate.Location, candidate.JSONParent)
	for _, hint := range d.Candidates {
		if strings.TrimSpace(hint.Name) != candidate.Name {
			continue
		}
		if strings.ToLower(strings.TrimSpace(hint.Location)) != candidate.Location {
			continue
		}
		if normalizeParent(candidate.Location, hint.JSONParent) != wantParent {
			continue
		}
		return hint, true
	}
	return CandidateHint{}, false
}

func normalizeParent(location, parent string) string {
	if location != model.LocationJSON {
		return ""
	}
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return "$"
	}
	return parent
}

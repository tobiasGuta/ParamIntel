package decision

import (
	"strings"
	"unicode"
)

type HeuristicPlanner struct{}

func (HeuristicPlanner) Plan(state State) Action {
	if state.RemainingRequestBudget <= 0 {
		return ActionStop
	}

	v := state.Verification
	if v.CandidateTrials > 0 &&
		v.CandidateChanged == v.CandidateTrials &&
		v.ControlChanged == 0 &&
		v.Confidence >= 0.90 {
		return ActionStop
	}

	if v.ControlChanged > 0 {
		return ActionStop
	}

	switch strings.ToLower(strings.TrimSpace(state.Candidate.ValueKind)) {
	case "boolean", "bool":
		return ActionBooleanProfile
	case "integer", "int":
		return ActionIntegerBoundaryProfile
	}

	nameTokens := semanticTokens(state.Candidate.Name)
	pathTokens := map[string]struct{}{}
	for _, path := range state.Evidence.Paths {
		for _, token := range semanticTokens(path) {
			pathTokens[token] = struct{}{}
		}
	}

	if hasAnyToken(nameTokens, "status", "state", "mode", "visibility", "role", "type", "format", "tier", "phase", "channel", "variant", "scope") {
		if hasAnyTokenMap(pathTokens, "allowed", "available", "supported", "valid", "possible", "statuses", "states", "modes", "visibilities", "roles", "types", "formats", "tiers", "phases", "channels", "variants", "scopes") {
			return ActionEnumProfile
		}
		return ActionRelatedValueProfile
	}

	if hasAnyToken(nameTokens, "enabled", "disabled", "include", "exclude", "active", "preview", "archived", "deleted") {
		return ActionBooleanProfile
	}

	if hasAnyToken(nameTokens, "limit", "count", "size", "offset", "page", "window", "batch", "depth", "level") {
		return ActionIntegerBoundaryProfile
	}

	return ActionStop
}

func semanticTokens(s string) []string {
	var tokens []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(string(current)))
		current = current[:0]
	}
	var prevLower bool
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if unicode.IsUpper(r) && prevLower {
				flush()
			}
			current = append(current, r)
			prevLower = unicode.IsLower(r)
			continue
		}
		flush()
		prevLower = false
	}
	flush()
	return tokens
}

func hasAnyToken(tokens []string, wanted ...string) bool {
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return hasAnyTokenMap(set, wanted...)
}

func hasAnyTokenMap(tokens map[string]struct{}, wanted ...string) bool {
	for _, token := range wanted {
		if _, ok := tokens[token]; ok {
			return true
		}
	}
	return false
}

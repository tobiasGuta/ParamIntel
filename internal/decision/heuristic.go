package decision

import (
	"strings"
	"unicode"
)

type HeuristicPlanner struct{}

type HeuristicDecision struct {
	Action  Action
	Decided bool
	Reason  string
}

func (p HeuristicPlanner) Plan(state State) Action {
	decision := p.Decide(state)
	if !decision.Decided {
		return ActionStop
	}
	return decision.Action
}

func (HeuristicPlanner) Decide(state State) HeuristicDecision {
	if state.RemainingRequestBudget <= 0 {
		return HeuristicDecision{Action: ActionStop, Decided: true, Reason: "request budget exhausted"}
	}

	v := state.Verification
	if v.CandidateTrials > 0 &&
		v.CandidateChanged == v.CandidateTrials &&
		v.ControlChanged == 0 &&
		v.Confidence >= 0.90 {
		return HeuristicDecision{Action: ActionStop, Decided: true, Reason: "existing verification evidence is already sufficient"}
	}

	if v.ControlChanged > 0 {
		return HeuristicDecision{Action: ActionStop, Decided: true, Reason: "control changed; characterization signal is not clean"}
	}

	switch strings.ToLower(strings.TrimSpace(state.Candidate.ValueKind)) {
	case "boolean", "bool":
		return HeuristicDecision{Action: ActionBooleanProfile, Decided: true, Reason: "boolean rule matched"}
	case "integer", "int":
		return HeuristicDecision{Action: ActionIntegerBoundaryProfile, Decided: true, Reason: "integer rule matched"}
	}

	if structural, ok := structuralEvidenceDecision(state.Candidate, state.Evidence); ok {
		return structural
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
			return HeuristicDecision{Action: ActionEnumProfile, Decided: true, Reason: "enum rule matched"}
		}
		return HeuristicDecision{Action: ActionRelatedValueProfile, Decided: true, Reason: "related-value rule matched"}
	}

	if hasAnyToken(nameTokens, "enabled", "disabled", "include", "exclude", "active", "preview", "archived", "deleted") {
		return HeuristicDecision{Action: ActionBooleanProfile, Decided: true, Reason: "boolean rule matched"}
	}

	if hasAnyToken(nameTokens, "limit", "count", "size", "offset", "page", "window", "batch", "depth", "level") {
		return HeuristicDecision{Action: ActionIntegerBoundaryProfile, Decided: true, Reason: "integer rule matched"}
	}

	return HeuristicDecision{Action: ActionStop, Decided: false, Reason: "no deterministic rule matched"}
}


func structuralEvidenceDecision(candidate CandidateState, evidence EvidenceState) (HeuristicDecision, bool) {
	candidateTokens := semanticTokens(candidate.Name)
	if len(candidateTokens) == 0 {
		return HeuristicDecision{}, false
	}

	for _, path := range evidence.Paths {
		pathTokens := semanticTokens(path)
		start, end, ok := findTokenSequence(pathTokens, candidateTokens)
		if !ok {
			continue
		}

		if start > 0 {
			switch canonicalToken(pathTokens[start-1]) {
			case "can", "has", "is":
				return HeuristicDecision{
					Action:  ActionBooleanProfile,
					Decided: true,
					Reason:  "evidence path uses a boolean relation around the candidate",
				}, true
			case "supported", "available", "allowed", "valid", "possible":
				return HeuristicDecision{
					Action:  ActionEnumProfile,
					Decided: true,
					Reason:  "evidence path exposes a bounded value-set relation around the candidate",
				}, true
			case "max", "maximum", "min", "minimum":
				return HeuristicDecision{
					Action:  ActionIntegerBoundaryProfile,
					Decided: true,
					Reason:  "evidence path exposes a numeric boundary relation around the candidate",
				}, true
			}
		}

		if end < len(pathTokens) {
			switch canonicalToken(pathTokens[end]) {
			case "supported", "enabled", "active":
				return HeuristicDecision{
					Action:  ActionBooleanProfile,
					Decided: true,
					Reason:  "evidence path exposes a boolean capability relation around the candidate",
				}, true
			}
		}
	}

	return HeuristicDecision{}, false
}

func findTokenSequence(haystack, needle []string) (int, int, bool) {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return 0, 0, false
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		match := true
		for i := range needle {
			if canonicalToken(haystack[start+i]) != canonicalToken(needle[i]) {
				match = false
				break
			}
		}
		if match {
			return start, start + len(needle), true
		}
	}
	return 0, 0, false
}

func canonicalToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasSuffix(s, "ies") && len(s) > 3:
		return strings.TrimSuffix(s, "ies") + "y"
	case (strings.HasSuffix(s, "ses") || strings.HasSuffix(s, "xes") || strings.HasSuffix(s, "zes") || strings.HasSuffix(s, "ches") || strings.HasSuffix(s, "shes")) && len(s) > 3:
		return strings.TrimSuffix(s, "es")
	case strings.HasSuffix(s, "s") && len(s) > 3 &&
		!strings.HasSuffix(s, "ss") &&
		!strings.HasSuffix(s, "us") &&
		!strings.HasSuffix(s, "is"):
		return strings.TrimSuffix(s, "s")
	default:
		return s
	}
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

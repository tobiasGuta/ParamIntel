package aiadvisor

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var enumHintKey = regexp.MustCompile(`(?i)(available|allowed|supported|valid|possible|visibility|status|state|mode|type|role|option|format)`)

func BuildSemanticValueHints(rawContext []byte, candidate ValueCandidate, limit int) []string {
	if limit <= 0 {
		limit = 12
	}
	body := extractResponseBody(rawContext)
	var root any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil
	}

	candidateTokens := normalizedSemanticTokens(candidate.Name)
	seen := map[string]struct{}{}
	var ranked []struct {
		value string
		score int
	}
	walkSemanticHints(root, "", candidateTokens, seen, &ranked)

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].value < ranked[j].value
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]string, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.value)
	}
	return out
}

func walkSemanticHints(v any, key string, candidateTokens map[string]struct{}, seen map[string]struct{}, out *[]struct {
	value string
	score int
}) {
	switch typed := v.(type) {
	case map[string]any:
		for childKey, child := range typed {
			walkSemanticHints(child, childKey, candidateTokens, seen, out)
		}
	case []any:
		if !enumHintKey.MatchString(key) && !tokensOverlap(candidateTokens, normalizedSemanticTokens(key)) {
			return
		}
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if !safeSemanticString.MatchString(s) {
				continue
			}
			k := strings.ToLower(s)
			if _, exists := seen[k]; exists {
				continue
			}
			seen[k] = struct{}{}
			score := 1
			if tokensOverlap(candidateTokens, normalizedSemanticTokens(key)) {
				score += 2
			}
			*out = append(*out, struct {
				value string
				score int
			}{value: s, score: score})
		}
	}
}

func ValueCandidateRelevance(input Input, candidate ValueCandidate) int {
	candidateTokens := normalizedSemanticTokens(candidate.Name)
	if len(candidateTokens) == 0 {
		return 0
	}
	score := 0
	for _, key := range input.QueryKeys {
		score = max(score, semanticKeyScore(candidateTokens, key))
	}
	for _, key := range input.FormKeys {
		score = max(score, semanticKeyScore(candidateTokens, key))
	}
	score = max(score, semanticShapeScore(candidateTokens, input.RequestJSONShape))
	score = max(score, semanticShapeScore(candidateTokens, input.ResponseJSONShape))
	return score
}

func semanticShapeScore(candidateTokens map[string]struct{}, shape map[string]any) int {
	best := 0
	for key, value := range shape {
		best = max(best, semanticKeyScore(candidateTokens, key))
		if child, ok := value.(map[string]any); ok {
			best = max(best, semanticShapeScore(candidateTokens, child))
		}
	}
	return best
}

func semanticKeyScore(candidateTokens map[string]struct{}, key string) int {
	keyTokens := normalizedSemanticTokens(key)
	if len(keyTokens) == 0 {
		return 0
	}
	if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(joinTokenSet(candidateTokens))) {
		return 100
	}
	if tokensOverlap(candidateTokens, keyTokens) {
		return 80
	}
	return 0
}

func normalizedSemanticTokens(s string) map[string]struct{} {
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	out := map[string]struct{}{}
	for _, part := range parts {
		part = singularSemanticToken(part)
		if len(part) < 3 {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

func singularSemanticToken(s string) string {
	switch {
	case len(s) > 4 && strings.HasSuffix(s, "ies"):
		return strings.TrimSuffix(s, "ies") + "y"
	case len(s) > 3 && strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss"):
		return strings.TrimSuffix(s, "s")
	default:
		return s
	}
}

func tokensOverlap(a, b map[string]struct{}) bool {
	for token := range a {
		if _, ok := b[token]; ok {
			return true
		}
	}
	return false
}

func joinTokenSet(tokens map[string]struct{}) string {
	if len(tokens) != 1 {
		return ""
	}
	for token := range tokens {
		return token
	}
	return ""
}

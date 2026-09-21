package discovery

import (
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/semantics"
)

type rescueEvidenceTier int

const (
	rescueTierGeneric rescueEvidenceTier = iota
	rescueTierHeuristic
	rescueTierAI
	rescueTierContext
	rescueTierApplication
)

type rescueRank struct {
	Tier               rescueEvidenceTier
	SourcePriority     int
	ContextRelevance   int
	Reason             string
}

func rankRescueCandidate(candidate model.Candidate, contextual SemanticValuePriority) rescueRank {
	rank := rescueRank{Tier: rescueTierGeneric, Reason: "generic candidate"}

	if len(semantics.ProfileValues(candidate.Name, candidate.Location)) > 0 {
		rank.Tier = rescueTierHeuristic
		rank.Reason = "local semantic profile"
	}

	for _, source := range candidate.Sources {
		if source.Priority > rank.SourcePriority {
			rank.SourcePriority = source.Priority
		}

		tier, reason := rescueSourceTier(source)
		if tier > rank.Tier {
			rank.Tier = tier
			rank.Reason = reason
		}
	}

	if contextual != nil {
		rank.ContextRelevance = contextual(candidate)
		if rank.ContextRelevance > 0 && rank.Tier < rescueTierContext {
			rank.Tier = rescueTierContext
			rank.Reason = "application-context semantic match"
		}
	}

	return rank
}

func rescueSourceTier(source model.CandidateSource) (rescueEvidenceTier, string) {
	switch source.Source {
	case "openapi_response_only_json_property", "context_response_only_json_property":
		return rescueTierApplication, source.Source
	case "context_response_scaffoldable_json_property":
		return rescueTierContext, source.Source
	case "ai_semantic_hypothesis":
		return rescueTierAI, source.Source
	}

	if source.Priority > 0 {
		return rescueTierContext, source.Source
	}
	return rescueTierGeneric, ""
}

func rescueRankLess(a, b rescueRank) bool {
	if a.Tier != b.Tier {
		return a.Tier > b.Tier
	}
	if a.SourcePriority != b.SourcePriority {
		return a.SourcePriority > b.SourcePriority
	}
	if a.ContextRelevance != b.ContextRelevance {
		return a.ContextRelevance > b.ContextRelevance
	}
	return false
}

func (r rescueRank) tierLabel() string {
	switch r.Tier {
	case rescueTierApplication:
		return "A"
	case rescueTierContext:
		return "B"
	case rescueTierAI:
		return "C"
	case rescueTierHeuristic:
		return "D"
	default:
		return "E"
	}
}

func (r rescueRank) auditReason() string {
	reason := strings.TrimSpace(r.Reason)
	if reason == "" {
		return "generic candidate"
	}
	return reason
}

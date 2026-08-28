package confidence

// Score is a deterministic evidence-ranking heuristic, not a calibrated
// probability or statistical-significance estimate. The weights deliberately
// prioritize three things for small active-testing trial counts: reproducible
// candidate behavior, divergence from random-name controls, and diversity of
// independent response evidence. They are intentionally conservative and can
// be revisited if a sufficiently large labeled benchmark corpus is collected.
func Score(candidateChanged, candidateTrials, controlChanged, controlTrials int, evidenceKinds int) float64 {
	if candidateTrials == 0 {
		return 0
	}
	cand := float64(candidateChanged) / float64(candidateTrials)
	ctrl := 0.0
	if controlTrials > 0 {
		ctrl = float64(controlChanged) / float64(controlTrials)
	}

	// Most weight goes to reproducible behavior that random controls do not
	// reproduce. A smaller reproducibility term prevents one evidence dimension
	// from dominating, and evidence diversity adds only a capped bonus.
	score := 0.80 * cand * (1 - ctrl)
	score += 0.10 * cand
	if evidenceKinds >= 2 {
		score += 0.10
	} else if evidenceKinds == 1 {
		score += 0.05
	}

	// If random unknown parameters reproduce the candidate at least as often,
	// strongly demote the result even when the endpoint itself is noisy.
	if ctrl >= cand && ctrl > 0 {
		score *= 0.25
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// Label converts a numeric confidence score into a stable human-readable tier.
func Label(score float64) string {
	switch {
	case score >= 0.85:
		return "high"
	case score >= 0.60:
		return "medium"
	default:
		return "low"
	}
}

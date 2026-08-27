package confidence

func Score(candidateChanged, candidateTrials, controlChanged, controlTrials int, evidenceKinds int) float64 {
	if candidateTrials == 0 {
		return 0
	}
	cand := float64(candidateChanged) / float64(candidateTrials)
	ctrl := 0.0
	if controlTrials > 0 {
		ctrl = float64(controlChanged) / float64(controlTrials)
	}

	// A candidate that behaves like a random unknown parameter is weak evidence,
	// even when the candidate response is perfectly reproducible.
	score := 0.80 * cand * (1 - ctrl)
	score += 0.10 * cand
	if evidenceKinds >= 2 {
		score += 0.10
	} else if evidenceKinds == 1 {
		score += 0.05
	}

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

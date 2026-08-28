package discovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/confidence"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func (e Engine) verify(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, candidate model.Candidate, trials int) (model.ParameterResult, error) {
	value := model.StringValue(token())
	candChanged, ctrlChanged := 0, 0
	evidence := map[string]model.Difference{}
	for i := 0; i < trials; i++ {
		s, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: candidate, Value: value}})
		if err != nil {
			return model.ParameterResult{}, err
		}
		comparison := cmp.AgainstBaseline(p, s)
		if comparison.Meaningful {
			candChanged++
			for _, d := range comparison.Differences {
				evidence[d.Kind+"|"+d.Path] = d
			}
		}

		control := candidate
		control.Name = "zz_pi_" + token()
		control.Sources = nil
		cs, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: control, Value: value}})
		if err != nil {
			return model.ParameterResult{}, err
		}
		if cmp.AgainstBaseline(p, cs).Meaningful {
			ctrlChanged++
		}
	}

	kinds := map[string]struct{}{}
	ev := make([]model.Difference, 0, len(evidence))
	for _, d := range evidence {
		kinds[d.Kind] = struct{}{}
		ev = append(ev, d)
	}
	sort.Slice(ev, func(i, j int) bool {
		if ev[i].Kind == ev[j].Kind {
			return ev[i].Path < ev[j].Path
		}
		return ev[i].Kind < ev[j].Kind
	})
	score := confidence.Score(candChanged, trials, ctrlChanged, trials, len(kinds))
	return model.ParameterResult{
		Name:                 candidate.Name,
		Location:             candidate.Location,
		JSONPath:             candidate.JSONPath(),
		CandidateSources:     append([]model.CandidateSource(nil), candidate.Sources...),
		Confidence:           model.ConfidenceScore(score),
		ConfidenceLabel:      confidence.Label(score),
		CandidateChanged:     candChanged,
		CandidateTrials:      trials,
		RandomControlChanged: ctrlChanged,
		RandomControlTrials:  trials,
		Evidence:             ev,
	}, nil
}

func token() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "a1b2c3d4"
	}
	return hex.EncodeToString(b[:])
}

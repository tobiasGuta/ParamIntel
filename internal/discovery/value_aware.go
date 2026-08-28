package discovery

import (
	"context"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/semantics"
)

type semanticBudget struct {
	remaining int
	used      int
	exhausted bool
}

func newSemanticBudget(limit int) *semanticBudget {
	if limit < 0 {
		limit = 0
	}
	return &semanticBudget{remaining: limit, exhausted: limit == 0}
}

func (b *semanticBudget) reserve(requests int) bool {
	if b == nil {
		return true
	}
	if requests <= 0 {
		return true
	}
	if b.remaining < requests {
		b.exhausted = true
		return false
	}
	b.remaining -= requests
	b.used += requests
	if b.remaining == 0 {
		b.exhausted = true
	}
	return true
}

// semanticRescueCandidate tries the existing conservative semantic profile for
// one candidate that generic random-string discovery could not prove.
//
// Each value first gets a cheap one-shot screen. A meaningful candidate probe
// is immediately paired with a random-name control using the exact same value
// and value kind. Only candidate-specific behavior proceeds to repeated
// verification. Evidence from different semantic values is never pooled.
func (e Engine) semanticRescueCandidate(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, candidate model.Candidate, trials int, minConfidence float64) (model.ParameterResult, model.ProbeValue, bool, error) {
	return e.semanticRescueCandidateBudgeted(ctx, tmpl, p, candidate, trials, minConfidence, nil)
}

func (e Engine) semanticRescueCandidateBudgeted(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, candidate model.Candidate, trials int, minConfidence float64, budget *semanticBudget) (model.ParameterResult, model.ProbeValue, bool, error) {
	values := semantics.ProfileValues(candidate.Name, candidate.Location)
	for _, value := range values {
		if !budget.reserve(1) {
			return model.ParameterResult{}, model.ProbeValue{}, false, nil
		}
		s, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: candidate, Value: value}})
		if err != nil {
			return model.ParameterResult{}, model.ProbeValue{}, false, err
		}
		if !cmp.AgainstBaseline(p, s).Meaningful {
			continue
		}

		if !budget.reserve(1) {
			return model.ParameterResult{}, model.ProbeValue{}, false, nil
		}
		controlToken, err := token()
		if err != nil {
			return model.ParameterResult{}, model.ProbeValue{}, false, err
		}
		control := candidate
		control.Name = "zz_pi_" + controlToken
		control.Sources = nil
		cs, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: control, Value: value}})
		if err != nil {
			return model.ParameterResult{}, model.ProbeValue{}, false, err
		}
		if cmp.AgainstBaseline(p, cs).Meaningful {
			continue
		}

		// Reserve the complete repeated verification cost before starting it so
		// the configured budget can never be crossed halfway through a finding.
		if !budget.reserve(2 * trials) {
			return model.ParameterResult{}, model.ProbeValue{}, false, nil
		}
		r, err := e.verifyWithValue(ctx, tmpl, p, candidate, value, trials)
		if err != nil {
			return model.ParameterResult{}, model.ProbeValue{}, false, err
		}
		if float64(r.Confidence) < minConfidence {
			continue
		}
		return r, value, true, nil
	}

	return model.ParameterResult{}, model.ProbeValue{}, false, nil
}

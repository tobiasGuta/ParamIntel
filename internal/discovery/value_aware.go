package discovery

import (
	"context"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/semantics"
)

// semanticRescueCandidate tries the existing conservative semantic profile for
// one candidate that generic random-string discovery could not prove.
//
// Each value first gets a cheap one-shot screen. A meaningful candidate probe
// is immediately paired with a random-name control using the exact same value
// and value kind. Only candidate-specific behavior proceeds to repeated
// verification. Evidence from different semantic values is never pooled.
func (e Engine) semanticRescueCandidate(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, candidate model.Candidate, trials int, minConfidence float64) (model.ParameterResult, model.ProbeValue, bool, error) {
	values := semantics.ProfileValues(candidate.Name, candidate.Location)
	for _, value := range values {
		s, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: candidate, Value: value}})
		if err != nil {
			return model.ParameterResult{}, model.ProbeValue{}, false, err
		}
		if !cmp.AgainstBaseline(p, s).Meaningful {
			continue
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

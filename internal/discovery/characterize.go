package discovery

import (
	"context"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/semantics"
)

func (e Engine) characterize(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, candidate model.Candidate, result *model.ParameterResult) error {
	generic, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: candidate, Value: model.StringValue(token())}})
	if err != nil {
		return err
	}
	guess := semantics.InferFromResponse(generic.Body)
	if guess.Name == "" {
		guess = semantics.GuessFromName(candidate.Name)
	}
	if guess.Name != "" {
		setType(result, guess)
	}

	for _, value := range semantics.ProfileValues(candidate.Name, candidate.Location) {
		s, err := baseline.SendMutations(ctx, e.Client, tmpl, []model.Mutation{{Candidate: candidate, Value: value}})
		if err != nil {
			return err
		}
		comparison := cmp.AgainstBaseline(p, s)
		classification := "baseline_like"
		if hint := semantics.InferFromResponse(s.Body); hint.Name != "" {
			classification = "validation_error"
			if result.TypeConfidence == nil || float64(*result.TypeConfidence) < hint.Confidence {
				setType(result, hint)
			}
		} else if comparison.Meaningful {
			classification = "behavioral_change"
		}
		result.ValueProfile = append(result.ValueProfile, model.ValueObservation{
			Value: value.Raw, ValueKind: value.Kind, Status: s.StatusCode,
			Classification: classification, Evidence: comparison.Differences,
		})
	}
	return nil
}

func setType(result *model.ParameterResult, guess semantics.TypeGuess) {
	result.InferredType = guess.Name
	score := model.ConfidenceScore(guess.Confidence)
	result.TypeConfidence = &score
	result.TypeEvidence = guess.Evidence
}

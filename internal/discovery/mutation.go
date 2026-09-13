package discovery

import "github.com/tobiasGuta/ParamIntel/internal/model"

// mutation is the single discovery-side constructor for active mutations.
// Controlled JSON scaffolding is authorized only when both the engine config
// explicitly enables it and the candidate carries response-derived scaffold
// metadata. Generic and AI candidates never acquire scaffold capability merely
// because the CLI flag is enabled.
func (e Engine) mutation(candidate model.Candidate, value model.ProbeValue) model.Mutation {
	return model.Mutation{
		Candidate:         candidate,
		Value:             value,
		AllowJSONScaffold: e.Config.JSONScaffold && candidate.RequiresJSONScaffold(),
	}
}

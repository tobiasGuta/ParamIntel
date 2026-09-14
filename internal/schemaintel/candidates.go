package schemaintel

import "github.com/tobiasGuta/ParamIntel/internal/model"

const existingParentCandidatePriority = 110

func ExistingParentCandidates(report Report) []model.Candidate {
	out := make([]model.Candidate, 0, len(report.Candidates))
	for _, descriptor := range report.Candidates {
		if descriptor.Placement != PlacementExistingParent {
			continue
		}
		out = append(out, model.Candidate{
			Name:       descriptor.Name,
			Location:   model.LocationJSON,
			JSONParent: descriptor.Parent,
			Sources: []model.CandidateSource{{
				Source:        descriptor.Source,
				Path:          descriptor.Path,
				Priority:      existingParentCandidatePriority,
				Reason:        descriptor.Reason,
				DeclaredTypes: append([]string(nil), descriptor.DeclaredTypes...),
				Nullable:      descriptor.Nullable,
				ReadOnly:      descriptor.ReadOnly,
				WriteOnly:     descriptor.WriteOnly,
				Required:      descriptor.Required,
				SchemaRef:     descriptor.SchemaRef,
			}},
		})
	}
	return out
}

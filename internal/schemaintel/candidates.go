package schemaintel

import "github.com/tobiasGuta/ParamIntel/internal/model"

const existingParentCandidatePriority = 110

// ExistingParentCandidates converts only descriptors whose JSON parent already
// exists in the captured request into normal ParamIntel candidates. Scaffold
// descriptors deliberately remain passive in v0.9 Slice 2.
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
				ReadOnly:      descriptor.ReadOnly,
				WriteOnly:     descriptor.WriteOnly,
				Required:      descriptor.Required,
				SchemaRef:     descriptor.SchemaRef,
			}},
		})
	}
	return out
}

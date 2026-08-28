package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/mutate"
)

func buildTargets(tmpl model.RequestTemplate, words, locations []string, maxJSONDepth int) ([]model.Candidate, error) {
	return buildTargetsWithSeeds(tmpl, words, locations, maxJSONDepth, nil)
}

func buildTargetsWithSeeds(tmpl model.RequestTemplate, words, locations []string, maxJSONDepth int, seeds []model.Candidate) ([]model.Candidate, error) {
	active := activeLocations(tmpl, locations, maxJSONDepth)
	activeSet := make(map[string]struct{}, len(active))
	for _, location := range active {
		activeSet[location] = struct{}{}
	}

	var out []model.Candidate
	seen := map[string]struct{}{}

	// Context-derived candidates are intentionally inserted before generic
	// wordlist candidates. This makes high-signal application-specific fields
	// run first while preserving the exact same verifier and controls.
	for _, seed := range seeds {
		if _, ok := activeSet[seed.Location]; !ok {
			continue
		}
		appendCandidate(&out, seen, seed)
	}

	for _, location := range active {
		switch location {
		case model.LocationQuery, model.LocationForm:
			for _, word := range words {
				appendCandidate(&out, seen, model.Candidate{Name: word, Location: location})
			}
		case model.LocationJSON:
			parents := mutate.JSONObjectParents(tmpl.Body, maxJSONDepth)
			if len(parents) == 0 {
				return nil, fmt.Errorf("JSON discovery requested but request body is not a JSON object")
			}
			for _, parent := range parents {
				for _, word := range words {
					appendCandidate(&out, seen, model.Candidate{Name: word, Location: model.LocationJSON, JSONParent: parent})
				}
			}
		default:
			return nil, fmt.Errorf("unknown discovery location %q", location)
		}
	}
	return out, nil
}

func activeLocations(tmpl model.RequestTemplate, locations []string, maxJSONDepth int) []string {
	active := locations
	if len(active) != 0 && !(len(active) == 1 && active[0] == "auto") {
		return active
	}
	active = []string{model.LocationQuery}
	ct := strings.ToLower(tmpl.Headers.Get("Content-Type"))
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		active = append(active, model.LocationForm)
	}
	// JSON-shaped bodies are discovered from structure, not MIME type.
	// This covers real applications that submit JSON with text/plain while
	// preserving the captured Content-Type during replay.
	if len(mutate.JSONObjectParents(tmpl.Body, maxJSONDepth)) > 0 {
		active = append(active, model.LocationJSON)
	}
	return active
}

func appendCandidate(out *[]model.Candidate, seen map[string]struct{}, c model.Candidate) {
	key := candidateKey(c)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*out = append(*out, c)
}

func groupTargets(targets []model.Candidate, chunkSize int) [][]model.Candidate {
	byPlacement := map[string][]model.Candidate{}
	var order []string
	for _, c := range targets {
		tier := "generic"
		if len(c.Sources) > 0 {
			tier = "context"
		}
		// Keep contextual candidates in their own first-pass groups rather than
		// mixing them into a generic dictionary batch at the same placement.
		placement := tier + "|" + c.Location + "|" + c.JSONParent
		if _, ok := byPlacement[placement]; !ok {
			order = append(order, placement)
		}
		byPlacement[placement] = append(byPlacement[placement], c)
	}
	var out [][]model.Candidate
	for _, placement := range order {
		items := byPlacement[placement]
		for len(items) > 0 {
			n := chunkSize
			if n <= 0 {
				n = 64
			}
			if len(items) < n {
				n = len(items)
			}
			out = append(out, items[:n])
			items = items[n:]
		}
	}
	return out
}

func (e Engine) narrow(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, group []model.Candidate) ([]model.Candidate, error) {
	if len(group) == 1 {
		return group, nil
	}
	mid := len(group) / 2
	var out []model.Candidate
	for _, half := range [][]model.Candidate{group[:mid], group[mid:]} {
		ok, err := e.groupInteresting(ctx, tmpl, p, half)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		found, err := e.narrow(ctx, tmpl, p, half)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	return out, nil
}

func (e Engine) groupInteresting(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, group []model.Candidate) (bool, error) {
	value := model.StringValue(token())
	mutations := make([]model.Mutation, 0, len(group))
	for _, candidate := range group {
		mutations = append(mutations, model.Mutation{Candidate: candidate, Value: value})
	}
	s, err := baseline.SendMutations(ctx, e.Client, tmpl, mutations)
	if err != nil {
		return false, fmt.Errorf("probe group %s: %w", fmtCandidate(group[0]), err)
	}
	return cmp.AgainstBaseline(p, s).Meaningful, nil
}

func targetLocations(targets []model.Candidate) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, c := range targets {
		if _, ok := seen[c.Location]; ok {
			continue
		}
		seen[c.Location] = struct{}{}
		out = append(out, c.Location)
	}
	sort.Strings(out)
	return out
}

func candidateKey(c model.Candidate) string {
	return c.Location + "|" + c.JSONParent + "|" + c.Name
}

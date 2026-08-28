package discovery

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/semantics"
)

type Config struct {
	ChunkSize        int
	Trials           int
	MinConfidence    float64
	Verbose          bool
	Logf             func(format string, args ...any)
	Locations        []string
	MaxJSONDepth     int
	Characterize     bool
	ValueAware       bool
	ValueAwareBudget int
}

type Engine struct {
	Client *http.Client
	Config Config
}

func (e Engine) Scan(ctx context.Context, tmpl model.RequestTemplate, profile model.BaselineProfile, words []string) ([]model.ParameterResult, error) {
	return e.ScanWithCandidates(ctx, tmpl, profile, words, nil)
}

// ScanWithCandidates preserves the v0.2/v0.3 discovery/verifier pipeline while
// allowing high-signal, exact candidate placements to be tested first and an
// optional bounded value-aware rescue pass to recover clean semantic misses.
func (e Engine) ScanWithCandidates(ctx context.Context, tmpl model.RequestTemplate, profile model.BaselineProfile, words []string, seeded []model.Candidate) ([]model.ParameterResult, error) {
	cfg := e.Config
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 64
	}
	if cfg.Trials <= 0 {
		cfg.Trials = 3
	}
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = .60
	}
	if cfg.MaxJSONDepth <= 0 {
		cfg.MaxJSONDepth = 3
	}
	if cfg.ValueAwareBudget < 0 {
		cfg.ValueAwareBudget = 0
	}

	targets, err := buildTargetsWithSeeds(tmpl, words, cfg.Locations, cfg.MaxJSONDepth, seeded)
	if err != nil {
		return nil, err
	}
	groups := groupTargets(targets, cfg.ChunkSize)
	e.verbosef("[*] Active locations: %s\n", strings.Join(targetLocations(targets), ","))
	if len(seeded) > 0 {
		e.verbosef("[*] Prioritized %d contextual candidate placements\n", len(seeded))
	}
	e.verbosef("[*] Testing %d candidate placements in %d initial groups\n", len(targets), len(groups))

	var survivors []model.Candidate
	for _, group := range groups {
		interesting, err := e.groupInteresting(ctx, tmpl, profile, group)
		if err != nil {
			return nil, err
		}
		if !interesting {
			continue
		}
		found, err := e.narrow(ctx, tmpl, profile, group)
		if err != nil {
			return nil, err
		}
		survivors = append(survivors, found...)
	}

	genericAttempted := map[string]struct{}{}
	accepted := map[string]struct{}{}
	rescueExcluded := map[string]struct{}{}
	results := make([]model.ParameterResult, 0)
	for _, candidate := range survivors {
		key := candidateKey(candidate)
		if _, ok := genericAttempted[key]; ok {
			continue
		}
		genericAttempted[key] = struct{}{}
		r, err := e.verify(ctx, tmpl, profile, candidate, cfg.Trials)
		if err != nil {
			return nil, err
		}
		if float64(r.Confidence) < cfg.MinConfidence {
			e.logVerification(r, false, cfg.MinConfidence)
			// Semantic rescue is deliberately conservative. If generic probing
			// produced any candidate or control activity, do not reinterpret it
			// using additional values. Only clean 0/0 misses can be rescued.
			if r.CandidateChanged > 0 || r.RandomControlChanged > 0 {
				rescueExcluded[key] = struct{}{}
			}
			continue
		}
		if cfg.Characterize {
			if err := e.characterize(ctx, tmpl, profile, candidate, &r); err != nil {
				return nil, err
			}
		}
		e.logVerification(r, true, cfg.MinConfidence)
		results = append(results, r)
		accepted[key] = struct{}{}
	}

	if cfg.ValueAware && cfg.ValueAwareBudget > 0 {
		eligible := 0
		for _, candidate := range targets {
			key := candidateKey(candidate)
			if _, ok := accepted[key]; ok {
				continue
			}
			if _, ok := rescueExcluded[key]; ok {
				continue
			}
			if len(semantics.ProfileValues(candidate.Name, candidate.Location)) == 0 {
				continue
			}
			eligible++
		}

		budget := newSemanticBudget(cfg.ValueAwareBudget)
		e.verbosef("[*] Value-aware rescue\n")
		e.verbosef("    eligible candidates: %d\n", eligible)
		e.verbosef("    semantic probe budget: %d requests\n", cfg.ValueAwareBudget)

		for _, candidate := range targets {
			if budget.exhausted {
				break
			}
			key := candidateKey(candidate)
			if _, ok := accepted[key]; ok {
				continue
			}
			if _, ok := rescueExcluded[key]; ok {
				continue
			}
			if len(semantics.ProfileValues(candidate.Name, candidate.Location)) == 0 {
				continue
			}

			r, value, ok, err := e.semanticRescueCandidateBudgeted(ctx, tmpl, profile, candidate, cfg.Trials, cfg.MinConfidence, budget)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			r.DiscoveryMode = "value_aware"
			r.DiscoveryValue = value.Raw
			r.DiscoveryValueKind = value.Kind
			if cfg.Characterize {
				if err := e.characterize(ctx, tmpl, profile, candidate, &r); err != nil {
					return nil, err
				}
			}
			e.logVerification(r, true, cfg.MinConfidence)
			results = append(results, r)
			accepted[key] = struct{}{}
		}

		if budget.exhausted {
			e.verbosef("    semantic probe budget exhausted: %d/%d requests used\n", budget.used, cfg.ValueAwareBudget)
		} else {
			e.verbosef("    semantic requests used: %d/%d\n", budget.used, cfg.ValueAwareBudget)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Confidence != results[j].Confidence {
			return results[i].Confidence > results[j].Confidence
		}
		if results[i].Location != results[j].Location {
			return results[i].Location < results[j].Location
		}
		if results[i].JSONPath != results[j].JSONPath {
			return results[i].JSONPath < results[j].JSONPath
		}
		return results[i].Name < results[j].Name
	})
	return results, nil
}

func (e Engine) logVerification(r model.ParameterResult, accepted bool, minConfidence float64) {
	if !e.Config.Verbose {
		return
	}
	prefix := "[-]"
	if accepted {
		prefix = "[+]"
	}
	label := r.Name
	if r.JSONPath != "" {
		label = r.JSONPath
	}
	e.verbosef("%s %s (%s)\n", prefix, label, r.Location)
	for _, source := range r.CandidateSources {
		e.verbosef("    source: %s %s", source.Source, source.Path)
		if source.ObservedType != "" {
			e.verbosef(" (observed %s)", source.ObservedType)
		}
		e.verbosef("\n")
	}
	if r.DiscoveryMode == "value_aware" {
		e.verbosef("    discovery: value-aware using %q (%s)\n", r.DiscoveryValue, r.DiscoveryValueKind)
	}
	e.verbosef("    candidate: changed %d/%d\n", r.CandidateChanged, r.CandidateTrials)
	e.verbosef("    control:   changed %d/%d\n", r.RandomControlChanged, r.RandomControlTrials)
	e.verbosef("    confidence: %.0f%% %s\n", float64(r.Confidence)*100, upperLabel(r.ConfidenceLabel))
	if accepted {
		e.verbosef("    accepted: confidence meets %.0f%% threshold\n", minConfidence*100)
		if r.InferredType != "" && r.TypeConfidence != nil {
			e.verbosef("    inferred type: %s (%.0f%%; %s)\n", r.InferredType, float64(*r.TypeConfidence)*100, r.TypeEvidence)
		}
		return
	}
	if r.RandomControlChanged >= r.CandidateChanged && r.RandomControlChanged > 0 {
		e.verbosef("    rejected: random negative control reproduced candidate behavior\n")
		return
	}
	e.verbosef("    rejected: confidence below %.0f%% threshold\n", minConfidence*100)
}

func (e Engine) verbosef(format string, args ...any) {
	if !e.Config.Verbose || e.Config.Logf == nil {
		return
	}
	e.Config.Logf(format, args...)
}

func upperLabel(label string) string {
	switch label {
	case "high":
		return "HIGH"
	case "medium":
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func fmtCandidate(c model.Candidate) string {
	if c.JSONPath() != "" {
		return c.JSONPath()
	}
	return fmt.Sprintf("%s:%s", c.Location, c.Name)
}

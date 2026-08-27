package discovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/confidence"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

type Config struct {
	ChunkSize     int
	Trials        int
	MinConfidence float64
	Verbose       bool
	Logf          func(format string, args ...any)
}

type Engine struct {
	Client *http.Client
	Config Config
}

func (e Engine) Scan(ctx context.Context, tmpl model.RequestTemplate, profile model.BaselineProfile, words []string) ([]model.ParameterResult, error) {
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
	groups := chunk(words, cfg.ChunkSize)
	e.verbosef("[*] Testing %d candidates in %d initial groups\n", len(words), len(groups))
	var survivors []string
	for _, g := range groups {
		s, err := e.groupInteresting(ctx, tmpl, profile, g)
		if err != nil {
			return nil, err
		}
		if s {
			found, err := e.narrow(ctx, tmpl, profile, g)
			if err != nil {
				return nil, err
			}
			survivors = append(survivors, found...)
		}
	}
	seen := map[string]struct{}{}
	var results []model.ParameterResult
	for _, name := range survivors {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		r, err := e.verify(ctx, tmpl, profile, name, cfg.Trials)
		if err != nil {
			return nil, err
		}
		if float64(r.Confidence) >= cfg.MinConfidence {
			e.logVerification(r, true, cfg.MinConfidence)
			results = append(results, r)
			continue
		}
		e.logVerification(r, false, cfg.MinConfidence)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Confidence > results[j].Confidence })
	return results, nil
}

func (e Engine) narrow(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, group []string) ([]string, error) {
	if len(group) == 1 {
		return group, nil
	}
	mid := len(group) / 2
	halves := [][]string{group[:mid], group[mid:]}
	var out []string
	for _, h := range halves {
		ok, err := e.groupInteresting(ctx, tmpl, p, h)
		if err != nil {
			return nil, err
		}
		if ok {
			x, err := e.narrow(ctx, tmpl, p, h)
			if err != nil {
				return nil, err
			}
			out = append(out, x...)
		}
	}
	return out, nil
}

func (e Engine) groupInteresting(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, group []string) (bool, error) {
	q := map[string]string{}
	for _, name := range group {
		q[name] = token()
	}
	s, err := baseline.Send(ctx, e.Client, tmpl, q)
	if err != nil {
		return false, fmt.Errorf("probe group: %w", err)
	}
	return cmp.AgainstBaseline(p, s).Meaningful, nil
}

func (e Engine) verify(ctx context.Context, tmpl model.RequestTemplate, p model.BaselineProfile, name string, trials int) (model.ParameterResult, error) {
	value := token()
	candChanged, ctrlChanged := 0, 0
	evidence := map[string]model.Difference{}
	for i := 0; i < trials; i++ {
		s, err := baseline.Send(ctx, e.Client, tmpl, map[string]string{name: value})
		if err != nil {
			return model.ParameterResult{}, err
		}
		c := cmp.AgainstBaseline(p, s)
		if c.Meaningful {
			candChanged++
			for _, d := range c.Differences {
				evidence[d.Kind+"|"+d.Path] = d
			}
		}
		controlName := "zz_pi_" + token()
		cs, err := baseline.Send(ctx, e.Client, tmpl, map[string]string{controlName: value})
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
		Name:                 name,
		Location:             "query",
		Confidence:           model.ConfidenceScore(score),
		ConfidenceLabel:      confidence.Label(score),
		CandidateChanged:     candChanged,
		CandidateTrials:      trials,
		RandomControlChanged: ctrlChanged,
		RandomControlTrials:  trials,
		Evidence:             ev,
	}, nil
}

func (e Engine) logVerification(r model.ParameterResult, accepted bool, minConfidence float64) {
	if !e.Config.Verbose {
		return
	}
	prefix := "[-]"
	if accepted {
		prefix = "[+]"
	}
	e.verbosef("%s %s\n", prefix, r.Name)
	e.verbosef("    candidate: changed %d/%d\n", r.CandidateChanged, r.CandidateTrials)
	e.verbosef("    control:   changed %d/%d\n", r.RandomControlChanged, r.RandomControlTrials)
	e.verbosef("    confidence: %.0f%% %s\n", float64(r.Confidence)*100, upperLabel(r.ConfidenceLabel))
	if accepted {
		e.verbosef("    accepted: confidence meets %.0f%% threshold\n", minConfidence*100)
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

func chunk(in []string, n int) [][]string {
	if n <= 0 {
		n = 64
	}
	var out [][]string
	for len(in) > 0 {
		m := n
		if len(in) < m {
			m = len(in)
		}
		out = append(out, in[:m])
		in = in[m:]
	}
	return out
}

func token() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "a1b2c3d4"
	}
	return hex.EncodeToString(b[:])
}

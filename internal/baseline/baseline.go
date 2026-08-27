package baseline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/mutate"
)

func Build(ctx context.Context, client *http.Client, tmpl model.RequestTemplate, samples int) (model.BaselineProfile, error) {
	if samples < 2 {
		samples = 3
	}
	shots := make([]model.Snapshot, 0, samples)
	for i := 0; i < samples; i++ {
		s, err := SendMutations(ctx, client, tmpl, nil)
		if err != nil {
			return model.BaselineProfile{}, fmt.Errorf("baseline request %d: %w", i+1, err)
		}
		shots = append(shots, s)
	}
	return cmp.BuildBaseline(shots), nil
}

// Send preserves the v0.1 query-only helper for callers and tests.
func Send(ctx context.Context, client *http.Client, tmpl model.RequestTemplate, query map[string]string) (model.Snapshot, error) {
	mutations := make([]model.Mutation, 0, len(query))
	for k, v := range query {
		mutations = append(mutations, model.Mutation{Candidate: model.Candidate{Name: k, Location: model.LocationQuery}, Value: model.StringValue(v)})
	}
	return SendMutations(ctx, client, tmpl, mutations)
}

func SendMutations(ctx context.Context, client *http.Client, tmpl model.RequestTemplate, mutations []model.Mutation) (model.Snapshot, error) {
	mutated, err := mutate.Apply(tmpl, mutations)
	if err != nil {
		return model.Snapshot{}, err
	}
	req, err := http.NewRequestWithContext(ctx, mutated.Method, mutated.URL, strings.NewReader(string(mutated.Body)))
	if err != nil {
		return model.Snapshot{}, err
	}
	req.Header = mutated.Headers.Clone()
	resp, err := client.Do(req)
	if err != nil {
		return model.Snapshot{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return model.Snapshot{}, err
	}
	return cmp.Snapshot(resp.StatusCode, resp.Header.Clone(), body), nil
}

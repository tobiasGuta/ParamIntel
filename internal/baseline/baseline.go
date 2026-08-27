package baseline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func Build(ctx context.Context, client *http.Client, tmpl model.RequestTemplate, samples int) (model.BaselineProfile, error) {
	if samples < 2 {
		samples = 3
	}
	shots := make([]model.Snapshot, 0, samples)
	for i := 0; i < samples; i++ {
		s, err := Send(ctx, client, tmpl, nil)
		if err != nil {
			return model.BaselineProfile{}, fmt.Errorf("baseline request %d: %w", i+1, err)
		}
		shots = append(shots, s)
	}
	return cmp.BuildBaseline(shots), nil
}

func Send(ctx context.Context, client *http.Client, tmpl model.RequestTemplate, query map[string]string) (model.Snapshot, error) {
	u := tmpl.URL
	req, err := http.NewRequestWithContext(ctx, tmpl.Method, u, strings.NewReader(string(tmpl.Body)))
	if err != nil {
		return model.Snapshot{}, err
	}
	req.Header = tmpl.Headers.Clone()
	q := req.URL.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()
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

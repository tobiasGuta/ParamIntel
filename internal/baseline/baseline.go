package baseline

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cmp "github.com/tobiasGuta/ParamIntel/internal/compare"
	"github.com/tobiasGuta/ParamIntel/internal/model"
	"github.com/tobiasGuta/ParamIntel/internal/mutate"
)

const (
	BackoffKindRateLimit     = "rate_limit"
	BackoffKindServerBackoff = "server_backoff"
)

// BackoffError marks a response that must not be used as discovery evidence.
// Callers can use errors.As to distinguish this condition from transport or
// parsing failures without string matching.
type BackoffError struct {
	StatusCode      int
	Kind            string
	RetryAfterRaw   string
	RetryAfter      time.Duration
	RetryAt         time.Time
	RetryAfterValid bool
}

func (e *BackoffError) Error() string {
	prefix := "rate limit detected"
	if e.Kind == BackoffKindServerBackoff {
		prefix = "server requested backoff"
	}
	if e.RetryAfterValid {
		if e.RetryAfterRaw != "" {
			return fmt.Sprintf("%s: HTTP %d (Retry-After: %s); response was not used as discovery evidence", prefix, e.StatusCode, e.RetryAfterRaw)
		}
		return fmt.Sprintf("%s: HTTP %d; response was not used as discovery evidence", prefix, e.StatusCode)
	}
	if e.RetryAfterRaw != "" {
		return fmt.Sprintf("%s: HTTP %d (unparsed Retry-After: %q); response was not used as discovery evidence", prefix, e.StatusCode, e.RetryAfterRaw)
	}
	return fmt.Sprintf("%s: HTTP %d; response was not used as discovery evidence", prefix, e.StatusCode)
}

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
	if err := classifyBackoff(resp, time.Now()); err != nil {
		return model.Snapshot{}, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return model.Snapshot{}, err
	}
	return cmp.Snapshot(resp.StatusCode, resp.Header.Clone(), body), nil
}

func classifyBackoff(resp *http.Response, now time.Time) error {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	delay, retryAt, valid := parseRetryAfter(raw, now)

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return &BackoffError{
			StatusCode:      resp.StatusCode,
			Kind:            BackoffKindRateLimit,
			RetryAfterRaw:   raw,
			RetryAfter:      delay,
			RetryAt:         retryAt,
			RetryAfterValid: valid,
		}
	case http.StatusServiceUnavailable:
		if raw == "" || !valid {
			return nil
		}
		return &BackoffError{
			StatusCode:      resp.StatusCode,
			Kind:            BackoffKindServerBackoff,
			RetryAfterRaw:   raw,
			RetryAfter:      delay,
			RetryAt:         retryAt,
			RetryAfterValid: true,
		}
	default:
		return nil
	}
}

func parseRetryAfter(raw string, now time.Time) (time.Duration, time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, time.Time{}, false
	}
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if seconds < 0 {
			return 0, time.Time{}, false
		}
		delay := time.Duration(seconds) * time.Second
		return delay, now.Add(delay), true
	}
	retryAt, err := http.ParseTime(raw)
	if err != nil {
		return 0, time.Time{}, false
	}
	delay := retryAt.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, retryAt, true
}

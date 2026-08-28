package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func requireRateLimitError(t *testing.T, err error) *baseline.BackoffError {
	t.Helper()
	if err == nil {
		t.Fatal("expected rate-limit error")
	}
	var backoff *baseline.BackoffError
	if !errors.As(err, &backoff) {
		t.Fatalf("error=%T %v; expected *baseline.BackoffError", err, err)
	}
	if backoff.Kind != baseline.BackoffKindRateLimit || backoff.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("backoff=%+v", backoff)
	}
	return backoff
}

func TestScanAbortsWhenGroupProbeIsRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) > 0 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":"rate limited"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 2, Trials: 1, MinConfidence: .60}}
	results, err := e.Scan(context.Background(), tmpl, profile, []string{"debug", "preview"})
	requireRateLimitError(t, err)
	if results != nil {
		t.Fatalf("results=%+v; rate-limited group probe must not produce scan results", results)
	}
}

func TestVerifyAbortsWhenCandidateProbeIsRateLimited(t *testing.T) {
	var active atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) == 0 {
			_, _ = fmt.Fprint(w, `{"ok":true}`)
			return
		}
		active.Add(1)
		if r.URL.Query().Has("debug") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	active.Store(0)

	e := Engine{Client: srv.Client()}
	result, err := e.verifyWithValue(context.Background(), tmpl, profile, model.Candidate{Name: "debug", Location: model.LocationQuery}, model.StringValue("true"), 3)
	requireRateLimitError(t, err)
	if active.Load() != 1 {
		t.Fatalf("active requests=%d want=1; control must not run after candidate 429", active.Load())
	}
	if result.CandidateTrials != 0 || result.RandomControlTrials != 0 || float64(result.Confidence) != 0 {
		t.Fatalf("invalid experiment must not return confidence/trials: %+v", result)
	}
}

func TestVerifyAbortsWhenRandomControlIsRateLimited(t *testing.T) {
	var active atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) == 0 {
			_, _ = fmt.Fprint(w, `{"ok":true}`)
			return
		}
		active.Add(1)
		if r.URL.Query().Has("debug") {
			_, _ = fmt.Fprint(w, `{"ok":true,"debug":true}`)
			return
		}
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	active.Store(0)

	e := Engine{Client: srv.Client()}
	result, err := e.verifyWithValue(context.Background(), tmpl, profile, model.Candidate{Name: "debug", Location: model.LocationQuery}, model.StringValue("true"), 3)
	requireRateLimitError(t, err)
	if active.Load() != 2 {
		t.Fatalf("active requests=%d want=2; first candidate/control pair should stop at control 429", active.Load())
	}
	if result.CandidateTrials != 0 || result.RandomControlTrials != 0 || float64(result.Confidence) != 0 {
		t.Fatalf("asymmetric limiter must not produce partial confidence: %+v", result)
	}
}

func TestValueAwareScreenAbortsOnRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) == 0 {
			_, _ = fmt.Fprint(w, `{"ok":true}`)
			return
		}
		if r.URL.Query().Has("debug") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	budget := newSemanticBudget(8)
	e := Engine{Client: srv.Client()}
	result, value, ok, err := e.semanticRescueCandidateBudgeted(context.Background(), tmpl, profile, model.Candidate{Name: "debug", Location: model.LocationQuery}, 3, .60, budget)
	requireRateLimitError(t, err)
	if ok || result.Name != "" || value.Raw != "" {
		t.Fatalf("rate-limited semantic screen must not return finding: result=%+v value=%+v ok=%t", result, value, ok)
	}
	if budget.used != 1 {
		t.Fatalf("semantic budget used=%d want=1 for the request actually sent", budget.used)
	}
}

func TestValueAwareControlAbortsOnRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) == 0 {
			_, _ = fmt.Fprint(w, `{"ok":true}`)
			return
		}
		if r.URL.Query().Has("debug") {
			_, _ = fmt.Fprint(w, `{"ok":true,"debug":true}`)
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	budget := newSemanticBudget(8)
	e := Engine{Client: srv.Client()}
	result, value, ok, err := e.semanticRescueCandidateBudgeted(context.Background(), tmpl, profile, model.Candidate{Name: "debug", Location: model.LocationQuery}, 3, .60, budget)
	requireRateLimitError(t, err)
	if ok || result.Name != "" || value.Raw != "" {
		t.Fatalf("rate-limited semantic control must not return finding: result=%+v value=%+v ok=%t", result, value, ok)
	}
	if budget.used != 2 {
		t.Fatalf("semantic budget used=%d want=2 for candidate screen + control", budget.used)
	}
}

func TestScanAbortsWhenCharacterizationIsRateLimited(t *testing.T) {
	var active atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) == 0 {
			_, _ = fmt.Fprint(w, `{"ok":true}`)
			return
		}

		n := active.Add(1)
		if n == 4 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.URL.Query().Has("organization_id") {
			_, _ = fmt.Fprint(w, `{"ok":true,"organization":true}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	active.Store(0)

	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 1, Trials: 1, MinConfidence: .60, Characterize: true}}
	results, err := e.Scan(context.Background(), tmpl, profile, []string{"organization_id"})
	requireRateLimitError(t, err)
	if results != nil {
		t.Fatalf("results=%+v; characterization backoff must abort normal scan result", results)
	}
	if active.Load() != 4 {
		t.Fatalf("active requests=%d want=4 (group, candidate, control, characterization)", active.Load())
	}
}

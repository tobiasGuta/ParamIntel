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

func requireStatusDrift(t *testing.T, err error, reason string) *BaselineStatusDriftError {
	t.Helper()
	var drift *BaselineStatusDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("error=%T %v, expected BaselineStatusDriftError", err, err)
	}
	if drift.Reason != reason || drift.ExpectedStatus != http.StatusOK ||
		drift.ProbeStatus != http.StatusUnauthorized {
		t.Fatalf("unexpected drift evidence: %+v", drift)
	}
	return drift
}

func TestScanStopsWhenAuthenticationDriftsAfterGoodBaseline(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 {
			_, _ = fmt.Fprint(w, `{"user":"member"}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"session expired"}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil || !p.StatusStable || p.StatusCode != http.StatusOK {
		t.Fatalf("unexpected initial baseline: %+v err=%v", p, err)
	}
	engine := Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK, ChunkSize: 1, Trials: 2,
	}}
	results, err := engine.Scan(context.Background(), tmpl, p, []string{"debug", "role"})
	drift := requireStatusDrift(t, err, "confirmed")
	if results != nil || len(drift.RecheckStatus) != 2 ||
		drift.RecheckStatus[0] != http.StatusUnauthorized ||
		drift.RecheckStatus[1] != http.StatusUnauthorized {
		t.Fatalf("must discard scan findings after confirmed drift: result=%+v drift=%+v", results, drift)
	}
	if got := calls.Load(); got != 6 { // 3 original baseline, 1 candidate, 2 clean checks
		t.Fatalf("unexpected extra requests after drift: %d", got)
	}
}

func TestCandidateSpecificUnauthorizedRemainsVerifiable(t *testing.T) {
	var clean atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Has("debug") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"debug disabled"}`)
			return
		}
		if len(r.URL.Query()) == 0 { clean.Add(1) }
		_, _ = fmt.Fprint(w, `{"user":"member"}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil { t.Fatal(err) }
	engine := Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK, ChunkSize: 1, Trials: 2,
	}}
	results, err := engine.Scan(context.Background(), tmpl, p, []string{"debug"})
	if err != nil { t.Fatalf("candidate-specific HTTP 401 should survive clean rechecks: %v", err) }
	if len(results) != 1 || results[0].Name != "debug" ||
		results[0].CandidateChanged != 2 || results[0].RandomControlChanged != 0 {
		t.Fatalf("candidate-specific response lost verification: %+v", results)
	}
	if got := clean.Load(); got != 9 { // baseline3, one screen and two trial responses each rechecked twice
		t.Fatalf("expected bounded 2 rechecks per observed 401; clean requests=%d", got)
	}
}

func TestAmbiguousCleanRechecksFailClosed(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 || n == 5 {
			_, _ = fmt.Fprint(w, `{"user":"member"}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil { t.Fatal(err) }
	results, err := (Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK, ChunkSize: 1,
	}}).Scan(context.Background(), tmpl, p, []string{"debug"})
	drift := requireStatusDrift(t, err, "ambiguous")
	if results != nil || len(drift.RecheckStatus) != 2 ||
		drift.RecheckStatus[0] != 200 || drift.RecheckStatus[1] != 401 {
		t.Fatalf("mixed clean responses must discard scan: %+v %+v", results, drift)
	}
}

func TestStatusDriftGuardOptInPreservesLegacyBehavior(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 {
			_, _ = fmt.Fprint(w, `{"user":"member"}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil { t.Fatal(err) }
	results, err := (Engine{Client: srv.Client(), Config: Config{ChunkSize: 1, Trials: 2}}).
		Scan(context.Background(), tmpl, p, []string{"debug"})
	if err != nil { t.Fatalf("unconfigured expectation must preserve legacy scan behavior: %v", err) }
	if len(results) != 0 { t.Fatalf("generic unknown-field behavior must not generate findings: %+v", results) }
	if calls.Load() != 8 { // 3 baseline, 1 group screen, 2 candidate and 2 controls
		t.Fatalf("unconfigured guard triggered extra baseline requests: %d", calls.Load())
	}
}

func TestStateChangingRequestDoesNotTriggerExtraCleanRequests(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 { _, _ = fmt.Fprint(w, `{"user":"member"}`); return }
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodPost, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil { t.Fatal(err) }
	results, err := (Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK,
	}}).Scan(context.Background(), tmpl, p, []string{"debug"})
	requireStatusDrift(t, err, "unsafe_to_recheck")
	if results != nil || calls.Load() != 4 {
		t.Fatalf("state-changing request must abort without any extra clean requests: calls=%d results=%+v", calls.Load(), results)
	}
}

func TestCleanRecheckRateLimitPreservesTypedBackoff(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 { _, _ = fmt.Fprint(w, `{"user":"member"}`); return }
		if n == 4 { w.WriteHeader(http.StatusUnauthorized); return }
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil { t.Fatal(err) }
	results, err := (Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK,
	}}).Scan(context.Background(), tmpl, p, []string{"debug"})
	if results != nil { t.Fatalf("backoff cannot produce a report: %+v", results) }
	var backoff *baseline.BackoffError
	if !errors.As(err, &backoff) || backoff.StatusCode != 429 {
		t.Fatalf("must retain typed backoff error instead of drift confirmation: %v", err)
	}
	if calls.Load() != 5 { t.Fatalf("rate-limit recheck must stop immediately: %d", calls.Load()) }
}

func TestAuthDriftRecheckBudgetIsGlobalAndFailClosed(t *testing.T) {
	var clean atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) == 0 { clean.Add(1); _, _ = fmt.Fprint(w, `{"ok":true}`); return }
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil { t.Fatal(err) }
	words := []string{"a","b","c","d","e","f","g"}
	results, err := (Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK, ChunkSize: 1, Trials: 1,
	}}).Scan(context.Background(), tmpl, p, words)
	drift := requireStatusDrift(t, err, "recheck_budget_exhausted")
	if results != nil || len(drift.RecheckStatus) != 0 {
		t.Fatalf("exhausted recheck budget must fail closed: results=%+v drift=%+v", results, drift)
	}
	if clean.Load() != 3+driftMaxRecheckRequests {
		t.Fatalf("expected only fixed global recheck budget, got %d clean requests", clean.Load())
	}
}

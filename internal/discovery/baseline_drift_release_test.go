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

// HTTP 403 must follow the same conservative state-drift handling as HTTP 401.
// A status-only block is not necessarily expired authentication: unmodified
// baseline replay must distinguish candidate-specific behavior from drift.
func TestForbiddenProbeDoesNotBecomeFalsePositiveOnAuthDrift(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 3 {
			_, _ = fmt.Fprint(w, `{"user":"member"}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":"session blocked"}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK,
		ChunkSize:              1,
		Trials:                 2,
	}}
	results, err := e.Scan(context.Background(), tmpl, profile, []string{"debug"})
	var drift *BaselineStatusDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("expected typed drift for in-scan HTTP 403: %T %v", err, err)
	}
	if drift.Reason != "confirmed" || drift.ExpectedStatus != 200 ||
		drift.ProbeStatus != 403 || len(drift.RecheckStatus) != 2 ||
		drift.RecheckStatus[0] != 403 || drift.RecheckStatus[1] != 403 {
		t.Fatalf("incorrect HTTP 403 drift evidence: %+v", drift)
	}
	if results != nil || count.Load() != 6 {
		t.Fatalf("confirmed 403 drift must fail closed with six requests: results=%+v count=%d", results, count.Load())
	}
}

func TestCandidateSpecificForbiddenWithClean200RemainsVerifiable(t *testing.T) {
	var clean atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Has("debug") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":"debug not allowed"}`)
			return
		}
		if len(r.URL.Query()) == 0 {
			clean.Add(1)
		}
		_, _ = fmt.Fprint(w, `{"user":"member"}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL, Headers: http.Header{}}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{
		ExpectedBaselineStatus: http.StatusOK,
		ChunkSize:              1,
		Trials:                 2,
	}}
	results, err := e.Scan(context.Background(), tmpl, profile, []string{"debug"})
	if err != nil {
		t.Fatalf("candidate-specific HTTP 403 should survive clean confirmation: %v", err)
	}
	if len(results) != 1 || results[0].Name != "debug" ||
		results[0].CandidateChanged != 2 || results[0].RandomControlChanged != 0 {
		t.Fatalf("expected normal candidate/control verification: %+v", results)
	}
	if clean.Load() != 9 { // initial baseline (3) + three anomalous probes x two clean rechecks
		t.Fatalf("unexpected count of clean 403 confirmation traffic: got=%d want=9", clean.Load())
	}
}

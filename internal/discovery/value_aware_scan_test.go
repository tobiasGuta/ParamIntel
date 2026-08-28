package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestScanValueAwareRescuesDebugAndRecordsProvenance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"users": []string{"alice", "bob"}}
		if r.URL.Query().Get("debug") == "true" {
			resp["debug"] = map[string]any{"database_ms": 12, "cache": "miss"}
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/users", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		Characterize:     false,
		ValueAware:       true,
		ValueAwareBudget: 8,
		Verbose:          true,
		Logf:             func(format string, args ...any) { fmt.Fprintf(&logs, format, args...) },
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"debug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v logs=%s", results, logs.String())
	}
	r := results[0]
	if r.Name != "debug" || r.Location != model.LocationQuery {
		t.Fatalf("result=%+v", r)
	}
	if r.DiscoveryMode != "value_aware" || r.DiscoveryValue != "true" || r.DiscoveryValueKind != "string" {
		t.Fatalf("discovery provenance=%+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 || float64(r.Confidence) != 1.0 {
		t.Fatalf("verification=%+v", r)
	}
	if len(r.ValueProfile) != 0 {
		t.Fatalf("characterize=false should leave value profile empty: %+v", r.ValueProfile)
	}
	if !strings.Contains(logs.String(), `discovery: value-aware using "true" (string)`) {
		t.Fatalf("missing value-aware diagnostic: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "semantic probe budget exhausted: 8/8 requests used") {
		t.Fatalf("unexpected budget diagnostic: %s", logs.String())
	}
}

func TestScanValueAwareNeverCrossesInsufficientBudget(t *testing.T) {
	baselineRequests := 0
	genericRequests := 0
	semanticRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"users": []string{"alice", "bob"}}
		q := r.URL.Query()
		if len(q) == 0 {
			baselineRequests++
		} else if v, ok := q["debug"]; ok && len(v) > 0 && v[0] != "true" {
			genericRequests++
		} else {
			semanticRequests++
		}
		if q.Get("debug") == "true" {
			resp["debug"] = true
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/users", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	engine := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:        4,
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		ValueAware:       true,
		ValueAwareBudget: 7,
		Verbose:          true,
		Logf:             func(format string, args ...any) { fmt.Fprintf(&logs, format, args...) },
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"debug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("insufficient budget must not start partial verification: %+v", results)
	}
	if baselineRequests != 5 {
		t.Fatalf("baseline requests=%d want=5", baselineRequests)
	}
	if genericRequests != 1 {
		t.Fatalf("generic requests=%d want=1", genericRequests)
	}
	// The semantic screen spends exactly debug=true plus its same-value random
	// control, then refuses to start the six-request repeated verification.
	if semanticRequests != 2 {
		t.Fatalf("semantic requests=%d want=2", semanticRequests)
	}
	if !strings.Contains(logs.String(), "semantic probe budget exhausted: 2/7 requests used") {
		t.Fatalf("missing exhaustion diagnostic: %s", logs.String())
	}
}

func TestScanValueAwareBudgetOrderingIsDeterministic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"status": "baseline"}
		q := r.URL.Query()
		if q.Get("debug") == "true" {
			resp["debug"] = true
		}
		if q.Get("preview") == "true" {
			resp["preview"] = true
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api/ordered", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}

	for run := 0; run < 2; run++ {
		engine := Engine{Client: srv.Client(), Config: Config{
			ChunkSize:        4,
			Trials:           3,
			MinConfidence:    .60,
			Locations:        []string{model.LocationQuery},
			ValueAware:       true,
			ValueAwareBudget: 8,
		}}
		results, err := engine.Scan(context.Background(), tmpl, profile, []string{"debug", "preview"})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Name != "debug" {
			t.Fatalf("run %d results=%+v; budget ordering must consistently confirm debug first", run, results)
		}
	}
}

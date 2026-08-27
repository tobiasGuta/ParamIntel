package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestScanFindsSemanticHiddenParameter(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Has("organization_id") {
			fmt.Fprintf(w, `{"request_id":%q,"data":[],"organization":{"visible":true}}`, fmt.Sprintf("r%d", n))
			return
		}
		fmt.Fprintf(w, `{"request_id":%q,"data":[]}`, fmt.Sprintf("r%d", n))
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL + "/api"}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 4, Trials: 3, MinConfidence: .60}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"debug", "organization_id", "preview", "unused"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	if results[0].Name != "organization_id" {
		t.Fatalf("found=%q", results[0].Name)
	}
	if results[0].CandidateChanged != 3 || results[0].RandomControlChanged != 0 {
		t.Fatalf("unexpected verification: %+v", results[0])
	}
}

func TestNegativeControlRejectsAnyUnknownParameterNoise(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) > 0 {
			fmt.Fprint(w, `{"data":[],"query_seen":true}`)
			return
		}
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 2, Trials: 3, MinConfidence: .60}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"admin", "debug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("generic unknown-param behavior should be rejected: %+v", results)
	}
}

func TestGraphQLStyleQueryParameterStatusChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("query") {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "Syntax Error")
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Query not present")
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL + "/api"}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 5)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 4, Trials: 3, MinConfidence: .60}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"admin", "debug", "query", "format"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.Name != "query" {
		t.Fatalf("found=%q", r.Name)
	}
	if float64(r.Confidence) != 1.0 || r.ConfidenceLabel != "high" {
		t.Fatalf("unexpected confidence: %+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 {
		t.Fatalf("unexpected verification: %+v", r)
	}
	seenStatus := false
	seenBody := false
	for _, d := range r.Evidence {
		if d.Kind == "status" && d.Before == "400" && d.After == "200" {
			seenStatus = true
		}
		if d.Kind == "body_changed" {
			seenBody = true
		}
	}
	if !seenStatus || !seenBody {
		t.Fatalf("missing expected evidence: %+v", r.Evidence)
	}
}

func TestVerboseRejectionExplainsNegativeControl(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Query()) > 0 {
			fmt.Fprint(w, "query-present")
			return
		}
		fmt.Fprint(w, "baseline")
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	var log string
	e := Engine{Client: srv.Client(), Config: Config{
		ChunkSize:     2,
		Trials:        3,
		MinConfidence: .60,
		Verbose:       true,
		Logf: func(format string, args ...any) {
			log += fmt.Sprintf(format, args...)
		},
	}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"admin", "debug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results: %+v", results)
	}
	if !strings.Contains(log, "rejected: random negative control reproduced candidate behavior") {
		t.Fatalf("missing rejection diagnostic:\n%s", log)
	}
	if !strings.Contains(log, "confidence:") || !strings.Contains(log, "LOW") {
		t.Fatalf("missing confidence diagnostic:\n%s", log)
	}
}

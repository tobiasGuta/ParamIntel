package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestScanFindsSemanticHiddenQueryParameter(t *testing.T) {
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
	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL + "/api", Headers: make(http.Header)}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 4, Trials: 3, MinConfidence: .60}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"debug", "organization_id", "preview", "unused"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "organization_id" || results[0].Location != model.LocationQuery {
		t.Fatalf("results=%+v", results)
	}
	if results[0].CandidateChanged != 3 || results[0].RandomControlChanged != 0 {
		t.Fatalf("unexpected verification: %+v", results[0])
	}
}

func TestNegativeControlRejectsAnyUnknownQueryParameterNoise(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if len(r.URL.Query()) > 0 {
			fmt.Fprint(w, `{"data":[],"query_seen":true}`)
			return
		}
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL, Headers: make(http.Header)}
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
	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL + "/api", Headers: make(http.Header)}
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
	if r.Name != "query" || float64(r.Confidence) != 1.0 || r.ConfidenceLabel != "high" {
		t.Fatalf("result=%+v", r)
	}
	if r.CandidateChanged != 3 || r.RandomControlChanged != 0 {
		t.Fatalf("unexpected verification: %+v", r)
	}
}

func TestScanFindsFormBodyParameter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := readForm(t, r)
		if body.Has("debug") {
			fmt.Fprint(w, `{"ok":true,"debug_seen":true}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: "POST", URL: srv.URL + "/search", Headers: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}}, Body: []byte("q=test")}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 3, Trials: 3, MinConfidence: .60, Characterize: true}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"unused", "debug", "preview"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.Name != "debug" || r.Location != model.LocationForm {
		t.Fatalf("result=%+v", r)
	}
	if r.RandomControlChanged != 0 || len(r.ValueProfile) != 4 {
		t.Fatalf("characterization=%+v", r)
	}
	if r.InferredType != "boolean" {
		t.Fatalf("inferred type=%q", r.InferredType)
	}
}

func TestScanFindsRootJSONParameterAndInfersBoolean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		v, ok := body["include_deleted"]
		if !ok {
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		b, ok := v.(bool)
		if !ok {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"include_deleted must be boolean"}`)
			return
		}
		fmt.Fprintf(w, `{"ok":true,"include_deleted":%t}`, b)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: "POST", URL: srv.URL + "/api", Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"q":"test"}`)}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 3, Trials: 3, MinConfidence: .60, Characterize: true}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"unused", "include_deleted", "preview"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.Location != model.LocationJSON || r.JSONPath != "$.include_deleted" {
		t.Fatalf("result=%+v", r)
	}
	if r.InferredType != "boolean" || r.TypeConfidence == nil || float64(*r.TypeConfidence) < .95 {
		t.Fatalf("type=%+v", r)
	}
	if len(r.ValueProfile) != 4 {
		t.Fatalf("profile=%+v", r.ValueProfile)
	}
	seenTyped, seenValidation := false, false
	for _, obs := range r.ValueProfile {
		if obs.ValueKind == "boolean" && obs.Classification == "behavioral_change" {
			seenTyped = true
		}
		if obs.ValueKind == "string" && obs.Classification == "validation_error" {
			seenValidation = true
		}
	}
	if !seenTyped || !seenValidation {
		t.Fatalf("profile=%+v", r.ValueProfile)
	}
}

func TestScanFindsNestedJSONParameterAndInfersInteger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		filters, _ := body["filters"].(map[string]any)
		if filters == nil {
			fmt.Fprint(w, `{"count":20}`)
			return
		}
		v, ok := filters["limit"]
		if !ok {
			fmt.Fprint(w, `{"count":20}`)
			return
		}
		n, ok := v.(float64)
		if !ok {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"limit must be an integer"}`)
			return
		}
		fmt.Fprintf(w, `{"count":%d}`, int(n))
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: "POST", URL: srv.URL + "/api/search", Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"filters":{"status":"active"}}`)}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 2, Trials: 3, MinConfidence: .60, Characterize: true, MaxJSONDepth: 3}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"limit", "unused"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.JSONPath != "$.filters.limit" || r.Location != model.LocationJSON {
		t.Fatalf("result=%+v", r)
	}
	if r.InferredType != "integer" || r.TypeConfidence == nil || float64(*r.TypeConfidence) < .95 {
		t.Fatalf("type=%+v", r)
	}
	if len(r.ValueProfile) != 4 {
		t.Fatalf("profile=%+v", r.ValueProfile)
	}
}

func TestNegativeControlRejectsAnyUnknownJSONFieldNoise(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body) > 1 {
			fmt.Fprint(w, `{"extra":true}`)
			return
		}
		fmt.Fprint(w, `{"extra":false}`)
	}))
	defer srv.Close()
	tmpl := model.RequestTemplate{Method: "POST", URL: srv.URL, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"q":"x"}`)}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	e := Engine{Client: srv.Client(), Config: Config{Locations: []string{model.LocationJSON}, ChunkSize: 2, Trials: 3, MinConfidence: .60}}
	results, err := e.Scan(context.Background(), tmpl, p, []string{"admin", "debug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("generic JSON-key behavior should be rejected: %+v", results)
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
	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL, Headers: make(http.Header)}
	p, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}
	var log string
	e := Engine{Client: srv.Client(), Config: Config{ChunkSize: 2, Trials: 3, MinConfidence: .60, Verbose: true, Logf: func(format string, args ...any) { log += fmt.Sprintf(format, args...) }}}
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

func readForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	return r.PostForm
}

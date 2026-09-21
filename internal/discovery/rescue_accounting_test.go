package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestRescueAuditRecordsVerifiedRequestCost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("format") == "json" {
			fmt.Fprint(w, `{"ok":true,"format":"json"}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	var audits []model.RescueCandidateAudit
	engine := Engine{Client: srv.Client(), Config: Config{
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		Characterize:     false,
		ValueAware:       true,
		ValueAwareBudget: 8,
		RescueAuditObserver: func(audit model.RescueCandidateAudit) {
			audits = append(audits, audit)
		},
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"format"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "format" {
		t.Fatalf("results=%+v", results)
	}
	if len(audits) != 1 {
		t.Fatalf("audits=%+v", audits)
	}
	audit := audits[0]
	if audit.RequestsUsed != 8 || audit.BudgetBefore != 8 || audit.BudgetAfter != 0 {
		t.Fatalf("budget audit=%+v", audit)
	}
	if audit.Outcome != "verified" || audit.DiscoveryMode != "value_aware" {
		t.Fatalf("outcome audit=%+v", audit)
	}
	if audit.EvidenceTier != "D" || audit.DeterministicValues != 3 || audit.AIQueried {
		t.Fatalf("classification audit=%+v", audit)
	}
}

func TestRescueAuditRecordsCheapCleanMiss(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: http.MethodGet, URL: srv.URL + "/api", Headers: make(http.Header)}
	profile, err := baseline.Build(context.Background(), srv.Client(), tmpl, 3)
	if err != nil {
		t.Fatal(err)
	}

	var audits []model.RescueCandidateAudit
	engine := Engine{Client: srv.Client(), Config: Config{
		Trials:           3,
		MinConfidence:    .60,
		Locations:        []string{model.LocationQuery},
		Characterize:     false,
		ValueAware:       true,
		ValueAwareBudget: 8,
		RescueAuditObserver: func(audit model.RescueCandidateAudit) {
			audits = append(audits, audit)
		},
	}}

	results, err := engine.Scan(context.Background(), tmpl, profile, []string{"debug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("results=%+v", results)
	}
	if len(audits) != 1 {
		t.Fatalf("audits=%+v", audits)
	}
	audit := audits[0]
	if audit.RequestsUsed != 4 || audit.BudgetBefore != 8 || audit.BudgetAfter != 4 || audit.Outcome != "miss" {
		t.Fatalf("audit=%+v", audit)
	}
	if audit.DeterministicValues != 4 || audit.AIQueried {
		t.Fatalf("audit=%+v", audit)
	}
}

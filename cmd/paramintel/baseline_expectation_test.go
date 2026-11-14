package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/baseline"
	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestValidateExpectedBaselineStatus(t *testing.T) {
	for _, expected := range []int{0, 100, 200, 401, 599} {
		if err := validateExpectedBaselineStatus(expected); err != nil {
			t.Fatalf("expected=%d should be valid: %v", expected, err)
		}
	}
	for _, expected := range []int{-1, 1, 99, 600} {
		if err := validateExpectedBaselineStatus(expected); err == nil {
			t.Fatalf("expected=%d should fail validation", expected)
		}
	}
}

func TestBaselineExpectationRejectsStableUnauthorizedResponse(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	profile, _, err := baseline.BuildWithSnapshot(context.Background(), srv.Client(),
		model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !profile.StatusStable || profile.StatusCode != http.StatusUnauthorized || requests != 3 {
		t.Fatalf("expected three stable 401 baseline samples: profile=%+v requests=%d", profile, requests)
	}
	// Legacy behavior remains available when the operator supplied no intent.
	if err := checkBaselineStatusExpectation(profile, 0); err != nil {
		t.Fatalf("unconfigured expectation must preserve current behavior: %v", err)
	}
	err = checkBaselineStatusExpectation(profile, http.StatusOK)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "200") {
		t.Fatalf("expected explicit mismatch for stable unauthorized baseline, got %v", err)
	}
	// The expectation check itself must not issue another request.
	if requests != 3 {
		t.Fatalf("unexpected requests after mismatch: %d", requests)
	}
}

func TestBaselineExpectationAcceptsStableIntendedResponse(t *testing.T) {
	profile := model.BaselineProfile{StatusStable: true, StatusCode: http.StatusOK}
	if err := checkBaselineStatusExpectation(profile, http.StatusOK); err != nil {
		t.Fatalf("matching expectation rejected: %v", err)
	}
}

func TestBaselineExpectationRejectsUnstableResponse(t *testing.T) {
	profile := model.BaselineProfile{StatusStable: false, StatusCode: http.StatusOK}
	if err := checkBaselineStatusExpectation(profile, http.StatusOK); err == nil ||
		!strings.Contains(err.Error(), "not stable") {
		t.Fatalf("unstable baseline must not satisfy the expectation: %v", err)
	}
}

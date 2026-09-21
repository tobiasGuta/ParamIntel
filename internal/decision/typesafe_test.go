package decision

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTypeSafeProviderChoiceContract(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "jev-test" {
			t.Fatalf("model=%v", payload["model"])
		}
		questions, ok := payload["questions"].(map[string]any)
		if !ok {
			t.Fatalf("questions=%T", payload["questions"])
		}
		next, ok := questions["next_experiment"].(map[string]any)
		if !ok || next["type"] != "choice" {
			t.Fatalf("next_experiment=%v", next)
		}
		criteria, ok := next["criteria"].(map[string]any)
		if !ok {
			t.Fatalf("criteria=%T", next["criteria"])
		}
		if _, ok := criteria[string(ActionRelatedValueProfile)]; !ok {
			t.Fatalf("missing related_value_profile criteria")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"jev-test",
			"answers":{
				"next_experiment":{
					"type":"choice",
					"choice":"related_value_profile",
					"confidence":0.91,
					"probabilities":{
						"related_value_profile":0.91,
						"stop":0.09
					}
				}
			},
			"usage":{"input_tokens":123,"output_tokens":0}
		}`))
	}))
	defer srv.Close()

	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "jev-test",
		Client:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Choose(context.Background(), Request{
		State: State{
			Candidate: CandidateState{Name: "visibility", Location: "query"},
			Verification: VerificationState{
				CandidateChanged: 3,
				CandidateTrials:  3,
				ControlTrials:    3,
				Confidence:       1,
			},
			RemainingRequestBudget: 12,
		},
		Options: []Option{
			{Action: ActionStop, Description: "stop"},
			{Action: ActionRelatedValueProfile, Description: "related values"},
		},
		Instructions: "choose next experiment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawAuth {
		t.Fatal("authorization header missing")
	}
	if result.Action != ActionRelatedValueProfile || result.Confidence != 0.91 {
		t.Fatalf("result=%+v", result)
	}
	if result.Probabilities[ActionStop] != 0.09 {
		t.Fatalf("probabilities=%v", result.Probabilities)
	}
}

func TestTypeSafeProviderRejectsUnknownAction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"jev-test",
			"answers":{
				"next_experiment":{
					"type":"choice",
					"choice":"invented_action",
					"confidence":0.99,
					"probabilities":{"invented_action":0.99}
				}
			},
			"usage":{"input_tokens":1,"output_tokens":0}
		}`))
	}))
	defer srv.Close()
	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "jev-test",
		Client:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Choose(context.Background(), Request{
		State: State{RemainingRequestBudget: 1},
		Options: []Option{
			{Action: ActionStop, Description: "stop"},
			{Action: ActionEnumProfile, Description: "enum"},
		},
		Instructions: "choose",
	})
	if err == nil {
		t.Fatal("expected unknown action to be rejected")
	}
}


type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testTypeSafeDecisionRequest() Request {
	return Request{
		State: State{
			Candidate: CandidateState{
				Name:      "region",
				Location:  "query",
				ValueKind: "string",
			},
			Evidence: EvidenceState{
				Paths: []string{"$.aliases.region"},
			},
			Verification: VerificationState{
				CandidateChanged: 3,
				CandidateTrials:  3,
				ControlChanged:   0,
				ControlTrials:    3,
				Confidence:       1,
			},
			RemainingRequestBudget: 8,
		},
		Options: []Option{
			{Action: ActionStop, Description: "stop"},
			{Action: ActionRelatedValueProfile, Description: "related values"},
		},
		Instructions: "choose next experiment",
	}
}

func TestNewTypeSafeProviderRequiresAPIKey(t *testing.T) {
	if _, err := NewTypeSafeProvider(TypeSafeConfig{}); err == nil {
		t.Fatal("expected missing API key to be rejected")
	}
}

func TestTypeSafeProviderRejectsHTTPFailures(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("provider failure"))
			}))
			defer srv.Close()

			provider, err := NewTypeSafeProvider(TypeSafeConfig{
				APIKey:  "test-key",
				BaseURL: srv.URL,
				Model:   "jev-test",
				Client:  srv.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Choose(context.Background(), testTypeSafeDecisionRequest()); err == nil {
				t.Fatalf("expected HTTP %d to fail", status)
			}
		})
	}
}

func TestTypeSafeProviderRejectsMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not-json"))
	}))
	defer srv.Close()

	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "jev-test",
		Client:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Choose(context.Background(), testTypeSafeDecisionRequest()); err == nil {
		t.Fatal("expected malformed JSON to fail")
	}
}

func TestTypeSafeProviderRejectsMissingAnswerAndChoice(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing answer",
			body: `{"model":"jev-test","answers":{}}`,
		},
		{
			name: "missing choice",
			body: `{"model":"jev-test","answers":{"next_experiment":{"type":"choice","confidence":0.8,"probabilities":{"stop":1}}}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			provider, err := NewTypeSafeProvider(TypeSafeConfig{
				APIKey:  "test-key",
				BaseURL: srv.URL,
				Model:   "jev-test",
				Client:  srv.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Choose(context.Background(), testTypeSafeDecisionRequest()); err == nil {
				t.Fatalf("expected %s to fail", tc.name)
			}
		})
	}
}

func TestTypeSafeProviderRejectsInvalidConfidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"jev-test",
			"answers":{
				"next_experiment":{
					"type":"choice",
					"choice":"related_value_profile",
					"confidence":1.2,
					"probabilities":{"related_value_profile":1}
				}
			}
		}`))
	}))
	defer srv.Close()

	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "jev-test",
		Client:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Choose(context.Background(), testTypeSafeDecisionRequest()); err == nil {
		t.Fatal("expected invalid confidence to fail")
	}
}

func TestTypeSafeProviderRejectsInvalidProbability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"jev-test",
			"answers":{
				"next_experiment":{
					"type":"choice",
					"choice":"related_value_profile",
					"confidence":0.8,
					"probabilities":{"related_value_profile":1.1}
				}
			}
		}`))
	}))
	defer srv.Close()

	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "jev-test",
		Client:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Choose(context.Background(), testTypeSafeDecisionRequest()); err == nil {
		t.Fatal("expected invalid probability to fail")
	}
}

func TestTypeSafeProviderRejectsTransportFailure(t *testing.T) {
	client := &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network unavailable")
		}),
	}
	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: "https://typesafe.invalid",
		Model:   "jev-test",
		Client:  client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Choose(context.Background(), testTypeSafeDecisionRequest()); err == nil {
		t.Fatal("expected transport failure to fail")
	}
}

func TestHybridPlannerFailsClosedOnTypeSafeHTTP429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	provider, err := NewTypeSafeProvider(TypeSafeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "jev-test",
		Client:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := (HybridPlanner{
		Local:    HeuristicPlanner{},
		Provider: provider,
	}).PlanNext(context.Background(), testTypeSafeDecisionRequest().State)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AppliedAction != ActionStop || !plan.Gated {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Provider != "typesafe" || plan.Model != "jev-test" {
		t.Fatalf("provider/model=%q/%q", plan.Provider, plan.Model)
	}
	if plan.GateReason != "decision provider failed closed" {
		t.Fatalf("gate reason=%q", plan.GateReason)
	}
}

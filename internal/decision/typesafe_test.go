package decision

import (
	"context"
	"encoding/json"
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

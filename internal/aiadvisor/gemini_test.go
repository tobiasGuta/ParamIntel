package aiadvisor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiProviderRequestAndStructuredResponse(t *testing.T) {
	var seenKey string
	var seen map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("x-goog-api-key")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &seen); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		structured := `{"candidates":[{"name":"include_archived","location":"query","json_parent":"","reason":"archive state observed","priority":90}]}`
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "completed",
			"steps": []any{map[string]any{
				"type":    "model_output",
				"content": []any{map[string]any{"type": "text", "text": structured}},
			}},
		})
	}))
	defer srv.Close()

	p, err := NewGeminiProvider(GeminiConfig{APIKey: "KEY123", Model: "gemini-test", Endpoint: srv.URL, Client: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Suggest(context.Background(), Input{
		Method:                 "GET",
		Path:                   "/api/projects",
		ActiveLocations:        []string{"query"},
		ExcludedCandidateNames: []string{"limit", "offset"},
		LocalCoveredNames:      []string{"private_custom_word"},
	}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if seenKey != "KEY123" {
		t.Fatalf("key header=%q", seenKey)
	}
	if seen["model"] != "gemini-test" {
		t.Fatalf("model=%v", seen["model"])
	}
	if seen["store"] != false {
		t.Fatalf("store=%v", seen["store"])
	}
	generation, ok := seen["generation_config"].(map[string]any)
	if !ok || generation["thinking_level"] != "low" {
		t.Fatalf("generation_config=%#v", seen["generation_config"])
	}
	inputText, _ := seen["input"].(string)
	if !strings.Contains(inputText, "excluded_candidate_names") || !strings.Contains(inputText, "limit") {
		t.Fatalf("provider exclusions missing from input: %s", inputText)
	}
	if strings.Contains(inputText, "private_custom_word") {
		t.Fatalf("local-only covered name leaked to provider: %s", inputText)
	}
	systemInstruction, _ := seen["system_instruction"].(string)
	if !strings.Contains(systemInstruction, "100 is the highest priority") || !strings.Contains(systemInstruction, "excluded_candidate_names") {
		t.Fatalf("priority/exclusion guidance missing: %s", systemInstruction)
	}
	rf, ok := seen["response_format"].(map[string]any)
	if !ok || rf["mime_type"] != "application/json" {
		t.Fatalf("response_format=%#v", seen["response_format"])
	}
	if len(got) != 1 || got[0].Name != "include_archived" || got[0].Priority != 90 {
		t.Fatalf("got=%+v", got)
	}
}

func TestGeminiProviderDefaultModel(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Provider: ProviderGemini, APIKey: "KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Model() != "gemini-3.5-flash-lite" {
		t.Fatalf("model=%q want=gemini-3.5-flash-lite", p.Model())
	}
}

func TestGeminiProviderDefaultTimeout(t *testing.T) {
	p, err := NewGeminiProvider(GeminiConfig{APIKey: "KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if p.client.Timeout != 2*time.Minute {
		t.Fatalf("timeout=%v want=2m", p.client.Timeout)
	}
}

func TestGeminiProviderRedactsAPIErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"invalid credentials"}}`)
	}))
	defer srv.Close()
	p, _ := NewGeminiProvider(GeminiConfig{APIKey: "KEY", Endpoint: srv.URL, Client: srv.Client()})
	_, err := p.Suggest(context.Background(), Input{ActiveLocations: []string{"query"}}, 1)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "invalid credentials") {
		t.Fatalf("err=%v", err)
	}
}

func TestDefaultAPIKeyEnv(t *testing.T) {
	got, err := DefaultAPIKeyEnv(" GEMINI ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "GEMINI_API_KEY" {
		t.Fatalf("got=%q", got)
	}
	if _, err := DefaultAPIKeyEnv("openai"); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

package aiadvisor

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestBuildInputRemovesSecretsAndValues(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method: "POST",
		URL:    "https://secret-target.example/api/users/123?token=supersecret&include=x",
		Headers: http.Header{
			"Authorization": []string{"Bearer TOPSECRET"},
			"Cookie":        []string{"session=COOKIESECRET"},
			"Content-Type":  []string{"application/json"},
		},
		Body: []byte(`{"email":"student@example.com","options":{"page_size":10,"csrf":"CSRFSECRET"},"active":true}`),
	}
	response := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"status\":\"private-state\",\"options\":{\"include_deleted\":false}}")
	input, err := BuildInput(tmpl, response, []string{"auto"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, secret := range []string{"secret-target.example", "supersecret", "TOPSECRET", "COOKIESECRET", "student@example.com", "CSRFSECRET", "private-state"} {
		if strings.Contains(text, secret) {
			t.Fatalf("sanitized input leaked %q: %s", secret, text)
		}
	}
	if input.Path != "/api/users/{int}" {
		t.Fatalf("path=%q", input.Path)
	}
	if len(input.QueryKeys) != 2 || input.QueryKeys[0] != "include" || input.QueryKeys[1] != "token" {
		t.Fatalf("query keys=%v", input.QueryKeys)
	}
	if input.RequestJSONShape["email"] != "string" {
		t.Fatalf("request shape=%#v", input.RequestJSONShape)
	}
	options, ok := input.ResponseJSONShape["options"].(map[string]any)
	if !ok || options["include_deleted"] != "boolean" {
		t.Fatalf("response shape=%#v", input.ResponseJSONShape)
	}
}

func TestBuildInputFormKeysOnly(t *testing.T) {
	tmpl := model.RequestTemplate{
		Method: "POST", URL: "https://example.test/login",
		Headers: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}},
		Body:    []byte("username=tobias&password=secret"),
	}
	input, err := BuildInput(tmpl, nil, []string{"auto"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(input.FormKeys, ",") != "password,username" {
		t.Fatalf("form=%v", input.FormKeys)
	}
	b, _ := json.Marshal(input)
	if strings.Contains(string(b), "tobias") || strings.Contains(string(b), "secret") {
		t.Fatalf("form values leaked: %s", b)
	}
}

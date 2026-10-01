package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tobiasGuta/ParamIntel/internal/externalhints"
)

func TestResolveRequestPathConfinesReadsToConfiguredRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "requests")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(root, "inside.req")
	outside := filepath.Join(base, "outside.req")
	for _, path := range []string{inside, outside} {
		if err := os.WriteFile(path, []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PARAMINTEL_MCP_REQUEST_ROOT", root)

	got, err := resolveRequestPath(inside)
	if err != nil {
		t.Fatalf("inside request rejected: %v", err)
	}
	if got == "" {
		t.Fatal("resolved path is empty")
	}
	if _, err := resolveRequestPath(outside); err == nil || !strings.Contains(err.Error(), "outside the configured MCP request root") {
		t.Fatalf("outside request error=%v", err)
	}

	link := filepath.Join(root, "escape.req")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRequestPath(link); err == nil || !strings.Contains(err.Error(), "outside the configured MCP request root") {
		t.Fatalf("symlink escape error=%v", err)
	}
}

func TestInspectRequestFileReturnsSanitizedStructure(t *testing.T) {
	root := t.TempDir()
	requestPath := filepath.Join(root, "request.txt")
	raw := strings.Join([]string{
		"POST /api/users?token=super-secret&mode=full HTTP/1.1",
		"Host: private.example",
		"Authorization: Bearer do-not-leak",
		"Cookie: session=do-not-leak",
		"Content-Type: application/json",
		"",
		`{"email":"secret@example.com","profile":{"role":"user"},"enabled":true}`,
	}, "\r\n")
	if err := os.WriteFile(requestPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARAMINTEL_MCP_REQUEST_ROOT", root)

	_, out, err := inspectRequestFile(context.Background(), nil, inspectRequestInput{
		RequestPath: requestPath,
		Scheme:      "https",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Method != "POST" || out.Path != "/api/users" {
		t.Fatalf("unexpected structure: %+v", out)
	}
	if len(out.QueryKeys) != 2 || out.QueryKeys[0] != "mode" || out.QueryKeys[1] != "token" {
		t.Fatalf("query keys=%v", out.QueryKeys)
	}
	if len(out.JSONParents) == 0 {
		t.Fatalf("expected JSON parents: %+v", out)
	}

	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{"super-secret", "private.example", "do-not-leak", "secret@example.com", `"user"`} {
		if strings.Contains(text, secret) {
			t.Fatalf("sanitized MCP inspection leaked %q: %s", secret, text)
		}
	}
}

func TestAnalyzeStateChangingRequiresCallerAndServerOptIn(t *testing.T) {
	root := t.TempDir()
	requestPath := filepath.Join(root, "request.txt")
	raw := "POST /api/users HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/json\r\n\r\n{}"
	if err := os.WriteFile(requestPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARAMINTEL_MCP_REQUEST_ROOT", root)
	t.Setenv("PARAMINTEL_MCP_ALLOW_STATE_CHANGING", "")

	hints := externalhints.Document{
		Candidates: []externalhints.CandidateHint{{
			Name:     "role",
			Location: "json",
			Values: []externalhints.ValueHint{{
				Value: "admin",
				Kind:  "string",
			}},
		}},
	}

	_, _, err := analyzeRequestFile(context.Background(), nil, analyzeRequestInput{
		RequestPath: requestPath,
		Hints:       hints,
	})
	if err == nil || !strings.Contains(err.Error(), "allow_state_changing") {
		t.Fatalf("caller gate error=%v", err)
	}

	_, _, err = analyzeRequestFile(context.Background(), nil, analyzeRequestInput{
		RequestPath:        requestPath,
		Hints:              hints,
		AllowStateChanging: true,
	})
	if err == nil || !strings.Contains(err.Error(), "disabled by the server operator") {
		t.Fatalf("server gate error=%v", err)
	}
}

func TestNormalizeMCPOptions(t *testing.T) {
	if got, err := normalizeScheme(""); err != nil || got != "https" {
		t.Fatalf("default scheme=%q err=%v", got, err)
	}
	if _, err := normalizeScheme("ftp"); err == nil {
		t.Fatal("expected invalid scheme rejection")
	}
	got, err := normalizeLocations([]string{"json", "query", "json"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "json,query" {
		t.Fatalf("locations=%q", got)
	}
	if _, err := normalizeLocations([]string{"header"}); err == nil {
		t.Fatal("expected invalid location rejection")
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestVersion(t *testing.T) {
	if version != "0.5.0" {
		t.Fatalf("version=%q want=0.5.0", version)
	}
}

func TestParseLocations(t *testing.T) {
	got, err := parseLocations("json, query,json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"json", "query"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("locations=%v want=%v", got, want)
	}
	if _, err := parseLocations("headers"); err == nil {
		t.Fatal("expected invalid location error")
	}
}

func TestMethodMayChangeState(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		if methodMayChangeState(method) {
			t.Fatalf("%s should be treated as non-state-changing", method)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if !methodMayChangeState(method) {
			t.Fatalf("%s should require explicit opt-in", method)
		}
	}
}

func TestValidateDelay(t *testing.T) {
	for _, delay := range []time.Duration{0, time.Millisecond, time.Second} {
		if err := validateDelay(delay); err != nil {
			t.Fatalf("delay=%v error=%v", delay, err)
		}
	}
	if err := validateDelay(-time.Millisecond); err == nil {
		t.Fatal("expected negative delay validation error")
	}
}

func TestCLIBackoffDoesNotWriteNormalReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	requestPath := filepath.Join(tmp, "request.txt")
	outputPath := filepath.Join(tmp, "findings.json")
	raw := "GET " + srv.URL + " HTTP/1.1\r\nHost: " + target.Host + "\r\n\r\n"
	if err := os.WriteFile(requestPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".", "-request", requestPath, "-baseline", "3", "-output", outputPath)
	combined, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected CLI failure, output:\n%s", combined)
	}
	text := string(combined)
	if !strings.Contains(text, "rate limit detected") || !strings.Contains(text, "response was not used as discovery evidence") {
		t.Fatalf("missing rate-limit evidence diagnostic:\n%s", text)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("normal findings report must not be written after backoff; stat error=%v", statErr)
	}
}

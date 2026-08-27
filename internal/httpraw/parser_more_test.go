package httpraw

import "testing"

func TestParseAbsoluteTargetAndBody(t *testing.T) {
	raw := []byte("POST https://example.test/search HTTP/1.1\r\nHost: ignored.test\r\nContent-Type: application/json\r\n\r\n{\"q\":\"x\"}")
	r, err := Parse(raw, "http")
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "https://example.test/search" {
		t.Fatalf("url=%q", r.URL)
	}
	if string(r.Body) != "{\"q\":\"x\"}" {
		t.Fatalf("body=%q", r.Body)
	}
}

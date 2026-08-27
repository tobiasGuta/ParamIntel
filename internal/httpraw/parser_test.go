package httpraw

import "testing"

func TestParseRelativeRawRequest(t *testing.T) {
	raw := []byte("GET /api/users?existing=1 HTTP/1.1\r\nHost: example.test\r\nAuthorization: Bearer abc\r\n\r\n")
	r, err := Parse(raw, "https")
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != "https://example.test/api/users?existing=1" {
		t.Fatalf("url=%q", r.URL)
	}
	if got := r.Headers.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("authorization=%q", got)
	}
}

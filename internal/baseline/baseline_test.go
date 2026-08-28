package baseline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tobiasGuta/ParamIntel/internal/model"
)

func TestSendPreservesExistingQueryAndAddsProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test" {
			t.Fatalf("authorization=%q", got)
		}
		fmt.Fprintf(w, `{"existing":%q,"probe":%q}`, r.URL.Query().Get("existing"), r.URL.Query().Get("probe"))
	}))
	defer srv.Close()

	tmpl := model.RequestTemplate{Method: "GET", URL: srv.URL + "?existing=1", Headers: http.Header{"Authorization": []string{"Bearer test"}}}
	s, err := Send(context.Background(), srv.Client(), tmpl, map[string]string{"probe": "yes"})
	if err != nil {
		t.Fatal(err)
	}
	if string(s.Body) != `{"existing":"1","probe":"yes"}` {
		t.Fatalf("body=%s", s.Body)
	}
}

func TestBuildLearnsJSONBaseline(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		fmt.Fprintf(w, `{"request_id":%d,"role":"member"}`, n)
	}))
	defer srv.Close()

	p, err := Build(context.Background(), srv.Client(), model.RequestTemplate{Method: "GET", URL: srv.URL}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if p.Samples != 3 || !p.IsJSON {
		t.Fatalf("profile=%+v", p)
	}
	if _, ok := p.StableJSONPaths["$.request_id"]; ok {
		t.Fatal("dynamic request_id should not be stable")
	}
	if got := p.StableJSONPaths["$.role"]; got != "s:member" {
		t.Fatalf("stable role=%q", got)
	}
}

func TestSendMutationsRejects429BeforeSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"too many requests"}`)
	}))
	defer srv.Close()

	s, err := SendMutations(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, nil)
	if err == nil {
		t.Fatalf("snapshot=%+v; expected rate-limit error", s)
	}
	var backoff *BackoffError
	if !errors.As(err, &backoff) {
		t.Fatalf("error type=%T %v", err, err)
	}
	if backoff.Kind != BackoffKindRateLimit || backoff.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("backoff=%+v", backoff)
	}
	if !backoff.RetryAfterValid || backoff.RetryAfter != 10*time.Second || backoff.RetryAfterRaw != "10" {
		t.Fatalf("retry metadata=%+v", backoff)
	}
	if len(s.Body) != 0 || s.StatusCode != 0 {
		t.Fatalf("rate-limit response must not become snapshot: %+v", s)
	}
}

func TestSendMutationsRejects429WithMalformedRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "later")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := SendMutations(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, nil)
	var backoff *BackoffError
	if !errors.As(err, &backoff) {
		t.Fatalf("error=%v", err)
	}
	if backoff.RetryAfterValid || backoff.RetryAfterRaw != "later" {
		t.Fatalf("backoff=%+v", backoff)
	}
	if !strings.Contains(err.Error(), "response was not used as discovery evidence") {
		t.Fatalf("error=%q", err.Error())
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, time.August, 28, 19, 0, 0, 0, time.UTC)
	retryAt := now.Add(45 * time.Second)
	delay, gotAt, ok := parseRetryAfter(retryAt.Format(http.TimeFormat), now)
	if !ok {
		t.Fatal("expected valid HTTP-date Retry-After")
	}
	if delay != 45*time.Second || !gotAt.Equal(retryAt) {
		t.Fatalf("delay=%v retryAt=%v", delay, gotAt)
	}
}

func TestSendMutationsRejects503OnlyWithValidRetryAfter(t *testing.T) {
	t.Run("valid retry-after", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		_, err := SendMutations(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, nil)
		var backoff *BackoffError
		if !errors.As(err, &backoff) {
			t.Fatalf("error=%v", err)
		}
		if backoff.Kind != BackoffKindServerBackoff || backoff.RetryAfter != 5*time.Second {
			t.Fatalf("backoff=%+v", backoff)
		}
	})

	t.Run("missing retry-after remains ordinary response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "maintenance")
		}))
		defer srv.Close()

		s, err := SendMutations(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if s.StatusCode != http.StatusServiceUnavailable || string(s.Body) != "maintenance" {
			t.Fatalf("snapshot=%+v", s)
		}
	})

	t.Run("malformed retry-after remains ordinary response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "later")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "maintenance")
		}))
		defer srv.Close()

		s, err := SendMutations(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if s.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("snapshot=%+v", s)
		}
	})
}

func TestSendMutationsDoesNotClassify403AsRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "forbidden")
	}))
	defer srv.Close()

	s, err := SendMutations(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.StatusCode != http.StatusForbidden || string(s.Body) != "forbidden" {
		t.Fatalf("snapshot=%+v", s)
	}
}

func TestBuildFailsWhenBaselineIsRateLimited(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 2 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	p, err := Build(context.Background(), srv.Client(), model.RequestTemplate{Method: http.MethodGet, URL: srv.URL}, 3)
	if err == nil {
		t.Fatalf("profile=%+v; expected baseline failure", p)
	}
	var backoff *BackoffError
	if !errors.As(err, &backoff) {
		t.Fatalf("error=%v", err)
	}
	if requests != 2 {
		t.Fatalf("requests=%d want=2; baseline must stop immediately", requests)
	}
}

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRateLimitedResponseBodyIsClosed(t *testing.T) {
	body := &trackingBody{Reader: strings.NewReader("rate limited")}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Body:       body,
			Request:    r,
		}, nil
	})}

	_, err := SendMutations(context.Background(), client, model.RequestTemplate{Method: http.MethodGet, URL: "http://example.test"}, nil)
	if err == nil {
		t.Fatal("expected rate-limit error")
	}
	if !body.closed {
		t.Fatal("rate-limited response body was not closed")
	}
}

package httppolicy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func okResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    req,
	}
}

func TestNewPacedTransportZeroDelayReturnsBase(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return okResponse(req), nil
	})
	if got := NewPacedTransport(base, 0); got != base {
		t.Fatalf("zero delay should return underlying transport unchanged")
	}
}

func TestSpacingNeededUsesRequestStartInterval(t *testing.T) {
	now := time.Date(2026, time.August, 28, 19, 0, 0, 0, time.UTC)
	if got := spacingNeeded(now.Add(-400*time.Millisecond), now, 250*time.Millisecond); got != 0 {
		t.Fatalf("slow prior request should require no extra wait, got %v", got)
	}
	if got := spacingNeeded(now.Add(-20*time.Millisecond), now, 250*time.Millisecond); got != 230*time.Millisecond {
		t.Fatalf("fast prior request spacing=%v want=230ms", got)
	}
}

func TestPacedTransportEnforcesMinimumStartSpacing(t *testing.T) {
	var mu sync.Mutex
	var starts []time.Time
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return okResponse(req), nil
	})
	client := &http.Client{Transport: NewPacedTransport(base, 40*time.Millisecond)}

	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.test", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	if len(starts) != 3 {
		t.Fatalf("starts=%d want=3", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 30*time.Millisecond {
			t.Fatalf("request start gap=%v; expected pacing near 40ms", gap)
		}
	}
}

func TestPacedTransportWaitHonorsContextCancellation(t *testing.T) {
	var calls atomic.Int32
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return okResponse(req), nil
	})
	client := &http.Client{Transport: NewPacedTransport(base, 500*time.Millisecond)}

	first, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.test", nil)
	resp, err := client.Do(first)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	ctx, cancel := context.WithCancel(context.Background())
	second, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.test", nil)
	done := make(chan error, 1)
	go func() {
		_, err := client.Do(second)
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context canceled", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("paced request did not stop promptly after cancellation")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying transport calls=%d want=1", got)
	}
}

func TestPacedTransportConcurrentUseIsSafe(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return okResponse(req), nil
	})
	client := &http.Client{Transport: NewPacedTransport(base, time.Millisecond)}

	const n = 12
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.test", nil)
			if err != nil {
				errCh <- err
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				errCh <- err
				return
			}
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

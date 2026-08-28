package httppolicy

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// NewPacedTransport wraps an HTTP transport with a per-run minimum interval
// between outbound request starts. A non-positive delay leaves the underlying
// transport unchanged.
func NewPacedTransport(base http.RoundTripper, delay time.Duration) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if delay <= 0 {
		return base
	}
	return &pacingTransport{base: base, delay: delay}
}

type pacingTransport struct {
	base      http.RoundTripper
	delay     time.Duration
	mu        sync.Mutex
	lastStart time.Time
}

func (t *pacingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// wait serializes only the request-start gate. The network request itself runs
// after the mutex is released, so future callers are paced by start time rather
// than response completion time.
func (t *pacingTransport) wait(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	if remaining := spacingNeeded(t.lastStart, time.Now(), t.delay); remaining > 0 {
		if err := waitContext(ctx, remaining); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	t.lastStart = time.Now()
	return nil
}

func spacingNeeded(lastStart, now time.Time, delay time.Duration) time.Duration {
	if delay <= 0 || lastStart.IsZero() {
		return 0
	}
	remaining := delay - now.Sub(lastStart)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

func waitContext(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

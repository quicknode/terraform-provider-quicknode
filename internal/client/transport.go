package client

import (
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"

	"golang.org/x/time/rate"
	"time"
)

const (
	defaultMaxRetries        = 4
	defaultBaseDelay         = 500 * time.Millisecond
	defaultMaxDelay          = 30 * time.Second
	defaultRequestsPerSecond = 5
)

// retryTransport retries throttled and transient responses. A terraform apply
// over a large workspace bursts many Admin API calls at once, so the provider
// backs off instead of surfacing a 429 as a resource failure.
type retryTransport struct {
	base       http.RoundTripper
	apiKey     string
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	limiter    *rate.Limiter
	sleep      func(time.Duration)
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	sleep := t.sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	if t.limiter != nil {
		if err := t.limiter.Wait(req.Context()); err != nil {
			return nil, err
		}
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			default:
			}
			sleep(t.delayFor(attempt, lastResp))
		}

		if lastResp != nil {
			drain(lastResp)
		}

		clone := req.Clone(req.Context())
		if t.apiKey != "" {
			clone.Header.Set("x-api-key", t.apiKey)
		}
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			clone.Body = body
		}

		lastResp, lastErr = base.RoundTrip(clone)
		if lastErr != nil {
			return nil, lastErr
		}
		if !shouldRetry(lastResp.StatusCode) {
			return lastResp, nil
		}
		if req.GetBody == nil && req.Body != nil {
			return lastResp, nil
		}
	}

	return lastResp, lastErr
}

func (t *retryTransport) delayFor(attempt int, resp *http.Response) time.Duration {
	if after := retryAfter(resp); after > 0 {
		return min(after, t.maxDelay)
	}
	backoff := time.Duration(float64(t.baseDelay) * math.Pow(2, float64(attempt-1)))
	jitter := time.Duration(rand.Int63n(int64(t.baseDelay)))
	return min(backoff+jitter, t.maxDelay)
}

func retryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	value := resp.Header.Get("Retry-After")
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func shouldRetry(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func drain(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

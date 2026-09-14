// Throttle handling: HTTP 429/503 responses are retried with Retry-After or
// capped exponential backoff, keeping transient rate limits out of the callers.

package client

import (
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Automatic backoff on throttling (HTTP 429/503): retry up to maxRetries times,
// honouring Retry-After, else exponential backoff with jitter, each wait capped.
const (
	maxRetries       = 3
	defaultRetryBase = 500 * time.Millisecond
	maxRetryWait     = 10 * time.Second
)

func isRateLimited(err error) bool {
	status, ok := HTTPStatus(err)
	return ok && (status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable)
}

// parseRetryAfter reads Retry-After in delta-seconds or HTTP-date form; -1 when
// absent or invalid.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return -1
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(h); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
		return 0
	}
	return -1
}

func retryWait(err error, backoff time.Duration) time.Duration {
	var ae *APIError
	if errors.As(err, &ae) && ae.RetryAfter >= 0 {
		return capWait(ae.RetryAfter)
	}
	return capWait(backoff + time.Duration(rand.Int63n(int64(250*time.Millisecond))))
}

func capWait(d time.Duration) time.Duration {
	if d > maxRetryWait {
		return maxRetryWait
	}
	if d < 0 {
		return 0
	}
	return d
}

// doWithRetry executes req, retrying transient throttling (429/503): up to
// maxRetries times, honouring Retry-After else exponential backoff with jitter.
// A request with a body is replayed via GetBody (set by http.NewRequest for the
// in-memory readers this client uses), so POST retries re-send the body.
func (c *Client) doWithRetry(req *http.Request) ([]byte, error) {
	backoff := defaultRetryBase
	for attempt := 0; ; attempt++ {
		attemptReq := req
		if attempt > 0 && req.GetBody != nil {
			b, berr := req.GetBody()
			if berr != nil {
				return nil, berr
			}
			attemptReq = req.Clone(req.Context())
			attemptReq.Body = b
		}
		c.refreshSessionHeaders(attemptReq)
		data, err := c.do(attemptReq)
		if !isRateLimited(err) || attempt >= maxRetries {
			return data, err
		}
		wait := retryWait(err, backoff)
		status, _ := HTTPStatus(err)
		c.logf("throttled: HTTP %d on %s — retrying %d/%d in %s", status, req.URL, attempt+1, maxRetries, wait.Round(time.Millisecond))
		time.Sleep(wait)
		backoff *= 2
	}
}

// Package client is a thin, dependency-free Go client for leroymerlin.es. The
// site has no public API; it server-renders its pages behind DataDome bot
// protection. This client presents Chrome's TLS fingerprint (uTLS) so DataDome
// keeps it off the JS-challenge path, fetches the same SSR HTML the browser
// gets, and lifts the structured JSON the page already embeds (per-product
// `cdl_products_list` blobs on search, schema.org JSON-LD on product pages).
package client

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// BaseURL is the Leroy Merlin España storefront.
	BaseURL = "https://www.leroymerlin.es"
	// DefaultUA mirrors a current desktop Chrome so requests look like the web app.
	DefaultUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
)

// Client is the API entrypoint. The zero value is not usable — use New.
type Client struct {
	HTTP      *http.Client
	BaseURL   string
	UserAgent string
	Lang      string // "es" (default) or "ca"

	// Cookie carries a browser session — notably the DataDome clearance cookie —
	// for the case where anonymous uTLS reads still draw a bot challenge. Lift it
	// with `leroymerlin set-cookie`. Empty = anonymous.
	Cookie string

	// Logf, when set, receives human-readable diagnostics about otherwise-silent
	// resilience events (throttle retries, transport fallback). Diagnostics belong
	// on stderr; nil disables them. The library never writes to os.Stderr itself.
	Logf func(format string, args ...any)

	// transportErr records why the uTLS Chrome transport could not initialise (nil
	// when it did). New cannot log it — Logf is set by the caller afterwards — so it
	// is surfaced lazily on the first request.
	transportErr error
	warnOnce     sync.Once
}

// logf emits a diagnostic through the optional Logf hook (no-op when unset).
func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// New returns a Client with web-app-like defaults (anonymous, Spanish). Its HTTP
// client presents Chrome's TLS (JA3) fingerprint via uTLS; if that fails to
// initialize it falls back to the stdlib transport.
func New() *Client {
	hc := &http.Client{Timeout: 30 * time.Second}
	c := &Client{
		HTTP:      hc,
		BaseURL:   BaseURL,
		UserAgent: DefaultUA,
		Lang:      "es",
	}
	if tr, err := newChromeTransport(); err == nil {
		hc.Transport = tr
	} else {
		c.transportErr = err
	}
	return c
}

// APIError carries a non-2xx response so callers (and agents) can branch on it.
// RetryAfter is the parsed Retry-After header on a 429/503 (>=0 when present).
type APIError struct {
	Status     int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("leroymerlin: HTTP %d: %s", e.Status, truncate(e.Body, 300))
}

// HTTPStatus reports the HTTP status carried by err when it is (or wraps) an
// *APIError. ok is false for any other error.
func HTTPStatus(err error) (status int, ok bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Status, true
	}
	return 0, false
}

// newReq builds a browser-like GET request for a storefront page.
func (c *Client) newReq(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if c.Lang == "ca" {
		req.Header.Set("accept-language", "ca-ES,ca;q=0.9,es;q=0.8,en;q=0.7")
	} else {
		req.Header.Set("accept-language", "es-ES,es;q=0.9,en;q=0.8")
	}
	req.Header.Set("user-agent", c.UserAgent)
	req.Header.Set("referer", c.BaseURL+"/")
	req.Header.Set("upgrade-insecure-requests", "1")
	if c.Cookie != "" {
		req.Header.Set("cookie", c.Cookie)
	}
	return req, nil
}

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

// parseRetryAfter reads the delta-seconds form of Retry-After; -1 when absent.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return -1
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
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

// maxBodyBytes caps how much of any response body we read into memory.
const maxBodyBytes = 16 << 20

// do sends req, reads the (capped) response body, and turns a non-2xx status
// into an *APIError carrying that body and any Retry-After.
func (c *Client) do(req *http.Request) ([]byte, error) {
	c.warnOnce.Do(func() {
		if c.transportErr != nil {
			c.logf("uTLS Chrome fingerprint unavailable (%v) — using the stdlib transport; requests may draw the DataDome JS challenge", c.transportErr)
		}
	})
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{
			Status:     resp.StatusCode,
			Body:       string(data),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	if readErr != nil {
		return nil, fmt.Errorf("read response body from %s: %w", req.URL, readErr)
	}
	return data, nil
}

// GetHTML fetches url (resolved against BaseURL if it is a path) and returns the
// SSR HTML, retrying transient throttling (429/503) so a burst of reads degrades
// gracefully instead of failing.
func (c *Client) GetHTML(url string) (string, error) {
	url = c.resolve(url)
	backoff := defaultRetryBase
	for attempt := 0; ; attempt++ {
		body, err := c.getOnce(url)
		if !isRateLimited(err) || attempt >= maxRetries {
			if err != nil {
				return "", err
			}
			return body, nil
		}
		wait := retryWait(err, backoff)
		status, _ := HTTPStatus(err)
		c.logf("throttled: HTTP %d on %s — retrying %d/%d in %s", status, url, attempt+1, maxRetries, wait.Round(time.Millisecond))
		time.Sleep(wait)
		backoff *= 2
	}
}

func (c *Client) getOnce(url string) (string, error) {
	req, err := c.newReq("GET", url)
	if err != nil {
		return "", err
	}
	data, err := c.do(req)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// resolve turns a site-relative path into an absolute URL against BaseURL; an
// already-absolute URL is returned unchanged.
func (c *Client) resolve(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	return c.BaseURL + u
}

// truncate caps s to n runes (with an ellipsis), never splitting a multibyte
// rune — error bodies are UTF-8 HTML and a byte cut would emit invalid UTF-8.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseRetryAfterHTTPDate(t *testing.T) {
	header := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(header)
	if got <= 0 || got > 10*time.Second {
		t.Fatalf("Retry-After date = %s, want a positive delay no greater than 10s", got)
	}
}

func TestResolveAvoidsDoubleSlashWithTrailingBaseURLSlash(t *testing.T) {
	c := New()
	c.BaseURL = "https://example.test/"
	if got := c.resolve("/search"); got != "https://example.test/search" {
		t.Fatalf("resolved URL = %q", got)
	}
}

func TestRetryUsesCookiesRotatedByThrottleResponse(t *testing.T) {
	attempts := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.SetCookie(w, &http.Cookie{Name: "retry-token", Value: "new", Path: "/"})
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if got := cookieValue(r.Header.Get("Cookie"), "retry-token"); got != "new" {
			http.Error(w, "stale retry cookie", http.StatusPreconditionFailed)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	c.Cookie = "datadome=DD; retry-token=old"
	if got, err := c.GetHTML("/"); err != nil || got != "ok" {
		t.Fatalf("retry result = %q, %v", got, err)
	}
}

func TestConcurrentRequestsKeepSessionStateSafe(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "request-token", Value: "ok", Path: "/"})
		_, _ = w.Write([]byte("ok"))
	})
	c.Cookie = "datadome=DD"
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.GetHTML("/"); err != nil {
				t.Errorf("request failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := cookieValue(c.Cookie, "request-token"); got != "ok" {
		t.Fatalf("request cookie = %q, want ok", got)
	}
}

// The mirror image of the test below: a foreign host must not be able to write
// to the session either. Its Set-Cookie used to be merged straight into the
// client, so a page that redirected the CLI off-site could plant a value that
// then rode along on the next request to the storefront.
func TestExternalResponsesCannotWriteToTheSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "datadome", Value: "PLANTED", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "lm-csrf", Value: "PLANTED", Path: "/"})
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New()
	c.HTTP = srv.Client()
	c.BaseURL = BaseURL
	c.Cookie = "datadome=DD"
	if _, err := c.GetHTML(srv.URL + "/productos/x.html"); err != nil {
		t.Fatal(err)
	}
	if got := c.CurrentSession().Cookie; got != "datadome=DD" {
		t.Fatalf("session = %q, want the foreign Set-Cookie ignored", got)
	}
}

func TestExternalRequestsDoNotReceiveSessionCookie(t *testing.T) {
	var leaked bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Cookie") != ""
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New()
	c.HTTP = srv.Client()
	c.BaseURL = BaseURL
	c.Cookie = "datadome=DD"
	if _, err := c.GetHTML(srv.URL + "/productos/x.html"); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Fatal("session cookie was sent to an external host")
	}
}

func TestCapWaitClampsBothEnds(t *testing.T) {
	for _, tc := range []struct{ in, want time.Duration }{
		{-1 * time.Second, 0},
		{0, 0},
		{2 * time.Second, 2 * time.Second},
		{maxRetryWait, maxRetryWait},
		{time.Hour, maxRetryWait},
	} {
		if got := capWait(tc.in); got != tc.want {
			t.Errorf("capWait(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// A retried POST must re-send its body: http.NewRequest sets GetBody for the
// in-memory readers this client uses, and doWithRetry replays through it. A
// throttled write that retried with an empty body would silently do nothing.
func TestRetriedPostResendsItsBody(t *testing.T) {
	var bodies []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if len(bodies) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.postJSON("/checkout/backend/cart", map[string]string{"ref": "abc"}, nil); err != nil {
		t.Fatalf("postJSON: %v", err)
	}

	if len(bodies) != 2 {
		t.Fatalf("server saw %d requests, want 2 (one throttled, one retried)", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("retry sent a different body:\n first: %q\n retry: %q", bodies[0], bodies[1])
	}
	if !strings.Contains(bodies[1], "abc") {
		t.Errorf("retried body lost its content: %q", bodies[1])
	}
}

// Once the retry budget is spent the throttle error reaches the caller rather
// than looping forever.
func TestRetriesGiveUpAndSurfaceTheThrottle(t *testing.T) {
	var hits int
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	err := c.getJSON("/checkout/backend/cart", &struct{}{})
	if err == nil {
		t.Fatal("want the throttle surfaced after the budget is spent")
	}
	if status, ok := HTTPStatus(err); !ok || status != http.StatusTooManyRequests {
		t.Errorf("err = %v, want an HTTP 429", err)
	}
	if hits != maxRetries+1 {
		t.Errorf("server saw %d attempts, want %d", hits, maxRetries+1)
	}
}

// Retry-After from the server wins over the computed backoff; without it the
// wait is the backoff plus jitter, always inside the cap.
func TestRetryWaitPrefersServerGuidance(t *testing.T) {
	honoured := retryWait(&APIError{Status: 429, RetryAfter: 2 * time.Second}, time.Minute)
	if honoured != 2*time.Second {
		t.Errorf("wait = %v, want the 2s Retry-After", honoured)
	}

	capped := retryWait(&APIError{Status: 429, RetryAfter: time.Hour}, time.Second)
	if capped != maxRetryWait {
		t.Errorf("wait = %v, want it capped at %v", capped, maxRetryWait)
	}

	// No Retry-After: fall back to backoff + jitter, still bounded.
	backoff := retryWait(errors.New("boom"), time.Second)
	if backoff < time.Second || backoff > maxRetryWait {
		t.Errorf("wait = %v, want between 1s and %v", backoff, maxRetryWait)
	}

	// A negative Retry-After means "absent", not "wait forever backwards".
	absent := retryWait(&APIError{Status: 429, RetryAfter: -1}, time.Second)
	if absent < time.Second || absent > maxRetryWait {
		t.Errorf("wait = %v, want the backoff path", absent)
	}
}

// Retry-After arrives as delta-seconds or an HTTP date; anything else means
// "absent" (-1) so the caller falls back to its own backoff.
func TestParseRetryAfterFormats(t *testing.T) {
	if got := parseRetryAfter("5"); got != 5*time.Second {
		t.Errorf("delta-seconds = %v, want 5s", got)
	}
	if got := parseRetryAfter("  7 "); got != 7*time.Second {
		t.Errorf("padded delta-seconds = %v, want 7s", got)
	}
	if got := parseRetryAfter("0"); got != 0 {
		t.Errorf("zero = %v, want 0", got)
	}
	for _, absent := range []string{"", "later", "-3"} {
		if got := parseRetryAfter(absent); got != -1 {
			t.Errorf("parseRetryAfter(%q) = %v, want -1 (absent)", absent, got)
		}
	}
	// A date already in the past means "retry now", not a negative wait.
	if got := parseRetryAfter(time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)); got != 0 {
		t.Errorf("past date = %v, want 0", got)
	}
	future := parseRetryAfter(time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat))
	if future <= 0 || future > 31*time.Second {
		t.Errorf("future date = %v, want about 30s", future)
	}
}

// A retried POST rebuilds its body through GetBody. If that rebuild fails the
// caller must see the body error, not the 429 that triggered the retry — the
// request never went out, so reporting it as throttling would be a lie.
func TestDoWithRetryReportsAFailedBodyReplay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	req, err := http.NewRequest("POST", srv.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("body gone") }

	c := New()
	c.BaseURL = srv.URL
	_, err = c.doWithRetry(req)
	if err == nil || !strings.Contains(err.Error(), "body gone") {
		t.Fatalf("err = %v, want the body-replay failure", err)
	}
	var ae *APIError
	if errors.As(err, &ae) {
		t.Errorf("err = %v, want the body error rather than the throttle", err)
	}
}

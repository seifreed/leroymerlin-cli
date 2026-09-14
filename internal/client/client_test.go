package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
)

// rtFunc adapts a function to http.RoundTripper so a test can fail a request at
// a chosen point without a server.
type rtFunc func(*http.Request) (*http.Response, error)

// newTestClient starts a stub storefront serving h and returns a client aimed at
// it — the httptest.NewServer + Close + New + BaseURL block every transport-level
// test in this package would otherwise repeat.
func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New()
	c.BaseURL = srv.URL
	return c
}

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// errBody is a response body that fails mid-read, which is what a truncated
// connection looks like after the status line has already arrived.
type errBody struct{}

func (errBody) Read([]byte) (int, error) { return 0, errors.New("connection reset mid-body") }
func (errBody) Close() error             { return nil }

// A URL that cannot form a request must be reported, not silently skipped.
func TestGetHTMLRejectsAnUnusableURL(t *testing.T) {
	c := New()
	if _, err := c.GetHTML("/\x7f"); err == nil {
		t.Fatal("want an error for a URL with a control character")
	}
}

func TestDoReportsATransportFailure(t *testing.T) {
	c := New()
	c.HTTP = &http.Client{Transport: rtFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial refused")
	})}

	if _, err := c.GetHTML("/x"); err == nil || !strings.Contains(err.Error(), "dial refused") {
		t.Fatalf("err = %v, want the transport failure", err)
	}
}

// A 2xx whose body fails mid-read is a distinct failure from a non-2xx status:
// the latter keeps whatever arrived as the error body, the former has nothing
// usable and must say so, naming the URL.
func TestDoReportsAnUnreadableResponseBody(t *testing.T) {
	c := New()
	c.HTTP = &http.Client{Transport: rtFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: errBody{}}, nil
	})}

	_, err := c.GetHTML("/x")
	if err == nil || !strings.Contains(err.Error(), "read response body from") {
		t.Fatalf("err = %v, want the body-read failure", err)
	}
}

var _ io.ReadCloser = errBody{}

// When the Chrome fingerprint cannot be built, New falls back to the stdlib
// transport and records why. The warning is deferred to the first request
// because Logf is set by the caller after New returns — and it must fire once,
// not on every request, or a long run drowns in it.
func TestNewWarnsOnceWhenTheChromeTransportIsUnavailable(t *testing.T) {
	withHelloID(t, utls.ClientHelloID{Client: "nope", Version: "0"})

	c := New()
	if c.transportErr == nil {
		t.Fatal("want the transport failure recorded")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()
	c.BaseURL = srv.URL

	var diagnostics []string
	c.Logf = func(format string, args ...any) {
		diagnostics = append(diagnostics, fmt.Sprintf(format, args...))
	}
	for range 2 {
		if _, err := c.GetHTML("/"); err != nil {
			t.Fatalf("GetHTML: %v", err)
		}
	}

	var warnings int
	for _, d := range diagnostics {
		if strings.Contains(d, "uTLS Chrome fingerprint unavailable") {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("warned %d times across two requests, want exactly 1: %v", warnings, diagnostics)
	}
}

package client

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// The cart/checkout endpoints run a stricter DataDome rule that scores a request
// as a bot unless it carries the Client Hints + Fetch Metadata a real Chrome XHR
// sends. This locks in that header set (a regression here is a silent 403 wall).
func TestNewJSONReqBrowserHeaders(t *testing.T) {
	c := New()
	c.Cookie = "datadome=DD; lm-csrf=TOK"
	req, err := c.newJSONReq("POST", "/cart/services/addToCart", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"accept":               "application/json, text/plain, */*",
		"accept-language":      "es-ES,es;q=0.9,en;q=0.8",
		"sec-ch-ua-mobile":     "?0",
		"sec-ch-ua-platform":   `"macOS"`,
		"sec-fetch-dest":       "empty",
		"sec-fetch-mode":       "cors",
		"sec-fetch-site":       "same-origin",
		"x-requested-with":     "XMLHttpRequest",
		"x-app-client-version": "v3.154.0",
		"place-type":           "ONLINE",
		"device-type":          "DESKTOP",
		"interface-type":       "WEB",
		"content-type":         "application/json",
		"lm-csrf":              "TOK", // double-submit token lifted from the cookie
	}
	for k, v := range want {
		if got := req.Header.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	if req.Header.Get("sec-ch-ua") == "" {
		t.Error("sec-ch-ua must be set")
	}
	if req.Header.Get("x-tab-id") == "" {
		t.Error("x-tab-id must be set")
	}
}

// A GET with no body must omit content-type but still carry the bot-evasion set.
func TestNewJSONReqGetNoBody(t *testing.T) {
	c := New()
	req, err := c.newJSONReq("GET", "/header-cart-module/backend/rest/cart-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ct := req.Header.Get("content-type"); ct != "" {
		t.Errorf("GET should have no content-type, got %q", ct)
	}
	if req.Header.Get("sec-fetch-site") != "same-origin" {
		t.Error("GET still needs the fetch-metadata headers")
	}
	// no cookie → no lm-csrf header
	if req.Header.Get("lm-csrf") != "" {
		t.Error("lm-csrf should be empty without a cookie")
	}
}

// Both request builders send the same Accept-Language: the storefront serves
// Spanish only, so there is nothing to negotiate — but a missing header is a bot
// tell, and the XHR endpoints are the ones scored most strictly.
func TestBothRequestBuildersSendTheSameAcceptLanguage(t *testing.T) {
	c := New()
	htmlReq, err := c.newReq("GET", c.resolve("/search?q=x"))
	if err != nil {
		t.Fatal(err)
	}
	jsonReq, err := c.newJSONReq("GET", "/checkout/backend/cart", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := htmlReq.Header.Get("accept-language"); got != acceptLanguage {
		t.Errorf("html accept-language = %q, want %q", got, acceptLanguage)
	}
	if got := jsonReq.Header.Get("accept-language"); got != acceptLanguage {
		t.Errorf("json accept-language = %q, want %q", got, acceptLanguage)
	}
}

func TestNewJSONReqCheckoutReferer(t *testing.T) {
	c := New()
	req, err := c.newJSONReq("PUT", "/checkout/backend/cart/update-offer-line-quantity/line", map[string]int{"quantity": 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("referer"); got != c.BaseURL+"/checkout/cart" {
		t.Errorf("checkout referer = %q, want checkout cart", got)
	}
}

func TestCurrentSessionSnapshotsCookieAndTab(t *testing.T) {
	c := New()
	c.Cookie = "datadome=abc; lm-csrf=tok"
	c.TabID = "tab-1"

	got := c.CurrentSession()
	if got.Cookie != c.Cookie || got.TabID != "tab-1" {
		t.Fatalf("CurrentSession() = %+v, want the live cookie and tab", got)
	}

	// The snapshot must be a copy: later mutation of the client must not alter it.
	c.Cookie = "datadome=rotated"
	if got.Cookie == c.Cookie {
		t.Error("CurrentSession returned a live view instead of a snapshot")
	}
}

// truncate caps error-body excerpts. Bodies are UTF-8 HTML, so a byte-wise cut
// would emit invalid UTF-8 — the rune path is the point of the function.
func TestTruncateCutsOnRunesNotBytes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly-10", 10, "exactly-10"},
		{"0123456789abc", 10, "0123456789…"},
		{"ñññññ", 3, "ñññ…"},
		{"ñññ", 10, "ñññ"},
		// 10 bytes but only 5 runes: over the byte cap, under the rune cap.
		{"ñññññ", 7, "ñññññ"},
	} {
		if got := truncate(tc.in, tc.n); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
		if !utf8.ValidString(truncate(tc.in, tc.n)) {
			t.Errorf("truncate(%q, %d) produced invalid UTF-8", tc.in, tc.n)
		}
	}
}

// The library never writes to stderr itself: diagnostics go through the caller's
// hook, and a nil hook must be a silent no-op rather than a panic.
func TestLogfIsASilentNoOpWithoutAHook(t *testing.T) {
	c := New()
	c.Logf = nil
	c.logf("this must not panic %d", 1)

	var got []string
	c.Logf = func(format string, args ...any) {
		got = append(got, fmt.Sprintf(format, args...))
	}
	c.logf("throttled %d", 429)
	if len(got) != 1 || got[0] != "throttled 429" {
		t.Fatalf("hook received %v, want one formatted message", got)
	}
}

// A BaseURL that cannot form a request must surface as an error, not a panic or
// a silently skipped call.
func TestJSONHelpersRejectAnUnusableBaseURL(t *testing.T) {
	c := New()
	c.BaseURL = "http://\x7f-invalid"

	if err := c.getJSON("/x", &struct{}{}); err == nil {
		t.Error("getJSON should reject an unusable URL")
	}
	if err := c.putJSON("/x", map[string]string{"a": "b"}); err == nil {
		t.Error("putJSON should reject an unusable URL")
	}
	if err := c.DeleteLine("line-1"); err == nil {
		t.Error("DeleteLine should reject an unusable URL")
	}
}

// resolve turns site-relative paths into absolute URLs and leaves already
// absolute ones alone — the guard that keeps a search result's URL usable
// whichever form the storefront returned it in.
func TestResolveHandlesBothURLForms(t *testing.T) {
	c := New()
	c.BaseURL = "https://www.leroymerlin.es"

	for _, tc := range []struct{ in, want string }{
		{"https://other.example/x", "https://other.example/x"},
		{"http://other.example/x", "http://other.example/x"},
		{"/productos/a.html", "https://www.leroymerlin.es/productos/a.html"},
		{"productos/a.html", "https://www.leroymerlin.es/productos/a.html"},
	} {
		if got := c.resolve(tc.in); got != tc.want {
			t.Errorf("resolve(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	c.BaseURL = "https://www.leroymerlin.es/"
	if got := c.resolve("/x"); got != "https://www.leroymerlin.es/x" {
		t.Errorf("trailing slash produced %q", got)
	}
}

// sameOrigin gates whether the session cookie is attached, so anything it
// cannot parse or that differs in scheme or host must be treated as foreign.
func TestSameOriginRejectsAnythingNotTheStorefront(t *testing.T) {
	c := New()
	c.BaseURL = "https://www.leroymerlin.es"

	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"https://www.leroymerlin.es/productos/a.html", true},
		{"https://WWW.LEROYMERLIN.ES/x", true},
		{"http://www.leroymerlin.es/x", false},
		{"https://evil.example/x", false},
		{"https://leroymerlin.es.evil.example/x", false},
		{"://not a url", false},
	} {
		if got := c.sameOrigin(tc.in); got != tc.want {
			t.Errorf("sameOrigin(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// The excerpt of a failed response is printed to the user's terminal, so an
// error page carrying escape sequences must not arrive intact either.
func TestAPIErrorExcerptCarriesNoControlCharacters(t *testing.T) {
	e := &APIError{Status: 403, Body: "blocked\x1b]0;PWNED\x07 by datadome"}
	got := e.Error()
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("APIError.Error() = %q, want no control characters", got)
	}
	if !strings.Contains(got, "blocked") || !strings.Contains(got, "by datadome") {
		t.Errorf("APIError.Error() = %q, want the readable text kept", got)
	}
}

// A storefront error page is 40 KB of markup whose first 300 characters are
// preload tags, so quoting an excerpt of it tells the user nothing.
func TestAPIErrorNamesAnHTMLBodyInsteadOfQuotingIt(t *testing.T) {
	e := &APIError{Status: 404, Body: `<!DOCTYPE html><html lang="es-ES"><head><link rel="preload" href="/font.woff2"></head></html>`}
	got := e.Error()
	if strings.Contains(got, "<link") || strings.Contains(got, "DOCTYPE") {
		t.Errorf("APIError.Error() = %q, want the markup kept out", got)
	}
	if !strings.Contains(got, "HTTP 404") || !strings.Contains(got, "HTML error page") {
		t.Errorf("APIError.Error() = %q, want the status and a named body", got)
	}
	plain := &APIError{Status: 412, Body: `{"message":"cart is stale"}`}
	if !strings.Contains(plain.Error(), "cart is stale") {
		t.Errorf("APIError.Error() = %q, want a non-HTML body quoted", plain.Error())
	}
}

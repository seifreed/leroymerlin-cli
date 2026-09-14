package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/browserutils/kooky"
)

func TestCookiesFromBrowserRejectsUnknownBrowserBeforeScan(t *testing.T) {
	if _, err := CookiesFromBrowser("not-a-browser"); err == nil {
		t.Fatal("unknown browser should be rejected")
	}
}

func TestSupportedBrowser(t *testing.T) {
	for _, b := range []string{"chrome", "chromium", "firefox", "safari", "edge", "brave"} {
		if !supportedBrowser(b) {
			t.Errorf("%q should be supported", b)
		}
	}
	for _, b := range []string{"", "Chrome", "opera", "ie"} {
		if supportedBrowser(b) {
			t.Errorf("%q should not be supported", b)
		}
	}
}

func TestPickSessionCookieSkipsAStoreWithoutClearance(t *testing.T) {
	// firefox is installed but was never used on the site: its cookies look
	// individually valid yet carry no DataDome clearance, so chrome must win.
	cookies := []browserCookie{
		{"firefox", "lm-csrf", "stale"},
		{"chrome", "datadome", "DD"},
		{"chrome", "lm-csrf", "tok"},
	}

	got, ok := pickSessionCookie(cookies, "")
	if !ok {
		t.Fatal("want the chrome store selected")
	}
	if !strings.Contains(got, "datadome=DD") || !strings.Contains(got, "lm-csrf=tok") {
		t.Errorf("cookie = %q, want the full chrome store", got)
	}
	if strings.Contains(got, "stale") {
		t.Errorf("cookie = %q, stores must not be merged across browsers", got)
	}
}

// A refreshed clearance supersedes a stale one within the same browser.
func TestPickSessionCookieKeepsTheLastValuePerBrowser(t *testing.T) {
	got, ok := pickSessionCookie([]browserCookie{
		{"chrome", "datadome", "OLD"},
		{"chrome", "datadome", "NEW"},
	}, "")

	if !ok || !strings.Contains(got, "datadome=NEW") || strings.Contains(got, "OLD") {
		t.Fatalf("cookie = %q (ok %v), want datadome=NEW only", got, ok)
	}
}

func TestPickSessionCookieHonoursTheBrowserFilter(t *testing.T) {
	cookies := []browserCookie{
		{"chrome", "datadome", "FROMCHROME"},
		{"firefox", "datadome", "FROMFIREFOX"},
	}

	got, ok := pickSessionCookie(cookies, "firefox")
	if !ok || !strings.Contains(got, "FROMFIREFOX") {
		t.Fatalf("cookie = %q (ok %v), want the firefox store", got, ok)
	}

	if _, ok := pickSessionCookie(cookies, "safari"); ok {
		t.Error("a browser with no cookies should yield nothing")
	}
}

func TestPickSessionCookieReportsNothingUsable(t *testing.T) {
	for _, cookies := range [][]browserCookie{
		nil,
		{{"chrome", "lm-csrf", "tok"}}, // no DataDome clearance
	} {
		if got, ok := pickSessionCookie(cookies, ""); ok {
			t.Errorf("pickSessionCookie(%v) = %q, want no usable cookie", cookies, got)
		}
	}
}

// fakeBrowser satisfies kooky.BrowserInfo so a synthesised cookie can claim a
// browser without a cookie store behind it.
type fakeBrowser struct{ name string }

func (b fakeBrowser) Browser() string        { return b.name }
func (b fakeBrowser) Profile() string        { return "default" }
func (b fakeBrowser) IsDefaultProfile() bool { return true }
func (b fakeBrowser) FilePath() string       { return "" }

type scanned struct {
	cookie *kooky.Cookie
	err    error
}

func browserCookieOf(browser, name, value string) *kooky.Cookie {
	return browserCookieFrom(browser, name, value, ".leroymerlin.es")
}

func browserCookieFrom(browser, name, value, domain string) *kooky.Cookie {
	c := &kooky.Cookie{}
	c.Name = name
	c.Value = value
	c.Domain = domain
	if browser != "" {
		c.Browser = fakeBrowser{name: browser}
	}
	return c
}

// stubTraversal replaces the browser-store scan with a fixed sequence.
func stubTraversal(t *testing.T, seq []scanned) {
	t.Helper()
	orig := traverseCookies
	traverseCookies = func(context.Context, ...kooky.Filter) kooky.CookieSeq {
		return func(yield func(*kooky.Cookie, error) bool) {
			for _, s := range seq {
				if !yield(s.cookie, s.err) {
					return
				}
			}
		}
	}
	t.Cleanup(func() { traverseCookies = orig })
}

func TestCookiesFromBrowserBuildsASessionFromTheStore(t *testing.T) {
	stubTraversal(t, []scanned{
		{cookie: browserCookieOf("chrome", "datadome", "DD")},
		{cookie: browserCookieOf("chrome", "lm-csrf", "tok")},
	})

	got, err := CookiesFromBrowser("")
	if err != nil {
		t.Fatalf("CookiesFromBrowser: %v", err)
	}
	if !strings.Contains(got.Cookie, "datadome=DD") || !strings.Contains(got.Cookie, "lm-csrf=tok") {
		t.Errorf("cookie = %q, want both values", got.Cookie)
	}
}

// A locked profile, a nil entry or a half-empty cookie must be skipped rather
// than abort the scan — one unreadable browser cannot cost the user the one
// that works.
func TestCookiesFromBrowserSkipsUnusableEntries(t *testing.T) {
	stubTraversal(t, []scanned{
		{err: errors.New("locked database")},
		{cookie: nil},
		{cookie: browserCookieOf("chrome", "", "novalue")},
		{cookie: browserCookieOf("chrome", "noname", "")},
		{cookie: browserCookieOf("", "datadome", "FROMUNKNOWN")},
	})

	got, err := CookiesFromBrowser("")
	if err != nil {
		t.Fatalf("CookiesFromBrowser: %v", err)
	}
	if !strings.Contains(got.Cookie, "datadome=FROMUNKNOWN") {
		t.Errorf("cookie = %q, want the usable entry from the unnamed browser", got.Cookie)
	}
}

func TestCookiesFromBrowserReportsAnEmptyScan(t *testing.T) {
	stubTraversal(t, nil)

	_, err := CookiesFromBrowser("")
	if err == nil || !strings.Contains(err.Error(), "your browser") {
		t.Fatalf("err = %v, want the generic not-found advice", err)
	}
}

// Asked for one browser, the error names that browser rather than the generic
// phrasing, so the user knows which store was searched.
func TestCookiesFromBrowserNamesTheBrowserItSearched(t *testing.T) {
	stubTraversal(t, []scanned{
		{cookie: browserCookieOf("firefox", "lm-csrf", "tok")},
	})

	_, err := CookiesFromBrowser("chrome")
	if err == nil || !strings.Contains(err.Error(), "chrome") {
		t.Fatalf("err = %v, want the browser named", err)
	}
}

// kooky filters the store with a substring match on the cookie domain, so a
// site the user visited at leroymerlin.es.example.com (or at fakeleroymerlin.es)
// lands in the scan. Replaying a cookie that site planted to the real storefront
// would hand it a session it never owned, so the domain is re-checked here.
func TestCookiesFromBrowserIgnoresLookAlikeDomains(t *testing.T) {
	for _, domain := range []string{
		"leroymerlin.es.example.com",
		"fakeleroymerlin.es",
		"leroymerlin.example.com",
	} {
		stubTraversal(t, []scanned{
			{cookie: browserCookieFrom("chrome", "datadome", "PLANTED", domain)},
		})
		if got, err := CookiesFromBrowser(""); err == nil {
			t.Errorf("domain %q yielded %q, want no cookie", domain, got.Cookie)
		}
	}
}

// The storefront's own cookies must still be lifted, including the subdomain
// form the site actually sets.
func TestCookiesFromBrowserAcceptsTheStorefrontDomains(t *testing.T) {
	for _, domain := range []string{"leroymerlin.es", ".leroymerlin.es", "www.leroymerlin.es"} {
		stubTraversal(t, []scanned{
			{cookie: browserCookieFrom("chrome", "datadome", "DD", domain)},
		})
		got, err := CookiesFromBrowser("")
		if err != nil || got.Cookie != "datadome=DD" {
			t.Errorf("domain %q = %q, err %v", domain, got.Cookie, err)
		}
	}
}

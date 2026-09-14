package client

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/all" // register Chrome/Firefox/Safari/Edge/Brave finders
)

// browserCookie is one cookie lifted from a browser store, reduced to what the
// session builder needs. Keeping it separate from kooky's type is what lets the
// selection rules below be tested without touching the filesystem.
type browserCookie struct{ browser, name, value string }

// pickSessionCookie groups cookies by the browser they came from and returns the
// first browser's cookie header that carries DataDome clearance. Grouping matters
// because a half-populated store must not shadow a usable one: a browser that is
// installed but was never used on the site contributes cookies that look valid
// individually. Within one browser the last value for a name wins, which is how a
// refreshed clearance supersedes a stale one. want filters to a single browser;
// "" considers every one, in traversal order.
func pickSessionCookie(cookies []browserCookie, want string) (string, bool) {
	stores := map[string]map[string]string{}
	var storeOrder []string
	for _, c := range cookies {
		if want != "" && c.browser != want {
			continue
		}
		if stores[c.browser] == nil {
			stores[c.browser] = map[string]string{}
			storeOrder = append(storeOrder, c.browser)
		}
		stores[c.browser][c.name] = c.value
	}
	for _, bname := range storeOrder {
		if cookie := buildCookieHeader(stores[bname]); CookieLooksUseful(cookie) {
			return cookie, true
		}
	}
	return "", false
}

// traverseCookies is kooky's browser-store scan, indirected so tests can supply a
// deterministic sequence instead of whatever browsers the host happens to have
// installed. Tests swapping it must not run in parallel.
var traverseCookies = kooky.TraverseCookies

// CookiesFromBrowser reads the Leroy Merlin cookies (the DataDome clearance
// among the rest) straight out of a browser's cookie store — the usual way in:
// the user signs in at the site in their everyday browser and the CLI lifts the
// cookie (HttpOnly included, since this reads the decrypted store, not page JS).
// browser filters to one of
// chrome|chromium|firefox|safari|edge|brave; "" reads every installed browser.
func CookiesFromBrowser(browser string) (Session, error) {
	browser = strings.ToLower(strings.TrimSpace(browser))
	if browser != "" && !supportedBrowser(browser) {
		return Session{}, fmt.Errorf("unsupported browser %q (want chrome|chromium|firefox|safari|edge|brave)", browser)
	}

	var cookies []browserCookie
	for c, err := range traverseCookies(context.Background(), kooky.DomainContains(leroyHost)) {
		if err != nil || c == nil || c.Name == "" || c.Value == "" {
			continue
		}
		// kooky's DomainContains is a plain substring match, so a look-alike store
		// entry — leroymerlin.es.example.com, or fakeleroymerlin.es — reaches us
		// here. Replaying an attacker-planted cookie to the real storefront is
		// exactly what must not happen, so re-check the domain properly.
		if !isCookieDomain(c.Domain) {
			continue
		}
		bname := ""
		if c.Browser != nil {
			bname = strings.ToLower(c.Browser.Browser())
		}
		cookies = append(cookies, browserCookie{browser: bname, name: c.Name, value: c.Value})
	}
	if cookie, ok := pickSessionCookie(cookies, browser); ok {
		return Session{Cookie: cookie}, nil
	}

	where := "your browser"
	if browser != "" {
		where = browser
	}
	return Session{}, fmt.Errorf("no leroymerlin.es cookie found in %s — "+
		"open www.leroymerlin.es in that browser once first "+
		"(or pass --from-browser <chrome|firefox|safari|edge|brave>)", where)
}

func supportedBrowser(browser string) bool {
	switch browser {
	case "chrome", "chromium", "firefox", "safari", "edge", "brave":
		return true
	default:
		return false
	}
}

// buildCookieHeader renders a cookie name→value map as a "n1=v1; n2=v2" header,
// name-sorted so the result is deterministic.
func buildCookieHeader(values map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		// A store entry that cannot be sent as a cookie would otherwise poison the
		// whole header: every later request fails to build, and cart and checkout
		// have already cached it to ~/.leroymerlin by then.
		if validCookiePair(name, values[name]) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
}

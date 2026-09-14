package client

import (
	"encoding/json"
	"fmt"
	neturl "net/url"
	"strings"
)

// leroyHost is the registrable domain; the site is served from www.leroymerlin.es.
// It is also the substring every Leroy Merlin cookie's domain carries, which is
// how the site's cookies are picked out of a browser's (large) cookie store.
const leroyHost = "leroymerlin.es"

// isLeroyHost matches the Leroy Merlin domain on the URL's HOST, not a substring
// of the whole URL — so a foreign request like https://evil.com/?x=leroymerlin.es
// (whose host is evil.com) is not mistaken for a Leroy Merlin request and its
// cookies are never read. Accepts the apex and any subdomain.
func isLeroyHost(host string) bool {
	host = strings.ToLower(host)
	return host == leroyHost || strings.HasSuffix(host, "."+leroyHost)
}

// isCookieDomain reports whether a browser cookie's Domain belongs to the
// storefront. Cookie domains carry a leading dot when they cover subdomains, so
// that is trimmed before the host rules are applied.
func isCookieDomain(domain string) bool {
	return isLeroyHost(strings.TrimPrefix(strings.TrimSpace(domain), "."))
}

// CookieLooksUseful reports whether a Cookie header carries the DataDome
// clearance — the part that lets a non-browser client past the bot challenge.
// Used to skip pre-clearance requests when reading a HAR.
func CookieLooksUseful(v string) bool {
	return strings.Contains(strings.ToLower(v), "datadome=")
}

// ParseHAR scans a HAR export (DevTools → Network → "Save all as HAR with
// sensitive data") for the freshest leroymerlin.es request carrying a Cookie
// header with the DataDome clearance, and returns that Cookie. Only request
// Cookie headers are read; request bodies (which can hold credentials) are not
// touched.
func ParseHAR(data []byte) (string, error) {
	var har struct {
		Log struct {
			Entries []struct {
				Request struct {
					URL     string `json:"url"`
					Headers []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
					} `json:"headers"`
				} `json:"request"`
			} `json:"entries"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &har); err != nil {
		return "", fmt.Errorf("parse HAR JSON: %w", err)
	}

	var cookie string
	var sawLeroyReq, sawCookieHeader bool
	for _, e := range har.Log.Entries {
		u, perr := neturl.Parse(e.Request.URL)
		if perr != nil || !isLeroyHost(u.Hostname()) {
			continue
		}
		sawLeroyReq = true
		for _, h := range e.Request.Headers {
			if !strings.EqualFold(h.Name, "cookie") {
				continue
			}
			sawCookieHeader = true
			if CookieLooksUseful(h.Value) {
				cookie = h.Value // last (freshest) cleared request wins
			}
		}
	}

	if cookie == "" {
		return "", harNoCookieErr(sawLeroyReq, sawCookieHeader)
	}
	return cookie, nil
}

// harNoCookieErr explains why no cookie was found, so the fix is obvious.
func harNoCookieErr(sawLeroyReq, sawCookieHeader bool) error {
	switch {
	case !sawLeroyReq:
		return fmt.Errorf("this HAR has no leroymerlin.es requests — export it while browsing the site")
	case !sawCookieHeader:
		return fmt.Errorf("HAR has no Cookie headers (sanitized export) — re-export with: " +
			"DevTools → Network → right-click a request → " +
			"\"Save all as HAR with sensitive data\" (not the ⤓ button, which sanitizes)")
	default:
		return fmt.Errorf("no DataDome cookie found in HAR — export it after the page has loaded normally")
	}
}

// The cookie jar: a single Cookie header string mutated under cookieMu, plus the
// same-origin rule that keeps it off third-party hosts.

package client

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) cookieHeader() string {
	c.cookieMu.RLock()
	defer c.cookieMu.RUnlock()
	return c.Cookie
}

func (c *Client) cookieHeaderFor(rawURL string) string {
	if !c.sameOrigin(rawURL) {
		return ""
	}
	return c.cookieHeader()
}

func (c *Client) sameOrigin(rawURL string) bool {
	target, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	base, err := url.Parse(c.BaseURL)
	return err == nil && strings.EqualFold(target.Scheme, base.Scheme) && strings.EqualFold(target.Host, base.Host)
}

// CurrentSession returns a consistent snapshot of the mutable session state.
func (c *Client) CurrentSession() Session {
	c.cookieMu.RLock()
	defer c.cookieMu.RUnlock()
	return Session{Cookie: c.Cookie, TabID: c.TabID}
}

// setCookieValue records a cookie the client learned from a response body rather
// than from Set-Cookie, where net/http has done no validation for us. The value
// is dropped unless it is a plain cookie-value: a ";" would smuggle extra pairs
// into the header, and a control character would make every later request fail
// to build — persistently, since the jar is cached to disk.
func (c *Client) setCookieValue(name, value string) {
	if name == "" || value == "" || !validCookiePair(name, value) {
		return
	}
	c.updateCookieValue(name, value, false)
}

// validCookiePair reports whether name and value can be sent as one pair of a
// Cookie header. net/http owns those rules and applies them to everything it
// parses itself; borrowing them covers the values that reach the jar without
// passing through it — a response body, or a browser's cookie store. Requiring
// the round trip back to the same single pair is what rejects a value that would
// have smuggled a second one in.
func validCookiePair(name, value string) bool {
	parsed, err := http.ParseCookie(name + "=" + value)
	return err == nil && len(parsed) == 1 && parsed[0].Name == name && parsed[0].Value == value
}

func (c *Client) applyResponseCookie(cookie *http.Cookie) {
	if cookie == nil || cookie.Name == "" {
		return
	}
	expired := cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && cookie.Expires.Before(time.Now()))
	c.updateCookieValue(cookie.Name, cookie.Value, expired)
}

func (c *Client) updateCookieValue(name, value string, remove bool) {
	if name == "" {
		return
	}
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()
	values := map[string]string{}
	var order []string
	for _, part := range strings.Split(c.Cookie, ";") {
		key, old, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = old
	}
	if remove {
		delete(values, name)
		parts := make([]string, 0, len(order))
		for _, key := range order {
			if _, exists := values[key]; exists {
				parts = append(parts, key+"="+values[key])
			}
		}
		c.Cookie = strings.Join(parts, "; ")
		return
	}
	if _, exists := values[name]; !exists {
		order = append(order, name)
	}
	values[name] = value
	parts := make([]string, 0, len(order))
	for _, key := range order {
		parts = append(parts, key+"="+values[key])
	}
	c.Cookie = strings.Join(parts, "; ")
}

// cookieValue extracts one cookie's value from a "n1=v1; n2=v2" header, or "".
func cookieValue(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, name+"="); ok {
			return v
		}
	}
	return ""
}

// refreshSessionHeaders puts the current cookie — and the lm-csrf double-submit
// token the site echoes back from it — on req, dropping both for a foreign host.
// It is the single place session headers are applied: both request builders call
// it, and so does every retry, so a cookie rotated mid-flight is picked up.
func (c *Client) refreshSessionHeaders(req *http.Request) {
	cookie := c.cookieHeaderFor(req.URL.String())
	if cookie == "" {
		req.Header.Del("cookie")
		req.Header.Del("lm-csrf")
		return
	}
	req.Header.Set("cookie", cookie)
	if token := cookieValue(cookie, "lm-csrf"); token != "" {
		req.Header.Set("lm-csrf", token)
	} else {
		req.Header.Del("lm-csrf")
	}
}

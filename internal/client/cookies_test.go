package client

import (
	"net/http"
	"testing"
	"time"
)

// setCookieValue ignores empty names and values so a blank header never wipes a
// live cookie out of the jar.
func TestSetCookieValueIgnoresBlankInput(t *testing.T) {
	c := New()
	c.Cookie = "datadome=DD"

	c.setCookieValue("", "x")
	c.setCookieValue("lm-csrf", "")
	if c.Cookie != "datadome=DD" {
		t.Fatalf("jar = %q, want it untouched", c.Cookie)
	}

	c.setCookieValue("lm-csrf", "tok")
	if c.cookieHeader() != "datadome=DD; lm-csrf=tok" {
		t.Fatalf("jar = %q", c.cookieHeader())
	}
}

// An expired Set-Cookie must remove the value, not store the empty string —
// a logout or a revoked clearance has to actually clear the jar.
func TestApplyResponseCookieRemovesExpiredValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cookie *http.Cookie
		want   string
	}{
		{"nil is ignored", nil, "datadome=DD"},
		{"nameless is ignored", &http.Cookie{Value: "x"}, "datadome=DD"},
		{"fresh value replaces", &http.Cookie{Name: "datadome", Value: "NEW"}, "datadome=NEW"},
		{"negative max-age removes", &http.Cookie{Name: "datadome", MaxAge: -1}, ""},
		{"past expiry removes", &http.Cookie{Name: "datadome", Expires: time.Unix(1, 0)}, ""},
		{"future expiry keeps", &http.Cookie{Name: "datadome", Value: "SOON", Expires: time.Now().Add(time.Hour)}, "datadome=SOON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			c.Cookie = "datadome=DD"
			c.applyResponseCookie(tc.cookie)
			if got := c.cookieHeader(); got != tc.want {
				t.Errorf("jar = %q, want %q", got, tc.want)
			}
		})
	}
}

// updateCookieValue is the single writer behind every cookie mutation; a
// nameless entry must leave the jar untouched rather than write "=value".
func TestUpdateCookieValueIgnoresANamelessEntry(t *testing.T) {
	c := New()
	c.Cookie = "datadome=DD"

	c.updateCookieValue("", "x", false)
	c.updateCookieValue("", "", true)

	if got := c.cookieHeader(); got != "datadome=DD" {
		t.Fatalf("jar = %q, want it untouched", got)
	}
}

// Order.id is taken from a response body, where net/http has validated nothing.
// A value carrying "; " would smuggle extra pairs into the Cookie header, and a
// control character would make every later request fail to build — persistently,
// because cart and checkout cache the jar to disk.
func TestCookieJarRefusesValuesFromAResponseBody(t *testing.T) {
	// The rule is net/http's own, so a space gets through — it cannot start a new
	// pair, and rejecting what the header writer would have accepted only invents
	// a second dialect. Everything that can smuggle or corrupt is refused.
	for _, hostile := range []string{
		"o1; datadome=PLANTED",
		"o1\x1b]0;PWNED\x07",
		`o1"quoted`,
		`o1\backslash`,
	} {
		c := New()
		c.Cookie = "datadome=DD"
		c.setCookieValue("Order.id", hostile)
		if got := c.CurrentSession().Cookie; got != "datadome=DD" {
			t.Errorf("value %q got into the jar: %q", hostile, got)
		}
	}

	// A well-formed order id still lands.
	c := New()
	c.setCookieValue("Order.id", "985cada6-1f2e")
	if got := c.CurrentSession().Cookie; got != "Order.id=985cada6-1f2e" {
		t.Errorf("jar = %q, want the valid order id", got)
	}
}

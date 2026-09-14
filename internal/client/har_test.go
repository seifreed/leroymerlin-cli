package client

import (
	"strings"
	"testing"
)

func harJSON(url, cookie string) string {
	return `{"log":{"entries":[{"request":{"url":"` + url + `","headers":[{"name":"Cookie","value":"` + cookie + `"}]}}]}}`
}

func TestParseHAR(t *testing.T) {
	got, err := ParseHAR([]byte(harJSON("https://www.leroymerlin.es/search?q=x", "datadome=ABC123; lm-csrf=zzz")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "datadome=ABC123; lm-csrf=zzz" {
		t.Errorf("cookie = %q", got)
	}
}

func TestParseHARFreshestWins(t *testing.T) {
	har := `{"log":{"entries":[
	  {"request":{"url":"https://www.leroymerlin.es/a","headers":[{"name":"Cookie","value":"datadome=OLD"}]}},
	  {"request":{"url":"https://www.leroymerlin.es/b","headers":[{"name":"Cookie","value":"datadome=NEW"}]}}
	]}}`
	got, err := ParseHAR([]byte(har))
	if err != nil || got != "datadome=NEW" {
		t.Fatalf("got %q, %v — want freshest (NEW)", got, err)
	}
}

func TestParseHARForeignHostIgnored(t *testing.T) {
	// A foreign host that merely mentions the domain in its query must not match.
	_, err := ParseHAR([]byte(harJSON("https://evil.com/?x=leroymerlin.es", "datadome=LEAK")))
	if err == nil {
		t.Fatal("foreign host should yield no cookie")
	}
}

func TestParseHARNoCookie(t *testing.T) {
	har := `{"log":{"entries":[{"request":{"url":"https://www.leroymerlin.es/x","headers":[{"name":"Accept","value":"*/*"}]}}]}}`
	if _, err := ParseHAR([]byte(har)); err == nil {
		t.Fatal("want error when no Cookie header present")
	}
}

func TestCookieLooksUseful(t *testing.T) {
	if !CookieLooksUseful("foo=1; DataDome=xyz") {
		t.Error("should match DataDome case-insensitively")
	}
	if CookieLooksUseful("lm-csrf=abc") {
		t.Error("should not match without datadome")
	}
}

// The three HAR failures need different fixes, so each gets its own advice:
// wrong site, sanitized export, or exported too early.
func TestHARFailuresExplainTheRightFix(t *testing.T) {
	for _, tc := range []struct {
		name string
		har  string
		want string
	}{
		{"no storefront requests", `{"log":{"entries":[]}}`, "no leroymerlin.es requests"},
		{
			"sanitized export",
			`{"log":{"entries":[{"request":{"url":"https://www.leroymerlin.es/x","headers":[]}}]}}`,
			"sanitized export",
		},
		{
			"cookie without clearance",
			harJSON("https://www.leroymerlin.es/x", "lm-csrf=tok"),
			"no DataDome cookie",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseHAR([]byte(tc.har))
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

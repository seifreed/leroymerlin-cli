package client

import "testing"

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
		"accept":             "application/json, text/plain, */*",
		"accept-language":    "es-ES,es;q=0.9,en;q=0.8",
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"macOS"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"x-requested-with":   "XMLHttpRequest",
		"content-type":       "application/json",
		"lm-csrf":            "TOK", // double-submit token lifted from the cookie
	}
	for k, v := range want {
		if got := req.Header.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	if req.Header.Get("sec-ch-ua") == "" {
		t.Error("sec-ch-ua must be set")
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

package client

import (
	"net/http"
	"testing"
)

func TestCheckAuthServedPage(t *testing.T) {
	// A served search page carries catalog markers even though DataDome's JS
	// (captcha-delivery) is also present — must still read as "ok".
	body := `<html><script src="//geo.captcha-delivery.com/x.js"></script>` +
		`<li class="product-thumbnail product-thumbnail-item"></li>` +
		`<script class="dataTms">[{"name":"cdl_products_list","value":[]}]</script></html>`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	ok, err := c.CheckAuth()
	if err != nil || !ok {
		t.Fatalf("served page: ok=%v err=%v, want true/nil", ok, err)
	}
}

func TestCheckAuthBlockPage(t *testing.T) {
	// A block page has DataDome content but no real catalog markers.
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Please verify <div class="dd-captcha"></div></body></html>`))
	})
	ok, err := c.CheckAuth()
	if err != nil || ok {
		t.Fatalf("block page: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestCheckAuthHTTPError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
	})
	if _, err := c.CheckAuth(); err == nil {
		t.Fatal("403 should surface an error")
	}
}

func TestBuildCookieHeader(t *testing.T) {
	got := buildCookieHeader(map[string]string{"datadome": "X", "abc": "1"})
	if got != "abc=1; datadome=X" { // sorted, deterministic
		t.Errorf("got %q", got)
	}
}

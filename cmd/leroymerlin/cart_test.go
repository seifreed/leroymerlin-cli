package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestResolveMax(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
	// flag wins
	if got := resolveMax(50); got != 50 {
		t.Errorf("flag: got %v", got)
	}
	// env when flag unset (negative sentinel)
	t.Setenv("LEROYMERLIN_MAX_EUR", "25")
	if got := resolveMax(-1); got != 25 {
		t.Errorf("env: got %v", got)
	}
	// no cap
	t.Setenv("LEROYMERLIN_MAX_EUR", "")
	if got := resolveMax(-1); got != 0 {
		t.Errorf("none: got %v", got)
	}
}

// cartStub serves a product page (with offer_id + price) and the cart endpoints.
func cartStub(t *testing.T, price string) (added *bool) {
	t.Helper()
	wasAdded := false
	added = &wasAdded
	prodHTML := `<script type="application/ld+json">{"@type":"Product","name":"Taladro","sku":"83085630","offers":{"price":"` + price + `","priceCurrency":"EUR"}}</script>` +
		`<script>{"offer_id":"deadbeefdeadbeef"}</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/productos/"):
			_, _ = w.Write([]byte(prodHTML))
		case r.URL.Path == "/cart/services/addToCart":
			wasAdded = true
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "cart-data"):
			_, _ = w.Write([]byte(`{"quantity":1,"order":"o1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_BASE_URL", srv.URL)
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	// seed a cookie so LoadAuth passes
	if err := os.WriteFile(dir+"/session.json", []byte(`{"cookie":"datadome=DD; lm-csrf=T"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return added
}

func TestCartAddUnderMax(t *testing.T) {
	added := cartStub(t, "13.99")
	out := captureStdout(t, func() {
		if code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "2", "--max", "50"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !*added {
		t.Error("expected the item to be added")
	}
	if !strings.Contains(out, "added 2") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCartAddOverMaxRefused(t *testing.T) {
	added := cartStub(t, "13.99")
	// 2 × 13.99 = 27.98 > 20 → refuse, no write
	code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "2", "--max", "20"})
	if code != 1 {
		t.Errorf("over-max exit = %d, want 1", code)
	}
	if *added {
		t.Error("item must NOT be added when over the cap")
	}
}

func TestCartAddNeedsCookie(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir()) // no session.json
	if code := run([]string{"cart", "add", "/productos/x-1.html"}); code != 1 {
		t.Errorf("no-cookie exit = %d, want 1", code)
	}
}

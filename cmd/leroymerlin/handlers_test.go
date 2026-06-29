package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what was
// written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

const cardHTML = `<script type="application/json" class="dataTms">
[{"name":"cdl_products_list","value":[{"brand":"DEXTER","identifier":"42","name":"Taladro","url":"/productos/x-42.html","rating":4.5,"product_is_sponsored":false,"total_offer_count":1,"offer":{"unitprice_ati":29.99,"unitprice_tf":24.0,"seller_name":"Leroy Merlin","seller_type":"1P","add_to_cart_availability":true}}]}]
</script>`

const productHTML = `<script type="application/ld+json">{"@type":"Product","name":"Taladro","sku":"42","brand":"DEXTER","offers":{"price":"29.99","priceCurrency":"EUR","availability":"http://schema.org/InStock"}}</script>`

func withStubServer(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LEROYMERLIN_BASE_URL", srv.URL)
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir()) // isolate from a real ~/.leroymerlin
}

func TestRunUnknownCommand(t *testing.T) {
	if code := run([]string{"frobnicate"}); code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Errorf("no args exit = %d, want 2", code)
	}
}

func TestSearchJSON(t *testing.T) {
	withStubServer(t, cardHTML)
	out := captureStdout(t, func() {
		if code := run([]string{"search", "--json", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"identifier": "42"`) || !strings.Contains(out, `"unitprice_ati": 29.99`) {
		t.Errorf("json missing expected fields:\n%s", out)
	}
}

func TestSearchHuman(t *testing.T) {
	withStubServer(t, cardHTML)
	out := captureStdout(t, func() { run([]string{"search", "taladro"}) })
	if !strings.Contains(out, "[42] Taladro — 29.99€") {
		t.Errorf("human line wrong:\n%s", out)
	}
}

func TestProductHuman(t *testing.T) {
	withStubServer(t, productHTML)
	out := captureStdout(t, func() {
		if code := run([]string{"product", "/productos/x-42.html"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "ref:          42") || !strings.Contains(out, "29.99 EUR") {
		t.Errorf("detail wrong:\n%s", out)
	}
}

func TestProductRejectsNonProductArg(t *testing.T) {
	if code := run([]string{"product", "taladro"}); code != 1 {
		t.Errorf("non-product arg exit = %d, want 1", code)
	}
}

func TestSetCookiePersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	if code := run([]string{"set-cookie", "datadome=abc; lm-csrf=def"}); code != 0 {
		t.Fatalf("set-cookie exit = %d", code)
	}
	if _, err := os.Stat(dir + "/session.json"); err != nil {
		t.Errorf("session.json not written: %v", err)
	}
}

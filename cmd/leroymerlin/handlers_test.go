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

// withRoutedServer serves productHTML for /productos/ paths and cardHTML for
// everything else (search), so total can exercise both resolution paths.
func withRoutedServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/productos/") {
			_, _ = w.Write([]byte(productHTML))
			return
		}
		_, _ = w.Write([]byte(cardHTML))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LEROYMERLIN_BASE_URL", srv.URL)
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
}

func TestBatchHuman(t *testing.T) {
	withStubServer(t, cardHTML)
	out := captureStdout(t, func() {
		if code := run([]string{"batch", "taladro", "broca"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "• taladro") || !strings.Contains(out, "• broca") {
		t.Errorf("batch output missing terms:\n%s", out)
	}
	if !strings.Contains(out, "[42] Taladro — 29.99€") {
		t.Errorf("batch missing resolved product:\n%s", out)
	}
}

func TestTotalTermAndURL(t *testing.T) {
	withRoutedServer(t)
	out := captureStdout(t, func() {
		// one search-term line (qty 2 via file) + one product url
		if code := run([]string{"total", "--json", "taladro", "/productos/x-42.html"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	// both lines price at 29.99 × 1 = 29.99 each → total 59.98
	if !strings.Contains(out, `"total": "59.98"`) {
		t.Errorf("total wrong:\n%s", out)
	}
	if !strings.Contains(out, `"complete": true`) {
		t.Errorf("expected complete:\n%s", out)
	}
}

func TestTotalQtyFromFile(t *testing.T) {
	withStubServer(t, cardHTML)
	dir := t.TempDir()
	f := dir + "/basket.txt"
	if err := os.WriteFile(f, []byte("taladro 3\n# comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if code := run([]string{"total", "-f", f, "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"total": "89.97"`) { // 29.99 × 3
		t.Errorf("qty from file wrong:\n%s", out)
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

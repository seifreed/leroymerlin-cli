package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

const detailCartJSON = `{
  "orderId":"o1","offersQuantity":2,"disabledCheckout":false,
  "cannotBeValidatedReasons":["ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER"],
  "orderResume":{"totalAmount":31.88,"offersAmount":27.98,"deliveryAmount":3.9},
  "cartVendors":[{"cartVendorItems":[
    {"id":"line-1","quantity":2,"discountPrice":27.98,"offer":{"refLM":"83085630","label":"Taladro"}}
  ]}]
}`

// cartMutStub serves the detailed cart and records PUT/DELETE mutations.
func cartMutStub(t *testing.T) (puts, deletes *[]string) {
	t.Helper()
	var mu sync.Mutex
	var p, d []string
	puts, deletes = &p, &d
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/checkout/backend/cart":
			_, _ = w.Write([]byte(detailCartJSON))
		case strings.Contains(r.URL.Path, "/update-offer-line-quantity/"):
			p = append(p, r.URL.Path)
			w.WriteHeader(200)
		case strings.Contains(r.URL.Path, "/delete-offer-line/"):
			d = append(d, r.URL.Path)
			w.WriteHeader(200)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_BASE_URL", srv.URL)
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	if err := os.WriteFile(dir+"/session.json", []byte(`{"cookie":"datadome=DD; lm-csrf=T"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return puts, deletes
}

func TestCartSetUpdatesQuantity(t *testing.T) {
	puts, _ := cartMutStub(t)
	if code := run([]string{"cart", "set", "83085630", "5"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(*puts) != 1 || !strings.HasSuffix((*puts)[0], "/line-1") {
		t.Errorf("expected one PUT to line-1, got %v", *puts)
	}
}

func TestCartSetZeroDeletes(t *testing.T) {
	_, deletes := cartMutStub(t)
	if code := run([]string{"cart", "set", "83085630", "0"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(*deletes) != 1 {
		t.Errorf("qty 0 should DELETE, got %v", *deletes)
	}
}

func TestCartSetUnknownRef(t *testing.T) {
	cartMutStub(t)
	if code := run([]string{"cart", "set", "99999", "1"}); code != 1 {
		t.Errorf("unknown ref exit = %d, want 1", code)
	}
}

func TestCartClear(t *testing.T) {
	_, deletes := cartMutStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"cart", "clear"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if len(*deletes) != 1 {
		t.Errorf("clear should DELETE each line, got %v", *deletes)
	}
	if !strings.Contains(out, "cleared 1") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCheckoutStatus(t *testing.T) {
	cartMutStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"total": 31.88`) || !strings.Contains(out, `"ready": false`) {
		t.Errorf("checkout json wrong:\n%s", out)
	}
}

func TestCartGetDetailed(t *testing.T) {
	cartMutStub(t)
	out := captureStdout(t, func() { run([]string{"cart", "get"}) })
	if !strings.Contains(out, "[83085630] Taladro — 2 ×") || !strings.Contains(out, "total: 31.88€") {
		t.Errorf("cart get detailed wrong:\n%s", out)
	}
}

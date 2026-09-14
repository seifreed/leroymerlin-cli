package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// sampleCard is one product card's dataTms script as the storefront emits it.
func sampleCard(ref, name string, price float64, seller string) string {
	return `<script type="application/json" class="dataTms">
[{"name":"cdl_products_list","value":[{"brand":"DEXTER","identifier":"` + ref + `","name":"` + name + `","url":"/productos/x-` + ref + `.html","rating":4.7,"product_is_sponsored":false,"total_offer_count":1,"offer":{"unitprice_ati":` + ftoa(price) + `,"unitprice_tf":1.0,"seller_name":"` + seller + `","seller_type":"1P","add_to_cart_availability":true}}]}]
</script>`
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func TestParseProducts(t *testing.T) {
	html := `<html><body>` +
		`<script type="application/json" class="dataTms">[{"name":"page_meta","value":{"x":1}}]</script>` +
		sampleCard("111", "Taladro A", 11.79, "Leroy Merlin") +
		sampleCard("222", "Destornillador B", 1.64, "Otro") +
		// duplicate ref — must be deduped
		sampleCard("111", "Taladro A dup", 11.79, "Leroy Merlin") +
		// malformed block — must be skipped, not fatal
		`<script type="application/json" class="dataTms">{ not json </script>` +
		`</body></html>`

	got := parseProducts(html)
	if len(got) != 2 {
		t.Fatalf("want 2 products, got %d: %+v", len(got), got)
	}
	if got[0].Identifier != "111" || got[1].Identifier != "222" {
		t.Fatalf("order/identity wrong: %+v", got)
	}
	if got[0].Name != "Taladro A" {
		t.Errorf("first card name = %q, want the original (not the dup)", got[0].Name)
	}
	if got[0].Offer.UnitPriceATI != 11.79 {
		t.Errorf("price = %v, want 11.79", got[0].Offer.UnitPriceATI)
	}
	if got[0].URL != "/productos/x-111.html" {
		t.Errorf("url = %q", got[0].URL)
	}
}

func TestParseProductsEmpty(t *testing.T) {
	if got := parseProducts("<html>no products here</html>"); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}

func TestSearchPaginates(t *testing.T) {
	// Each page p serves two unique products; page 1 has no p param.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("p")
		if p == "" {
			p = "1"
		}
		base := p + "0" // page 1 → "10","11"; page 2 → "20","21"; …
		_, _ = w.Write([]byte(sampleCard(base+"a", "A", 9.99, "LM") + sampleCard(base+"b", "B", 9.99, "LM")))
	})
	got, err := c.Search("x", 5) // wants 5 → needs 3 pages (2+2+2=6 → trim to 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 across pages, got %d: %+v", len(got), got)
	}
	// first two from page 1, then page 2, page 3 — all unique
	ids := map[string]bool{}
	for _, p := range got {
		if ids[p.Identifier] {
			t.Errorf("duplicate across pages: %s", p.Identifier)
		}
		ids[p.Identifier] = true
	}
	if got[0].Identifier != "10a" || got[2].Identifier != "20a" {
		t.Errorf("page order wrong: %s … %s", got[0].Identifier, got[2].Identifier)
	}
}

func TestSearchPropagatesLaterPageError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("p") == "2" {
			http.Error(w, "page unavailable", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(sampleCard("1", "A", 9.99, "LM")))
	})
	got, err := c.Search("x", 2)
	if err == nil {
		t.Fatalf("later page error was swallowed; got %d results", len(got))
	}
	if len(got) != 1 {
		t.Fatalf("want results collected before the error, got %d", len(got))
	}
}

func TestSearchStopsWhenPageRepeats(t *testing.T) {
	// Every page returns the SAME products (param ignored, like a landing page):
	// dedup makes page 2 add nothing → stop, no runaway to maxPages.
	hits := 0
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleCard("1", "A", 9.99, "LM") + sampleCard("2", "B", 9.99, "LM")))
	})
	got, err := c.Search("x", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 unique, got %d", len(got))
	}
	if hits > 2 { // page 1 + one more that adds nothing, then stop
		t.Errorf("fetched %d pages, expected to stop after the repeat", hits)
	}
}

func TestSearchLimitZeroOnePage(t *testing.T) {
	hits := 0
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleCard("1", "A", 9.99, "LM")))
	})
	if _, err := c.Search("x", 0); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("limit 0 must fetch exactly one page, got %d", hits)
	}
}

func TestSearchLimitAndHTTP(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			t.Errorf("missing q param: %s", r.URL)
		}
		_, _ = w.Write([]byte(sampleCard("1", "A", 11.79, "LM") + sampleCard("2", "B", 1.64, "LM")))
	})
	got, err := c.Search("taladro", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("limit not applied: got %d", len(got))
	}
}

// Pagination stops at the page cap rather than walking a huge catalog forever;
// the diagnostic tells the caller the result set was truncated by the cap and
// not by the catalog running out.
func TestSearchStopsAtThePageCapAndSaysSo(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		// Every page carries a distinct product, so the walk never runs dry.
		id := r.URL.Query().Get("p")
		if id == "" {
			id = "1"
		}
		_, _ = w.Write([]byte(`<script type="application/json" class="dataTms">` +
			`[{"name":"cdl_products_list","value":[{"identifier":"p` + id + `","name":"P"}]}]` +
			`</script>`))
	}))
	defer srv.Close()

	var diagnostics []string
	c := New()
	c.BaseURL = srv.URL
	c.Logf = func(format string, args ...any) {
		diagnostics = append(diagnostics, fmt.Sprintf(format, args...))
	}

	got, err := c.Search("taladro", 10000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if pages > maxPages {
		t.Errorf("fetched %d pages, want no more than the %d cap", pages, maxPages)
	}
	if len(got) != maxPages {
		t.Errorf("results = %d, want one per capped page (%d)", len(got), maxPages)
	}
	var warned bool
	for _, d := range diagnostics {
		if strings.Contains(d, "stopped at") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no page-cap diagnostic; got %v", diagnostics)
	}
}

// A single malformed cdl_products_list bucket must not discard the page: cards
// are independent blobs and the storefront ships a broken one now and then.
func TestParseProductsSkipsAMalformedBucket(t *testing.T) {
	html := `<script type="application/json" class="dataTms">` +
		`[{"name":"cdl_products_list","value":"not an array"},` +
		`{"name":"cdl_products_list","value":[{"identifier":"7","name":"Taladro"}]}]` +
		`</script>`

	got := parseProducts(html)

	if len(got) != 1 || got[0].Identifier != "7" {
		t.Fatalf("products = %+v, want only the well-formed one", got)
	}
}

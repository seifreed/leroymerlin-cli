package client

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("p")
		if p == "" {
			p = "1"
		}
		base := p + "0" // page 1 → "10","11"; page 2 → "20","21"; …
		_, _ = w.Write([]byte(sampleCard(base+"a", "A", 9.99, "LM") + sampleCard(base+"b", "B", 9.99, "LM")))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
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

func TestSearchStopsWhenPageRepeats(t *testing.T) {
	// Every page returns the SAME products (param ignored, like a landing page):
	// dedup makes page 2 add nothing → stop, no runaway to maxPages.
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleCard("1", "A", 9.99, "LM") + sampleCard("2", "B", 9.99, "LM")))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleCard("1", "A", 9.99, "LM")))
	}))
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL
	if _, err := c.Search("x", 0); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("limit 0 must fetch exactly one page, got %d", hits)
	}
}

func TestSearchLimitAndHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			t.Errorf("missing q param: %s", r.URL)
		}
		_, _ = w.Write([]byte(sampleCard("1", "A", 11.79, "LM") + sampleCard("2", "B", 1.64, "LM")))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	got, err := c.Search("taladro", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("limit not applied: got %d", len(got))
	}
}

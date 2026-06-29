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

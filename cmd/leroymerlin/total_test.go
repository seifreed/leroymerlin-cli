package main

import (
	"net/http"
	"strings"
	"testing"
)

// priceRef takes one of two routes depending on the input: a /productos/ URL is
// fetched directly, anything else is searched and the cheapest hit priced.
func TestPriceRefPricesAProductURLDirectly(t *testing.T) {
	withStubServer(t, productHTML)
	cl := newClient()

	name, price, cents, err := priceRef(cl, "/productos/taladro-42.html")
	if err != nil {
		t.Fatalf("priceRef: %v", err)
	}
	if name != "Taladro" || price != "29.99" || cents != 2999 {
		t.Errorf("got %q/%q/%d, want Taladro/29.99/2999", name, price, cents)
	}
}

func TestPriceRefSearchesABareTerm(t *testing.T) {
	withStubServer(t, cardHTML)
	cl := newClient()

	name, _, cents, err := priceRef(cl, "taladro")
	if err != nil {
		t.Fatalf("priceRef: %v", err)
	}
	if name != "Taladro" || cents != 2999 {
		t.Errorf("got %q/%d, want Taladro/2999", name, cents)
	}
}

// A product URL that 404s is the common case of a stale link, and deserves
// advice rather than a raw HTTP error.
func TestPriceRefExplainsAMissingProduct(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	cl := newClient()

	_, _, _, err := priceRef(cl, "/productos/ausente-1.html")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want the not-found advice", err)
	}
}

// A page with no Product JSON-LD reaches the same advice by a different route
// (ErrNoProduct rather than a 404).
func TestPriceRefExplainsAPageWithoutProductData(t *testing.T) {
	withStubServer(t, `<html><body>sin datos</body></html>`)
	cl := newClient()

	_, _, _, err := priceRef(cl, "/productos/vacio-1.html")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want the not-found advice", err)
	}
}

func TestPriceRefReportsATermWithNoResults(t *testing.T) {
	withStubServer(t, `<html><body>sin resultados</body></html>`)
	cl := newClient()

	_, _, _, err := priceRef(cl, "termino-inexistente")
	if err == nil || !strings.Contains(err.Error(), "no results") {
		t.Fatalf("err = %v, want the no-results error", err)
	}
}

// An unpriced product still returns its name, so the caller can report which
// line failed rather than just "error".
func TestPriceRefKeepsTheNameWhenThePriceIsUnusable(t *testing.T) {
	withStubServer(t, `<script type="application/ld+json">`+
		`{"@type":"Product","name":"Taladro","sku":"42","offers":{"price":"","priceCurrency":"EUR"}}</script>`)
	cl := newClient()

	name, _, _, err := priceRef(cl, "/productos/taladro-42.html")
	if err == nil {
		t.Fatal("want an error for an unparseable price")
	}
	if name != "Taladro" {
		t.Errorf("name = %q, want it preserved alongside the error", name)
	}
}

// A basket line that cannot be priced is listed with its error, excluded from
// the total, and reflected in the exit status — a silent partial total would
// understate what the basket costs.
func TestTotalReportsUnpriceableLinesAndFails(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/productos/ausente") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(cardHTML))
	})

	out := captureStdout(t, func() {
		if code := run([]string{"total", "taladro", "/productos/ausente-1.html"}); code == 0 {
			t.Error("want a non-zero exit when a line could not be priced")
		}
	})

	if !strings.Contains(out, "ERROR:") {
		t.Errorf("output missing the per-line error:\n%s", out)
	}
	if !strings.Contains(out, "total:") {
		t.Errorf("output missing the partial total:\n%s", out)
	}
}

// The same partial result in --json form must carry complete:false so a script
// can tell a full basket from a partial one.
func TestTotalJSONMarksAPartialBasket(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/productos/ausente") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(cardHTML))
	})

	out := captureStdout(t, func() {
		_ = run([]string{"total", "--json", "taladro", "/productos/ausente-1.html"})
	})
	if !strings.Contains(out, `"complete": false`) {
		t.Errorf("json missing complete:false:\n%s", out)
	}
}

// A 404 on a product URL is stale-link advice; any other status is a real
// failure and must surface as itself rather than be flattened into "not found".
func TestPriceRefSurfacesNonNotFoundStatuses(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	cl := newClient()

	_, _, _, err := priceRef(cl, "/productos/taladro-1.html")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want the real status, not the stale-link advice", err)
	}
}

func TestPriceRefSurfacesASearchFailure(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	cl := newClient()

	if _, _, _, err := priceRef(cl, "taladro"); err == nil {
		t.Fatal("want the search failure surfaced")
	}
}

// A price the cent conversion cannot represent still returns the product name,
// so the basket can say which line failed.
func TestPriceRefKeepsTheNameWhenASearchPriceIsUnrepresentable(t *testing.T) {
	huge := `<script type="application/json" class="dataTms">
[{"name":"cdl_products_list","value":[{"brand":"DEXTER","identifier":"1","name":"Taladro",
"url":"/productos/x-1.html","offer":{"unitprice_ati":1e300,"add_to_cart_availability":true}}]}]
</script>`
	withStubServer(t, huge)
	cl := newClient()

	name, _, _, err := priceRef(cl, "taladro")
	if err == nil {
		t.Fatal("want an error for an unrepresentable price")
	}
	if name != "Taladro" {
		t.Errorf("name = %q, want it preserved alongside the error", name)
	}
}

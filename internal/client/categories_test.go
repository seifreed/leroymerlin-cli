package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCategories(t *testing.T) {
	html := `<nav>
	  <a href="/productos/iluminacion/"><span>Iluminación</span></a>
	  <a href="/productos/banos/">Baños</a>
	  <a href="/productos/iluminacion/">dup label, same path</a>
	  <a href="/productos/herramientas/electricas/">too deep — ignored</a>
	  <a href="/search?q=x">not a category</a>
	</nav>`
	cats := parseCategories(html)
	if len(cats) != 2 {
		t.Fatalf("want 2 categories, got %d: %+v", len(cats), cats)
	}
	if cats[0].Name != "Iluminación" || cats[0].Path != "/productos/iluminacion/" {
		t.Errorf("first = %+v", cats[0])
	}
	if cats[1].Name != "Baños" {
		t.Errorf("second name = %q", cats[1].Name)
	}
}

func TestNormalizeCategoryPath(t *testing.T) {
	for _, in := range []string{"iluminacion", "/iluminacion/", "productos/iluminacion", "/productos/iluminacion/"} {
		if got := normalizeCategoryPath(in); got != "/productos/iluminacion/" {
			t.Errorf("%q → %q", in, got)
		}
	}
}

func TestCategoryProductsReusesParser(t *testing.T) {
	card := `<script type="application/json" class="dataTms">[{"name":"cdl_products_list","value":[{"identifier":"7","name":"Lámpara","url":"/productos/x-7.html","offer":{"unitprice_ati":3.99,"add_to_cart_availability":true}}]}]</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/productos/iluminacion/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(card))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	prods, err := c.CategoryProducts("iluminacion", 0)
	if err != nil || len(prods) != 1 || prods[0].Identifier != "7" {
		t.Fatalf("got %+v, %v", prods, err)
	}
}

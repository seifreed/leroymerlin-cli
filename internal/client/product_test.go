package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const productLD = `<html><head>
<script type="application/ld+json">{"@type":"Organization","name":"Leroy Merlin"}</script>
<script type="application/ld+json">[
  {"@type":"Website","name":"x"},
  {"@type":"Product","name":"Taladro PRACTYL","sku":"83085630","gtin":"3276007383270",
   "description":"Taladro con cable.","brand":{"@type":"Brand","name":"PRACTYL"},
   "offers":{"@type":"Offer","price":"13.99","priceCurrency":"EUR","availability":"http://schema.org/InStock","url":"/productos/x-83085630.html","itemCondition":"NewCondition"},
   "aggregateRating":{"ratingValue":"4.56","reviewCount":"234"},
   "image":["https://media.adeo.com/a.jpg","https://media.adeo.com/b.jpg"]}
]</script>
</head><body>...</body></html>`

func TestParseProductDetail(t *testing.T) {
	d, err := parseProductDetail(productLD)
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Taladro PRACTYL" || d.SKU != "83085630" {
		t.Errorf("name/sku wrong: %+v", d)
	}
	if string(d.Brand) != "PRACTYL" {
		t.Errorf("brand = %q (object form not reduced)", d.Brand)
	}
	if d.Price() != "13.99" || d.Currency() != "EUR" {
		t.Errorf("price/currency = %q/%q", d.Price(), d.Currency())
	}
	if d.Availability() != "InStock" {
		t.Errorf("availability = %q, want InStock", d.Availability())
	}
	if string(d.Image) != "https://media.adeo.com/a.jpg" {
		t.Errorf("image (array) not reduced to first: %q", d.Image)
	}
	if d.Rating == nil || d.Rating.Count != "234" {
		t.Errorf("rating wrong: %+v", d.Rating)
	}
}

func TestParseProductDetailFlexForms(t *testing.T) {
	// brand as a bare string; offers as a list; image as a bare string.
	html := `<script type="application/ld+json">
{"@type":"Product","name":"X","sku":"1","brand":"Bosch",
 "offers":[{"price":"9.99","priceCurrency":"EUR","availability":"InStock"}],
 "image":"https://x/y.jpg"}
</script>`
	d, err := parseProductDetail(html)
	if err != nil {
		t.Fatal(err)
	}
	if string(d.Brand) != "Bosch" {
		t.Errorf("string brand = %q", d.Brand)
	}
	if d.Price() != "9.99" {
		t.Errorf("list offer price = %q", d.Price())
	}
	if d.Availability() != "InStock" {
		t.Errorf("availability = %q", d.Availability())
	}
}

func TestParseProductDetailNoProduct(t *testing.T) {
	_, err := parseProductDetail(`<script type="application/ld+json">{"@type":"Website"}</script>`)
	if err != ErrNoProduct {
		t.Fatalf("want ErrNoProduct, got %v", err)
	}
}

func TestProductHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(productLD))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	d, err := c.Product("/productos/x-83085630.html")
	if err != nil {
		t.Fatal(err)
	}
	if d.URL != srv.URL+"/productos/x-83085630.html" {
		t.Errorf("URL not set to fetched page: %q", d.URL)
	}
}

package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCartData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cartDataPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"quantity":3,"order":"abc-123","redirectionUrl":"/checkout/cart"}`))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	sum, err := c.CartData()
	if err != nil || sum.Quantity != 3 || sum.Order != "abc-123" {
		t.Fatalf("got %+v, %v", sum, err)
	}
}

func TestAddToCartRequest(t *testing.T) {
	var gotBody []addToCartItem
	var gotCSRF, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case cartAddPath:
			if r.Method != "POST" {
				t.Errorf("method = %s", r.Method)
			}
			gotCSRF = r.Header.Get("lm-csrf")
			gotCT = r.Header.Get("content-type")
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &gotBody)
			_, _ = w.Write([]byte(`{}`))
		case cartDataPath:
			_, _ = w.Write([]byte(`{"quantity":2,"order":"o1"}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	c.Cookie = "datadome=DD; lm-csrf=TOK123"
	sum, err := c.AddToCart("83085630", "deadbeefdeadbeef", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Quantity != 2 {
		t.Errorf("summary after add = %+v", sum)
	}
	if len(gotBody) != 1 {
		t.Fatalf("body = %+v", gotBody)
	}
	it := gotBody[0]
	if it.Reflm != "83085630" || it.OfferID != "deadbeefdeadbeef" || it.Quantity != 2 {
		t.Errorf("item fields = %+v", it)
	}
	if it.ContextCode != defaultContextCode || it.ATCButtonLocation != "main offer" {
		t.Errorf("constants = %+v", it)
	}
	if gotCSRF != "TOK123" {
		t.Errorf("lm-csrf header = %q, want TOK123 (from cookie)", gotCSRF)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
}

func TestCookieValue(t *testing.T) {
	c := "datadome=ABC; lm-csrf=XYZ; foo=1"
	if cookieValue(c, "lm-csrf") != "XYZ" {
		t.Errorf("lm-csrf = %q", cookieValue(c, "lm-csrf"))
	}
	if cookieValue(c, "missing") != "" {
		t.Error("missing cookie should be empty")
	}
}

func TestProductOffer(t *testing.T) {
	html := `<html>
	<script type="application/ld+json">{"@type":"Product","name":"Taladro","sku":"83085630","offers":{"price":"13.99","priceCurrency":"EUR"}}</script>
	<script>{"offer_id":"15cd48c7225762da","contextId":"058","seller_id":"002"}</script>
	</html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	reflm, offerID, contextCode, detail, err := c.ProductOffer("/productos/taladro-83085630.html")
	if err != nil {
		t.Fatal(err)
	}
	if reflm != "83085630" {
		t.Errorf("reflm = %q (should come from URL)", reflm)
	}
	if offerID != "15cd48c7225762da" {
		t.Errorf("offerId = %q", offerID)
	}
	if contextCode != "058" {
		t.Errorf("contextCode = %q, want 058 (from offer JSON)", contextCode)
	}
	if detail == nil || detail.Price() != "13.99" {
		t.Errorf("detail = %+v", detail)
	}
}

func TestProductOfferMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>no offer here</html>`))
	}))
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL
	if _, _, _, _, err := c.ProductOffer("/productos/x-1.html"); err == nil {
		t.Fatal("want error when offerId absent")
	}
}

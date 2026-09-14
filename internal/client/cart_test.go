package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCartData(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cartDataPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"quantity":3,"order":"abc-123","redirectionUrl":"/checkout/cart"}`))
	})
	sum, err := c.CartData()
	if err != nil || sum.Quantity != 3 || sum.Order != "abc-123" {
		t.Fatalf("got %+v, %v", sum, err)
	}
}

func TestAddToCartRequest(t *testing.T) {
	var gotBody []addToCartItem
	var gotCSRF, gotCT string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
	})
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

func TestAddToCartUsesFreshDetailCount(t *testing.T) {
	detailCalls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case cartAddPath:
			_, _ = w.Write([]byte(`{}`))
		case cartDataPath:
			// The header widget can lag immediately after a write.
			_, _ = w.Write([]byte(`{"quantity":0,"order":"o1"}`))
		case cartDetailPath:
			detailCalls++
			quantity := 0
			if detailCalls > 1 {
				quantity = 1
			}
			_, _ = fmt.Fprintf(w, `{"orderId":"o1","offersQuantity":%d,"orderResume":{"totalAmount":13.99,"offersAmount":13.99}}`, quantity)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	c.Cookie = "datadome=DD; lm-csrf=TOK123"
	sum, err := c.AddToCart("83085630", "deadbeefdeadbeef", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Quantity != 1 {
		t.Fatalf("summary quantity = %d, want fresh detail count 1", sum.Quantity)
	}
	if detailCalls != 2 {
		t.Fatalf("detail calls = %d, want one retry", detailCalls)
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

func TestSetCookieValue(t *testing.T) {
	c := New()
	c.Cookie = "datadome=DD; lm-csrf=old"
	c.setCookieValue("Order.id", "order-1")
	c.setCookieValue("lm-csrf", "new")
	if got := cookieValue(c.Cookie, "Order.id"); got != "order-1" {
		t.Errorf("Order.id = %q", got)
	}
	if got := cookieValue(c.Cookie, "lm-csrf"); got != "new" {
		t.Errorf("lm-csrf = %q", got)
	}
}

func TestResponseCookiesPersistInClient(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Set-Cookie", "Order.id=order-123; Path=/")
		_, _ = w.Write([]byte("ok"))
	})
	if _, err := c.GetHTML("/"); err != nil {
		t.Fatal(err)
	}
	if got := cookieValue(c.Cookie, "Order.id"); got != "order-123" {
		t.Errorf("Order.id = %q, want persisted response cookie", got)
	}
}

func TestResponseCookieDeletionRemovesStaleValue(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "lm-csrf", MaxAge: -1, Path: "/"})
		_, _ = w.Write([]byte("ok"))
	})
	c.Cookie = "datadome=DD; lm-csrf=stale"
	if _, err := c.GetHTML("/"); err != nil {
		t.Fatal(err)
	}
	if got := cookieValue(c.Cookie, "lm-csrf"); got != "" {
		t.Fatalf("revoked cookie = %q, want removed", got)
	}
}

func TestProductOffer(t *testing.T) {
	html := `<html>
	<script type="application/ld+json">{"@type":"Product","name":"Taladro","sku":"83085630","offers":{"price":"13.99","priceCurrency":"EUR"}}</script>
	<script>{"offer_id":"15cd48c7225762da","contextId":"058","seller_id":"002"}</script>
	</html>`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(html))
	})
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
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>no offer here</html>`))
	})
	if _, _, _, _, err := c.ProductOffer("/productos/x-1.html"); err == nil {
		t.Fatal("want error when offerId absent")
	}
}

func TestProductOfferRejectsExternalURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<script>{"offer_id":"15cd48c7225762da"}</script>`))
	}))
	defer srv.Close()

	c := New()
	if _, _, _, _, err := c.ProductOffer(srv.URL + "/productos/x-1.html"); err == nil {
		t.Fatal("external product URL should be rejected")
	}
}

// A negative quantity is not a small cart, it is a broken response; treating it
// as data would let it flow into totals.
func TestCartDataRejectsANegativeQuantity(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"quantity":-1,"order":"o1"}`))
	})
	if _, err := c.CartData(); err == nil {
		t.Fatal("want an error for a negative cart quantity")
	}
}

func TestCartDataSurfacesTransportFailures(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.CartData(); err == nil {
		t.Fatal("want the HTTP 500 surfaced")
	}
}

// An absent quantity and context code fall back to defaults rather than sending
// the storefront a zero-quantity line.
func TestAddToCartAppliesDefaults(t *testing.T) {
	var body string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == cartAddPath {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"quantity":1,"order":"o1"}`))
	})
	if _, err := c.AddToCart("ref", "offer", "", 0); err != nil {
		t.Fatalf("AddToCart: %v", err)
	}
	if !strings.Contains(body, `"quantity":1`) {
		t.Errorf("body = %s, want quantity defaulted to 1", body)
	}
	if !strings.Contains(body, defaultContextCode) {
		t.Errorf("body = %s, want the default context code", body)
	}
}

func TestAddToCartSurfacesAFailedWrite(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := c.AddToCart("ref", "offer", "058", 1); err == nil {
		t.Fatal("want the rejected write surfaced")
	}
}

// A 2xx whose cart stays empty is what a wrong contextCode looks like: accepted
// and discarded. It must surface as an error rather than a successful add of
// nothing, which is what the exit code has to reflect.
func TestAddToCartGivesUpWhenBothProjectionsStayEmpty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case cartAddPath:
			_, _ = w.Write([]byte(`{}`))
		case cartDetailPath:
			_, _ = w.Write([]byte(`{"orderId":"o1","offersQuantity":0,"cartVendors":[]}`))
		default:
			_, _ = w.Write([]byte(`{"quantity":0,"order":"o1"}`))
		}
	})
	sum, err := c.AddToCart("ref", "offer", "058", 1)
	if err == nil {
		t.Fatalf("AddToCart reported success with %+v, want the no-op surfaced", sum)
	}
	if !strings.Contains(err.Error(), "cart did not change") {
		t.Errorf("error = %v, want it to say the cart did not change", err)
	}
}

func TestDeleteLineSurfacesAFailedDelete(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})
	if err := c.DeleteLine("line-1"); err == nil {
		t.Fatal("want the rejected delete surfaced")
	}
}

// Most product URLs carry the reference in their slug, but a curated landing
// URL does not; the SKU from the page's JSON-LD is the fallback, without which
// the add-to-cart call would go out with an empty reference.
func TestProductOfferFallsBackToTheJSONLDSKU(t *testing.T) {
	page := `<script type="application/ld+json">` +
		`{"@type":"Product","name":"Taladro","sku":"83085630",` +
		`"offers":{"price":"13.99","priceCurrency":"EUR"}}</script>` +
		`<script>{"offer_id":"deadbeefdeadbeef"}</script>`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	})
	reflm, _, _, detail, err := c.ProductOffer("/productos/taladros/")
	if err != nil {
		t.Fatalf("ProductOffer: %v", err)
	}
	if reflm != "83085630" {
		t.Errorf("reflm = %q, want the JSON-LD sku", reflm)
	}
	if detail == nil || detail.Name != "Taladro" {
		t.Errorf("detail = %+v, want the parsed product", detail)
	}
}

// The summary endpoint can answer without an order id right after a write while
// the detailed cart already has it; the add must report the detail's id rather
// than an empty one.
func TestAddToCartTakesTheOrderIDFromTheDetailedCart(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case cartAddPath:
			_, _ = w.Write([]byte(`{}`))
		case cartDetailPath:
			_, _ = w.Write([]byte(`{"orderId":"from-detail","offersQuantity":2,
			  "cartVendors":[{"cartVendorItems":[{"id":"l1","quantity":2,"offer":{"refLM":"r","label":"x"}}]}]}`))
		default:
			_, _ = w.Write([]byte(`{"quantity":0,"order":""}`))
		}
	})
	sum, err := c.AddToCart("ref", "offer", "058", 1)
	if err != nil {
		t.Fatalf("AddToCart: %v", err)
	}
	if sum.Quantity != 2 {
		t.Errorf("quantity = %d, want the detailed cart's 2", sum.Quantity)
	}
	if sum.Order != "from-detail" {
		t.Errorf("order = %q, want from-detail", sum.Order)
	}
}

// If the detailed cart fails while the summary is still lagging at zero, the
// add must surface that failure rather than report an empty cart as success.
func TestAddToCartSurfacesADetailFailureWhileTheSummaryLags(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case cartAddPath:
			_, _ = w.Write([]byte(`{}`))
		case cartDetailPath:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(`{"quantity":0,"order":"o1"}`))
		}
	})
	if _, err := c.AddToCart("ref", "offer", "058", 1); err == nil {
		t.Fatal("want the detailed-cart failure surfaced")
	}
}

// A fractional line quantity is not a cart we can price, so the parse refuses
// rather than truncating silently.
func TestCartDetailRefusesAFractionalLineQuantity(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"orderId":"o1","offersQuantity":1,"cartVendors":[{"cartVendorItems":[
		  {"id":"l1","quantity":1.5,"offer":{"refLM":"r","label":"x"}}]}]}`))
	})
	if _, err := c.Cart(); err == nil {
		t.Fatal("want a fractional line quantity refused")
	}
}

// The page renders the add-to-cart form as hidden inputs, and that block — not
// any JSON blob — is what carries contextCode. Reading it is what makes the
// write land; the storefront accepts a guessed code and changes nothing.
func TestProductOfferPrefersTheAddToCartForm(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>` +
			`<script>{"offer_id":"aaaaaaaaaaaaaaaaaaaa","contextCode":"999"}</script>` +
			`<div class="js-atc-list__item">` +
			`<input type="hidden" name="reflm" value="88878737"/>` +
			`<input type="hidden" name="offerId" value="25fb478aa09ee12eb0478f305d4641c7"/>` +
			`<input type="hidden" name="contextCode" value="033"/>` +
			`</div></html>`))
	})

	reflm, offerID, contextCode, _, err := c.ProductOffer("/productos/cinta-88878737.html")
	if err != nil {
		t.Fatal(err)
	}
	if reflm != "88878737" || offerID != "25fb478aa09ee12eb0478f305d4641c7" || contextCode != "033" {
		t.Errorf("got %q/%q/%q, want the form's values, not the JSON blob's", reflm, offerID, contextCode)
	}
}

// Pages without that block still resolve through the JSON and the URL.
func TestProductOfferFallsBackToTheJSONBlob(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><script>{"offer_id":"15cd48c7225762da","contextId":"058"}</script></html>`))
	})

	reflm, offerID, contextCode, _, err := c.ProductOffer("/productos/taladro-83085630.html")
	if err != nil {
		t.Fatal(err)
	}
	if reflm != "83085630" || offerID != "15cd48c7225762da" || contextCode != "058" {
		t.Errorf("got %q/%q/%q, want the JSON fallback", reflm, offerID, contextCode)
	}
}

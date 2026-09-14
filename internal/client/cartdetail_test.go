package client

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const cartJSON = `{
  "orderId":"985cada6",
  "offersQuantity":2,
  "disabledCheckout":false,
  "cannotBeValidatedReasons":["ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER","SIMULATION_NEEDS_CITY"],
  "orderResume":{"totalAmount":31.88,"offersAmount":27.98,"deliveryAmount":3.9},
  "cartVendors":[{"cartVendorItems":[
    {"id":"line-uuid-1","quantity":2,"discountPrice":27.98,"offer":{"refLM":"83085630","label":"Taladro PRACTYL"}}
  ]}]
}`

func TestCartDetailParse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cartDetailPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(cartJSON))
	})
	cart, err := c.Cart()
	if err != nil {
		t.Fatal(err)
	}
	if cart.Quantity != 2 || cart.TotalAmount != 31.88 || cart.DeliveryAmount != 3.9 {
		t.Errorf("totals wrong: %+v", cart)
	}
	if len(cart.Lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(cart.Lines))
	}
	l := cart.Lines[0]
	if l.LineID != "line-uuid-1" || l.Reflm != "83085630" || l.Quantity != 2 || l.Price != 27.98 {
		t.Errorf("line wrong: %+v", l)
	}
	if len(cart.Blockers) != 2 {
		t.Errorf("blockers = %v", cart.Blockers)
	}
}

func TestCartDetailFloatCounts(t *testing.T) {
	// The live API serialises counts with a decimal (offersQuantity: 0.0,
	// quantity: 3.0) — these must decode, not error on int unmarshal.
	body := `{"orderId":"o","offersQuantity":3.0,"orderResume":{"totalAmount":4.47},
	  "cartVendors":[{"cartVendorItems":[{"id":"l","quantity":3.0,"discountPrice":4.47,"offer":{"refLM":"1","label":"x"}}]}]}`
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	cart, err := c.Cart()
	if err != nil {
		t.Fatalf("float counts must decode: %v", err)
	}
	if cart.Quantity != 3 || cart.Lines[0].Quantity != 3 {
		t.Errorf("quantities = %d / %d, want 3 / 3", cart.Quantity, cart.Lines[0].Quantity)
	}
}

func TestCartDetailRejectsFractionalCounts(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"orderId":"o","offersQuantity":1.5}`))
	})
	if _, err := c.Cart(); err == nil {
		t.Fatal("fractional cart quantity should be rejected")
	}
}

func TestSetLineQuantityRequest(t *testing.T) {
	var method, path string
	var body map[string]int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(200)
	})
	if err := c.SetLineQuantity("line-uuid-1", 5); err != nil {
		t.Fatal(err)
	}
	if method != "PUT" || !strings.HasSuffix(path, "/update-offer-line-quantity/line-uuid-1") {
		t.Errorf("PUT to %q (%s)", path, method)
	}
	if body["quantity"] != 5 {
		t.Errorf("body quantity = %d", body["quantity"])
	}
}

func TestDeleteLineRequest(t *testing.T) {
	var method, path string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(200)
	})
	if err := c.DeleteLine("line-uuid-1"); err != nil {
		t.Fatal(err)
	}
	if method != "DELETE" || !strings.HasSuffix(path, "/delete-offer-line/line-uuid-1") {
		t.Errorf("DELETE to %q (%s)", path, method)
	}
}

// A malformed body must be reported as a decode failure naming the URL, not as
// a silent zero value.
func TestGetJSONReportsAMalformedBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})
	var out map[string]any
	err := c.getJSON("/checkout/backend/cart", &out)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("err = %v, want a decode error", err)
	}
}

// An empty body is not a decode failure: several storefront writes answer 200
// with nothing.
func TestDoDecodeAcceptsAnEmptyBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	var out map[string]any
	if err := c.getJSON("/checkout/backend/cart", &out); err != nil {
		t.Fatalf("empty body should decode cleanly, got %v", err)
	}
}

func TestPostJSONRejectsAnUnencodableBody(t *testing.T) {
	c := New()
	c.BaseURL = "http://example.invalid"

	if err := c.postJSON("/x", make(chan int), nil); err == nil {
		t.Fatal("want an error for an unencodable body")
	}
}

// Both cart projections and the shipping read must surface a storefront failure
// rather than answer with an empty cart, which the CLI would print as "empty".
func TestCartAndShippingSurfaceTransportFailures(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := c.Cart(); err == nil {
		t.Error("Cart should surface the failure")
	}
	if _, err := c.Shipping(); err == nil {
		t.Error("Shipping should surface the failure")
	}
}

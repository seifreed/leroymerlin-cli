package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cartDetailPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(cartJSON))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	cart, err := c.Cart()
	if err != nil {
		t.Fatalf("float counts must decode: %v", err)
	}
	if cart.Quantity != 3 || cart.Lines[0].Quantity != 3 {
		t.Errorf("quantities = %d / %d, want 3 / 3", cart.Quantity, cart.Lines[0].Quantity)
	}
}

func TestSetLineQuantityRequest(t *testing.T) {
	var method, path string
	var body map[string]int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := New()
	c.BaseURL = srv.URL
	if err := c.DeleteLine("line-uuid-1"); err != nil {
		t.Fatal(err)
	}
	if method != "DELETE" || !strings.HasSuffix(path, "/delete-offer-line/line-uuid-1") {
		t.Errorf("DELETE to %q (%s)", path, method)
	}
}

package client

import (
	"net/http"
	"testing"
)

const shippingJSON = `{
  "addresses": {
    "deliveryAddress": {"line1":"Calle Falsa 123","postalCode":"08001","city":"Barcelona","firstName":"Ada"},
    "invoiceAddress": {"line1":"Calle Falsa 123","city":"Barcelona"},
    "relayPointAddress": null,
    "technicalData": {"x":1}
  },
  "deliveryVendors": [{
    "deliveryVendorDeliveryGroups": [{
      "deliveryVendorServiceLevels": [
        {"mode":"PICKUP_IN_STORE","labelCode":"PICKUP_EXP","amount":0,"selected":true,"appointmentDate":"2026-07-01T18:30:00+02:00"},
        {"mode":"PICKUP_IN_STORE","labelCode":"PICKUP_EXP","amount":0,"selected":true,"appointmentDate":"2026-07-01T18:30:00+02:00"},
        {"mode":"HOME_DELIVERY","labelCode":"HOME_STD","amount":3.9,"selected":false,"appointmentDate":null,"deliveryDateInfo":{"fixedDate":"2026-07-02T06:00:00Z"}}
      ]
    }]
  }]
}`

func TestShipping(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != shippingPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(shippingJSON))
	})
	info, err := c.Shipping()
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Slots) != 2 {
		t.Fatalf("want 2 slots, got %d", len(info.Slots))
	}
	if info.Slots[0].Mode != "PICKUP_IN_STORE" || !info.Slots[0].Selected || info.Slots[0].Amount != 0 {
		t.Errorf("slot 0 = %+v", info.Slots[0])
	}
	// second slot has no appointmentDate → falls back to deliveryDateInfo.fixedDate
	if info.Slots[1].Date != "2026-07-02T06:00:00Z" || info.Slots[1].Amount != 3.9 {
		t.Errorf("slot 1 date/amount = %+v", info.Slots[1])
	}
	if d := info.Addresses["deliveryAddress"]; d["city"] != "Barcelona" {
		t.Errorf("delivery address city = %v", d["city"])
	}
}

// The storefront reports a slot once per delivery group, so the chosen option
// comes back twice — identical but for the selected flag. Listing both showed
// the same choice twice, one of them unmarked.
func TestShippingKeepsOneRowPerOptionAndMarksIt(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"deliveryVendors":[{"deliveryVendorDeliveryGroups":[
			{"deliveryVendorServiceLevels":[
			  {"mode":"PICKUP_IN_STORE","labelCode":"EXP_ONE_HOUR","amount":0,"appointmentDate":"2026-09-14T11:35:32.866Z","selected":false}]},
			{"deliveryVendorServiceLevels":[
			  {"mode":"PICKUP_IN_STORE","labelCode":"EXP_ONE_HOUR","amount":0,"appointmentDate":"2026-09-14T11:35:32.866Z","selected":true},
			  {"mode":"HOME_DELIVERY","labelCode":"STD","amount":3.9,"appointmentDate":"2026-09-16T06:00:00Z","selected":false}]}]}]}`))
	})

	info, err := c.Shipping()
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Slots) != 2 {
		t.Fatalf("got %d slots, want the duplicate pickup collapsed: %+v", len(info.Slots), info.Slots)
	}
	if !info.Slots[0].Selected {
		t.Errorf("the surviving pickup row lost its selected mark: %+v", info.Slots[0])
	}
	if info.Slots[1].Mode != "HOME_DELIVERY" || info.Slots[1].Selected {
		t.Errorf("second slot = %+v, want the unselected home delivery", info.Slots[1])
	}
}

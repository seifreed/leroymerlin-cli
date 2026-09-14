package client

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// escJSON is the escape sequence as it appears inside a JSON string — raw
// control bytes would make the document itself invalid. escRaw is the same
// sequence as real bytes, for the fixtures scraped straight out of HTML.
const (
	escJSON = `\u001b]0;PWNED\u0007\u001b[2J`
	escRaw  = "\x1b]0;PWNED\x07\x1b[2J"
)

// assertNoControl walks v — structs, slices, maps, pointers — and fails on any
// string carrying a control character, so a field added later is covered without
// anyone remembering to extend this test.
func assertNoControl(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		if s := v.String(); strings.ContainsFunc(s, isControl) {
			t.Errorf("%s = %q carries a control character", path, s)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			assertNoControl(t, v.Elem(), path)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			assertNoControl(t, v.Index(i), path+"[]")
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			assertNoControl(t, v.MapIndex(k), path+"["+k.String()+"]")
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				assertNoControl(t, v.Field(i), path+"."+v.Type().Field(i).Name)
			}
		}
	}
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// Every read is parsed from text a marketplace seller can write, and all of it
// can reach a terminal. Rather than listing the fields that get scrubbed, this
// walks whatever the parsers produce, so a field added later is covered.
func TestNoParsedFieldCarriesControlCharacters(t *testing.T) {
	t.Run("search", func(t *testing.T) {
		card := `<script type="application/json" class="dataTms">[{"name":"cdl_products_list","value":[` +
			`{"identifier":"1` + escJSON + `","name":"Taladro` + escJSON + `","brand":"DEX` + escJSON + `",` +
			`"url":"/productos/x-1.html","offer":{"unitprice_ati":9.99,"seller_name":"Ven` + escJSON + `",` +
			`"seller_type":"3P` + escJSON + `","offer_type":"o` + escJSON + `","commercial_animations":{"label":"promo` + escJSON + `"}}}` +
			`]}]</script>`
		got := parseProducts(card)
		if len(got) != 1 {
			t.Fatalf("parsed %d products, want the hostile card", len(got))
		}
		assertNoControl(t, reflect.ValueOf(got), "products")
	})

	t.Run("product page", func(t *testing.T) {
		html := `<script type="application/ld+json">{"@type":"Product","name":"Taladro` + escJSON + `",` +
			`"sku":"42` + escJSON + `","gtin":"g` + escJSON + `","description":"desc` + escJSON + `","brand":"DEX` + escJSON + `",` +
			`"image":"/i.png` + escJSON + `","aggregateRating":{"ratingValue":"4` + escJSON + `","reviewCount":"7` + escJSON + `"},` +
			`"offers":{"price":"9.99","priceCurrency":"EUR` + escJSON + `","availability":"InStock` + escJSON + `",` +
			`"url":"/u` + escJSON + `","itemCondition":"New` + escJSON + `"}}</script>` +
			`<li class="o-main-characteristics__li">Potencia` + escRaw + ` : 500 W` + escRaw + `</li>` +
			`<script>{"available_deliveries":[{"type":"homeDelivery` + escJSON + `","stockStatus":"IN` + escJSON + `",` +
			`"stock":3,"price":0,"time":"2 HOUR` + escJSON + `"}]}</script>`
		d, err := parseProductDetail(html)
		if err != nil {
			t.Fatal(err)
		}
		d.Specs = parseSpecs(html)
		d.Deliveries = parseDeliveries(html)
		if len(d.Specs) == 0 || len(d.Deliveries) == 0 {
			t.Fatalf("specs=%d deliveries=%d, want the hostile rows parsed", len(d.Specs), len(d.Deliveries))
		}
		assertNoControl(t, reflect.ValueOf(d), "detail")
	})

	t.Run("cart and shipping", func(t *testing.T) {
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "shipping") {
				_, _ = w.Write([]byte(`{"addresses":{"deliveryAddress":{"city":"Madrid` + escJSON + `","line1":"C/ A` + escJSON + `"}},` +
					`"deliveryVendors":[{"deliveryVendorDeliveryGroups":[{"deliveryVendorServiceLevels":[` +
					`{"mode":"HOME` + escJSON + `","labelCode":"STD` + escJSON + `","amount":3.9,"appointmentDate":"2026-01-01` + escJSON + `"}]}]}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"orderId":"o1` + escJSON + `","offersQuantity":1,` +
				`"cannotBeValidatedReasons":["SIMULATION_NEEDS_CITY` + escJSON + `"],` +
				`"orderResume":{"totalAmount":9.99},` +
				`"cartVendors":[{"cartVendorItems":[{"id":"l1` + escJSON + `","quantity":1,` +
				`"offer":{"refLM":"r1` + escJSON + `","label":"Taladro` + escJSON + `"}}]}]}`))
		})

		cart, err := c.Cart()
		if err != nil {
			t.Fatal(err)
		}
		assertNoControl(t, reflect.ValueOf(cart), "cart")

		shipping, err := c.Shipping()
		if err != nil {
			t.Fatal(err)
		}
		assertNoControl(t, reflect.ValueOf(shipping), "shipping")
	})
}

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
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

func TestParseSpecs(t *testing.T) {
	html := `<ul class="o-main-characteristics__ul">
	  <li class="o-main-characteristics__li"><span class="o-main-characteristics__text">Función percutor : Sí</span></li>
	  <li class="o-main-characteristics__li"><span class="o-main-characteristics__text">Dimensiones (mm) : 270 : 225 : 70</span></li>
	  <li class="o-main-characteristics__li"><span class="o-main-characteristics__text">Función percutor : Sí</span></li>
	  <li class="o-main-characteristics__li">Ver más window.x = { foo: 1 } 21 en stock en Badalona Añadir al carrito</li>
	</ul>`
	specs := parseSpecs(html)
	if len(specs) != 2 {
		t.Fatalf("want 2 specs (deduped, junk dropped), got %d: %+v", len(specs), specs)
	}
	if specs[0].Label != "Función percutor" || specs[0].Value != "Sí" {
		t.Errorf("first spec = %+v", specs[0])
	}
	// value keeps its inner colons (split only on the first " : ")
	if specs[1].Value != "270 : 225 : 70" {
		t.Errorf("colon-in-value spec = %+v", specs[1])
	}
}

func TestParseSpecsNone(t *testing.T) {
	if specs := parseSpecs(`<html>no characteristics here</html>`); specs != nil {
		t.Errorf("want nil, got %+v", specs)
	}
}

func TestParseDeliveries(t *testing.T) {
	html := `<script>{"offer":{"available_deliveries":[
	  {"price":0.0,"stock":21,"stockStatus":"ONSITE","time":"2 HOUR","type":"storeDelivery"},
	  {"price":3.9,"stock":25,"stockStatus":"ONSITE","time":"1 OPENING_DAY","type":"homeDelivery"}
	]}}</script>`
	ds := parseDeliveries(html)
	if len(ds) != 2 {
		t.Fatalf("want 2 delivery options, got %d", len(ds))
	}
	if ds[0].Type != "storeDelivery" || ds[0].Stock != 21 || ds[0].Price != 0 || ds[0].Status != "ONSITE" {
		t.Errorf("store delivery = %+v", ds[0])
	}
	if ds[1].Type != "homeDelivery" || ds[1].Price != 3.9 || ds[1].Stock != 25 {
		t.Errorf("home delivery = %+v", ds[1])
	}
}

func TestExtractJSONArray(t *testing.T) {
	// nested brackets must stay balanced
	s := `foo "k":[{"a":[1,2]},{"b":3}] bar`
	if got := extractJSONArray(s, `"k"`); got != `[{"a":[1,2]},{"b":3}]` {
		t.Errorf("got %q", got)
	}
	if extractJSONArray("no key here", `"k"`) != "" {
		t.Error("missing key should yield empty")
	}
}

func TestParseProductDetailNoProduct(t *testing.T) {
	_, err := parseProductDetail(`<script type="application/ld+json">{"@type":"Website"}</script>`)
	if err != ErrNoProduct {
		t.Fatalf("want ErrNoProduct, got %v", err)
	}
}

func TestProductHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

// schema.org @type is a string in most pages but an array in some; typeIs has to
// accept both without tripping on absent or malformed values.
func TestTypeIsAcceptsStringAndArrayForms(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`"Product"`, true},
		{`"WebPage"`, false},
		{`["WebPage","Product"]`, true},
		{`["WebPage"]`, false},
		{`{}`, false},
		{`null`, false},
		{``, false},
	} {
		got := typeIs(json.RawMessage(tc.raw), "Product")
		if got != tc.want {
			t.Errorf("typeIs(%s) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// Storefront JSON-LD is inconsistent: brand and image come back as a bare
// string, an object, or an array depending on the page. The flex types absorb
// all three so a shape change does not drop the field.
func TestFlexNameAcceptsStringObjectAndNull(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    flexName
		wantErr bool
	}{
		{`"Bosch"`, "Bosch", false},
		{`{"name":"Bosch"}`, "Bosch", false},
		{`{"@type":"Brand"}`, "", false},
		{`null`, "", false},
		{`[1,2]`, "", true},
		// Starts like a string but is not one: the quoted branch must report the
		// decode failure rather than swallow it.
		{`"unterminated`, "", true},
		{`"bad\q"`, "", true},
	} {
		var got flexName
		err := got.UnmarshalJSON([]byte(tc.raw))
		if (err != nil) != tc.wantErr {
			t.Errorf("flexName(%s) error = %v, wantErr %v", tc.raw, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("flexName(%s) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestFlexImageAcceptsStringArrayAndObject(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    flexImage
		wantErr bool
	}{
		{`"https://x/1.jpg"`, "https://x/1.jpg", false},
		{`["https://x/1.jpg","https://x/2.jpg"]`, "https://x/1.jpg", false},
		{`[]`, "", false},
		{`{"url":"https://x/3.jpg"}`, "https://x/3.jpg", false},
		{`null`, "", false},
		{``, "", false},
		{`["ok", 5]`, "", true},
		{`5`, "", true},
		{`"unterminated`, "", true},
		{`"bad\q"`, "", true},
	} {
		var got flexImage
		err := got.UnmarshalJSON([]byte(tc.raw))
		if (err != nil) != tc.wantErr {
			t.Errorf("flexImage(%s) error = %v, wantErr %v", tc.raw, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("flexImage(%s) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// The product page is scraped, not an API: every parser has to return empty
// rather than fail when the storefront's markup shifts.
func TestProductParsersTolerateBrokenMarkup(t *testing.T) {
	for _, tc := range []struct {
		name string
		html string
	}{
		{"no JSON-LD block at all", `<html><body>nada</body></html>`},
		{"JSON-LD that is not JSON", `<script type="application/ld+json">{not json</script>`},
		{"JSON-LD array with no Product", `<script type="application/ld+json">[{"@type":"WebPage"}]</script>`},
		{"JSON-LD array that is malformed", `<script type="application/ld+json">[{"@type":</script>`},
		{"empty JSON-LD block", `<script type="application/ld+json"></script>`},
		{"Product type but unreadable payload", `<script type="application/ld+json">{"@type":"Product","offers":5}</script>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if d, err := parseProductDetail(tc.html); err == nil && d != nil {
				t.Errorf("parsed a product out of broken markup: %+v", d)
			}
		})
	}
}

// Deliveries and specs are optional embedded blobs; malformed ones are dropped,
// never propagated as an error that would hide an otherwise usable page.
func TestDeliveryAndSpecParsersDropMalformedBlocks(t *testing.T) {
	if got := parseDeliveries(`<html>available_deliveries = {not an array}</html>`); got != nil {
		t.Errorf("deliveries = %+v, want nil for a malformed blob", got)
	}
	if got := parseDeliveries(`<html>no deliveries here</html>`); got != nil {
		t.Errorf("deliveries = %+v, want nil when absent", got)
	}
}

// extractJSONArray hunts a bracketed blob inside a <script>; a key present
// without an array after it must yield nothing rather than a truncated slice.
func TestExtractJSONArrayNeedsBothKeyAndArray(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"key absent", `var x = [1,2,3]`},
		{"key without an array", `"available_deliveries": null`},
		{"unterminated array", `"available_deliveries": [{"a":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractJSONArray(tc.in, `"available_deliveries"`); got != "" {
				t.Errorf("extracted %q, want empty", got)
			}
		})
	}
}

// Spec rows are lifted from free-form markup, so template leftovers land in the
// candidate set: script fragments, call-to-action labels and price/stock strings
// all have to be filtered or the spec table fills with junk. Rows are the
// o-main-characteristics__li elements and the separator is " : ".
func TestParseSpecsFiltersTemplateJunk(t *testing.T) {
	row := func(text string) string {
		return `<li class="o-main-characteristics__li">` + text + `</li>`
	}
	html := `<ul>` +
		row(`Potencia : 700W`) +
		row(`Script : window.dataLayer`) +
		row(`Llave : {"a":1}`) +
		row(`Comentario : algo -->`) +
		row(`Accion : Ver más`) +
		row(`Precio : 13,99 EUR`) +
		row(`Stock : en stock`) +
		row(`Vacio : `) +
		row(`sin separador`) +
		row(`Potencia : 900W`) +
		`</ul>`

	specs := parseSpecs(html)

	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want only the real characteristic", specs)
	}
	if specs[0].Label != "Potencia" || specs[0].Value != "700W" {
		t.Errorf("spec = %+v, want Potencia/700W", specs[0])
	}
}

// An over-long label or value is template spill, not a characteristic.
func TestParseSpecsDropsOverLongRows(t *testing.T) {
	long := strings.Repeat("x", 81)
	html := `<li class="o-main-characteristics__li">` + long + ` : 1</li>` +
		`<li class="o-main-characteristics__li">Largo : ` + long + `</li>`

	if specs := parseSpecs(html); len(specs) != 0 {
		t.Errorf("specs = %+v, want none", specs)
	}
}

// An unterminated <script type="application/ld+json"> must end the scan rather
// than read past the end of the document.
func TestParseProductDetailStopsAtAnUnterminatedBlock(t *testing.T) {
	for _, html := range []string{
		`<script type="application/ld+json"`,                    // no closing >
		`<script type="application/ld+json">{"@type":"Product"`, // no </script>
	} {
		if d, err := parseProductDetail(html); err == nil && d != nil {
			t.Errorf("parsed %+v out of an unterminated block", d)
		}
	}
}

// offers can be a single object or an array; a null or absent one leaves the
// slice empty instead of failing the whole page.
func TestOfferNodeAcceptsNullAndArray(t *testing.T) {
	var o offerNode
	if err := o.UnmarshalJSON([]byte(`null`)); err != nil || len(o) != 0 {
		t.Errorf("null offers = %+v, %v", o, err)
	}
	if err := o.UnmarshalJSON([]byte(`[{"price":"1.00"},{"price":"2.00"}]`)); err != nil || len(o) != 2 {
		t.Errorf("array offers = %+v, %v", o, err)
	}
}

// A balanced array of the wrong element type gets past extractJSONArray and
// fails in the decoder — a different branch from "no array at all", which is
// where every other negative case lands.
func TestParseDeliveriesIgnoresAWellFormedArrayOfTheWrongShape(t *testing.T) {
	if got := parseDeliveries(`<script>{"available_deliveries":["x"]}</script>`); got != nil {
		t.Errorf("deliveries = %+v, want nil", got)
	}
}

// Product takes a URL from a search result, so it must refuse one that points
// somewhere else — the same guard cart add has always had on ProductOffer.
func TestProductRejectsAnOffStorefrontURL(t *testing.T) {
	c := New()
	c.BaseURL = "https://www.leroymerlin.es"

	for _, target := range []string{
		"https://example.com/productos/x-1.html",
		"http://www.leroymerlin.es/productos/x-1.html", // scheme downgrade
		"https://www.leroymerlin.es.example.com/productos/x-1.html",
	} {
		if _, err := c.Product(target); err == nil {
			t.Errorf("Product(%q) = no error, want a refusal", target)
		}
	}
}

// The JSON-LD on a product page can lag the offer the storefront bills from: a
// drill listed and charged at 14.95 advertised 13.99 in its structured data.
// Detail, basket totals and the --max guard all price from this, so the live
// offer wins.
func TestProductPricesFromTheLiveOfferNotStaleJSONLD(t *testing.T) {
	page := `<html><script type="application/ld+json">` +
		`{"@type":"Product","name":"Taladro","sku":"83085630",` +
		`"offers":{"price":"13.99","priceCurrency":"EUR"}}</script>` +
		`<input type="hidden" name="offerId" value="25fb478aa09ee12eb0478f305d4641c7"/>` +
		`<script>{"offer_id":"25fb478aa09ee12eb0478f305d4641c7","seller_name":"Leroy Merlin",` +
		`"unitprice_ati":14.95,"unitprice_tf":12.36}</script></html>`

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	})

	d, err := c.Product("/productos/taladro-83085630.html")
	if err != nil {
		t.Fatal(err)
	}
	if d.Price() != "14.95" {
		t.Errorf("Product price = %q, want the live 14.95 rather than the JSON-LD 13.99", d.Price())
	}
	if d.Currency() != "EUR" {
		t.Errorf("currency = %q, want EUR kept from the JSON-LD", d.Currency())
	}

	// cart add prices the --max guard from the same detail.
	_, _, _, detail, err := c.ProductOffer("/productos/taladro-83085630.html")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Price() != "14.95" {
		t.Errorf("ProductOffer price = %q, want the live 14.95", detail.Price())
	}
}

// A page whose offer blob carries no price leaves the JSON-LD alone.
func TestProductKeepsJSONLDWhenNoLiveOfferPrice(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><script type="application/ld+json">` +
			`{"@type":"Product","name":"Taladro","offers":{"price":"13.99","priceCurrency":"EUR"}}` +
			`</script></html>`))
	})

	d, err := c.Product("/productos/taladro-83085630.html")
	if err != nil {
		t.Fatal(err)
	}
	if d.Price() != "13.99" {
		t.Errorf("price = %q, want the JSON-LD kept", d.Price())
	}
}

// A page with the live offer but no product JSON-LD is not a product page, so
// the detail is absent and only the offer fields resolve.
func TestApplyLivePriceCreatesAnOfferWhenJSONLDHasNone(t *testing.T) {
	d := &domain.ProductDetail{Name: "Taladro"}
	applyLivePrice(d, `{"offer_id":"aaaaaaaaaaaaaaaa","unitprice_ati":7.5}`, "aaaaaaaaaaaaaaaa")
	if d.Price() != "7.5" || d.Currency() != "EUR" {
		t.Errorf("offers = %+v, want the live price in euros", d.Offers)
	}
	applyLivePrice(nil, "", "")
	applyLivePrice(d, "", "")
	if d.Price() != "7.5" {
		t.Errorf("price changed without a live offer: %q", d.Price())
	}
}

// livePrice only trusts the offer the page names, so an id that is not in the
// page — or an offer object with no price — leaves the detail alone. pageOfferID
// falls back to the JSON hash when the add-to-cart form is absent.
func TestLivePriceAndPageOfferIDEdges(t *testing.T) {
	const blob = `{"offer_id":"15cd48c7225762da","unitprice_ati":9.99}`

	if got := livePrice(blob, ""); got != "" {
		t.Errorf("no offer id should yield no price, got %q", got)
	}
	if got := livePrice(blob, "ffffffffffffffff"); got != "" {
		t.Errorf("an id absent from the page should yield no price, got %q", got)
	}
	if got := livePrice(`{"offer_id":"15cd48c7225762da","seller_name":"LM"}`, "15cd48c7225762da"); got != "" {
		t.Errorf("an offer without a price should yield none, got %q", got)
	}
	if got := livePrice(blob, "15cd48c7225762da"); got != "9.99" {
		t.Errorf("livePrice = %q, want 9.99", got)
	}

	if got := pageOfferID(blob); got != "15cd48c7225762da" {
		t.Errorf("pageOfferID = %q, want the JSON fallback", got)
	}
	if got := pageOfferID("<html>nada</html>"); got != "" {
		t.Errorf("pageOfferID = %q, want empty when the page names no offer", got)
	}
}

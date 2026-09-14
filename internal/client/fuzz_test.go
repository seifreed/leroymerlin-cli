package client

import "testing"

// Fuzz the scrapers/parsers that consume server HTML and user-supplied HAR. The
// whole client is built on scraping untrusted, possibly-truncated markup, so
// none of these may panic on adversarial input — a crash is a DoS on every read.
// The seed corpus also runs under plain `go test`.

var htmlSeeds = []string{
	"",
	"<html>",
	`<script type="application/json" class="dataTms">[{"name":"cdl_products_list","value":[{`,
	`<script type="application/json" class="dataTms">[{"name":"cdl_products_list","value":[{"identifier":"1","offer":{"unitprice_ati":1.5}}]}]</script>`,
	`<script type="application/ld+json">{"@type":"Product","offers":[`,
	`<li class="o-main-characteristics__li">a : b</li>`,
	`<a data-button-name="X" href="/productos/a/b/">`,
	`"available_deliveries":[{"stock":1,`,
	`<a href="/productos/x/">N</a>`,
}

func FuzzParseProducts(f *testing.F) {
	for _, s := range htmlSeeds {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) { _ = parseProducts(s) })
}

func FuzzParseProductDetail(f *testing.F) {
	for _, s := range htmlSeeds {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) { _, _ = parseProductDetail(s) })
}

func FuzzScrapeProductExtras(f *testing.F) {
	for _, s := range htmlSeeds {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) {
		_ = parseSpecs(s)
		_ = parseDeliveries(s)
		_ = parseCategories(s)
		_ = parseSubcategories(s, "/productos/herramientas/")
		_ = extractJSONArray(s, `"available_deliveries"`)
	})
}

func FuzzParseHAR(f *testing.F) {
	for _, s := range []string{
		"", "{}", `{"log":{}}`, `{"log":{"entries":[]}}`,
		`{"log":{"entries":[{"request":{"url":"https://www.leroymerlin.es/x","headers":[{"name":"Cookie","value":"datadome=1"}]}}]}}`,
		`{"log":{"entries":[{"request":{`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) { _, _ = ParseHAR([]byte(s)) })
}

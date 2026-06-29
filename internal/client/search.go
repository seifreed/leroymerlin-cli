package client

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Offer is the pricing/seller block the storefront embeds per product. Prices
// are euros; *_ati is tax-included (what the site shows), *_tf is tax-free.
type Offer struct {
	UnitPriceATI float64  `json:"unitprice_ati"`
	UnitPriceTF  float64  `json:"unitprice_tf"`
	InitialPrice *float64 `json:"initial_price"` // was-price when discounted (else null)
	DiscountATI  *float64 `json:"discount_ati"`
	DiscountRate *float64 `json:"discount_rate"`
	SellerName   string   `json:"seller_name"`
	SellerType   string   `json:"seller_type"` // "1P" = Leroy Merlin, "3P" = marketplace
	OfferType    string   `json:"offer_type"`  // NEW, RECONDITIONED…
	AddToCart    bool     `json:"add_to_cart_availability"`
	Animations   *struct {
		Label string `json:"label"`
	} `json:"commercial_animations"`
}

// Promo returns the commercial-animation label (e.g. "Envío gratis en pedidos
// +29€"), or "" when there is none.
func (o Offer) Promo() string {
	if o.Animations != nil {
		return o.Animations.Label
	}
	return ""
}

// Product is one catalog item as embedded in a search/listing card's
// `cdl_products_list` JSON. Identifier is the Leroy Merlin reference shown on
// the site; URL is the product page (pass it to Product detail).
type Product struct {
	Brand      string  `json:"brand"`
	Identifier string  `json:"identifier"`
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	Rating     float64 `json:"rating"`
	Sponsored  bool    `json:"product_is_sponsored"`
	OfferCount int     `json:"total_offer_count"`
	Offer      Offer   `json:"offer"`
}

// tmsEntry is one item in a card's `dataTms` JSON array; only cdl_products_list
// carries products.
type tmsEntry struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

const dataTmsMarker = `class="dataTms">`

// Search runs a full-text product search and returns the embedded product
// listing. limit caps the result count (0 = all the page returned, ~56). The
// site paginates by "load more" rather than a query param, so one call yields a
// single page — enough for ranking and basket pricing.
func (c *Client) Search(term string, limit int) ([]Product, error) {
	html, err := c.GetHTML("/search?q=" + url.QueryEscape(term))
	if err != nil {
		return nil, err
	}
	products := parseProducts(html)
	if limit > 0 && len(products) > limit {
		products = products[:limit]
	}
	return products, nil
}

// parseProducts lifts every product from the `cdl_products_list` blobs the page
// embeds, one per product card, in document order. Cards whose JSON is malformed
// are skipped rather than failing the whole read.
func parseProducts(html string) []Product {
	var out []Product
	seen := make(map[string]bool)
	rest := html
	for {
		i := strings.Index(rest, dataTmsMarker)
		if i < 0 {
			break
		}
		rest = rest[i+len(dataTmsMarker):]
		end := strings.Index(rest, "</script>")
		if end < 0 {
			break
		}
		block := rest[:end]
		rest = rest[end+len("</script>"):]
		var entries []tmsEntry
		if json.Unmarshal([]byte(strings.TrimSpace(block)), &entries) != nil {
			continue
		}
		for _, e := range entries {
			if e.Name != "cdl_products_list" {
				continue
			}
			var ps []Product
			if json.Unmarshal(e.Value, &ps) != nil {
				continue
			}
			for _, p := range ps {
				// A product can appear in more than one TMS bucket; keep the first.
				if p.Identifier == "" || seen[p.Identifier] {
					continue
				}
				seen[p.Identifier] = true
				out = append(out, p)
			}
		}
	}
	return out
}

package client

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

var (
	// offerIDRE matches the offer hash the page embeds (snake_case, same value the
	// addToCart call uses).
	offerIDRE = regexp.MustCompile(`"offer_id"\s*:\s*"([0-9a-fA-F]{16,})"`)
	// contextIDRE matches the offer's contextId/contextCode the addToCart call sends.
	contextIDRE = regexp.MustCompile(`"context(?:Id|Code)"\s*:\s*"([0-9A-Za-z]{2,8})"`)
	// refFromURLRE pulls the reflm from the "-<digits>.html" URL suffix.
	refFromURLRE = regexp.MustCompile(`-(\d+)\.html`)
	// atcFieldRE pulls one hidden field out of the add-to-cart block the page
	// renders behind its "Añadir al carrito" button. That block is what the button
	// submits, so it is the authority — above all for contextCode, which the page
	// carries nowhere else. Guessing it is not harmless: the storefront accepts
	// the write with a wrong code and leaves the cart untouched.
	atcFieldRE = regexp.MustCompile(`name="(reflm|offerId|contextCode)"\s+value="([^"]*)"`)
	// unitPriceRE matches the tax-included price inside an offer object.
	unitPriceRE = regexp.MustCompile(`"unitprice_ati"\s*:\s*([0-9]+(?:\.[0-9]+)?)`)
)

// livePrice returns the price the storefront bills from: the unitprice_ati of
// the offer the add-to-cart form names. The JSON-LD on the same page can lag it
// — a drill the cart charges 14.95 for advertised 13.99 in its structured data
// — and detail, basket totals and the --max guard must not price from stale
// data. The offer object serialises its keys alphabetically, so unitprice_ati
// follows offer_id within the same object.
func livePrice(html, offerID string) string {
	if offerID == "" {
		return ""
	}
	i := strings.Index(html, `"offer_id":"`+offerID+`"`)
	if i < 0 {
		return ""
	}
	if m := unitPriceRE.FindStringSubmatch(html[i:min(i+800, len(html))]); m != nil {
		return m[1]
	}
	return ""
}

// applyLivePrice overwrites the detail's first offer price with the live one.
func applyLivePrice(detail *domain.ProductDetail, html, offerID string) {
	price := livePrice(html, offerID)
	if price == "" || detail == nil {
		return
	}
	if len(detail.Offers) == 0 {
		// This storefront bills in euros; the JSON-LD that would have said so is
		// exactly what is missing here.
		detail.Offers = []domain.ProductOffer{{Price: price, PriceCurrency: "EUR"}}
		return
	}
	detail.Offers[0].Price = price
}

// pageOfferID is the offer the page's add-to-cart form names, falling back to
// the first offer hash in the embedded JSON.
func pageOfferID(html string) string {
	if id := atcFields(html)["offerId"]; id != "" {
		return id
	}
	if m := offerIDRE.FindStringSubmatch(html); m != nil {
		return m[1]
	}
	return ""
}

// atcFields reads the add-to-cart form fields, first occurrence winning — the
// product's own block precedes any rendered for related products.
func atcFields(html string) map[string]string {
	fields := map[string]string{}
	for _, m := range atcFieldRE.FindAllStringSubmatch(html, -1) {
		if _, seen := fields[m[1]]; !seen {
			fields[m[1]] = m[2]
		}
	}
	return fields
}

// ProductOffer fetches a product page and extracts what `cart add` needs: the
// reflm (from the URL, falling back to the JSON-LD sku), the offerId, the offer's
// contextCode (empty if not found → caller uses the default), and the product
// detail (name/price) for display and the spending guard.
func (c *Client) ProductOffer(urlOrPath string) (reflm, offerID, contextCode string, detail *domain.ProductDetail, err error) {
	resolved := c.resolve(urlOrPath)
	if !c.sameOrigin(resolved) {
		return "", "", "", nil, fmt.Errorf("product URL must belong to the configured storefront")
	}
	html, err := c.GetHTML(urlOrPath)
	if err != nil {
		return "", "", "", nil, err
	}
	fields := atcFields(html)
	reflm, offerID, contextCode = fields["reflm"], fields["offerId"], fields["contextCode"]
	if reflm == "" {
		if m := refFromURLRE.FindStringSubmatch(urlOrPath); m != nil {
			reflm = m[1]
		}
	}
	if offerID == "" {
		if m := offerIDRE.FindStringSubmatch(html); m != nil {
			offerID = m[1]
		}
	}
	if contextCode == "" {
		if m := contextIDRE.FindStringSubmatch(html); m != nil {
			contextCode = m[1]
		}
	}
	if d, derr := parseProductDetail(html); derr == nil {
		applyLivePrice(d, html, offerID)
		d.URL = resolved
		// The same page already carries the per-channel stock, and `cart add` warns
		// on a product with none of it; parsing it here costs no extra request.
		d.Deliveries = parseDeliveries(html)
		detail = d
		if reflm == "" {
			reflm = d.SKU
		}
	}
	if reflm == "" || offerID == "" {
		return reflm, offerID, contextCode, detail, fmt.Errorf("could not extract reflm/offerId from %s (is it a product page?)", urlOrPath)
	}
	return reflm, offerID, contextCode, detail, nil
}

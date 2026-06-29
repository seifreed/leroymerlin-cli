package client

import (
	"fmt"
	"regexp"
)

var (
	// offerIDRE matches the offer hash the page embeds (snake_case, same value the
	// addToCart call uses).
	offerIDRE = regexp.MustCompile(`"offer_id"\s*:\s*"([0-9a-fA-F]{16,})"`)
	// contextIDRE matches the offer's contextId/contextCode the addToCart call sends.
	contextIDRE = regexp.MustCompile(`"context(?:Id|Code)"\s*:\s*"([0-9A-Za-z]{2,8})"`)
	// refFromURLRE pulls the reflm from the "-<digits>.html" URL suffix.
	refFromURLRE = regexp.MustCompile(`-(\d+)\.html`)
)

// ProductOffer fetches a product page and extracts what `cart add` needs: the
// reflm (from the URL, falling back to the JSON-LD sku), the offerId, the offer's
// contextCode (empty if not found → caller uses the default), and the product
// detail (name/price) for display and the spending guard.
func (c *Client) ProductOffer(urlOrPath string) (reflm, offerID, contextCode string, detail *ProductDetail, err error) {
	html, err := c.GetHTML(urlOrPath)
	if err != nil {
		return "", "", "", nil, err
	}
	if m := refFromURLRE.FindStringSubmatch(urlOrPath); m != nil {
		reflm = m[1]
	}
	if m := offerIDRE.FindStringSubmatch(html); m != nil {
		offerID = m[1]
	}
	if m := contextIDRE.FindStringSubmatch(html); m != nil {
		contextCode = m[1]
	}
	if d, derr := parseProductDetail(html); derr == nil {
		d.URL = c.resolve(urlOrPath)
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

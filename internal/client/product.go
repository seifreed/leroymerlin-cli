package client

import (
	"fmt"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// Product fetches a product page (URL or site-relative path, as returned by
// Search) and distils its JSON-LD into a domain.ProductDetail.
func (c *Client) Product(urlOrPath string) (*domain.ProductDetail, error) {
	// Same guard as ProductOffer: a caller handing over an absolute URL must not
	// be able to point the CLI at a host that is not the configured storefront.
	if !c.sameOrigin(c.resolve(urlOrPath)) {
		return nil, fmt.Errorf("product URL must belong to the configured storefront")
	}
	html, err := c.GetHTML(urlOrPath)
	if err != nil {
		return nil, err
	}
	d, err := parseProductDetail(html)
	if err != nil {
		return nil, err
	}
	applyLivePrice(d, html, pageOfferID(html))
	d.URL = c.resolve(urlOrPath)
	d.Specs = parseSpecs(html)
	d.Deliveries = parseDeliveries(html)
	return d, nil
}

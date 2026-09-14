package client

import "strings"

// catalogMarkers are strong positives that appear only on a served search page
// with real results — not on a DataDome block/challenge page. DataDome's client
// JS (captcha-delivery) is injected on every legit page too, so a challenge can
// only be told apart by the ABSENCE of real catalog content, not the presence of
// a DataDome reference.
var catalogMarkers = []string{
	"cdl_products_list",      // the per-product embedded JSON blob
	"product-thumbnail-item", // a search result card
	"js-guidance-component",  // the product-list Svelte component
}

// CheckAuth reports whether reads currently get through to the real storefront
// (not a DataDome challenge) — in practice, whether the cached session is still
// being accepted, since the clearance expires. It fetches one cheap search page
// and looks for real catalog content.
func (c *Client) CheckAuth() (bool, error) {
	html, err := c.GetHTML("/search?q=test")
	if err != nil {
		return false, err
	}
	for _, m := range catalogMarkers {
		if strings.Contains(html, m) {
			return true, nil
		}
	}
	return false, nil
}

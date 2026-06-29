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
// (not a DataDome challenge). With a cookie loaded it verifies that cookie is
// accepted; anonymously it verifies the uTLS path is still clearing the WAF. It
// fetches one cheap search page and looks for real catalog content.
func CheckAuth(c *Client) (bool, error) {
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

// CheckAuth is also exposed as a method for symmetry with the rest of the API.
func (c *Client) CheckAuth() (bool, error) { return CheckAuth(c) }

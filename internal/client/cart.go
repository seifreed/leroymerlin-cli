package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// cartDataPath returns the header cart summary: total item quantity and the
// guest/order id. cartAddPath is the add-to-cart write the web app's "Añadir al
// carrito" button calls.
const (
	cartDataPath = "/header-cart-module/backend/rest/cart-data"
	cartAddPath  = "/cart/services/addToCart"
	// defaultContextCode is the constant the storefront's add-to-cart JS sends; it
	// is not present in the page, so it is mirrored verbatim from the captured call.
	defaultContextCode = "058"
)

// CartSummary is the cart-data response: the total item quantity and the
// guest/order id the cart is tied to. The endpoint does not return line items.
type CartSummary struct {
	Quantity       int    `json:"quantity"`
	Order          string `json:"order"`
	RedirectionURL string `json:"redirectionUrl"`
}

// CartData fetches the cart summary (item count + order id). A cart is tied to
// the browser session, so a meaningful result needs an imported cookie
// (LoadAuth); anonymously it reports an empty cart.
func (c *Client) CartData() (*CartSummary, error) {
	var s CartSummary
	if err := c.getJSON(cartDataPath, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// addToCartItem is one element of the addToCart POST body, mirroring the exact
// shape the web app sends.
type addToCartItem struct {
	Quantity          int    `json:"quantity"`
	Reflm             string `json:"reflm"`
	OfferID           string `json:"offerId"`
	ContextCode       string `json:"contextCode"`
	ATCButtonLocation string `json:"atcButtonLocation"`
}

// AddToCart adds qty of a product (by its reflm + offerId, both from the product
// page or a search result) to the session cart, then returns the updated summary.
// Requires an imported cookie — the cart is bound to the browser session.
func (c *Client) AddToCart(reflm, offerID string, qty int) (*CartSummary, error) {
	if qty <= 0 {
		qty = 1
	}
	body := []addToCartItem{{
		Quantity:          qty,
		Reflm:             reflm,
		OfferID:           offerID,
		ContextCode:       defaultContextCode,
		ATCButtonLocation: "main offer",
	}}
	if err := c.postJSON(cartAddPath, body, nil); err != nil {
		return nil, err
	}
	return c.CartData()
}

// getJSON issues a GET and decodes a JSON response, retrying transient throttling.
func (c *Client) getJSON(path string, out any) error {
	req, err := c.newJSONReq("GET", path, nil)
	if err != nil {
		return err
	}
	return c.doDecode(req, out)
}

// postJSON issues a POST with a JSON body (and the cookie + CSRF header cart
// writes need), decoding a JSON response into out when non-nil.
func (c *Client) postJSON(path string, body, out any) error {
	req, err := c.newJSONReq("POST", path, body)
	if err != nil {
		return err
	}
	return c.doDecode(req, out)
}

// newJSONReq builds a JSON request (GET or POST) carrying the browser-like
// headers, the cookie, and — for writes — the lm-csrf header derived from the
// cookie (the site's double-submit CSRF token).
func (c *Client) newJSONReq(method, path string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.resolve(path), r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", c.UserAgent)
	req.Header.Set("referer", c.BaseURL+"/")
	req.Header.Set("origin", c.BaseURL)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	if c.Cookie != "" {
		req.Header.Set("cookie", c.Cookie)
		// Leroy Merlin uses a double-submit CSRF token: the lm-csrf cookie value is
		// echoed back in a header on writes. Best-effort — sending it is harmless if
		// unneeded, and required if it is.
		if tok := cookieValue(c.Cookie, "lm-csrf"); tok != "" {
			req.Header.Set("lm-csrf", tok)
		}
	}
	return req, nil
}

// doDecode runs a request through the throttle-aware path and decodes a JSON body.
func (c *Client) doDecode(req *http.Request, out any) error {
	data, err := c.doWithRetry(req)
	if err != nil {
		return err
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s: %w", req.URL, err)
		}
	}
	return nil
}

// cookieValue extracts one cookie's value from a "n1=v1; n2=v2" header, or "".
func cookieValue(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, name+"="); ok {
			return v
		}
	}
	return ""
}

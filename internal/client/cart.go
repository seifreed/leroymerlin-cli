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

const (
	cartDetailPath = "/checkout/backend/cart"
	cartUpdatePath = "/checkout/backend/cart/update-offer-line-quantity/"
	cartDeletePath = "/checkout/backend/cart/delete-offer-line/"
)

// CartLine is one offer-line in the detailed cart. LineID is the UUID the
// update/delete endpoints address; Reflm is the product reference. Price is the
// line total (qty × unit), as the site reports it.
type CartLine struct {
	LineID   string  `json:"lineId"`
	Reflm    string  `json:"reflm"`
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// CartDetail is the full cart from /checkout/backend/cart: lines, the order
// resume totals, and the checkout-readiness flags.
type CartDetail struct {
	OrderID          string     `json:"orderId"`
	Quantity         int        `json:"quantity"` // total articles
	Lines            []CartLine `json:"lines"`
	TotalAmount      float64    `json:"totalAmount"`
	OffersAmount     float64    `json:"offersAmount"`
	DeliveryAmount   float64    `json:"deliveryAmount"`
	DisabledCheckout bool       `json:"disabledCheckout"`
	Blockers         []string   `json:"blockers"` // cannotBeValidatedReasons
}

// rawCart projects the fields we read from the detailed-cart response. Count
// fields are float64 because the API serialises them as JSON numbers with a
// decimal (e.g. offersQuantity: 0.0) that won't unmarshal into int.
type rawCart struct {
	OrderID                  string   `json:"orderId"`
	OffersQuantity           float64  `json:"offersQuantity"`
	DisabledCheckout         bool     `json:"disabledCheckout"`
	CannotBeValidatedReasons []string `json:"cannotBeValidatedReasons"`
	OrderResume              struct {
		TotalAmount    float64 `json:"totalAmount"`
		OffersAmount   float64 `json:"offersAmount"`
		DeliveryAmount float64 `json:"deliveryAmount"`
	} `json:"orderResume"`
	CartVendors []struct {
		CartVendorItems []struct {
			ID            string  `json:"id"`
			Quantity      float64 `json:"quantity"`
			DiscountPrice float64 `json:"discountPrice"`
			Offer         struct {
				RefLM string `json:"refLM"`
				Label string `json:"label"`
			} `json:"offer"`
		} `json:"cartVendorItems"`
	} `json:"cartVendors"`
}

// Cart fetches the detailed cart (lines + totals + checkout readiness). Requires
// an imported cookie — the cart endpoints are DataDome-protected.
func (c *Client) Cart() (*CartDetail, error) {
	var rc rawCart
	if err := c.getJSON(cartDetailPath, &rc); err != nil {
		return nil, err
	}
	d := &CartDetail{
		OrderID:          rc.OrderID,
		Quantity:         int(rc.OffersQuantity),
		TotalAmount:      rc.OrderResume.TotalAmount,
		OffersAmount:     rc.OrderResume.OffersAmount,
		DeliveryAmount:   rc.OrderResume.DeliveryAmount,
		DisabledCheckout: rc.DisabledCheckout,
		Blockers:         rc.CannotBeValidatedReasons,
	}
	for _, v := range rc.CartVendors {
		for _, it := range v.CartVendorItems {
			d.Lines = append(d.Lines, CartLine{
				LineID:   it.ID,
				Reflm:    it.Offer.RefLM,
				Name:     it.Offer.Label,
				Quantity: int(it.Quantity),
				Price:    it.DiscountPrice,
			})
		}
	}
	return d, nil
}

// SetLineQuantity sets a cart line's absolute quantity via the update endpoint.
func (c *Client) SetLineQuantity(lineID string, qty int) error {
	return c.putJSON(cartUpdatePath+lineID, map[string]int{"quantity": qty})
}

// DeleteLine removes a cart line entirely.
func (c *Client) DeleteLine(lineID string) error {
	req, err := c.newJSONReq("DELETE", cartDeletePath+lineID, nil)
	if err != nil {
		return err
	}
	return c.doDecode(req, nil)
}

// putJSON issues a PUT with a JSON body (cart line updates).
func (c *Client) putJSON(path string, body any) error {
	req, err := c.newJSONReq("PUT", path, body)
	if err != nil {
		return err
	}
	return c.doDecode(req, nil)
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
// contextCode is the offer's delivery context — empty falls back to the default
// the storefront uses. Requires an imported cookie (the cart is session-bound).
func (c *Client) AddToCart(reflm, offerID, contextCode string, qty int) (*CartSummary, error) {
	if qty <= 0 {
		qty = 1
	}
	if contextCode == "" {
		contextCode = defaultContextCode
	}
	body := []addToCartItem{{
		Quantity:          qty,
		Reflm:             reflm,
		OfferID:           offerID,
		ContextCode:       contextCode,
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
	// Broad accept like the web app's XHR — addToCart replies with text, not JSON,
	// and a strict application/json accept draws a 406 (even though the write lands).
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("accept-language", "es-ES,es;q=0.9,en;q=0.8")
	req.Header.Set("user-agent", c.UserAgent)
	req.Header.Set("referer", c.BaseURL+"/")
	req.Header.Set("origin", c.BaseURL)
	// Mirror the Client Hints + Fetch Metadata a real Chrome XHR sends — the cart
	// endpoints run a stricter DataDome rule that scores their absence as bot-like.
	req.Header.Set("sec-ch-ua", `"Google Chrome";v="149", "Chromium";v="149", "Not)A;Brand";v="24"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"macOS"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
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

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
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

// CartData fetches the cart summary (item count + order id). The cart belongs to
// the browser session, and the cart endpoints are scored more strictly than the
// rest: without a session they answer 403, not an empty cart. Their clearance
// also expires sooner, so a read that starts failing while search still works is
// the cue to lift the cookie again.
func (c *Client) CartData() (*domain.CartSummary, error) {
	var s domain.CartSummary
	if err := c.getJSON(cartDataPath, &s); err != nil {
		return nil, err
	}
	if s.Quantity < 0 {
		return nil, fmt.Errorf("invalid cart quantity %d", s.Quantity)
	}
	c.setCookieValue("Order.id", s.Order)
	return &s, nil
}

const (
	cartDetailPath = "/checkout/backend/cart"
	cartUpdatePath = "/checkout/backend/cart/update-offer-line-quantity/"
	cartDeletePath = "/checkout/backend/cart/delete-offer-line/"
)

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

// Cart fetches the detailed cart (lines + totals + checkout readiness) from
// /checkout/backend/cart. Requires an imported cookie — the cart endpoints are
// DataDome-protected. Each line carries the LineID the update and delete
// endpoints address, the product reference, and a Price that is the line total
// rather than the unit price.
func (c *Client) Cart() (*domain.CartDetail, error) {
	var rc rawCart
	if err := c.getJSON(cartDetailPath, &rc); err != nil {
		return nil, err
	}
	quantity, err := integerCount(rc.OffersQuantity, "offersQuantity")
	if err != nil {
		return nil, err
	}
	c.setCookieValue("Order.id", rc.OrderID)
	for i, reason := range rc.CannotBeValidatedReasons {
		rc.CannotBeValidatedReasons[i] = scrubText(reason)
	}
	d := &domain.CartDetail{
		OrderID:          scrubText(rc.OrderID),
		Quantity:         quantity,
		TotalAmount:      rc.OrderResume.TotalAmount,
		OffersAmount:     rc.OrderResume.OffersAmount,
		DeliveryAmount:   rc.OrderResume.DeliveryAmount,
		DisabledCheckout: rc.DisabledCheckout,
		Blockers:         rc.CannotBeValidatedReasons,
	}
	for _, v := range rc.CartVendors {
		for _, it := range v.CartVendorItems {
			quantity, err := integerCount(it.Quantity, "line quantity")
			if err != nil {
				return nil, err
			}
			d.Lines = append(d.Lines, domain.CartLine{
				LineID:   scrubText(it.ID),
				Reflm:    scrubText(it.Offer.RefLM),
				Name:     scrubText(it.Offer.Label),
				Quantity: quantity,
				Price:    it.DiscountPrice,
			})
		}
	}
	return d, nil
}

func integerCount(value float64, field string) (int, error) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value >= float64(math.MaxInt) {
		return 0, fmt.Errorf("invalid %s %v (want a non-negative integer)", field, value)
	}
	return int(value), nil
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
func (c *Client) AddToCart(reflm, offerID, contextCode string, qty int) (*domain.CartSummary, error) {
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
	// Baseline before the write: a discarded add leaves the count unchanged, and
	// on a cart that already holds items "unchanged" is not the same as "zero".
	before, err := c.CartData()
	if err != nil {
		return nil, err
	}
	if err := c.postJSON(cartAddPath, body, nil); err != nil {
		return nil, err
	}
	sum, err := c.CartData()
	if err != nil {
		return nil, err
	}
	if sum.Quantity > before.Quantity {
		return sum, nil
	}
	// Both cart projections can lag immediately after a write, so briefly poll the
	// detailed cart before reporting the add as discarded.
	for attempt := 0; attempt < 4; attempt++ {
		detail, derr := c.Cart()
		if derr != nil {
			return nil, derr
		}
		if detail.Quantity > before.Quantity {
			sum.Quantity = detail.Quantity
			if sum.Order == "" {
				sum.Order = detail.OrderID
			}
			return sum, nil
		}
		if attempt < 3 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	// The storefront answered 2xx and the cart did not grow, which is what a wrong
	// contextCode or a stale offerId looks like: the write is accepted and
	// discarded. Reporting that as a successful add is worse than useless to
	// anything reading the exit code.
	return nil, fmt.Errorf("the storefront accepted the add but the cart did not change — "+
		"the offer for %s may be stale; re-read the product page and retry", reflm)
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
	req.Header.Set("accept-language", acceptLanguage)
	req.Header.Set("user-agent", c.UserAgent)
	referer := c.BaseURL + "/"
	if strings.HasPrefix(path, "/checkout/") {
		referer = c.BaseURL + "/checkout/cart"
	}
	req.Header.Set("referer", referer)
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
	req.Header.Set("x-app-client-version", "v3.154.0")
	req.Header.Set("x-tab-id", c.TabID)
	req.Header.Set("place-type", "ONLINE")
	req.Header.Set("device-type", "DESKTOP")
	req.Header.Set("interface-type", "WEB")
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	c.refreshSessionHeaders(req)
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

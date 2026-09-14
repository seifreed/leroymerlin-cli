package client

import (
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// scrubText removes the control characters that must never survive a read. The
// storefront lists marketplace products whose name and description the seller
// writes, so remote text reaches a terminal: an ANSI escape in it would repaint
// the user's screen, retitle the window, or drive whatever else the emulator
// honours. It is also what keeps --toon usable — the TOON encoder rejects a
// control character outright, so one hostile listing would otherwise fail the
// whole command. Tabs and newlines become spaces; the rest are dropped.
func scrubText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		}
		return r
	}, s)
}

// scrubProduct cleans every free-text field of a search hit in place.
func scrubProduct(p *domain.Product) {
	p.Brand = scrubText(p.Brand)
	p.Identifier = scrubText(p.Identifier)
	p.Name = scrubText(p.Name)
	p.URL = scrubText(p.URL)
	p.Offer.SellerName = scrubText(p.Offer.SellerName)
	p.Offer.SellerType = scrubText(p.Offer.SellerType)
	p.Offer.OfferType = scrubText(p.Offer.OfferType)
	if p.Offer.Animations != nil {
		p.Offer.Animations.Label = scrubText(p.Offer.Animations.Label)
	}
}

// scrubProductDetail cleans every free-text field of a product page in place.
func scrubProductDetail(d *domain.ProductDetail) {
	d.Name = scrubText(d.Name)
	d.SKU = scrubText(d.SKU)
	d.GTIN = scrubText(d.GTIN)
	d.Description = scrubText(d.Description)
	d.Brand = scrubText(d.Brand)
	d.Image = scrubText(d.Image)
	for i := range d.Offers {
		d.Offers[i].Price = scrubText(d.Offers[i].Price)
		d.Offers[i].PriceCurrency = scrubText(d.Offers[i].PriceCurrency)
		d.Offers[i].Availability = scrubText(d.Offers[i].Availability)
		d.Offers[i].URL = scrubText(d.Offers[i].URL)
		d.Offers[i].ItemCondition = scrubText(d.Offers[i].ItemCondition)
	}
	if d.Rating != nil {
		d.Rating.Value = scrubText(d.Rating.Value)
		d.Rating.Count = scrubText(d.Rating.Count)
	}
}

// scrubAddresses cleans the free-form address map the checkout page returns.
// Only the string values are printed, so the rest is passed through untouched.
func scrubAddresses(in map[string]map[string]any) map[string]map[string]any {
	for _, address := range in {
		for key, value := range address {
			if s, ok := value.(string); ok {
				address[key] = scrubText(s)
			}
		}
	}
	return in
}

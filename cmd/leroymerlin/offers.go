package main

import "github.com/seifreed/leroymerlin-cli/internal/client"

// classifyOffer reports whether a product carries an actionable offer and a short
// label for it: "discount" with a was-price when initial_price beats the current
// price, else "promo" with the commercial-animation text (e.g. free shipping),
// else "", "". The was-price wins because it's the money-saving signal.
func classifyOffer(p client.Product) (kind, label string) {
	o := p.Offer
	if o.InitialPrice != nil && *o.InitialPrice > o.UnitPriceATI {
		return "discount", "antes " + eur(*o.InitialPrice)
	}
	if promo := o.Promo(); promo != "" {
		return "promo", promo
	}
	return "", ""
}

// onOffer reports whether a product has any actionable offer (discount or promo).
func onOffer(p client.Product) bool {
	kind, _ := classifyOffer(p)
	return kind != ""
}

// filterOnOffer keeps only products carrying an offer.
func filterOnOffer(in []client.Product) []client.Product {
	out := in[:0:0]
	for _, p := range in {
		if onOffer(p) {
			out = append(out, p)
		}
	}
	return out
}

package domain

import (
	"fmt"
	"sort"
	"strings"
)

// BrandPreferences describes the user's preferred brands without tying the
// selection policy to a configuration file format.
type BrandPreferences struct {
	Preferred []string
	Mode      string
	Overrides map[string][]string
}

// normalizeBrand canonicalises a brand for accent-insensitive comparison.
func normalizeBrand(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch r {
		case 'à', 'á', 'â', 'ä', 'a':
			b.WriteRune('a')
		case 'è', 'é', 'ê', 'ë':
			b.WriteRune('e')
		case 'í', 'ì', 'î', 'ï':
			b.WriteRune('i')
		case 'ò', 'ó', 'ô', 'ö':
			b.WriteRune('o')
		case 'ú', 'ù', 'û', 'ü':
			b.WriteRune('u')
		case 'ç':
			b.WriteRune('c')
		case 'ñ':
			b.WriteRune('n')
		default:
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// preferredFor returns the preference list for a term and whether it came from
// a matching term override.
func preferredFor(term string, prefs BrandPreferences) ([]string, bool) {
	nt := normalizeBrand(term)
	var matched []string
	matchedKey := ""
	for key, list := range prefs.Overrides {
		nk := normalizeBrand(key)
		if nk != "" && nt != "" && (strings.Contains(nt, nk) || strings.Contains(nk, nt)) {
			if len(nk) > len(matchedKey) || (len(nk) == len(matchedKey) && nk < matchedKey) {
				matched, matchedKey = list, nk
			}
		}
	}
	if matchedKey != "" {
		return matched, true
	}
	return prefs.Preferred, false
}

// pickByBrand chooses a preferred organic product, using the configured mode.
func pickByBrand(products []Product, preferences []string, mode string) (Product, bool) {
	wanted := make([]string, 0, len(preferences))
	for _, preference := range preferences {
		if normalized := normalizeBrand(preference); normalized != "" {
			wanted = append(wanted, normalized)
		}
	}
	if len(wanted) == 0 {
		return Product{}, false
	}
	if mode == "strict_priority" {
		for _, preferred := range wanted {
			if hits := brandMatches(products, []string{preferred}); len(hits) > 0 {
				return cheapestOf(hits), true
			}
		}
		return Product{}, false
	}
	hits := brandMatches(products, wanted)
	if len(hits) == 0 {
		return Product{}, false
	}
	return cheapestOf(hits), true
}

// CheapestHit picks the cheapest non-sponsored product that can be added to a
// cart, then falls back to sponsored in-stock and finally any product.
func CheapestHit(products []Product) (Product, bool) {
	if len(products) == 0 {
		return Product{}, false
	}
	if product, ok := cheapestMatching(products, func(p Product) bool { return p.Offer.AddToCart && !p.Sponsored }); ok {
		return product, true
	}
	if product, ok := cheapestMatching(products, func(p Product) bool { return p.Offer.AddToCart }); ok {
		return product, true
	}
	return cheapestMatching(products, func(Product) bool { return true })
}

// ResolveBrandHit chooses a term's product and returns the preference status.
func ResolveBrandHit(products []Product, term string, prefs BrandPreferences, explicit []string, disabled bool) (Product, string) {
	// CheapestHit reports no hit only for an empty result set, where the zero
	// Product is the honest answer — indexing would panic.
	fallback := func() Product {
		product, _ := CheapestHit(products)
		return product
	}
	if disabled {
		return fallback(), "off"
	}
	selected := explicit
	if len(selected) == 0 {
		if list, override := preferredFor(term, prefs); override {
			selected = list
		}
	}
	if len(selected) > 0 {
		if product, ok := pickByBrand(products, selected, prefs.Mode); ok {
			if len(explicit) > 0 {
				return product, "override"
			}
			return product, "preferred"
		}
		return fallback(), "none"
	}
	if len(prefs.Preferred) == 0 {
		return fallback(), "off"
	}
	if product, ok := pickByBrand(products, prefs.Preferred, prefs.Mode); ok {
		return product, "preferred"
	}
	// "none" — not "off" — for the same reason an explicit --brand reports it: the
	// preference was in play and could not be honoured, which is what makes the
	// CLI warn. "off" means no preference applied at all, and saying that here
	// would hide an unstocked favourite behind a silent fallback.
	return fallback(), "none"
}

// ClassifyOffer returns the offer kind and display label.
func ClassifyOffer(product Product) (kind, label string) {
	offer := product.Offer
	if offer.InitialPrice != nil && *offer.InitialPrice > offer.UnitPriceATI {
		return "discount", fmt.Sprintf("antes %.2f€", *offer.InitialPrice)
	}
	if promo := offer.Promo(); promo != "" {
		return "promo", promo
	}
	return "", ""
}

// OnOffer reports whether a product has an explicit discount or promotion.
func OnOffer(product Product) bool {
	kind, _ := ClassifyOffer(product)
	return kind != ""
}

// Filter keeps the products satisfying keep, in order.
func Filter(products []Product, keep func(Product) bool) []Product {
	filtered := make([]Product, 0, len(products))
	for _, product := range products {
		if keep(product) {
			filtered = append(filtered, product)
		}
	}
	return filtered
}

// FilterOnOffer keeps only products carrying an offer.
func FilterOnOffer(products []Product) []Product {
	return Filter(products, OnOffer)
}

// cheapestOf returns the lowest-price product, preserving the first on ties.
func cheapestOf(products []Product) Product {
	cheapest, _ := cheapestMatching(products, func(Product) bool { return true })
	return cheapest
}

// SortByPrice orders products from lowest to highest price, preserving ties.
func SortByPrice(products []Product) {
	sort.SliceStable(products, func(i, j int) bool {
		return products[i].Offer.UnitPriceATI < products[j].Offer.UnitPriceATI
	})
}

func brandMatches(products []Product, wanted []string) []Product {
	var matches []Product
	for _, product := range organic(products) {
		brand := normalizeBrand(product.Brand)
		for _, candidate := range wanted {
			if brand == candidate {
				matches = append(matches, product)
				break
			}
		}
	}
	return matches
}

func organic(products []Product) []Product {
	return Filter(products, func(p Product) bool { return !p.Sponsored })
}

func cheapestMatching(products []Product, matches func(Product) bool) (Product, bool) {
	var best Product
	found := false
	for _, product := range products {
		if matches(product) && (!found || product.Offer.UnitPriceATI < best.Offer.UnitPriceATI) {
			best, found = product, true
		}
	}
	return best, found
}

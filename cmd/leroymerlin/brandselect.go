package main

import (
	"sort"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/config"
)

// normBrand canonicalises a brand for comparison: lowercase, accents folded, and
// everything but letters/digits dropped — so "Würth" matches "wurth" and
// "Leroy Merlin" matches "leroymerlin". Small fold table covers Spanish/Catalan,
// avoiding an x/text dependency.
func normBrand(s string) string {
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

// preferredFor returns the brand list that applies to a search term: the first
// override whose key matches the term (normalized substring, either direction)
// wins; otherwise the global Preferred list. isOverride reports which it was.
func preferredFor(term string, b config.Brands) (brands []string, isOverride bool) {
	nt := normBrand(term)
	for key, list := range b.Overrides {
		nk := normBrand(key)
		// Both sides non-empty: strings.Contains(x, "") is always true, so without
		// the guard a symbol-only term would spuriously match the first override
		// visited (and map order is random → nondeterministic).
		if nk != "" && nt != "" && (strings.Contains(nt, nk) || strings.Contains(nk, nt)) {
			return list, true
		}
	}
	return b.Preferred, false
}

// pickByBrand chooses, among organic (non-sponsored) results, the cheapest
// product whose brand is in prefs (mode "cheapest_among"), or the cheapest of the
// highest-priority brand present (mode "strict_priority"). matched is false when
// no preferred brand is stocked.
func pickByBrand(prods []client.Product, prefs []string, mode string) (chosen client.Product, matched bool) {
	wanted := make([]string, 0, len(prefs))
	for _, p := range prefs {
		if n := normBrand(p); n != "" {
			wanted = append(wanted, n)
		}
	}
	if len(wanted) == 0 {
		return client.Product{}, false
	}
	if mode == "strict_priority" {
		for _, w := range wanted {
			if hits := brandMatches(prods, []string{w}); len(hits) > 0 {
				return cheapestOf(hits), true
			}
		}
		return client.Product{}, false
	}
	hits := brandMatches(prods, wanted)
	if len(hits) == 0 {
		return client.Product{}, false
	}
	return cheapestOf(hits), true
}

// organic returns the non-sponsored products, preserving order.
func organic(prods []client.Product) []client.Product {
	out := make([]client.Product, 0, len(prods))
	for _, p := range prods {
		if !p.Sponsored {
			out = append(out, p)
		}
	}
	return out
}

// brandMatches returns the organic products whose brand normalizes to one of the
// wanted (already-normalized) brands.
func brandMatches(prods []client.Product, wanted []string) []client.Product {
	var out []client.Product
	for _, p := range organic(prods) {
		nb := normBrand(p.Brand)
		for _, w := range wanted {
			if nb == w {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// cheapestOf returns the lowest-price product (ties → first, stable).
func cheapestOf(prods []client.Product) client.Product {
	sorted := append([]client.Product(nil), prods...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Offer.UnitPriceATI < sorted[j].Offer.UnitPriceATI
	})
	return sorted[0]
}

// resolveBrandHit picks the product for a term honouring brand preferences: a
// --brand override wins, else the configured brands (global or per-term override);
// with no preference (or --no-brands) it falls back to the cheapest in-stock hit.
// The tag (preferred|override|none|off) lets the caller flag a missed preference.
func resolveBrandHit(prods []client.Product, term string, brands config.Brands, override []string, disabled bool) (client.Product, string) {
	fallback := func() client.Product {
		if p, ok := cheapestHit(prods); ok {
			return p
		}
		return prods[0]
	}
	if disabled {
		return fallback(), "off"
	}

	explicit := override
	if len(explicit) == 0 {
		if list, isOv := preferredFor(term, brands); isOv {
			explicit = list
		}
	}
	if len(explicit) > 0 {
		if chosen, ok := pickByBrand(prods, explicit, brands.Mode); ok {
			tag := "preferred"
			if len(override) > 0 {
				tag = "override"
			}
			return chosen, tag
		}
		return fallback(), "none"
	}

	if len(brands.Preferred) == 0 {
		return fallback(), "off"
	}
	if chosen, ok := pickByBrand(prods, brands.Preferred, brands.Mode); ok {
		return chosen, "preferred"
	}
	return fallback(), "off"
}

// splitBrands parses a comma-separated --brand value into a trimmed list.
func splitBrands(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

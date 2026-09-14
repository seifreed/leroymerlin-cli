package application

import "github.com/seifreed/leroymerlin-cli/internal/domain"

// BatchHit is the resolved result for one natural-language shopping term.
type BatchHit struct {
	Term       string          `json:"term"`
	Product    *domain.Product `json:"product"`
	BrandMatch string          `json:"brandMatch,omitempty"`
	Offer      string          `json:"offer,omitempty"`
	OfferLabel string          `json:"offerLabel,omitempty"`
	// Relaxed marks a term the storefront could not match: it widened the query
	// and answered with something else, so this hit is a suggestion, not a find.
	Relaxed bool `json:"relaxed,omitempty"`
}

// ResolveBatch searches and resolves each term using the supplied brand policy.
func ResolveBatch(reader CatalogReader, terms []string, prefs domain.BrandPreferences, explicit []string, disabled, onOfferOnly bool) ([]BatchHit, int) {
	hits := make([]BatchHit, 0, len(terms))
	missing := 0
	for _, term := range terms {
		found, err := reader.Search(term, 0)
		if err != nil || len(found.Products) == 0 {
			hits = append(hits, BatchHit{Term: term})
			missing++
			continue
		}
		products := found.Products
		// Narrow the candidates before choosing, not the choice afterwards: a term
		// whose cheapest hit carries no discount can still have one that does, and
		// that is the product the user asked to see.
		if onOfferOnly {
			products = domain.FilterOnOffer(products)
			if len(products) == 0 {
				continue
			}
		}
		product, match := domain.ResolveBrandHit(products, term, prefs, explicit, disabled)
		kind, label := domain.ClassifyOffer(product)
		hits = append(hits, BatchHit{Term: term, Product: &product, BrandMatch: match, Offer: kind, OfferLabel: label, Relaxed: found.Relaxed})
	}
	return hits, missing
}

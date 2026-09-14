package application

import (
	"errors"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

type batchCatalog struct {
	products map[string][]domain.Product
	relaxed  bool
}

func (c batchCatalog) Search(term string, _ int) (domain.SearchResult, error) {
	return domain.SearchResult{Products: c.products[term], Relaxed: c.relaxed}, nil
}

func TestResolveBatchAppliesPreferencesAndTracksMissing(t *testing.T) {
	products := []domain.Product{
		{Identifier: "cheap", Brand: "Practyl", Offer: domain.Offer{UnitPriceATI: 5, AddToCart: true}},
		{Identifier: "preferred", Brand: "Bosch", Offer: domain.Offer{UnitPriceATI: 10, AddToCart: true}},
	}
	hits, missing := ResolveBatch(batchCatalog{products: map[string][]domain.Product{"taladro": products}}, []string{"taladro", "broca"}, domain.BrandPreferences{Preferred: []string{"Bosch"}}, nil, false, false)
	if missing != 1 || len(hits) != 2 || hits[0].Product == nil || hits[0].Product.Identifier != "preferred" || hits[0].BrandMatch != "preferred" {
		t.Fatalf("hits=%+v missing=%d", hits, missing)
	}
}

// --on-offer drops a term whose best product is not discounted, rather than
// listing it without an offer label.
func TestResolveBatchMarksTermsWithoutAnOffer(t *testing.T) {
	discounted := 30.0
	fake := &fakeCatalog{products: []domain.Product{
		{Identifier: "plain", Brand: "Bosch", Offer: domain.Offer{UnitPriceATI: 10, AddToCart: true}},
	}}

	hits, missing := ResolveBatch(fake, []string{"taladro"}, domain.BrandPreferences{}, nil, false, true)
	if len(hits) != 1 || hits[0].Product != nil || !hits[0].NoOffer {
		t.Fatalf("hits = %+v, want the term kept and marked as having no offer", hits)
	}
	if missing != 0 {
		t.Errorf("missing = %d; a filtered term is not a missing one", missing)
	}

	fake.products[0].Offer.InitialPrice = &discounted
	hits, _ = ResolveBatch(fake, []string{"taladro"}, domain.BrandPreferences{}, nil, false, true)
	if len(hits) != 1 || hits[0].Offer == "" {
		t.Fatalf("hits = %+v, want the discounted term kept with its offer kind", hits)
	}
}

// A search failure and an empty result set are both "missing", so a scripted
// basket sees one consistent signal.
func TestResolveBatchCountsFailuresAndEmptiesAsMissing(t *testing.T) {
	hits, missing := ResolveBatch(&fakeCatalog{err: errors.New("boom")},
		[]string{"a", "b"}, domain.BrandPreferences{}, nil, false, false)
	if missing != 2 || len(hits) != 2 {
		t.Fatalf("hits=%d missing=%d, want 2/2", len(hits), missing)
	}
	for _, h := range hits {
		if h.Product != nil {
			t.Errorf("term %q should carry no product", h.Term)
		}
	}
}

// A term the catalogue does not stock still resolves to a product, so the hit
// has to carry the storefront's verdict or the plan prices the wrong thing.
func TestResolveBatchMarksRelaxedTerms(t *testing.T) {
	catalog := batchCatalog{
		relaxed:  true,
		products: map[string][]domain.Product{"tirafondos": {{Identifier: "papel-pintado", Offer: domain.Offer{UnitPriceATI: 14.99, AddToCart: true}}}},
	}
	hits, missing := ResolveBatch(catalog, []string{"tirafondos"}, domain.BrandPreferences{}, nil, false, false)
	if missing != 0 || len(hits) != 1 || !hits[0].Relaxed {
		t.Fatalf("hits = %+v, missing = %d; want the relaxed term flagged", hits, missing)
	}
}

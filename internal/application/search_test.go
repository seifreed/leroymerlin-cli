package application

import (
	"errors"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

type fakeCatalog struct {
	products []domain.Product
	limit    int
	err      error
}

func (f *fakeCatalog) Search(_ string, limit int) ([]domain.Product, error) {
	f.limit = limit
	return append([]domain.Product(nil), f.products...), f.err
}

func TestSearchProductsAppliesPolicies(t *testing.T) {
	fake := &fakeCatalog{products: []domain.Product{
		{Identifier: "unavailable", Offer: domain.Offer{UnitPriceATI: 5}},
		{Identifier: "regular", Offer: domain.Offer{UnitPriceATI: 20, AddToCart: true}},
		{Identifier: "offer", Offer: domain.Offer{UnitPriceATI: 10, AddToCart: true, Animations: &struct {
			Label string `json:"label"`
		}{Label: "rebaja"}}},
	}}

	// The policies run after the fetch, so the catalog is asked for a whole page
	// (limit 0) and the truncation to 1 happens once they have.
	got, err := SearchProducts(fake, "taladro", SearchOptions{Limit: 1, InStock: true, OnOfferOnly: true, Cheapest: true})
	if err != nil || len(got) != 1 || got[0].Identifier != "offer" || fake.limit != 0 {
		t.Fatalf("products=%+v err=%v limit=%d", got, err, fake.limit)
	}
}

// Capping the fetch at limit used to answer the wrong question: against the real
// storefront `search --cheapest --limit 3 taladro` returned the three sponsored
// hits at 169, 218 and 219 EUR because those lead the page, while the three
// cheapest were 14.95, 20.99 and 23.49.
func TestSearchProductsRanksBeforeTruncating(t *testing.T) {
	fake := &fakeCatalog{products: []domain.Product{
		{Identifier: "sponsored-1", Offer: domain.Offer{UnitPriceATI: 169, AddToCart: true}},
		{Identifier: "sponsored-2", Offer: domain.Offer{UnitPriceATI: 219, AddToCart: true}},
		{Identifier: "cheap", Offer: domain.Offer{UnitPriceATI: 14.95, AddToCart: true}},
	}}

	got, err := SearchProducts(fake, "taladro", SearchOptions{Limit: 1, Cheapest: true})
	if err != nil || len(got) != 1 || got[0].Identifier != "cheap" {
		t.Fatalf("--cheapest --limit 1 = %+v, want the cheapest hit, err %v", got, err)
	}
}

// Same for a filter: asking for N in-stock hits must not stop looking after the
// first N products, most of which may be unavailable.
func TestSearchProductsFiltersBeforeTruncating(t *testing.T) {
	fake := &fakeCatalog{products: []domain.Product{
		{Identifier: "out-1"},
		{Identifier: "out-2"},
		{Identifier: "in-1", Offer: domain.Offer{UnitPriceATI: 5, AddToCart: true}},
		{Identifier: "in-2", Offer: domain.Offer{UnitPriceATI: 6, AddToCart: true}},
	}}

	got, err := SearchProducts(fake, "taladro", SearchOptions{Limit: 2, InStock: true})
	if err != nil || len(got) != 2 {
		t.Fatalf("--in-stock --limit 2 returned %d hits, want 2: %+v", len(got), got)
	}
}

// A limit past one page keeps paginating, so the catalog still gets it.
func TestSearchProductsPaginatesForALimitBeyondOnePage(t *testing.T) {
	fake := &fakeCatalog{}
	if _, err := SearchProducts(fake, "taladro", SearchOptions{Limit: 200, Cheapest: true}); err != nil {
		t.Fatal(err)
	}
	if fake.limit != 200 {
		t.Errorf("catalog asked for %d, want the full 200 so pagination still runs", fake.limit)
	}
}

func TestSearchProductsPropagatesTheCatalogFailure(t *testing.T) {
	if _, err := SearchProducts(&fakeCatalog{err: errors.New("boom")}, "taladro", SearchOptions{}); err == nil {
		t.Fatal("want the catalog error surfaced")
	}
}

// The on-offer filter and the in-stock filter compose: asking for both must not
// return a discounted item that cannot be bought.
func TestSearchProductsCombinesTheStockAndOfferFilters(t *testing.T) {
	initial := 30.0
	fake := &fakeCatalog{products: []domain.Product{
		{Identifier: "offer-oos", Offer: domain.Offer{UnitPriceATI: 10, InitialPrice: &initial}},
		{Identifier: "offer-ok", Offer: domain.Offer{UnitPriceATI: 20, InitialPrice: &initial, AddToCart: true}},
		{Identifier: "plain-ok", Offer: domain.Offer{UnitPriceATI: 5, AddToCart: true}},
	}}

	got, err := SearchProducts(fake, "taladro", SearchOptions{InStock: true, OnOfferOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Identifier != "offer-ok" {
		t.Fatalf("results = %+v, want only offer-ok", got)
	}
}

// The limit is applied after filtering and ranking, so --cheapest --limit 1
// returns the cheapest of the whole result set, not the cheapest of an
// arbitrary first page.
func TestSearchProductsTruncatesAfterRanking(t *testing.T) {
	fake := &fakeCatalog{products: []domain.Product{
		{Identifier: "expensive", Offer: domain.Offer{UnitPriceATI: 50, AddToCart: true}},
		{Identifier: "cheap", Offer: domain.Offer{UnitPriceATI: 2, AddToCart: true}},
		{Identifier: "mid", Offer: domain.Offer{UnitPriceATI: 20, AddToCart: true}},
	}}

	got, err := SearchProducts(fake, "taladro", SearchOptions{Cheapest: true, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Identifier != "cheap" {
		t.Fatalf("results = %+v, want the cheapest overall", got)
	}
}

type fakeCategories struct {
	products []domain.Product
	limit    int
	err      error
}

func (f *fakeCategories) CategoryProducts(_ string, limit int) ([]domain.Product, error) {
	f.limit = limit
	return append([]domain.Product(nil), f.products...), f.err
}

// categories --cheapest had the same defect search did: ranking the first N
// hits of a section answers the wrong question, and a section leads with the
// expensive listings just as a search page leads with the sponsored ones.
func TestListCategoryProductsRanksBeforeTruncating(t *testing.T) {
	fake := &fakeCategories{products: []domain.Product{
		{Identifier: "power-station", Offer: domain.Offer{UnitPriceATI: 549}},
		{Identifier: "allen-keys", Offer: domain.Offer{UnitPriceATI: 5.29}},
	}}

	got, err := ListCategoryProducts(fake, "herramientas", SearchOptions{Limit: 1, Cheapest: true})
	if err != nil || len(got) != 1 || got[0].Identifier != "allen-keys" {
		t.Fatalf("--cheapest --limit 1 = %+v, want the cheapest, err %v", got, err)
	}
	if fake.limit != 0 {
		t.Errorf("catalog asked for %d, want a whole page", fake.limit)
	}
}

func TestListCategoryProductsPropagatesTheCatalogFailure(t *testing.T) {
	if _, err := ListCategoryProducts(&fakeCategories{err: errors.New("boom")}, "x", SearchOptions{}); err == nil {
		t.Fatal("want the catalog error surfaced")
	}
}

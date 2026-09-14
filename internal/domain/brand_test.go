package domain

import "testing"

func TestTallyBrandsSkipsUnbrandedAndRanksByCount(t *testing.T) {
	products := []Product{
		{Brand: "Bosch", Offer: Offer{UnitPriceATI: 20}},
		{Brand: "  ", Offer: Offer{UnitPriceATI: 1}},
		{Brand: "Makita", Offer: Offer{UnitPriceATI: 30}},
		{Brand: "Bosch", Offer: Offer{UnitPriceATI: 10}},
	}

	got := TallyBrands(products)

	if len(got) != 2 {
		t.Fatalf("tallies = %d (%v), want 2 (unbranded skipped)", len(got), got)
	}
	if got[0].Brand != "Bosch" || got[0].Count != 2 {
		t.Fatalf("first tally = %+v, want Bosch with count 2", got[0])
	}
	if got[0].MinPrice != 10 {
		t.Fatalf("Bosch MinPrice = %v, want 10 (cheapest of the group)", got[0].MinPrice)
	}
}

func TestTallyBrandsBreaksCountTiesAlphabetically(t *testing.T) {
	products := []Product{
		{Brand: "Makita", Offer: Offer{UnitPriceATI: 5}},
		{Brand: "Bosch", Offer: Offer{UnitPriceATI: 9}},
	}

	got := TallyBrands(products)

	if len(got) != 2 || got[0].Brand != "Bosch" || got[1].Brand != "Makita" {
		t.Fatalf("tie order = %v, want Bosch before Makita", got)
	}
}

// A zero price must never win the MinPrice slot: unpriced offers are absent
// data, not free products.
func TestTallyBrandsIgnoresZeroPricesWhenTracking(t *testing.T) {
	products := []Product{
		{Brand: "Bosch", Offer: Offer{UnitPriceATI: 0}},
		{Brand: "Bosch", Offer: Offer{UnitPriceATI: 15}},
	}

	got := TallyBrands(products)

	if len(got) != 1 || got[0].MinPrice != 15 {
		t.Fatalf("MinPrice = %v, want 15 (zero price ignored)", got)
	}
}

func TestTallyBrandsReturnsEmptyForNoBrandedProducts(t *testing.T) {
	if got := TallyBrands(nil); len(got) != 0 {
		t.Fatalf("tallies = %v, want empty", got)
	}
}

package main

import (
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/config"
)

func brandProd(ref, brand string, price float64, inStock, sponsored bool) client.Product {
	p := client.Product{Identifier: ref, Name: ref, Brand: brand}
	p.Offer.UnitPriceATI = price
	p.Offer.AddToCart = inStock
	p.Sponsored = sponsored
	return p
}

func TestNormBrand(t *testing.T) {
	if normBrand("Würth") != "wurth" {
		t.Errorf("Würth → %q", normBrand("Würth"))
	}
	if normBrand("BLACK + DECKER") != normBrand("black  decker") {
		t.Error("punctuation/space should fold away")
	}
}

func TestResolveBrandHitOverride(t *testing.T) {
	prods := []client.Product{
		brandProd("a", "Practyl", 13.99, true, false),
		brandProd("b", "Bosch", 57.99, true, false),
		brandProd("c", "Bosch", 80.00, true, false),
	}
	// --brand Bosch → cheapest Bosch (b)
	got, tag := resolveBrandHit(prods, "taladro", config.Brands{}, []string{"Bosch"}, false)
	if got.Identifier != "b" || tag != "override" {
		t.Errorf("override pick = %q tag %q, want b/override", got.Identifier, tag)
	}
	// preferred brand not stocked → fall back to cheapest, tag none
	got2, tag2 := resolveBrandHit(prods, "taladro", config.Brands{}, []string{"Makita"}, false)
	if got2.Identifier != "a" || tag2 != "none" {
		t.Errorf("missing brand = %q tag %q, want a/none", got2.Identifier, tag2)
	}
	// no preference → cheapest in-stock, tag off
	got3, tag3 := resolveBrandHit(prods, "taladro", config.Brands{}, nil, false)
	if got3.Identifier != "a" || tag3 != "off" {
		t.Errorf("no-pref = %q tag %q, want a/off", got3.Identifier, tag3)
	}
}

func TestResolveBrandHitConfigPreferred(t *testing.T) {
	prods := []client.Product{
		brandProd("a", "Practyl", 13.99, true, false),
		brandProd("b", "Dexter", 20.99, true, false),
	}
	cfg := config.Brands{Preferred: []string{"Dexter"}}
	got, tag := resolveBrandHit(prods, "taladro", cfg, nil, false)
	if got.Identifier != "b" || tag != "preferred" {
		t.Errorf("config preferred = %q tag %q, want b/preferred", got.Identifier, tag)
	}
	// --no-brands ignores the preference
	got2, tag2 := resolveBrandHit(prods, "taladro", cfg, nil, true)
	if got2.Identifier != "a" || tag2 != "off" {
		t.Errorf("--no-brands = %q tag %q, want a/off", got2.Identifier, tag2)
	}
}

func TestPreferredForOverride(t *testing.T) {
	b := config.Brands{
		Preferred: []string{"Bosch"},
		Overrides: map[string][]string{"taladro": {"Makita"}},
	}
	list, isOv := preferredFor("taladro percutor", b)
	if !isOv || len(list) != 1 || list[0] != "Makita" {
		t.Errorf("override = %v (isOv %v)", list, isOv)
	}
	list2, isOv2 := preferredFor("silicona", b)
	if isOv2 || list2[0] != "Bosch" {
		t.Errorf("fallthrough = %v (isOv %v)", list2, isOv2)
	}
}

func TestTallyBrands(t *testing.T) {
	prods := []client.Product{
		brandProd("a", "Dexter", 20.99, true, false),
		brandProd("b", "Dexter", 15.00, true, false),
		brandProd("c", "Bosch", 57.99, true, false),
		brandProd("d", "", 1.00, true, false), // unbranded — skipped
	}
	got := tallyBrands(prods)
	if len(got) != 2 {
		t.Fatalf("want 2 brands, got %d: %+v", len(got), got)
	}
	if got[0].Brand != "Dexter" || got[0].Count != 2 || got[0].MinPrice != 15.00 {
		t.Errorf("top tally = %+v, want Dexter/2/15.00", got[0])
	}
}

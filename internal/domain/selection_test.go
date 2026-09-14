package domain

import "testing"

func TestPreferredForChoosesMostSpecificOverride(t *testing.T) {
	prefs := BrandPreferences{
		Preferred: []string{"Bosch"},
		Overrides: map[string][]string{
			"taladro":          {"Makita"},
			"taladro percutor": {"DeWalt"},
		},
	}

	got, override := preferredFor("taladro percutor", prefs)
	if !override || len(got) != 1 || got[0] != "DeWalt" {
		t.Fatalf("override = %v (isOverride %v), want DeWalt/true", got, override)
	}
}

func TestCheapestHitPrefersAddToCart(t *testing.T) {
	products := []Product{
		{Identifier: "out-of-stock", Offer: Offer{UnitPriceATI: 5, AddToCart: false}},
		{Identifier: "available", Offer: Offer{UnitPriceATI: 10, AddToCart: true}},
		{Identifier: "sponsored", Sponsored: true, Offer: Offer{UnitPriceATI: 1, AddToCart: true}},
	}

	got, ok := CheapestHit(products)
	if !ok || got.Identifier != "available" {
		t.Fatalf("cheapest hit = %q (ok %v), want available/true", got.Identifier, ok)
	}
}

func TestPickByBrandHonoursMode(t *testing.T) {
	products := []Product{
		{Identifier: "bosch-1", Brand: "Bosch", Offer: Offer{UnitPriceATI: 20}},
		{Identifier: "bosch-2", Brand: "Bosch", Offer: Offer{UnitPriceATI: 10}},
		{Identifier: "makita", Brand: "Makita", Offer: Offer{UnitPriceATI: 5}},
		{Identifier: "sponsored", Brand: "Bosch", Sponsored: true, Offer: Offer{UnitPriceATI: 1}},
	}

	got, ok := pickByBrand(products, []string{"Bosch", "Makita"}, "cheapest_among")
	if !ok || got.Identifier != "makita" {
		t.Fatalf("cheapest among = %q (ok %v), want makita/true", got.Identifier, ok)
	}
	got, ok = pickByBrand(products, []string{"Bosch", "Makita"}, "strict_priority")
	if !ok || got.Identifier != "bosch-2" {
		t.Fatalf("strict priority = %q (ok %v), want bosch-2/true", got.Identifier, ok)
	}
}

func TestClassifyOfferAndFilter(t *testing.T) {
	initial := 20.0
	promo := &struct {
		Label string `json:"label"`
	}{Label: "2ª unidad"}
	products := []Product{
		{Identifier: "discount", Offer: Offer{UnitPriceATI: 15, InitialPrice: &initial}},
		{Identifier: "promo", Offer: Offer{UnitPriceATI: 10, Animations: promo}},
		{Identifier: "regular", Offer: Offer{UnitPriceATI: 10}},
	}

	kind, label := ClassifyOffer(products[0])
	if kind != "discount" || label != "antes 20.00€" {
		t.Fatalf("discount = %q/%q", kind, label)
	}
	if kind, label = ClassifyOffer(products[1]); kind != "promo" || label != "2ª unidad" {
		t.Fatalf("promo = %q/%q", kind, label)
	}
	if got := FilterOnOffer(products); len(got) != 2 {
		t.Fatalf("offer filter = %d products, want 2", len(got))
	}
}

func TestCheapestOfPreservesTieOrder(t *testing.T) {
	products := []Product{
		{Identifier: "first", Offer: Offer{UnitPriceATI: 5}},
		{Identifier: "second", Offer: Offer{UnitPriceATI: 5}},
	}
	if got := cheapestOf(products); got.Identifier != "first" {
		t.Fatalf("tie winner = %q, want first", got.Identifier)
	}
}

func TestSortByPricePreservesTieOrder(t *testing.T) {
	products := []Product{
		{Identifier: "expensive", Offer: Offer{UnitPriceATI: 10}},
		{Identifier: "first", Offer: Offer{UnitPriceATI: 5}},
		{Identifier: "second", Offer: Offer{UnitPriceATI: 5}},
	}
	SortByPrice(products)
	if products[0].Identifier != "first" || products[1].Identifier != "second" {
		t.Fatalf("sorted ids = %q, %q; want first, second", products[0].Identifier, products[1].Identifier)
	}
}

func TestResolveBrandHitReportsWhyAProductWasChosen(t *testing.T) {
	products := []Product{
		{Identifier: "bosch", Brand: "Bosch", Offer: Offer{UnitPriceATI: 20, AddToCart: true}},
		{Identifier: "makita", Brand: "Makita", Offer: Offer{UnitPriceATI: 30, AddToCart: true}},
		{Identifier: "cheap", Brand: "Ryobi", Offer: Offer{UnitPriceATI: 5, AddToCart: true}},
	}
	prefs := BrandPreferences{
		Preferred: []string{"Bosch"},
		Overrides: map[string][]string{"taladro": {"Makita"}},
		Mode:      "strict_priority",
	}

	for _, tc := range []struct {
		name       string
		term       string
		explicit   []string
		disabled   bool
		wantID     string
		wantStatus string
	}{
		{"disabled falls back to cheapest", "taladro", nil, true, "cheap", "off"},
		{"explicit brand wins", "taladro", []string{"Bosch"}, false, "bosch", "override"},
		{"term override applies", "taladro", nil, false, "makita", "preferred"},
		{"global preference applies", "sierra", nil, false, "bosch", "preferred"},
		{"unmatched explicit brand reports none", "taladro", []string{"Festool"}, false, "cheap", "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, status := ResolveBrandHit(products, tc.term, prefs, tc.explicit, tc.disabled)
			if got.Identifier != tc.wantID || status != tc.wantStatus {
				t.Fatalf("resolved %q/%q, want %q/%q", got.Identifier, status, tc.wantID, tc.wantStatus)
			}
		})
	}
}

func TestResolveBrandHitWithoutPreferencesReportsOff(t *testing.T) {
	products := []Product{
		{Identifier: "only", Brand: "Bosch", Offer: Offer{UnitPriceATI: 7, AddToCart: true}},
	}

	got, status := ResolveBrandHit(products, "taladro", BrandPreferences{}, nil, false)
	if got.Identifier != "only" || status != "off" {
		t.Fatalf("resolved %q/%q, want only/off", got.Identifier, status)
	}
}

// Brand names arrive from the storefront with inconsistent accents, case and
// punctuation; normalization is what makes "Saint-Gobain" and "saint gobain"
// compare equal.
func TestNormalizeBrandFoldsAccentsCaseAndPunctuation(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Bosch", "bosch"},
		{"  BOSCH  ", "bosch"},
		{"Saint-Gobain", "saintgobain"},
		{"saint gobain", "saintgobain"},
		{"Créatif", "creatif"},
		{"Ròdano Ôrbita Ösram", "rodanoorbitaosram"},
		{"Bôsch Ìtalia", "boschitalia"},
		{"Muñoz", "munoz"},
		{"Açaí", "acai"},
		{"Grohe 3000", "grohe3000"},
		{"", ""},
		{"!!!", ""},
		{"Über", "uber"},
	} {
		if got := normalizeBrand(tc.in); got != tc.want {
			t.Errorf("normalizeBrand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCheapestHitFallbackOrder(t *testing.T) {
	t.Run("no products", func(t *testing.T) {
		if _, ok := CheapestHit(nil); ok {
			t.Error("empty input should report no hit")
		}
	})

	t.Run("falls back to sponsored when nothing organic is buyable", func(t *testing.T) {
		products := []Product{
			{Identifier: "organic-oos", Offer: Offer{UnitPriceATI: 1}},
			{Identifier: "sponsored-ok", Sponsored: true, Offer: Offer{UnitPriceATI: 9, AddToCart: true}},
		}
		got, ok := CheapestHit(products)
		if !ok || got.Identifier != "sponsored-ok" {
			t.Fatalf("got %q (ok %v), want sponsored-ok", got.Identifier, ok)
		}
	})

	t.Run("falls back to any product when none are buyable", func(t *testing.T) {
		products := []Product{
			{Identifier: "expensive", Offer: Offer{UnitPriceATI: 50}},
			{Identifier: "cheap", Offer: Offer{UnitPriceATI: 2}},
		}
		got, ok := CheapestHit(products)
		if !ok || got.Identifier != "cheap" {
			t.Fatalf("got %q (ok %v), want cheap", got.Identifier, ok)
		}
	})
}

func TestPickByBrandReportsNoMatch(t *testing.T) {
	products := []Product{
		{Identifier: "bosch", Brand: "Bosch", Offer: Offer{UnitPriceATI: 10}},
	}

	// No usable brand names at all.
	if _, ok := pickByBrand(products, []string{"", "  "}, "cheapest_among"); ok {
		t.Error("blank brand names should match nothing")
	}
	// A real name that nothing carries, in both modes.
	for _, mode := range []string{"cheapest_among", "strict_priority"} {
		if _, ok := pickByBrand(products, []string{"Festool"}, mode); ok {
			t.Errorf("%s: an absent brand should match nothing", mode)
		}
	}
}

// When nothing can be added to a cart, CheapestHit reports no hit, and
// ResolveBrandHit still has to return a product — it falls back to the first.
func TestResolveBrandHitFallsBackToTheFirstProduct(t *testing.T) {
	products := []Product{
		{Identifier: "first", Sponsored: true},
		{Identifier: "second", Sponsored: true},
	}

	got, status := ResolveBrandHit(products, "taladro", BrandPreferences{}, nil, true)
	if got.Identifier != "first" || status != "off" {
		t.Fatalf("resolved %q/%q, want first/off", got.Identifier, status)
	}
}

// A preferred brand the result set does not carry falls back rather than
// returning nothing, and says so: "none" is what makes the CLI warn, and a
// configured favourite going unstocked deserves the same warning an explicit
// --brand gets.
func TestResolveBrandHitFallsBackWhenThePreferenceIsAbsent(t *testing.T) {
	products := []Product{
		{Identifier: "ryobi", Brand: "Ryobi", Offer: Offer{UnitPriceATI: 5, AddToCart: true}},
	}
	prefs := BrandPreferences{Preferred: []string{"Festool"}, Mode: "cheapest_among"}

	got, status := ResolveBrandHit(products, "taladro", prefs, nil, false)
	if got.Identifier != "ryobi" || status != "none" {
		t.Fatalf("resolved %q/%q, want ryobi/none", got.Identifier, status)
	}
}

// An empty result set must yield the zero Product, not a panic. ResolveBatch
// filters empties today, but the fallback must not depend on that.
func TestResolveBrandHitOnAnEmptyProductList(t *testing.T) {
	got, status := ResolveBrandHit(nil, "taladro", BrandPreferences{Preferred: []string{"Bosch"}}, nil, false)
	if got.Identifier != "" || status != "none" {
		t.Fatalf("resolved %+v/%q, want the zero product and none", got, status)
	}
}

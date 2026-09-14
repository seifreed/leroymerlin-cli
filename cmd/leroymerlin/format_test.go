package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

func TestDeliveryLabelFallsBackToTheRawType(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"storeDelivery", "recogida en tienda:"},
		{"homeDelivery", "envío a domicilio:"},
		{"relayDelivery", "punto de recogida:"},
		{"somethingNew", "somethingNew:"},
	} {
		if got := deliveryLabel(tc.in); got != tc.want {
			t.Errorf("deliveryLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLeadTime(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"2 HOUR", "en 2 h"},
		{"1 OPENING_DAY", "en 1 día lab."},
		{"48 OPENING_DAY", "en 48 día lab."},
		{"", ""},
		{"UNKNOWN_UNIT", "UNKNOWN_UNIT"},
	} {
		if got := leadTime(tc.in); got != tc.want {
			t.Errorf("leadTime(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// truncateLine collapses whitespace before measuring, and must cut on runes so
// accented product names never come back as invalid UTF-8.
func TestTruncateLineCollapsesWhitespaceAndCutsOnRunes(t *testing.T) {
	if got := truncateLine("  taladro   percutor\tBosch ", 40); got != "taladro percutor Bosch" {
		t.Errorf("collapse = %q", got)
	}
	got := truncateLine("ñañañaña", 4)
	if got != "ñaña…" {
		t.Errorf("truncateLine = %q, want ñaña…", got)
	}
	if !strings.ContainsRune(got, 'ñ') {
		t.Errorf("truncation split a multibyte rune: %q", got)
	}
	if got := truncateLine("corto", 40); got != "corto" {
		t.Errorf("short input changed: %q", got)
	}
}

func TestFmtQtyDropsTrailingZeroOnWholeQuantities(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{{3, "3"}, {0, "0"}, {1.5, "1.5"}, {0.25, "0.25"}} {
		if got := fmtQty(tc.in); got != tc.want {
			t.Errorf("fmtQty(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "línea", "líneas"); got != "1 línea" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(3, "línea", "líneas"); got != "3 líneas" {
		t.Errorf("plural(3) = %q", got)
	}
	if got := plural(0, "línea", "líneas"); got != "0 líneas" {
		t.Errorf("plural(0) = %q", got)
	}
}

func TestShortDateLeavesUnparseableValuesAlone(t *testing.T) {
	if got := shortDate("2026-09-13T18:45:00Z"); got != "2026-09-13 18:45" {
		t.Errorf("shortDate = %q", got)
	}
	for _, short := range []string{"", "2026-09-13"} {
		if got := shortDate(short); got != short {
			t.Errorf("shortDate(%q) = %q, want it unchanged", short, got)
		}
	}
}

func TestHumanBlockerHumanisesUnknownCodes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER", "needs an account (log in)"},
		{"SIMULATION_NEEDS_DELIVERY_ADDRESS", "needs delivery address"},
		{"SOMETHING_ELSE", "needs something else"},
	} {
		if got := humanBlocker(tc.in); got != tc.want {
			t.Errorf("humanBlocker(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// cleanCartErr swaps the two storefront statuses that have a known operator fix
// for actionable advice, and passes everything else through untouched.
func TestCleanCartErrExplainsRecoverableStatuses(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{{403, "login --from-browser"}, {412, "refresh it, then retry"}} {
		got := cleanCartErr(&client.APIError{Status: tc.status})
		if !strings.Contains(got.Error(), tc.want) {
			t.Errorf("HTTP %d advice = %q, want it to mention %q", tc.status, got, tc.want)
		}
	}

	original := fmt.Errorf("connection reset")
	if got := cleanCartErr(original); got != original {
		t.Errorf("non-HTTP error was rewritten: %v", got)
	}
	if got := cleanCartErr(&client.APIError{Status: 500}); !strings.Contains(got.Error(), "500") {
		t.Errorf("unrelated status lost its detail: %v", got)
	}
}

func promoOffer(label string) *struct {
	Label string `json:"label"`
} {
	return &struct {
		Label string `json:"label"`
	}{Label: label}
}

// productLine appends a badge per condition; each one is opt-in, so a plain
// in-stock first-party product must come back with none of them.
func TestProductLineShowsOnlyTheApplicableBadges(t *testing.T) {
	plain := domain.Product{Identifier: "123", Name: " Taladro ", Offer: domain.Offer{UnitPriceATI: 13.99, AddToCart: true}}

	got := productLine(plain)
	if got != "[123] Taladro — 13.99€" {
		t.Fatalf("plain line = %q", got)
	}
	for _, badge := range []string{"antes", "★", "patrocinado", "no disponible", "⟨"} {
		if strings.Contains(got, badge) {
			t.Errorf("plain line should not carry %q: %q", badge, got)
		}
	}
}

func TestProductLineBadges(t *testing.T) {
	before := 20.0
	cheaper := 5.0

	for _, tc := range []struct {
		name    string
		mutate  func(*domain.Product)
		want    string
		notWant string
	}{
		{"discount", func(p *domain.Product) { p.Offer.InitialPrice = &before }, "(antes 20.00€)", ""},
		{"a higher current price is not a discount", func(p *domain.Product) { p.Offer.InitialPrice = &cheaper }, "", "antes"},
		{"rating", func(p *domain.Product) { p.Rating = 4.5 }, "(4.5★)", ""},
		{"zero rating hidden", func(p *domain.Product) { p.Rating = 0 }, "", "★"},
		{"sponsored", func(p *domain.Product) { p.Sponsored = true }, "(patrocinado)", ""},
		{"named marketplace seller", func(p *domain.Product) {
			p.Offer.SellerType, p.Offer.SellerName = "3P", "AcmeTools"
		}, "[AcmeTools]", ""},
		{"unnamed marketplace seller", func(p *domain.Product) { p.Offer.SellerType = "3P" }, "[marketplace]", ""},
		{"out of stock", func(p *domain.Product) { p.Offer.AddToCart = false }, "[no disponible]", ""},
		{"promo", func(p *domain.Product) { p.Offer.Animations = promoOffer("2x1") }, "⟨2x1⟩", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := domain.Product{Identifier: "1", Name: "X", Offer: domain.Offer{UnitPriceATI: 10, AddToCart: true}}
			tc.mutate(&p)
			got := productLine(p)
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("line = %q, want it to contain %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("line = %q, should not contain %q", got, tc.notWant)
			}
		})
	}
}

// detailLines omits every optional row rather than printing an empty label.
func TestDetailLinesOmitsAbsentFields(t *testing.T) {
	got := detailLines(&domain.ProductDetail{Name: "Taladro", SKU: "123", URL: "/p/1"})

	if !strings.Contains(got, "ref:") || !strings.Contains(got, "/p/1") {
		t.Fatalf("required rows missing:\n%s", got)
	}
	for _, absent := range []string{"marca:", "precio:", "disponible:", "valoración:", "gtin:", "características:", "disponibilidad:"} {
		if strings.Contains(got, absent) {
			t.Errorf("absent field %q was printed:\n%s", absent, got)
		}
	}
}

func TestDetailLinesRendersEveryPresentField(t *testing.T) {
	got := detailLines(&domain.ProductDetail{
		Name:        "Taladro",
		Brand:       "Bosch",
		SKU:         "123",
		GTIN:        "84001",
		Description: "  un   taladro  ",
		Offers:      []domain.ProductOffer{{Price: "13.99", PriceCurrency: "EUR", Availability: "https://schema.org/InStock"}},
		Rating:      &domain.ProductRating{Value: "4.5", Count: "12"},
		Specs:       []domain.Spec{{Label: "Potencia", Value: "700W"}},
		Deliveries: []domain.DeliveryOption{
			{Type: "storeDelivery", Stock: 3, Price: 0, Time: "2 HOUR"},
			{Type: "homeDelivery", Stock: 1, Price: 3.9, Time: "1 OPENING_DAY"},
		},
		URL: "/p/1",
	})

	for _, want := range []string{
		"Bosch", "84001", "13.99 EUR", "InStock", "4.5 (12 reseñas)",
		"un taladro", "Potencia: 700W",
		"recogida en tienda:", "gratis", "en 2 h",
		"envío a domicilio:", "3.90€", "en 1 día lab.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

// The storefront sends the raw average rating, so the human view rounds it to
// the one decimal the search listing already shows. A non-numeric value is the
// site telling us something we should not reinterpret.
func TestRatingValueRoundsTheRawAverage(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"4.5721393034825875", "4.6"},
		{"4.5", "4.5"},
		{"5", "5.0"},
		{" 3.94 ", "3.9"},
		{"", ""},
		{"muy bueno", "muy bueno"},
	} {
		if got := ratingValue(tc.in); got != tc.want {
			t.Errorf("ratingValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	got := detailLines(&domain.ProductDetail{
		Name:   "Taladro",
		Rating: &domain.ProductRating{Value: "4.5721393034825875", Count: "201"},
	})
	if !strings.Contains(got, "4.6 (201 reseñas)") {
		t.Errorf("detail rating not rounded:\n%s", got)
	}
}

// emitTOON must report an unencodable value rather than print nothing and
// return success.
func TestEmitTOONReportsAnUnencodableValue(t *testing.T) {
	if err := emitTOON(make(chan int)); err == nil {
		t.Fatal("a channel is not encodable; want an error")
	}
	if _, err := emitStructured(&common{toon: true}, make(chan int)); err == nil {
		t.Fatal("emitStructured should surface the TOON failure")
	}
}

// The page's structured data says OutOfStock for a product with 119 units in
// store, so the per-channel stock is what the availability line reports —
// and the seller of a marketplace offer is named, since it ships on its
// own terms.
func TestDetailLinesPrefersChannelStockAndNamesTheSeller(t *testing.T) {
	d := &domain.ProductDetail{
		Name: "Cinta", SKU: "1",
		Offers:     []domain.ProductOffer{{Price: "0.59", Availability: "http://schema.org/OutOfStock"}},
		Seller:     "HOGARCONECTADO",
		SellerType: "3P",
		Deliveries: []domain.DeliveryOption{{Type: "storeDelivery", Stock: 119}},
	}
	out := detailLines(d)
	if !strings.Contains(out, "disponible:   InStock (stock por canal)") {
		t.Errorf("output = %q, want the channel stock to decide availability", out)
	}
	if !strings.Contains(out, "vendedor:     HOGARCONECTADO (marketplace") {
		t.Errorf("output = %q, want the marketplace seller named", out)
	}

	empty := &domain.ProductDetail{
		Name: "Taladro", SKU: "2",
		Offers:     []domain.ProductOffer{{Price: "219", Availability: "http://schema.org/InStock"}},
		Seller:     "Leroy Merlin",
		SellerType: "1P",
		Deliveries: []domain.DeliveryOption{{Type: "storeDelivery", Stock: 0}},
	}
	out = detailLines(empty)
	if !strings.Contains(out, "disponible:   OutOfStock (stock por canal)") {
		t.Errorf("output = %q, want an empty channel to read OutOfStock", out)
	}
	if strings.Contains(out, "marketplace") {
		t.Errorf("output = %q, want no marketplace note for a 1P seller", out)
	}
}

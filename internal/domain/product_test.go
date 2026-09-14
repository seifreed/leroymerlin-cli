package domain

import "testing"

func TestProductDetailAccessorsReadFirstOffer(t *testing.T) {
	detail := ProductDetail{Offers: []ProductOffer{
		{Price: "19.95", PriceCurrency: "EUR", Availability: "https://schema.org/InStock"},
		{Price: "99.00", PriceCurrency: "USD", Availability: "https://schema.org/OutOfStock"},
	}}

	if got := detail.Price(); got != "19.95" {
		t.Fatalf("Price() = %q, want 19.95", got)
	}
	if got := detail.Currency(); got != "EUR" {
		t.Fatalf("Currency() = %q, want EUR", got)
	}
	if got := detail.Availability(); got != "InStock" {
		t.Fatalf("Availability() = %q, want the short schema.org token", got)
	}
}

// A product page without offers is common (delisted items); the accessors must
// report empty rather than panic on the missing slice entry.
func TestProductDetailAccessorsTolerateNoOffers(t *testing.T) {
	var detail ProductDetail

	if got := detail.Price(); got != "" {
		t.Fatalf("Price() = %q, want empty", got)
	}
	if got := detail.Currency(); got != "" {
		t.Fatalf("Currency() = %q, want empty", got)
	}
	if got := detail.Availability(); got != "" {
		t.Fatalf("Availability() = %q, want empty", got)
	}
}

func TestAvailabilityPassesThroughUnprefixedValue(t *testing.T) {
	detail := ProductDetail{Offers: []ProductOffer{{Availability: "InStock"}}}

	if got := detail.Availability(); got != "InStock" {
		t.Fatalf("Availability() = %q, want InStock", got)
	}
}

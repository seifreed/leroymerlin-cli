package main

import (
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

func offerProd(price float64, initial *float64, promo string) client.Product {
	p := client.Product{Identifier: "x"}
	p.Offer.UnitPriceATI = price
	p.Offer.InitialPrice = initial
	if promo != "" {
		p.Offer.Animations = &struct {
			Label string `json:"label"`
		}{Label: promo}
	}
	return p
}

func f(v float64) *float64 { return &v }

func TestClassifyOffer(t *testing.T) {
	// was-price discount wins over a promo label
	k, l := classifyOffer(offerProd(10, f(15), "Envío gratis"))
	if k != "discount" || l != "antes 15.00€" {
		t.Errorf("discount: %q / %q", k, l)
	}
	// promo only
	k, l = classifyOffer(offerProd(10, nil, "Envío gratis +29€"))
	if k != "promo" || l != "Envío gratis +29€" {
		t.Errorf("promo: %q / %q", k, l)
	}
	// initial_price not above current → no discount
	k, _ = classifyOffer(offerProd(10, f(10), ""))
	if k != "" {
		t.Errorf("equal initial price should not be a discount, got %q", k)
	}
	// nothing
	if k, _ := classifyOffer(offerProd(10, nil, "")); k != "" {
		t.Errorf("no offer should be empty, got %q", k)
	}
}

func TestFilterOnOffer(t *testing.T) {
	in := []client.Product{
		offerProd(10, f(20), ""),   // discount
		offerProd(5, nil, ""),      // none
		offerProd(8, nil, "promo"), // promo
	}
	out := filterOnOffer(in)
	if len(out) != 2 {
		t.Fatalf("want 2 on-offer, got %d", len(out))
	}
}

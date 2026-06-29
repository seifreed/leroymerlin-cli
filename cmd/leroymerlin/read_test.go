package main

import (
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

func TestParseBasketLine(t *testing.T) {
	cases := []struct {
		in      string
		wantRef string
		wantQty float64
		wantErr bool
	}{
		{"martillo", "martillo", 1, false},
		{"martillo 2", "martillo", 2, false},
		{"cinta métrica 3", "cinta métrica", 3, false},               // term with a space + qty
		{"/productos/x-42.html 5", "/productos/x-42.html", 5, false}, // url + qty
		{"/productos/x-42.html", "/productos/x-42.html", 1, false},   // bare url
		{"taladro percutor", "taladro percutor", 1, false},           // trailing word, not a number
		{"martillo -1", "", 0, true},                                 // negative qty
	}
	for _, c := range cases {
		bl, err := parseBasketLine(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: want error, got %+v", c.in, bl)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
			continue
		}
		if bl.ref != c.wantRef || bl.qty != c.wantQty {
			t.Errorf("%q: got {%q, %v}, want {%q, %v}", c.in, bl.ref, bl.qty, c.wantRef, c.wantQty)
		}
	}
}

func TestPriceCentsAndStr(t *testing.T) {
	c, err := priceCents("13.99")
	if err != nil || c != 1399 {
		t.Fatalf("priceCents = %d, %v", c, err)
	}
	if got := centsStr(lineCents(1399, 3)); got != "41.97" {
		t.Errorf("3×13.99 = %s, want 41.97", got)
	}
	if got := centsStr(eurosToCents(1.58 * 1)); got != "1.58" {
		t.Errorf("eurosToCents drift: %s", got)
	}
	if _, err := priceCents("abc"); err == nil {
		t.Error("want error on unparseable price")
	}
}

func prod(ref string, price float64, inStock, sponsored bool) client.Product {
	p := client.Product{Identifier: ref, Name: ref}
	p.Offer.UnitPriceATI = price
	p.Offer.AddToCart = inStock
	p.Sponsored = sponsored
	return p
}

func TestCheapestHit(t *testing.T) {
	if _, ok := cheapestHit(nil); ok {
		t.Error("empty slice should return ok=false")
	}
	// cheapest in-stock non-sponsored wins over a cheaper sponsored one
	ps := []client.Product{
		prod("a", 5.00, true, false),
		prod("b", 2.00, true, true), // cheaper but sponsored — skipped
		prod("c", 3.00, true, false),
	}
	got, ok := cheapestHit(ps)
	if !ok || got.Identifier != "c" {
		t.Errorf("got %q, want c (cheapest organic in-stock)", got.Identifier)
	}
	// all sponsored → fall back to cheapest in-stock
	ps2 := []client.Product{prod("a", 5, true, true), prod("b", 2, true, true)}
	got2, _ := cheapestHit(ps2)
	if got2.Identifier != "b" {
		t.Errorf("all-sponsored fallback: got %q, want b", got2.Identifier)
	}
	// none in stock → cheapest of any
	ps3 := []client.Product{prod("a", 9, false, false), prod("b", 4, false, false)}
	got3, _ := cheapestHit(ps3)
	if got3.Identifier != "b" {
		t.Errorf("no-stock fallback: got %q, want b", got3.Identifier)
	}
}

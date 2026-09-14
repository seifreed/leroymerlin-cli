package application

import (
	"errors"
	"math"
	"testing"
)

func TestPriceBasketSumsSuccessesAndKeepsFailures(t *testing.T) {
	wanted := errors.New("missing")
	result := PriceBasket([]BasketLine{{Ref: "taladro", Quantity: 2}, {Ref: "broca", Quantity: 1}}, func(ref string) (PriceQuote, error) {
		if ref == "broca" {
			return PriceQuote{}, wanted
		}
		return PriceQuote{Name: "Taladro", DisplayPrice: "13.99", UnitCents: 1399}, nil
	})

	if result.TotalCents != 2798 || result.Failed != 1 || len(result.Lines) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if !errors.Is(result.Lines[1].Err, wanted) {
		t.Fatalf("line error = %v, want %v", result.Lines[1].Err, wanted)
	}
}

func TestPriceBasketRejectsSubtotalOverflow(t *testing.T) {
	result := PriceBasket([]BasketLine{{Ref: "excess", Quantity: math.MaxFloat64}}, func(string) (PriceQuote, error) {
		return PriceQuote{UnitCents: 100}, nil
	})
	if result.Failed != 1 || result.TotalCents != 0 || result.Lines[0].Err == nil {
		t.Fatalf("overflow result = %+v", result)
	}
}

func TestPriceBasketRejectsTotalOverflow(t *testing.T) {
	unit := int64(1) << 62
	result := PriceBasket([]BasketLine{{Ref: "a", Quantity: 1}, {Ref: "b", Quantity: 1}}, func(string) (PriceQuote, error) {
		return PriceQuote{UnitCents: unit}, nil
	})
	if result.Failed != 1 || result.TotalCents != unit || result.Lines[1].Err == nil {
		t.Fatalf("total overflow result = %+v", result)
	}
}

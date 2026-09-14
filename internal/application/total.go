package application

import (
	"fmt"
	"math"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// BasketLine is a user-requested reference and quantity.
type BasketLine struct {
	Ref      string
	Quantity float64
}

// PriceQuote is the transport-independent result of pricing one reference.
type PriceQuote struct {
	Name         string
	DisplayPrice string
	UnitCents    int64
}

// PricedLine contains either a quote and subtotal or the error for that line.
type PricedLine struct {
	Line          BasketLine
	Quote         PriceQuote
	SubtotalCents int64
	Err           error
}

// BasketTotal is the complete pricing result, including partial failures.
type BasketTotal struct {
	Lines      []PricedLine
	TotalCents int64
	Failed     int
}

// PriceBasket resolves every line and sums successful subtotals.
func PriceBasket(lines []BasketLine, resolve func(string) (PriceQuote, error)) BasketTotal {
	result := BasketTotal{Lines: make([]PricedLine, 0, len(lines))}
	for _, line := range lines {
		quote, err := resolve(line.Ref)
		priced := PricedLine{Line: line, Quote: quote, Err: err}
		if err != nil {
			result.Failed++
			result.Lines = append(result.Lines, priced)
			continue
		}
		priced.SubtotalCents, err = domain.MultiplyCents(quote.UnitCents, line.Quantity)
		if err != nil {
			priced.Err = err
			result.Failed++
			result.Lines = append(result.Lines, priced)
			continue
		}
		if priced.SubtotalCents > math.MaxInt64-result.TotalCents {
			priced.Err = fmt.Errorf("basket total exceeds integer-cents range")
			result.Failed++
			result.Lines = append(result.Lines, priced)
			continue
		}
		result.TotalCents += priced.SubtotalCents
		result.Lines = append(result.Lines, priced)
	}
	return result
}

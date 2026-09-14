package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// maxCents is the largest value representable in the int64 cent space. Rounded
// amounts at or above it would wrap, so conversions reject them instead.
const maxCents = float64(math.MaxInt64)

// ParsePriceCents parses a "13.99"-style euro price into integer cents so totals
// sum exactly with no floating-point drift across many lines.
func ParsePriceCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty price")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("unparseable price %q", s)
	}
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid price %q", s)
	}
	return EurosToCents(v)
}

// EurosToCents rounds a finite, non-negative euro float to whole cents.
func EurosToCents(v float64) (int64, error) {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid price %v", v)
	}
	rounded := math.Round(v * 100)
	if rounded >= maxCents {
		return 0, fmt.Errorf("price %v exceeds integer-cents range", v)
	}
	return int64(rounded), nil
}

// MultiplyCents returns a subtotal in whole cents: unit-price cents × quantity.
// It is the single overflow-guarded multiplication behind both cart lines and
// basket totals.
func MultiplyCents(unitCents int64, quantity float64) (int64, error) {
	if unitCents < 0 || quantity < 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, fmt.Errorf("invalid amount: %d cents × %v", unitCents, quantity)
	}
	rounded := math.Round(float64(unitCents) * quantity)
	if rounded >= maxCents {
		return 0, fmt.Errorf("subtotal exceeds integer-cents range")
	}
	return int64(rounded), nil
}

// FormatCents renders integer cents as a "35.00"-style euro string, sign-safe.
func FormatCents(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

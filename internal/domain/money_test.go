package domain

import (
	"math"
	"testing"
)

func mathNaN() float64 { return math.NaN() }
func mathInf() float64 { return math.Inf(1) }

func TestParsePriceCentsAvoidsFloatDrift(t *testing.T) {
	c, err := ParsePriceCents("13.99")
	if err != nil || c != 1399 {
		t.Fatalf("ParsePriceCents = %d, %v; want 1399", c, err)
	}
	line, err := MultiplyCents(1399, 3)
	if err != nil || FormatCents(line) != "41.97" {
		t.Errorf("3×13.99 = %s, want 41.97", FormatCents(line))
	}
	c, err = EurosToCents(1.58 * 1)
	if err != nil || FormatCents(c) != "1.58" {
		t.Errorf("EurosToCents drift: %s", FormatCents(c))
	}
}

func TestParsePriceCentsRejectsUnusableInput(t *testing.T) {
	for _, input := range []string{"", "abc", "-1.05", "NaN", "+Inf", "-Inf", "1e308"} {
		if _, err := ParsePriceCents(input); err == nil {
			t.Errorf("ParsePriceCents(%q) should reject invalid or overflowing prices", input)
		}
	}
}

func TestMultiplyCentsRejectsUnusableInput(t *testing.T) {
	for _, tc := range []struct {
		unit int64
		qty  float64
	}{{-1, 1}, {100, -1}, {100, mathNaN()}, {100, mathInf()}, {1 << 62, 1 << 10}} {
		if _, err := MultiplyCents(tc.unit, tc.qty); err == nil {
			t.Errorf("MultiplyCents(%d, %v) should be rejected", tc.unit, tc.qty)
		}
	}
}

func TestFormatCentsIsSignSafe(t *testing.T) {
	for _, tc := range []struct {
		cents int64
		want  string
	}{{0, "0.00"}, {5, "0.05"}, {4197, "41.97"}, {-4197, "-41.97"}} {
		if got := FormatCents(tc.cents); got != tc.want {
			t.Errorf("FormatCents(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func FuzzParsePriceCents(f *testing.F) {
	for _, s := range []string{"", "0", "13.99", "-1.05", "abc", "1e308", "NaN", "  10  ", "1,69"} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = ParsePriceCents(s) // may error, must not panic
	})
}

// The cent conversions must reject what they cannot represent rather than
// silently wrapping into a negative or truncated amount.
func TestEurosToCentsRejectsOutOfRangeValues(t *testing.T) {
	for _, v := range []float64{-0.01, math.NaN(), math.Inf(1), math.Inf(-1), 1e18} {
		if _, err := EurosToCents(v); err == nil {
			t.Errorf("EurosToCents(%v) should be rejected", v)
		}
	}
	for _, tc := range []struct {
		in   float64
		want int64
	}{{0, 0}, {1.58, 158}, {0.005, 1}} {
		got, err := EurosToCents(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("EurosToCents(%v) = %d, %v; want %d", tc.in, got, err, tc.want)
		}
	}
}

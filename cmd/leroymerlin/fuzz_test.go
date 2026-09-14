package main

import (
	"math"
	"testing"
)

// Fuzz the parsers that consume user-supplied list files (-f basket/term files)
// and API price strings. None may panic on adversarial input — a crash would be
// a DoS on total/batch/cart. The seed corpus also runs under plain `go test`.

func FuzzParseBasketLine(f *testing.F) {
	for _, s := range []string{"", "82231893", "82231893 2", "x NaN", "a b c", "  ", "1 -0", "9 1e400", "ref 0.5", "/productos/x-1.html 3"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		bl, err := parseBasketLine(s)
		if err == nil && (bl.qty < 0 || math.IsNaN(bl.qty) || math.IsInf(bl.qty, 0)) {
			t.Fatalf("parseBasketLine(%q) accepted a bad qty %v", s, bl.qty)
		}
	})
}

func FuzzStripComment(f *testing.F) {
	for _, s := range []string{"", "#", "a # b", "82231893 1 # name", "####"} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, s string) {
		_ = stripComment(s)
	})
}

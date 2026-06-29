package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

type basketLine struct {
	ref string // a product url/path or a search term
	qty float64
}

// collectBasket reads basket lines from a file/stdin (-f, one '<ref> [qty]' per
// line, '#' comments skipped) or, with no file, from positional args (each a
// bare ref with qty 1). A per-line missing qty defaults to 1.
func collectBasket(file string, posArgs []string) ([]basketLine, error) {
	if file == "" {
		lines := make([]basketLine, 0, len(posArgs))
		for _, a := range posArgs {
			lines = append(lines, basketLine{ref: a, qty: 1})
		}
		return lines, nil
	}
	var lines []basketLine
	err := scanSource(file, func(ln int, t string) error {
		bl, perr := parseBasketLine(t)
		if perr != nil {
			return fmt.Errorf("line %d: %w", ln, perr)
		}
		lines = append(lines, bl)
		return nil
	})
	return lines, err
}

// scanSource invokes fn for each non-empty, comment-stripped line read from a
// file (or stdin when path is "-"), with the line's 1-based number for errors.
func scanSource(path string, fn func(lineNo int, text string) error) error {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	for sc.Scan() {
		ln++
		if t := stripComment(sc.Text()); t != "" {
			if err := fn(ln, t); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// stripComment drops a '#' comment — whole-line or trailing — and trims.
func stripComment(s string) string {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseBasketLine parses '<ref> [qty]'. A bare ref takes qty 1. Because a ref
// (search term) may contain spaces, only a trailing finite non-negative number
// is treated as the quantity; otherwise the whole line is the ref.
func parseBasketLine(s string) (basketLine, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return basketLine{}, fmt.Errorf("empty line")
	}
	last := f[len(f)-1]
	if q, err := strconv.ParseFloat(last, 64); err == nil && len(f) > 1 {
		if q < 0 || math.IsInf(q, 0) || math.IsNaN(q) {
			return basketLine{}, fmt.Errorf("invalid qty %q (want a finite, non-negative number)", last)
		}
		return basketLine{ref: strings.Join(f[:len(f)-1], " "), qty: q}, nil
	}
	return basketLine{ref: s, qty: 1}, nil
}

// collectLines returns non-empty comment-stripped lines from a file/stdin, or the
// positional args verbatim. Used by batch for free-text search terms.
func collectLines(file string, posArgs []string) ([]string, error) {
	if file == "" {
		return posArgs, nil
	}
	var out []string
	err := scanSource(file, func(_ int, t string) error {
		out = append(out, t)
		return nil
	})
	return out, err
}

// priceCents parses a "13.99"-style euro price into integer cents so totals sum
// exactly with no floating-point drift across many lines.
func priceCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty price")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("unparseable price %q", s)
	}
	return eurosToCents(v), nil
}

// eurosToCents rounds a euro float to whole cents.
func eurosToCents(v float64) int64 { return int64(math.Round(v * 100)) }

// lineCents returns a line subtotal in whole cents: unit-price cents × qty.
func lineCents(unitCents int64, qty float64) int64 {
	return int64(math.Round(float64(unitCents) * qty))
}

// centsStr renders integer cents as a "35.00"-style euro string, sign-safe.
func centsStr(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}

func fmtQty(q float64) string {
	if q == math.Trunc(q) {
		return strconv.FormatInt(int64(q), 10)
	}
	return strconv.FormatFloat(q, 'g', -1, 64)
}

// plural renders "1 línea" / "3 líneas".
func plural(n int, one, many string) string {
	w := many
	if n == 1 {
		w = one
	}
	return fmt.Sprintf("%d %s", n, w)
}

// cheapestHit picks the best hit among search results: the cheapest in-stock,
// non-sponsored product. It falls back to the cheapest in-stock product (if all
// are sponsored), then the cheapest of any (if none are in stock). Returns false
// only for an empty slice.
func cheapestHit(prods []client.Product) (client.Product, bool) {
	if len(prods) == 0 {
		return client.Product{}, false
	}
	pick := func(filter func(client.Product) bool) (client.Product, bool) {
		var best client.Product
		found := false
		for _, p := range prods {
			if !filter(p) {
				continue
			}
			if !found || p.Offer.UnitPriceATI < best.Offer.UnitPriceATI {
				best, found = p, true
			}
		}
		return best, found
	}
	if p, ok := pick(func(p client.Product) bool { return p.Offer.AddToCart && !p.Sponsored }); ok {
		return p, true
	}
	if p, ok := pick(func(p client.Product) bool { return p.Offer.AddToCart }); ok {
		return p, true
	}
	return pick(func(client.Product) bool { return true })
}

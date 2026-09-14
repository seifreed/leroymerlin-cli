package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
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

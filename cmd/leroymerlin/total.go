package main

import (
	"fmt"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// cmdTotal computes a deterministic basket total from '<ref> [qty]' lines: each
// ref is either a product url (priced from its page) or a search term (priced
// from its cheapest in-stock hit). Subtotals sum in integer cents — exact and
// reproducible. A pre-cart estimate; the site stays authoritative at checkout.
func cmdTotal(args []string) error {
	fs, cf := newCommonFlags("total")
	file := fs.String("f", "", "file with one '<url|term> [qty]' per line ('-' for stdin); else refs are positional (qty 1)")
	parseFlags(fs, args)

	lines, err := collectBasket(*file, fs.Args())
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return fmt.Errorf("no basket lines (use -f file, stdin, or '<url|term> [<...>]' args)")
	}
	cl := newClient(cf)

	type lineResult struct {
		Ref      string  `json:"ref"`
		Name     string  `json:"name,omitempty"`
		Qty      float64 `json:"qty"`
		Price    string  `json:"price,omitempty"`
		Subtotal string  `json:"subtotal,omitempty"`
		Error    string  `json:"error,omitempty"`
	}
	results := make([]lineResult, 0, len(lines))
	var totalCents int64
	failed := 0
	for _, bl := range lines {
		name, priceStr, unitCents, lerr := priceRef(cl, bl.ref)
		if lerr != nil {
			results = append(results, lineResult{Ref: bl.ref, Qty: bl.qty, Error: lerr.Error()})
			failed++
			continue
		}
		sub := lineCents(unitCents, bl.qty)
		totalCents += sub
		results = append(results, lineResult{
			Ref: bl.ref, Name: name, Qty: bl.qty,
			Price: priceStr, Subtotal: centsStr(sub),
		})
	}

	if done, err := emitStructured(cf, map[string]any{
		"lines":    results,
		"total":    centsStr(totalCents),
		"count":    len(lines),
		"complete": failed == 0,
	}); done {
		return err
	}
	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("  %s  ERROR: %s\n", r.Ref, r.Error)
			continue
		}
		fmt.Printf("  %s — %s × %s€ = %s€\n", firstNonEmpty(r.Name, r.Ref), fmtQty(r.Qty), r.Price, r.Subtotal)
	}
	fmt.Printf("  total: %s€  (%s)\n", centsStr(totalCents), plural(len(lines), "línea", "líneas"))
	if failed > 0 {
		return fmt.Errorf("%d of %d lines could not be priced (excluded from the total)", failed, len(lines))
	}
	return nil
}

// priceRef resolves a basket ref to (name, priceString, unitCents). A
// /productos/ url is priced from its product page; anything else is treated as a
// search term and priced from its cheapest in-stock hit.
func priceRef(cl *client.Client, ref string) (name, price string, cents int64, err error) {
	if strings.Contains(ref, "/productos/") {
		pd, perr := cl.Product(ref)
		if perr != nil {
			if status, ok := client.HTTPStatus(perr); perr == client.ErrNoProduct || (ok && status == 404) {
				return "", "", 0, fmt.Errorf("not found (check the url from search)")
			}
			return "", "", 0, perr
		}
		c, cerr := priceCents(pd.Price())
		if cerr != nil {
			return pd.Name, "", 0, cerr
		}
		return pd.Name, pd.Price(), c, nil
	}
	prods, serr := cl.Search(ref, 0)
	if serr != nil {
		return "", "", 0, serr
	}
	p, ok := cheapestHit(prods)
	if !ok {
		return "", "", 0, fmt.Errorf("no results for %q", ref)
	}
	priceStr := centsStr(eurosToCents(p.Offer.UnitPriceATI))
	return p.Name, priceStr, eurosToCents(p.Offer.UnitPriceATI), nil
}

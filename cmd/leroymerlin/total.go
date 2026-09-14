package main

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/application"
	"github.com/seifreed/leroymerlin-cli/internal/client"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
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
	cl := newClient()

	type lineResult struct {
		Ref      string  `json:"ref"`
		Name     string  `json:"name,omitempty"`
		Qty      float64 `json:"qty"`
		Price    string  `json:"price,omitempty"`
		Subtotal string  `json:"subtotal,omitempty"`
		Error    string  `json:"error,omitempty"`
	}
	appLines := make([]application.BasketLine, 0, len(lines))
	for _, line := range lines {
		appLines = append(appLines, application.BasketLine{Ref: line.ref, Quantity: line.qty})
	}
	total := application.PriceBasket(appLines, func(ref string) (application.PriceQuote, error) {
		name, price, cents, err := priceRef(cl, ref)
		return application.PriceQuote{Name: name, DisplayPrice: price, UnitCents: cents}, err
	})
	results := make([]lineResult, 0, len(total.Lines))
	for _, priced := range total.Lines {
		result := lineResult{Ref: priced.Line.Ref, Qty: priced.Line.Quantity}
		if priced.Err != nil {
			result.Error = priced.Err.Error()
		} else {
			result.Name = priced.Quote.Name
			result.Price = priced.Quote.DisplayPrice
			result.Subtotal = domain.FormatCents(priced.SubtotalCents)
		}
		results = append(results, result)
	}

	if done, err := emitStructured(cf, map[string]any{
		"lines":    results,
		"total":    domain.FormatCents(total.TotalCents),
		"count":    len(lines),
		"complete": total.Failed == 0,
	}); done {
		return err
	}
	for _, r := range results {
		if r.Error != "" {
			fmt.Printf("  %s  ERROR: %s\n", r.Ref, r.Error)
			continue
		}
		fmt.Printf("  %s — %s × %s€ = %s€\n", cmp.Or(r.Name, r.Ref), fmtQty(r.Qty), r.Price, r.Subtotal)
	}
	fmt.Printf("  total: %s€  (%s)\n", domain.FormatCents(total.TotalCents), plural(len(lines), "línea", "líneas"))
	if total.Failed > 0 {
		return fmt.Errorf("%d of %d lines could not be priced (excluded from the total)", total.Failed, len(lines))
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
		c, cerr := domain.ParsePriceCents(pd.Price())
		if cerr != nil {
			return pd.Name, "", 0, cerr
		}
		return pd.Name, pd.Price(), c, nil
	}
	found, serr := cl.Search(ref, 0)
	if serr != nil {
		return "", "", 0, serr
	}
	p, ok := domain.CheapestHit(found.Products)
	if !ok {
		return "", "", 0, fmt.Errorf("no results for %q", ref)
	}
	if found.Relaxed {
		return "", "", 0, fmt.Errorf("no exact match for %q — the storefront answered with %q; price it by url if that is what you meant", ref, strings.TrimSpace(p.Name))
	}
	unitCents, cerr := domain.EurosToCents(p.Offer.UnitPriceATI)
	if cerr != nil {
		return p.Name, "", 0, cerr
	}
	return p.Name, domain.FormatCents(unitCents), unitCents, nil
}

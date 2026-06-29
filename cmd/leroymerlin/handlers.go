package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// cmdSearch runs a full-text product search.
func cmdSearch(args []string) error {
	fs, cf := newCommonFlags("search")
	limit := fs.Int("limit", 0, "cap results (0 = all the page returns)")
	cheapest := fs.Bool("cheapest", false, "rank by price, low → high")
	inStock := fs.Bool("in-stock", false, "keep only buyable items")
	parseFlags(fs, args)

	term := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(term) == "" {
		return fmt.Errorf("usage: leroymerlin search <term...>")
	}

	cl := newClient(cf)
	products, err := cl.Search(term, 0) // limit applied after filtering/sorting
	if err != nil {
		return err
	}

	if *inStock {
		kept := products[:0]
		for _, p := range products {
			if p.Offer.AddToCart {
				kept = append(kept, p)
			}
		}
		products = kept
	}
	if *cheapest {
		sort.SliceStable(products, func(i, j int) bool {
			return products[i].Offer.UnitPriceATI < products[j].Offer.UnitPriceATI
		})
	}
	if *limit > 0 && len(products) > *limit {
		products = products[:*limit]
	}

	if emitted, err := emitStructured(cf, products); emitted || err != nil {
		return err
	}
	if len(products) == 0 {
		fmt.Fprintln(os.Stderr, "no results")
		return nil
	}
	for _, p := range products {
		fmt.Println(productLine(p))
	}
	return nil
}

// cmdProduct shows a product page's detail. The argument is the url (or path)
// from a search result.
func cmdProduct(args []string) error {
	fs, cf := newCommonFlags("product")
	parseFlags(fs, args)

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: leroymerlin product <url|path>  (pass the url from a search result)")
	}
	target := rest[0]
	if !strings.Contains(target, "/productos/") {
		return fmt.Errorf("expected a product url like /productos/...-<ref>.html (from a search result), got %q", target)
	}

	cl := newClient(cf)
	d, err := cl.Product(target)
	if err != nil {
		if err == client.ErrNoProduct {
			return fmt.Errorf("no product found at %s — check the url from `leroymerlin search`", target)
		}
		return err
	}

	if emitted, err := emitStructured(cf, d); emitted || err != nil {
		return err
	}
	fmt.Print(detailLines(d))
	return nil
}

// cmdSetCookie seeds a raw Cookie header (browser DataDome clearance) used when
// anonymous reads draw a bot challenge.
func cmdSetCookie(args []string) error {
	fs := newFlagSet("set-cookie")
	stdin := fs.Bool("stdin", false, "read the cookie from stdin")
	parseFlags(fs, args)

	var cookie string
	if *stdin {
		b, err := io.ReadAll(bufio.NewReader(os.Stdin))
		if err != nil {
			return err
		}
		cookie = strings.TrimSpace(string(b))
	} else {
		cookie = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if cookie == "" {
		return fmt.Errorf("usage: leroymerlin set-cookie '<cookie header>'  (or --stdin)")
	}
	if err := client.SaveSession(client.Session{Cookie: cookie}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "cookie saved")
	return nil
}

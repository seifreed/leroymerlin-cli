package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/application"
	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// cmdSearch runs a full-text product search.
func cmdSearch(args []string) error {
	fs, cf := newCommonFlags("search")
	limit := fs.Int("limit", 0, "cap results (0 = one page ~48; higher auto-paginates)")
	cheapest := fs.Bool("cheapest", false, "rank by price, low → high")
	inStock := fs.Bool("in-stock", false, "keep only buyable items")
	onOfferOnly := fs.Bool("on-offer", false, "keep only items with a discount/promo")
	parseFlags(fs, args)

	term := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(term) == "" {
		return fmt.Errorf("usage: leroymerlin search <term...>")
	}

	cl := newClient()
	products, err := application.SearchProducts(cl, term, application.SearchOptions{
		Limit: *limit, Cheapest: *cheapest, InStock: *inStock, OnOfferOnly: *onOfferOnly,
	})
	if err != nil {
		return err
	}

	if emitted, err := emitStructured(cf, products); emitted || err != nil {
		return err
	}
	printProducts(products, "no results")
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

	cl := newClient()
	d, err := cl.Product(target)
	if err != nil {
		if status, ok := client.HTTPStatus(err); err == client.ErrNoProduct || (ok && status == 404) {
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

// cmdImportHar lifts the browser cookie (DataDome clearance) from a DevTools HAR
// export and caches it for challenged reads.
func cmdImportHar(args []string) error {
	fs := newFlagSet("import-har")
	file := fs.String("file", "", "path to the HAR export ('-' for stdin)")
	parseFlags(fs, args)
	if *file == "" {
		return fmt.Errorf("usage: leroymerlin import-har --file <export.har>")
	}
	var data []byte
	var err error
	if *file == "-" {
		data, err = io.ReadAll(bufio.NewReader(os.Stdin))
	} else {
		data, err = os.ReadFile(*file)
	}
	if err != nil {
		return err
	}
	cookie, err := client.ParseHAR(data)
	if err != nil {
		return err
	}
	if err := saveSession(client.Session{Cookie: cookie}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "cookie imported from HAR")
	return nil
}

// cmdSetCookie seeds a raw Cookie header copied from a signed-in browser, for
// the case where the cookie store cannot be read directly.
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
	if err := saveSession(client.Session{Cookie: cookie}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "cookie saved")
	return nil
}

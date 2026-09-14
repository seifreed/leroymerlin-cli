package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/application"
	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// cmdBrands aggregates the distinct brands selling a product type, so you can
// discover names for the [brands] config that batch then honours. Leroy Merlin
// has no global brand catalogue endpoint (bonpreu's `brands` lists one), so this
// adapts the feature to "which brands stock <term>", ranked by how many hits each
// has — the part that actually feeds brand preferences.
func cmdBrands(args []string) error {
	fs, cf := newCommonFlags("brands")
	limit := fs.Int("limit", 0, "cap the brands listed (0 = all)")
	parseFlags(fs, args)

	term := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if term == "" {
		return fmt.Errorf("usage: leroymerlin brands <term...>  (lists the brands selling that product type)")
	}
	cl := newClient()
	prods, err := application.SearchProducts(cl, term, application.SearchOptions{})
	if err != nil {
		return err
	}
	tallies := domain.TallyBrands(prods)
	if *limit > 0 && len(tallies) > *limit {
		tallies = tallies[:*limit]
	}

	if done, err := emitStructured(cf, tallies); done {
		return err
	}
	if len(tallies) == 0 {
		fmt.Fprintln(os.Stderr, "no brands found")
		return nil
	}
	w := 0
	for _, b := range tallies {
		if len(b.Brand) > w {
			w = len(b.Brand)
		}
	}
	for _, b := range tallies {
		fmt.Printf("  %-*s  %2d ud.  desde %s\n", w, b.Brand, b.Count, eur(b.MinPrice))
	}
	return nil
}

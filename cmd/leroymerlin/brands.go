package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// brandTally is one brand's footprint within a result set.
type brandTally struct {
	Brand    string  `json:"brand"`
	Count    int     `json:"count"`
	MinPrice float64 `json:"minPrice"`
}

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
	cl := newClient(cf)
	prods, err := cl.Search(term, 0)
	if err != nil {
		return err
	}
	tallies := tallyBrands(prods)
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

// tallyBrands groups products by brand, counting hits and the cheapest price per
// brand, sorted by count desc then brand name. Unbranded products are skipped.
func tallyBrands(prods []client.Product) []brandTally {
	idx := map[string]*brandTally{}
	for _, p := range prods {
		name := strings.TrimSpace(p.Brand)
		if name == "" {
			continue
		}
		t := idx[name]
		if t == nil {
			t = &brandTally{Brand: name, MinPrice: p.Offer.UnitPriceATI}
			idx[name] = t
		}
		t.Count++
		if p.Offer.UnitPriceATI > 0 && (t.MinPrice == 0 || p.Offer.UnitPriceATI < t.MinPrice) {
			t.MinPrice = p.Offer.UnitPriceATI
		}
	}
	out := make([]brandTally, 0, len(idx))
	for _, t := range idx {
		out = append(out, *t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Brand < out[j].Brand
	})
	return out
}

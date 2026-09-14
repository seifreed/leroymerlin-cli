package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/application"
	"github.com/seifreed/leroymerlin-cli/internal/config"
	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// cmdBatch resolves many search terms in one command — the preferred brand when
// configured (or --brand), else the cheapest in-stock, non-sponsored hit per
// term. Bridges plain-word shopping lists to refs and prices, one request each.
func cmdBatch(args []string) error {
	fs, cf := newCommonFlags("batch")
	file := fs.String("f", "", "file with one search term per line ('-' for stdin); else terms are positional")
	brand := fs.String("brand", "", "prefer these brands (comma-separated), overriding config for this run")
	noBrands := fs.Bool("no-brands", false, "ignore configured brand preferences (pick cheapest)")
	onOfferOnly := fs.Bool("on-offer", false, "resolve each term among its discounted/promo products only")
	parseFlags(fs, args)

	terms, err := collectLines(*file, fs.Args())
	if err != nil {
		return err
	}
	if len(terms) == 0 {
		return fmt.Errorf("no terms (use -f file, stdin, or '<term>...' args)")
	}
	cl := newClient()
	cfg, _ := config.LoadConfig()
	override := splitBrands(*brand)

	out, missing := application.ResolveBatch(cl, terms, domain.BrandPreferences{
		Preferred: cfg.Brands.Preferred, Mode: cfg.Brands.Mode, Overrides: cfg.Brands.Overrides,
	}, override, *noBrands, *onOfferOnly)

	if done, err := emitStructured(cf, out); done {
		return err
	}
	if len(out) == 0 {
		fmt.Fprintln(os.Stderr, "no term has a product on offer")
		return nil
	}
	w := 0
	for _, h := range out {
		if len(h.Term) > w {
			w = len(h.Term)
		}
	}
	for _, h := range out {
		if h.Product == nil {
			fmt.Printf("• %-*s → (sin resultados)\n", w, h.Term)
			continue
		}
		line := productLine(*h.Product)
		if h.BrandMatch == "none" {
			line += "  ⚠ marca preferida no disponible"
		}
		fmt.Printf("• %-*s → %s\n", w, h.Term, line)
	}
	if missing > 0 {
		return fmt.Errorf("%d of %d terms returned no product", missing, len(terms))
	}
	return nil
}

// splitBrands parses a --brand "a,b" list into trimmed, non-empty brand names.
func splitBrands(value string) []string {
	var brands []string
	for _, brand := range strings.Split(value, ",") {
		if brand = strings.TrimSpace(brand); brand != "" {
			brands = append(brands, brand)
		}
	}
	return brands
}

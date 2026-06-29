package main

import (
	"fmt"

	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/config"
)

// cmdBatch resolves many search terms in one command — the preferred brand when
// configured (or --brand), else the cheapest in-stock, non-sponsored hit per
// term. Bridges plain-word shopping lists to refs and prices, one request each.
func cmdBatch(args []string) error {
	fs, cf := newCommonFlags("batch")
	file := fs.String("f", "", "file with one search term per line ('-' for stdin); else terms are positional")
	brand := fs.String("brand", "", "prefer these brands (comma-separated), overriding config for this run")
	noBrands := fs.Bool("no-brands", false, "ignore configured brand preferences (pick cheapest)")
	onOfferOnly := fs.Bool("on-offer", false, "only resolve terms whose chosen hit has a discount/promo")
	parseFlags(fs, args)

	terms, err := collectLines(*file, fs.Args())
	if err != nil {
		return err
	}
	if len(terms) == 0 {
		return fmt.Errorf("no terms (use -f file, stdin, or '<term>...' args)")
	}
	cl := newClient(cf)
	cfg, _ := config.LoadConfig()
	override := splitBrands(*brand)

	type hit struct {
		Term       string          `json:"term"`
		Product    *client.Product `json:"product"`
		BrandMatch string          `json:"brandMatch,omitempty"` // preferred | override | none | off
		Offer      string          `json:"offer,omitempty"`      // discount | promo
		OfferLabel string          `json:"offerLabel,omitempty"`
	}
	out := make([]hit, 0, len(terms))
	missing := 0
	for _, t := range terms {
		prods, serr := cl.Search(t, 0)
		if serr != nil || len(prods) == 0 {
			out = append(out, hit{Term: t})
			missing++
			continue
		}
		p, match := resolveBrandHit(prods, t, cfg.Brands, override, *noBrands)
		if *onOfferOnly && !onOffer(p) {
			continue // term resolved, but its hit has no offer → drop under --on-offer
		}
		kind, label := classifyOffer(p)
		out = append(out, hit{Term: t, Product: &p, BrandMatch: match, Offer: kind, OfferLabel: label})
	}

	if done, err := emitStructured(cf, out); done {
		return err
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

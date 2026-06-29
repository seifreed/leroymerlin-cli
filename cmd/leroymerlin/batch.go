package main

import (
	"fmt"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// cmdBatch resolves many search terms in one command — the cheapest in-stock,
// non-sponsored hit per term — bridging plain-word shopping lists to refs and
// prices. One request per term.
func cmdBatch(args []string) error {
	fs, cf := newCommonFlags("batch")
	file := fs.String("f", "", "file with one search term per line ('-' for stdin); else terms are positional")
	parseFlags(fs, args)

	terms, err := collectLines(*file, fs.Args())
	if err != nil {
		return err
	}
	if len(terms) == 0 {
		return fmt.Errorf("no terms (use -f file, stdin, or '<term>...' args)")
	}
	cl := newClient(cf)

	type hit struct {
		Term    string          `json:"term"`
		Product *client.Product `json:"product"`
	}
	out := make([]hit, 0, len(terms))
	missing := 0
	for _, t := range terms {
		prods, serr := cl.Search(t, 0)
		if serr != nil {
			out = append(out, hit{Term: t})
			missing++
			continue
		}
		p, ok := cheapestHit(prods)
		if !ok {
			out = append(out, hit{Term: t})
			missing++
			continue
		}
		out = append(out, hit{Term: t, Product: &p})
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
		fmt.Printf("• %-*s → %s\n", w, h.Term, productLine(*h.Product))
	}
	if missing > 0 {
		return fmt.Errorf("%d of %d terms returned no product", missing, len(terms))
	}
	return nil
}

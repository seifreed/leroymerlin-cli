package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// sortCheapest stable-sorts products by tax-included price, low → high.
func sortCheapest(products []client.Product) {
	sort.SliceStable(products, func(i, j int) bool {
		return products[i].Offer.UnitPriceATI < products[j].Offer.UnitPriceATI
	})
}

// cmdCategories lists the top-level catalog sections, or — given a category
// slug/path — that category's products.
func cmdCategories(args []string) error {
	fs, cf := newCommonFlags("categories")
	limit := fs.Int("limit", 0, "max products when listing a category")
	cheapest := fs.Bool("cheapest", false, "rank a category's products by price, low → high")
	parseFlags(fs, args)

	cl := newClient(cf)
	rest := fs.Args()

	if len(rest) > 0 {
		path := rest[0]
		products, err := cl.CategoryProducts(path, *limit)
		if err != nil {
			if status, ok := client.HTTPStatus(err); ok && status == 404 {
				return fmt.Errorf("category %q not found — see `leroymerlin categories` for valid sections", path)
			}
			return err
		}
		if *cheapest {
			sortCheapest(products)
		}
		if *limit > 0 && len(products) > *limit {
			products = products[:*limit]
		}
		if done, err := emitStructured(cf, products); done {
			return err
		}
		if len(products) == 0 {
			fmt.Fprintln(os.Stderr, "no products listed (try a different category slug)")
			return nil
		}
		for _, p := range products {
			fmt.Println(productLine(p))
		}
		return nil
	}

	cats, err := cl.Categories()
	if err != nil {
		return err
	}
	if done, err := emitStructured(cf, cats); done {
		return err
	}
	if len(cats) == 0 {
		fmt.Fprintln(os.Stderr, "no categories found")
		return nil
	}
	w := 0
	for _, c := range cats {
		if len(c.Path) > w {
			w = len(c.Path)
		}
	}
	for _, c := range cats {
		fmt.Printf("  %-*s  %s\n", w, c.Path, c.Name)
	}
	return nil
}

package main

import (
	"fmt"
	"os"

	"github.com/seifreed/leroymerlin-cli/internal/application"
	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// cmdCategories lists the top-level catalog sections, or — given a category
// slug/path — that category's products.
func cmdCategories(args []string) error {
	fs, cf := newCommonFlags("categories")
	limit := fs.Int("limit", 0, "max products when listing a category")
	cheapest := fs.Bool("cheapest", false, "rank a category's products by price, low → high")
	subs := fs.Bool("subs", false, "with a slug: list child subcategories instead of products")
	parseFlags(fs, args)

	cl := newClient()
	rest := fs.Args()

	if len(rest) > 0 && *subs {
		kids, err := cl.Subcategories(rest[0])
		if err != nil {
			if status, ok := client.HTTPStatus(err); ok && status == 404 {
				return fmt.Errorf("category %q not found — see `leroymerlin categories`", rest[0])
			}
			return err
		}
		if done, err := emitStructured(cf, kids); done {
			return err
		}
		if len(kids) == 0 {
			fmt.Fprintln(os.Stderr, "no subcategories (leaf or curated landing)")
			return nil
		}
		printCategoryList(kids)
		return nil
	}

	if len(rest) > 0 {
		path := rest[0]
		products, err := application.ListCategoryProducts(cl, path, application.SearchOptions{
			Limit: *limit, Cheapest: *cheapest,
		})
		if err != nil {
			if status, ok := client.HTTPStatus(err); ok && status == 404 {
				return fmt.Errorf("category %q not found — see `leroymerlin categories` for valid sections", path)
			}
			return err
		}
		if done, err := emitStructured(cf, products); done {
			return err
		}
		printProducts(products, "no products listed (try a different category slug)")
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
	printCategoryList(cats)
	return nil
}

// printCategoryList renders categories as "  <path>  <name>", path column aligned.
func printCategoryList(cats []domain.Category) {
	w := 0
	for _, c := range cats {
		if len(c.Path) > w {
			w = len(c.Path)
		}
	}
	for _, c := range cats {
		fmt.Printf("  %-*s  %s\n", w, c.Path, c.Name)
	}
}

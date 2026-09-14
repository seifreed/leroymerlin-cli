package application

import (
	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// CatalogReader is the catalog operation needed by the search use case.
type CatalogReader interface {
	Search(term string, limit int) ([]domain.Product, error)
}

// SearchOptions controls post-fetch search policies.
type SearchOptions struct {
	Limit       int
	Cheapest    bool
	InStock     bool
	OnOfferOnly bool
}

// productsPerPage is what one listing page returns. It mirrors the storefront,
// and being a little off only costs (or saves) one page of fetching.
const productsPerPage = 48

// narrows reports whether a policy runs after the fetch and changes which
// products survive or in what order.
func (o SearchOptions) narrows() bool {
	return o.Cheapest || o.InStock || o.OnOfferOnly
}

// fetchLimit is how many hits to ask the catalog for. Without a post-fetch
// policy that is just the limit. With one it must not be: ranking the first N
// answers "the cheapest of the first N" rather than "the N cheapest", and
// filtering them returns fewer than N while more exist further down. Whole
// pages are fetched instead and the truncation happens at the end; a limit past
// one page still paginates as before.
func (o SearchOptions) fetchLimit() int {
	if o.Limit <= 0 || !o.narrows() || o.Limit >= productsPerPage {
		return o.Limit
	}
	return 0 // one page
}

// SearchProducts fetches, filters, ranks, and truncates catalog results.
func SearchProducts(reader CatalogReader, term string, options SearchOptions) ([]domain.Product, error) {
	products, err := reader.Search(term, options.fetchLimit())
	if err != nil {
		return nil, err
	}
	return narrow(products, options), nil
}

// CategoryReader is the catalog section listing needed by the categories use case.
type CategoryReader interface {
	CategoryProducts(path string, limit int) ([]domain.Product, error)
}

// ListCategoryProducts lists a section's products under the same rule as
// SearchProducts: a policy that runs after the fetch has to see a whole page,
// or ranking answers "the cheapest of the first N" instead of "the N cheapest".
func ListCategoryProducts(reader CategoryReader, path string, options SearchOptions) ([]domain.Product, error) {
	products, err := reader.CategoryProducts(path, options.fetchLimit())
	if err != nil {
		return nil, err
	}
	return narrow(products, options), nil
}

// narrow applies the post-fetch policies and the final truncation. It runs last
// so the limit counts what survived, not what was fetched.
func narrow(products []domain.Product, options SearchOptions) []domain.Product {
	if options.InStock {
		products = domain.Filter(products, func(product domain.Product) bool { return product.Offer.AddToCart })
	}
	if options.OnOfferOnly {
		products = domain.FilterOnOffer(products)
	}
	if options.Cheapest {
		domain.SortByPrice(products)
	}
	if options.Limit > 0 && len(products) > options.Limit {
		products = products[:options.Limit]
	}
	return products
}

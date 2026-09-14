package domain

import (
	"sort"
	"strings"
)

// BrandTally describes a brand's presence in a product result set.
type BrandTally struct {
	Brand    string  `json:"brand"`
	Count    int     `json:"count"`
	MinPrice float64 `json:"minPrice"`
}

// TallyBrands groups products by brand, skipping unbranded results.
func TallyBrands(products []Product) []BrandTally {
	groups := map[string]*BrandTally{}
	for _, product := range products {
		brand := strings.TrimSpace(product.Brand)
		if brand == "" {
			continue
		}
		tally := groups[brand]
		if tally == nil {
			tally = &BrandTally{Brand: brand, MinPrice: product.Offer.UnitPriceATI}
			groups[brand] = tally
		}
		tally.Count++
		if product.Offer.UnitPriceATI > 0 && (tally.MinPrice == 0 || product.Offer.UnitPriceATI < tally.MinPrice) {
			tally.MinPrice = product.Offer.UnitPriceATI
		}
	}
	result := make([]BrandTally, 0, len(groups))
	for _, tally := range groups {
		result = append(result, *tally)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Brand < result[j].Brand
	})
	return result
}

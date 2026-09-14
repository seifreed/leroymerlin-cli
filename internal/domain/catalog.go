// Package domain contains the shopping concepts and policies that do not depend
// on Leroy Merlin's HTTP or HTML representation.
package domain

// Offer is the pricing and seller information for a catalog product.
type Offer struct {
	UnitPriceATI float64  `json:"unitprice_ati"`
	UnitPriceTF  float64  `json:"unitprice_tf"`
	InitialPrice *float64 `json:"initial_price"`
	DiscountATI  *float64 `json:"discount_ati"`
	DiscountRate *float64 `json:"discount_rate"`
	SellerName   string   `json:"seller_name"`
	SellerType   string   `json:"seller_type"`
	OfferType    string   `json:"offer_type"`
	AddToCart    bool     `json:"add_to_cart_availability"`
	Animations   *struct {
		Label string `json:"label"`
	} `json:"commercial_animations"`
}

// Promo returns the active commercial-animation label, if present.
func (o Offer) Promo() string {
	if o.Animations == nil {
		return ""
	}
	return o.Animations.Label
}

// Product is a catalog item independent of the source that supplied it.
type Product struct {
	Brand      string  `json:"brand"`
	Identifier string  `json:"identifier"`
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	Rating     float64 `json:"rating"`
	Sponsored  bool    `json:"product_is_sponsored"`
	OfferCount int     `json:"total_offer_count"`
	Offer      Offer   `json:"offer"`
}

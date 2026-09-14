package domain

import "strings"

// ProductDetail is the source-independent detail of a catalog product.
type ProductDetail struct {
	Name        string           `json:"name"`
	SKU         string           `json:"sku"`
	GTIN        string           `json:"gtin"`
	Description string           `json:"description"`
	Brand       string           `json:"brand"`
	Offers      []ProductOffer   `json:"offers"`
	Rating      *ProductRating   `json:"aggregateRating"`
	Image       string           `json:"image"`
	Specs       []Spec           `json:"specs,omitempty"`
	Deliveries  []DeliveryOption `json:"deliveries,omitempty"`
	URL         string           `json:"-"`
}

// ProductOffer is one schema.org offer from a product page.
type ProductOffer struct {
	Price         string `json:"price"`
	PriceCurrency string `json:"priceCurrency"`
	Availability  string `json:"availability"`
	URL           string `json:"url"`
	ItemCondition string `json:"itemCondition"`
}

// ProductRating contains the aggregate product review values.
type ProductRating struct {
	Value string `json:"ratingValue"`
	Count string `json:"reviewCount"`
}

// Spec is one technical-characteristic row.
type Spec struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// DeliveryOption is one fulfilment channel and its session-specific stock.
type DeliveryOption struct {
	Type   string  `json:"type"`
	Status string  `json:"status"`
	Stock  int     `json:"stock"`
	Price  float64 `json:"price"`
	Time   string  `json:"time"`
}

// Price returns the first offer's tax-included price.
func (p ProductDetail) Price() string {
	if len(p.Offers) == 0 {
		return ""
	}
	return p.Offers[0].Price
}

// Currency returns the first offer's currency code.
func (p ProductDetail) Currency() string {
	if len(p.Offers) == 0 {
		return ""
	}
	return p.Offers[0].PriceCurrency
}

// Availability returns the first offer's short schema.org availability value.
func (p ProductDetail) Availability() string {
	if len(p.Offers) == 0 {
		return ""
	}
	availability := p.Offers[0].Availability
	if i := strings.LastIndex(availability, "/"); i >= 0 {
		return availability[i+1:]
	}
	return availability
}

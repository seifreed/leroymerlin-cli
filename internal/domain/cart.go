package domain

// CartSummary is the cart-data response needed by add-to-cart workflows.
type CartSummary struct {
	Quantity       int    `json:"quantity"`
	Order          string `json:"order"`
	RedirectionURL string `json:"redirectionUrl"`
}

// CartLine is one item in the current cart.
type CartLine struct {
	LineID   string  `json:"lineId"`
	Reflm    string  `json:"reflm"`
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// CartDetail is the cart contents, totals, and checkout readiness.
type CartDetail struct {
	OrderID          string     `json:"orderId"`
	Quantity         int        `json:"quantity"`
	Lines            []CartLine `json:"lines"`
	TotalAmount      float64    `json:"totalAmount"`
	OffersAmount     float64    `json:"offersAmount"`
	DeliveryAmount   float64    `json:"deliveryAmount"`
	DisabledCheckout bool       `json:"disabledCheckout"`
	Blockers         []string   `json:"blockers"`
}

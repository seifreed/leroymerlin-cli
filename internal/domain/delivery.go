package domain

// DeliverySlot is one delivery or pickup option offered for the cart.
type DeliverySlot struct {
	Mode     string  `json:"mode"`
	Label    string  `json:"label"`
	Amount   float64 `json:"amount"`
	Date     string  `json:"date"`
	Selected bool    `json:"selected"`
}

// ShippingInfo contains saved addresses and the available delivery options.
type ShippingInfo struct {
	Addresses map[string]map[string]any `json:"addresses"`
	Slots     []DeliverySlot            `json:"slots"`
}

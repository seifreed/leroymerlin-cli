package domain

// DeliverySlot is one delivery or pickup option offered for the cart.
type DeliverySlot struct {
	Mode   string  `json:"mode"`
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
	Date   string  `json:"date"`
	// Vendor is who ships this option. A cart with a marketplace line is several
	// shipments, each with its own choice, so "selected" is per vendor and an
	// unlabelled list reads as one choice made twice.
	Vendor   string `json:"vendor,omitempty"`
	Selected bool   `json:"selected"`
}

// ShippingInfo contains saved addresses and the available delivery options.
type ShippingInfo struct {
	Addresses map[string]map[string]any `json:"addresses"`
	Slots     []DeliverySlot            `json:"slots"`
}

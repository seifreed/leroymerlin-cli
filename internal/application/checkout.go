package application

import "github.com/seifreed/leroymerlin-cli/internal/domain"

// CheckoutReader provides the read-only storefront data needed by checkout.
type CheckoutReader interface {
	Cart() (*domain.CartDetail, error)
	Shipping() (*domain.ShippingInfo, error)
}

// CheckoutStatus is the checkout readiness projection used by the CLI.
type CheckoutStatus struct {
	Items    int
	Total    float64
	Offers   float64
	Delivery float64
	Ready    bool
	Blockers []string
}

// ReadCheckoutStatus reports totals and whether checkout has blockers.
func ReadCheckoutStatus(reader CheckoutReader) (CheckoutStatus, error) {
	cart, err := reader.Cart()
	if err != nil {
		return CheckoutStatus{}, err
	}
	return CheckoutStatus{
		Items:    cart.Quantity,
		Total:    cart.TotalAmount,
		Offers:   cart.OffersAmount,
		Delivery: cart.DeliveryAmount,
		Ready:    !cart.DisabledCheckout && len(cart.Blockers) == 0,
		Blockers: cart.Blockers,
	}, nil
}

// ShippingView contains the saved addresses and slots for a non-empty cart.
type ShippingView struct {
	CartEmpty bool
	Addresses map[string]map[string]any
	Slots     []domain.DeliverySlot
}

// ReadShipping returns delivery data, skipping the shipping call for an empty cart.
func ReadShipping(reader CheckoutReader) (ShippingView, error) {
	cart, err := reader.Cart()
	if err != nil {
		return ShippingView{}, err
	}
	if cart.Quantity == 0 {
		return ShippingView{CartEmpty: true}, nil
	}
	shipping, err := reader.Shipping()
	if err != nil {
		return ShippingView{}, err
	}
	return ShippingView{Addresses: shipping.Addresses, Slots: shipping.Slots}, nil
}

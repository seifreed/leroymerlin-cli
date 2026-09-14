package application

import (
	"errors"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

type checkoutReader struct {
	cart          *domain.CartDetail
	shipping      *domain.ShippingInfo
	shippingReads int
	cartErr       error
	shippingErr   error
}

func (r *checkoutReader) Cart() (*domain.CartDetail, error) { return r.cart, r.cartErr }

func (r *checkoutReader) Shipping() (*domain.ShippingInfo, error) {
	r.shippingReads++
	return r.shipping, r.shippingErr
}

func TestReadCheckoutStatus(t *testing.T) {
	status, err := ReadCheckoutStatus(&checkoutReader{cart: &domain.CartDetail{
		Quantity: 2, TotalAmount: 30, OffersAmount: 25, DeliveryAmount: 5,
		Blockers: []string{"SIMULATION_NEEDS_APPOINTMENT_DATE"},
	}})
	if err != nil || status.Ready || status.Items != 2 || len(status.Blockers) != 1 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestReadShippingSkipsEmptyCart(t *testing.T) {
	reader := &checkoutReader{cart: &domain.CartDetail{}}
	view, err := ReadShipping(reader)
	if err != nil || !view.CartEmpty || reader.shippingReads != 0 {
		t.Fatalf("view=%+v err=%v shippingReads=%d", view, err, reader.shippingReads)
	}
}

// A shipping read on an empty cart must not call the shipping endpoint at all:
// the storefront errors on it, so the empty case is answered from the cart alone.
func TestReadShippingSkipsTheShippingCallOnAnEmptyCart(t *testing.T) {
	reader := &checkoutReader{cart: &domain.CartDetail{Quantity: 0}}

	view, err := ReadShipping(reader)
	if err != nil {
		t.Fatalf("ReadShipping: %v", err)
	}
	if !view.CartEmpty {
		t.Error("want CartEmpty set")
	}
	if reader.shippingReads != 0 {
		t.Errorf("shipping was fetched %d times for an empty cart", reader.shippingReads)
	}
}

func TestReadShippingPropagatesBothFailures(t *testing.T) {
	boom := errors.New("boom")

	if _, err := ReadShipping(&checkoutReader{cartErr: boom}); err == nil {
		t.Error("want the cart error surfaced")
	}

	reader := &checkoutReader{cart: &domain.CartDetail{Quantity: 1}, shippingErr: boom}
	if _, err := ReadShipping(reader); err == nil {
		t.Error("want the shipping error surfaced")
	}
}

func TestReadCheckoutStatusPropagatesTheCartFailure(t *testing.T) {
	if _, err := ReadCheckoutStatus(&checkoutReader{cartErr: errors.New("boom")}); err == nil {
		t.Fatal("want the cart error surfaced")
	}
}

// Checkout is ready only when nothing blocks it: a disabled flag alone is
// enough to hold it, even with an empty blocker list.
func TestReadCheckoutStatusHonoursTheDisabledFlag(t *testing.T) {
	status, err := ReadCheckoutStatus(&checkoutReader{cart: &domain.CartDetail{
		Quantity: 1, DisabledCheckout: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready {
		t.Error("a disabled checkout must not report ready")
	}
}

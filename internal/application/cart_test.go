package application

import (
	"errors"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

type fakeCart struct {
	cart       *domain.CartDetail
	reads      int
	updatedID  string
	updatedQty int
	deleted    string
	readErr    error
	writeErr   error
}

func (f *fakeCart) Cart() (*domain.CartDetail, error) {
	f.reads++
	return f.cart, f.readErr
}

func (f *fakeCart) SetLineQuantity(lineID string, qty int) error {
	f.updatedID, f.updatedQty = lineID, qty
	return f.writeErr
}

func (f *fakeCart) DeleteLine(lineID string) error {
	f.deleted = lineID
	return f.writeErr
}

func TestSetCartQuantityUsesLineIDAndRefreshes(t *testing.T) {
	fake := &fakeCart{cart: &domain.CartDetail{Lines: []domain.CartLine{{LineID: "line-1", Reflm: "83085630"}}}}
	if _, err := SetCartQuantity(fake, fake, "83085630", 3); err != nil {
		t.Fatal(err)
	}
	if fake.updatedID != "line-1" || fake.updatedQty != 3 || fake.reads != 2 {
		t.Fatalf("updated=%q/%d reads=%d, want line-1/3/2", fake.updatedID, fake.updatedQty, fake.reads)
	}
}

func TestClearCartDeletesAllLines(t *testing.T) {
	fake := &fakeCart{cart: &domain.CartDetail{Lines: []domain.CartLine{{LineID: "line-1"}, {LineID: "line-2"}}}}
	count, err := ClearCart(fake, fake)
	if err != nil || count != 2 || fake.deleted != "line-2" {
		t.Fatalf("count=%d err=%v deleted=%q", count, err, fake.deleted)
	}
}

func TestSetCartQuantityZeroDeletesTheLine(t *testing.T) {
	fake := &fakeCart{cart: &domain.CartDetail{Lines: []domain.CartLine{{LineID: "line-1", Reflm: "ref"}}}}

	if _, err := SetCartQuantity(fake, fake, "ref", 0); err != nil {
		t.Fatal(err)
	}
	if fake.deleted != "line-1" {
		t.Errorf("deleted = %q, want line-1", fake.deleted)
	}
	if fake.updatedID != "" {
		t.Errorf("quantity was also updated: %q", fake.updatedID)
	}
}

// An unknown reference is reported as ErrCartLineNotFound so callers can tell it
// apart from a transport failure, and no write is attempted.
func TestSetCartQuantityRejectsAnUnknownRef(t *testing.T) {
	fake := &fakeCart{cart: &domain.CartDetail{Lines: []domain.CartLine{{LineID: "line-1", Reflm: "ref"}}}}

	_, err := SetCartQuantity(fake, fake, "otra", 2)
	if !errors.Is(err, ErrCartLineNotFound) {
		t.Fatalf("err = %v, want ErrCartLineNotFound", err)
	}
	if fake.updatedID != "" || fake.deleted != "" {
		t.Error("a write was attempted for an unknown ref")
	}
}

func TestSetCartQuantityPropagatesReadAndWriteFailures(t *testing.T) {
	boom := errors.New("boom")

	if _, err := SetCartQuantity(&fakeCart{readErr: boom}, &fakeCart{}, "ref", 1); err == nil {
		t.Error("want the cart read error surfaced")
	}

	fake := &fakeCart{
		cart:     &domain.CartDetail{Lines: []domain.CartLine{{LineID: "line-1", Reflm: "ref"}}},
		writeErr: boom,
	}
	if _, err := SetCartQuantity(fake, fake, "ref", 2); err == nil {
		t.Error("want the write error surfaced")
	}
}

// A nil cart must be handled, not dereferenced: the storefront can answer 200
// with no body.
func TestSetCartQuantityHandlesANilCart(t *testing.T) {
	fake := &fakeCart{cart: nil}

	if _, err := SetCartQuantity(fake, fake, "ref", 1); !errors.Is(err, ErrCartLineNotFound) {
		t.Fatalf("err = %v, want ErrCartLineNotFound for a nil cart", err)
	}
}

func TestClearCartStopsAtTheFirstDeleteFailure(t *testing.T) {
	boom := errors.New("boom")
	fake := &fakeCart{
		cart:     &domain.CartDetail{Lines: []domain.CartLine{{LineID: "l1"}, {LineID: "l2"}}},
		writeErr: boom,
	}

	n, err := ClearCart(fake, fake)
	if err == nil {
		t.Fatal("want the delete error surfaced")
	}
	if n != 0 {
		t.Errorf("removed = %d, want 0 on failure", n)
	}
}

func TestClearCartOnAnEmptyCartRemovesNothing(t *testing.T) {
	fake := &fakeCart{cart: &domain.CartDetail{}}

	n, err := ClearCart(fake, fake)
	if err != nil || n != 0 {
		t.Fatalf("ClearCart = %d, %v; want 0, nil", n, err)
	}
}

// Package application contains use cases independent of CLI presentation and
// storefront transport details.
package application

import (
	"errors"
	"fmt"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// ErrCartLineNotFound identifies a reference that is not present in the cart.
var ErrCartLineNotFound = errors.New("cart line not found")

// CartReader is the part of the storefront needed by cart use cases.
type CartReader interface {
	Cart() (*domain.CartDetail, error)
}

// CartWriter mutates one existing cart line.
type CartWriter interface {
	SetLineQuantity(lineID string, qty int) error
	DeleteLine(lineID string) error
}

// SetCartQuantity applies an absolute quantity and returns the refreshed cart.
func SetCartQuantity(reader CartReader, writer CartWriter, ref string, qty int) (*domain.CartDetail, error) {
	cart, err := reader.Cart()
	if err != nil {
		return nil, err
	}
	line, ok := lineByRef(cart, ref)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrCartLineNotFound, ref)
	}
	if qty == 0 {
		err = writer.DeleteLine(line.LineID)
	} else {
		err = writer.SetLineQuantity(line.LineID, qty)
	}
	if err != nil {
		return nil, err
	}
	return reader.Cart()
}

// ClearCart removes every existing cart line and returns the number removed.
func ClearCart(reader CartReader, writer CartWriter) (int, error) {
	cart, err := reader.Cart()
	if err != nil {
		return 0, err
	}
	for _, line := range cart.Lines {
		if err := writer.DeleteLine(line.LineID); err != nil {
			return 0, err
		}
	}
	return len(cart.Lines), nil
}

func lineByRef(cart *domain.CartDetail, ref string) (domain.CartLine, bool) {
	if cart == nil {
		return domain.CartLine{}, false
	}
	for _, line := range cart.Lines {
		if line.Reflm == ref {
			return line, true
		}
	}
	return domain.CartLine{}, false
}

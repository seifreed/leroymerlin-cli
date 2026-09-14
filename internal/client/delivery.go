package client

import (
	"cmp"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

const shippingPath = "/checkout/backend/shipping"

// rawShipping projects the fields Shipping reads from /checkout/backend/shipping.
type rawShipping struct {
	Addresses       map[string]map[string]any `json:"addresses"`
	DeliveryVendors []struct {
		Name                         string `json:"name"`
		DeliveryVendorDeliveryGroups []struct {
			DeliveryVendorServiceLevels []struct {
				Mode             string  `json:"mode"`
				LabelCode        string  `json:"labelCode"`
				Amount           float64 `json:"amount"`
				Selected         bool    `json:"selected"`
				AppointmentDate  string  `json:"appointmentDate"`
				DeliveryDateInfo struct {
					FixedDate string `json:"fixedDate"`
					StartDate string `json:"startDate"`
				} `json:"deliveryDateInfo"`
			} `json:"deliveryVendorServiceLevels"`
		} `json:"deliveryVendorDeliveryGroups"`
	} `json:"deliveryVendors"`
}

// Shipping fetches the checkout shipping page: saved addresses and delivery
// slots. Slot modes are PICKUP_IN_STORE, HOME_DELIVERY and RELAY_POINT; the
// address map is keyed by role — invoiceAddress, deliveryAddress,
// installationAddress, relayPointAddress. Needs a populated cart and an imported cookie (the endpoint is
// DataDome-protected, same as the cart).
func (c *Client) Shipping() (*domain.ShippingInfo, error) {
	var rs rawShipping
	if err := c.getJSON(shippingPath, &rs); err != nil {
		return nil, err
	}
	info := &domain.ShippingInfo{Addresses: scrubAddresses(rs.Addresses)}
	// Key on the option itself, not on whether it happens to be the chosen one.
	// The storefront reports a slot once per delivery group, so the selected
	// option arrives twice — once selected, once not — and listing both reads as
	// two identical choices, one of them inexplicably unmarked.
	type option struct {
		vendor, mode, label, date string
		amount                    float64
	}
	at := map[option]int{}
	for _, v := range rs.DeliveryVendors {
		for _, g := range v.DeliveryVendorDeliveryGroups {
			for _, sl := range g.DeliveryVendorServiceLevels {
				date := cmp.Or(sl.AppointmentDate, sl.DeliveryDateInfo.FixedDate, sl.DeliveryDateInfo.StartDate)
				slot := domain.DeliverySlot{
					Mode:     scrubText(sl.Mode),
					Label:    scrubText(sl.LabelCode),
					Amount:   sl.Amount,
					Date:     scrubText(date),
					Vendor:   scrubText(v.Name),
					Selected: sl.Selected,
				}
				key := option{slot.Vendor, slot.Mode, slot.Label, slot.Date, slot.Amount}
				if i, ok := at[key]; ok {
					info.Slots[i].Selected = info.Slots[i].Selected || slot.Selected
					continue
				}
				at[key] = len(info.Slots)
				info.Slots = append(info.Slots, slot)
			}
		}
	}
	return info, nil
}

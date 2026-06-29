package client

const shippingPath = "/checkout/backend/shipping"

// DeliverySlot is one delivery/pickup option from the shipping page: its mode
// (PICKUP_IN_STORE, HOME_DELIVERY, RELAY_POINT…), a label code, its cost, the
// date it would happen, and whether it's the currently selected one.
type DeliverySlot struct {
	Mode     string  `json:"mode"`
	Label    string  `json:"label"`
	Amount   float64 `json:"amount"`
	Date     string  `json:"date"`
	Selected bool    `json:"selected"`
}

// ShippingInfo is the checkout shipping page: the order's addresses keyed by
// role (invoiceAddress, deliveryAddress, installationAddress, relayPointAddress)
// and the available delivery slots.
type ShippingInfo struct {
	Addresses map[string]map[string]any `json:"addresses"`
	Slots     []DeliverySlot            `json:"slots"`
}

// rawShipping projects the fields Shipping reads from /checkout/backend/shipping.
type rawShipping struct {
	Addresses       map[string]map[string]any `json:"addresses"`
	DeliveryVendors []struct {
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
// slots. Needs a populated cart and an imported cookie (the endpoint is
// DataDome-protected, same as the cart).
func (c *Client) Shipping() (*ShippingInfo, error) {
	var rs rawShipping
	if err := c.getJSON(shippingPath, &rs); err != nil {
		return nil, err
	}
	info := &ShippingInfo{Addresses: rs.Addresses}
	for _, v := range rs.DeliveryVendors {
		for _, g := range v.DeliveryVendorDeliveryGroups {
			for _, sl := range g.DeliveryVendorServiceLevels {
				date := firstNonEmptyStr(sl.AppointmentDate, sl.DeliveryDateInfo.FixedDate, sl.DeliveryDateInfo.StartDate)
				info.Slots = append(info.Slots, DeliverySlot{
					Mode:     sl.Mode,
					Label:    sl.LabelCode,
					Amount:   sl.Amount,
					Date:     date,
					Selected: sl.Selected,
				})
			}
		}
	}
	return info, nil
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

package main

import (
	"fmt"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/application"
)

// cmdCheckout reports checkout readiness, or (with a subcommand) the saved
// delivery addresses or available delivery slots. All read-only — payment is
// never automated.
func cmdCheckout(args []string) error {
	// A leading non-flag word is a subcommand; anything else is a flag for the
	// default status view. A typo must not silently print a different report.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "addresses":
			return checkoutAddresses(args[1:])
		case "slots":
			return checkoutSlots(args[1:])
		case "status":
			args = args[1:] // `checkout` and `checkout status` are the same view
		default:
			return fmt.Errorf("unknown checkout subcommand %q (want status|addresses|slots)", args[0])
		}
	}
	return checkoutStatus(args)
}

// checkoutStatus reports whether the cart can proceed to checkout: its totals and
// the blockers the cart simulation reports. A guest cart always blocks on
// customer/address fields; logging in clears those.
func checkoutStatus(args []string) error {
	fs, cf := newCommonFlags("checkout")
	parseFlags(fs, args)

	cl := newClient()
	warnWithoutSession(cl)
	status, err := application.ReadCheckoutStatus(cl)
	if err != nil {
		return cleanCartErr(err)
	}
	if err := persistClientSession(cl); err != nil {
		return err
	}
	if done, err := emitStructured(cf, map[string]any{
		"items": status.Items, "total": status.Total, "offers": status.Offers,
		"delivery": status.Delivery, "ready": status.Ready, "blockers": status.Blockers,
	}); done {
		return err
	}

	if status.Items == 0 {
		fmt.Println("cart is empty — nothing to check out")
		return nil
	}
	fmt.Printf("total: %s  (productos %s + envío %s)  ·  %s\n",
		eur(status.Total), eur(status.Offers), eur(status.Delivery),
		plural(status.Items, "artículo", "artículos"))
	if status.Ready {
		fmt.Println("checkout: ready ✓")
		return nil
	}
	fmt.Println("checkout: blocked")
	for _, b := range status.Blockers {
		fmt.Printf("  - %s\n", humanBlocker(b))
	}
	return nil
}

// humanBlocker turns a SIMULATION_NEEDS_* / ORDER_* code into a short hint. Most
// guest-cart blockers mean "log in and add delivery details to continue".
func humanBlocker(code string) string {
	switch code {
	case "ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER":
		return "needs an account (log in)"
	default:
		c := strings.TrimPrefix(code, "SIMULATION_NEEDS_")
		c = strings.ReplaceAll(strings.ToLower(c), "_", " ")
		return "needs " + c
	}
}

// checkoutAddresses lists the customer's saved delivery addresses (from the
// shipping page). Needs a populated cart and your session cookie.
func checkoutAddresses(args []string) error {
	fs, cf := newCommonFlags("checkout addresses")
	parseFlags(fs, args)
	view, err := loadShipping("addresses")
	if err != nil {
		return err
	}
	if view.CartEmpty {
		if done, err := emitStructured(cf, map[string]any{}); done {
			return err
		}
		fmt.Println("no addresses set for this cart (cart is empty)")
		return nil
	}
	if done, err := emitStructured(cf, view.Addresses); done {
		return err
	}
	// Render the address roles in a stable, meaningful order.
	roles := []struct{ key, label string }{
		{"deliveryAddress", "entrega"},
		{"invoiceAddress", "factura"},
		{"installationAddress", "instalación"},
		{"relayPointAddress", "punto de recogida"},
	}
	shown := 0
	for _, r := range roles {
		a := view.Addresses[r.key]
		if line := addressLine(a); line != "" {
			fmt.Printf("  %-18s %s\n", r.label+":", line)
			shown++
		}
	}
	if shown == 0 {
		fmt.Println("no addresses set for this cart (store pickup selected, or none saved)")
	}
	return nil
}

// checkoutSlots lists the available delivery/pickup options for the cart, with
// their dates and costs (the ★ marks the currently selected one).
func checkoutSlots(args []string) error {
	fs, cf := newCommonFlags("checkout slots")
	parseFlags(fs, args)
	view, err := loadShipping("slots")
	if err != nil {
		return err
	}
	if view.CartEmpty {
		if done, err := emitStructured(cf, []any{}); done {
			return err
		}
		fmt.Println("no delivery slots offered (cart is empty)")
		return nil
	}
	if done, err := emitStructured(cf, view.Slots); done {
		return err
	}
	if len(view.Slots) == 0 {
		fmt.Println("no delivery slots offered (empty cart, or none available right now)")
		return nil
	}
	for _, s := range view.Slots {
		mark := "  "
		if s.Selected {
			mark = "★ "
		}
		cost := "gratis"
		if s.Amount > 0 {
			cost = eur(s.Amount)
		}
		fmt.Printf("%s%-28s %s  %s\n", mark, strings.ToLower(strings.ReplaceAll(s.Label, "_", " ")), shortDate(s.Date), cost)
	}
	return nil
}

// loadShipping fetches the checkout shipping page for a view that needs it:
// require a session, read, then persist the (possibly rotated) cookie.
func loadShipping(operation string) (application.ShippingView, error) {
	cl, err := requireSession(operation)
	if err != nil {
		return application.ShippingView{}, err
	}
	view, err := application.ReadShipping(cl)
	if err != nil {
		return application.ShippingView{}, cleanCartErr(err)
	}
	if err := persistClientSession(cl); err != nil {
		return application.ShippingView{}, err
	}
	return view, nil
}

// addressLine renders one address (the ADEO checkout schema: firstName/lastName,
// line1–line4, postalCode, city, province…) as a single comma-joined line, in a
// stable order, skipping empty fields. Returns "" for a nil/empty address.
func addressLine(a map[string]any) string {
	if len(a) == 0 {
		return ""
	}
	order := []string{"firstName", "lastName", "corporateName", "line1", "line2", "line3", "line4", "postalCode", "city", "province", "countryCode", "phoneNumber"}
	var parts []string
	seen := map[string]bool{}
	for _, k := range order {
		if v, ok := a[k].(string); ok {
			if v = strings.TrimSpace(v); v != "" && !seen[v] {
				seen[v] = true
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// shortDate trims an ISO timestamp to "YYYY-MM-DD HH:MM" for display.
func shortDate(iso string) string {
	if len(iso) < 16 {
		return iso
	}
	return iso[:10] + " " + iso[11:16]
}

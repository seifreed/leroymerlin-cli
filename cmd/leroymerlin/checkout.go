package main

import (
	"fmt"
	"strings"
)

// cmdCheckout reports whether the cart can proceed to checkout: its totals and
// the blockers the cart simulation reports. It is read-only — payment is not
// automated. A guest cart always blocks on customer/address fields; logging in
// (importing a session cookie tied to your account) clears those.
func cmdCheckout(args []string) error {
	if len(args) > 0 && args[0] == "status" {
		args = args[1:] // accept `checkout` and `checkout status` alike
	}
	fs, cf := newCommonFlags("checkout")
	parseFlags(fs, args)

	cl := newClient(cf)
	if !cl.LoadAuth() {
		stderrLogf("no cookie cached — the cart is tied to your browser session; run `leroymerlin login --from-browser chrome` first")
	}
	cart, err := cl.Cart()
	if err != nil {
		return cleanCartErr(err)
	}

	ready := !cart.DisabledCheckout && len(cart.Blockers) == 0
	if done, err := emitStructured(cf, map[string]any{
		"items":    cart.Quantity,
		"total":    cart.TotalAmount,
		"offers":   cart.OffersAmount,
		"delivery": cart.DeliveryAmount,
		"ready":    ready,
		"blockers": cart.Blockers,
	}); done {
		return err
	}

	if cart.Quantity == 0 {
		fmt.Println("cart is empty — nothing to check out")
		return nil
	}
	fmt.Printf("total: %s  (productos %s + envío %s)  ·  %s\n",
		eur(cart.TotalAmount), eur(cart.OffersAmount), eur(cart.DeliveryAmount),
		plural(cart.Quantity, "artículo", "artículos"))
	if ready {
		fmt.Println("checkout: ready ✓")
		return nil
	}
	fmt.Println("checkout: blocked")
	for _, b := range cart.Blockers {
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

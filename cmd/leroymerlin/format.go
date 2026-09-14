package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// eur formats a euro amount as "12.99€".
func eur(amount float64) string {
	return fmt.Sprintf("%.2f€", amount)
}

// productLine renders a one-line search hit:
// "[19557783] Name — 11.79€ (4.7★)  ⟨promo⟩  [marketplace]".
func productLine(p domain.Product) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s — %s", p.Identifier, strings.TrimSpace(p.Name), eur(p.Offer.UnitPriceATI))
	if p.Offer.InitialPrice != nil && *p.Offer.InitialPrice > p.Offer.UnitPriceATI {
		fmt.Fprintf(&b, " (antes %s)", eur(*p.Offer.InitialPrice))
	}
	if p.Rating > 0 {
		fmt.Fprintf(&b, " (%.1f★)", p.Rating)
	}
	if p.Sponsored {
		b.WriteString("  (patrocinado)")
	}
	if p.Offer.SellerType == "3P" {
		seller := p.Offer.SellerName
		if seller == "" {
			seller = "marketplace"
		}
		fmt.Fprintf(&b, "  [%s]", seller)
	}
	if !p.Offer.AddToCart {
		b.WriteString("  [no disponible]")
	}
	if promo := p.Offer.Promo(); promo != "" {
		fmt.Fprintf(&b, "  ⟨%s⟩", promo)
	}
	return b.String()
}

// printProducts writes one line per product, or emptyMsg to stderr when the
// result set is empty (stderr so --json consumers never see it on stdout).
func printProducts(products []domain.Product, emptyMsg string) {
	if len(products) == 0 {
		fmt.Fprintln(os.Stderr, emptyMsg)
		return
	}
	for _, product := range products {
		fmt.Println(productLine(product))
	}
}

// detailLines renders a product page as a human-readable block.
func detailLines(d *domain.ProductDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", strings.TrimSpace(d.Name))
	if d.Brand != "" {
		fmt.Fprintf(&b, "  marca:        %s\n", d.Brand)
	}
	fmt.Fprintf(&b, "  ref:          %s\n", d.SKU)
	if price := d.Price(); price != "" {
		// The page writes 219 euros as "219" and sixty cents as "0.6"; render from
		// cents so every price in the CLI reads the same way.
		if cents, err := domain.ParsePriceCents(price); err == nil {
			price = domain.FormatCents(cents)
		}
		fmt.Fprintf(&b, "  precio:       %s %s\n", price, d.Currency())
	}
	if av := d.Availability(); av != "" {
		fmt.Fprintf(&b, "  disponible:   %s\n", av)
	}
	if d.Rating != nil && d.Rating.Value != "" {
		fmt.Fprintf(&b, "  valoración:   %s (%s reseñas)\n", ratingValue(d.Rating.Value), d.Rating.Count)
	}
	if d.GTIN != "" {
		fmt.Fprintf(&b, "  gtin:         %s\n", d.GTIN)
	}
	if desc := strings.TrimSpace(d.Description); desc != "" {
		fmt.Fprintf(&b, "  %s\n", truncateLine(desc, 300))
	}
	if len(d.Specs) > 0 {
		fmt.Fprintf(&b, "  características:\n")
		for _, s := range d.Specs {
			fmt.Fprintf(&b, "    %s: %s\n", s.Label, s.Value)
		}
	}
	if len(d.Deliveries) > 0 {
		fmt.Fprintf(&b, "  disponibilidad:\n")
		for _, dl := range d.Deliveries {
			cost := "gratis"
			if dl.Price > 0 {
				cost = eur(dl.Price)
			}
			fmt.Fprintf(&b, "    %-22s %d uds  %s  %s\n", deliveryLabel(dl.Type), dl.Stock, cost, leadTime(dl.Time))
		}
	}
	fmt.Fprintf(&b, "  %s\n", d.URL)
	return b.String()
}

// ratingValue renders a schema.org ratingValue for a human. The storefront sends
// the raw average — "4.5721393034825875" — where the search listing shows one
// decimal, so this matches it. Anything that is not a number is left alone, and
// --json keeps the site's value untouched.
func ratingValue(s string) string {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// deliveryLabel turns an available_deliveries type into a Spanish label.
func deliveryLabel(t string) string {
	switch t {
	case "storeDelivery":
		return "recogida en tienda:"
	case "homeDelivery":
		return "envío a domicilio:"
	case "relayDelivery":
		return "punto de recogida:"
	default:
		return t + ":"
	}
}

// leadTime turns "2 HOUR" / "1 OPENING_DAY" into "en 2 h" / "en 1 día lab.".
func leadTime(t string) string {
	switch {
	case strings.HasSuffix(t, "HOUR"):
		return "en " + strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(t, "HOUR")), " ") + " h"
	case strings.HasSuffix(t, "OPENING_DAY"):
		return "en " + strings.TrimSpace(strings.TrimSuffix(t, "OPENING_DAY")) + " día lab."
	default:
		return t
	}
}

func truncateLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

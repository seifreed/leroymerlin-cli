package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/application"
	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/config"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// cmdCart manages the session cart. Reads (get) work with any session cookie;
// writes (add) bind to that session, so an imported cookie is required
// (`leroymerlin login --from-browser …`). The cart is a guest cart — no account
// or purchase is involved.
func cmdCart(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: leroymerlin cart <get|add|set|clear> …")
	}
	switch args[0] {
	case "get":
		return cartGet(args[1:])
	case "add":
		return cartAdd(args[1:])
	case "set":
		return cartSet(args[1:])
	case "clear":
		return cartClear(args[1:])
	default:
		return fmt.Errorf("unknown cart subcommand %q (want get|add|set|clear)", args[0])
	}
}

func cartGet(args []string) error {
	fs, cf := newCommonFlags("cart get")
	parseFlags(fs, args)
	cl, err := requireSession("cart reads")
	if err != nil {
		return err
	}
	cart, err := cl.Cart()
	if err != nil {
		return cleanCartErr(err)
	}
	if err := persistClientSession(cl); err != nil {
		return err
	}
	if done, err := emitStructured(cf, cart); done {
		return err
	}
	printCart(cart)
	return nil
}

// printCart renders the detailed cart: one line per item, then the totals.
func printCart(cart *domain.CartDetail) {
	if len(cart.Lines) == 0 {
		fmt.Println("cart is empty")
		return
	}
	for _, l := range cart.Lines {
		fmt.Printf("  [%s] %s — %d × = %s\n", l.Reflm, strings.TrimSpace(l.Name), l.Quantity, eur(l.Price))
	}
	fmt.Printf("  ─ artículos: %s  ·  productos: %s  ·  envío: %s  ·  total: %s\n",
		fmt.Sprintf("%d", cart.Quantity), eur(cart.OffersAmount), eur(cart.DeliveryAmount), eur(cart.TotalAmount))
}

// cartSet sets a product's absolute quantity in the cart (0 removes it). The
// product is identified by its reflm (the [ref] shown by search / cart get).
func cartSet(args []string) error {
	fs, cf := newCommonFlags("cart set")
	parseFlags(fs, args)
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: leroymerlin cart set <ref> <qty>  (ref is the [number] from `cart get`; qty 0 removes)")
	}
	ref := rest[0]
	qty, perr := strconv.Atoi(rest[1])
	if perr != nil || qty < 0 {
		return fmt.Errorf("invalid qty %q (want a non-negative integer)", rest[1])
	}

	cl, err := requireSession("cart writes")
	if err != nil {
		return err
	}
	updated, err := application.SetCartQuantity(cl, cl, ref, qty)
	if err != nil {
		if errors.Is(err, application.ErrCartLineNotFound) {
			return fmt.Errorf("ref %q is not in the cart — `leroymerlin cart get` to see what is", ref)
		}
		return cleanCartErr(err)
	}
	if err := persistClientSession(cl); err != nil {
		return err
	}
	if done, err := emitStructured(cf, updated); done {
		return err
	}
	printCart(updated)
	return nil
}

// cartClear removes every line from the cart.
func cartClear(args []string) error {
	fs, cf := newCommonFlags("cart clear")
	parseFlags(fs, args)
	cl, err := requireSession("cart writes")
	if err != nil {
		return err
	}
	cleared, err := application.ClearCart(cl, cl)
	if err != nil {
		return cleanCartErr(err)
	}
	if err := persistClientSession(cl); err != nil {
		return err
	}
	if done, err := emitStructured(cf, map[string]any{"cleared": cleared}); done {
		return err
	}
	fmt.Printf("cleared %s\n", plural(cleared, "línea", "líneas"))
	return nil
}

// cleanCartErr maps a DataDome 403 on the (heavily protected) cart endpoints to
// an actionable hint instead of dumping the captcha-challenge body.
func cleanCartErr(err error) error {
	status, ok := client.HTTPStatus(err)
	if !ok {
		return err
	}
	switch status {
	case 403:
		return fmt.Errorf("DataDome challenged the cart request (HTTP 403) — the cart endpoints need a fresh browser cookie; run `leroymerlin login --from-browser chrome`")
	case 412:
		return fmt.Errorf("leroy Merlin rejected the cart operation (HTTP 412) — open /checkout/cart in the browser, refresh it, then retry")
	}
	return err
}

func cartAdd(args []string) error {
	fs, cf := newCommonFlags("cart add")
	maxFlag := fs.Float64("max", -1, "refuse a line over this many euros (also LEROYMERLIN_MAX_EUR / [limits] max_eur)")
	parseFlags(fs, args)
	maxSet := false
	fs.Visit(func(f *flag.Flag) {
		maxSet = maxSet || f.Name == "max"
	})

	rest := fs.Args()
	if len(rest) < 1 {
		return fmt.Errorf("usage: leroymerlin cart add <product-url> [qty]  (url from `leroymerlin search`)")
	}
	target := rest[0]
	if !strings.Contains(target, "/productos/") {
		return fmt.Errorf("expected a product url like /productos/...-<ref>.html, got %q", target)
	}
	qty := 1
	if len(rest) >= 2 {
		q, perr := strconv.Atoi(rest[1])
		if perr != nil || q <= 0 {
			return fmt.Errorf("invalid qty %q (want a positive integer)", rest[1])
		}
		qty = q
	}

	cl, err := requireSession("cart writes")
	if err != nil {
		return err
	}

	reflm, offerID, contextCode, detail, err := cl.ProductOffer(target)
	if err != nil {
		if isProductMissing(err) {
			return productMissingErr(target)
		}
		return cleanCartErr(err)
	}

	// Spending guard: price the line and refuse before writing if it exceeds the cap.
	maxEUR, err := resolveMax(*maxFlag, maxSet)
	if err != nil {
		return err
	}
	if err := enforceMaxLine(detail, qty, maxEUR); err != nil {
		return err
	}

	sum, err := cl.AddToCart(reflm, offerID, contextCode, qty)
	if err != nil {
		return cleanCartErr(err)
	}
	if err := persistClientSession(cl); err != nil {
		return err
	}
	if done, err := emitStructured(cf, sum); done {
		return err
	}
	name := reflm
	if detail != nil && detail.Name != "" {
		name = strings.TrimSpace(detail.Name)
	}
	fmt.Printf("added %d × %s → cart now has %s\n", qty, name, plural(sum.Quantity, "artículo", "artículos"))
	return nil
}

// enforceMaxLine refuses a cart write whose line total would exceed maxEUR euros.
// A zero or negative maxEUR disables the guard. Prices are compared in integer
// cents so the check never drifts.
func enforceMaxLine(detail *domain.ProductDetail, qty int, maxEUR float64) error {
	if maxEUR <= 0 {
		return nil
	}
	if detail == nil {
		return fmt.Errorf("cannot enforce --max: product price unavailable")
	}
	unit, err := domain.ParsePriceCents(detail.Price())
	if err != nil {
		return fmt.Errorf("cannot enforce --max: %w", err)
	}
	line, err := domain.MultiplyCents(unit, float64(qty))
	if err != nil {
		return fmt.Errorf("cannot enforce --max: %w", err)
	}
	capCents, err := domain.EurosToCents(maxEUR)
	if err != nil {
		return fmt.Errorf("cannot enforce --max: %w", err)
	}
	if line > capCents {
		return fmt.Errorf("line %s€ exceeds --max %.2f€ — not added (raise --max to override)", domain.FormatCents(line), maxEUR)
	}
	return nil
}

// resolveMax resolves the spending cap by precedence: --max > env > config.
// Zero means "no cap"; invalid values are errors because they must not bypass
// the spending guard.
func resolveMax(flagVal float64, flagSet bool) (float64, error) {
	if flagSet {
		return validateMax(flagVal, "--max")
	}
	if v := os.Getenv("LEROYMERLIN_MAX_EUR"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid LEROYMERLIN_MAX_EUR %q: %w", v, err)
		}
		return validateMax(f, "LEROYMERLIN_MAX_EUR")
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return 0, fmt.Errorf("load spending limit: %w", err)
	}
	return validateMax(cfg.Limits.MaxEUR, "[limits] max_eur")
}

func validateMax(value float64, source string) (float64, error) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid spending limit %s=%v (want a finite non-negative number)", source, value)
	}
	return value, nil
}

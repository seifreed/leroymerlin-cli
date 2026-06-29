package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/config"
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
	cl := newClient(cf)
	if !cl.LoadAuth() {
		stderrLogf("no cookie cached — a cart is tied to your browser session; run `leroymerlin login --from-browser chrome` first")
	}
	cart, err := cl.Cart()
	if err != nil {
		return cleanCartErr(err)
	}
	if done, err := emitStructured(cf, cart); done {
		return err
	}
	printCart(cart)
	return nil
}

// printCart renders the detailed cart: one line per item, then the totals.
func printCart(cart *client.CartDetail) {
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

	cl := newClient(cf)
	if !cl.LoadAuth() {
		return fmt.Errorf("cart writes need your browser session — run `leroymerlin login --from-browser chrome` first")
	}
	cart, err := cl.Cart()
	if err != nil {
		return cleanCartErr(err)
	}
	line := findLine(cart, ref)
	if line == nil {
		return fmt.Errorf("ref %q is not in the cart — `leroymerlin cart get` to see what is", ref)
	}
	if qty == 0 {
		err = cl.DeleteLine(line.LineID)
	} else {
		err = cl.SetLineQuantity(line.LineID, qty)
	}
	if err != nil {
		return cleanCartErr(err)
	}
	updated, err := cl.Cart()
	if err != nil {
		return cleanCartErr(err)
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
	cl := newClient(cf)
	if !cl.LoadAuth() {
		return fmt.Errorf("cart writes need your browser session — run `leroymerlin login --from-browser chrome` first")
	}
	cart, err := cl.Cart()
	if err != nil {
		return cleanCartErr(err)
	}
	for _, l := range cart.Lines {
		if derr := cl.DeleteLine(l.LineID); derr != nil {
			return cleanCartErr(derr)
		}
	}
	if done, err := emitStructured(cf, map[string]any{"cleared": len(cart.Lines)}); done {
		return err
	}
	fmt.Printf("cleared %s\n", plural(len(cart.Lines), "línea", "líneas"))
	return nil
}

// findLine returns the cart line whose reflm matches ref, or nil.
func findLine(cart *client.CartDetail, ref string) *client.CartLine {
	for i := range cart.Lines {
		if cart.Lines[i].Reflm == ref {
			return &cart.Lines[i]
		}
	}
	return nil
}

// cleanCartErr maps a DataDome 403 on the (heavily protected) cart endpoints to
// an actionable hint instead of dumping the captcha-challenge body.
func cleanCartErr(err error) error {
	if status, ok := client.HTTPStatus(err); ok && status == 403 {
		return fmt.Errorf("DataDome challenged the cart request (HTTP 403) — the cart endpoints need a fresh browser cookie; run `leroymerlin login --from-browser chrome`")
	}
	return err
}

func cartAdd(args []string) error {
	fs, cf := newCommonFlags("cart add")
	maxFlag := fs.Float64("max", -1, "refuse a line over this many euros (also LEROYMERLIN_MAX_EUR / [limits] max_eur)")
	parseFlags(fs, args)

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

	cl := newClient(cf)
	if !cl.LoadAuth() {
		return fmt.Errorf("cart writes need your browser session — run `leroymerlin login --from-browser chrome` (or import-har / set-cookie) first")
	}

	reflm, offerID, detail, err := cl.ProductOffer(target)
	if err != nil {
		return err
	}

	// Spending guard: price the line and refuse before writing if it exceeds the cap.
	if max := resolveMax(*maxFlag); max > 0 && detail != nil {
		if cents, cerr := priceCents(detail.Price()); cerr == nil {
			line := lineCents(cents, float64(qty))
			if line > int64(max*100+0.5) {
				return fmt.Errorf("line %s€ exceeds --max %.2f€ — not added (raise --max to override)", centsStr(line), max)
			}
		}
	}

	sum, err := cl.AddToCart(reflm, offerID, qty)
	if err != nil {
		return cleanCartErr(err)
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

// resolveMax resolves the spending cap by precedence: --max flag (>=0) > env
// LEROYMERLIN_MAX_EUR > config [limits] max_eur. Returns 0 for "no cap".
func resolveMax(flagVal float64) float64 {
	if flagVal >= 0 {
		return flagVal
	}
	if v := os.Getenv("LEROYMERLIN_MAX_EUR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	cfg, _ := config.LoadConfig()
	return cfg.Limits.MaxEUR
}

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
		return fmt.Errorf("usage: leroymerlin cart <get|add> …")
	}
	switch args[0] {
	case "get":
		return cartGet(args[1:])
	case "add":
		return cartAdd(args[1:])
	default:
		return fmt.Errorf("unknown cart subcommand %q (want get|add)", args[0])
	}
}

func cartGet(args []string) error {
	fs, cf := newCommonFlags("cart get")
	parseFlags(fs, args)
	cl := newClient(cf)
	if !cl.LoadAuth() {
		stderrLogf("no cookie cached — a cart is tied to your browser session; run `leroymerlin login --from-browser chrome` first")
	}
	sum, err := cl.CartData()
	if err != nil {
		return cleanCartErr(err)
	}
	if done, err := emitStructured(cf, sum); done {
		return err
	}
	fmt.Printf("cart: %s  (order %s)\n", plural(sum.Quantity, "artículo", "artículos"), firstNonEmpty(sum.Order, "—"))
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

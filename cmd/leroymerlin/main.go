// Command leroymerlin is an unofficial, agent-friendly CLI for leroymerlin.es.
//
// The site has no public API and server-renders its pages behind DataDome bot
// protection. This CLI presents Chrome's TLS fingerprint (uTLS) so DataDome
// keeps it off the JS-challenge path, fetches the same SSR HTML the browser
// gets, and lifts the structured data the page already embeds: full-text
// product search and product detail (price, brand, rating, availability).
// Every command supports --json (data to stdout, logs to stderr) for agents.
package main

import (
	"fmt"
	"os"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// Build metadata, injected at release time via -ldflags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() { os.Exit(run(os.Args[1:])) }

// run dispatches a command and returns the process exit code. Split from main so
// it is testable without os.Exit.
func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	var err error
	switch args[0] {
	case "search":
		err = cmdSearch(args[1:])
	case "batch":
		err = cmdBatch(args[1:])
	case "total":
		err = cmdTotal(args[1:])
	case "brands":
		err = cmdBrands(args[1:])
	case "categories":
		err = cmdCategories(args[1:])
	case "product":
		err = cmdProduct(args[1:])
	case "login":
		err = cmdLogin(args[1:])
	case "whoami":
		err = cmdWhoami(args[1:])
	case "cart":
		err = cmdCart(args[1:])
	case "checkout":
		err = cmdCheckout(args[1:])
	case "import-har":
		err = cmdImportHar(args[1:])
	case "set-cookie":
		err = cmdSetCookie(args[1:])
	case "version", "--version", "-v":
		fmt.Println(versionString())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", explain(err))
		return 1
	}
	return 0
}

// explain turns a bare WAF rejection into the action that fixes it. A 403 from
// the storefront is DataDome asking for a browser, and printing its challenge
// page — "Please enable JS and disable any ad blocker" — tells the user nothing
// they can act on. The cart commands already map their own 403, which is more
// specific, and reaches here as a plain error.
func explain(err error) error {
	if status, ok := client.HTTPStatus(err); ok && status == 403 {
		return fmt.Errorf("the storefront challenged this request (HTTP 403) — run " +
			"`leroymerlin login --from-browser chrome` to lift your browser's cookie, then retry")
	}
	return err
}

func versionString() string {
	if commit == "" {
		return version
	}
	if date != "" {
		return fmt.Sprintf("%s (%s, %s)", version, commit, date)
	}
	return fmt.Sprintf("%s (%s)", version, commit)
}

func usage() {
	fmt.Fprint(os.Stderr, `leroymerlin — unofficial CLI for leroymerlin.es

USAGE:
  leroymerlin <command> [flags]

SESSION (do this first — every command runs on your browser session):
  login                   lift the session from a browser you already use.
                          REQUIRED: be signed in at www.leroymerlin.es in that
                          browser and have loaded a page there — a fresh browser
                          has no session and DataDome will challenge it.
                          --from-browser b  read just one store
                          (chrome|chromium|firefox|safari|edge|brave)
  import-har --file f     lift the cookie from a DevTools HAR ("Save all as HAR
                          with sensitive data"). --file - reads stdin.
  set-cookie '<cookie>'   paste a raw Cookie header from DevTools. --stdin too.
  whoami                  check the session is still being accepted

READ COMMANDS:
  search <term...>        full-text product search
                          --limit N    cap results (auto-paginates above one page)
                          --cheapest   rank by price (low → high)
                          --in-stock   keep only buyable items
                          --on-offer   keep only items with a discount/promo
  batch [-f file]         resolve many terms at once — preferred brand (config
                          [brands] or --brand a,b) else cheapest in-stock hit per
                          term. --no-brands ignores preferences. One request each.
                          --brand a,b / --no-brands  brand preferences
                          --on-offer   resolve each term among its deals only
  brands <term...>        list the brands selling that product type (count +
                          cheapest), to fill [brands] preferred in config.toml
  total [-f file]         deterministic basket total from '<url|term> [qty]'
                          lines — summed in integer cents. URLs price from the
                          product page, terms from their cheapest hit.
  categories [<slug>]     list top-level catalog sections; with a slug
                          (e.g. herramientas) list that section's products
                          --subs       list child subcategories instead
                          --limit N    cap products
                          --cheapest   rank that section's products by price
  product <url|path>      product detail (price, brand, rating, availability).
                          Pass the url field from a search result.

CART & CHECKOUT:
  cart get                show the cart — lines, quantities, totals
  cart add <url> [qty]    add a product (url from search) to the cart
                          --max <eur>  refuse a line over <eur> (spending guard;
                          also LEROYMERLIN_MAX_EUR or [limits] max_eur in config)
  cart set <ref> <qty>    set a product's absolute quantity (0 removes it)
  cart clear              remove every line from the cart
  checkout [status]       cart total + whether it can be checked out (blockers)
  checkout addresses      your saved delivery addresses
  checkout slots          available delivery/pickup options (date + cost)
                          (all read-only — payment is never automated)

COMMON FLAGS (may go anywhere after the command):
  --json                  emit raw JSON (data→stdout, logs→stderr)
  --toon                  emit TOON instead of JSON (fewer tokens; for agents)

BRAND PREFERENCES (config.toml [brands]; discover names with the 'brands' command):
  preferred = ["Bosch", …]        favourites by priority — used by batch when stocked
  mode = "cheapest_among"         among matches: cheapest (default) | "strict_priority"
  [brands.overrides]              per-term brand, e.g. "taladro" = ["Bosch"]

ENV:
  LEROYMERLIN_BASE_URL    override the host (debugging proxy, mock, staging)
  LEROYMERLIN_CONFIG_DIR  override ~/.leroymerlin

  version | help
`)
}

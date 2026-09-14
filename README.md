<p align="center">
  <img src="https://img.shields.io/badge/leroymerlin--cli-storefront%20CLI-blue?style=for-the-badge" alt="leroymerlin-cli">
</p>

<h1 align="center">leroymerlin-cli</h1>

<p align="center">
  <strong>Unofficial, agent-friendly CLI for leroymerlin.es — search the catalog, price a basket, and drive the cart from the command line</strong>
</p>

<p align="center">
  <a href="https://github.com/seifreed/leroymerlin-cli/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/seifreed/leroymerlin-cli/ci.yml?style=flat-square&logo=github&label=CI" alt="CI Status"></a>
  <img src="https://img.shields.io/badge/go-1.27%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
  <a href="https://github.com/seifreed/leroymerlin-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/single%20binary-no%20runtime%20deps-brightgreen?style=flat-square" alt="Single binary">
  <img src="https://img.shields.io/badge/output-JSON%20%7C%20TOON%20%7C%20human-blue?style=flat-square" alt="Output">
</p>

<p align="center">
  <a href="https://github.com/seifreed/leroymerlin-cli/stargazers"><img src="https://img.shields.io/github/stars/seifreed/leroymerlin-cli?style=flat-square" alt="GitHub Stars"></a>
  <a href="https://github.com/seifreed/leroymerlin-cli/issues"><img src="https://img.shields.io/github/issues/seifreed/leroymerlin-cli?style=flat-square" alt="GitHub Issues"></a>
  <a href="https://buymeacoffee.com/seifreed"><img src="https://img.shields.io/badge/Buy%20Me%20a%20Coffee-support-yellow?style=flat-square&logo=buy-me-a-coffee&logoColor=white" alt="Buy Me a Coffee"></a>
</p>

---

## Overview

**leroymerlin-cli** is a single static Go binary that reads the (unofficial) `leroymerlin.es` storefront: full-text product search, product detail, deterministic basket totals, and the guest cart. Every command speaks structured `--json` / `--toon` (data to stdout, logs to stderr, exit `1` on error), so it is usable from scripts and from agents.

It runs on **your own logged-in browser session** — see [Session](#session). Without one, DataDome challenges the client and even `search` returns `403`.

### Key Features

| Feature | Description |
|---------|-------------|
| **Chrome TLS fingerprint** | uTLS presents Chrome's JA3, keeping the client off DataDome's JS-challenge path |
| **Your session, your data** | Prices, stock, delivery options and the cart follow the store your signed-in browser has selected |
| **Agent-first output** | `--json` and `--toon` on every command; data on stdout, diagnostics on stderr |
| **Exact money** | Basket totals sum in integer cents; prices read from the offer the storefront bills, not from stale JSON-LD |
| **Spending guard** | `--max` refuses a cart line over the cap, fail-closed: an unreadable price refuses the write |
| **Brand preferences** | `batch` resolves each term to a favourite brand, with per-term overrides |
| **Terminal-safe** | Control characters are stripped from remote text — a marketplace seller writes the product name |
| **Read-only checkout** | Reports totals and blockers; payment is never automated |

### Where the data comes from

```text
search hits         the per-product cdl_products_list JSON in each result card
product detail      the page's schema.org Product JSON-LD (name, brand, GTIN, rating)
price               the offer object the add-to-cart form names — NOT the JSON-LD,
                    which can lag it (one drill advertised 13.99 there, cart charged 14.95)
add-to-cart fields  the hidden reflm / offerId / contextCode inputs of that same form
specs & stock       the o-main-characteristics rows and the available_deliveries blob
```

---

## Installation

### Prebuilt binary (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/seifreed/leroymerlin-cli/main/install.sh | sh
```

### With Go

```bash
go install github.com/seifreed/leroymerlin-cli/cmd/leroymerlin@latest
```

### From source

```bash
git clone https://github.com/seifreed/leroymerlin-cli.git
cd leroymerlin-cli
go build -o leroymerlin ./cmd/leroymerlin
./leroymerlin version
```

---

## Quick Start

```bash
# Sign in at www.leroymerlin.es in your everyday browser, load a page, then:
leroymerlin login                    # lifts that browser's session
leroymerlin whoami                   # confirms it is being accepted

# Full-text search
leroymerlin search taladro --limit 5

# The 10 cheapest buyable hits, as JSON for scripts and agents
leroymerlin search --cheapest --in-stock --limit 10 --json destornillador

# Product detail — pass the url from a search result
leroymerlin product /productos/taladro-percutor-practyl-500-w-con-tope-de-profundidad-83085630.html
```

---

## Usage

### Session

Every command runs on the session of a browser **you already use and are signed in at**. DataDome challenges a client it does not recognise, so without one even `search` gets a `403`; and prices, stock, delivery options and the cart are yours only if the session is.

The CLI never drives a browser of its own. It used to offer that, and it could not work: a freshly launched profile has no history with the site, so DataDome challenges it on sight and the clearance cookie never appears.

The clearance **rotates and expires**, and the cart endpoints are scored more strictly than the rest — a `cart get` that starts failing while `search` still works is the cue to run `login` again, not a sign that anything broke.

**Signed in, not just recognised.** A cookie lifted from a browser that was *not* signed in still reads, but its cart is a guest cart: items land in it, the storefront answers `2xx`, and your own cart page stays empty. So `cart` and `checkout` refuse a session without an account and tell you to sign in and run `login` again; `search`, `product`, `batch` and the rest keep working.

### Commands

| Command | Description |
|---------|-------------|
| `leroymerlin login` | lift the session from a browser you already use; `--from-browser chrome\|chromium\|firefox\|safari\|edge\|brave` reads just one store |
| `leroymerlin whoami` | check the session is still being accepted |
| `leroymerlin search <term...>` | full-text search — `--limit N`, `--cheapest`, `--in-stock`, `--on-offer` |
| `leroymerlin batch [-f file]` | resolve many terms at once: preferred brand, else cheapest in-stock hit per term |
| `leroymerlin brands <term...>` | brands selling a product type (count + cheapest), to fill `[brands]` in config |
| `leroymerlin total [-f file]` | deterministic basket total from `<url\|term> [qty]` lines, summed in integer cents |
| `leroymerlin categories [<slug>]` | top-level sections; with a slug, its products (`--limit`, `--cheapest`) or children (`--subs`) |
| `leroymerlin product <url\|path>` | product detail: price, brand, rating, specs, per-channel stock |
| `leroymerlin cart get` | show the cart — lines, quantities, totals |
| `leroymerlin cart add <url> [qty]` | add a product (`--max <eur>` spending guard) |
| `leroymerlin cart set <ref> <qty>` | set an absolute quantity (0 removes the line) |
| `leroymerlin cart clear` | empty the cart |
| `leroymerlin checkout [status]` | total + whether checkout is blocked; read-only, never pays |
| `leroymerlin checkout addresses` | saved delivery and invoice addresses |
| `leroymerlin checkout slots` | delivery/pickup options with date and cost (★ = selected) |
| `leroymerlin import-har --file f` | lift the cookie from a DevTools HAR ("Save all as HAR with sensitive data") |
| `leroymerlin set-cookie '<cookie>'` | paste a raw `Cookie` header; `--stdin` supported |
| `leroymerlin version` / `help` | — |

### Common flags

| Option | Description |
|--------|-------------|
| `--json` | emit raw JSON (data → stdout, logs → stderr) |
| `--toon` | emit [TOON](https://github.com/toon-format/toon-go) instead of JSON (fewer tokens; for agents) |
| `--max <eur>` | `cart add` / `cart set`: refuse a line over the cap |

Flags may appear anywhere after the command, and `--` ends flag parsing.

---

## Configuration

`~/.leroymerlin/config.toml`, all optional:

```toml
[limits]
max_eur = 25                    # refuse a cart line over this; 0 = no cap

[brands]
preferred = ["Bosch", "Dexter"] # favourites, by priority — used by batch
mode = "cheapest_among"         # among matches: cheapest (default) | "strict_priority"

[brands.overrides]
taladro = ["Practyl"]           # per-term preference, wins over `preferred`

[auth]
cookie = "…"                    # a session to fall back on; `login` normally writes it
```

`~/.leroymerlin/session.json` is machine-managed (mode `0600`) — `login`, `import-har` and `set-cookie` write it. Discover brand names for `preferred` with `leroymerlin brands <term>`.

### Environment

| Var | Description |
|-----|-------------|
| `LEROYMERLIN_BASE_URL` | override the host (debugging proxy, mock, staging) |
| `LEROYMERLIN_CONFIG_DIR` | override `~/.leroymerlin` |
| `LEROYMERLIN_MAX_EUR` | spending cap for `cart add` / `cart set`; `--max` wins over it, and it wins over `[limits] max_eur` |

---

## Claude skill

`.claude/skills/leroymerlin-shop/` bundles a `leroymerlin-shop` skill that drives this CLI to do a home-improvement / DIY shop from a **photo** (a tool, a part to replace, a room to reform, a materials list) or a pasted list: it reads the image, resolves each item to a real product, confirms the plan, and fills the cart under a hard `--max` cap — and never places the order.

---

## Notes and limits

- **`--limit` counts what survives, not what was fetched.** With `--cheapest`, `--in-stock` or `--on-offer` in play, a whole page is read first and the cut to N happens after — otherwise `--cheapest --limit 3` would answer "the cheapest of the first 3", which on a page led by sponsored listings is the three most expensive. A limit past one page auto-paginates (the site pages via `p`), deduping across pages, capped at 25. `--limit 0` returns a single page (~48).
- **`batch --on-offer` narrows the candidates, then picks** — so a term resolves to its discounted product instead of being dropped because the cheapest hit carried no offer. `search --on-offer` filters the list the same way.
- **`product` needs the URL**, not a bare reference: the site requires the full slug. Take it from `search` output (`url` field).
- **Per-store stock.** `product` reports pickup / home delivery / relay-point stock and cost for the store your session is set to. To check another, select it on leroymerlin.es and run `login` again. There is no standalone store command — the store context lives in the session, not in a clean public endpoint.
- **Checkout never pays.** It reports the total and what blocks checkout (a guest cart blocks on account and address). Placing an order is out of scope by design.
- **Spanish only.** The storefront does not negotiate language: the same page requested with a Catalan `Accept-Language` comes back byte for byte identical, still `lang="es-ES"`, with no `hreflang` alternate.

---

## Requirements

- Go 1.27+ to build from source (the released binary is static, with no runtime dependencies)
- A browser signed in at `leroymerlin.es`: Chrome, Chromium, Firefox, Safari, Edge or Brave

---

## Contributing

Contributions are welcome.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Run the gate (`make check`) — formatting, vet, tests and build must pass
5. Open a Pull Request

---

## Support the Project

If this project is useful in your workflows, you can support development:

<a href="https://buymeacoffee.com/seifreed" target="_blank">
  <img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" height="50">
</a>

---

## License

This project is licensed under the MIT license. See [LICENSE](LICENSE).

> Unofficial. Leroy Merlin has no public API. This fetches the same pages the website serves and reads the structured data they already embed; use at a sane request rate.

**Attribution**
- Author: **Marc Rivero López** | [@seifreed](https://github.com/seifreed)
- Repository: [github.com/seifreed/leroymerlin-cli](https://github.com/seifreed/leroymerlin-cli)

---

<p align="center">
  <sub>Built for practical home-improvement shopping and agent automation</sub>
</p>

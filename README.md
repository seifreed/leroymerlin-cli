<h1 align="center">leroymerlin-cli</h1>

<p align="center">
  <strong>Unofficial, agent-friendly CLI for leroymerlin.es — search the catalog and read product detail from the command line</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/go-1.26%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/single%20binary-no%20runtime%20deps-brightgreen?style=flat-square" alt="Single binary">
  <img src="https://img.shields.io/badge/output-JSON%20%7C%20TOON%20%7C%20human-blue?style=flat-square" alt="Output">
</p>

---

## Overview

**leroymerlin-cli** is a single static Go binary that reads the (unofficial)
`leroymerlin.es` storefront: full-text product search and product detail
(price, brand, rating, availability, GTIN). Every command speaks structured
`--json` / `--toon` (data to stdout, logs to stderr, exit `1` on error) so it's
friendly to scripts and agents.

It's a port of [`bonpreu-cli`](https://github.com/seifreed/bonpreu-cli) to Leroy
Merlin — **same stack** (Go 1.26, uTLS Chrome fingerprint, dependency-free
client, TOON/JSON output), same agent-first shape, adapted to a different site.

> Unofficial. Leroy Merlin has no public API. This fetches the same pages the
> website serves and reads the structured data they already embed; use at a sane
> request rate. Reads need no login.

### How it works

The site has no JSON API and fronts every page with **DataDome** bot protection
(a plain `curl` gets `403`). leroymerlin-cli presents **Chrome's TLS (JA3)
fingerprint via uTLS**, so DataDome keeps it off the JS-challenge path and
anonymous reads work from any IP. It then lifts the structured data the SSR HTML
already carries:

| Command | Data source |
|---------|-------------|
| `search` | the per-product `cdl_products_list` JSON embedded in each result card |
| `product` | the page's schema.org `Product` JSON-LD |

If a read ever draws a DataDome challenge, seed a browser cookie once with
`set-cookie` (see below).

---

## Install

```bash
git clone https://github.com/seifreed/leroymerlin-cli.git
cd leroymerlin-cli
go build -o leroymerlin ./cmd/leroymerlin
./leroymerlin version
```

Put `leroymerlin` on your `PATH` to call it from anywhere.

---

## Usage

```bash
# Full-text search (anonymous, no login)
leroymerlin search taladro --limit 5

# Cheapest in-stock first, as JSON for scripts/agents
leroymerlin search --cheapest --in-stock --limit 10 --json destornillador

# Product detail — pass the `url` from a search result
leroymerlin product /productos/taladro-percutor-practyl-500-w-con-tope-de-profundidad-83085630.html
```

### Commands

| Command | Description |
|---------|-------------|
| `search <term...>` | full-text product search. `--limit N`, `--cheapest`, `--in-stock` |
| `batch [-f file]` | resolve many terms at once — preferred brand (config/`--brand`) else cheapest in-stock hit per term |
| `brands <term...>` | list the brands selling a product type (count + cheapest), to fill `[brands]` in config |
| `total [-f file]` | deterministic basket total from `<url\|term> [qty]` lines, summed in integer cents |
| `categories [<slug>]` | list top-level sections; with a slug (e.g. `iluminacion`) list that section's products (`--limit`, `--cheapest`) |
| `product <url\|path>` | product detail; pass the `url` field a search result returns |
| `login --from-browser b` | lift the cookie from a browser store (chrome/firefox/safari/edge/brave) — easiest WAF fallback |
| `import-har --file f` | lift the cookie from a DevTools HAR ("Save all as HAR with sensitive data") |
| `set-cookie '<cookie>'` | seed a raw Cookie header (DataDome clearance); `--stdin` supported |
| `whoami` | check whether reads work, and whether a cookie is cached |
| `version` / `help` | — |

### Common flags

| Flag | Description |
|------|-------------|
| `--lang es` | language: `es` (default) or `ca` |
| `--json` | emit raw JSON (data→stdout, logs→stderr) |
| `--toon` | emit [TOON](https://github.com/toon-format/toon-go) instead of JSON (fewer tokens; for LLM/agents) |

### Env

| Var | Description |
|-----|-------------|
| `LEROYMERLIN_BASE_URL` | override the host (debugging proxy, mock, staging) |
| `LEROYMERLIN_CONFIG_DIR` | override `~/.leroymerlin` |

---

## Parity with bonpreu-cli

Ported (same stack, adapted to Leroy Merlin's site): `search`, `batch`, `total`,
`categories`, `product`, `login --from-browser`, `import-har`, `set-cookie`,
`whoami`, and the `--json` / `--toon` / `--lang` output modes.

`brands` is **adapted**, not a 1:1 port: bonpreu lists a ~3k brand catalogue from
a dedicated endpoint; Leroy Merlin has none, so `brands <term>` instead tallies
the brands selling that product type — the part that actually feeds the
`[brands]` preferences `batch` honours.

Deliberately **not** ported:

- **`cart` / `checkout`** — these need an authenticated Leroy Merlin account and
  reverse-engineering the cart/checkout *write* API, plus performing real cart
  writes to test. Out of scope for an anonymous read client.
- **`--fresh`** — drops frozen/canned groceries; meaningless for a hardware store.

## Notes & limits

- **Pagination**: the site loads more results via a "load more" button, not a
  query param, so one `search` (or `categories <slug>`) returns a single page
  (~24–56 hits) — plenty for ranking. Multi-page paging is not implemented.
- **`product` needs the URL**, not a bare reference — the site requires the full
  slug. Take it from `search` output (`url` field / the printed link).
- **Subcategory tree**: only the top-level sections scrape cleanly; deeper
  subcategories don't, so drill down with `search` or per-section listing.

## License

MIT — see [LICENSE](LICENSE).

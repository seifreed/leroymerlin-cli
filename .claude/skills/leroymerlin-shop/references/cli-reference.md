# `leroymerlin` CLI reference

Every command, its flags, and the `--json` shapes. Data → stdout, logs/errors → stderr. Exit codes:
`0` ok, `1` runtime error, `2` usage / unknown command. Common flags (anywhere after the subcommand):
`--lang es|ca`, `--json` (raw JSON), `--toon` (TOON — same shapes as `--json`, fewer tokens; prefer it
when an agent parses the output).

## Read commands (anonymous, no login)

| Command | What it does |
|---|---|
| `leroymerlin search <term…>` | Full-text search. `--limit N`, `--cheapest` (rank by price low→high), `--in-stock` (keep only buyable). |
| `leroymerlin batch -f <file\|->` | One hit per term (one term per line; positional terms also ok): the **cheapest in-stock** hit, or your preferred brand if `[brands]` is configured. `--brand a,b` prefers brands for this run; `--no-brands` ignores config. |
| `leroymerlin total -f <file\|->` | Deterministic total from `<url\|term> [qty]` lines, summed in integer cents. A `/productos/…` URL prices from its page; a plain term from its cheapest hit. Positional args also ok (qty 1; qty per line needs `-f`). |
| `leroymerlin product <url\|path>` | Detail: name, brand, price, availability, rating, GTIN, description. **Takes the product URL** (the `url` field from a search hit), not a bare ref. |
| `leroymerlin categories [<slug>]` | No arg: list top-level sections (`name` + `/productos/<slug>/` path). With a slug (e.g. `iluminacion`): list that section's products (`--limit N`, `--cheapest`). |
| `leroymerlin brands <term…>` | Tally the brands selling that product type: brand, hit count, cheapest price. The menu for building `[brands]` favourites. `--limit N`. |

### Search / batch product hit shape (`--json`)

```json
{
  "brand": "DEXTER",
  "identifier": "19557783",                 // the REF — what cart set / brands / cart get show
  "name": "Juego de 48 piezas con puntas, llaves vaso y carraca en T DEXTER",
  "url": "/productos/juego-…-19557783.html", // pass THIS to `product` and `cart add`
  "rating": 4.7,
  "product_is_sponsored": false,             // batch skips sponsored placements
  "total_offer_count": 1,
  "offer": {
    "unitprice_ati": 11.79,                  // price incl. tax — the displayed price
    "unitprice_tf": 9.74,                    // tax-free
    "initial_price": null,                   // was-price when discounted
    "seller_name": "Leroy Merlin",
    "seller_type": "1P",                     // 1P = Leroy Merlin · 3P = marketplace (flag it)
    "add_to_cart_availability": true,        // false = not buyable
    "commercial_animations": { "label": "Envío gratis en pedidos +29€" }   // promo, when present
  }
}
```

`batch --json` wraps each hit: `{ "term": "...", "product": { … }, "brandMatch": "preferred" }`.
`brandMatch` ∈ `preferred` (global) | `override` (per-term / `--brand`) | `none` | `off`. **`none`** = an
explicit override/`--brand` brand wasn't stocked → **ask the user before substituting** (see SKILL
"Brand preferences"); the others need no prompt.

`total --json` → `{ "lines": [{ "ref", "name", "qty", "price", "subtotal" }], "total": "41.97", "count": 2, "complete": true }`.

`product --json` → `{ "name", "sku", "gtin", "brand", "offers": [{ "price": "13.99", "priceCurrency": "EUR", "availability": "http://schema.org/InStock" }], "aggregateRating": { "ratingValue", "reviewCount" }, "image" }`.

`brands --json` → `[{ "brand": "DEXTER", "count": 7, "minPrice": 20.99 }, …]` (sorted by count desc).

`categories --json` (no slug) → `[{ "name": "Iluminación", "path": "/productos/iluminacion/" }, …]`;
with a slug → the same product-hit array as `search`.

## Cart & checkout (need a browser cookie)

The cart endpoints are DataDome-protected (HTTP 403 without a cookie). Get one with `login
--from-browser` (easiest), `import-har`, or `set-cookie`. The cookie **rotates** — re-run `login
--from-browser` when a cart command returns the `DataDome challenged …` 403 hint.

| Command | What it does |
|---|---|
| `leroymerlin login --from-browser <name>` | **Easiest.** Lift the cookie from a browser store: `chrome`\|`chromium`\|`firefox`\|`safari`\|`edge`\|`brave` (empty = any). Self-verifies. macOS Chrome may prompt the Keychain once. |
| `leroymerlin import-har --file <har\|->` | Extract the cookie from a DevTools HAR. Use **"Save all as HAR with sensitive data"** — the plain ⤓ export is sanitized and rejected. |
| `leroymerlin set-cookie '<cookie>'` | Seed a raw Cookie header manually (`--stdin`). |
| `leroymerlin whoami` | Report whether reads work and whether a cookie is cached. |
| `leroymerlin cart get` | Show the cart: lines (`[ref] name — qty × = line€`) + totals. `--json` for the structured cart. |
| `leroymerlin cart add <url> [qty]` | Add a product by its **URL** (from search). Additive. `--max <eur>` per-line spending cap. |
| `leroymerlin cart set <ref> <qty>` | Set a product's absolute qty by **ref** (`0` removes). Idempotent. |
| `leroymerlin cart clear` | Empty the cart. |
| `leroymerlin checkout [status]` | Read-only readiness: total (items + shipping), `ready`, and the blockers. Never pays. |

### Cart shape (`cart get --json`)

```json
{
  "orderId": "985cada6-…",
  "quantity": 2,                              // total articles
  "lines": [
    { "lineId": "1681bcfd-…",                 // internal UUID (cart set/clear use it under the hood)
      "reflm": "83085630",                    // the REF you pass to `cart set`
      "name": "Taladro percutor PRACTYL 500 W …",
      "quantity": 2,
      "price": 27.98 }                         // LINE total (qty × unit)
  ],
  "totalAmount": 31.88,                        // items + delivery
  "offersAmount": 27.98,                       // items only
  "deliveryAmount": 3.90,
  "disabledCheckout": false,
  "blockers": ["ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER", "SIMULATION_NEEDS_CITY", …]
}
```

### Checkout status shape (`checkout --json`)

```json
{ "items": 2, "total": 31.88, "offers": 27.98, "delivery": 3.90,
  "ready": false,
  "blockers": ["ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER", "SIMULATION_NEEDS_CITY", "SIMULATION_NEEDS_USER_ADDRESS", …] }
```

`ready` is `true` only when nothing blocks. A **guest cart is never `ready`** — it blocks on
account/address fields (`ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER`, `SIMULATION_NEEDS_*`). That's expected:
the user finishes account + address + delivery + payment in the browser.

## Spending guard

`--max <eur>` on `cart add` refuses a line over the cap (also `LEROYMERLIN_MAX_EUR` env, or `[limits]
max_eur` in `~/.leroymerlin/config.toml`; precedence flag > env > config; `0`/unset = no limit).

## Notes

- **No order placement.** There is no `submit`. The CLI prepares the cart and reports readiness; the
  user chooses delivery and pays in the browser.
- **`product` / `cart add` take the URL**, not a bare ref — the site requires the full product slug.
  Take it from a search hit's `url`. `cart set` / `brands` / `cart get` use the short **ref**.
- **DataDome.** Anonymous reads clear the bot wall via the CLI's Chrome TLS fingerprint (uTLS). The
  **cart** endpoints are stricter — they need a real browser cookie, which rotates; re-lift it with
  `login --from-browser` when a 403 hint appears.
- **Config / state.** `~/.leroymerlin/config.toml` (`[defaults] lang`, `[limits] max_eur`, `[auth]
  cookie`, `[brands]` favourites) and `session.json` (machine-managed cookie). Override the dir with
  `LEROYMERLIN_CONFIG_DIR`; override the host with `LEROYMERLIN_BASE_URL`.
- **Brand prefs (`[brands]`).** `preferred = [...]` global favourites (priority order); `mode =
  "cheapest_among"` (default) | `"strict_priority"`; `[brands.overrides]` maps a term to its own ordered
  brand list (`"taladro" = ["Bosch"]`), winning over the global list. Empty = pick cheapest as before.

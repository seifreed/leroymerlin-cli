# `leroymerlin` CLI reference

Every command, its flags, and the `--json` shapes. Data → stdout, logs/errors → stderr. Exit codes:
`0` ok, `1` runtime error, `2` usage / unknown command. Common flags (anywhere after the subcommand):
`--lang es|ca`, `--json` (raw JSON), `--toon` (TOON — same shapes as `--json`, fewer tokens; prefer it
when an agent parses the output).

## Read commands (anonymous, no login)

| Command | What it does |
|---|---|
| `leroymerlin search <term…>` | Full-text search. `--limit N` (above one page, **auto-paginates** via the site's `p` param, deduped, ≤25 pages; `0` = one page ~48). `--cheapest` (rank by price), `--in-stock` (buyable only), `--on-offer` (discount/promo only). |
| `leroymerlin batch -f <file\|->` | One hit per term (one term per line; positional terms also ok): the **cheapest in-stock** hit, or your preferred brand if `[brands]` is configured. `--brand a,b` prefers brands for this run; `--no-brands` ignores config; `--on-offer` only resolves terms whose hit has a deal. |
| `leroymerlin total -f <file\|->` | Deterministic total from `<url\|term> [qty]` lines, summed in integer cents. A `/productos/…` URL prices from its page; a plain term from its cheapest hit. Positional args ok (qty 1; per-line qty needs `-f`). |
| `leroymerlin product <url\|path>` | Detail: name, brand, price, availability, rating, GTIN, description, **`características`** (technical specs) and **`disponibilidad`** (per-channel stock for the session's store). **Takes the product URL** (a search hit's `url`), not a bare ref. |
| `leroymerlin categories [<slug>]` | No arg: top-level sections (`name` + `/productos/<slug>/`). With a slug: that section's **products** (`--limit N`, `--cheapest`), or its **child subcategories** with `--subs`. (Section pages are curated landing sets, not paginated.) |
| `leroymerlin brands <term…>` | Tally the brands selling that product type: brand, hit count, cheapest price. The menu for building `[brands]` favourites. `--limit N`. |

### Search / batch product hit shape (`--json`)

```json
{
  "brand": "DEXTER",
  "identifier": "19557783",                 // the REF — shown by cart get / brands; pass to cart set
  "name": "Juego de 48 piezas … DEXTER",
  "url": "/productos/juego-…-19557783.html", // pass THIS to `product` and `cart add`
  "rating": 4.7,
  "product_is_sponsored": false,             // batch skips sponsored placements
  "offer": {
    "unitprice_ati": 11.79,                  // price incl. tax — the displayed price
    "initial_price": null,                   // was-price when discounted (drives offer="discount")
    "seller_name": "Leroy Merlin",
    "seller_type": "1P",                     // 1P = Leroy Merlin · 3P = marketplace (flag it)
    "add_to_cart_availability": true,        // false = not buyable
    "commercial_animations": { "label": "Envío gratis en pedidos +29€" }
  }
}
```

`batch --json` wraps each hit: `{ "term": "...", "product": { … }, "brandMatch": "...", "offer": "...", "offerLabel": "..." }`.
- `brandMatch` ∈ `preferred` (global) | `override` (per-term / `--brand`) | `none` | `off`. **`none`** = an
  explicit override/`--brand` brand wasn't stocked → **ask the user before substituting**; others need no prompt.
- `offer` ∈ `discount` (with `offerLabel` = "antes X€") | `promo` (label, e.g. free shipping) | "" (none).

`total --json` → `{ "lines": [{ "ref", "name", "qty", "price", "subtotal" }], "total": "41.97", "count": 2, "complete": true }`.

`brands --json` → `[{ "brand": "DEXTER", "count": 7, "minPrice": 20.99 }, …]` (sorted by count desc).

`categories --json` (no slug, or `--subs`) → `[{ "name": "Herramientas manuales", "path": "/productos/herramientas/herramientas-de-mano/" }, …]`; with a slug → the same product-hit array as `search`.

### Product detail shape (`product --json`)

```json
{
  "name": "Taladro percutor PRACTYL 500 W …",
  "sku": "83085630", "gtin": "3276007383270", "brand": "PRACTYL",
  "offers": [{ "price": "13.99", "priceCurrency": "EUR", "availability": "http://schema.org/InStock" }],
  "aggregateRating": { "ratingValue": "4.56", "reviewCount": "234" },
  "specs": [{ "label": "Función percutor", "value": "Sí" }, { "label": "Diámetro … (mm)", "value": "13" }],
  "deliveries": [
    { "type": "storeDelivery", "status": "ONSITE", "stock": 21, "price": 0,   "time": "2 HOUR" },
    { "type": "homeDelivery",  "status": "ONSITE", "stock": 25, "price": 3.9, "time": "1 OPENING_DAY" },
    { "type": "relayDelivery", "status": "ONSITE", "stock": 5083, "price": 2.9, "time": "2 OPENING_DAY" }
  ]
}
```

`deliveries[].type`: `storeDelivery` = pickup at the session's store (its `stock` is the **local store
stock**), `homeDelivery` = shipped, `relayDelivery` = pickup point. The store follows the session cookie.

## Cart & checkout (need a browser cookie)

The cart/checkout endpoints are DataDome-protected (HTTP 403 without a cookie). Get one with `login
--from-browser` (easiest), `import-har`, or `set-cookie`. The cookie **rotates** — re-run `login
--from-browser` when a command returns the `DataDome challenged …` 403 hint.

| Command | What it does |
|---|---|
| `leroymerlin login --from-browser <name>` | **Easiest.** Lift the cookie from a browser store: `chrome`\|`chromium`\|`firefox`\|`safari`\|`edge`\|`brave` (empty = any). Self-verifies. macOS Chrome prompts the Keychain once. Captures whichever **store** the browser is set to. |
| `leroymerlin import-har --file <har\|->` | Extract the cookie from a DevTools HAR. Use **"Save all as HAR with sensitive data"** — the plain ⤓ export is sanitized and rejected. |
| `leroymerlin set-cookie '<cookie>'` | Seed a raw Cookie header manually (`--stdin`). |
| `leroymerlin whoami` | Report whether reads work and whether a cookie is cached. |
| `leroymerlin cart get` | Show the cart: lines (`[ref] name — qty × = line€`) + totals. `--json` for the structured cart. |
| `leroymerlin cart add <url> [qty]` | Add a product by its **URL** (from search). Additive. `--max <eur>` per-line spending cap. |
| `leroymerlin cart set <ref> <qty>` | Set a product's absolute qty by **ref** (`0` removes). Idempotent — safe to re-run a whole plan. |
| `leroymerlin cart clear` | Empty the cart. |
| `leroymerlin checkout [status]` | Read-only readiness: total (items + shipping), `ready`, and the blockers. |
| `leroymerlin checkout slots` | Delivery/pickup options for the cart: `mode`, `date`, `amount` (★ = selected). |
| `leroymerlin checkout addresses` | The order's addresses by role (delivery / invoice / installation / relay). |

### Cart shape (`cart get --json`)

```json
{
  "orderId": "985cada6-…",
  "quantity": 2,                              // total articles
  "lines": [
    { "lineId": "1681bcfd-…",                 // internal UUID (cart set/clear use it under the hood)
      "reflm": "83085630",                    // the REF you pass to `cart set`
      "name": "Taladro percutor PRACTYL …",
      "quantity": 2,
      "price": 27.98 }                         // LINE total (qty × unit)
  ],
  "totalAmount": 31.88,                        // items + delivery
  "offersAmount": 27.98,                       // items only
  "deliveryAmount": 3.90,
  "disabledCheckout": false,
  "blockers": ["SIMULATION_NEEDS_APPOINTMENT_DATE", …]   // empty when ready
}
```

### Checkout status / slots / addresses shapes (`--json`)

```json
// checkout
{ "items": 2, "total": 31.88, "offers": 27.98, "delivery": 3.90, "ready": false,
  "blockers": ["SIMULATION_NEEDS_APPOINTMENT_DATE"] }

// checkout slots → []
{ "mode": "PICKUP_IN_STORE", "label": "PICKUP_IN_STORE_EXP_ONE_HOUR", "amount": 0,
  "date": "2026-07-01T18:30:00+02:00", "selected": true }

// checkout addresses → { role: address } (delivery/invoice/installation/relay)
{ "deliveryAddress": { "firstName":"…", "lastName":"…", "line1":"…", "postalCode":"08001", "city":"…", "province":"…" } }
```

`ready` is `true` only when nothing blocks. A **logged-in cart** typically blocks only on the delivery
slot (`SIMULATION_NEEDS_APPOINTMENT_DATE`); a **guest cart** also blocks on account/address
(`ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER`, `SIMULATION_NEEDS_*`). Either way the user finishes the slot
choice + payment in the browser.

## Spending guard

`--max <eur>` on `cart add` refuses a line over the cap **before writing** (also `LEROYMERLIN_MAX_EUR`
env, or `[limits] max_eur` in `~/.leroymerlin/config.toml`; precedence flag > env > config; `0`/unset = no
limit). A blocked line exits non-zero with `error: line … exceeds --max …` — stop-and-report.

## Notes

- **No order placement.** There is no `submit`. The CLI prepares the cart and reports readiness; the
  user chooses a slot and pays in the browser.
- **URL vs ref.** `product` / `cart add` take the **URL** (the full product slug); `cart set` / `brands`
  / `cart get` use the short **ref**. Take both from a search hit (`url`, `identifier`).
- **DataDome.** Anonymous reads clear the bot wall via the CLI's Chrome TLS fingerprint; the cart/checkout
  endpoints are stricter and need a real browser cookie, which **rotates** — re-lift it with `login
  --from-browser` when a 403 hint appears.
- **Per-store stock.** `product`'s `disponibilidad` reflects the store your imported session is set to.
  To check another store, select it on leroymerlin.es and re-run `login --from-browser`. There is no
  standalone `store` command.
- **Config / state.** `~/.leroymerlin/config.toml` (`[defaults] lang`, `[limits] max_eur`, `[auth]
  cookie`, `[brands]` favourites) and `session.json` (machine-managed cookie). Override the dir with
  `LEROYMERLIN_CONFIG_DIR`; the host with `LEROYMERLIN_BASE_URL`.
- **Brand prefs (`[brands]`).** `preferred = [...]` global favourites (priority order); `mode =
  "cheapest_among"` (default) | `"strict_priority"`; `[brands.overrides]` maps a term to its own ordered
  brand list (`"taladro" = ["Bosch"]`), winning over the global list. Empty = cheapest as before.

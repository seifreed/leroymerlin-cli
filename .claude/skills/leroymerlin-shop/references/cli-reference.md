# `leroymerlin` CLI reference

Every command, its flags, and the `--json` shapes. Data → stdout, logs/errors → stderr. Exit codes:
`0` ok, `1` runtime error, `2` usage / unknown command. Common flags (anywhere after the subcommand):
`--json` (raw JSON), `--toon` (TOON — same shapes as `--json`, fewer tokens; prefer it
when an agent parses the output).

## Read commands

All of them need the session (see `login --from-browser`): DataDome returns 403 without one. Cart and
checkout need it **signed in** on top of that — see the note under Session commands.

| Command | What it does |
|---|---|
| `leroymerlin search <term…>` | Full-text search. `--limit N` counts what survives the filters, not what was fetched: with `--cheapest`/`--in-stock`/`--on-offer` a whole page is read first and the cut happens after, so `--cheapest --limit 3` really is the three cheapest. Above one page it **auto-paginates** via the site's `p` param, deduped, ≤25 pages; `0` = one page ~48. `--cheapest` (rank by price), `--in-stock` (buyable only), `--on-offer` (discount/promo only). |
| `leroymerlin batch -f <file\|->` | One hit per term (one term per line; positional terms also ok): the **cheapest in-stock** hit, or your preferred brand if `[brands]` is configured. `--brand a,b` prefers brands for this run; `--no-brands` ignores config; `--on-offer` resolves each term among its discounted products only, skipping terms with none. |
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
| `leroymerlin import-har --file <har\|->` | Extract the cookie from a DevTools HAR. Use **"Save all as HAR with sensitive data"** — the plain ⤓ export is sanitized and rejected. Self-verifies like `login`: a stale cookie fails here, not two commands later. |
| `leroymerlin set-cookie '<cookie>'` | Seed a raw Cookie header manually (`--stdin`). Self-verifies like `login`, and names a cookie without an account. |
| `leroymerlin whoami` | Report whether reads work, whether a cookie is cached, and whether it is **signed in** (`--json`: `{cookie, signed_in, reads_ok}`). A signed-out one reads fine and is refused by cart/checkout. |

Every `cart …` and `checkout …` command additionally requires the cookie to carry a **signed-in
account**. A cookie lifted from a signed-out browser reads and writes fine but addresses a guest cart
the user never sees, so those commands refuse it (`… carries no signed-in account`) and `login` says so
when it caches one. Reads are unaffected.

| `leroymerlin cart get` | Show the cart: lines (`[ref] name — qty × = line€`) + totals. `--json` for the structured cart. |
| `leroymerlin cart add <url> [qty]` | Add a product by its **URL** (from search). Additive. `--max <eur>` per-line spending cap. Errors if the cart does not grow — a 2xx the storefront discards is not an add. |
| `leroymerlin cart set <ref> <qty>` | Set a product's absolute qty by **ref** (`0` removes). Idempotent — safe to re-run a whole plan. `--max <eur>` caps the resulting line, priced from the cart's own unit price; `0` is never over the cap. |
| `leroymerlin cart clear` | Empty the cart, then re-read it: a line that survives its delete is an error, not a cleared cart. |
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

`ready` is `true` only when nothing blocks. The cart is always the account's, so it typically blocks
only on the delivery slot (`SIMULATION_NEEDS_APPOINTMENT_DATE`); an account/address blocker
(`ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER`, `SIMULATION_NEEDS_*`) means a stale cookie — re-lift it. Either way the user finishes the slot
choice + payment in the browser.

## Spending guard

`--max <eur>` on `cart add` **and `cart set`** refuses a line over the cap **before writing** (also `LEROYMERLIN_MAX_EUR`
env, or `[limits] max_eur` in `~/.leroymerlin/config.toml`; precedence flag > env > config; `0`/unset = no
limit). A blocked line exits non-zero with `error: line … exceeds --max …` — stop-and-report.

## Notes

- **No order placement.** There is no `submit`. The CLI prepares the cart and reports readiness; the
  user chooses a slot and pays in the browser.
- **URL vs ref.** `product` / `cart add` take the **URL** (the full product slug); `cart set` / `brands`
  / `cart get` use the short **ref**. Take both from a search hit (`url`, `identifier`).
- **A search never returns nothing.** With no match the storefront widens the query and answers with
  unrelated products. The CLI reads the page's verdict (`searchType`): `search`/`brands` warn on
  stderr, `batch --json` sets `"relaxed": true` on the hit, and `total` refuses the line
  (`no exact match for …`) instead of pricing it. Treat a relaxed hit as **not found**, not as a match.
- **Soft throttling looks like a 404.** Back-to-back requests (a search+add loop over a list) make
  product pages answer HTTP 404 with an HTML error page while `search` keeps working. `cart add` and
  `product` report `no product found at …`; pace the loop and retry the item once rather than treating
  it as missing.
- **Signed in for the cart.** `cart` and `checkout` refuse a cookie without the account token: it
  would address a guest cart invisible to the user. Reads do not care.
- **DataDome.** Every request needs the browser session — reads included. The Chrome TLS fingerprint
  keeps the CLI off the JS-challenge path but does not stand in for a cookie, and the cookie **rotates**:
  re-lift it with `login --from-browser` whenever a 403 hint appears.
- **Per-store stock.** `product`'s `disponibilidad` reflects the store your imported session is set to.
  To check another store, select it on leroymerlin.es and re-run `login --from-browser`. There is no
  standalone `store` command.
- **Config / state.** `~/.leroymerlin/config.toml` (`[limits] max_eur`, `[auth]
  cookie`, `[brands]` favourites) and `session.json` (machine-managed cookie). Override the dir with
  `LEROYMERLIN_CONFIG_DIR`; the host with `LEROYMERLIN_BASE_URL`.
- **Brand prefs (`[brands]`).** `preferred = [...]` global favourites (priority order); `mode =
  "cheapest_among"` (default) | `"strict_priority"`; `[brands.overrides]` maps a term to its own ordered
  brand list (`"taladro" = ["Bosch"]`), winning over the global list. Empty = cheapest as before.

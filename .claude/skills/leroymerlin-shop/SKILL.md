---
name: leroymerlin-shop
description: >-
  Do the home-improvement / DIY shop at Leroy Merlin (leroymerlin.es) by driving the local
  `leroymerlin` CLI: turn a materials list, a project, or a PHOTO (a tool, a broken part, a room to
  reform, a handwritten or printed list) into real priced Leroy Merlin products, confirm them, and fill
  the cart with a hard spending cap. Use whenever the user wants to shop, price a DIY/reform list, build
  a cart, or "do the Leroy shop" — including when they paste a list or drop an image and mention Leroy
  Merlin. Spanish phrasings: "hazme la compra de Leroy", "pásame esto a Leroy Merlin", "añade X al
  carrito", "¿cuánto cuesta esta lista en Leroy?", "compra estos materiales", "mira esta foto y pídelo",
  "necesito esto para la reforma". English: "do my Leroy Merlin shop", "price this list at Leroy", "fill
  my Leroy cart", "shop this photo at Leroy Merlin". Always confirm the resolved products before touching
  the cart, always cap spend with --max, and never claim an order was placed — this CLI prepares the cart
  but does NOT submit orders. Do NOT use for other retailers (Bricomart, Bauhaus, Amazon) — Leroy Merlin only.
---

# Leroy Merlin shop

Turn a materials list — or a photo of a project — into a real Leroy Merlin cart by driving the
`leroymerlin` CLI: read the image, resolve each item to a real product with its price, confirm with the
user, and fill the cart with a spending cap. The CLI talks to the same endpoints `leroymerlin.es` uses;
this skill is the playbook for using it safely on a real account.

## Three things that are non-negotiable (read first)

This touches a real account and real money, so:

1. **Confirm before touching the cart.** Reading, searching and pricing are free and side-effect-free
   — do them freely. But before the *first* `cart add`/`cart set`, show the user the resolved plan
   (each list/photo item → the exact product you picked: `[ref] name — price`, with its URL) and wait
   for an explicit OK. Product matching is fuzzy (a photo adds a second fuzzy step), so this is where
   the user catches a wrong model, size, voltage or quantity — cheap now, annoying after the cart is full.
2. **Always cap spend with `--max`.** Put a hard euro ceiling on every `cart add` and every `cart set` so a wrong match or a
   fat-fingered quantity can't run up the order. Use the user's stated budget, else the agreed plan
   total per line with a little margin. A line over the cap fails with `error: line … exceeds --max …`
   and a non-zero exit — treat that as **stop-and-report**, don't raise the cap unless the user does.
3. **This CLI does NOT place orders.** There is no `submit`. The skill prepares the cart and reports
   checkout readiness (`leroymerlin checkout`), but the user finishes the order themselves in the
   browser (delivery/store pickup + payment). Never tell the user you "placed the order" — you can't.
   Say the cart is ready and point them to the web to choose delivery and pay.

## Locate the binary

Run `leroymerlin` from `PATH`. If it isn't there, build it from the repo (Go 1.26+):

```bash
go build -o leroymerlin ./cmd/leroymerlin   # in the leroymerlin-cli checkout; or put it on PATH
leroymerlin version
```

The storefront is Spanish-only. **Every command needs the session** (next section) — start there.

## Authenticate (bring-your-own session)

**Every command needs the user's browser session — searches included.** Leroy Merlin fronts the site
with **DataDome** bot protection: it challenges a client it does not recognise, so without a session even
`search` returns HTTP 403 (the CLI says so and names the command that fixes it). The Chrome TLS
fingerprint keeps the CLI off the JS-challenge path; it does not replace the session.

There is no account login to automate — the cookie is the DataDome clearance plus the storefront
session. That session is also what makes the answers *theirs*: prices, stock, delivery options and the
cart all follow the store the signed-in user has selected.

**Signed in, not just recognised.** A cookie lifted from a browser that was *not* signed in still reads,
but the cart it addresses is a **guest cart**: items go in, the storefront answers `2xx`, and the user's
own cart page stays empty. So `cart` and `checkout` refuse a session without the account and say
`… carries no signed-in account`; the fix is the user signing in at `leroymerlin.es` and re-running
`leroymerlin login`. `search`, `product`, `batch` and the other reads keep working meanwhile. Never
present a guest cart as the user's.

**Requirement, not a fallback: the session comes from a browser the user already uses and is signed in
at.** The CLI never drives a browser of its own — a fresh profile has no history with the site and
DataDome challenges it on sight. So the user signs in at `leroymerlin.es` in their everyday browser
(residential IP), loads a page, and the CLI reads the cookie that browser left behind:

1. **`leroymerlin login`** — the way in. Reads the cookie straight from the browser's cookie store;
   `--from-browser chrome|chromium|firefox|safari|edge|brave` narrows it to one, otherwise every
   installed browser is tried. No DevTools, no files. On macOS Chrome it pops a one-time Keychain
   prompt. Self-verifies and prints `ok — cookie lifted from browser, reads working`.
2. **`leroymerlin import-har --file export.har`** — DevTools → Network → right-click a request →
   **"Save all as HAR with sensitive data"** (NOT the ⤓ button — that strips the Cookie headers and is
   rejected).
3. **`leroymerlin set-cookie '<cookie>'`** (or `--stdin`) — paste the raw `Cookie:` header manually.

Always confirm with `leroymerlin whoami`. The DataDome cookie **rotates and expires**, so a cached
session goes stale: if a cart command returns the `DataDome challenged …` 403 hint, just re-run
`leroymerlin login` to lift a fresh one — after making sure the browser still has the site open.

If the user says their **browser cart looks empty** while `cart get` shows lines, the session is not
theirs: a cookie lifted before they signed in (a guest cart), or a different account. Re-run
`leroymerlin login` and compare `cart get` again — never argue the cart is there.

**Never echo the cookie back to the user.** Feed it via `--from-browser`, the HAR or `--stdin`, never print it.

## The photo → cart workflow (the headline)

When the user drops an **image** (a tool, a broken fitting/part to replace, a room or wall to reform, a
receipt, a handwritten or printed materials list):

### 1. Read the image into a clean item list — yourself

You have vision. Extract the items from the photo into a plain list of **names + quantities**, in the
user's language. Be literal about what you can see; don't invent items. For a receipt/printed list,
transcribe it; for a photo of a part, name what it is as specifically as you can (e.g. "grifo monomando
de cocina", "bisagra de cazoleta 35 mm", "tirafondos 4x40", "silicona sanitaria blanca", "rodillo de
pintura antigota"). Capture distinguishing specs you can read — **size, diameter, voltage, colour,
finish** — they're what makes a DIY match right or wrong. If parts are ambiguous or unreadable, list
your best guess and flag it — you'll confirm everything before buying anyway.

**Show the user what you parsed** and let them correct it *before* you price anything. This is the
photo-specific checkpoint: vision can misread a measurement or miss a detail.

### 2. Resolve names → real products

Price the whole parsed list in one command with `batch` — one term per line. It picks the **cheapest
in-stock** hit per term (sponsored placements skipped), or your **preferred brand** when `[brands]` is
configured (see "Brand preferences"):

```bash
printf 'taladro percutor\nbrocas hormigón\nsilicona sanitaria blanca\ntirafondos 4x40\n' | leroymerlin batch -f -
```

`batch` is fast but a rough match — and DIY items are spec-sensitive (a 6 mm vs 8 mm broca, 18 V vs
20 V, blanco vs cromo are different products). So:

- For anything the user was specific about (model, size, voltage, finish), or where the top hit looks
  off, run `leroymerlin search "<term>" --limit 5` and pick deliberately. Add the spec to the term —
  `search "broca hormigón 8mm"`, `search "grifo cocina monomando cromo"`. `--limit` above one page
  auto-paginates, so `--limit 100` casts a wider net when the first page misses the right variant.
- `--cheapest` ranks by price; `--in-stock` keeps only buyable items; `--on-offer` keeps only items
  with a discount/promo (use it when the user wants deals or to catch a was-price drop). Combining
  them with `--limit N` is safe: the limit counts what survives the filter, so `--cheapest --limit 3`
  is the three cheapest of the page, not the cheapest of the first three.
- **"The cheapest" is the cheapest of what the query returned.** Search is full-text: `mesa comedor
  blanca` matches those words, it does not filter by colour or material, so `--cheapest` ranks the
  hits — not the catalogue. Tell the user it that way ("la más barata de las que salen con esta
  búsqueda"), and widen the term or raise `--limit` before claiming a minimum.
- **Prices agree across commands.** `search`, `product`, `total` and the cart all read the offer the
  storefront bills from, so do not try to reconcile a `product` price against a `search` hit — if they
  ever differ, report it rather than averaging or preferring one.
- Browse, don't search, when the user is vague: `leroymerlin categories` lists the top sections,
  `categories <slug> --subs` drills into subcategories, `categories <slug>` lists that section's
  products — good for "enséñame opciones de griferías de cocina".
- Use `--json` when you need to parse refs/prices/URLs reliably (or `--toon` for the same fields at
  fewer tokens). **Keep each hit's `url`** — `cart add` needs it, and `product <url>` gives full detail.

**Verify spec-sensitive or pricey picks with `product <url>` before committing them to the plan.** The
detail view adds two things `search`/`batch` don't:

- **`características`** (specs: potencia, diámetro, voltaje, material, medidas) — confirm the match is the
  right 8 mm / 18 V / cromo variant, not a lookalike. Cheap insurance on a taladro, grifo or anything
  dimensional.
- **`disponibilidad`** (per-channel stock for the user's store: recogida en tienda / envío a domicilio /
  punto de recogida, each with stock + cost + lead time). If the user wants **store pickup**, check the
  *recogida en tienda* line has stock at their store; if it's 0 but home delivery has stock, say so. The
  store follows their session cookie (see "Authenticate").

If the product description conflicts with its technical specifications, treat the requirement as
**unverified**. Do not add it on the strength of marketing copy; show the conflict and ask the user to
choose another verified product or explicitly accept the mismatch.

**Watch as you match:** packs vs units (a box of 100 tornillos is qty 1, not 100), the right
dimension/voltage/finish, marketplace sellers (a `seller_type` of `3P` ships from a third party — fine,
but flag it), and items sold by length/area (cable, moulding, tiles — buy the right quantity). DIY
catalogues are deep; eyeball every hit and re-`search` anything that looks wrong.

**Mark what doesn't exist or got substituted.** Some specific parts aren't stocked, or only a near
equivalent is. Don't force a bad match: keep an explicit **"not found — check in store / buy elsewhere"**
list, and flag every substitution (⚠️ 6 mm offered vs 8 mm asked, brand X vs Y) in the plan so the user
vetoes or sources it himself.

### 3. Present the plan and get the OK (Gate 1)

Show a compact table: each item → `[ref] name — price` (and the product URL), plus a line quantity and
the not-found / substitution notes. **Compute the total with code, not by hand.**

For a **small list**, feed the resolved `<url-or-term> [qty]` lines to `leroymerlin total`:

```bash
printf '/productos/...-83085630.html 1\nsilicona sanitaria blanca 2\n' | leroymerlin total -f -
```

`total` prices a `/productos/…` URL from its page and a plain term from its cheapest hit, and sums in
integer cents. For a **large list (≈20+ items), prefer summing the prices you already have** from the
`batch --json` pass (`price × qty`, in cents) — `total` re-fetches each line and is slower; use it only
for the few lines you re-priced via `search`.

Surface every assumption (model, size, voltage, finish, a guessed photo item, a substitution, a `3P`
marketplace seller) so the user can veto it. Edit and re-total until they're happy. Only then touch the cart.

### 4. Fill the cart (after the OK)

Add each confirmed line with a spending cap. `cart add <url> [qty]` takes the product **URL** (from the
search/`batch` `url` field) and is additive; the URL is required because the site has no short add-by-id.
A write the storefront accepts but discards is reported as an error, not as a successful add of
nothing: `cart add` requires the cart to actually grow, and `cart clear` re-reads the emptied cart. So
trust a non-zero exit — do not "confirm" it by retrying blind.

Run `cart get` immediately before the first add, especially after refreshing or re-importing a browser
cookie; an existing browser cart may reappear when the session is renewed.

```bash
leroymerlin cart add "/productos/...-83085630.html" 1 --max 60
leroymerlin cart get                      # verify lines + totals
```

After items are in, adjust by **ref** (the `[number]` shown by `cart get` / search): `cart set <ref>
<qty>` sets the **absolute** quantity (`0` removes the line), and is idempotent — re-running a plan
converges instead of double-adding. `cart clear` empties the cart (check `cart get` first so you don't
wipe items the user added himself). **Pass `--max` to `cart set` too**: raising a quantity spends
exactly like adding one (9999 × a 0.59€ tape is 5 899€), and the cap is what stops a fat-fingered
quantity — `0` is never over it. `--max` is a **per-line** cap, not a basket total.

> The cart backend can lag a write by a beat — if a `cart get` right after a write looks stale, re-run
> it; it converges in a second or two. If a cart command returns the DataDome 403 hint, the cookie
> rotated — re-run `login --from-browser` and continue.

> **`no product found at …` for a URL a search just returned is throttling, not a dead product.** The
> storefront starts answering 404 (an HTML error page) to product pages while `search` still works,
> when the requests come back-to-back: six search+add pairs in a fast loop reproduced it, the same
> adds one at a time all went through. Put a beat between items and retry that item once before you
> mark it not-found — silently dropping it from the plan is the real damage.

> If a cart mutation or checkout read returns HTTP 412, open `/checkout/cart` in the browser, refresh
> the page, and retry once. Do not repeat `cart add` while the state is unresolved; verify with `cart get`
> first to avoid duplicating a line.

### 5. Hand off for checkout (the CLI stops here)

```bash
leroymerlin checkout              # readiness: total (items + shipping) and what's blocking
leroymerlin checkout slots        # delivery/pickup options: mode, date, cost (★ = selected)
leroymerlin checkout addresses    # the order's delivery/invoice addresses
```

All three are **read-only**. `checkout` reports the cart total and the blockers:

- With a **logged-in session** (the normal case here — you imported the cookie via `login
  --from-browser`), the cart is linked to the account, so the only blocker is usually **picking a
  delivery slot** (`needs appointment date`). Surface the options with `checkout slots` — you can tell
  the user "recogida en tienda gratis hoy 18:30, o envío a domicilio 3,90 € el 2-jul" — but the final
  slot choice + payment happen in the browser.
- Blockers about the **account** (`needs an account (log in)`, `needs city`, …) mean the cart is not
  linked to anyone — with `checkout` refusing signed-out sessions, that is a stale cookie: re-run
  `leroymerlin login`.

The CLI can't book delivery or pay. Tell the user the cart is ready, summarise the delivery options and
the address it would ship to (`checkout addresses`), and point them to the web to choose a slot and pay.
**Never claim the order was placed** — there is no `submit`.

> Free shipping kicks in at **+29€** (the "Envío gratis en pedidos +29€" promo on many products); under
> that, a shipping cost shows in `checkout` / `checkout slots`. There is no hard order minimum.

## Plain lists, projects, and clarifying

Not every request is a photo. For a **pasted list** ("taladro, brocas, tacos, tornillos") resolve each
item — same steps 2-5. For a **project** ("monta una estantería", "pinta una habitación de 15 m²",
"cambia el grifo del baño"), first turn it into a materials list, and **ask the few things that
genuinely move the basket** before pricing:

- **Dimensions / quantity drivers** — area to paint (m² → litres of paint + rollers), wall material
  (drill bit + fixing type differ for ladrillo vs pladur vs hormigón), shelf length, number of points.
- **Compatibility** — battery platform/voltage for power tools, thread/diameter for plumbing, finish to
  match (cromo/níquel/blanco).
- **What they already own** — don't re-buy a drill if they have one; ask.

Prefer the **AskUserQuestion** tool — one axis per question, 2-4 options, sensible default first, in the
user's language. Don't silently invent a consequential value (wall type, area, voltage); mark safe
defaults in the plan so the user can veto them.

## Brand preferences

The user can pin favourite brands in `~/.leroymerlin/config.toml` `[brands]` — a global priority-ordered
`preferred` list, plus per-term `[brands.overrides]` (`"taladro" = ["Bosch"]`). `batch` (and `--brand
a,b`) **applies these automatically**; you read the result and handle the one case that needs a human:

- Each `batch --json` hit carries `brandMatch`: `preferred`/`override` (got a wanted brand) or `off`
  (no preference applied) → nothing to do. **`none`** means an *explicit* per-term override (or
  `--brand`) brand wasn't stocked.
- On `none`, **ask before substituting**: show the cheapest/top hit you'd otherwise use and offer
  alternatives via **AskUserQuestion** (e.g. "Bosch no está para 'amoladora' — ¿la más barata (DEXTER
  26€), otra marca, o lo salto?"). Only add to the cart after the user picks.
- A global favourite simply not making a product is **not** a `none` and needs no prompt — it's `off`,
  pick normally. No `[brands]` config → cheapest in-stock hit, as before.

To help the user build the list, `leroymerlin brands <term>` tallies the brands selling that product
type (count + cheapest price) — the menu for `[brands]` favourites. If the user names a brand mid-shop
("los tornillos siempre Spax"), offer to add it to `[brands]` so it sticks next time.

## Reference

- `references/cli-reference.md` — every command, its flags, and the `--json` field shapes (search hit
  with `offer`/`brandMatch`, product `specs`/`deliveries`, cart line, checkout status/slots/addresses)
  for precise parsing.

## Why no all-in-one "shop the photo" script

This skill drives the CLI step by step on purpose: the confirm-before-cart gate and the spending cap
need a human in the loop, and matching a photo to real DIY products needs judgment. Keep the control
flow here, in the conversation, where the user can steer it.

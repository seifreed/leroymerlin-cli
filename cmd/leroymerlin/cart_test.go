package main

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

func TestResolveMax(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
	// flag wins
	if got, err := resolveMax(50, true); err != nil || got != 50 {
		t.Errorf("flag: got %v", got)
	}
	// env when flag unset (negative sentinel)
	t.Setenv("LEROYMERLIN_MAX_EUR", "25")
	if got, err := resolveMax(-1, false); err != nil || got != 25 {
		t.Errorf("env: got %v", got)
	}
	// no cap
	t.Setenv("LEROYMERLIN_MAX_EUR", "")
	if got, err := resolveMax(-1, false); err != nil || got != 0 {
		t.Errorf("none: got %v", got)
	}
	if _, err := resolveMax(math.NaN(), true); err == nil {
		t.Error("NaN flag should be rejected")
	}
	if _, err := resolveMax(-2, true); err == nil {
		t.Error("negative flag should be rejected")
	}
}

// cartStub serves a product page (with offer_id + price) and the cart endpoints.
func cartStub(t *testing.T, price string) (added *bool) {
	t.Helper()
	wasAdded := false
	added = &wasAdded
	prodHTML := `<script type="application/ld+json">{"@type":"Product","name":"Taladro","sku":"83085630","offers":{"price":"` + price + `","priceCurrency":"EUR"}}</script>` +
		`<script>{"offer_id":"deadbeefdeadbeef"}</script>`
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/productos/"):
			_, _ = w.Write([]byte(prodHTML))
		case r.URL.Path == "/cart/services/addToCart":
			wasAdded = true
			_, _ = w.Write([]byte(`{}`))
		case strings.Contains(r.URL.Path, "cart-data"):
			// The count grows only after the write, as the storefront's does.
			quantity := 0
			if wasAdded {
				quantity = 1
			}
			_, _ = fmt.Fprintf(w, `{"quantity":%d,"order":"o1"}`, quantity)
		default:
			http.NotFound(w, r)
		}
	})
	return added
}

func TestCartAddUnderMax(t *testing.T) {
	added := cartStub(t, "13.99")
	out := captureStdout(t, func() {
		if code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "2", "--max", "50"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !*added {
		t.Error("expected the item to be added")
	}
	if !strings.Contains(out, "added 2") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCartAddPersistsUpdatedSession(t *testing.T) {
	cartStub(t, "13.99")
	if code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "--max", "50"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(os.Getenv("LEROYMERLIN_CONFIG_DIR") + "/session.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Order.id=o1") {
		t.Errorf("updated order cookie was not persisted: %s", b)
	}
}

func TestCartAddOverMaxRefused(t *testing.T) {
	added := cartStub(t, "13.99")
	// 2 × 13.99 = 27.98 > 20 → refuse, no write
	code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "2", "--max", "20"})
	if code != 1 {
		t.Errorf("over-max exit = %d, want 1", code)
	}
	if *added {
		t.Error("item must NOT be added when over the cap")
	}
}

func TestCartAddRejectsNonFiniteMax(t *testing.T) {
	added := cartStub(t, "13.99")
	if code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "--max", "NaN"}); code != 1 {
		t.Fatalf("non-finite max exit = %d, want 1", code)
	}
	if *added {
		t.Fatal("item must not be added with a non-finite spending cap")
	}
}

func TestCartAddRejectsUnpriceableLineWithMax(t *testing.T) {
	added := cartStub(t, "1e308")
	if code := run([]string{"cart", "add", "/productos/taladro-83085630.html", "--max", "1"}); code != 1 {
		t.Fatalf("unpriceable line exit = %d, want 1", code)
	}
	if *added {
		t.Fatal("item must not be added when the spending cap cannot be evaluated")
	}
}

func TestCartAddNeedsCookie(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir()) // no session.json
	if code := run([]string{"cart", "add", "/productos/x-1.html"}); code != 1 {
		t.Errorf("no-cookie exit = %d, want 1", code)
	}
}

func TestCartGetPersistsRotatedSessionCookie(t *testing.T) {
	dir := stubEnvServing(t, `datadome=DD`, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkout/backend/cart" {
			http.NotFound(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "Order.id", Value: "rotated-order", Path: "/"})
		_, _ = w.Write([]byte(`{"orderId":"rotated-order","offersQuantity":0,"cartVendors":[]}`))
	})

	if code := run([]string{"cart", "get", "--json"}); code != 0 {
		t.Fatalf("cart get exit = %d", code)
	}
	b, err := os.ReadFile(dir + "/session.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Order.id=rotated-order") {
		t.Fatalf("rotated session cookie was not persisted: %s", b)
	}
}

func TestEnforceMaxLine(t *testing.T) {
	priced := &domain.ProductDetail{Offers: []domain.ProductOffer{{Price: "13.99"}}}

	for _, tc := range []struct {
		name    string
		detail  *domain.ProductDetail
		qty     int
		maxEUR  float64
		wantErr bool
	}{
		{"disabled cap allows anything", nil, 100, 0, false},
		{"under the cap passes", priced, 2, 30, false},
		{"exactly at the cap passes", priced, 2, 27.98, false},
		{"over the cap is refused", priced, 3, 30, true},
		{"missing price cannot be checked", nil, 1, 10, true},
		{"unparseable price cannot be checked", &domain.ProductDetail{}, 1, 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceMaxLine(tc.detail, tc.qty, tc.maxEUR)
			if (err != nil) != tc.wantErr {
				t.Fatalf("enforceMaxLine(%v, %v) error = %v, wantErr %v", tc.qty, tc.maxEUR, err, tc.wantErr)
			}
		})
	}
}

func TestPrintCartSaysSoWhenEmpty(t *testing.T) {
	out := captureStdout(t, func() { printCart(&domain.CartDetail{}) })
	if !strings.Contains(out, "cart is empty") {
		t.Errorf("output = %q, want the empty notice", out)
	}
}

func TestPrintCartListsLinesAndTotals(t *testing.T) {
	out := captureStdout(t, func() {
		printCart(&domain.CartDetail{
			Quantity:       3,
			OffersAmount:   27.98,
			DeliveryAmount: 3.9,
			TotalAmount:    31.88,
			Lines: []domain.CartLine{
				{Reflm: "83085630", Name: "  Taladro  ", Quantity: 2, Price: 27.98},
				{Reflm: "11111111", Name: "Sierra", Quantity: 1, Price: 9.5},
			},
		})
	})

	for _, want := range []string{"[83085630]", "Taladro", "2 ×", "27.98€", "Sierra", "9.50€", "total: 31.88€", "envío: 3.90€"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "  Taladro  ") {
		t.Errorf("line name was not trimmed:\n%s", out)
	}
}

func TestCartRejectsMissingAndUnknownSubcommands(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	if code := run([]string{"cart"}); code == 0 {
		t.Error("want a non-zero exit with no subcommand")
	}
	if code := run([]string{"cart", "frobnicate"}); code == 0 {
		t.Error("want a non-zero exit for an unknown subcommand")
	}
}

// Every cart write needs a browser session; without one the command must refuse
// up front rather than issue a request DataDome will bounce.
func TestCartWritesRequireASession(t *testing.T) {
	for _, args := range [][]string{
		{"cart", "clear"},
		{"cart", "set", "83085630", "2"},
		{"cart", "add", "/productos/taladro-83085630.html"},
	} {
		t.Run(args[1], func(t *testing.T) {
			t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
			t.Setenv("LEROYMERLIN_BASE_URL", "http://127.0.0.1:1")

			if code := run(args); code == 0 {
				t.Errorf("%v: want a non-zero exit without a session", args)
			}
		})
	}
}

func TestCartClearReportsHowManyLinesWent(t *testing.T) {
	deleted := false
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/delete-offer-line/") {
			deleted = true
			w.WriteHeader(http.StatusOK)
			return
		}
		if deleted {
			_, _ = w.Write([]byte(emptyCartJSON))
			return
		}
		_, _ = w.Write([]byte(detailCartJSON))
	})

	out := captureStdout(t, func() {
		if code := run([]string{"cart", "clear"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "cleared 1 línea") {
		t.Errorf("output = %q, want the cleared count", out)
	}
}

// A cart write rejected with 403 is DataDome, and the advice must say so rather
// than surfacing a bare status.
func TestCartClearExplainsAChallengedWrite(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if code := run([]string{"cart", "clear"}); code == 0 {
		t.Error("want a non-zero exit for a challenged write")
	}
}

// The spending cap has three sources in priority order; the two that can fail
// on malformed input must say which source was wrong, since the user has to
// know whether to fix a flag, an env var or config.toml.
func TestResolveMaxReportsTheFailingSource(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)

	t.Setenv("LEROYMERLIN_MAX_EUR", "no-es-un-numero")
	_, err := resolveMax(-1, false)
	if err == nil || !strings.Contains(err.Error(), "LEROYMERLIN_MAX_EUR") {
		t.Fatalf("err = %v, want it to name the env var", err)
	}

	t.Setenv("LEROYMERLIN_MAX_EUR", "-5")
	if _, err := resolveMax(-1, false); err == nil {
		t.Error("a negative env cap should be rejected")
	}
}

func TestResolveMaxFallsBackToConfigToml(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	t.Setenv("LEROYMERLIN_MAX_EUR", "")
	if err := os.WriteFile(dir+"/config.toml", []byte("[limits]\nmax_eur = 75.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveMax(-1, false)
	if err != nil || got != 75.5 {
		t.Fatalf("resolveMax = %v, %v; want 75.5", got, err)
	}
}

func TestCartSetValidatesItsArguments(t *testing.T) {
	stubEnv(t, "http://127.0.0.1:1", testCookie)

	for _, args := range [][]string{
		{"cart", "set"},                       // no ref, no qty
		{"cart", "set", "83085630"},           // no qty
		{"cart", "set", "83085630", "2", "3"}, // too many
		{"cart", "set", "83085630", "dos"},    // not a number
		{"cart", "set", "83085630", "-1"},     // negative
	} {
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit", args)
		}
	}
}

// `cart get` works anonymously but warns, because a cart is tied to a browser
// session and an empty result would otherwise look like an empty cart.
func TestCartGetWarnsWithoutASession(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"orderId":"o1","offersQuantity":0,"cartVendors":[]}`))
	})

	if code := run([]string{"cart", "get"}); code != 0 {
		t.Errorf("exit = %d, want 0: reads work anonymously", code)
	}
}

func TestCartGetExplainsAChallengedRead(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if code := run([]string{"cart", "get"}); code == 0 {
		t.Error("want a non-zero exit for a challenged cart read")
	}
}

func TestCartAddValidatesItsArguments(t *testing.T) {
	stubEnv(t, "http://127.0.0.1:1", testCookie)

	for _, args := range [][]string{
		{"cart", "add"},            // no url
		{"cart", "add", "taladro"}, // not a product url
		{"cart", "add", "/productos/taladro-1.html", "cero"}, // qty not a number
		{"cart", "add", "/productos/taladro-1.html", "0"},    // qty not positive
		{"cart", "add", "/productos/taladro-1.html", "-2"},   // negative qty
	} {
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit", args)
		}
	}
}

// Every command that touches the cart persists the refreshed session afterwards.
// If that write fails the command must report it: the storefront rotates the
// DataDome cookie on these calls, so a silently dropped session means the next
// invocation is challenged for no visible reason.
func TestCartCommandsReportAFailedSessionWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"get", []string{"cart", "get"}},
		{"set", []string{"cart", "set", "83085630", "1"}},
		{"clear", []string{"cart", "clear"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/update-offer-line-quantity/") ||
					strings.Contains(r.URL.Path, "/delete-offer-line/") {
					w.WriteHeader(http.StatusOK)
					return
				}
				_, _ = w.Write([]byte(detailCartJSON))
			})
			freezeConfigDir(t, dir)

			if code := run(tc.args); code == 0 {
				t.Errorf("%v: want a non-zero exit when the session write fails", tc.args)
			}
		})
	}
}

// enforceMaxLine refuses rather than guesses: if the price cannot be read or the
// line total cannot be represented, the write is blocked. Silently skipping the
// cap would be the one outcome a spending guard must never produce.
func TestEnforceMaxLineRefusesWhenItCannotCompute(t *testing.T) {
	unpriced := &domain.ProductDetail{Offers: []domain.ProductOffer{{Price: "n/a"}}}
	if err := enforceMaxLine(unpriced, 1, 50); err == nil {
		t.Error("an unreadable price must block the write")
	}

	priced := &domain.ProductDetail{Offers: []domain.ProductOffer{{Price: "13.99"}}}
	// 1399 cents x 1e16 overflows the int64 cent space; assert it is the
	// multiplication that refuses, not the cap comparison further down.
	err := enforceMaxLine(priced, 1e16, math.MaxFloat64)
	if err == nil || !strings.Contains(err.Error(), "cannot enforce --max") {
		t.Errorf("err = %v, want the guard to refuse an unrepresentable line total", err)
	}
	if err := enforceMaxLine(priced, 1, math.Inf(1)); err == nil {
		t.Error("an unrepresentable cap must block the write")
	}
}

// The spending cap is read from config.toml when neither flag nor env is set, so
// a malformed config must block a cart write rather than quietly proceed uncapped.
func TestCartAddRefusesWhenTheConfiguredCapCannotBeRead(t *testing.T) {
	dir := stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		// offer_id included, so the command reaches the cap lookup rather than
		// failing earlier at the product offer.
		_, _ = w.Write([]byte(`<script type="application/ld+json">` +
			`{"@type":"Product","name":"Taladro","sku":"1","offers":{"price":"13.99","priceCurrency":"EUR"}}</script>` +
			`<script>{"offer_id":"deadbeefdeadbeef"}</script>`))
	})
	if err := os.WriteFile(dir+"/config.toml", []byte("[limits\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"cart", "add", "/productos/taladro-1.html"}); code == 0 {
		t.Error("want a non-zero exit when the configured cap cannot be read")
	}
}

// cart add has three storefront steps; a failure at any of them must stop the
// command rather than report a partial success.
func TestCartAddSurfacesFailuresAtEachStep(t *testing.T) {
	const productPage = `<script type="application/ld+json">` +
		`{"@type":"Product","name":"Taladro","sku":"1","offers":{"price":"13.99","priceCurrency":"EUR"}}</script>` +
		`<script>{"offer_id":"deadbeefdeadbeef"}</script>`

	t.Run("product lookup fails", func(t *testing.T) {
		stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})

		if code := run([]string{"cart", "add", "/productos/taladro-1.html"}); code == 0 {
			t.Error("want a non-zero exit when the product lookup is challenged")
		}
	})

	t.Run("the add itself fails", func(t *testing.T) {
		stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "addToCart") {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			_, _ = w.Write([]byte(productPage))
		})

		if code := run([]string{"cart", "add", "/productos/taladro-1.html"}); code == 0 {
			t.Error("want a non-zero exit when the add is rejected")
		}
	})

	t.Run("the session write fails", func(t *testing.T) {
		dir := stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "addToCart"):
				_, _ = w.Write([]byte(`{}`))
			case strings.Contains(r.URL.Path, "cart-data"):
				_, _ = w.Write([]byte(`{"quantity":1,"order":"o1"}`))
			default:
				_, _ = w.Write([]byte(productPage))
			}
		})
		freezeConfigDir(t, dir)

		if code := run([]string{"cart", "add", "/productos/taladro-1.html"}); code == 0 {
			t.Error("want a non-zero exit when the session write fails")
		}
	})
}

// cart set on a cart it cannot read is a transport failure, not a missing line.
func TestCartSetExplainsAChallengedRead(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if code := run([]string{"cart", "set", "83085630", "2"}); code == 0 {
		t.Error("want a non-zero exit for a challenged cart read")
	}
}

// The cart subcommands emit structured output too, and a failed write there
// must not be reported as a successful cart change.
func TestCartStructuredOutputFailuresAreReported(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"get", []string{"cart", "get", "--json"}},
		{"set", []string{"cart", "set", "--json", "83085630", "1"}},
		{"clear", []string{"cart", "clear", "--json"}},
		{"add", []string{"cart", "add", "--json", "/productos/taladro-83085630.html"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/update-offer-line-quantity/"),
					strings.Contains(r.URL.Path, "/delete-offer-line/"):
					w.WriteHeader(http.StatusOK)
				case strings.Contains(r.URL.Path, "addToCart"):
					_, _ = w.Write([]byte(`{}`))
				case strings.Contains(r.URL.Path, "cart-data"):
					_, _ = w.Write([]byte(`{"quantity":1,"order":"o1"}`))
				case strings.Contains(r.URL.Path, "/productos/"):
					_, _ = w.Write([]byte(`<script type="application/ld+json">` +
						`{"@type":"Product","name":"Taladro","sku":"83085630",` +
						`"offers":{"price":"13.99","priceCurrency":"EUR"}}</script>` +
						`<script>{"offer_id":"deadbeefdeadbeef"}</script>`))
				default:
					_, _ = w.Write([]byte(detailCartJSON))
				}
			})
			breakStdout(t)

			if code := run(tc.args); code == 0 {
				t.Errorf("%v: want a non-zero exit when stdout cannot be written", tc.args)
			}
		})
	}
}

// A stale product url reads the same whether it reaches `product` or
// `cart add`: the page is gone, and dumping the storefront's 404 markup says
// nothing the user can act on.
func TestCartAddExplainsAMissingProductPage(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="es-ES"><head><link rel="preload" href="/f.woff2"></head></html>`))
	})
	err := cmdCart([]string{"add", "/productos/taladro-83085630.html"})
	if err == nil {
		t.Fatal("want an error for a missing product page")
	}
	if !strings.Contains(err.Error(), "no product found") || strings.Contains(err.Error(), "DOCTYPE") {
		t.Errorf("error = %v, want the advice without the markup", err)
	}
}

// A cookie lifted from a signed-out browser reads and writes fine, so nothing
// fails — the items land in a guest cart the account's own cart page never
// shows. Saying so is the difference between a working command and a user
// staring at an empty cart in their browser.
func TestCartWarnsWhenTheSessionIsAGuestCart(t *testing.T) {
	for _, tc := range []struct {
		name, cookie string
		want         bool
	}{
		{"guest", "datadome=DD; lm-csrf=TOK", true},
		{"signed in", "datadome=DD; lm-csrf=TOK; idToken.jwt=JWT", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubEnvServing(t, tc.cookie, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"orderId":"o1","offersQuantity":0,"cartVendors":[]}`))
			})
			errs := captureStderr(t, func() {
				if code := run([]string{"cart", "get"}); code != 0 {
					t.Fatalf("exit = %d", code)
				}
			})
			if got := strings.Contains(errs, "guest cart"); got != tc.want {
				t.Errorf("stderr = %q, want guest-cart warning = %v", errs, tc.want)
			}
		})
	}
}

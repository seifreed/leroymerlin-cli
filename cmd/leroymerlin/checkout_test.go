package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const detailCartJSON = `{
  "orderId":"o1","offersQuantity":2,"disabledCheckout":false,
  "cannotBeValidatedReasons":["ORDER_NEED_TO_BE_LINKED_TO_A_CUSTOMER"],
  "orderResume":{"totalAmount":31.88,"offersAmount":27.98,"deliveryAmount":3.9},
  "cartVendors":[{"cartVendorItems":[
    {"id":"line-1","quantity":2,"discountPrice":27.98,"offer":{"refLM":"83085630","label":"Taladro"}}
  ]}]
}`

// emptyCartJSON is the same cart once its only line is gone.
const emptyCartJSON = `{"orderId":"o1","offersQuantity":0,"orderResume":{},"cartVendors":[]}`

// cartMutStub serves the detailed cart and records PUT/DELETE mutations.
func cartMutStub(t *testing.T) (puts, deletes *[]string) {
	t.Helper()
	var mu sync.Mutex
	var p, d []string
	puts, deletes = &p, &d
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/checkout/backend/cart":
			body := detailCartJSON
			if len(d) > 0 {
				body = emptyCartJSON
			}
			_, _ = w.Write([]byte(body))
		case strings.Contains(r.URL.Path, "/update-offer-line-quantity/"):
			p = append(p, r.URL.Path)
			w.WriteHeader(200)
		case strings.Contains(r.URL.Path, "/delete-offer-line/"):
			d = append(d, r.URL.Path)
			w.WriteHeader(200)
		default:
			http.NotFound(w, r)
		}
	})
	return puts, deletes
}

func TestCartSetUpdatesQuantity(t *testing.T) {
	puts, _ := cartMutStub(t)
	if code := run([]string{"cart", "set", "83085630", "5"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(*puts) != 1 || !strings.HasSuffix((*puts)[0], "/line-1") {
		t.Errorf("expected one PUT to line-1, got %v", *puts)
	}
}

func TestCartSetZeroDeletes(t *testing.T) {
	_, deletes := cartMutStub(t)
	if code := run([]string{"cart", "set", "83085630", "0"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(*deletes) != 1 {
		t.Errorf("qty 0 should DELETE, got %v", *deletes)
	}
}

func TestCartSetUnknownRef(t *testing.T) {
	cartMutStub(t)
	if code := run([]string{"cart", "set", "99999", "1"}); code != 1 {
		t.Errorf("unknown ref exit = %d, want 1", code)
	}
}

func TestCartClear(t *testing.T) {
	_, deletes := cartMutStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"cart", "clear"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if len(*deletes) != 1 {
		t.Errorf("clear should DELETE each line, got %v", *deletes)
	}
	if !strings.Contains(out, "cleared 1") {
		t.Errorf("output:\n%s", out)
	}
}

func TestAddressLine(t *testing.T) {
	a := map[string]any{
		"firstName": "Ada", "lastName": "Lovelace", "line1": "Calle Falsa 123",
		"postalCode": "08001", "city": "Barcelona", "phoneNumber": "600",
	}
	got := addressLine(a)
	if got != "Ada, Lovelace, Calle Falsa 123, 08001, Barcelona, 600" {
		t.Errorf("addressLine = %q", got)
	}
	if addressLine(nil) != "" {
		t.Error("nil address should render empty")
	}
}

const shippingStubJSON = `{
  "addresses": {"deliveryAddress": {"line1":"Calle Falsa 123","city":"Barcelona"}, "invoiceAddress": null},
  "deliveryVendors": [{"deliveryVendorDeliveryGroups": [{"deliveryVendorServiceLevels": [
    {"mode":"PICKUP_IN_STORE","labelCode":"PICKUP_EXP","amount":0,"selected":true,"appointmentDate":"2026-07-01T18:30:00+02:00"},
    {"mode":"HOME_DELIVERY","labelCode":"HOME_STD","amount":3.9,"selected":false,"appointmentDate":"2026-07-02T06:00:00+02:00"}
  ]}]}]
}`

func shippingStub(t *testing.T) {
	t.Helper()
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checkout/backend/cart" {
			_, _ = w.Write([]byte(detailCartJSON))
			return
		}
		if strings.Contains(r.URL.Path, "/checkout/backend/shipping") {
			_, _ = w.Write([]byte(shippingStubJSON))
			return
		}
		http.NotFound(w, r)
	})
}

func emptyCheckoutStub(t *testing.T) {
	t.Helper()
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checkout/backend/cart" {
			_, _ = w.Write([]byte(`{"orderId":"o1","offersQuantity":0,"cartVendors":[]}`))
			return
		}
		t.Errorf("shipping must not be called for an empty cart: %s", r.URL.Path)
		http.NotFound(w, r)
	})
}

func TestCheckoutSlotsEmptyCart(t *testing.T) {
	emptyCheckoutStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "slots", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("empty slots json = %q", out)
	}
}

func TestCheckoutAddressesEmptyCart(t *testing.T) {
	emptyCheckoutStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "addresses", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if strings.TrimSpace(out) != "{}" {
		t.Errorf("empty addresses json = %q", out)
	}
}

func TestCheckoutSlots(t *testing.T) {
	shippingStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "slots", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"mode": "PICKUP_IN_STORE"`) || !strings.Contains(out, `"amount": 3.9`) {
		t.Errorf("slots json wrong:\n%s", out)
	}
}

func TestCheckoutAddresses(t *testing.T) {
	shippingStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "addresses"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "Calle Falsa 123") || !strings.Contains(out, "entrega:") {
		t.Errorf("addresses output wrong:\n%s", out)
	}
}

func TestCheckoutStatus(t *testing.T) {
	cartMutStub(t)
	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"total": 31.88`) || !strings.Contains(out, `"ready": false`) {
		t.Errorf("checkout json wrong:\n%s", out)
	}
}

// A mistyped subcommand must say so rather than quietly printing the status
// report, which is what `checkout slot` used to do.
func TestCheckoutRejectsAnUnknownSubcommand(t *testing.T) {
	cartMutStub(t)
	if code := run([]string{"checkout", "slot"}); code == 0 {
		t.Error("want a non-zero exit for an unknown checkout subcommand")
	}
}

func TestCartGetDetailed(t *testing.T) {
	cartMutStub(t)
	out := captureStdout(t, func() { run([]string{"cart", "get"}) })
	if !strings.Contains(out, "[83085630] Taladro — 2 ×") || !strings.Contains(out, "total: 31.88€") {
		t.Errorf("cart get detailed wrong:\n%s", out)
	}
}

// checkoutCartStub serves one fixed cart payload to the checkout endpoint.
func checkoutCartStub(t *testing.T, cartJSON string) {
	t.Helper()
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkout/backend/cart" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(cartJSON))
	})
}

func TestCheckoutReportsBlockersInHumanForm(t *testing.T) {
	checkoutCartStub(t, detailCartJSON)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})

	for _, want := range []string{"31.88", "27.98", "3.90", "2 artículos", "blocked", "needs an account"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestCheckoutReportsReadyWhenNothingBlocks(t *testing.T) {
	checkoutCartStub(t, `{"orderId":"o1","offersQuantity":1,"disabledCheckout":false,
	  "cannotBeValidatedReasons":[],
	  "orderResume":{"totalAmount":10,"offersAmount":10,"deliveryAmount":0},
	  "cartVendors":[{"cartVendorItems":[{"id":"l1","quantity":1,"offer":{"refLM":"1","label":"x"}}]}]}`)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "ready") {
		t.Errorf("output = %q, want it to report ready", out)
	}
}

func TestCheckoutSaysSoOnAnEmptyCart(t *testing.T) {
	checkoutCartStub(t, `{"orderId":"","offersQuantity":0,"orderResume":{},"cartVendors":[]}`)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "cart is empty") {
		t.Errorf("output = %q, want the empty-cart notice", out)
	}
}

// --toon is the agent-facing output path; it must carry the same fields as the
// human view and stay parseable.
func TestCheckoutToonOutputCarriesTheTotals(t *testing.T) {
	checkoutCartStub(t, detailCartJSON)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "--toon"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	for _, want := range []string{"total", "31.88", "blockers"} {
		if !strings.Contains(out, want) {
			t.Errorf("TOON output missing %q:\n%s", want, out)
		}
	}
}

// The --json paths are covered above; these pin the human-readable view, where
// the selected slot is marked and free delivery reads "gratis" rather than 0.00€.
func TestCheckoutSlotsHumanViewMarksTheSelectedSlot(t *testing.T) {
	shippingStub(t)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "slots"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})

	for _, want := range []string{"pickup exp", "gratis", "home std", "3.90€"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\u2605 pickup exp") {
		t.Errorf("the selected slot is not the marked one:\n%s", out)
	}
	if strings.Contains(out, "\u2605 home std") {
		t.Errorf("an unselected slot was marked:\n%s", out)
	}
}

func TestCheckoutSlotsHumanViewOnAnEmptyCart(t *testing.T) {
	emptyCheckoutStub(t)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "slots"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "cart is empty") {
		t.Errorf("output = %q, want the empty-cart notice", out)
	}
}

// A populated cart with no offered slots is a different message from an empty
// cart, and reaches a different branch.
func TestCheckoutSlotsReportsNoneAvailable(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checkout/backend/cart":
			_, _ = w.Write([]byte(detailCartJSON))
		case "/checkout/backend/shipping":
			_, _ = w.Write([]byte(`{"deliveryVendors":[]}`))
		default:
			http.NotFound(w, r)
		}
	})

	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "slots"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "none available") {
		t.Errorf("output = %q, want the no-slots notice", out)
	}
}

// Without a session the command refuses up front rather than issuing a request
// DataDome will bounce.
func TestCheckoutSlotsRequiresASession(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
	t.Setenv("LEROYMERLIN_BASE_URL", "http://127.0.0.1:1")

	if code := run([]string{"checkout", "slots"}); code == 0 {
		t.Error("want a non-zero exit without a session")
	}
}

func TestCheckoutAddressesRequiresASession(t *testing.T) {
	stubEnv(t, "http://127.0.0.1:1", "")

	if code := run([]string{"checkout", "addresses"}); code == 0 {
		t.Error("want a non-zero exit without a session")
	}
}

func TestCheckoutAddressesExplainsAChallengedRead(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if code := run([]string{"checkout", "addresses"}); code == 0 {
		t.Error("want a non-zero exit for a challenged read")
	}
}

// The human view of an empty cart says so rather than printing a blank block.
func TestCheckoutAddressesHumanViewOnAnEmptyCart(t *testing.T) {
	emptyCheckoutStub(t)

	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "addresses"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "cart is empty") {
		t.Errorf("output = %q, want the empty-cart notice", out)
	}
}

// A cart with no saved addresses — store pickup, typically — must say so rather
// than print an empty block that reads as a rendering bug.
func TestCheckoutAddressesSaysSoWhenNoneAreSet(t *testing.T) {
	shippingStub := func() {
		stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/checkout/backend/cart":
				_, _ = w.Write([]byte(detailCartJSON))
			default:
				_, _ = w.Write([]byte(`{"addresses":{},"deliveryVendors":[]}`))
			}
		})
	}
	shippingStub()

	out := captureStdout(t, func() {
		if code := run([]string{"checkout", "addresses"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "no addresses set") {
		t.Errorf("output = %q, want the no-addresses notice", out)
	}
}

func TestCheckoutCommandsReportAFailedSessionWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"status", []string{"checkout"}},
		{"addresses", []string{"checkout", "addresses"}},
		{"slots", []string{"checkout", "slots"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := stubEnvServing(t, testCookie, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/checkout/backend/cart" {
					_, _ = w.Write([]byte(detailCartJSON))
					return
				}
				_, _ = w.Write([]byte(shippingStubJSON))
			})
			freezeConfigDir(t, dir)

			if code := run(tc.args); code == 0 {
				t.Errorf("%v: want a non-zero exit when the session write fails", tc.args)
			}
		})
	}
}

// A challenged read on any checkout subcommand becomes the DataDome advice.
func TestCheckoutSubcommandsExplainAChallengedRead(t *testing.T) {
	for _, args := range [][]string{
		{"checkout"},
		{"checkout", "addresses"},
		{"checkout", "slots"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		stubEnv(t, srv.URL, testCookie)
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit for a challenged read", args)
		}
		srv.Close()
	}
}

func TestCheckoutStructuredOutputFailuresAreReported(t *testing.T) {
	for _, args := range [][]string{
		{"checkout", "--json"},
		{"checkout", "addresses", "--json"},
		{"checkout", "slots", "--json"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/checkout/backend/cart" {
				_, _ = w.Write([]byte(detailCartJSON))
				return
			}
			_, _ = w.Write([]byte(shippingStubJSON))
		}))
		stubEnv(t, srv.URL, testCookie)
		breakStdout(t)

		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit when stdout cannot be written", args)
		}
		srv.Close()
	}
}

// `cart set` raises a quantity as freely as `cart add` adds one — 9999 units of
// a 0.59€ tape is 5 899€ — so it answers to the same cap, priced from the cart's
// own line (13.99€ × 5 here).
func TestCartSetRefusesOverTheCap(t *testing.T) {
	puts, _ := cartMutStub(t)
	if code := run([]string{"cart", "set", "83085630", "5", "--max", "20"}); code != 1 {
		t.Errorf("over-cap exit = %d, want 1", code)
	}
	if len(*puts) != 0 {
		t.Errorf("nothing must be written over the cap, got %v", *puts)
	}
}

func TestCartSetUnderTheCapWrites(t *testing.T) {
	puts, _ := cartMutStub(t)
	if code := run([]string{"cart", "set", "83085630", "5", "--max", "100"}); code != 0 {
		t.Errorf("under-cap exit = %d, want 0", code)
	}
	if len(*puts) != 1 {
		t.Errorf("expected one PUT under the cap, got %v", *puts)
	}
}

// The cap comes from the environment too, and removing a line is never over it.
func TestCartSetCapFromEnvAndZeroAlwaysRemoves(t *testing.T) {
	_, deletes := cartMutStub(t)
	t.Setenv("LEROYMERLIN_MAX_EUR", "1")
	if code := run([]string{"cart", "set", "83085630", "3"}); code != 1 {
		t.Errorf("env cap exit = %d, want 1", code)
	}
	if code := run([]string{"cart", "set", "83085630", "0"}); code != 0 {
		t.Errorf("removal exit = %d, want 0 even under a 1€ cap", code)
	}
	if len(*deletes) != 1 {
		t.Errorf("qty 0 should DELETE, got %v", *deletes)
	}
}

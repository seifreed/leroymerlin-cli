package main

import (
	"fmt"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// cmdLogin lifts the Leroy Merlin cookie out of the cookie store of a browser
// the user already uses, and caches it. This is the first thing anyone runs, and
// the browser has to be one they are signed in at: DataDome challenges a client
// it does not recognise, so without that session even a search gets a 403, and
// the session is also what makes prices, stock and the cart theirs rather than a
// stranger's. No credentials are handled here — the browser did that, and this
// reads the cookie it left behind.
// cookiesFromBrowser is the browser-store scan, indirected so tests can drive
// everything after it — persist, reload, probe — without an installed browser.
// The seam inside internal/client is unexported, so cmd needs its own.
var cookiesFromBrowser = client.CookiesFromBrowser

func cmdLogin(args []string) error {
	fs, cf := newCommonFlags("login")
	fromBrowser := fs.String("from-browser", "", "which browser store to read: chrome|chromium|firefox|safari|edge|brave (default: every installed one)")
	parseFlags(fs, args)

	s, err := cookiesFromBrowser(*fromBrowser)
	if err != nil {
		return err
	}
	if err := saveSession(s); err != nil {
		return err
	}

	cl := newClient()
	if cl.Cookie == "" {
		return fmt.Errorf("cookie saved but did not load back")
	}
	ok, emitted, err := probeReads(cf, cl)
	if err != nil || emitted {
		return err
	}
	if !ok {
		return fmt.Errorf("cookie saved but reads are still challenged — sign in at www.leroymerlin.es in that browser, load a page, then retry")
	}
	fmt.Println("ok — cookie lifted from browser, reads working")
	warnGuestCart(cl)
	return nil
}

// cmdWhoami reports whether a session is cached and whether reads currently get
// through to the storefront with it.
func cmdWhoami(args []string) error {
	fs, cf := newCommonFlags("whoami")
	parseFlags(fs, args)

	cl := newClient()
	hasCookie := cl.Cookie != ""
	ok, emitted, err := probeReads(cf, cl)
	if err != nil || emitted {
		return err
	}
	cookieState := "no session cached — run `leroymerlin login --from-browser chrome`"
	if hasCookie {
		// Reads working is not the whole answer: cart and checkout need the cookie
		// to carry the account, and a session that lost it looks fine until the
		// first cart command refuses.
		cookieState = "signed-in cookie cached"
		if !client.CookieIsSignedIn(cl.Cookie) {
			cookieState = "cookie cached but signed out (cart and checkout refuse it)"
		}
	}
	if ok {
		fmt.Printf("ok — reads working; %s\n", cookieState)
		if hasCookie && !client.CookieIsSignedIn(cl.Cookie) {
			fmt.Println("  sign in at www.leroymerlin.es, then run `leroymerlin login --from-browser chrome` again")
		}
		return nil
	}
	return fmt.Errorf("reads are being challenged; %s — try `leroymerlin login --from-browser chrome`", cookieState)
}

// probeReads checks whether catalog reads are getting through and emits the
// {cookie, reads_ok} structured view that login and whoami both report. emitted
// is true when a structured format was requested, meaning the caller has already
// said everything it needs to.
func probeReads(cf *common, cl *client.Client) (ok, emitted bool, err error) {
	ok, err = cl.CheckAuth()
	if err != nil {
		return false, false, fmt.Errorf("could not verify reads: %w", err)
	}
	emitted, err = emitStructured(cf, map[string]any{
		"cookie":    cl.Cookie != "",
		"signed_in": client.CookieIsSignedIn(cl.Cookie),
		"reads_ok":  ok,
	})
	return ok, emitted, err
}

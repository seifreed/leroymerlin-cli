package main

import (
	"flag"
	"fmt"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// cmdLogin lifts the Leroy Merlin cookie (DataDome clearance) from a browser's
// cookie store and caches it — the easy WAF fallback, no DevTools. Reads are
// anonymous, so there is no account login; this only seeds the clearance cookie
// for when an anonymous read draws a challenge.
func cmdLogin(args []string) error {
	fs, cf := newCommonFlags("login")
	fromBrowser := fs.String("from-browser", "", "read cookies from a browser store: chrome|chromium|firefox|safari|edge|brave (empty = any installed)")
	parseFlags(fs, args)

	fromBrowserSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "from-browser" {
			fromBrowserSet = true
		}
	})
	if !fromBrowserSet {
		return fmt.Errorf("usage: leroymerlin login --from-browser <chrome|firefox|safari|edge|brave>\n" +
			"(reads need no account; this only seeds the DataDome cookie for challenged reads)")
	}

	s, err := client.CookiesFromBrowser(*fromBrowser)
	if err != nil {
		return err
	}
	if err := client.SaveSession(s); err != nil {
		return err
	}

	cl := newClient(cf)
	if !cl.LoadAuth() {
		return fmt.Errorf("cookie saved but did not load back")
	}
	ok, err := cl.CheckAuth()
	if err != nil {
		return fmt.Errorf("could not verify reads: %w", err)
	}
	if done, err := emitStructured(cf, map[string]any{"cookie": true, "reads_ok": ok}); done {
		return err
	}
	if !ok {
		return fmt.Errorf("cookie saved but reads are still challenged — open www.leroymerlin.es in the browser, then retry")
	}
	fmt.Println("ok — cookie lifted from browser, reads working")
	return nil
}

// cmdWhoami reports whether a cookie is cached and whether reads currently get
// through to the storefront (anonymously or with that cookie).
func cmdWhoami(args []string) error {
	fs, cf := newCommonFlags("whoami")
	parseFlags(fs, args)

	cl := newClient(cf)
	hasCookie := cl.LoadAuth()
	ok, err := cl.CheckAuth()
	if err != nil {
		return fmt.Errorf("could not verify reads: %w", err)
	}
	if done, err := emitStructured(cf, map[string]any{"cookie": hasCookie, "reads_ok": ok}); done {
		return err
	}
	cookieState := "no cookie cached (reads are anonymous)"
	if hasCookie {
		cookieState = "cookie cached"
	}
	if ok {
		fmt.Printf("ok — reads working; %s\n", cookieState)
		return nil
	}
	return fmt.Errorf("reads are being challenged; %s — try `leroymerlin login --from-browser chrome`", cookieState)
}

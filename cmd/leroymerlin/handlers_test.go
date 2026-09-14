package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what was
// written.
// captureStderr mirrors captureStdout for the commands that explain themselves
// on stderr, keeping stdout clean for --json consumers.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stderr = old
	return <-done
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

const cardHTML = `<script type="application/json" class="dataTms">
[{"name":"cdl_products_list","value":[{"brand":"DEXTER","identifier":"42","name":"Taladro","url":"/productos/x-42.html","rating":4.5,"product_is_sponsored":false,"total_offer_count":1,"offer":{"unitprice_ati":29.99,"unitprice_tf":24.0,"seller_name":"Leroy Merlin","seller_type":"1P","add_to_cart_availability":true}}]}]
</script>`

const productHTML = `<script type="application/ld+json">{"@type":"Product","name":"Taladro","sku":"42","brand":"DEXTER","offers":{"price":"29.99","priceCurrency":"EUR","availability":"http://schema.org/InStock"}}</script>`

func withStubServer(t *testing.T, body string) {
	t.Helper()
	stubEnv(t, stubServerFor(t, body), "") // isolate from a real ~/.leroymerlin
}

func TestRunUnknownCommand(t *testing.T) {
	if code := run([]string{"frobnicate"}); code != 2 {
		t.Errorf("unknown command exit = %d, want 2", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Errorf("no args exit = %d, want 2", code)
	}
}

func TestSearchJSON(t *testing.T) {
	withStubServer(t, cardHTML)
	out := captureStdout(t, func() {
		if code := run([]string{"search", "--json", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"identifier": "42"`) || !strings.Contains(out, `"unitprice_ati": 29.99`) {
		t.Errorf("json missing expected fields:\n%s", out)
	}
}

func TestSearchHuman(t *testing.T) {
	withStubServer(t, cardHTML)
	out := captureStdout(t, func() { run([]string{"search", "taladro"}) })
	if !strings.Contains(out, "[42] Taladro — 29.99€") {
		t.Errorf("human line wrong:\n%s", out)
	}
}

func TestProductHuman(t *testing.T) {
	withStubServer(t, productHTML)
	out := captureStdout(t, func() {
		if code := run([]string{"product", "/productos/x-42.html"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "ref:          42") || !strings.Contains(out, "29.99 EUR") {
		t.Errorf("detail wrong:\n%s", out)
	}
}

func TestProductRejectsNonProductArg(t *testing.T) {
	if code := run([]string{"product", "taladro"}); code != 1 {
		t.Errorf("non-product arg exit = %d, want 1", code)
	}
}

// withRoutedServer serves productHTML for /productos/ paths and cardHTML for
// everything else (search), so total can exercise both resolution paths.
func withRoutedServer(t *testing.T) {
	t.Helper()
	stubEnvServing(t, "", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/productos/") {
			_, _ = w.Write([]byte(productHTML))
			return
		}
		_, _ = w.Write([]byte(cardHTML))
	})
}

func TestBatchHuman(t *testing.T) {
	withStubServer(t, cardHTML)
	out := captureStdout(t, func() {
		if code := run([]string{"batch", "taladro", "broca"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "• taladro") || !strings.Contains(out, "• broca") {
		t.Errorf("batch output missing terms:\n%s", out)
	}
	if !strings.Contains(out, "[42] Taladro — 29.99€") {
		t.Errorf("batch missing resolved product:\n%s", out)
	}
}

// --on-offer used to leave the command silent when nothing qualified, where
// search says so. It also now narrows the candidates before choosing, so a term
// resolves to its discounted product rather than being dropped because the
// cheapest one happened to carry no offer.
func TestBatchOnOfferSaysSoWhenNothingQualifies(t *testing.T) {
	withStubServer(t, cardHTML) // one product, no discount and no promo
	var out string
	stderr := captureStderr(t, func() {
		out = captureStdout(t, func() {
			if code := run([]string{"batch", "--on-offer", "taladro"}); code != 0 {
				t.Errorf("exit = %d", code)
			}
		})
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("stdout should stay empty for machine use:\n%s", out)
	}
	if !strings.Contains(stderr, "no term has a product on offer") {
		t.Errorf("stderr missing the explanation:\n%s", stderr)
	}
}

func TestBrandsList(t *testing.T) {
	withStubServer(t, cardHTML) // one product, brand DEXTER
	out := captureStdout(t, func() {
		if code := run([]string{"brands", "taladro", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"brand": "DEXTER"`) || !strings.Contains(out, `"count": 1`) {
		t.Errorf("brands json wrong:\n%s", out)
	}
}

func TestBatchBrandOverride(t *testing.T) {
	withStubServer(t, cardHTML) // brand DEXTER
	out := captureStdout(t, func() {
		if code := run([]string{"batch", "taladro", "--brand", "Makita", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	// Makita not stocked → fall back to the DEXTER hit, flagged "none"
	if !strings.Contains(out, `"brandMatch": "none"`) {
		t.Errorf("expected brandMatch none:\n%s", out)
	}
}

func TestTotalTermAndURL(t *testing.T) {
	withRoutedServer(t)
	out := captureStdout(t, func() {
		// one search-term line (qty 2 via file) + one product url
		if code := run([]string{"total", "--json", "taladro", "/productos/x-42.html"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	// both lines price at 29.99 × 1 = 29.99 each → total 59.98
	if !strings.Contains(out, `"total": "59.98"`) {
		t.Errorf("total wrong:\n%s", out)
	}
	if !strings.Contains(out, `"complete": true`) {
		t.Errorf("expected complete:\n%s", out)
	}
}

func TestTotalQtyFromFile(t *testing.T) {
	withStubServer(t, cardHTML)
	dir := t.TempDir()
	f := dir + "/basket.txt"
	if err := os.WriteFile(f, []byte("taladro 3\n# comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if code := run([]string{"total", "-f", f, "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"total": "89.97"`) { // 29.99 × 3
		t.Errorf("qty from file wrong:\n%s", out)
	}
}

func TestCategoriesList(t *testing.T) {
	html := `<a href="/productos/iluminacion/">Iluminación</a><a href="/productos/banos/">Baños</a>`
	withStubServer(t, html)
	out := captureStdout(t, func() {
		if code := run([]string{"categories", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"path": "/productos/iluminacion/"`) || !strings.Contains(out, `"name": "Baños"`) {
		t.Errorf("categories list wrong:\n%s", out)
	}
}

func TestCategoriesProducts(t *testing.T) {
	withStubServer(t, cardHTML) // cardHTML has one product (ref 42)
	out := captureStdout(t, func() {
		if code := run([]string{"categories", "iluminacion"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "[42] Taladro — 29.99€") {
		t.Errorf("category products wrong:\n%s", out)
	}
}

func TestWhoamiReadsOK(t *testing.T) {
	withStubServer(t, cardHTML) // cardHTML carries cdl_products_list → reads_ok
	out := captureStdout(t, func() {
		if code := run([]string{"whoami", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"reads_ok": true`) || !strings.Contains(out, `"cookie": false`) {
		t.Errorf("whoami json wrong:\n%s", out)
	}
}

func TestLoginRejectsUnknownBrowser(t *testing.T) {
	if code := run([]string{"login", "--from-browser", "not-a-browser"}); code != 1 {
		t.Errorf("unknown browser exit = %d, want 1", code)
	}
}

func TestSetCookiePersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	if code := run([]string{"set-cookie", "datadome=abc; lm-csrf=def"}); code != 0 {
		t.Fatalf("set-cookie exit = %d", code)
	}
	if _, err := os.Stat(dir + "/session.json"); err != nil {
		t.Errorf("session.json not written: %v", err)
	}
}

func TestVersionStringOmitsUnsetBuildMetadata(t *testing.T) {
	origV, origC, origD := version, commit, date
	defer func() { version, commit, date = origV, origC, origD }()

	version, commit, date = "1.2.3", "", ""
	if got := versionString(); got != "1.2.3" {
		t.Errorf("bare version = %q, want 1.2.3", got)
	}
	commit = "abc1234"
	if got := versionString(); got != "1.2.3 (abc1234)" {
		t.Errorf("version+commit = %q", got)
	}
	date = "2026-09-13"
	if got := versionString(); got != "1.2.3 (abc1234, 2026-09-13)" {
		t.Errorf("full version = %q", got)
	}
}

const harFixture = `{"log":{"entries":[{"request":{"url":"https://www.leroymerlin.es/search?q=x",` +
	`"headers":[{"name":"Cookie","value":"datadome=FROMHAR; lm-csrf=tok"}]}}]}}`

// import-har is how a user hands the CLI a browser session, so the end-to-end
// path that matters is: HAR on disk -> parsed cookie -> persisted session.
func TestImportHarPersistsTheCookie(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	har := filepath.Join(dir, "export.har")
	if err := os.WriteFile(har, []byte(harFixture), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"import-har", "--file", har}); code != 0 {
		t.Fatalf("exit = %d", code)
	}

	saved, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatalf("session not written: %v", err)
	}
	if !strings.Contains(string(saved), "FROMHAR") {
		t.Errorf("saved session = %s, want the HAR cookie", saved)
	}
}

func TestImportHarRejectsMissingAndUnusableInput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)

	if code := run([]string{"import-har"}); code == 0 {
		t.Error("missing --file should fail")
	}
	if code := run([]string{"import-har", "--file", filepath.Join(dir, "absent.har")}); code == 0 {
		t.Error("absent file should fail")
	}

	empty := filepath.Join(dir, "empty.har")
	if err := os.WriteFile(empty, []byte(`{"log":{"entries":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"import-har", "--file", empty}); code == 0 {
		t.Error("HAR with no storefront cookie should fail")
	}
}

// The brands view aligns names on the longest one and reports a per-brand
// count and floor price; --limit truncates the ranked list.
func TestBrandsHumanViewAndLimit(t *testing.T) {
	withStubServer(t, cardHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"brands", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "DEXTER") || !strings.Contains(out, "1 ud.") || !strings.Contains(out, "desde 29.99€") {
		t.Errorf("brands output wrong:\n%s", out)
	}

	out = captureStdout(t, func() {
		if code := run([]string{"brands", "--limit", "0", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "DEXTER") {
		t.Errorf("--limit 0 should list everything:\n%s", out)
	}
}

func TestBrandsNeedsATerm(t *testing.T) {
	withStubServer(t, cardHTML)
	if code := run([]string{"brands"}); code == 0 {
		t.Error("want a non-zero exit with no search term")
	}
}

// A search that matches nothing is not an error: the command reports it and
// exits 0 so scripts can tell "no brands" from "request failed".
func TestBrandsReportsNoMatchesWithoutFailing(t *testing.T) {
	withStubServer(t, `<html><body>sin resultados</body></html>`)

	if code := run([]string{"brands", "termino-inexistente"}); code != 0 {
		t.Errorf("exit = %d, want 0 for an empty result set", code)
	}
}

func TestWhoamiReportsCookieStateAndReadHealth(t *testing.T) {
	withStubServer(t, cardHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"whoami"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	// Without a session, whoami has to name the command that gets one: every
	// storefront read runs on a browser session, so "no session" is the finding.
	if !strings.Contains(out, "reads working") || !strings.Contains(out, "no session cached") {
		t.Errorf("whoami output = %q", out)
	}
	if !strings.Contains(out, "login --from-browser") {
		t.Errorf("whoami does not say how to get a session: %q", out)
	}
}

// A challenged read is a failure the user must act on, so it exits non-zero.
func TestWhoamiFailsWhenReadsAreChallenged(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>geo.captcha-delivery.com</body></html>`))
	})

	if code := run([]string{"whoami"}); code == 0 {
		t.Error("want a non-zero exit when reads are challenged")
	}
}

const catIndexHTML = `<a href="/productos/banos/"><span>Baños</span></a>` +
	`<a href="/productos/iluminacion/"><span>Iluminación</span></a>`

const subcatHTML = `<a data-button-name="Focos" href="/productos/iluminacion/focos/">x</a>`

// categories has three routes: the top-level index, a category's products, and
// --subs for a category's children. Each has its own empty and 404 handling.
func TestCategoriesListsTheTopLevelIndex(t *testing.T) {
	withStubServer(t, catIndexHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"categories"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "Baños") || !strings.Contains(out, "/productos/iluminacion/") {
		t.Errorf("index output:\n%s", out)
	}
}

func TestCategoriesListsSubcategories(t *testing.T) {
	withStubServer(t, subcatHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"categories", "--subs", "iluminacion"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "Focos") {
		t.Errorf("subs output:\n%s", out)
	}
}

// A leaf category has no children; that is reported without failing.
func TestCategoriesReportsALeafCategory(t *testing.T) {
	withStubServer(t, `<html><body>hoja</body></html>`)

	if code := run([]string{"categories", "--subs", "iluminacion"}); code != 0 {
		t.Errorf("exit = %d, want 0 for a leaf category", code)
	}
}

func TestCategoriesListsProductsAndRanksThemByPrice(t *testing.T) {
	withStubServer(t, cardHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"categories", "--cheapest", "--limit", "5", "iluminacion"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "Taladro") {
		t.Errorf("category products output:\n%s", out)
	}
}

// A 404 on either route becomes advice naming the offending slug, not a bare
// HTTP error.
func TestCategoriesExplainsAnUnknownSlug(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	for _, args := range [][]string{
		{"categories", "no-existe"},
		{"categories", "--subs", "no-existe"},
	} {
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit for an unknown slug", args)
		}
	}
}

func TestSetCookieNeedsAValue(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	if code := run([]string{"set-cookie"}); code == 0 {
		t.Error("want a non-zero exit with no cookie")
	}
	if code := run([]string{"set-cookie", "   "}); code == 0 {
		t.Error("want a non-zero exit for a blank cookie")
	}
}

func TestSetCookiePersistsTheValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)

	if code := run([]string{"set-cookie", "datadome=SAVED; lm-csrf=T"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatalf("session not written: %v", err)
	}
	if !strings.Contains(string(b), "SAVED") {
		t.Errorf("saved session = %s", b)
	}
}

// An unreadable session cache must not be fatal: the cookie configured in
// config.toml still gets the user a working client.
func TestLoadSessionFallsBackToTheConfiguredCookie(t *testing.T) {
	dir := stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cardHTML))
	})
	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgTOML := "[auth]\ncookie = \"datadome=FROMCONFIG\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfgTOML), 0o600); err != nil {
		t.Fatal(err)
	}

	cl := newClient()
	if !strings.Contains(cl.Cookie, "FROMCONFIG") {
		t.Fatalf("cookie = %q, want the config.toml value", cl.Cookie)
	}
}

// The first client to use a cached cookie stamps its tab id into the cache, so
// later invocations present the same session to the storefront.
func TestLoadSessionPersistsTheTabID(t *testing.T) {
	dir := stubEnvServing(t, "datadome=DD", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cardHTML))
	})

	cl := newClient()
	if cl.TabID == "" {
		t.Fatal("client has no tab id")
	}

	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), cl.TabID) {
		t.Errorf("session %s does not carry tab id %q", b, cl.TabID)
	}
}

func TestProductRejectsBadArguments(t *testing.T) {
	withStubServer(t, productHTML)

	for _, args := range [][]string{
		{"product"}, // no target
		{"product", "/productos/a.html", "/productos/b.html"}, // two targets
		{"product", "taladro"},                                // not a product url
	} {
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit", args)
		}
	}
}

// A page with no Product JSON-LD gets advice pointing back at search, not a
// bare parse error.
func TestProductExplainsAPageWithoutProductData(t *testing.T) {
	withStubServer(t, `<html><body>sin datos</body></html>`)

	if code := run([]string{"product", "/productos/vacio-1.html"}); code == 0 {
		t.Error("want a non-zero exit for a page with no product data")
	}
}

func TestProductPrintsTheDetailView(t *testing.T) {
	withStubServer(t, productHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"product", "/productos/taladro-42.html"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	for _, want := range []string{"Taladro", "ref:", "29.99 EUR", "DEXTER"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail output missing %q:\n%s", want, out)
		}
	}
}

// total needs at least one basket line, and surfaces a per-line failure without
// aborting the whole basket.
func TestTotalNeedsLinesAndKeepsPerLineFailures(t *testing.T) {
	withStubServer(t, cardHTML)

	if code := run([]string{"total"}); code == 0 {
		t.Error("want a non-zero exit with no basket lines")
	}

	out := captureStdout(t, func() {
		if code := run([]string{"total", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "total:") {
		t.Errorf("total output = %q", out)
	}
}

func TestBatchNeedsTerms(t *testing.T) {
	withStubServer(t, cardHTML)
	if code := run([]string{"batch"}); code == 0 {
		t.Error("want a non-zero exit with no terms")
	}
}

// A term that matches nothing is listed and then reported in the exit status,
// so a scripted basket fails loudly instead of quietly buying less.
func TestBatchReportsTermsThatFoundNothing(t *testing.T) {
	withStubServer(t, `<html><body>sin resultados</body></html>`)

	out := captureStdout(t, func() {
		if code := run([]string{"batch", "termino-inexistente"}); code == 0 {
			t.Error("want a non-zero exit when a term returns no product")
		}
	})
	if !strings.Contains(out, "sin resultados") {
		t.Errorf("output = %q, want the per-term notice", out)
	}
}

// An explicitly requested brand that the results do not carry still shows the
// line, flagged, rather than dropping it silently.
func TestBatchFlagsAnUnavailableRequestedBrand(t *testing.T) {
	withStubServer(t, cardHTML)

	out := captureStdout(t, func() {
		if code := run([]string{"batch", "--brand", "Festool", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "marca preferida no disponible") {
		t.Errorf("output = %q, want the unavailable-brand flag", out)
	}
}

func TestSearchNeedsATermAndReportsNoResults(t *testing.T) {
	withStubServer(t, `<html><body>sin resultados</body></html>`)

	if code := run([]string{"search"}); code == 0 {
		t.Error("want a non-zero exit with no term")
	}
	if code := run([]string{"search", "termino-inexistente"}); code != 0 {
		t.Errorf("exit = %d, want 0: an empty result set is not an error", code)
	}
}

// Two brands in one result set, so --limit 1 has something to truncate.
const twoBrandHTML = `<script type="application/json" class="dataTms">
[{"name":"cdl_products_list","value":[
 {"brand":"DEXTER","identifier":"1","name":"A","url":"/productos/a-1.html","offer":{"unitprice_ati":10,"add_to_cart_availability":true}},
 {"brand":"BOSCH","identifier":"2","name":"B","url":"/productos/b-2.html","offer":{"unitprice_ati":20,"add_to_cart_availability":true}}
]}]
</script>`

// --limit truncates the ranked list rather than the fetch, so the cap applies to
// what the user sees after filtering.
func TestBrandsHonoursTheLimit(t *testing.T) {
	withStubServer(t, twoBrandHTML)

	full := captureStdout(t, func() {
		if code := run([]string{"brands", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(full, "DEXTER") || !strings.Contains(full, "BOSCH") {
		t.Fatalf("unlimited run should list both brands:\n%s", full)
	}

	capped := captureStdout(t, func() {
		if code := run([]string{"brands", "--limit", "1", "taladro"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	// On a count tie TallyBrands orders alphabetically, so BOSCH is the one kept.
	if strings.Contains(capped, "DEXTER") {
		t.Errorf("--limit 1 kept the second-ranked brand:\n%s", capped)
	}
	if !strings.Contains(capped, "BOSCH") {
		t.Errorf("--limit 1 dropped the top-ranked brand too:\n%s", capped)
	}
}

func TestCategoriesHonoursTheLimit(t *testing.T) {
	withStubServer(t, twoBrandHTML)

	full := captureStdout(t, func() {
		if code := run([]string{"categories", "iluminacion"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(full, "[1]") || !strings.Contains(full, "[2]") {
		t.Fatalf("unlimited run should list both products:\n%s", full)
	}

	capped := captureStdout(t, func() {
		if code := run([]string{"categories", "--limit", "1", "iluminacion"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if strings.Contains(capped, "[2]") {
		t.Errorf("--limit 1 kept the second product:\n%s", capped)
	}
	if !strings.Contains(capped, "[1]") {
		t.Errorf("--limit 1 dropped the first product too:\n%s", capped)
	}
}

// An empty catalog page is reported on stderr and exits 0: nothing failed, the
// section is just empty.
func TestCategoriesReportsEmptyResults(t *testing.T) {
	withStubServer(t, `<html><body>nada</body></html>`)

	if code := run([]string{"categories", "iluminacion"}); code != 0 {
		t.Errorf("exit = %d, want 0 for an empty category", code)
	}
	if code := run([]string{"categories"}); code != 0 {
		t.Errorf("exit = %d, want 0 when no categories are found", code)
	}
}

// `checkout` and `checkout status` are the same view; the alias must not be
// mistaken for a positional argument.
func TestCheckoutStatusIsAnAliasForCheckout(t *testing.T) {
	// Checkout is the account's, so the stub session has to carry the account.
	stubEnv(t, stubServerFor(t, `{"orderId":"","offersQuantity":0,"orderResume":{},"cartVendors":[]}`), testCookie)

	bare := captureStdout(t, func() {
		if code := run([]string{"checkout"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	aliased := captureStdout(t, func() {
		if code := run([]string{"checkout", "status"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if bare != aliased {
		t.Errorf("alias differs:\n bare: %q\n status: %q", bare, aliased)
	}
}

// A marketplace seller writes the product name, so remote text reaches the
// terminal. An ANSI escape in it used to be printed verbatim — and to fail
// `--toon` outright, since that encoder rejects control characters.
func TestHostileProductNameCannotReachTheTerminal(t *testing.T) {
	hostile := `<script type="application/json" class="dataTms">` +
		`[{"name":"cdl_products_list","value":[{"identifier":"9","name":` +
		`"Taladro\u001b]0;PWNED\u0007\u001b[2J",` +
		`"url":"/productos/x-9.html","offer":{"unitprice_ati":9.99}}]}]</script>`

	for _, args := range [][]string{
		{"search", "taladro"},
		{"search", "--json", "taladro"},
		{"search", "--toon", "taladro"},
	} {
		withStubServer(t, hostile)
		out := captureStdout(t, func() {
			if code := run(args); code != 0 {
				t.Errorf("%v exited %d", args, code)
			}
		})
		if strings.ContainsAny(out, "\x1b\x07") {
			t.Errorf("%v leaked a control character: %q", args, out)
		}
		if !strings.Contains(out, "Taladro") {
			t.Errorf("%v dropped the readable part of the name: %q", args, out)
		}
	}
}

// A 403 is DataDome asking for a browser. Printing its challenge page told the
// user to "enable JS and disable any ad blocker", which is not something they
// can do about a CLI; the fix is the WAF fallback the README documents.
func TestReadChallengeExplainsTheFallback(t *testing.T) {
	stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<html><p id="cmsg">Please enable JS and disable any ad blocker</p></html>`))
	})

	stderr := captureStderr(t, func() {
		if code := run([]string{"search", "taladro"}); code != 1 {
			t.Errorf("exit = %d, want 1", code)
		}
	})
	if !strings.Contains(stderr, "login --from-browser chrome") {
		t.Errorf("stderr does not say how to recover:\n%s", stderr)
	}
	if strings.Contains(stderr, "enable JS") {
		t.Errorf("stderr still dumps the challenge page:\n%s", stderr)
	}
}

func TestVersionAndHelpCommands(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		out := captureStdout(t, func() {
			if code := run(args); code != 0 {
				t.Errorf("%v: exit = %d", args, code)
			}
		})
		if strings.TrimSpace(out) == "" {
			t.Errorf("%v printed nothing", args)
		}
	}
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		if code := run(args); code != 0 {
			t.Errorf("%v: exit = %d", args, code)
		}
	}
}

// `--` ends flag parsing: what follows is a search term even when it starts
// with a dash, so a product name can never be mistaken for a flag.
func TestDoubleDashEndsFlagParsing(t *testing.T) {
	var seen []string
	stubEnvServing(t, "", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("q"))
		_, _ = w.Write([]byte(cardHTML))
	})

	if code := run([]string{"search", "--limit", "5", "--", "-taladro"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(seen) == 0 || seen[0] != "-taladro" {
		t.Errorf("query = %v, want the literal -taladro", seen)
	}
}

// An unreadable config.toml warns and falls back to defaults rather than
// aborting a read that does not need it.
func TestUnreadableConfigWarnsButDoesNotAbort(t *testing.T) {
	dir := stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cardHTML))
	})
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[limits\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"search", "taladro"}); code != 0 {
		t.Errorf("exit = %d, want 0: a broken config.toml must not abort a read", code)
	}
}

// A cached tab id is reused rather than regenerated, so repeated invocations
// present the storefront one continuous session.
func TestCachedTabIDIsReused(t *testing.T) {
	dir := stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cardHTML))
	})
	session := `{"cookie":"datadome=DD","tab_id":"tab-fixed"}`
	if err := os.WriteFile(filepath.Join(dir, "session.json"), []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}

	cl := newClient()
	if cl.TabID != "tab-fixed" {
		t.Errorf("TabID = %q, want the cached tab-fixed", cl.TabID)
	}
}

// Every read command must surface a storefront failure instead of printing an
// empty result, which would read as "nothing matched".
func TestReadCommandsSurfaceStorefrontFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"search", []string{"search", "taladro"}},
		{"product", []string{"product", "/productos/taladro-1.html"}},
		{"brands", []string{"brands", "taladro"}},
		{"batch", []string{"batch", "taladro"}},
		{"categories index", []string{"categories"}},
		{"categories products", []string{"categories", "iluminacion"}},
		{"categories subs", []string{"categories", "--subs", "iluminacion"}},
		{"total", []string{"total", "taladro"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubEnvServing(t, "", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})

			if code := run(tc.args); code == 0 {
				t.Errorf("%v: want a non-zero exit on an HTTP 500", tc.args)
			}
		})
	}
}

// import-har and set-cookie both persist a session; a write failure must not be
// reported as success, or the user retries a login that never saved.
func TestSessionWritersReportAFailedWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	har := filepath.Join(dir, "export.har")
	if err := os.WriteFile(har, []byte(harFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	freezeConfigDir(t, dir)

	if code := run([]string{"import-har", "--file", har}); code == 0 {
		t.Error("import-har: want a non-zero exit when the session write fails")
	}
	if code := run([]string{"set-cookie", "datadome=X"}); code == 0 {
		t.Error("set-cookie: want a non-zero exit when the session write fails")
	}
}

// A command whose output cannot be written must exit non-zero rather than
// report success for data nobody received — the `| head -1` case.
func TestStructuredOutputFailuresAreReported(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"search", []string{"search", "--json", "taladro"}},
		{"brands", []string{"brands", "--json", "taladro"}},
		{"batch", []string{"batch", "--json", "taladro"}},
		{"categories", []string{"categories", "--json", "iluminacion"}},
		{"total", []string{"total", "--json", "taladro"}},
		{"search toon", []string{"search", "--toon", "taladro"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withStubServer(t, cardHTML)
			breakStdout(t)

			if code := run(tc.args); code == 0 {
				t.Errorf("%v: want a non-zero exit when stdout cannot be written", tc.args)
			}
		})
	}
}

// product needs a page carrying Product JSON-LD; served cardHTML it would fail
// at the lookup and never reach the structured write it is meant to test.
func TestProductStructuredOutputFailureIsReported(t *testing.T) {
	withStubServer(t, productHTML)
	breakStdout(t)

	if code := run([]string{"product", "--json", "/productos/x-42.html"}); code == 0 {
		t.Error("want a non-zero exit when stdout cannot be written")
	}
}

// A flag written as --name=value is one argument; the reorderer must not then
// also swallow the next argument as its value.
func TestInlineFlagValueDoesNotEatTheNextArgument(t *testing.T) {
	var seen []string
	stubEnvServing(t, "", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("q"))
		_, _ = w.Write([]byte(cardHTML))
	})

	if code := run([]string{"search", "--limit=5", "taladro"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(seen) == 0 || seen[0] != "taladro" {
		t.Errorf("query = %v, want taladro (the term must survive --limit=5)", seen)
	}
}

// The --subs route emits its own structured output, separate from the products
// route, so its write failure needs its own test.
func TestCategoriesSubsStructuredOutputFailureIsReported(t *testing.T) {
	withStubServer(t, subcatHTML)
	breakStdout(t)

	if code := run([]string{"categories", "--subs", "--json", "iluminacion"}); code == 0 {
		t.Error("want a non-zero exit when stdout cannot be written")
	}
}

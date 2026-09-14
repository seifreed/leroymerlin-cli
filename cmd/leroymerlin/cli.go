package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	toon "github.com/toon-format/toon-go"

	"github.com/seifreed/leroymerlin-cli/internal/client"
	"github.com/seifreed/leroymerlin-cli/internal/config"
)

// stderrLogf routes diagnostics (client retries/fallbacks, CLI-side warnings) to
// stderr, prefixed, so they never mix with --json data on stdout. A var so tests
// can capture what would be logged.
var stderrLogf = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "leroymerlin: "+format+"\n", args...)
}

const sessionFile = "session.json"

// common flags shared by every subcommand.
type common struct {
	jsonOut bool
	toon    bool
}

func newCommonFlags(name string) (*flag.FlagSet, *common) {
	fs := newFlagSet(name)
	return fs, addCommon(fs)
}

// newFlagSet builds a bare flag set (no common flags) for commands that take none.
func newFlagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ExitOnError)
}

func addCommon(fs *flag.FlagSet) *common {
	c := &common{}
	fs.BoolVar(&c.jsonOut, "json", false, "emit raw JSON to stdout")
	fs.BoolVar(&c.toon, "toon", false, "emit TOON (token-oriented) to stdout")
	return c
}

// newClient builds a client, resolving language and loading the cached session.
func newClient() *client.Client {
	cl := client.New()
	cl.Logf = stderrLogf
	if u := os.Getenv("LEROYMERLIN_BASE_URL"); u != "" {
		cl.BaseURL = u
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		stderrLogf("config.toml could not be read (%v) — using defaults", err)
	}
	loadSession(cl, cfg)
	return cl
}

// loadSession applies the persisted cookie and tab identity to a client. The
// CLI owns this filesystem boundary; the HTTP client only carries session data.
func loadSession(cl *client.Client, cfg config.Config) {
	var s client.Session
	if err := config.Load(sessionFile, &s); err != nil && !os.IsNotExist(err) {
		stderrLogf("cookie cache could not be read (%v) — re-seed with `leroymerlin set-cookie`", err)
	}
	if s.Cookie == "" {
		s.Cookie = cfg.Auth.Cookie
	}
	if s.Cookie == "" {
		return
	}
	cl.Cookie = s.Cookie
	if s.TabID != "" {
		cl.TabID = s.TabID
		return
	}
	s.TabID = cl.TabID
	if err := saveSession(s); err != nil {
		stderrLogf("cookie cache could not be updated (%v)", err)
	}
}

// warnWithoutSession tells the user on stderr why a cart-backed read came back
// empty: the cart belongs to a session, and there is none. The read itself still
// answers, which is what makes the warning worth printing.
func warnWithoutSession(cl *client.Client) {
	if cl.Cookie == "" {
		stderrLogf("no cookie cached — the cart is tied to your browser session; run `leroymerlin login --from-browser chrome` first")
		return
	}
	warnGuestCart(cl)
}

// warnGuestCart names the cart the user is actually operating on. A cookie
// lifted from a signed-out browser works, so nothing fails — the items just
// land in a guest cart that the account's own cart page never shows.
func warnGuestCart(cl *client.Client) {
	if !client.CookieIsSignedIn(cl.Cookie) {
		stderrLogf("guest cart: the cached cookie carries no signed-in account, so these items will not appear in your account's cart — sign in at www.leroymerlin.es, then run `leroymerlin login --from-browser chrome` again")
	}
}

// requireSession builds a client for an operation that cannot work anonymously,
// naming it in the one hint that tells the user how to obtain a session.
func requireSession(operation string) (*client.Client, error) {
	cl := newClient()
	if cl.Cookie == "" {
		return nil, fmt.Errorf("%s need your browser session — run `leroymerlin login --from-browser chrome` (or import-har / set-cookie) first", operation)
	}
	warnGuestCart(cl)
	return cl, nil
}

func saveSession(s client.Session) error {
	return config.Save(sessionFile, s)
}

// persistClientSession writes the client's (possibly rotated) session back to
// the cache, reporting a failure the way every command that calls it reported it.
func persistClientSession(cl *client.Client) error {
	if err := saveSession(cl.CurrentSession()); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// parseFlags parses args into fs, letting flags appear among positionals.
func parseFlags(fs *flag.FlagSet, args []string) {
	_ = fs.Parse(reorderArgs(fs, args))
}

// reorderArgs lets flags appear anywhere among positional args. The stdlib flag
// parser stops at the first positional; this hoists flags (and their values)
// ahead of a `--` terminator so a normal fs.Parse sees them all.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && (a[1] < '0' || a[1] > '9') && a[1] != '.' {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.IndexByte(name, '=') >= 0 {
				continue
			}
			if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	out := make([]string, 0, len(flags)+1+len(positional))
	out = append(out, flags...)
	out = append(out, "--")
	return append(out, positional...)
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// toonEncode renders v as TOON, routing through JSON first so field names match
// --json exactly. Map keys are sorted, so output is deterministic.
func toonEncode(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var generic any
	_ = json.Unmarshal(raw, &generic)
	return toon.MarshalString(generic)
}

func emitTOON(v any) error {
	s, err := toonEncode(v)
	if err != nil || s == "" {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, s)
	return err
}

// emitStructured emits v in the selected structured format (--toon or --json) and
// reports whether it did, so callers fall through to the human-readable view only
// when neither flag is set.
func emitStructured(cf *common, v any) (emitted bool, err error) {
	switch {
	case cf.toon:
		return true, emitTOON(v)
	case cf.jsonOut:
		return true, emitJSON(v)
	}
	return false, nil
}

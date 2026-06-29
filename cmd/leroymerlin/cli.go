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

// common flags shared by every subcommand.
type common struct {
	lang    string
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
	// Empty default so newClient can tell "user passed --lang" from "unset" and
	// fall back to config [defaults] before the built-in es.
	fs.StringVar(&c.lang, "lang", "", "language: es (default) or ca")
	fs.BoolVar(&c.jsonOut, "json", false, "emit raw JSON to stdout")
	fs.BoolVar(&c.toon, "toon", false, "emit TOON (token-oriented) to stdout")
	return c
}

// newClient builds a client, resolving lang by precedence (explicit --lang flag >
// config.toml [defaults] > built-in es). Any cached/configured cookie is loaded
// so a challenged read can carry the user's DataDome clearance.
func newClient(c *common) *client.Client {
	cl := client.New()
	cl.Logf = stderrLogf
	if u := os.Getenv("LEROYMERLIN_BASE_URL"); u != "" {
		cl.BaseURL = u
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		stderrLogf("config.toml could not be read (%v) — using defaults", err)
	}
	if v := firstNonEmpty(c.lang, cfg.Defaults.Lang); v != "" {
		cl.Lang = v
	}
	cl.LoadAuth()
	return cl
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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

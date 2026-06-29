// Command leroymerlin is an unofficial, agent-friendly CLI for leroymerlin.es.
//
// The site has no public API and server-renders its pages behind DataDome bot
// protection. This CLI presents Chrome's TLS fingerprint (uTLS) so DataDome
// keeps it off the JS-challenge path, fetches the same SSR HTML the browser
// gets, and lifts the structured data the page already embeds: full-text
// product search and product detail (price, brand, rating, availability).
// Every command supports --json (data to stdout, logs to stderr) for agents.
package main

import (
	"fmt"
	"os"
)

// Build metadata, injected at release time via -ldflags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() { os.Exit(run(os.Args[1:])) }

// run dispatches a command and returns the process exit code. Split from main so
// it is testable without os.Exit.
func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	var err error
	switch args[0] {
	case "search":
		err = cmdSearch(args[1:])
	case "batch":
		err = cmdBatch(args[1:])
	case "total":
		err = cmdTotal(args[1:])
	case "product":
		err = cmdProduct(args[1:])
	case "login":
		err = cmdLogin(args[1:])
	case "whoami":
		err = cmdWhoami(args[1:])
	case "import-har":
		err = cmdImportHar(args[1:])
	case "set-cookie":
		err = cmdSetCookie(args[1:])
	case "version", "--version", "-v":
		fmt.Println(versionString())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func versionString() string {
	if commit == "" {
		return version
	}
	if date != "" {
		return fmt.Sprintf("%s (%s, %s)", version, commit, date)
	}
	return fmt.Sprintf("%s (%s)", version, commit)
}

func usage() {
	fmt.Fprint(os.Stderr, `leroymerlin — unofficial CLI for leroymerlin.es

USAGE:
  leroymerlin <command> [flags]

READ COMMANDS (anonymous):
  search <term...>        full-text product search
                          --limit N    cap results
                          --cheapest   rank by price (low → high)
                          --in-stock   keep only buyable items
  batch [-f file]         resolve many terms at once — cheapest in-stock hit per
                          term (one request each). Terms positional or one/line.
  total [-f file]         deterministic basket total from '<url|term> [qty]'
                          lines — summed in integer cents. URLs price from the
                          product page, terms from their cheapest hit.
  product <url|path>      product detail (price, brand, rating, availability).
                          Pass the url field from a search result.

WAF FALLBACK (only if anonymous reads draw a DataDome challenge):
  login --from-browser b  lift the cookie from a browser's cookie store
                          (chrome|chromium|firefox|safari|edge|brave; empty = any) — easiest
  import-har --file f     lift the cookie from a DevTools HAR ("Save all as HAR
                          with sensitive data"). --file - reads stdin.
  set-cookie '<cookie>'   seed a raw Cookie header from your browser (DevTools →
                          copy the request's Cookie header). --stdin supported.
  whoami                  check whether reads work (and if a cookie is cached)

COMMON FLAGS (may go anywhere after the command):
  --lang es               language: es (default) or ca
  --json                  emit raw JSON (data→stdout, logs→stderr)
  --toon                  emit TOON instead of JSON (fewer tokens; for agents)

ENV:
  LEROYMERLIN_BASE_URL    override the host (debugging proxy, mock, staging)
  LEROYMERLIN_CONFIG_DIR  override ~/.leroymerlin

  version | help
`)
}

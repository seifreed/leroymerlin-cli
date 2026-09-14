package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cliBin is the compiled CLI, built once for the package. Empty when the Go
// toolchain is unavailable, in which case the subprocess tests skip.
var cliBin string

// TestMain builds the binary once so the per-test cost is just fork+exec. When
// LEROYMERLIN_TEST_COVERDIR is set (the cover target does it) the child is built
// instrumented and writes its profile there, which is the only way main() gets
// coverage credit — a test binary never executes it.
func TestMain(m *testing.M) {
	code := func() int {
		if _, err := exec.LookPath("go"); err != nil {
			return m.Run()
		}
		dir, err := os.MkdirTemp("", "lmcli-bin-*")
		if err != nil {
			return m.Run()
		}
		defer func() { _ = os.RemoveAll(dir) }()

		bin := filepath.Join(dir, "leroymerlin")
		args := []string{"build"}
		if os.Getenv("LEROYMERLIN_TEST_COVERDIR") != "" {
			args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
		}
		args = append(args, "-o", bin, ".")
		if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
			panic("building the CLI for subprocess tests: " + err.Error() + "\n" + string(out))
		}
		cliBin = bin
		return m.Run()
	}()
	os.Exit(code)
}

// runCLI executes the built binary and returns its streams and exit code.
// t.Setenv does not reach a subprocess, so the config dir is passed explicitly —
// without it the child would read the developer's real ~/.leroymerlin.
func runCLI(t *testing.T, stdin string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	if cliBin == "" {
		t.Skip("go toolchain unavailable; cannot build the CLI")
	}
	cmd := exec.Command(cliBin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = append(os.Environ(), "LEROYMERLIN_CONFIG_DIR="+t.TempDir())
	if d := os.Getenv("LEROYMERLIN_TEST_COVERDIR"); d != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+d)
	}
	cmd.Env = append(cmd.Env, env...)

	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running the CLI: %v", err)
	}
	return out.String(), errb.String(), code
}

// Invoked with nothing to do, the CLI prints help and exits 2 — and the help
// goes to stderr, so `leroymerlin | something` is not fed a usage screen.
func TestBinaryExitsTwoOnNoArgs(t *testing.T) {
	stdout, stderr, code := runCLI(t, "", nil)

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "USAGE") {
		t.Errorf("stderr = %q, want the usage screen", stderr)
	}
}

func TestBinaryExitsTwoOnAnUnknownCommand(t *testing.T) {
	_, stderr, code := runCLI(t, "", nil, "frobnicate")

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr = %q", stderr)
	}
}

// flag.ExitOnError means a malformed flag calls os.Exit from inside the handler,
// bypassing run() entirely. A subprocess is the only way to observe it.
func TestBinaryExitsTwoOnAMalformedFlag(t *testing.T) {
	stdout, _, code := runCLI(t, "", nil, "search", "--limit=notanumber", "taladro")

	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
}

// The contract the whole --json design rests on: data on stdout, diagnostics on
// stderr, so a pipe receives parseable JSON and nothing else.
// --lang was removed: leroymerlin.es serves Spanish only and ignored the header
// entirely. Only the real binary can show this, since an unknown flag exits the
// process rather than returning to the caller.
func TestBinaryRejectsTheRemovedLangFlag(t *testing.T) {
	_, stderr, code := runCLI(t, "", nil, "search", "--lang", "ca", "taladro")
	if code != 2 {
		t.Errorf("exit = %d, want 2 for an unknown flag", code)
	}
	if !strings.Contains(stderr, "not defined") {
		t.Errorf("stderr = %q, want the unknown-flag complaint", stderr)
	}
}

func TestBinaryKeepsJSONOnStdoutAndDiagnosticsOnStderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(cardHTML))
	}))
	defer srv.Close()

	stdout, _, code := runCLI(t, "", []string{"LEROYMERLIN_BASE_URL=" + srv.URL}, "search", "--json", "taladro")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var parsed any
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("stdout is not clean JSON (%v):\n%s", err, stdout)
	}
}

func TestBinaryReportsItsVersion(t *testing.T) {
	stdout, _, code := runCLI(t, "", nil, "version")

	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) != "dev" {
		t.Errorf("version = %q, want dev for an un-stamped build", stdout)
	}
}

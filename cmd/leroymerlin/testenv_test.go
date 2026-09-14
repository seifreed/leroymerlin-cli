package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// stubEnv points the CLI at srvURL with a config dir isolated from the real
// ~/.leroymerlin, and returns that dir so a test can inspect what the command
// persisted. A non-empty cookie is seeded as the cached session, which is what
// commands that write to the cart require; pass "" to run anonymous.
func stubEnv(t *testing.T, srvURL, cookie string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_BASE_URL", srvURL)
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	if cookie != "" {
		session := []byte(`{"cookie":"` + cookie + `"}`)
		if err := os.WriteFile(filepath.Join(dir, "session.json"), session, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const testCookie = `datadome=DD; lm-csrf=T`

// stubEnvServing points the CLI at a stub storefront serving h, with a config dir
// isolated from the real ~/.leroymerlin seeded with cookie, and returns that dir.
func stubEnvServing(t *testing.T, cookie string, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return stubEnv(t, srv.URL, cookie)
}

// freezeConfigDir makes the config dir unwritable so the session write at the end
// of a command fails while the read at the start still succeeds. That is the only
// way to reach the "save … session" paths without a seam in production code.
// Skips where the filesystem does not enforce the mode (running as root).
func freezeConfigDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("cannot make %s read-only: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	probe := filepath.Join(dir, "write-probe")
	if err := os.WriteFile(probe, []byte("x"), 0o600); err == nil {
		_ = os.Remove(probe)
		t.Skip("filesystem does not enforce directory permissions here")
	}
}

// breakStdout points os.Stdout at a read-only file so every write fails, which
// is what a command sees when its output is piped into something that exits
// early (`leroymerlin search --json | head -1`). Restored on cleanup.
func breakStdout(t *testing.T) {
	t.Helper()
	ro, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = ro
	t.Cleanup(func() {
		os.Stdout = orig
		_ = ro.Close()
	})
}

// withStdin points os.Stdin at a file holding content, so the commands that read
// the global can be driven in process. A real file rather than a pipe: no
// goroutine, and no way to block on an unclosed writer.
//
// These helpers mutate a process global, so tests using them must not run in
// parallel. Nothing in this package does today.
func withStdin(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig; _ = f.Close() })
}

// breakStdin points os.Stdin at a write-only file so every Read fails, which is
// what a command sees when its input descriptor is unusable.
func breakStdin(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unreadable")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig; _ = f.Close() })
}

// stubServerFor serves one body for every path and returns its URL, for tests
// that need to seed the config dir themselves rather than take withStubServer's
// anonymous default.
func stubServerFor(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

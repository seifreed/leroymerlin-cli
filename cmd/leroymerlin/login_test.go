package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/client"
)

// stubBrowserCookies replaces the browser-store scan for one test.
func stubBrowserCookies(t *testing.T, s client.Session, err error) {
	t.Helper()
	orig := cookiesFromBrowser
	cookiesFromBrowser = func(string) (client.Session, error) { return s, err }
	t.Cleanup(func() { cookiesFromBrowser = orig })
}

// The whole point of `login --from-browser` is that the lifted cookie is
// persisted and then proven to work, so the success path must do both.
func TestLoginPersistsTheBrowserCookieAndProbesReads(t *testing.T) {
	srv := stubServerFor(t, cardHTML)
	dir := stubEnv(t, srv, "")
	stubBrowserCookies(t, client.Session{Cookie: testCookie}, nil)

	out := captureStdout(t, func() {
		if code := run([]string{"login", "--from-browser", "chrome"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "reads working") {
		t.Errorf("output = %q, want the success line", out)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil || !strings.Contains(string(saved), "datadome=DD") {
		t.Fatalf("session = %s, %v", saved, err)
	}
}

func TestLoginSurfacesAFailedScan(t *testing.T) {
	srv := stubServerFor(t, cardHTML)
	stubEnv(t, srv, "")
	stubBrowserCookies(t, client.Session{}, errors.New("no cookie found"))

	if code := run([]string{"login", "--from-browser", "chrome"}); code == 0 {
		t.Error("want a non-zero exit when the scan fails")
	}
}

func TestLoginReportsAFailedSessionWrite(t *testing.T) {
	srv := stubServerFor(t, cardHTML)
	dir := stubEnv(t, srv, "")
	stubBrowserCookies(t, client.Session{Cookie: testCookie}, nil)
	freezeConfigDir(t, dir)

	if code := run([]string{"login", "--from-browser", "chrome"}); code == 0 {
		t.Error("want a non-zero exit when the session cannot be saved")
	}
}

// A cookie that saves but does not load back means the cache is broken; saying
// "logged in" there would send the user in circles.
func TestLoginReportsACookieThatDoesNotLoadBack(t *testing.T) {
	srv := stubServerFor(t, cardHTML)
	stubEnv(t, srv, "")
	stubBrowserCookies(t, client.Session{Cookie: ""}, nil)

	if code := run([]string{"login", "--from-browser", "chrome"}); code == 0 {
		t.Error("want a non-zero exit when the cookie does not load back")
	}
}

// The cookie can be lifted successfully and still not satisfy DataDome, which
// is a different failure from not finding one.
func TestLoginReportsStillChallengedReads(t *testing.T) {
	srv := stubServerFor(t, `<html><body>geo.captcha-delivery.com</body></html>`)
	stubEnv(t, srv, "")
	stubBrowserCookies(t, client.Session{Cookie: testCookie}, nil)

	if code := run([]string{"login", "--from-browser", "chrome"}); code == 0 {
		t.Error("want a non-zero exit when reads are still challenged")
	}
}

func TestLoginEmitsTheStructuredProbe(t *testing.T) {
	srv := stubServerFor(t, cardHTML)
	stubEnv(t, srv, "")
	stubBrowserCookies(t, client.Session{Cookie: testCookie}, nil)

	out := captureStdout(t, func() {
		if code := run([]string{"login", "--from-browser", "chrome", "--json"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"reads_ok"`) {
		t.Errorf("json = %q, want the probe result", out)
	}
}

func TestWhoamiReportsACachedCookie(t *testing.T) {
	srv := stubServerFor(t, cardHTML)
	stubEnv(t, srv, testCookie)

	out := captureStdout(t, func() {
		if code := run([]string{"whoami"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "cookie cached") {
		t.Errorf("output = %q, want the cached-cookie state", out)
	}
}

// A storefront failure during the probe is not "challenged reads" — it is a
// broken request, and the wrapper says so rather than advising a re-login.
func TestProbeReadsWrapsAFailedCheck(t *testing.T) {
	stubEnvServing(t, testCookie, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if code := run([]string{"whoami"}); code == 0 {
		t.Error("want a non-zero exit when the probe request fails")
	}
}

// Reads working is not the whole answer: a cookie that lost the account still
// reads, and whoami is where the user checks before a cart command refuses.
func TestWhoamiReportsTheSignedInState(t *testing.T) {
	for _, tc := range []struct {
		name, cookie, want string
	}{
		{"signed in", testCookie, "signed-in cookie cached"},
		{"guest", guestCookie, "signed out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubEnv(t, stubServerFor(t, cardHTML), tc.cookie)
			out := captureStdout(t, func() {
				if code := run([]string{"whoami"}); code != 0 {
					t.Fatalf("exit = %d", code)
				}
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("whoami = %q, want %q", out, tc.want)
			}
		})
	}
}

func TestWhoamiJSONCarriesSignedIn(t *testing.T) {
	stubEnv(t, stubServerFor(t, cardHTML), guestCookie)
	out := captureStdout(t, func() {
		if code := run([]string{"whoami", "--json"}); code != 0 {
			t.Fatalf("exit = %d", code)
		}
	})
	if !strings.Contains(out, `"signed_in": false`) {
		t.Errorf("whoami --json = %q, want signed_in false", out)
	}
}

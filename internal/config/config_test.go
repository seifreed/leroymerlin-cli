package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	want := struct {
		Cookie string `json:"cookie"`
	}{Cookie: "datadome=abc"}

	if err := Save("session.json", want); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Cookie string `json:"cookie"`
	}
	if err := Load("session.json", &got); err != nil {
		t.Fatal(err)
	}
	if got.Cookie != want.Cookie {
		t.Fatalf("cookie = %q, want %q", got.Cookie, want.Cookie)
	}
	if mode := fileMode(t, filepath.Join(dir, "session.json")); mode != 0o600 {
		t.Fatalf("session mode = %o, want 600", mode)
	}
}

func TestLoadConfigMissingAndToml(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	if got, err := LoadConfig(); err != nil || len(got.Brands.Preferred) > 0 || len(got.Brands.Overrides) > 0 {
		t.Fatalf("missing config = %+v, err %v", got, err)
	}
	contents := "[limits]\nmax_eur = 25\n[brands]\npreferred = [\"Bosch\"]\n"
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil || got.Limits.MaxEUR != 25 || len(got.Brands.Preferred) != 1 {
		t.Fatalf("config = %+v, err %v", got, err)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestDirPrefersTheEnvironmentOverride(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", "/tmp/lm-config-override")
	if got := Dir(); got != "/tmp/lm-config-override" {
		t.Fatalf("Dir() = %q, want the override", got)
	}

	t.Setenv("LEROYMERLIN_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}
	if got, want := Dir(), filepath.Join(home, ".leroymerlin"); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

func TestLoadReportsMissingAndCorruptFiles(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	var v map[string]string
	if err := Load("absent.json", &v); err == nil {
		t.Error("want an error for a missing file")
	}

	if err := os.WriteFile(filepath.Join(Dir(), "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Load("broken.json", &v); err == nil {
		t.Error("want an error for malformed JSON")
	}
}

// Save is atomic (temp file + rename) and 0600; a leftover temp file would mean
// a crash could strand secrets in the config dir.
func TestSaveIsAtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)

	if err := Save("state.json", map[string]string{"cookie": "secret"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}

	var back map[string]string
	if err := Load("state.json", &back); err != nil || back["cookie"] != "secret" {
		t.Fatalf("round-trip = %v, %v", back, err)
	}
}

func TestSaveRejectsUnencodableValues(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
	if err := Save("bad.json", make(chan int)); err == nil {
		t.Fatal("want an error for an unencodable value")
	}
}

// When the config dir cannot exist — a plain file already occupies its path —
// Save must report it rather than lose the data silently.
func TestSaveReportsAnUnusableConfigDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEROYMERLIN_CONFIG_DIR", blocker)

	if err := Save("state.json", map[string]string{"a": "b"}); err == nil {
		t.Fatal("want an error when the config dir is a file")
	}
	if err := Load("state.json", &map[string]string{}); err == nil {
		t.Fatal("want an error loading from an unusable config dir")
	}
}

// A name carrying a path separator must not escape the config dir.
func TestSaveRejectsANameWithAPathSeparator(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	if err := Save("nested/state.json", map[string]string{"a": "b"}); err == nil {
		t.Fatal("want an error for a name containing a separator")
	}
}

// Without a discoverable home directory the config dir falls back to a relative
// path rather than an empty one, which would resolve to the filesystem root.
func TestDirFallsBackWhenHomeIsUnknown(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", "")
	t.Setenv("HOME", "")
	if runtime.GOOS == "windows" {
		t.Skip("home resolution differs on windows")
	}

	if got := Dir(); got == "" || filepath.IsAbs(got) && got == "/" {
		t.Fatalf("Dir() = %q, want a usable fallback", got)
	}
}

// A config.toml that cannot be parsed must surface, not silently yield defaults
// that quietly ignore the user's spending limit.
func TestLoadConfigReportsMalformedToml(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[limits\nmax_eur = "), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadConfig(); err == nil {
		t.Fatal("want an error for malformed config.toml")
	}
}

// An absent config.toml is the normal case and must not be an error.
func TestLoadConfigTreatsAnAbsentFileAsDefaults(t *testing.T) {
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("absent config.toml should yield defaults, got %v", err)
	}
	if cfg.Limits.MaxEUR != 0 || len(cfg.Brands.Preferred) != 0 {
		t.Errorf("cfg = %+v, want zero values", cfg)
	}
}

// A stat failure that is not "absent" must surface: silently using defaults
// would ignore a spending limit the user believes is set.
func TestLoadConfigSurfacesAnUnreadablePath(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEROYMERLIN_CONFIG_DIR", blocker)

	if _, err := LoadConfig(); err == nil {
		t.Fatal("want an error when the config path cannot be stat'd")
	}
}

// stubTemp is a temp file that fails where a real one cannot be made to.
type stubTemp struct {
	name               string
	writeErr, closeErr error
	closes             int
}

func (f *stubTemp) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *stubTemp) Name() string { return f.name }
func (f *stubTemp) Close() error { f.closes++; return f.closeErr }

// The atomic write has two failure points once the temp file exists. Neither may
// publish a half-written cache — this file holds the session cookie, and a
// truncated one would leave the CLI authenticating with garbage.
func TestSaveSurfacesTempFileWriteAndCloseFailures(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	orig := createTemp
	t.Cleanup(func() { createTemp = orig })
	target := filepath.Join(dir, "session.json")

	t.Run("a failed write closes the descriptor and reports the cause", func(t *testing.T) {
		wf := &stubTemp{name: filepath.Join(dir, "session.json.tmp-w"), writeErr: errors.New("no space left on device")}
		createTemp = func(string, string) (tempFile, error) { return wf, nil }

		err := Save("session.json", map[string]string{"cookie": "x"})
		if err == nil || !strings.Contains(err.Error(), "no space left") {
			t.Fatalf("Save = %v, want the write error", err)
		}
		if wf.closes != 1 {
			t.Errorf("descriptor closed %d times, want 1", wf.closes)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Error("a failed write must not publish the cache file")
		}
	})

	t.Run("a failed close does not publish either", func(t *testing.T) {
		cf := &stubTemp{name: filepath.Join(dir, "session.json.tmp-c"), closeErr: errors.New("flush failed")}
		createTemp = func(string, string) (tempFile, error) { return cf, nil }

		err := Save("session.json", map[string]string{"cookie": "x"})
		if err == nil || !strings.Contains(err.Error(), "flush failed") {
			t.Fatalf("Save = %v, want the close error", err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Error("a failed close means the bytes may never have landed; the rename must not happen")
		}
	})

	t.Run("a failed temp-file creation is reported", func(t *testing.T) {
		createTemp = func(string, string) (tempFile, error) { return nil, errors.New("too many open files") }

		if err := Save("session.json", map[string]string{"cookie": "x"}); err == nil {
			t.Fatal("want the creation failure surfaced")
		}
	})
}

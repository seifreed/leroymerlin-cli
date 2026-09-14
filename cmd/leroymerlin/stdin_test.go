package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `-f -` is how a shopping list gets piped in, so the stdin path carries the
// same comment-stripping and quantity parsing as a file.
func TestBatchReadsTermsFromStdin(t *testing.T) {
	withStubServer(t, cardHTML)
	withStdin(t, "taladro\n# una nota\n\nsierra\n")

	out := captureStdout(t, func() {
		if code := run([]string{"batch", "-f", "-"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	for _, want := range []string{"taladro", "sierra"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "una nota") {
		t.Errorf("comment leaked into the terms:\n%s", out)
	}
}

func TestTotalReadsTheBasketFromStdin(t *testing.T) {
	withStubServer(t, cardHTML)
	withStdin(t, "# lista\ntaladro 2\n")

	out := captureStdout(t, func() {
		if code := run([]string{"total", "-f", "-"}); code != 0 {
			t.Errorf("exit = %d", code)
		}
	})
	if !strings.Contains(out, "total:") {
		t.Errorf("output = %q, want a priced basket", out)
	}
}

// import-har from stdin is the documented way to pipe a DevTools export without
// it touching the disk.
func TestImportHarReadsTheExportFromStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	withStdin(t, harFixture)

	if code := run([]string{"import-har", "--file", "-"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil || !strings.Contains(string(saved), "FROMHAR") {
		t.Fatalf("session = %s, %v", saved, err)
	}
}

func TestSetCookieReadsTheCookieFromStdin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEROYMERLIN_CONFIG_DIR", dir)
	withStdin(t, "  datadome=PIPED; lm-csrf=T\n")

	if code := run([]string{"set-cookie", "--stdin"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "datadome=PIPED") {
		t.Errorf("session = %s, want the piped cookie", saved)
	}
	if strings.Contains(string(saved), "\\n") || strings.Contains(string(saved), `"  `) {
		t.Errorf("session = %s, want the cookie trimmed", saved)
	}
}

// An unreadable stdin must fail the command rather than proceed with nothing.
func TestStdinReadersReportAnUnreadableInput(t *testing.T) {
	for _, args := range [][]string{
		{"import-har", "--file", "-"},
		{"set-cookie", "--stdin"},
	} {
		t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())
		breakStdin(t)
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit when stdin cannot be read", args)
		}
	}
}

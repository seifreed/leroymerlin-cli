package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A -f file is the bulk entry point: comments and blank lines are dropped so a
// hand-maintained shopping list stays editable.
func TestCollectLinesStripsCommentsAndBlanks(t *testing.T) {
	path := writeTemp(t, "taladro\n\n# una nota\nsierra  # inline\n   \nmartillo\n")

	got, err := collectLines(path, nil)
	if err != nil {
		t.Fatalf("collectLines: %v", err)
	}
	want := []string{"taladro", "sierra", "martillo"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("lines = %v, want %v", got, want)
	}
}

func TestCollectLinesFallsBackToPositionalArgs(t *testing.T) {
	got, err := collectLines("", []string{"taladro", "sierra"})
	if err != nil || len(got) != 2 || got[0] != "taladro" {
		t.Fatalf("collectLines = %v, %v", got, err)
	}
}

func TestCollectLinesReportsAnUnreadableFile(t *testing.T) {
	if _, err := collectLines(filepath.Join(t.TempDir(), "absent.txt"), nil); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

func TestCollectBasketParsesQuantitiesAndDefaultsToOne(t *testing.T) {
	path := writeTemp(t, "# lista\nref-a 3\nref-b\nref-c 0.5\n")

	got, err := collectBasket(path, nil)
	if err != nil {
		t.Fatalf("collectBasket: %v", err)
	}
	want := []basketLine{{"ref-a", 3}, {"ref-b", 1}, {"ref-c", 0.5}}
	if len(got) != len(want) {
		t.Fatalf("lines = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A trailing non-number is part of the ref, not a bad quantity: refs are often
// multi-word search terms ("taladro percutor bosch").
func TestCollectBasketTreatsAMultiWordLineAsOneRef(t *testing.T) {
	path := writeTemp(t, "taladro percutor bosch\n")

	got, err := collectBasket(path, nil)
	if err != nil {
		t.Fatalf("collectBasket: %v", err)
	}
	if len(got) != 1 || got[0].ref != "taladro percutor bosch" || got[0].qty != 1 {
		t.Fatalf("line = %+v, want the whole line as the ref with qty 1", got)
	}
}

// The line number in the error is what makes a bad entry findable in a long file.
func TestCollectBasketReportsTheOffendingLineNumber(t *testing.T) {
	path := writeTemp(t, "ref-a 1\n# comentario\nref-b -5\n")

	_, err := collectBasket(path, nil)
	if err == nil {
		t.Fatal("want an error for the negative quantity")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %q, want it to point at line 3", err)
	}
}

func TestCollectBasketFromPositionalArgsDefaultsToQtyOne(t *testing.T) {
	got, err := collectBasket("", []string{"ref-a", "ref-b"})
	if err != nil || len(got) != 2 || got[0].qty != 1 || got[1].ref != "ref-b" {
		t.Fatalf("collectBasket = %+v, %v", got, err)
	}
}

// printCategoryList pads paths to a common width so the names line up; the
// padding is measured from the longest path, not a fixed guess.
func TestPrintCategoryListAlignsNamesOnTheLongestPath(t *testing.T) {
	out := captureStdout(t, func() {
		printCategoryList([]domain.Category{
			{Path: "/productos/banos/", Name: "Baños"},
			{Path: "/productos/iluminacion-mucho-mas-larga/", Name: "Iluminación"},
		})
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), out)
	}
	first, second := strings.Index(lines[0], "Baños"), strings.Index(lines[1], "Iluminación")
	if first != second {
		t.Errorf("names not aligned (col %d vs %d):\n%s", first, second, out)
	}
}

func TestPrintCategoryListPrintsNothingForNoCategories(t *testing.T) {
	if out := captureStdout(t, func() { printCategoryList(nil) }); out != "" {
		t.Errorf("empty list printed %q", out)
	}
}

// -f naming a file that cannot be read must fail the command outright: silently
// pricing an empty basket would report a total of zero as success.
func TestBulkCommandsRejectAnUnreadableInputFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.txt")
	t.Setenv("LEROYMERLIN_CONFIG_DIR", t.TempDir())

	for _, args := range [][]string{
		{"batch", "-f", missing},
		{"total", "-f", missing},
	} {
		if code := run(args); code == 0 {
			t.Errorf("%v: want a non-zero exit for an unreadable -f file", args)
		}
	}
}

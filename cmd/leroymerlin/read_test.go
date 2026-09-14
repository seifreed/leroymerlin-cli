package main

import (
	"strings"
	"testing"
)

func TestParseBasketLine(t *testing.T) {
	cases := []struct {
		in      string
		wantRef string
		wantQty float64
		wantErr bool
	}{
		{"martillo", "martillo", 1, false},
		{"martillo 2", "martillo", 2, false},
		{"cinta métrica 3", "cinta métrica", 3, false},               // term with a space + qty
		{"/productos/x-42.html 5", "/productos/x-42.html", 5, false}, // url + qty
		{"/productos/x-42.html", "/productos/x-42.html", 1, false},   // bare url
		{"taladro percutor", "taladro percutor", 1, false},           // trailing word, not a number
		{"martillo -1", "", 0, true},                                 // negative qty
	}
	for _, c := range cases {
		bl, err := parseBasketLine(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: want error, got %+v", c.in, bl)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", c.in, err)
			continue
		}
		if bl.ref != c.wantRef || bl.qty != c.wantQty {
			t.Errorf("%q: got {%q, %v}, want {%q, %v}", c.in, bl.ref, bl.qty, c.wantRef, c.wantQty)
		}
	}
}

func TestToonEncodeIsDeterministicAndUsesJSONFieldNames(t *testing.T) {
	type row struct {
		Ref  string  `json:"ref"`
		Qty  float64 `json:"qty"`
		Skip string  `json:"-"`
	}
	v := map[string]any{"lines": []row{{Ref: "abc", Qty: 2, Skip: "hidden"}}}

	got, err := toonEncode(v)
	if err != nil {
		t.Fatalf("toonEncode: %v", err)
	}
	if !strings.Contains(got, "ref") || !strings.Contains(got, "abc") {
		t.Errorf("json field names missing from TOON output: %q", got)
	}
	if strings.Contains(got, "hidden") || strings.Contains(got, "Skip") {
		t.Errorf(`json:"-" field leaked into TOON output: %q`, got)
	}

	again, err := toonEncode(v)
	if err != nil || again != got {
		t.Errorf("toonEncode is not deterministic:\n%q\n%q", got, again)
	}
}

func TestToonEncodeReportsUnencodableValues(t *testing.T) {
	if _, err := toonEncode(make(chan int)); err == nil {
		t.Fatal("want an error for an unencodable value")
	}
}

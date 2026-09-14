package client

import (
	"testing"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

func TestScrubTextRemovesOnlyControlCharacters(t *testing.T) {
	esc := "\x1b"
	for _, tc := range []struct{ in, want string }{
		{"Taladro PRACTYL 500 W", "Taladro PRACTYL 500 W"},
		{"Ca\u00f1o de 3 cm \u2014 \u00f1, \u00e7, \u20ac", "Ca\u00f1o de 3 cm \u2014 \u00f1, \u00e7, \u20ac"},
		{"Taladro" + esc + "]0;PWNED\x07 barato", "Taladro]0;PWNED barato"},
		{"dos\tcolumnas\nl\u00ednea\r\n", "dos columnas l\u00ednea  "},
		{"borra\x7fdel y\u0085NEL", "borradel yNEL"},
	} {
		if got := scrubText(tc.in); got != tc.want {
			t.Errorf("scrubText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The promo label is the one product field behind a pointer, so it needs a nil
// check the flat fields do not.
func TestScrubProductCleansThePromoLabel(t *testing.T) {
	p := domain.Product{}
	p.Offer.Animations = &struct {
		Label string `json:"label"`
	}{Label: "Env\u00edo\x1b[31m gratis"}
	scrubProduct(&p)
	if got := p.Offer.Animations.Label; got != "Env\u00edo[31m gratis" {
		t.Errorf("promo label = %q", got)
	}
}

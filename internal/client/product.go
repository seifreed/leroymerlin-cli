package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// ProductDetail is a product page distilled from its schema.org JSON-LD. Prices
// are tax-included euros as decimal strings (the JSON-LD form), matching the
// figure the page shows.
type ProductDetail struct {
	Name        string    `json:"name"`
	SKU         string    `json:"sku"`
	GTIN        string    `json:"gtin"`
	Description string    `json:"description"`
	Brand       flexName  `json:"brand"`
	Offers      offerNode `json:"offers"`
	Rating      *struct {
		Value string `json:"ratingValue"`
		Count string `json:"reviewCount"`
	} `json:"aggregateRating"`
	Image flexImage `json:"image"`
	Specs []Spec    `json:"specs,omitempty"` // technical characteristics scraped from the page
	URL   string    `json:"-"`               // the page we fetched it from
}

// Spec is one "characteristic" row, e.g. {"Función percutor", "Sí"}.
type Spec struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Price/Currency/Availability surface the first offer's money fields.
func (p ProductDetail) Price() string        { return p.Offers.first().Price }
func (p ProductDetail) Currency() string     { return p.Offers.first().PriceCurrency }
func (p ProductDetail) Availability() string { return shortAvailability(p.Offers.first().Availability) }

// shortAvailability trims the schema.org URL prefix: "http://schema.org/InStock"
// → "InStock".
func shortAvailability(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// offer is one schema.org Offer.
type offer struct {
	Price         string `json:"price"`
	PriceCurrency string `json:"priceCurrency"`
	Availability  string `json:"availability"`
	URL           string `json:"url"`
	ItemCondition string `json:"itemCondition"`
}

// offerNode accepts `offers` as a single Offer or a list of them (multi-seller
// products), exposing the first via first().
type offerNode []offer

func (o *offerNode) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, (*[]offer)(o))
	}
	var single offer
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*o = offerNode{single}
	return nil
}

func (o offerNode) first() offer {
	if len(o) == 0 {
		return offer{}
	}
	return o[0]
}

// flexName accepts a string or an object with a "name" field (schema.org's
// Brand can be either) and reduces it to the name.
type flexName string

func (f *flexName) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexName(s)
		return nil
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	*f = flexName(obj.Name)
	return nil
}

// flexImage accepts a string, a list of strings, or an object with a "url",
// reducing it to the first image URL.
type flexImage string

func (f *flexImage) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexImage(s)
	case '[':
		var ss []string
		if err := json.Unmarshal(b, &ss); err != nil {
			return err
		}
		if len(ss) > 0 {
			*f = flexImage(ss[0])
		}
	default:
		var obj struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		*f = flexImage(obj.URL)
	}
	return nil
}

// ErrNoProduct means the page carried no schema.org Product JSON-LD (wrong URL,
// a 404 page, or a non-product page).
var ErrNoProduct = errors.New("no product data found on page")

// Product fetches a product page (URL or site-relative path, as returned by
// Search) and distils its JSON-LD into a ProductDetail.
func (c *Client) Product(urlOrPath string) (*ProductDetail, error) {
	html, err := c.GetHTML(urlOrPath)
	if err != nil {
		return nil, err
	}
	d, err := parseProductDetail(html)
	if err != nil {
		return nil, err
	}
	d.URL = c.resolve(urlOrPath)
	d.Specs = parseSpecs(html)
	return d, nil
}

// specRowRE matches one technical-characteristic list item; the captured inner
// HTML is "Label : Value" (after tag stripping).
var specRowRE = regexp.MustCompile(`(?s)o-main-characteristics__li[^>]*>(.*?)</li>`)

// specJunk marks a captured row that ran past a real characteristic into page
// scaffolding (the "Ver más" expander wraps scripts/markup) — drop it.
var specJunk = regexp.MustCompile(`(?i)\{|window\.|-->|Ver más|Añadir|Vendido|EUR|en stock`)

// parseSpecs scrapes the product page's technical characteristics — the
// "o-main-characteristics" list, where each row reads "Etiqueta : Valor" — into
// label/value pairs. Returns nil when the page has no such table. Rows whose
// value is implausibly long or carries page scaffolding are skipped (a malformed
// list item can otherwise swallow trailing markup).
func parseSpecs(html string) []Spec {
	var out []Spec
	seen := make(map[string]bool)
	for _, m := range specRowRE.FindAllStringSubmatch(html, -1) {
		text := cleanText(m[1])
		// Split on the first " : " so a value containing a colon stays intact.
		i := strings.Index(text, " : ")
		if i < 0 {
			continue
		}
		label := strings.TrimSpace(text[:i])
		value := strings.TrimSpace(text[i+3:])
		if label == "" || value == "" || seen[label] {
			continue
		}
		if len(label) > 80 || len(value) > 80 || specJunk.MatchString(value) {
			continue
		}
		seen[label] = true
		out = append(out, Spec{Label: label, Value: value})
	}
	return out
}

const ldMarker = `application/ld+json`

// parseProductDetail scans the page's ld+json blocks for the schema.org Product
// node (handling both a bare object and an array of nodes) and decodes it.
func parseProductDetail(html string) (*ProductDetail, error) {
	rest := html
	for {
		i := strings.Index(rest, ldMarker)
		if i < 0 {
			break
		}
		rest = rest[i+len(ldMarker):]
		gt := strings.IndexByte(rest, '>')
		if gt < 0 {
			break
		}
		rest = rest[gt+1:]
		end := strings.Index(rest, "</script>")
		if end < 0 {
			break
		}
		block := strings.TrimSpace(rest[:end])
		rest = rest[end+len("</script>"):]
		if d := productFromLD([]byte(block)); d != nil {
			return d, nil
		}
	}
	return nil, ErrNoProduct
}

// productFromLD returns the Product node from one ld+json block, or nil. A block
// may be a single node or an array of them.
func productFromLD(block []byte) *ProductDetail {
	block = bytes.TrimSpace(block)
	if len(block) == 0 {
		return nil
	}
	if block[0] == '[' {
		var nodes []json.RawMessage
		if json.Unmarshal(block, &nodes) != nil {
			return nil
		}
		for _, n := range nodes {
			if d := productFromLD(n); d != nil {
				return d
			}
		}
		return nil
	}
	var probe struct {
		Type json.RawMessage `json:"@type"`
	}
	if json.Unmarshal(block, &probe) != nil || !typeIs(probe.Type, "Product") {
		return nil
	}
	var d ProductDetail
	if json.Unmarshal(block, &d) != nil {
		return nil
	}
	return &d
}

// typeIs reports whether a JSON-LD @type (a string or array of strings) contains
// want.
func typeIs(raw json.RawMessage, want string) bool {
	if len(raw) == 0 {
		return false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == want
	}
	var ss []string
	if json.Unmarshal(raw, &ss) == nil {
		for _, v := range ss {
			if v == want {
				return true
			}
		}
	}
	return false
}

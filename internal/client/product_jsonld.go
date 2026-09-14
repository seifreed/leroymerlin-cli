package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

// offerNode decodes schema.org "offers" — either one offer object or an array of
// them — straight into the domain type, whose JSON field names are identical.
type offerNode []domain.ProductOffer

func (o *offerNode) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, (*[]domain.ProductOffer)(o))
	}
	var single domain.ProductOffer
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*o = offerNode{single}
	return nil
}

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

// ErrNoProduct means the page carried no schema.org Product JSON-LD.
var ErrNoProduct = errors.New("no product data found on page")

const ldMarker = `application/ld+json`

func parseProductDetail(html string) (*domain.ProductDetail, error) {
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

type schemaProductDetail struct {
	Name        string                `json:"name"`
	SKU         string                `json:"sku"`
	GTIN        string                `json:"gtin"`
	Description string                `json:"description"`
	Brand       flexName              `json:"brand"`
	Offers      offerNode             `json:"offers"`
	Rating      *domain.ProductRating `json:"aggregateRating"`
	Image       flexImage             `json:"image"`
}

func productFromLD(block []byte) *domain.ProductDetail {
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
	var raw schemaProductDetail
	if json.Unmarshal(block, &raw) != nil {
		return nil
	}
	d := raw.toDomain()
	scrubProductDetail(d)
	return d
}

func (p schemaProductDetail) toDomain() *domain.ProductDetail {
	return &domain.ProductDetail{
		Name: p.Name, SKU: p.SKU, GTIN: p.GTIN, Description: p.Description,
		// append onto an empty slice so a product with no offers still marshals as
		// [] rather than null, as --json consumers have always seen it.
		Brand: string(p.Brand), Offers: append([]domain.ProductOffer{}, p.Offers...),
		Rating: p.Rating, Image: string(p.Image),
	}
}

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

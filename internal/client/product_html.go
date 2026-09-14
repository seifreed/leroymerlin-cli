package client

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/seifreed/leroymerlin-cli/internal/domain"
)

type rawDelivery struct {
	Type   string  `json:"type"`
	Status string  `json:"stockStatus"`
	Stock  int     `json:"stock"`
	Price  float64 `json:"price"`
	Time   string  `json:"time"`
}

func parseDeliveries(html string) []domain.DeliveryOption {
	raw := extractJSONArray(html, `"available_deliveries"`)
	if raw == "" {
		return nil
	}
	var rds []rawDelivery
	if json.Unmarshal([]byte(raw), &rds) != nil {
		return nil
	}
	out := make([]domain.DeliveryOption, 0, len(rds))
	for _, r := range rds {
		out = append(out, domain.DeliveryOption{
			Type: scrubText(r.Type), Status: scrubText(r.Status),
			Stock: r.Stock, Price: r.Price, Time: scrubText(r.Time),
		})
	}
	return out
}

func extractJSONArray(s, key string) string {
	k := strings.Index(s, key)
	if k < 0 {
		return ""
	}
	start := strings.IndexByte(s[k:], '[')
	if start < 0 {
		return ""
	}
	start += k
	// Brackets inside a string value are data, not structure: a delivery label
	// carrying one used to unbalance the count and drop the whole block.
	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		switch c := s[i]; {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '[':
			depth++
		case c == ']':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

var specRowRE = regexp.MustCompile(`(?s)o-main-characteristics__li[^>]*>(.*?)</li>`)
var specJunk = regexp.MustCompile(`(?i)\{|window\.|-->|Ver más|Añadir|Vendido|EUR|en stock`)

func parseSpecs(html string) []domain.Spec {
	var out []domain.Spec
	seen := make(map[string]bool)
	for _, m := range specRowRE.FindAllStringSubmatch(html, -1) {
		text := cleanText(m[1])
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
		out = append(out, domain.Spec{Label: label, Value: value})
	}
	return out
}

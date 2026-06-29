package client

import (
	"regexp"
	"strings"
)

// Category is a top-level catalog section, e.g. {"Iluminación",
// "/productos/iluminacion/"}.
type Category struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// categoryAnchorRE matches a single-segment category link and its label, e.g.
// <a href="/productos/iluminacion/">Iluminación</a>. The label capture is
// bounded so a runaway match can't swallow the rest of the document.
var categoryAnchorRE = regexp.MustCompile(`(?s)<a[^>]+href="(/productos/[a-z0-9-]+/)"[^>]*>(.{0,80}?)</a>`)

var tagRE = regexp.MustCompile(`<[^>]+>`)

// cleanText strips HTML tags and collapses whitespace in an anchor's label.
func cleanText(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(tagRE.ReplaceAllString(s, " ")), " "))
}

// Categories returns the top-level catalog sections, scraped from the /productos/
// index in document order.
func (c *Client) Categories() ([]Category, error) {
	html, err := c.GetHTML("/productos/")
	if err != nil {
		return nil, err
	}
	return parseCategories(html), nil
}

// parseCategories lifts the {name, path} pairs from single-segment /productos/
// links, deduped (first label wins).
func parseCategories(html string) []Category {
	var out []Category
	seen := make(map[string]bool)
	for _, m := range categoryAnchorRE.FindAllStringSubmatch(html, -1) {
		path, name := m[1], cleanText(m[2])
		if name == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, Category{Name: name, Path: path})
	}
	return out
}

// subcatAnchorRE matches a child-category link in the "thematic mesh": the name
// is a clean data-button-name attribute, the href a two-segment product path.
var subcatAnchorRE = regexp.MustCompile(`data-button-name="([^"]+)"[^>]*?href="(/productos/[a-z0-9-]+/[a-z0-9-]+/)"`)

// Subcategories scrapes a category page's child sections (name + path), deduped
// in document order. Only true children of this category are returned (links to
// brands or other sections in the same mesh are filtered out). Returns nil for a
// leaf category or a curated landing page with no children.
func (c *Client) Subcategories(path string) ([]Category, error) {
	parent := normalizeCategoryPath(path)
	html, err := c.GetHTML(parent)
	if err != nil {
		return nil, err
	}
	return parseSubcategories(html, parent), nil
}

func parseSubcategories(html, parent string) []Category {
	var out []Category
	seen := make(map[string]bool)
	for _, m := range subcatAnchorRE.FindAllStringSubmatch(html, -1) {
		name, p := cleanText(m[1]), m[2]
		// Keep only direct children of the parent (not brand/other-section links).
		if name == "" || seen[p] || !strings.HasPrefix(p, parent) || p == parent {
			continue
		}
		seen[p] = true
		out = append(out, Category{Name: name, Path: p})
	}
	return out
}

// CategoryProducts lists the products on a category page. path may be a slug
// ("iluminacion"), a "productos/iluminacion" path, or a full "/productos/
// iluminacion/" path — all normalized. limit caps the result count: 0 returns a
// single page; a larger limit auto-paginates (same `p` query param as search).
func (c *Client) CategoryProducts(path string, limit int) ([]Product, error) {
	return c.paginated(normalizeCategoryPath(path), limit)
}

// normalizeCategoryPath turns any accepted category reference into the canonical
// "/productos/<slug>/" form.
func normalizeCategoryPath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	p = strings.TrimPrefix(p, "productos/")
	return "/productos/" + p + "/"
}

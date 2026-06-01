// Package web is the HTTP + server-rendered HTML layer (html/template + htmx).
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"finance-tracker/internal/domain"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// StaticFS exposes the embedded static assets for the router.
func StaticFS() fs.FS {
	sub, _ := fs.Sub(staticFS, "static")
	return sub
}

var funcMap = template.FuncMap{
	"formatINR":   func(m domain.Money) string { return m.FormatINR() },
	"rupees":      func(m domain.Money) float64 { return m.Rupees() },
	"isNeg":       func(m domain.Money) bool { return m.IsNegative() },
	"amountClass": amountClass,
	"formatDate":  formatDate,
	"formatMonth": formatMonth,
	"pct":         func(f float64) string { return fmt.Sprintf("%.1f%%", f*100) },
	"pct0":        func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) },
	"barPct":      barPct,
	"flowColor":   flowColor,
	"title":       func(s string) string { return s },
	"dict":        dict,
	"upper":       strings.ToUpper,
	// pathEscape makes a value safe to embed in a URL path segment (e.g. a
	// category name with spaces, '&', '#', '?').
	"pathEscape": url.PathEscape,
}

// flowPalette colors the sankey/flow segments deterministically by index.
var flowPalette = []string{
	"#1e40af", "#16a34a", "#dc2626", "#7c3aed", "#fbbf24", "#0891b2",
	"#db2777", "#65a30d", "#ea580c", "#4f46e5", "#0d9488", "#b45309",
	"#9333ea", "#15803d", "#be123c", "#2563eb",
}

func flowColor(i int) string { return flowPalette[i%len(flowPalette)] }

// barPct clamps a 0..1+ ratio to an integer 0..100 for progress-bar widths.
func barPct(f float64) int {
	p := int(f * 100)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// amountClass returns a Tailwind text color for an amount (red if negative).
func amountClass(m domain.Money) string {
	if m.IsNegative() {
		return "text-red-600"
	}
	return "text-green-600"
}

// formatDate renders an ISO date "2006-01-02" as "02-Jan-2006".
func formatDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02-Jan-2006")
}

// formatMonth renders a "YYYY-MM" key (or ISO date) as "Jan 2006".
func formatMonth(key string) string {
	if len(key) > 7 {
		key = key[:7]
	}
	t, err := time.Parse("2006-01", key)
	if err != nil {
		return key
	}
	return t.Format("Jan 2006")
}

// dict builds a map from alternating key/value pairs for passing multiple
// values into a partial template.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict requires an even number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %d is not a string", i)
		}
		m[k] = pairs[i+1]
	}
	return m, nil
}

// Renderer holds the parsed page templates and a standalone fragment set.
type Renderer struct {
	pages     map[string]*template.Template // each executes "layout"
	fragments *template.Template            // partials, executed by define-name
}

// NewRenderer parses all embedded templates.
func NewRenderer() (*Renderer, error) {
	pageNames := []string{"dashboard", "transactions", "recurring", "month", "settings", "planning", "year"}
	r := &Renderer{pages: map[string]*template.Template{}}

	for _, name := range pageNames {
		t := template.New("layout").Funcs(funcMap)
		t, err := t.ParseFS(templateFS,
			"templates/layout.html",
			"templates/partials/*.html",
			"templates/"+name+".html",
		)
		if err != nil {
			return nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		r.pages[name] = t
	}

	frag := template.New("fragments").Funcs(funcMap)
	frag, err := frag.ParseFS(templateFS, "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse fragments: %w", err)
	}
	r.fragments = frag
	return r, nil
}

// Page renders a full page (layout + the named page's content block).
func (r *Renderer) Page(w http.ResponseWriter, name string, data any) {
	t, ok := r.pages[name]
	if !ok {
		http.Error(w, "unknown page: "+name, http.StatusInternalServerError)
		return
	}
	r.exec(w, t, "layout", data)
}

// Fragment renders a single partial by its template name (e.g. "tx_table").
func (r *Renderer) Fragment(w http.ResponseWriter, name string, data any) {
	r.exec(w, r.fragments, name, data)
}

// exec renders into a buffer first so a template error doesn't emit a partial,
// half-written response.
func (r *Renderer) exec(w http.ResponseWriter, t *template.Template, name string, data any) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// Package slides is the machinery the presentation exporters share.
//
// There are four of them now - reveal.js, impress.js, WebSlides and deck.js -
// and they are four drawings of one thing: an org file is a deck, a headline is
// a slide, a property says how that slide behaves. The frameworks disagree
// about the html and about almost nothing else, so everything up to the html
// lives here: which headlines are slides, where the speaker notes are, what a
// background means, how a file link becomes a url this server will serve.
//
// This package exists because the first two exporters were a copy of each other
// - `GetProp`, `GetPropTag`, `WriteRegularLink`, `funcMap` and `ValidateMap`
// were duplicated verbatim, and the copies had already drifted (impress grew
// numeric and indexed property readers that reveal never got; reveal's image
// writer gained a fallback that impress's lacks). A third and fourth copy would
// have been two more things to forget, and the interesting rules - what a slide
// level is, where notes come from - would have been said four times.
//
// The rule it buys: **a property means the same thing in every framework.**
// `:BACKGROUND: blue` is a blue slide in all four. `:REVEAL_BACKGROUND:` is a
// blue slide in reveal only. Nothing has to be written four times to work four
// times.
package slides

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

// ── Reading a setting ────────────────────────────────────────────────────────

// Conf is one presentation's settings, read the way org reads settings: the
// narrowest answer wins.
//
// For a headline, in order: its own `:REVEAL_FOO:`, its own `:SLIDE_FOO:`, its
// own `:FOO:`, then the document's `#+REVEAL_FOO:`, then `#+SLIDE_FOO:`. For
// the document, the last two of those.
//
// The bare `:FOO:` form is the point of the ladder. A deck written with
// `:BACKGROUND:` and `:CLASS:` properties is a deck in all four frameworks; the
// prefixed forms are there for the slide that needs to say something only one
// of them understands.
type Conf struct {
	Doc    *org.Document
	Prefix string
}

func (c Conf) names(name string) []string {
	name = strings.ToUpper(name)
	if c.Prefix == "" {
		return []string{"SLIDE_" + name, name}
	}
	return []string{c.Prefix + "_" + name, "SLIDE_" + name}
}

// DocStr is a document-wide setting.
func (c Conf) DocStr(name, def string) string {
	if c.Doc == nil {
		return def
	}
	for _, n := range c.names(name) {
		if v := strings.TrimSpace(c.Doc.Get(n)); v != "" {
			return v
		}
	}
	return def
}

// Str is a setting for one slide, falling back to the document.
func (c Conf) Str(p PropGet, name, def string) string {
	if p != nil {
		for _, n := range append(c.names(name), strings.ToUpper(name)) {
			if v, ok := p(n); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return c.DocStr(name, def)
}

// Has reports whether this slide or the document said anything at all about it.
func (c Conf) Has(p PropGet, name string) bool {
	return c.Str(p, name, "\x00") != "\x00"
}

// Bool is a switch. Org has no boolean, so every spelling anybody uses for one
// is accepted, and *the absence of a value is true* - `:AUTO_ANIMATE:` on its
// own is somebody turning it on, not setting it to the empty string.
func (c Conf) Bool(p PropGet, name string, def bool) bool {
	v := c.Str(p, name, "\x00")
	if v == "\x00" {
		return def
	}
	return Truthy(v)
}

// DocBool is the same for the document.
func (c Conf) DocBool(name string, def bool) bool {
	v := c.DocStr(name, "\x00")
	if v == "\x00" {
		return def
	}
	return Truthy(v)
}

func (c Conf) Int(p PropGet, name string, def int) int {
	if n, err := strconv.Atoi(c.Str(p, name, "")); err == nil {
		return n
	}
	return def
}

func (c Conf) DocInt(name string, def int) int {
	if n, err := strconv.Atoi(c.DocStr(name, "")); err == nil {
		return n
	}
	return def
}

func (c Conf) Float(p PropGet, name string, def float64) (float64, bool) {
	if f, err := strconv.ParseFloat(c.Str(p, name, ""), 64); err == nil {
		return f, true
	}
	return def, false
}

// Truthy is org's idea of yes. An empty value counts as yes because a property
// written with nothing after it is somebody switching something on.
func Truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "nil", "false", "off", "no", "n", "f", "0", "none":
		return false
	}
	return true
}

// ── Building up html attributes ──────────────────────────────────────────────

// Attrs is an html attribute list under construction. Classes and styles
// accumulate rather than overwrite, because a slide collects them from several
// places - its own property, the framework's idea of a background, a layout
// that positions it - and whichever ran last should not win.
type Attrs struct {
	id      string
	classes []string
	styles  []string
	pairs   []string
	flags   []string
}

func (a *Attrs) ID(id string) {
	if id != "" {
		a.id = id
	}
}

func (a *Attrs) Class(c ...string) {
	for _, one := range c {
		for _, w := range strings.Fields(one) {
			if !contains(a.classes, w) {
				a.classes = append(a.classes, w)
			}
		}
	}
}

func (a *Attrs) HasClass(c string) bool { return contains(a.classes, c) }

func (a *Attrs) Style(s string) {
	if s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ";")); s != "" {
		a.styles = append(a.styles, s)
	}
}

// Set writes an attribute, skipping it when there is no value - "no opinion" and
// "the empty string" are different things in html and only the second is worth
// writing out.
func (a *Attrs) Set(k, v string) {
	if k == "" || v == "" {
		return
	}
	a.pairs = append(a.pairs, fmt.Sprintf(`%s="%s"`, k, html.EscapeString(v)))
}

// SetIf writes the attribute when the setting says anything at all.
func (a *Attrs) SetIf(cond bool, k, v string) {
	if cond {
		a.Set(k, v)
	}
}

// Flag is a valueless attribute: data-auto-animate, hidden.
func (a *Attrs) Flag(k string) {
	if k != "" && !contains(a.flags, k) {
		a.flags = append(a.flags, k)
	}
}

func (a *Attrs) String() string {
	out := []string{}
	if a.id != "" {
		out = append(out, fmt.Sprintf(`id="%s"`, html.EscapeString(a.id)))
	}
	if len(a.classes) > 0 {
		out = append(out, fmt.Sprintf(`class="%s"`, html.EscapeString(strings.Join(a.classes, " "))))
	}
	out = append(out, a.pairs...)
	out = append(out, a.flags...)
	if len(a.styles) > 0 {
		out = append(out, fmt.Sprintf(`style="%s"`, html.EscapeString(strings.Join(a.styles, "; "))))
	}
	if len(out) == 0 {
		return ""
	}
	return " " + strings.Join(out, " ")
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// ── Where a slide's id comes from ────────────────────────────────────────────

var idSafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// SlideID is the id a slide gets in the html: its own CUSTOM_ID if it has one,
// so a link into the deck keeps working when a slide is moved, and a positional
// one otherwise. A positional id is still worth having - every one of these
// frameworks can be deep-linked, and `#/slide-3` is better than nothing.
func SlideID(c Conf, p PropGet, fallback string) string {
	if p != nil {
		for _, key := range []string{"CUSTOM_ID", "ID"} {
			if v, ok := p(key); ok && strings.TrimSpace(v) != "" {
				return idSafe.ReplaceAllString(strings.TrimSpace(v), "-")
			}
		}
	}
	return fallback
}

// ── A file link turned into something a browser can fetch ────────────────────

// Media turns an org file: link into something a browser can fetch.
//
// The sum itself is `plugs.MediaURL`, because the html exporter has to do
// exactly the same one and two answers to "where is that picture" is one answer
// too many. This is here so a slide writer does not have to know that.
func Media(doc *org.Document, pm *common.PluginManager, opts, target string) string {
	return plugs.MediaURL(doc, pm, opts, target)
}

// IsExternal reports whether a target is already somewhere a browser can go.
func IsExternal(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "//") || strings.HasPrefix(s, "data:")
}

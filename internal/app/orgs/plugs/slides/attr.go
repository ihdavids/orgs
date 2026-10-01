package slides

// Per-element attributes: `#+ATTR_SLIDE: :frag fade-in :class big`
//
// A slide property says something about a whole slide. This says something
// about the next thing on it, which is what fragments mostly want to be:
// "these bullets, one at a time" is a sentence about one list, not about the
// slide it happens to be on.
//
// Why it works at all is worth writing down. go-org parses exactly three
// affiliated keywords into a node's metadata - CAPTION, ATTR_HTML and
// ATTR_LATEX - and any other `#+KEY:` line falls through to `default: return 0,
// nil`, which means it is parsed as a plain `org.Keyword` node sitting in the
// body *immediately before* the node it was written above. So a writer can
// pick it up in WriteKeyword, hold it, and spend it on the next element. The
// alternative was teaching go-org a fourth keyword, which would have put a
// presentation feature in a parser four other exporters share.
//
// `#+ATTR_HTML: :class fragment` keeps working and always did - it goes through
// go-org's own machinery. This is for the things ATTR_HTML cannot say, like
// "one item at a time", which is four different classes depending on who is
// drawing.

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
)

// Pending is what the last `#+ATTR_SLIDE:` line asked for, waiting to be spent
// on the next element.
type Pending struct {
	Frag    bool
	Style   string // the framework's fragment style, e.g. reveal's fade-in
	Classes []string
	Styles  []string
	Attrs   []string
}

// Any reports whether anything is waiting.
func (p Pending) Any() bool {
	return p.Frag || len(p.Classes) > 0 || len(p.Styles) > 0 || len(p.Attrs) > 0
}

var attrKeyRe = regexp.MustCompile(`:([A-Za-z_-]+)\s*`)

// IsAttrKeyword reports whether this keyword is one of ours: the framework's own
// `#+ATTR_REVEAL:`, or the neutral `#+ATTR_SLIDE:` that works in all of them.
func IsAttrKeyword(key, prefix string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	return key == "ATTR_SLIDE" || (prefix != "" && key == "ATTR_"+prefix)
}

// AddAttr reads one `#+ATTR_SLIDE:` value into the pending set. Repeated lines
// accumulate, so a run of them above one element is one description of it.
func (p *Pending) AddAttr(value string) {
	// Split ":key rest" pairs, keeping the order they were written in.
	idx := attrKeyRe.FindAllStringSubmatchIndex(value, -1)
	for n, m := range idx {
		key := strings.ToLower(value[m[2]:m[3]])
		end := len(value)
		if n+1 < len(idx) {
			end = idx[n+1][0]
		}
		val := strings.TrimSpace(value[m[1]:end])
		switch key {
		case "frag", "fragment", "incremental":
			p.Frag = true
			if val != "" && !strings.EqualFold(val, "t") {
				p.Style = val
			}
		case "class":
			p.Classes = append(p.Classes, strings.Fields(val)...)
		case "style":
			if val != "" {
				p.Styles = append(p.Styles, strings.TrimSuffix(val, ";"))
			}
		case "frag_index", "fragindex", "index":
			if val != "" {
				p.Attrs = append(p.Attrs, fmt.Sprintf(`data-fragment-index="%s"`, html.EscapeString(val)))
			}
		default:
			// Anything else is passed through as an attribute, so a framework
			// feature nobody here has heard of is still reachable from org.
			if val != "" {
				p.Attrs = append(p.Attrs, fmt.Sprintf(`data-%s="%s"`, key, html.EscapeString(val)))
			} else {
				p.Attrs = append(p.Attrs, "data-"+key)
			}
		}
	}
}

// Take spends what is pending and resets it. Attributes apply to one element:
// leaving them down would quietly decorate the rest of the slide.
func (p *Pending) Take() Pending {
	out := *p
	*p = Pending{}
	return out
}

// Wrap puts the pending classes, styles and attributes onto one rendered
// element by opening a span or div around it. Wrapping rather than editing the
// element's own tag keeps this out of go-org's business - the element has
// already been rendered by the writer that knows how.
func (p Pending) Wrap(inner, tag, fragClass string) string {
	if !p.Any() {
		return inner
	}
	a := &Attrs{}
	a.Class(p.Classes...)
	if p.Frag && fragClass != "" {
		a.Class(fragClass)
	}
	for _, s := range p.Styles {
		a.Style(s)
	}
	extra := ""
	if len(p.Attrs) > 0 {
		extra = " " + strings.Join(p.Attrs, " ")
	}
	return fmt.Sprintf("<%s%s%s>%s</%s>", tag, a.String(), extra, inner, tag)
}

var listTags = map[string][]string{
	"unordered":   {"<ul>", "</ul>"},
	"ordered":     {"<ol>", "</ol>"},
	"descriptive": {"<dl>", "</dl>"},
}

// FragList writes a list whose items appear one at a time, asking the caller
// what one revealed item is called in its framework - a reveal `fragment`, a
// deck.js nested `slide`, an impress `substep`.
//
// `itemClass` is asked per item rather than once, because reveal counts
// fragments and a caller may want to say so.
//
// The list is rebuilt here rather than decorated afterwards: an `<li>` is
// written by go-org and the class has to be on *that* tag, not on a span inside
// it - every one of these frameworks hides the element it is given, and hiding
// a span inside a bullet leaves the bullet behind.
func FragList(w *org.HTMLWriter, l org.List, itemClass func(i int) string) string {
	tags, ok := listTags[l.Kind]
	if !ok {
		return w.WriteNodesAsString(l)
	}
	var b strings.Builder
	b.WriteString(tags[0] + "\n")
	n := 0
	for _, item := range l.Items {
		cls := itemClass(n)
		switch li := item.(type) {
		case org.ListItem:
			a := &Attrs{}
			a.Class(cls)
			if li.Value != "" {
				a.Set("value", li.Value)
			}
			if li.Status != "" {
				a.Class(statusClass(li.Status))
			}
			b.WriteString(fmt.Sprintf("<li%s>", a.String()))
			b.WriteString(w.WriteNodesAsString(li.Children...))
			b.WriteString("</li>\n")
			n++
		case org.DescriptiveListItem:
			a := &Attrs{}
			a.Class(cls)
			b.WriteString(fmt.Sprintf("<dt%s>", a.String()))
			b.WriteString(w.WriteNodesAsString(li.Term...))
			b.WriteString("</dt>\n")
			b.WriteString(fmt.Sprintf("<dd%s>", a.String()))
			b.WriteString(w.WriteNodesAsString(li.Details...))
			b.WriteString("</dd>\n")
			n++
		default:
			b.WriteString(w.WriteNodesAsString(item))
		}
	}
	b.WriteString(tags[1] + "\n")
	return b.String()
}

func statusClass(status string) string {
	switch status {
	case " ":
		return "unchecked"
	case "-":
		return "indeterminate"
	case "X", "x":
		return "checked"
	}
	return ""
}

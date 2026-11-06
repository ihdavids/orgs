package common

// Callouts: markdown's `> [!warning] Title` boxes, written the way org writes
// a box already - a special block.
//
//	#+begin_warning Mind the gap :fold closed
//	Text of the callout.
//	#+end_warning
//
// The block's name is the type (the usual callout types and aliases), the words
// after it the title (or `:title "..."`), `:fold open|closed` makes it fold
// (markdown's `+`/`-`), `:icon` and `:color` change the look. `#+begin_callout
// Title :type example` is the general form, for a type whose own name is
// already something else in org (example is a verbatim block).
//
// This file is the one list of types and the one reading of a begin line:
// the html, latex, typst, beamer, text and markdown writers, the server's
// /callouts endpoints and `orgs lint` all ask it. The TypeScript copies
// (worg src/callouts.ts, vscode-org-everywhere src/callouts-model.ts) are
// pinned by internal/app/orgs/testdata/callout_cases.json.
//
// Nothing here looks at the feature switch except CalloutsActive: the
// writers call it, so with `features: callouts:` off every export is what it
// was before.

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ihdavids/go-org/org"
)

// CalloutType is one row of the type table.
type CalloutType struct {
	Name    string   // the canonical name
	Title   string   // the title shown when the block has none
	Aliases []string // other block names meaning the same
	Icon    string   // the lucide icon name (for clients with an icon set)
	Glyph   string   // a character standing in for the icon where there is none
	Color   string   // #rrggbb, the default colour
	Beamer  string   // block, alertblock or exampleblock
}

// CalloutTypes is the table of callout types, in the usual order.
var CalloutTypes = []CalloutType{
	{"note", "Note", nil, "pencil", "✎", "#086ddd", "block"},
	{"abstract", "Abstract", []string{"summary", "tldr"}, "clipboard-list", "☰", "#00bfbc", "block"},
	{"info", "Info", nil, "info", "ℹ", "#086ddd", "block"},
	{"todo", "Todo", nil, "check-circle-2", "☐", "#086ddd", "block"},
	{"tip", "Tip", []string{"hint", "important"}, "flame", "✦", "#00bfbc", "exampleblock"},
	{"success", "Success", []string{"check", "done"}, "check", "✔", "#08b94e", "exampleblock"},
	{"question", "Question", []string{"help", "faq"}, "help-circle", "?", "#ec7500", "block"},
	{"warning", "Warning", []string{"caution", "attention"}, "alert-triangle", "⚠", "#ec7500", "alertblock"},
	{"failure", "Failure", []string{"fail", "missing"}, "x", "✘", "#e93147", "alertblock"},
	{"danger", "Danger", []string{"error"}, "zap", "⚡", "#e93147", "alertblock"},
	{"bug", "Bug", nil, "bug", "✱", "#e93147", "alertblock"},
	{"example", "Example", nil, "list", "≡", "#7852ee", "exampleblock"},
	{"quote", "Quote", []string{"cite"}, "quote", "❝", "#9e9e9e", "block"},
}

// Callout is one callout block, as written.
type Callout struct {
	Type    string // the canonical type; "" never (an unknown type is drawn as a note)
	Name    string // the block's name as written, lower case
	Title   string // as written; "" means the type's title
	Fold    string // "", "open" or "closed"
	Icon    string // :icon, as written
	Color   string // :color, when it is a colour
	Unknown string // a type no row names (`:type foo`, or #+begin_foo :fold open)
	Line    int    `json:",omitempty"` // the begin line, from 1 (CalloutsInLines)
	EndLine int    `json:",omitempty"` // the end line, from 1
	Depth   int    `json:",omitempty"` // callouts it sits inside
}

var calloutByName = func() map[string]*CalloutType {
	m := map[string]*CalloutType{}
	for i := range CalloutTypes {
		t := &CalloutTypes[i]
		m[t.Name] = t
		for _, a := range t.Aliases {
			m[a] = t
		}
	}
	return m
}()

// CalloutTypeOf finds a type by its name or an alias, in any case.
func CalloutTypeOf(name string) (CalloutType, bool) {
	if t, ok := calloutByName[strings.ToLower(strings.TrimSpace(name))]; ok {
		return *t, true
	}
	return CalloutType{}, false
}

// Org blocks that are something else: never callouts by their own name.
// example is verbatim; the rest are org's own blocks.
var orgOwnBlocks = map[string]bool{
	"src": true, "example": true, "export": true, "comment": true, "verse": true,
	"center": true, "dyn": true,
}

// Names that are callouts only when the block says so - with a title or a
// callout parameter - because plain they already mean something: a quote
// block, and the abstract LaTeX (and every org user) writes.
var calloutOnlyWithMore = map[string]bool{"quote": true, "abstract": true}

var calloutParamKeys = []string{":title", ":fold", ":icon", ":color", ":colour", ":type"}

var calloutColorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{3,20}|rgba?\([0-9., %]+\))$`)

// calloutOf decides whether a block is a callout. head is the text before
// the first header argument (the title), params the arguments.
func calloutOf(name, head string, params map[string]string) (Callout, bool) {
	name = strings.ToLower(name)
	head = squeeze(head)
	more := head != ""
	for _, k := range calloutParamKeys {
		if _, ok := params[k]; ok {
			more = true
		}
	}
	c := Callout{Name: name}
	if name == "callout" {
		ty := strings.ToLower(unquote(params[":type"]))
		if ty == "" {
			ty = "note"
		}
		if t, ok := CalloutTypeOf(ty); ok {
			c.Type = t.Name
		} else {
			c.Type, c.Unknown = "note", ty
		}
	} else if orgOwnBlocks[name] {
		return Callout{}, false
	} else if t, ok := CalloutTypeOf(name); ok {
		if calloutOnlyWithMore[name] && !more {
			return Callout{}, false
		}
		c.Type = t.Name
	} else if hasCalloutParam(params) {
		// #+begin_foo :fold closed: meant as a callout, of a type no row
		// names. Drawn as a note, the usual fallback for an unknown type.
		c.Type, c.Unknown = "note", name
	} else {
		return Callout{}, false
	}
	c.Title = head
	if t, ok := params[":title"]; ok {
		c.Title = squeeze(unquote(t))
	}
	c.Title = unquote(c.Title)
	if f, ok := params[":fold"]; ok {
		switch strings.ToLower(unquote(f)) {
		case "open", "+", "nil", "no", "unfolded", "show":
			c.Fold = "open"
		case "none", "off":
		default: // closed, -, t, yes, folded, hide, or :fold alone
			c.Fold = "closed"
		}
	}
	c.Icon = unquote(params[":icon"])
	col := unquote(params[":color"])
	if col == "" {
		col = unquote(params[":colour"])
	}
	if calloutColorRe.MatchString(col) {
		c.Color = col
	}
	return c, true
}

var blanksRe = regexp.MustCompile(`[ \t]+`)

// squeeze makes runs of spaces and tabs one space (only those: a no-break
// space is part of the title, written to keep a " :" from starting an
// argument).
func squeeze(s string) string {
	return strings.Trim(blanksRe.ReplaceAllString(s, " "), " ")
}

func hasCalloutParam(params map[string]string) bool {
	for _, k := range []string{":fold", ":icon", ":color", ":colour", ":title"} {
		if _, ok := params[k]; ok {
			return true
		}
	}
	return false
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// splitBlockArgs reads what follows #+begin_NAME as go-org does: the text
// up to the first " :" is the head, then :key value pairs.
func splitBlockArgs(s string) (string, map[string]string) {
	parts := strings.Split(" "+s, " :")
	params := map[string]string{}
	for _, p := range parts[1:] {
		kv := strings.SplitN(p+" ", " ", 2)
		params[":"+strings.ToLower(kv[0])] = strings.TrimSpace(kv[1])
	}
	return strings.TrimSpace(parts[0]), params
}

var calloutBeginRe = regexp.MustCompile(`(?i)^[ \t]*#\+begin_(\S+)(.*)$`)
var blockEndRe = regexp.MustCompile(`(?i)^[ \t]*#\+end_(\S+)`)

// ParseCalloutLine reads a `#+begin_` line. ok is false for a line that is
// not a callout's.
func ParseCalloutLine(line string) (Callout, bool) {
	m := calloutBeginRe.FindStringSubmatch(line)
	if m == nil {
		return Callout{}, false
	}
	head, params := splitBlockArgs(m[2])
	return calloutOf(m[1], head, params)
}

// CalloutOfBlock is ParseCalloutLine for a block go-org has parsed: the head
// is the "language" and the switches go-org split it into.
func CalloutOfBlock(b org.Block) (Callout, bool) {
	head := []string{}
	params := map[string]string{}
	i := 0
	if len(b.Parameters) > 0 && !strings.HasPrefix(b.Parameters[0], ":") {
		head = append(head, b.Parameters[0])
		i = 1
	}
	head = append(head, b.Switches...)
	for ; i+1 < len(b.Parameters); i += 2 {
		params[strings.ToLower(b.Parameters[i])] = b.Parameters[i+1]
	}
	return calloutOf(b.Name, strings.Join(head, " "), params)
}

// CalloutsInLines lists every callout in a file's lines, nested ones too,
// skipping what is inside a src, example, export or comment block.
func CalloutsInLines(lines []string) []Callout {
	out := []Callout{}
	type open struct {
		name string
		idx  int // into out, or -1 for a block that is not a callout
	}
	stack := []open{}
	raw := ""
	depth := 0
	for i, l := range lines {
		if raw != "" {
			if m := blockEndRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[1], raw) {
				raw = ""
			}
			continue
		}
		if m := calloutBeginRe.FindStringSubmatch(l); m != nil {
			name := strings.ToLower(m[1])
			switch name {
			case "src", "example", "export", "comment":
				raw = name
				continue
			}
			if c, ok := ParseCalloutLine(l); ok {
				c.Line, c.Depth = i+1, depth
				out = append(out, c)
				stack = append(stack, open{name, len(out) - 1})
				depth++
			} else {
				stack = append(stack, open{name, -1})
			}
			continue
		}
		if m := blockEndRe.FindStringSubmatch(l); m != nil {
			name := strings.ToLower(m[1])
			for j := len(stack) - 1; j >= 0; j-- {
				if stack[j].name == name {
					for k := len(stack) - 1; k >= j; k-- {
						if stack[k].idx >= 0 {
							out[stack[k].idx].EndLine = i + 1
							depth--
						}
					}
					stack = stack[:j]
					break
				}
			}
		}
	}
	// A block never closed is not a block, as in org.
	kept := out[:0]
	for _, c := range out {
		if c.EndLine > 0 {
			kept = append(kept, c)
		}
	}
	return kept
}

// TypeInfo is the callout's row; an unknown type gets the note's look and
// its own name as title.
func (c Callout) TypeInfo() CalloutType {
	t, _ := CalloutTypeOf(c.Type)
	if t.Name == "" {
		t = CalloutTypes[0]
	}
	if c.Unknown != "" {
		t.Title = capitalise(c.Unknown)
	}
	return t
}

// ShownTitle is the title a reader sees: the block's, else the type's.
func (c Callout) ShownTitle() string {
	if c.Title != "" {
		return c.Title
	}
	return c.TypeInfo().Title
}

// ShownColor is :color, else the type's.
func (c Callout) ShownColor() string {
	if c.Color != "" {
		return c.Color
	}
	return c.TypeInfo().Color
}

// ShownGlyph is the icon as a character: :icon naming a type is that type's
// glyph, a short :icon (an emoji) is itself, else the type's.
func (c Callout) ShownGlyph() string {
	if c.Icon != "" {
		if t, ok := CalloutTypeOf(c.Icon); ok {
			return t.Glyph
		}
		if utf8.RuneCountInString(c.Icon) <= 4 {
			return c.Icon
		}
	}
	return c.TypeInfo().Glyph
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// CalloutsOn is set by the server to its feature switch; nil is off (a
// client, or a test that does not set it).
var CalloutsOn func() bool

// CalloutsActive says whether the writers draw callouts.
func CalloutsActive() bool { return CalloutsOn != nil && CalloutsOn() }

// CalloutHTML is the html of a callout around its body. titleHTML is the
// title already written as html. A folding callout is a <details>.
func CalloutHTML(c Callout, titleHTML, bodyHTML string) string {
	t := c.TypeInfo()
	cls := "callout callout-" + t.Name
	if c.Unknown != "" {
		cls += " callout-" + html.EscapeString(c.Unknown)
	}
	style := ""
	if c.Color != "" {
		style = ` style="--callout-color: ` + html.EscapeString(c.Color) + `"`
	}
	icon := `<span class="callout-icon" aria-hidden="true">` + html.EscapeString(c.ShownGlyph()) + `</span>`
	title := `<span class="callout-title-inner">` + titleHTML + `</span>`
	data := ` data-callout="` + html.EscapeString(t.Name) + `"`
	if c.Fold != "" {
		open := ""
		if c.Fold == "open" {
			open = " open"
		}
		return `<details class="` + cls + ` is-collapsible"` + data + style + open + `>` + "\n" +
			`<summary class="callout-title">` + icon + title + `</summary>` + "\n" +
			`<div class="callout-content">` + "\n" + bodyHTML + "</div>\n</details>\n"
	}
	return `<div class="` + cls + `"` + data + style + `>` + "\n" +
		`<div class="callout-title">` + icon + title + `</div>` + "\n" +
		`<div class="callout-content">` + "\n" + bodyHTML + "</div>\n</div>\n"
}

// CalloutCSS is the stylesheet the html exporter appends to every theme
// while callouts are on (as it does the audio player's): colours come from
// --callout-color, mixed into the page's own background, so one sheet suits
// a light and a dark theme. dark says the theme is dark whatever the
// reader's setting; follows says the theme itself follows the reader's
// light or dark setting (prefers-color-scheme, [data-theme]), as the
// documentation themes do - a light theme stays light on a dark desktop.
func CalloutCSS(dark, follows bool) string {
	var b strings.Builder
	b.WriteString(`
.callout { --callout-color: #086ddd; margin: 1em 0; border: 1px solid color-mix(in srgb, var(--callout-color) 28%, transparent); border-left: 4px solid var(--callout-color); border-radius: 6px; background: color-mix(in srgb, var(--callout-color) 8%, transparent); overflow: hidden; }
`)
	for _, t := range CalloutTypes {
		b.WriteString(".callout-" + t.Name + " { --callout-color: " + t.Color + "; }\n")
	}
	b.WriteString(`.callout-title { display: flex; align-items: baseline; gap: 0.5em; padding: 0.45em 0.8em; font-weight: 600; color: var(--callout-color); }
.callout-title strong, .callout-title b, .callout-title a { color: inherit; }
.callout-icon { flex: none; width: 1.2em; text-align: center; }
.callout-title-inner { flex: 1 1 auto; }
summary.callout-title { cursor: pointer; list-style: none; }
summary.callout-title::-webkit-details-marker { display: none; }
summary.callout-title::after { content: "\25B8"; flex: none; transition: transform 0.15s; opacity: 0.7; }
details.callout[open] > summary.callout-title::after { transform: rotate(90deg); }
.callout-content { padding: 0.05em 1em 0.5em; }
.callout-content > :first-child { margin-top: 0.2em; }
.callout-content > :last-child { margin-bottom: 0.3em; }
`)
	darkRules := `.callout { background: color-mix(in srgb, var(--callout-color) 14%, transparent); }
.callout-title { color: color-mix(in srgb, var(--callout-color) 70%, white); }
`
	if dark {
		b.WriteString(darkRules)
	} else if follows {
		b.WriteString("@media (prefers-color-scheme: dark) {\n:root:not([data-theme=\"light\"]) " +
			strings.ReplaceAll(strings.TrimSpace(darkRules), "\n", "\n:root:not([data-theme=\"light\"]) ") + "\n}\n")
		b.WriteString(":root[data-theme=\"dark\"] " + strings.ReplaceAll(strings.TrimSpace(darkRules), "\n", "\n:root[data-theme=\"dark\"] ") + "\n")
	}
	b.WriteString("@media print { details.callout > .callout-content { display: block; } }\n")
	return b.String()
}

// CalloutLatexHeader is the preamble a LaTeX export adds when the document
// has a callout: tcolorbox, and one colour per type.
func CalloutLatexHeader() string {
	var b strings.Builder
	b.WriteString("\\usepackage{xcolor}\n\\usepackage[most]{tcolorbox}\n")
	for _, t := range CalloutTypes {
		b.WriteString("\\definecolor{callout-" + t.Name + "}{HTML}{" + strings.ToUpper(strings.TrimPrefix(t.Color, "#")) + "}\n")
	}
	b.WriteString("\\newtcolorbox{orgcallout}[2]{enhanced,breakable,colback=#1!6!white,colframe=#1,coltitle=#1!75!black,colbacktitle=#1!14!white,boxrule=0.4pt,leftrule=3pt,arc=2pt,fonttitle=\\bfseries,title={#2}}\n")
	return b.String()
}

// LatexColorOf is the colour name (or a defined one) for \begin{orgcallout}.
func (c Callout) LatexColorOf() (name, define string) {
	if c.Color != "" && strings.HasPrefix(c.Color, "#") && (len(c.Color) == 7) {
		return "callout-custom-" + strings.ToLower(c.Color[1:]), "\\definecolor{callout-custom-" + strings.ToLower(c.Color[1:]) + "}{HTML}{" + strings.ToUpper(c.Color[1:]) + "}"
	}
	return "callout-" + c.TypeInfo().Name, ""
}

// HasCallouts walks a parsed document for a callout block.
func HasCallouts(nodes []org.Node) bool {
	for _, n := range nodes {
		switch v := n.(type) {
		case *org.Headline:
			if HasCallouts(v.Children) {
				return true
			}
		case org.Headline:
			if HasCallouts(v.Children) {
				return true
			}
		case *org.Block:
			if _, ok := CalloutOfBlock(*v); ok {
				return true
			}
			if HasCallouts(v.Children) {
				return true
			}
		case org.Block:
			if _, ok := CalloutOfBlock(v); ok {
				return true
			}
			if HasCallouts(v.Children) {
				return true
			}
		case org.NodeWithMeta:
			if HasCallouts([]org.Node{v.Node}) {
				return true
			}
		case org.NodeWithName:
			if HasCallouts([]org.Node{v.Node}) {
				return true
			}
		case org.List:
			if HasCallouts(v.Items) {
				return true
			}
		case org.ListItem:
			if HasCallouts(v.Children) {
				return true
			}
		case org.DescriptiveListItem:
			if HasCallouts(v.Details) {
				return true
			}
		case *org.Drawer:
			if HasCallouts(v.Children) {
				return true
			}
		}
	}
	return false
}

// CalloutTitleNodes parses a callout's title as org inline text, for a
// writer to write in its own way (emphasis, links). One paragraph's
// children, or nil.
func CalloutTitleNodes(title string) []org.Node {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	d := org.New().Silent().Parse(strings.NewReader(title+"\n"), "callout-title.org")
	if d.Error != nil || len(d.Nodes) == 0 {
		return nil
	}
	if p, ok := d.Nodes[0].(org.Paragraph); ok {
		return p.Children
	}
	return nil
}

// CalloutHTMLWith writes a callout with an html writer (the html exporter's
// or a slide deck's): its title as org inline text, its body as that writer
// writes it.
func CalloutHTMLWith(w *org.HTMLWriter, c Callout, b org.Block) string {
	title := html.EscapeString(c.ShownTitle())
	if nodes := CalloutTitleNodes(c.Title); nodes != nil {
		title = w.WriteNodesAsString(nodes...)
	}
	return CalloutHTML(c, title, w.WriteNodesAsString(b.Children...))
}

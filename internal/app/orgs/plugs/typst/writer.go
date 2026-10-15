package typst

import (
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
)

// Writer writes an org document as the body of a typst document.
//
// It implements go-org's Writer itself rather than borrowing the html writer
// the markdown and confluence exporters are built on: markdown contains html,
// so a node with no markdown form can fall through to it, but typst contains
// nothing of the sort and a fallthrough would print tags in the pdf. Every node
// has a method here, which is also why a node type go-org adds later fails to
// compile here rather than appearing as `<div>` in somebody's report.
type Writer struct {
	b   strings.Builder
	doc *org.Document
	set settings

	// Where an image link is pointed. The text export keeps the link as it is
	// written (relative to the org file, which is where the .typ is expected to
	// sit); the pdf build copies each picture beside the .typ and answers its
	// new name.
	ImagePath func(target string) string
	// The pictures the document uses, as written, in the order they appear.
	Images []string

	// Labels for everything a link can point at, by what the link says:
	// "h:<index>" a heading, "title:<title>" the heading a [[*Title]] names,
	// "id:<ID>", "cid:<CUSTOM_ID>", "name:<#+NAME>" and "target:<<<name>>>".
	labels map[string]string
	used   map[string]bool
	// Footnote definitions by name, and the label of a footnote already
	// written once (a second reference points at the first).
	footDefs map[string][]org.Node
	footSeen map[string]string

	// The last thing written was an embedded expression (#strong[...]), so a
	// `.` or `(` straight after it would be read as part of it.
	afterExpr bool
	// Inside a verse block, where a line break is a line break.
	verse bool
	// Whether any formula went to mitex, so the preamble imports it.
	usesMitex bool

	// `#+ATTR_TYPST:` and a `#+CAPTION:` go-org did not attach to anything,
	// waiting for the next picture, table or block.
	pendingAttrs   map[string]string
	pendingCaption []org.Node
	// The counter behind {{{n}}}.
	macroN map[string]int
}

func newWriter(doc *org.Document, set settings) *Writer {
	w := &Writer{
		doc:      doc,
		set:      set,
		labels:   map[string]string{},
		used:     map[string]bool{},
		footDefs: map[string][]org.Node{},
		footSeen: map[string]string{},
		macroN:   map[string]int{},
	}
	w.ImagePath = func(t string) string { return t }
	w.index(doc.Nodes)
	return w
}

// ---------------------------------------------------------------------------
// Output
// ---------------------------------------------------------------------------

func (w *Writer) String() string { return w.b.String() }

// raw writes typst as it is.
func (w *Writer) raw(s string) {
	if s == "" {
		return
	}
	w.b.WriteString(s)
	w.afterExpr = false
}

// expr writes an embedded expression - `#strong[...]` - and remembers that
// it did.
func (w *Writer) expr(s string) {
	w.b.WriteString(s)
	w.afterExpr = true
}

// text writes prose, escaped for where it lands.
func (w *Writer) text(s string) {
	if s == "" {
		return
	}
	out := escapeMarkup(s, w.atLineStart())
	if w.afterExpr && (out[0] == '.' || out[0] == '(') {
		out = `\` + out
	}
	w.raw(out)
}

// atLineStart says whether what comes next starts a line as far as typst's
// parser is concerned: nothing but spaces since a newline, or since the `[`
// that opened a content block.
func (w *Writer) atLineStart() bool {
	s := w.b.String()
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ' ', '\t':
			continue
		case '\n':
			return true
		case '[':
			// An escaped \[ is text, not the start of a block.
			return i == 0 || s[i-1] != '\\'
		default:
			return false
		}
	}
	return true
}

// blankLine ends the paragraph or block before whatever comes next.
func (w *Writer) blankLine() {
	s := w.b.String()
	if s == "" || strings.HasSuffix(s, "\n\n") || strings.HasSuffix(s, "[") {
		return
	}
	if strings.HasSuffix(s, "\n") {
		w.raw("\n")
	} else {
		w.raw("\n\n")
	}
}

// inner is nodes written on their own, for a content block or an argument.
func (w *Writer) inner(nodes ...org.Node) string {
	return strings.TrimSpace(w.WriteNodesAsString(nodes...))
}

func (w *Writer) WriteNodesAsString(nodes ...org.Node) string {
	saved, after := w.b, w.afterExpr
	w.b = strings.Builder{}
	// A fresh builder is the start of a line to atLineStart, which is what a
	// content block's first character is.
	org.WriteNodes(w, nodes...)
	out := w.b.String()
	w.b, w.afterExpr = saved, after
	return out
}

func (w *Writer) WriterWithExtensions() org.Writer { return w }
func (w *Writer) Before(*org.Document)             {}
func (w *Writer) After(*org.Document)              {}
func (w *Writer) NodeIdx(int)                      {}
func (w *Writer) ResetLineBreak()                  {}

// ---------------------------------------------------------------------------
// The first pass: what can be linked to, and the footnotes
// ---------------------------------------------------------------------------

func (w *Writer) labelFor(key, want string) string {
	if l, ok := w.labels[key]; ok {
		return l
	}
	base := labelText(want)
	if base == "" {
		base = "label"
	}
	l, n := base, 2
	for w.used[l] {
		l = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	w.used[l] = true
	w.labels[key] = l
	return l
}

func (w *Writer) index(nodes []org.Node) {
	walk(nodes, func(n org.Node) {
		switch n := n.(type) {
		case org.Headline:
			w.indexHeadline(&n)
		case *org.Headline:
			w.indexHeadline(n)
		case org.NodeWithName:
			w.labelFor("name:"+n.Name, n.Name)
		case *org.NodeWithName:
			w.labelFor("name:"+n.Name, n.Name)
		case org.Target:
			w.labelFor("target:"+n.Name, "target-"+n.Name)
		case org.RadioTarget:
			w.labelFor("target:"+n.Name, "target-"+n.Name)
		case org.FootnoteDefinition:
			if _, ok := w.footDefs[n.Name]; !ok {
				w.footDefs[n.Name] = n.Children
			}
		}
	})
}

func (w *Writer) indexHeadline(h *org.Headline) {
	title := org.String(h.Title...)
	want := "sec-" + plainText(h.Title)
	if h.Properties != nil {
		if cid, ok := h.Properties.Get("CUSTOM_ID"); ok && strings.TrimSpace(cid) != "" {
			want = cid
		}
	}
	l := w.labelFor(fmt.Sprintf("h:%d", h.Index), want)
	if h.Properties != nil {
		if cid, ok := h.Properties.Get("CUSTOM_ID"); ok && cid != "" {
			w.labels["cid:"+cid] = l
		}
		if id, ok := h.Properties.Get("ID"); ok && id != "" {
			w.labels["id:"+id] = l
		}
	}
	for _, k := range []string{"title:" + title, "title~" + loose(title)} {
		if _, ok := w.labels[k]; !ok {
			w.labels[k] = l
		}
	}
}

func loose(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// plainText is what a reader sees of some nodes: a link is its description
// (or its target when it has none), emphasis its words. A label made from a
// heading that is a link should not be made from its url.
func plainText(nodes []org.Node) string {
	b := strings.Builder{}
	for _, n := range nodes {
		switch n := n.(type) {
		case org.Text:
			b.WriteString(n.Content)
		case org.RegularLink:
			if n.Description != nil {
				b.WriteString(plainText(n.Description))
			} else {
				b.WriteString(n.URL)
			}
		case org.Emphasis:
			b.WriteString(plainText(n.Content))
		case org.Timestamp, org.StatisticToken, org.FootnoteLink:
		default:
			b.WriteString(org.String(n))
		}
	}
	return b.String()
}

// walk calls fn on every node, depth first.
func walk(nodes []org.Node, fn func(org.Node)) {
	for _, n := range nodes {
		if n == nil {
			continue
		}
		fn(n)
		switch n := n.(type) {
		case org.Headline:
			walk(n.Title, fn)
			walk(n.Children, fn)
		case *org.Headline:
			walk(n.Title, fn)
			walk(n.Children, fn)
		case org.Block:
			walk(n.Children, fn)
		case *org.Block:
			walk(n.Children, fn)
		case org.List:
			walk(n.Items, fn)
		case org.ListItem:
			walk(n.Children, fn)
		case org.DescriptiveListItem:
			walk(n.Term, fn)
			walk(n.Details, fn)
		case org.Paragraph:
			walk(n.Children, fn)
		case org.Drawer:
			walk(n.Children, fn)
		case *org.Drawer:
			walk(n.Children, fn)
		case org.NodeWithMeta:
			walk([]org.Node{n.Node}, fn)
		case org.NodeWithName:
			walk([]org.Node{n.Node}, fn)
		case *org.NodeWithName:
			walk([]org.Node{n.Node}, fn)
		case org.Emphasis:
			walk(n.Content, fn)
		case org.FootnoteDefinition:
			walk(n.Children, fn)
		case org.FootnoteLink:
			if n.Definition != nil {
				walk([]org.Node{*n.Definition}, fn)
			}
		case org.RegularLink:
			walk(n.Description, fn)
		case org.RadioTarget:
			walk(n.Children, fn)
		case org.IncludedContent:
			walk(n.Nodes, fn)
		case org.Table:
			walkTable(&n, fn)
		case *org.Table:
			walkTable(n, fn)
		}
	}
}

func walkTable(t *org.Table, fn func(org.Node)) {
	for _, r := range t.Rows {
		for _, c := range r.Columns {
			walk(c.Children, fn)
		}
	}
}

// ---------------------------------------------------------------------------
// Headlines
// ---------------------------------------------------------------------------

func (w *Writer) option(key string) string { return option(w.doc, key) }

// The export options org assumes when #+OPTIONS: does not say.
var optionDefaults = map[string]string{
	"toc": "t", "num": "t", "H": "3", "todo": "t", "pri": "t", "tags": "t",
	"p": "nil", "c": "nil", "prop": "nil", "d": "t", "f": "t", "<": "t",
	"e": "t", "arch": "headline",
}

// option is an export option as #+OPTIONS: (or the configured default) sets
// it, else org's own default. go-org's GetOption answers "nil" for anything
// not set, which would turn numbering and the contents off for every file
// that does not mention them.
func option(doc *org.Document, key string) string {
	get := func(settings map[string]string) string {
		for _, field := range strings.Fields(settings["OPTIONS"]) {
			if strings.HasPrefix(field, key+":") {
				return field[len(key)+1:]
			}
		}
		return ""
	}
	if v := get(doc.BufferSettings); v != "" {
		return v
	}
	if v := get(doc.DefaultSettings); v != "" {
		return v
	}
	return optionDefaults[key]
}

func (w *Writer) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.doc) || h.IsComment {
		return
	}
	arch := w.option("arch")
	if h.IsArchived() {
		if arch == "nil" {
			return
		} else if arch != "t" {
			h.Children = nil
		}
	}
	w.blankLine()
	title := strings.TrimSpace(w.WriteNodesAsString(h.Title...))
	parts := []string{}
	if h.Status != "" && w.option("todo") != "nil" {
		parts = append(parts, fmt.Sprintf("#org-todo(%s, done: %t);", str(h.Status), h.IsDone()))
	}
	if h.Priority != "" && w.option("pri") != "nil" {
		parts = append(parts, fmt.Sprintf("#org-priority(%s);", str(h.Priority)))
	}
	parts = append(parts, title)
	if len(h.Tags) > 0 && w.option("tags") != "nil" {
		tags := []string{}
		for _, t := range h.Tags {
			tags = append(tags, str(t))
		}
		parts = append(parts, fmt.Sprintf("#org-tags((%s,));", strings.Join(tags, ", ")))
	}
	line := strings.Join(parts, " ")
	label := w.labels[fmt.Sprintf("h:%d", h.Index)]

	// Past H: (org's headline levels) a heading is a paragraph, as org's own
	// latex export makes it a list item; a strong line keeps it readable.
	if maxLvl, err := strconv.Atoi(w.option("H")); err == nil && maxLvl > 0 && h.Lvl > maxLvl {
		w.raw(fmt.Sprintf("#strong[%s] <%s>\n\n", line, label))
	} else {
		w.raw(fmt.Sprintf("%s %s <%s>\n\n", strings.Repeat("=", h.Lvl), line, label))
	}
	org.WriteNodes(w, h.Children...)
	w.blankLine()
}

// SCHEDULED, DEADLINE and CLOSED: only with p:t, as in org's own exports.
func (w *Writer) WriteSDC(s org.SDC) {
	if w.option("p") != "t" || s.Date == nil {
		return
	}
	name := map[org.DateType]string{org.Scheduled: "SCHEDULED", org.Deadline: "DEADLINE", org.Closed: "CLOSED"}[s.DateType]
	w.blankLine()
	w.expr(fmt.Sprintf("#org-planning(%s, %s);", str(name), str(s.Date.ToString())))
	w.raw("\n\n")
}

// Clock lines: only with c:t.
func (w *Writer) WriteClock(c org.Clock) {
	if w.option("c") != "t" {
		return
	}
	w.blankLine()
	w.expr(fmt.Sprintf("#org-planning(%s, %s);", str("CLOCK"), str(org.String(c))))
	w.raw("\n\n")
}

// Property drawers: only with prop:t.
func (w *Writer) WritePropertyDrawer(d org.PropertyDrawer) {
	if w.option("prop") != "t" || len(d.Properties) == 0 {
		return
	}
	w.blankLine()
	cells := []string{}
	for _, p := range d.Properties {
		if len(p) < 2 {
			continue
		}
		cells = append(cells, fmt.Sprintf("[%s], [%s]", escapeMarkup(p[0], true), escapeMarkup(p[1], true)))
	}
	w.expr(fmt.Sprintf("#org-properties(%s);", strings.Join(cells, ", ")))
	w.raw("\n\n")
}

func (w *Writer) WriteDrawer(d org.Drawer) {
	// A LOGBOOK is org's record of what happened, not the document; d:nil
	// leaves every drawer out.
	if strings.EqualFold(d.Name, "LOGBOOK") || w.option("d") == "nil" {
		return
	}
	org.WriteNodes(w, d.Children...)
}

// ---------------------------------------------------------------------------
// Keywords, comments, includes
// ---------------------------------------------------------------------------

func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	fields := strings.Fields(s)
	key := ""
	for _, f := range fields {
		if strings.HasPrefix(f, ":") && len(f) > 1 {
			key = strings.ToLower(f[1:])
			out[key] = ""
			continue
		}
		if key != "" {
			if out[key] != "" {
				out[key] += " "
			}
			out[key] += f
		}
	}
	return out
}

func (w *Writer) WriteKeyword(k org.Keyword) {
	switch k.Key {
	case "TYPST":
		w.blankLine()
		w.raw(k.Value + "\n")
	case "ATTR_TYPST":
		w.pendingAttrs = parseAttrs(k.Value)
	case "CAPTION":
		// go-org attaches a caption to what follows only when nothing but
		// CAPTION, ATTR_HTML, ATTR_LATEX and ENV come between them, so a
		// `#+NAME:` or `#+ATTR_TYPST:` after the caption leaves it here.
		w.pendingCaption = inlineOf(w.doc, k.Value)
	case "TOC":
		if m := regexp.MustCompile(`headlines\s+(\d+)`).FindStringSubmatch(k.Value); m != nil {
			w.blankLine()
			w.raw(fmt.Sprintf("#outline(depth: %s)\n\n", m[1]))
		} else if strings.HasPrefix(strings.TrimSpace(k.Value), "headlines") {
			w.blankLine()
			w.raw("#outline()\n\n")
		}
	}
}

// inlineOf parses a keyword's value as org inline text.
func inlineOf(doc *org.Document, s string) []org.Node {
	d := doc.Parse(strings.NewReader(s), doc.Path)
	if len(d.Nodes) == 1 {
		if p, ok := d.Nodes[0].(org.Paragraph); ok {
			return p.Children
		}
	}
	return d.Nodes
}

func (w *Writer) WriteComment(org.Comment) {}

func (w *Writer) WriteInclude(i org.Include) {
	if i.Resolve != nil {
		org.WriteNodes(w, i.Resolve())
	}
}

func (w *Writer) WriteBabelCall(c org.BabelCall) {
	headers := c.InsideHeader + " " + c.EndHeader
	if c.Result != nil && !strings.Contains(headers, ":exports none") && !strings.Contains(headers, ":exports code") {
		org.WriteNodes(w, c.Result)
	}
}

// ---------------------------------------------------------------------------
// Named and captioned things: figures
// ---------------------------------------------------------------------------

func (w *Writer) WriteNodeWithMeta(n org.NodeWithMeta) {
	var caption []org.Node
	if len(n.Meta.Caption) > 0 {
		caption = n.Meta.Caption[len(n.Meta.Caption)-1]
	}
	attrs := map[string]string{}
	for _, a := range n.Meta.LatexAttributes {
		for i := 0; i+1 < len(a); i += 2 {
			attrs[strings.ToLower(strings.TrimPrefix(a[i], ":"))] = a[i+1]
		}
	}
	w.figure(n.Node, caption, "", attrs)
}

func (w *Writer) WriteNodeWithName(n org.NodeWithName) {
	var caption []org.Node
	node := n.Node
	attrs := map[string]string{}
	if m, ok := node.(org.NodeWithMeta); ok {
		if len(m.Meta.Caption) > 0 {
			caption = m.Meta.Caption[len(m.Meta.Caption)-1]
		}
		for _, a := range m.Meta.LatexAttributes {
			for i := 0; i+1 < len(a); i += 2 {
				attrs[strings.ToLower(strings.TrimPrefix(a[i], ":"))] = a[i+1]
			}
		}
		node = m.Node
	}
	w.figure(node, caption, w.labels["name:"+n.Name], attrs)
}

// figure writes a picture, table or source block, as a typst figure when it
// has a caption or a name to be referred to by, and as itself otherwise.
func (w *Writer) figure(node org.Node, caption []org.Node, label string, attrs map[string]string) {
	if caption == nil && w.pendingCaption != nil {
		caption = w.pendingCaption
	}
	w.pendingCaption = nil
	for k, v := range w.pendingAttrs {
		attrs[k] = v
	}
	w.pendingAttrs = nil

	var body string
	switch n := node.(type) {
	case org.Paragraph:
		if img, ok := w.loneImage(n); ok {
			body = w.imageCall(img, attrs)
		}
	case org.Table:
		body = w.tableCall(n, attrs)
	case *org.Table:
		body = w.tableCall(*n, attrs)
	case org.Block:
		if strings.EqualFold(n.Name, "SRC") && n.Parameters != nil {
			if code, ok := w.srcCall(n); ok {
				body = code
			}
		}
	}
	if body == "" {
		// Not something a figure holds: written as it is, with its label on
		// an empty anchor so a link to the name still lands here.
		if label != "" {
			w.blankLine()
			w.expr(fmt.Sprintf("#metadata(none)<%s>", label))
			w.raw("\n")
		}
		org.WriteNodes(w, node)
		return
	}
	w.blankLine()
	if caption == nil && label == "" {
		if _, isImg := node.(org.Paragraph); isImg {
			w.expr("#align(center, " + body + ")")
		} else {
			w.expr("#" + body)
		}
	} else {
		args := body
		if caption != nil {
			args += ", caption: [" + w.inner(caption...) + "]"
		}
		w.expr("#figure(" + args + ")")
		if label != "" {
			w.raw(fmt.Sprintf(" <%s>", label))
		}
	}
	w.raw("\n\n")
	// A source block's results come after it, outside the figure.
	if b, ok := node.(org.Block); ok && b.Result != nil {
		w.writeResult(b)
	}
}

// ---------------------------------------------------------------------------
// Paragraphs and text
// ---------------------------------------------------------------------------

func (w *Writer) WriteParagraph(p org.Paragraph) {
	if _, ok := w.loneImage(p); ok {
		w.figure(p, nil, "", map[string]string{})
		return
	}
	w.blankLine()
	org.WriteNodes(w, p.Children...)
	w.raw("\n\n")
}

func (w *Writer) WriteText(t org.Text) {
	s := t.Content
	if !t.IsRaw && w.option("e") != "nil" {
		// Org's entities (\alpha, \rarr, --, ...) as characters. go-org knows
		// the table and writes it as html, which is undone here.
		hw := org.NewHTMLWriter()
		hw.Document = w.doc
		hw.WriteText(org.Text{Content: s})
		s = html.UnescapeString(hw.String())
	}
	w.text(s)
}

func (w *Writer) WriteLineBreak(l org.LineBreak) {
	if w.verse {
		w.raw(strings.Repeat(" \\\n", l.Count))
		return
	}
	w.raw(strings.Repeat("\n", l.Count))
}

func (w *Writer) WriteExplicitLineBreak(org.ExplicitLineBreak) { w.raw(" \\\n") }

var emphasisFns = map[string]string{
	"*": "strong", "/": "emph", "_": "underline", "+": "strike",
	"_{}": "sub", "^{}": "super",
}

func (w *Writer) WriteEmphasis(e org.Emphasis) {
	if e.Kind == "=" || e.Kind == "~" {
		w.expr("#raw(" + str(org.String(e.Content...)) + ")")
		return
	}
	fn, ok := emphasisFns[e.Kind]
	if !ok {
		org.WriteNodes(w, e.Content...)
		return
	}
	w.expr("#" + fn + "[" + w.inner(e.Content...) + "]")
}

func (w *Writer) WriteStatisticToken(s org.StatisticToken) {
	w.expr("#org-stat(" + str(s.Content) + ")")
}

func (w *Writer) WriteTimestamp(t org.Timestamp) {
	if w.option("<") == "nil" || t.Time == nil {
		return
	}
	w.expr("#org-timestamp(" + str(t.Time.ToString()) + ")")
}

func (w *Writer) WriteTarget(t org.Target) {
	if l := w.labels["target:"+t.Name]; l != "" {
		w.expr(fmt.Sprintf("#metadata(none)<%s>", l))
	}
}

func (w *Writer) WriteRadioTarget(t org.RadioTarget) {
	org.WriteNodes(w, t.Children...)
	if l := w.labels["target:"+t.Name]; l != "" {
		w.expr(fmt.Sprintf("#metadata(none)<%s>", l))
	}
}

func (w *Writer) WriteMacro(m org.Macro) {
	arg := func(i int) string {
		if i < len(m.Parameters) {
			return strings.TrimSpace(m.Parameters[i])
		}
		return ""
	}
	macro, defined := w.doc.Macros[m.Name]
	if !defined {
		switch strings.ToLower(m.Name) {
		case "title", "author", "email", "date":
			macro = w.doc.Get(strings.ToUpper(m.Name))
		case "keyword":
			macro = w.doc.Get(strings.ToUpper(arg(0)))
		case "results":
			macro = arg(0)
		case "n":
			name := arg(0)
			if arg(1) == "-" {
				w.macroN[name] = 0
			} else if v, err := strconv.Atoi(arg(1)); err == nil {
				w.macroN[name] = v - 1
			}
			w.macroN[name]++
			w.text(strconv.Itoa(w.macroN[name]))
			return
		}
	}
	if macro == "" {
		return
	}
	for i, p := range m.Parameters {
		macro = strings.ReplaceAll(macro, fmt.Sprintf("$%d", i+1), p)
	}
	org.WriteNodes(w, inlineOf(w.doc, macro)...)
}

// ---------------------------------------------------------------------------
// Footnotes
// ---------------------------------------------------------------------------

func (w *Writer) WriteFootnoteDefinition(org.FootnoteDefinition) {}

func (w *Writer) WriteFootnoteLink(l org.FootnoteLink) {
	if w.option("f") == "nil" {
		return
	}
	if l.Name != "" {
		if label, ok := w.footSeen[l.Name]; ok {
			w.expr(fmt.Sprintf("#footnote(<%s>)", label))
			return
		}
	}
	var body []org.Node
	if l.Definition != nil {
		body = l.Definition.Children
	} else {
		body = w.footDefs[l.Name]
	}
	// A definition is usually a paragraph; inside a footnote it is the text.
	if len(body) == 1 {
		if p, ok := body[0].(org.Paragraph); ok {
			body = p.Children
		}
	}
	w.expr("#footnote[" + w.inner(body...) + "]")
	if l.Name != "" {
		label := w.labelFor("fn:"+l.Name, "fn-"+l.Name)
		w.footSeen[l.Name] = label
		w.expr(fmt.Sprintf("<%s>", label))
	}
}

// ---------------------------------------------------------------------------
// Links and pictures
// ---------------------------------------------------------------------------

var imageExtRe = regexp.MustCompile(`(?i)[.](png|gif|jpe?g|svg|webp)$`)

func isImage(target string) bool {
	if i := strings.IndexAny(target, "?#"); i >= 0 {
		target = target[:i]
	}
	return imageExtRe.MatchString(target)
}

// The file a file: (or protocol-less) link names, without `file:` and any
// ::search.
func linkPath(l org.RegularLink) string {
	u := l.URL
	if l.Protocol == "file" {
		u = strings.TrimPrefix(strings.TrimPrefix(u, "file:"), "//")
	}
	if i := strings.Index(u, "::"); i >= 0 {
		u = u[:i]
	}
	return u
}

func isLocal(l org.RegularLink) bool { return l.Protocol == "file" || l.Protocol == "" }

// loneImage is the picture a paragraph holds when that is all it holds.
func (w *Writer) loneImage(p org.Paragraph) (org.RegularLink, bool) {
	var found *org.RegularLink
	for _, n := range p.Children {
		switch n := n.(type) {
		case org.RegularLink:
			if found != nil || n.Description != nil || !isLocal(n) || !isImage(linkPath(n)) {
				return org.RegularLink{}, false
			}
			l := n
			found = &l
		case org.Text:
			if strings.TrimSpace(n.Content) != "" {
				return org.RegularLink{}, false
			}
		case org.LineBreak:
		default:
			return org.RegularLink{}, false
		}
	}
	if found == nil {
		return org.RegularLink{}, false
	}
	return *found, true
}

// width turns the sizes org files carry into one typst reads: 60%, 5cm, and
// LaTeX's 0.6\textwidth.
var texWidthRe = regexp.MustCompile(`^([0-9.]+)\s*\\(text|line|column)width$`)
var typstLenRe = regexp.MustCompile(`^[0-9.]+(%|pt|mm|cm|in|em|fr)$`)

func width(s string) string {
	s = strings.TrimSpace(s)
	if m := texWidthRe.FindStringSubmatch(s); m != nil {
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			return strconv.FormatFloat(f*100, 'f', -1, 64) + "%"
		}
	}
	if typstLenRe.MatchString(s) {
		return s
	}
	return ""
}

func (w *Writer) imageCall(l org.RegularLink, attrs map[string]string) string {
	target := linkPath(l)
	w.Images = append(w.Images, target)
	args := []string{str(w.ImagePath(target))}
	if wd := width(attrs["width"]); wd != "" {
		args = append(args, "width: "+wd)
	} else if w.set.imageWidth != "" {
		args = append(args, "width: "+w.set.imageWidth)
	}
	if h := width(attrs["height"]); h != "" {
		args = append(args, "height: "+h)
	}
	if alt := attrs["alt"]; alt != "" {
		args = append(args, "alt: "+str(alt))
	}
	return "image(" + strings.Join(args, ", ") + ")"
}

func (w *Writer) WriteRegularLink(l org.RegularLink) {
	desc := ""
	if l.Description != nil {
		desc = w.inner(l.Description...)
	}
	link := func(dest, fallback string) {
		if desc == "" {
			desc = escapeMarkup(fallback, false)
		}
		w.expr("#link(" + dest + ")[" + desc + "]")
	}
	// #+LINK: abbreviations.
	url := l.URL
	if prefix := w.doc.Links[l.Protocol]; prefix != "" {
		tag := strings.TrimPrefix(l.URL, l.Protocol+":")
		if strings.Contains(prefix, "%s") {
			url = strings.ReplaceAll(prefix, "%s", tag)
		} else {
			url = prefix + tag
		}
		w.expr("#link(" + str(url) + ")" + func() string {
			if desc != "" {
				return "[" + desc + "]"
			}
			return ""
		}())
		return
	}

	switch {
	case l.Protocol == "" && strings.HasPrefix(url, "*"):
		title := strings.TrimSpace(url[1:])
		if lab := w.firstLabel("title:"+title, "title~"+loose(title)); lab != "" {
			link("<"+lab+">", title)
			return
		}
		w.text(title)
		return
	case l.Protocol == "" && strings.HasPrefix(url, "#"):
		if lab := w.labels["cid:"+url[1:]]; lab != "" {
			link("<"+lab+">", url[1:])
			return
		}
	case l.Protocol == "id":
		id := strings.TrimPrefix(url, "id:")
		if lab := w.labels["id:"+id]; lab != "" {
			link("<"+lab+">", id)
			return
		}
		// A heading in another file: nothing to point at in this pdf.
		if desc != "" {
			w.raw(desc)
		} else {
			w.text(id)
		}
		return
	case l.Protocol == "" && !strings.ContainsAny(url, "/.") && w.firstLabel("target:"+url, "name:"+url) != "":
		link("<"+w.firstLabel("target:"+url, "name:"+url)+">", url)
		return
	case l.Protocol == "attachment":
		if desc != "" {
			w.raw(desc)
		} else {
			w.text(strings.TrimPrefix(url, "attachment:"))
		}
		return
	}

	if isLocal(l) {
		path := linkPath(l)
		if isImage(path) && l.Description == nil {
			w.expr("#" + w.imageCall(l, w.takeAttrs()))
			return
		}
		// Another org file is written beside this one as a pdf, as the html
		// exporter points at the .html.
		if strings.EqualFold(filepath.Ext(path), ".org") {
			path = strings.TrimSuffix(path, filepath.Ext(path)) + ".pdf"
		}
		link(str(path), path)
		return
	}
	if desc == "" {
		w.expr("#link(" + str(url) + ")")
		return
	}
	link(str(url), url)
}

// takeAttrs hands over the waiting #+ATTR_TYPST: for an inline picture.
func (w *Writer) takeAttrs() map[string]string {
	a := w.pendingAttrs
	w.pendingAttrs = nil
	if a == nil {
		a = map[string]string{}
	}
	return a
}

func (w *Writer) firstLabel(keys ...string) string {
	for _, k := range keys {
		if l := w.labels[k]; l != "" {
			return l
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Maths
// ---------------------------------------------------------------------------

func (w *Writer) WriteLatexFragment(l org.LatexFragment) {
	tex := org.String(l.Content...)
	display := l.OpeningPair == "$$" || l.OpeningPair == `\[` || strings.HasPrefix(l.OpeningPair, `\begin`)
	if strings.HasPrefix(l.OpeningPair, `\begin`) {
		tex = l.OpeningPair + tex + l.ClosingPair
	}
	if w.set.math == "typst" && !strings.HasPrefix(l.OpeningPair, `\begin`) {
		if t, err := texToTypst(tex); err == nil {
			if display {
				w.blankLine()
				w.raw("$ " + t + " $\n\n")
			} else {
				w.expr("$" + t + "$")
			}
			return
		}
	}
	w.usesMitex = true
	if display {
		w.blankLine()
		w.expr("#mitex(" + str(tex) + ")")
		w.raw("\n\n")
		return
	}
	w.expr("#mi(" + str(tex) + ")")
}

// ---------------------------------------------------------------------------
// Blocks
// ---------------------------------------------------------------------------

func rawContent(children []org.Node) string {
	return strings.TrimRight(org.String(children...), "\n")
}

// srcCall is a source block as a typst raw block expression, or false when
// the block says its code is not exported.
func (w *Writer) srcCall(b org.Block) (string, bool) {
	params := b.ParameterMap()
	if params[":exports"] == "results" || params[":exports"] == "none" {
		return "", false
	}
	lang := ""
	if len(b.Parameters) > 0 {
		lang = langWord(strings.ToLower(b.Parameters[0]))
	}
	code := rawContent(b.Children)
	if params[":noweb"] == "strip-export" {
		code = regexp.MustCompile(`<<[^>]+>>`).ReplaceAllString(code, "")
	}
	args := []string{"block: true"}
	if lang != "" {
		args = append(args, "lang: "+str(lang))
	}
	args = append(args, str(code))
	return "raw(" + strings.Join(args, ", ") + ")", true
}

func (w *Writer) writeResult(b org.Block) {
	params := b.ParameterMap()
	if b.Result == nil || params[":exports"] == "code" || params[":exports"] == "none" {
		return
	}
	org.WriteNodes(w, b.Result)
}

func (w *Writer) WriteResult(r org.Result) { org.WriteNodes(w, r.Node) }

func (w *Writer) WriteBlock(b org.Block) {
	name := strings.ToUpper(b.Name)
	switch name {
	case "SRC":
		if w.pendingCaption != nil || w.pendingAttrs != nil {
			w.figure(b, nil, "", map[string]string{})
			return
		}
		if code, ok := w.srcCall(b); ok {
			// A fence reads better in the .typ than raw("..."), and is the
			// same thing to typst.
			lang := ""
			if len(b.Parameters) > 0 {
				lang = langWord(strings.ToLower(b.Parameters[0]))
			}
			src := rawContent(b.Children)
			if strings.Contains(code, `<<`) && b.ParameterMap()[":noweb"] == "strip-export" {
				src = regexp.MustCompile(`<<[^>]+>>`).ReplaceAllString(src, "")
			}
			f := fence(src)
			w.blankLine()
			// A first line typst would take for the language tag, or one that
			// starts with backticks, goes through raw() instead.
			if lang == "" && strings.TrimSpace(src) != "" && !strings.HasPrefix(src, "\n") && !strings.HasPrefix(src, " ") {
				w.expr("#" + code)
			} else {
				w.raw(f + lang + "\n" + src + "\n" + f)
			}
			w.raw("\n\n")
		}
		w.writeResult(b)
	case "EXAMPLE":
		w.blankLine()
		w.expr("#org-example(" + str(rawContent(b.Children)) + ")")
		w.raw("\n\n")
	case "EXPORT":
		if len(b.Parameters) > 0 && strings.EqualFold(b.Parameters[0], "typst") {
			w.blankLine()
			w.raw(rawContent(b.Children) + "\n\n")
		}
	case "COMMENT":
	case "QUOTE":
		w.blankLine()
		w.expr("#quote(block: true)[" + w.inner(b.Children...) + "]")
		w.raw("\n\n")
	case "CENTER":
		w.blankLine()
		w.expr("#align(center)[" + w.inner(b.Children...) + "]")
		w.raw("\n\n")
	case "VERSE":
		w.verse = true
		body := w.inner(b.Children...)
		w.verse = false
		w.blankLine()
		w.expr("#org-verse[" + body + "]")
		w.raw("\n\n")
	default:
		w.blankLine()
		w.expr(fmt.Sprintf("#org-special(%s)[%s]", str(strings.ToLower(b.Name)), w.inner(b.Children...)))
		w.raw("\n\n")
	}
}

func (w *Writer) WriteInlineBlock(b org.InlineBlock) {
	content := org.String(b.Children...)
	switch b.Name {
	case "src":
		lang := ""
		if len(b.Parameters) > 0 {
			lang = langWord(strings.ToLower(b.Parameters[0]))
		}
		if lang != "" {
			w.expr(fmt.Sprintf("#raw(lang: %s, %s)", str(lang), str(content)))
		} else {
			w.expr("#raw(" + str(content) + ")")
		}
	case "export":
		if len(b.Parameters) > 0 && strings.EqualFold(b.Parameters[0], "typst") {
			w.raw(content)
		}
	}
}

func (w *Writer) WriteExample(e org.Example) {
	lines := []string{}
	for _, n := range e.Children {
		lines = append(lines, org.String(n))
	}
	w.blankLine()
	w.expr("#org-example(" + str(strings.Join(lines, "\n")) + ")")
	w.raw("\n\n")
}

func (w *Writer) WriteHorizontalRule(org.HorizontalRule) {
	w.blankLine()
	w.expr("#org-hr()")
	w.raw("\n\n")
}

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

// Lists are written as calls - #list([...], [...]) - rather than as typst's
// `- item` markup, so that an item holding paragraphs, code and lists of its
// own needs no indentation bookkeeping: a content block holds anything.
func (w *Writer) WriteList(l org.List) {
	items := []string{}
	switch l.Kind {
	case "descriptive":
		for _, n := range l.Items {
			if di, ok := n.(org.DescriptiveListItem); ok {
				items = append(items, fmt.Sprintf("terms.item[%s][%s]", w.inner(di.Term...), w.itemBody(di.Status, di.Details)))
			}
		}
		w.blankLine()
		w.expr("#terms(\n  " + strings.Join(items, ",\n  ") + ",\n)")
		w.raw("\n\n")
		return
	case "ordered":
		numbering, start := "1.", 0
		for i, n := range l.Items {
			li, ok := n.(org.ListItem)
			if !ok {
				continue
			}
			if i == 0 {
				numbering, start = orderedStyle(li.Bullet)
			}
			body := "[" + w.itemBody(li.Status, li.Children) + "]"
			if v, err := strconv.Atoi(li.Value); err == nil {
				body = fmt.Sprintf("enum.item(%d)%s", v, body)
			}
			items = append(items, body)
		}
		args := []string{}
		if numbering != "1." {
			args = append(args, "numbering: "+str(numbering))
		}
		if start > 1 {
			args = append(args, fmt.Sprintf("start: %d", start))
		}
		w.blankLine()
		w.expr("#enum(\n  " + strings.Join(append(args, items...), ",\n  ") + ",\n)")
		w.raw("\n\n")
		return
	}
	for _, n := range l.Items {
		if li, ok := n.(org.ListItem); ok {
			items = append(items, "["+w.itemBody(li.Status, li.Children)+"]")
		}
	}
	w.blankLine()
	w.expr("#list(\n  " + strings.Join(items, ",\n  ") + ",\n)")
	w.raw("\n\n")
}

// org's `a.` or `3)` as typst's enum numbering, and the number it starts at.
func orderedStyle(bullet string) (string, int) {
	b := strings.TrimSpace(bullet)
	if b == "" {
		return "1.", 0
	}
	suffix := b[len(b)-1:]
	if suffix != "." && suffix != ")" {
		suffix = "."
	}
	body := strings.TrimRight(b, ".)")
	if n, err := strconv.Atoi(body); err == nil {
		return "1" + suffix, n
	}
	if len(body) == 1 {
		r := body[0]
		switch {
		case r >= 'a' && r <= 'z':
			return "a" + suffix, int(r-'a') + 1
		case r >= 'A' && r <= 'Z':
			return "A" + suffix, int(r-'A') + 1
		}
	}
	return "1" + suffix, 0
}

func (w *Writer) itemBody(status string, children []org.Node) string {
	body := w.inner(children...)
	if status != "" {
		box := map[string]string{"X": "true", " ": "false", "-": "none"}[status]
		if box == "" {
			box = "false"
		}
		body = fmt.Sprintf("#org-check(%s); ", box) + body
	}
	return body
}

// Written only through WriteList, which reads the items itself.
func (w *Writer) WriteListItem(li org.ListItem) { org.WriteNodes(w, li.Children...) }
func (w *Writer) WriteDescriptiveListItem(di org.DescriptiveListItem) {
	org.WriteNodes(w, di.Details...)
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

func (w *Writer) WriteTable(t org.Table) {
	if w.pendingCaption != nil || w.pendingAttrs != nil {
		w.figure(t, nil, "", map[string]string{})
		return
	}
	w.blankLine()
	w.expr("#" + w.tableCall(t, map[string]string{}))
	w.raw("\n\n")
}

var numericRe = regexp.MustCompile(`^[-+]?[$€£]?[0-9][0-9,]*(\.[0-9]+)?%?$`)

// tableCall is an org table as a typst table() expression.
//
// The rows above the first |---+---| rule are the header when rows come after
// it - org's own reading - and later rules are horizontal lines. A column says
// how it is aligned with a <l> <c> <r> cookie; one with none is right aligned
// when most of its cells are numbers, as org's html export does.
func (w *Writer) tableCall(t org.Table, attrs map[string]string) string {
	type row struct {
		cells []string
		rule  bool
	}
	rows := []row{}
	cols := 0
	numeric, filled := []int{}, []int{}
	for _, r := range t.Rows {
		if len(r.Columns) == 0 {
			rows = append(rows, row{rule: true})
			continue
		}
		if r.IsSpecial {
			continue
		}
		cells := []string{}
		for i, c := range r.Columns {
			text := w.inner(c.Children...)
			cells = append(cells, "["+text+"]")
			for len(numeric) <= i {
				numeric, filled = append(numeric, 0), append(filled, 0)
			}
			if plain := strings.TrimSpace(org.String(c.Children...)); plain != "" {
				filled[i]++
				if numericRe.MatchString(plain) {
					numeric[i]++
				}
			}
		}
		if len(cells) > cols {
			cols = len(cells)
		}
		rows = append(rows, row{cells: cells})
	}
	if cols == 0 {
		return "table(columns: 1)"
	}
	// The header: the rows before the first rule, if any data follows it.
	headerEnd := -1
	for i, r := range rows {
		if r.rule {
			for _, later := range rows[i+1:] {
				if !later.rule {
					headerEnd = i
					break
				}
			}
			break
		}
	}
	aligns := []string{}
	for i := 0; i < cols; i++ {
		a := ""
		if i < len(t.ColumnInfos) {
			a = t.ColumnInfos[i].Align
		}
		switch a {
		case "l":
			a = "left"
		case "c":
			a = "center"
		case "r":
			a = "right"
		default:
			a = "left"
			if i < len(filled) && filled[i] > 0 && numeric[i]*2 > filled[i] {
				a = "right"
			}
		}
		aligns = append(aligns, a)
	}
	lines := []string{fmt.Sprintf("columns: %d", cols), "align: (" + strings.Join(aligns, ", ") + ",)"}
	pad := func(cells []string) string {
		for len(cells) < cols {
			cells = append(cells, "[]")
		}
		return strings.Join(cells, ", ")
	}
	if headerEnd > 0 {
		header := []string{}
		for _, r := range rows[:headerEnd] {
			if !r.rule {
				cells := []string{}
				for _, c := range r.cells {
					cells = append(cells, "org-th"+c)
				}
				header = append(header, pad(cells))
			}
		}
		// The rule under the header is the table's own line, drawn however
		// the template draws table.hline.
		lines = append(lines, "table.header("+strings.Join(header, ", ")+", table.hline())")
		rows = rows[headerEnd+1:]
	}
	for i, r := range rows {
		if r.rule {
			// A rule at the very top or bottom is the table's own border.
			if i > 0 && i < len(rows)-1 {
				lines = append(lines, "table.hline()")
			}
			continue
		}
		lines = append(lines, pad(r.cells))
	}
	return "table(\n  " + strings.Join(lines, ",\n  ") + ",\n)"
}

func (w *Writer) WriteTableEl(t org.TableEl) {
	w.WriteExample(org.Example{Children: func() []org.Node {
		out := []org.Node{}
		for _, l := range t.Lines {
			out = append(out, org.Text{Content: l, IsRaw: true})
		}
		return out
	}()})
}

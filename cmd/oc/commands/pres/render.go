package pres

// From org nodes to lines of styled text.
//
// A slide is drawn in two passes. This one turns a slide's nodes into a list of
// lines - each a run of styled segments, an indent, and the fragment step it
// appears on - for a column of a given width. The layout pass (layout.go) then
// places the lines on the screen. Keeping them apart is what lets one slide be
// drawn to a terminal, to a pipe as ansi, or measured to see whether it fits,
// with one set of rules about what a list or a table looks like.
//
// Nodes are matched as values *and* pointers throughout: go-org hands out both
// for the same type depending on where the node was parsed (see **Traps:
// pointer trap**), and a case that quietly handles one of them is a slide with
// a hole in it.

import (
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/mattn/go-runewidth"
)

type seg struct {
	s  string
	st tcell.Style
}

// line is one row of a slide's content.
type line struct {
	segs   []seg
	indent int
	// step is the fragment this line belongs to: shown once the slide has been
	// stepped that far, left as blank space before. Blank rather than absent so
	// that revealing a bullet does not move every line below it.
	step int
	// band is a panel behind the line (code, examples), bandW columns wide
	// from the indent. Zero width is no panel.
	band   tcell.Style
	bandW  int
	center bool
	// gap marks the blank line between two blocks, which is where a slide too
	// tall for the screen gives up space first.
	gap bool
}

func (l line) width() int {
	w := 0
	for _, s := range l.segs {
		w += runewidth.StringWidth(s.s)
	}
	return w
}

func (l line) blank() bool { return len(l.segs) == 0 && l.bandW == 0 }

type renderer struct {
	p    *palette
	conf slides.Conf
	// files is how an image link is found on this disk.
	files *finder
	// maxImg is the most rows a picture may take.
	maxImg int
	// cur is the fragment step lines are being issued on, steps the highest
	// issued so far.
	cur, steps int
	// slideFrag is the slide's `:FRAGMENT:`: its top-level list items step.
	slideFrag bool
	listDepth int
	pend      slides.Pending
	// hard is set inside a verse, where a line break in the file is one on
	// the slide.
	hard  bool
	out   []line
	foots []org.FootnoteDefinition
}

func newRenderer(p *palette, c slides.Conf, files *finder, maxImg int) *renderer {
	if maxImg < 4 {
		maxImg = 4
	}
	return &renderer{p: p, conf: c, files: files, maxImg: maxImg}
}

func (r *renderer) base() tcell.Style { return r.p.base() }

func (r *renderer) emit(l line) {
	l.step = r.cur
	r.out = append(r.out, l)
}

// gap puts one blank line between blocks - never two, and never one at the top.
func (r *renderer) gap() {
	if len(r.out) == 0 || r.out[len(r.out)-1].blank() {
		return
	}
	r.out = append(r.out, line{gap: true})
}

// fragment issues the next step and runs fn on it.
func (r *renderer) fragment(fn func()) {
	r.steps++
	prev := r.cur
	r.cur = r.steps
	fn()
	r.cur = prev
}

// prefixed runs fn one indent deeper and then hangs a prefix in front of what
// it drew: a bullet on the first line and spaces under it, or a quote's bar on
// every line. Lines drawn further in keep their own indent behind the prefix.
func (r *renderer) prefixed(indent, width int, first, rest []seg, fn func(indent, width int)) {
	pw := segsWidth(first)
	start := len(r.out)
	fn(indent+pw, width-pw)
	if len(r.out) == start {
		r.emit(line{indent: indent, segs: first})
		return
	}
	for i := start; i < len(r.out); i++ {
		l := &r.out[i]
		pre := rest
		if i == start {
			pre = first
		}
		if l.blank() {
			if segsWidth(pre) > 0 && strings.TrimSpace(segsText(pre)) != "" {
				l.segs = append([]seg{}, pre...)
				l.indent = indent
				l.gap = false
			}
			continue
		}
		pad := l.indent - (indent + pw)
		segs := append([]seg{}, pre...)
		if pad > 0 {
			segs = append(segs, seg{strings.Repeat(" ", pad), r.base()})
		}
		body := l.segs
		if l.bandW > 0 {
			// A panel now starts after the prefix rather than at the indent,
			// so its ground is drawn as segments padded out to its width.
			body = bandToSegs(*l)
			l.bandW = 0
		}
		l.segs = append(segs, body...)
		l.indent = indent
	}
}

// bandToSegs turns a panel line into plain segments padded out to its width.
func bandToSegs(l line) []seg {
	out := append([]seg{}, l.segs...)
	if w := segsWidth(out); w < l.bandW {
		out = append(out, seg{strings.Repeat(" ", l.bandW-w), l.band})
	}
	return out
}

// ── Blocks ──────────────────────────────────────────────────────────────────

func (r *renderer) blocks(nodes []org.Node, indent, width int, tight bool) {
	for _, n := range nodes {
		r.block(n, indent, width, tight)
	}
}

func (r *renderer) block(n org.Node, indent, width int, tight bool) {
	if width < 8 {
		width = 8
	}
	// An `#+ATTR_SLIDE:` line is spent on the element after it.
	if k, ok := asKeyword(n); ok {
		if slides.IsAttrKeyword(k.Key, "TERM") || slides.IsAttrKeyword(k.Key, "REVEAL") {
			r.pend.AddAttr(k.Value)
		}
		return
	}
	pend := r.pend.Take()
	draw := func() { r.element(n, indent, width, tight, pend) }
	if pend.Frag && !isList(n) {
		r.fragment(draw)
		return
	}
	draw()
}

func (r *renderer) element(n org.Node, indent, width int, tight bool, pend slides.Pending) {
	switch v := n.(type) {
	case org.Paragraph:
		r.paragraph(v.Children, indent, width, tight)
	case *org.Paragraph:
		r.paragraph(v.Children, indent, width, tight)
	case org.List:
		r.list(v, indent, width, pend.Frag)
	case *org.List:
		r.list(*v, indent, width, pend.Frag)
	case org.Table:
		r.table(&v, indent, width)
	case *org.Table:
		r.table(v, indent, width)
	case org.Block:
		r.blockNode(&v, indent, width)
	case *org.Block:
		r.blockNode(v, indent, width)
	case org.Example:
		r.gap()
		r.panel(exampleLines(v.Children), "", false, indent, width)
	case *org.Example:
		r.gap()
		r.panel(exampleLines(v.Children), "", false, indent, width)
	case org.Result:
		r.block(v.Node, indent, width, tight)
	case *org.Result:
		r.block(v.Node, indent, width, tight)
	case org.HorizontalRule, *org.HorizontalRule:
		r.gap()
		mark := "─────  ◆  ─────"
		r.emit(line{indent: indent, segs: []seg{{mark, r.base().Foreground(r.p.rule)}}, center: true})
	case org.Headline:
		r.subheading(&v, indent, width)
	case *org.Headline:
		r.subheading(v, indent, width)
	case org.NodeWithMeta:
		r.block(v.Node, indent, width, tight)
		r.caption(v.Meta.Caption, indent, width)
	case *org.NodeWithMeta:
		r.block(v.Node, indent, width, tight)
		r.caption(v.Meta.Caption, indent, width)
	case org.NodeWithName:
		r.block(v.Node, indent, width, tight)
	case *org.NodeWithName:
		r.block(v.Node, indent, width, tight)
	case org.FootnoteDefinition:
		r.foots = append(r.foots, v)
	case *org.FootnoteDefinition:
		r.foots = append(r.foots, *v)
	case org.LatexFragment:
		r.gap()
		r.emit(line{indent: indent, segs: []seg{{rawText(v.Content, " "), r.code()}}, center: true})
	case org.Drawer, *org.Drawer, org.PropertyDrawer, *org.PropertyDrawer,
		org.Comment, *org.Comment, org.Include, *org.Include, org.Keyword, *org.Keyword:
		// Bookkeeping, not content.
	default:
		// Anything else that has inline text is shown as a paragraph rather
		// than dropped: a node type this file has not heard of is still
		// somebody's words.
		if s := strings.TrimSpace(org.String(n)); s != "" {
			r.paragraph([]org.Node{org.Text{Content: s}}, indent, width, tight)
		}
	}
}

func (r *renderer) paragraph(nodes []org.Node, indent, width int, tight bool) {
	if imgs := imagesOnly(nodes); len(imgs) > 0 {
		for _, l := range imgs {
			if !tight {
				r.gap()
			}
			r.image(l, indent, width)
		}
		return
	}
	if !tight {
		r.gap()
	}
	segs := r.inline(nodes, r.base())
	for _, ws := range wrap(segs, width) {
		r.emit(line{indent: indent, segs: ws})
	}
}

func (r *renderer) subheading(h *org.Headline, indent, width int) {
	r.gap()
	st := r.base().Foreground(r.p.accent).Bold(true)
	segs := r.inline(h.Title, st)
	for _, ws := range wrap(segs, width) {
		r.emit(line{indent: indent, segs: ws})
	}
	r.blocks(h.Children, indent, width, false)
}

func (r *renderer) caption(caps [][]org.Node, indent, width int) {
	for _, c := range caps {
		st := r.base().Foreground(r.p.soft).Italic(true)
		for _, ws := range wrap(r.inline(c, st), width) {
			r.emit(line{indent: indent, segs: ws, center: true})
		}
	}
}

// ── Lists ───────────────────────────────────────────────────────────────────

var bullets = []string{"", "◦", "▪", "‣"}

func (r *renderer) list(l org.List, indent, width int, frag bool) {
	if r.listDepth == 0 {
		r.gap()
	}
	// The slide's :FRAGMENT: steps its top-level list; `#+ATTR_SLIDE: :frag`
	// steps the list it is written above.
	step := frag || (r.slideFrag && r.listDepth == 0)
	r.listDepth++
	defer func() { r.listDepth-- }()

	// Ordered bullets are right-aligned on their dot, so 9. and 10. line up.
	bw := 0
	for _, it := range l.Items {
		if li, ok := asListItem(it); ok && l.Kind == "ordered" {
			bw = max(bw, runewidth.StringWidth(li.Bullet))
		}
	}
	for _, it := range l.Items {
		draw := func() { r.item(it, l.Kind, bw, indent, width) }
		if step {
			r.fragment(draw)
		} else {
			draw()
		}
	}
}

func (r *renderer) item(n org.Node, kind string, bw, indent, width int) {
	mark := r.base().Foreground(r.p.accent)
	if li, ok := asListItem(n); ok {
		var first []seg
		switch kind {
		case "ordered":
			b := li.Bullet
			first = []seg{{strings.Repeat(" ", bw-runewidth.StringWidth(b)) + b + " ", mark}}
		default:
			b := r.p.bullet
			if r.listDepth > 1 {
				b = bullets[min(r.listDepth-1, len(bullets)-1)]
			}
			first = []seg{{b + " ", mark}}
		}
		body := li.Children
		switch li.Status {
		case "X":
			first = append(first, seg{"✔ ", r.base().Foreground(r.p.accent)})
		case " ":
			first = append(first, seg{"☐ ", r.base().Foreground(r.p.soft)})
		case "-":
			first = append(first, seg{"◐ ", r.base().Foreground(r.p.accent)})
		}
		rest := []seg{{strings.Repeat(" ", segsWidth(first)), r.base()}}
		r.prefixed(indent, width, first, rest, func(in, w int) {
			if li.Status == "X" {
				// A done item is still read, but it is behind us.
				save := r.p
				p := *r.p
				p.ink = p.soft
				r.p = &p
				defer func() { r.p = save }()
			}
			r.blocks(body, in, w, true)
		})
		return
	}
	if di, ok := asDescItem(n); ok {
		b := r.p.bullet
		first := []seg{{b + " ", mark}}
		rest := []seg{{strings.Repeat(" ", segsWidth(first)), r.base()}}
		r.prefixed(indent, width, first, rest, func(in, w int) {
			term := r.inline(di.Term, r.base().Foreground(r.p.head).Bold(true))
			for _, ws := range wrap(term, w) {
				r.emit(line{indent: in, segs: ws})
			}
			r.prefixed(in, w, []seg{{"  ", r.base()}}, []seg{{"  ", r.base()}}, func(in, w int) {
				r.blocks(di.Details, in, w, true)
			})
		})
	}
}

// ── Blocks: code, quotes, verse ─────────────────────────────────────────────

func (r *renderer) blockNode(b *org.Block, indent, width int) {
	name := strings.ToUpper(b.Name)
	params := strings.Fields(strings.Join(b.Parameters, " "))
	switch name {
	case "SRC":
		r.gap()
		lang := ""
		if len(params) > 0 {
			lang = params[0]
		}
		numbers := false
		for _, p := range params[min(1, len(params)):] {
			if p == "-n" || p == "+n" {
				numbers = true
			}
		}
		r.source(rawText(b.Children, "\n"), lang, numbers, indent, width)
		if b.Result != nil {
			r.results(b.Result, indent, width)
		}
	case "EXAMPLE":
		r.gap()
		r.panel(strings.Split(rawText(b.Children, "\n"), "\n"), "", false, indent, width)
	case "QUOTE":
		r.gap()
		bar := []seg{{"▎ ", r.base().Foreground(r.p.accent)}}
		r.prefixed(indent, width, bar, bar, func(in, w int) {
			save := r.p
			p := *r.p
			p.ink = lerp(p.ink, p.soft, 0.35)
			r.p = &p
			r.blocks(b.Children, in, w, true)
			r.p = save
		})
	case "VERSE":
		r.gap()
		r.hard = true
		r.blocks(b.Children, indent+2, width-2, true)
		r.hard = false
	case "CENTER":
		start := len(r.out)
		r.blocks(b.Children, indent, width, false)
		for i := start; i < len(r.out); i++ {
			r.out[i].center = true
		}
	case "COMMENT", "EXPORT", "NOTES", "NOTE":
		// Not for the audience, or not for a terminal.
	default:
		// A special block - `#+begin_tip`, `#+begin_warning` - is a card with
		// its name on it, which is what every html theme makes of one too.
		r.gap()
		bar := []seg{{"┃ ", r.base().Foreground(r.p.accent)}}
		r.prefixed(indent, width, bar, bar, func(in, w int) {
			r.emit(line{indent: in, segs: []seg{{strings.ToUpper(b.Name), r.base().Foreground(r.p.accent).Bold(true)}}})
			r.blocks(b.Children, in, w, true)
		})
	}
}

func (r *renderer) results(n org.Node, indent, width int) {
	var inner org.Node
	switch v := n.(type) {
	case org.Result:
		inner = v.Node
	case *org.Result:
		inner = v.Node
	default:
		inner = n
	}
	if inner == nil {
		return
	}
	r.emit(line{indent: indent, segs: []seg{{"⇒", r.base().Foreground(r.p.soft)}}})
	r.block(inner, indent, width, true)
}

// code is the style of inline code: a chip on the code panel's ground.
func (r *renderer) code() tcell.Style {
	return r.base().Foreground(r.p.accent).Background(r.p.codeBg)
}

// panel draws lines of verbatim text on the code ground: examples, and source
// blocks once they have been coloured.
func (r *renderer) panel(text []string, label string, numbers bool, indent, width int) {
	for len(text) > 0 && strings.TrimSpace(text[len(text)-1]) == "" {
		text = text[:len(text)-1]
	}
	lines := make([][]seg, len(text))
	st := r.base().Background(r.p.codeBg).Foreground(r.p.codeInk)
	for i, t := range text {
		lines[i] = []seg{{t, st}}
	}
	r.panelSegs(lines, label, numbers, indent, width)
}

func (r *renderer) panelSegs(lines [][]seg, label string, numbers bool, indent, width int) {
	bg := r.base().Background(r.p.codeBg)
	gutter := 0
	if numbers {
		gutter = len(itoa(len(lines))) + 2
	}
	inner := 0
	for _, l := range lines {
		inner = max(inner, segsWidth(l))
	}
	inner = max(inner+gutter, runewidth.StringWidth(label)+2)
	pw := min(width, inner+4)
	room := pw - 4 - gutter
	band := func(segs []seg) {
		r.emit(line{indent: indent, segs: segs, band: bg, bandW: pw})
	}
	// The language sits in the top padding, right-aligned and quiet: a reader
	// wants to know, an audience does not need telling twice.
	top := []seg{}
	if label != "" {
		lw := runewidth.StringWidth(label)
		top = []seg{{strings.Repeat(" ", max(0, pw-lw-2)), bg}, {label, bg.Foreground(r.p.soft).Italic(true)}}
	}
	band(top)
	for i, l := range lines {
		segs := []seg{{"  ", bg}}
		if numbers {
			n := itoa(i + 1)
			segs = append(segs, seg{strings.Repeat(" ", gutter-2-len(n)) + n + "  ", bg.Foreground(r.p.soft)})
		}
		segs = append(segs, clip(l, room, bg.Foreground(r.p.soft))...)
		band(segs)
	}
	band(nil)
}

// ── Inline ──────────────────────────────────────────────────────────────────

var (
	dashes = strings.NewReplacer("---", "—", "--", "–", "...", "…")
	super  = map[rune]rune{'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴', '5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹', '+': '⁺', '-': '⁻', '=': '⁼', '(': '⁽', ')': '⁾', 'n': 'ⁿ', 'i': 'ⁱ'}
	sub    = map[rune]rune{'0': '₀', '1': '₁', '2': '₂', '3': '₃', '4': '₄', '5': '₅', '6': '₆', '7': '₇', '8': '₈', '9': '₉', '+': '₊', '-': '₋', '=': '₌', '(': '₍', ')': '₎'}
)

func (r *renderer) inline(nodes []org.Node, st tcell.Style) []seg {
	out := []seg{}
	for _, n := range nodes {
		switch v := n.(type) {
		case org.Text:
			out = append(out, r.text(v, st))
		case *org.Text:
			out = append(out, r.text(*v, st))
		case org.LineBreak:
			if r.hard {
				for i := 0; i < max(1, v.Count); i++ {
					out = append(out, seg{"\n", st})
				}
			} else {
				out = append(out, seg{" ", st})
			}
		case org.ExplicitLineBreak, *org.ExplicitLineBreak:
			out = append(out, seg{"\n", st})
		case org.Emphasis:
			out = append(out, r.emphasis(v, st)...)
		case *org.Emphasis:
			out = append(out, r.emphasis(*v, st)...)
		case org.RegularLink:
			out = append(out, r.link(v, st)...)
		case *org.RegularLink:
			out = append(out, r.link(*v, st)...)
		case org.Timestamp:
			out = append(out, seg{strings.Trim(org.String(v), "<>[]"), st.Foreground(r.p.soft)})
		case org.StatisticToken:
			out = append(out, seg{"[" + v.Content + "]", st.Foreground(r.p.soft)})
		case org.FootnoteLink:
			out = append(out, seg{script(v.Name, super, "["+v.Name+"]"), st.Foreground(r.p.accent)})
			if v.Definition != nil {
				r.foots = append(r.foots, *v.Definition)
			}
		case org.LatexFragment:
			out = append(out, seg{rawText(v.Content, " "), st.Foreground(r.p.accent)})
		case org.Macro:
			out = append(out, seg{"{{{" + v.Name + "}}}", st.Foreground(r.p.soft)})
		default:
			if s := org.String(n); s != "" {
				out = append(out, seg{s, st})
			}
		}
	}
	return out
}

func (r *renderer) text(t org.Text, st tcell.Style) seg {
	s := strings.ReplaceAll(t.Content, "\n", " ")
	if !t.IsRaw {
		s = dashes.Replace(s)
	}
	return seg{s, st}
}

func (r *renderer) emphasis(e org.Emphasis, st tcell.Style) []seg {
	switch e.Kind {
	case "*":
		return r.inline(e.Content, st.Bold(true).Foreground(r.p.head))
	case "/":
		return r.inline(e.Content, st.Italic(true))
	case "_":
		return r.inline(e.Content, st.Underline(true))
	case "+":
		return r.inline(e.Content, st.StrikeThrough(true).Foreground(r.p.soft))
	case "=", "~":
		return []seg{{rawText(e.Content, " "), st.Foreground(r.p.accent).Background(r.p.codeBg)}}
	case "^{}":
		s := rawText(e.Content, "")
		return []seg{{script(s, super, "^"+s), st}}
	case "_{}":
		s := rawText(e.Content, "")
		return []seg{{script(s, sub, "_"+s), st}}
	}
	return r.inline(e.Content, st)
}

func (r *renderer) link(l org.RegularLink, st tcell.Style) []seg {
	ls := st.Foreground(r.p.link).Underline(true)
	if l.Kind() == "image" && len(l.Description) == 0 {
		return []seg{{"▣ " + baseName(l.URL), st.Foreground(r.p.soft)}}
	}
	if len(l.Description) > 0 {
		return r.inline(l.Description, ls)
	}
	u := l.URL
	for _, p := range []string{"https://", "http://", "file:", "mailto:"} {
		u = strings.TrimPrefix(u, p)
	}
	return []seg{{u, ls}}
}

// script writes a sub- or superscript in the unicode forms when every letter
// has one, and as written otherwise: H₂O is better than H_2O, and x_{max} is
// better than half of it lowered.
func script(s string, table map[rune]rune, fallback string) string {
	b := strings.Builder{}
	for _, c := range s {
		m, ok := table[c]
		if !ok {
			return fallback
		}
		b.WriteRune(m)
	}
	return b.String()
}

// ── Wrapping ────────────────────────────────────────────────────────────────

type word struct {
	segs  []seg
	space seg // the whitespace in front of it, in its own style
	hard  bool
}

// wrap fills styled text into lines of a width: whole words where they fit,
// broken where one word is wider than the line.
func wrap(segs []seg, width int) [][]seg {
	words := []word{}
	cur := word{}
	pendingSpace := seg{}
	flush := func() {
		if len(cur.segs) > 0 {
			words = append(words, cur)
		}
		cur = word{}
	}
	for _, s := range segs {
		if s.s == "\n" {
			flush()
			words = append(words, word{hard: true})
			pendingSpace = seg{}
			continue
		}
		start := 0
		for i, c := range s.s {
			if c == ' ' || c == '\t' {
				if i > start {
					if len(cur.segs) == 0 {
						cur.space = pendingSpace
					}
					cur.segs = append(cur.segs, seg{s.s[start:i], s.st})
				}
				flush()
				pendingSpace = seg{" ", s.st}
				start = i + utf8.RuneLen(c)
			}
		}
		if start < len(s.s) {
			if len(cur.segs) == 0 {
				cur.space = pendingSpace
			}
			cur.segs = append(cur.segs, seg{s.s[start:], s.st})
			pendingSpace = seg{}
		}
	}
	flush()

	lines := [][]seg{}
	ln := []seg{}
	lw := 0
	for _, w := range words {
		if w.hard {
			lines = append(lines, ln)
			ln, lw = []seg{}, 0
			continue
		}
		ww := segsWidth(w.segs)
		need := ww
		if lw > 0 {
			need++
		}
		if lw > 0 && lw+need > width {
			lines = append(lines, ln)
			ln, lw = []seg{}, 0
			need = ww
		}
		if ww > width {
			// One word wider than the line: break it where it must.
			for _, piece := range breakWord(w.segs, width-lw) {
				if lw > 0 && lw+segsWidth(piece) > width {
					lines = append(lines, ln)
					ln, lw = []seg{}, 0
				}
				ln = append(ln, piece...)
				lw += segsWidth(piece)
			}
			continue
		}
		if lw > 0 {
			sp := w.space
			if sp.s == "" {
				sp = seg{" ", w.segs[0].st}
			}
			// A space between two links is part of neither.
			sp.st = sp.st.Underline(false).StrikeThrough(false)
			ln = append(ln, sp)
		}
		ln = append(ln, w.segs...)
		lw += need
	}
	if len(ln) > 0 || len(lines) == 0 {
		lines = append(lines, ln)
	}
	return lines
}

func breakWord(segs []seg, first int) [][]seg {
	out := [][]seg{}
	cur := []seg{}
	room := max(1, first)
	w := 0
	for _, s := range segs {
		b := strings.Builder{}
		for _, c := range s.s {
			cw := runewidth.RuneWidth(c)
			if w+cw > room && w > 0 {
				if b.Len() > 0 {
					cur = append(cur, seg{b.String(), s.st})
					b.Reset()
				}
				out = append(out, cur)
				cur, w = []seg{}, 0
			}
			b.WriteRune(c)
			w += cw
		}
		if b.Len() > 0 {
			cur = append(cur, seg{b.String(), s.st})
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// clip cuts a line of segments to a width, marking the cut.
func clip(segs []seg, width int, mark tcell.Style) []seg {
	if segsWidth(segs) <= width {
		return segs
	}
	out := []seg{}
	w := 0
	for _, s := range segs {
		b := strings.Builder{}
		for _, c := range s.s {
			cw := runewidth.RuneWidth(c)
			if w+cw > width-1 {
				if b.Len() > 0 {
					out = append(out, seg{b.String(), s.st})
				}
				return append(out, seg{"…", mark})
			}
			b.WriteRune(c)
			w += cw
		}
		out = append(out, seg{b.String(), s.st})
	}
	return out
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func segsWidth(segs []seg) int {
	w := 0
	for _, s := range segs {
		w += runewidth.StringWidth(s.s)
	}
	return w
}

func segsText(segs []seg) string {
	b := strings.Builder{}
	for _, s := range segs {
		b.WriteString(s.s)
	}
	return b.String()
}

// rawText is the literal text of verbatim nodes: a source block's lines, an
// example's, the inside of =code=.
func rawText(nodes []org.Node, nl string) string {
	b := strings.Builder{}
	for _, n := range nodes {
		switch v := n.(type) {
		case org.Text:
			b.WriteString(v.Content)
		case *org.Text:
			b.WriteString(v.Content)
		case org.LineBreak:
			b.WriteString(strings.Repeat(nl, max(1, v.Count)))
		default:
			b.WriteString(org.String(n))
		}
	}
	return strings.ReplaceAll(b.String(), "\t", "    ")
}

// exampleLines is a `: ` example's text. Its lines are Text nodes with no line
// breaks between them, unlike a source block's.
func exampleLines(nodes []org.Node) []string {
	out := []string{}
	for _, n := range nodes {
		out = append(out, strings.ReplaceAll(rawText([]org.Node{n}, "\n"), "\n", ""))
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func baseName(p string) string {
	p = strings.TrimPrefix(strings.TrimPrefix(p, "file:"), "//")
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func asKeyword(n org.Node) (org.Keyword, bool) {
	switch v := n.(type) {
	case org.Keyword:
		return v, true
	case *org.Keyword:
		return *v, true
	}
	return org.Keyword{}, false
}

func asListItem(n org.Node) (org.ListItem, bool) {
	switch v := n.(type) {
	case org.ListItem:
		return v, true
	case *org.ListItem:
		return *v, true
	}
	return org.ListItem{}, false
}

func asDescItem(n org.Node) (org.DescriptiveListItem, bool) {
	switch v := n.(type) {
	case org.DescriptiveListItem:
		return v, true
	case *org.DescriptiveListItem:
		return *v, true
	}
	return org.DescriptiveListItem{}, false
}

func isList(n org.Node) bool {
	switch n.(type) {
	case org.List, *org.List:
		return true
	}
	return false
}

// imagesOnly is the image links of a paragraph that is nothing but image
// links: a picture on its own line is a picture on the slide, while one in the
// middle of a sentence is a name in it.
func imagesOnly(nodes []org.Node) []org.RegularLink {
	out := []org.RegularLink{}
	for _, n := range nodes {
		switch v := n.(type) {
		case org.RegularLink:
			if v.Kind() != "image" {
				return nil
			}
			out = append(out, v)
		case *org.RegularLink:
			if v.Kind() != "image" {
				return nil
			}
			out = append(out, *v)
		case org.LineBreak, org.ExplicitLineBreak:
		case org.Text:
			if strings.TrimSpace(v.Content) != "" {
				return nil
			}
		default:
			return nil
		}
	}
	return out
}

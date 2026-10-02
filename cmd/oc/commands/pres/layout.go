package pres

// Placing a slide on the screen.
//
// Three kinds of page, as in the html decks: the title page (`#+TITLE:`,
// subtitle, author, date and whatever is written above the first headline), a
// section divider (a headline with nothing of its own under it - the name of
// the next part of the talk, set large), and an ordinary slide (title, a short
// accent rule, body). Everything is drawn into a canvas of cells first, so a
// transition can blend two finished pages and the same page can be printed to
// a pipe.

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/mattn/go-runewidth"
)

// ── The canvas ──────────────────────────────────────────────────────────────

type cell struct {
	r    rune
	comb []rune
	st   tcell.Style
	// cont is the right half of a wide character drawn in the cell before.
	cont bool
}

type canvas struct {
	w, h  int
	cells []cell
}

func newCanvas(w, h int, st tcell.Style) *canvas {
	c := &canvas{w: w, h: h, cells: make([]cell, w*h)}
	for i := range c.cells {
		c.cells[i] = cell{r: ' ', st: st}
	}
	return c
}

func (c *canvas) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return nil
	}
	return &c.cells[y*c.w+x]
}

// put draws a string from x and returns where it ended.
func (c *canvas) put(x, y int, s string, st tcell.Style) int {
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w == 0 {
			if p := c.at(x-1, y); p != nil {
				p.comb = append(p.comb, r)
			}
			continue
		}
		if p := c.at(x, y); p != nil {
			if x+w > c.w {
				break
			}
			*p = cell{r: r, st: st}
			if w == 2 {
				if q := c.at(x+1, y); q != nil {
					*q = cell{r: ' ', st: st, cont: true}
				}
			}
		}
		x += w
	}
	return x
}

func (c *canvas) fill(x, y, w int, st tcell.Style) {
	for i := 0; i < w; i++ {
		if p := c.at(x+i, y); p != nil {
			*p = cell{r: ' ', st: st}
		}
	}
}

// ── The deck, as the terminal sees it ───────────────────────────────────────

// page is one stop in the talk: the title page, or a slide.
type page struct {
	s     *slides.Slide // nil for the title page
	title bool
}

type deck struct {
	d     *slides.Deck
	doc   *org.Document
	conf  slides.Conf
	pages []page
	path  string
}

func newDeck(doc *org.Document, path string) *deck {
	c := slides.Conf{Doc: doc, Prefix: "TERM"}
	d := slides.BuildDeck(c, doc)
	dk := &deck{d: d, doc: doc, conf: c, path: path}
	if d.TitleSlide {
		dk.pages = append(dk.pages, page{title: true})
	}
	for _, s := range d.Flat() {
		dk.pages = append(dk.pages, page{s: s})
	}
	return dk
}

func (d *deck) title() string {
	if d.d.Title != "" {
		return d.d.Title
	}
	return strings.TrimSuffix(baseName(d.path), ".org")
}

// titleOf is a page's name, for the overview and the presenter's "next".
func (d *deck) titleOf(i int) string {
	if i < 0 || i >= len(d.pages) {
		return ""
	}
	if d.pages[i].title {
		return d.title()
	}
	return strings.TrimSpace(org.String(d.pages[i].s.Title...))
}

// ── Composing a page ────────────────────────────────────────────────────────

type view struct {
	pal    *palette
	files  *finder
	footer bool
	scroll int
	// maxWidth is the widest a column of text gets, however wide the
	// terminal: past ninety-odd characters a line is hard to follow from
	// the back of a room.
	maxWidth int
	idx      int
	timer    string
	msg      string
	// noStep shows every fragment, which is what a printed deck wants.
	noStep bool
}

// composed is a finished page and what was learned drawing it.
type composed struct {
	c        *canvas
	steps    int
	overflow int // rows that did not fit
}

// slidePalette is the theme with this slide's `:BACKGROUND:` applied, and the
// text turned light or dark to suit it (slides.ReadInk, the rule the html
// decks follow).
func (d *deck) slidePalette(p *palette, s *slides.Slide) *palette {
	var props slides.PropGet
	if s != nil {
		props = s.Props()
	}
	bg := slides.ReadBackground(d.conf, props, "", nil)
	if bg.Kind != slides.BgColor && bg.Kind != slides.BgGradient {
		return p
	}
	v := bg.Value
	if bg.Kind == slides.BgGradient {
		if m := hexInGradient.FindString(v); m != "" {
			v = m
		} else {
			return p
		}
	}
	r, g, b, ok := cssColour(v, [3]float64{0, 0, 0})
	if !ok {
		return p
	}
	out := *p
	out.bg = rgb(r, g, b)
	ink := slides.ReadInk(d.conf, props, bg)
	dark := ink == slides.InkOnDark
	if ink != slides.InkUnknown && dark != p.dark {
		// The accent was chosen against the theme's ground, so it is pulled
		// toward the new ink until it reads on this one.
		if dark {
			out.ink, out.soft, out.head = hex(0xeef0f6), hex(0xb4b9cc), hex(0xffffff)
			out.accent, out.link = lerp(p.accent, hex(0xffffff), 0.35), lerp(p.link, hex(0xffffff), 0.35)
		} else {
			out.ink, out.soft, out.head = hex(0x1f2128), hex(0x5a5f6e), hex(0x0b0c10)
			out.accent, out.link = lerp(p.accent, hex(0x000000), 0.5), lerp(p.link, hex(0x000000), 0.5)
		}
		out.dark = dark
	}
	// The code ground sits a little off the slide's own colour.
	if dark {
		out.codeBg = rgb(r*0.7, g*0.7, b*0.7)
	} else {
		out.codeBg = rgb(r+(255-r)*0.4, g+(255-g)*0.4, b+(255-b)*0.4)
	}
	out.rule = lerp(out.bg, out.soft, 0.3)
	return &out
}

// compose draws page i at a step.
func (d *deck) compose(i, step, w, h int, v view) composed {
	pg := d.pages[i]
	pal := d.slidePalette(v.pal, pg.s)
	cv := newCanvas(w, h, pal.base())
	out := composed{c: cv}

	foot := 0
	if v.footer {
		foot = 2
	}
	margin := max(2, w/12)
	colW := min(w-2*margin, v.maxWidth)
	if colW < 20 {
		colW = max(10, w-2)
	}
	left := (w - colW) / 2
	top := max(1, h/10)
	bottom := h - foot - 1
	avail := max(1, bottom-top)

	files := v.files
	r := newRenderer(pal, d.conf, files, max(4, avail*2/3))

	var lines []line
	center := false
	vcenter := false
	switch {
	case pg.title:
		lines = d.titlePage(r, colW)
		center, vcenter = true, true
	case len(pg.s.Body) == 0:
		lines = d.divider(r, pg.s, colW)
		center, vcenter = true, true
	default:
		s := pg.s
		r.slideFrag = slides.ReadFragment(d.conf, s.Props()).On
		st := pal.base().Foreground(pal.head).Bold(true)
		for _, ws := range wrap(r.inline(s.Title, st), colW) {
			r.out = append(r.out, line{segs: ws})
		}
		r.out = append(r.out, line{segs: []seg{{"━━━━", pal.base().Foreground(pal.accent)}}})
		r.out = append(r.out, line{})
		head := len(r.out)
		r.blocks(s.Body, 0, colW, false)
		r.footnotes(colW)
		lines = r.out
		// Leading blank lines in the body would double the gap under the rule.
		for len(lines) > head && lines[head].blank() {
			lines = append(lines[:head], lines[head+1:]...)
		}
		switch slides.Align(d.conf, s.Props()) {
		case "center", "centre", "middle":
			center, vcenter = true, true
		}
		vcenter = vcenter || d.conf.Bool(s.Props(), "CENTER", false)
	}
	out.steps = r.steps
	if v.noStep {
		step = r.steps
	}

	// Too tall: give up the blank lines between blocks first, then scroll.
	if len(lines) > avail {
		tight := []line{}
		for _, l := range lines {
			if !l.gap {
				tight = append(tight, l)
			}
		}
		lines = tight
	}
	y := top
	if vcenter && len(lines) < avail {
		y = top + (avail-len(lines))/2
		if !pg.title && pg.s != nil && len(pg.s.Body) > 0 {
			y = top + (avail-len(lines))/3
		}
	}
	if len(lines) > avail {
		out.overflow = len(lines) - avail
	}
	scroll := min(max(0, v.scroll), out.overflow)
	for li := scroll; li < len(lines) && y < bottom; li++ {
		l := lines[li]
		if l.step <= step {
			x := left + l.indent
			if center || l.center {
				x = left + l.indent + max(0, (colW-l.indent-max(l.width(), l.bandW))/2)
			}
			if l.bandW > 0 {
				cv.fill(x, y, min(l.bandW, w-x), l.band)
			}
			for _, s := range l.segs {
				x = cv.put(x, y, s.s, s.st)
			}
		}
		y++
	}
	if out.overflow > 0 {
		soft := pal.base().Foreground(pal.soft)
		if scroll < out.overflow {
			cv.put(left+colW-1, bottom-1, "▾", soft)
		}
		if scroll > 0 {
			cv.put(left+colW-1, top, "▴", soft)
		}
	}
	if v.footer {
		d.footer(cv, pal, i, step, out.steps, v)
	}
	return out
}

// titlePage is the first page: the title in the display face, then the rest.
func (d *deck) titlePage(r *renderer, width int) []line {
	p := r.p
	out := []line{}
	title := d.title()
	if rows, ok := big(title, width); ok {
		for _, row := range rows {
			out = append(out, line{segs: []seg{{row, p.base().Foreground(p.head)}}, center: true})
		}
	} else {
		for _, ws := range wrap([]seg{{title, p.base().Foreground(p.head).Bold(true)}}, width) {
			out = append(out, line{segs: ws, center: true})
		}
	}
	if d.d.Subtitle != "" {
		out = append(out, line{})
		for _, ws := range wrap([]seg{{d.d.Subtitle, p.base().Foreground(p.soft).Italic(true)}}, width) {
			out = append(out, line{segs: ws, center: true})
		}
	}
	out = append(out, line{}, line{segs: []seg{{"━━━━━━", p.base().Foreground(p.accent)}}, center: true}, line{})
	by := []seg{}
	if d.d.Author != "" {
		by = append(by, seg{d.d.Author, p.base().Foreground(p.ink)})
	}
	if d.d.Date != "" {
		if len(by) > 0 {
			by = append(by, seg{"  ·  ", p.base().Foreground(p.rule)})
		}
		by = append(by, seg{strings.Trim(d.d.Date, "<>[]"), p.base().Foreground(p.soft)})
	}
	if len(by) > 0 {
		out = append(out, line{segs: by, center: true})
	}
	if d.d.Email != "" {
		out = append(out, line{segs: []seg{{d.d.Email, p.base().Foreground(p.soft)}}, center: true})
	}
	if len(d.d.Preamble) > 0 {
		r.blocks(d.d.Preamble, 0, width, false)
		if len(r.out) > 0 {
			out = append(out, line{})
			for _, l := range r.out {
				l.center = true
				out = append(out, l)
			}
		}
	}
	return out
}

// divider is a slide with nothing but its name: the start of a part of the
// talk, so it is set large.
func (d *deck) divider(r *renderer, s *slides.Slide, width int) []line {
	p := r.p
	title := strings.TrimSpace(org.String(s.Title...))
	out := []line{}
	if rows, ok := big(title, width); ok && len(s.Subs) > 0 {
		for _, row := range rows {
			out = append(out, line{segs: []seg{{row, p.base().Foreground(p.head)}}, center: true})
		}
	} else {
		for _, ws := range wrap(r.inline(s.Title, p.base().Foreground(p.head).Bold(true)), width) {
			out = append(out, line{segs: ws, center: true})
		}
	}
	out = append(out, line{}, line{segs: []seg{{"━━━━", p.base().Foreground(p.accent)}}, center: true})
	return out
}

func (r *renderer) footnotes(width int) {
	if len(r.foots) == 0 {
		return
	}
	r.gap()
	seen := map[string]bool{}
	for _, f := range r.foots {
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		mark := script(f.Name, super, "["+f.Name+"]") + " "
		st := r.base().Foreground(r.p.soft)
		save := r.p
		p := *r.p
		p.ink = p.soft
		r.p = &p
		r.prefixed(0, width, []seg{{mark, st}}, []seg{{strings.Repeat(" ", runewidth.StringWidth(mark)), st}}, func(in, w int) {
			r.blocks(f.Children, in, w, true)
		})
		r.p = save
	}
}

// footer is the bottom two rows: the deck's name, where we are, the fragment
// dots and a timer above a progress rule the width of the screen.
func (d *deck) footer(cv *canvas, p *palette, i, step, steps int, v view) {
	w, h := cv.w, cv.h
	soft := p.base().Foreground(p.soft)
	y := h - 2
	margin := max(2, w/24)
	right := itoa(i+1) + " / " + itoa(len(d.pages))
	if steps > 0 {
		dots := ""
		for k := 1; k <= steps; k++ {
			if k <= step {
				dots += "●"
			} else {
				dots += "○"
			}
		}
		right = dots + "   " + right
	}
	rx := w - margin - runewidth.StringWidth(right)
	cv.put(rx, y, right, soft)
	// The middle (timer, or a message) wins over the deck's name, which is
	// the one thing on this row the audience already knows.
	end := rx - 2
	mid := v.timer
	if v.msg != "" {
		mid = v.msg
	}
	if mw := runewidth.StringWidth(mid); mid != "" && mw < rx-margin-2 {
		mx := max(margin, (w-mw)/2)
		if mx+mw > rx-1 {
			mx = rx - 1 - mw
		}
		cv.put(mx, y, mid, p.base().Foreground(p.accent))
		end = mx - 2
	}
	cv.put(margin, y, clipStr(d.title(), min(end-margin, w/2-margin)), soft)
	// Progress counts fragments as part of the way through a slide.
	frac := float64(i) / float64(max(1, len(d.pages)-1))
	if steps > 0 && i < len(d.pages)-1 {
		frac += float64(step) / float64(steps+1) / float64(max(1, len(d.pages)-1))
	}
	done := int(frac*float64(w) + 0.5)
	cv.put(0, h-1, strings.Repeat("━", done), p.base().Foreground(p.accent))
	cv.put(done, h-1, strings.Repeat("─", w-done), p.base().Foreground(p.rule))
}

func clipStr(s string, w int) string {
	if w <= 1 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

package pres

// Tables, pictures and source code: the three things on a slide that are not
// running text.

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/mattn/go-runewidth"
)

// ── Tables ──────────────────────────────────────────────────────────────────

var numeric = regexp.MustCompile(`^[-+]?[$€£]?[0-9][0-9,]*(\.[0-9]+)?%?$`)

// table draws an org table in a rounded box. Rows above the first rule are
// the header. Columns of numbers are right-aligned (go-org's own alignment says
// "right" for every column, so it is not read). A table wider than the slide
// gives up a character at a time from its widest column, as `orgs tables`
// does, and cut cells end in an ellipsis.
func (r *renderer) table(t *org.Table, indent, width int) {
	r.gap()
	type row struct {
		cells [][]seg
		rule  bool
	}
	rows := []row{}
	ncol := 0
	for _, rw := range t.Rows {
		if rw == nil {
			continue
		}
		if len(rw.Columns) == 0 {
			rows = append(rows, row{rule: true})
			continue
		}
		if rw.IsSpecial {
			continue
		}
		cells := [][]seg{}
		for _, c := range rw.Columns {
			cells = append(cells, r.inline(c.Children, r.base()))
		}
		ncol = max(ncol, len(cells))
		rows = append(rows, row{cells: cells})
	}
	if ncol == 0 {
		return
	}
	// The header is whatever sits above the first rule, when there is one
	// with something under it.
	head := 0
	for i, rw := range rows {
		if rw.rule {
			if i > 0 && i < len(rows)-1 {
				head = i
			}
			break
		}
	}
	widths := make([]int, ncol)
	numCount := make([]int, ncol)
	bodyCount := make([]int, ncol)
	for i, rw := range rows {
		for c, cell := range rw.cells {
			widths[c] = max(widths[c], segsWidth(cell))
			if i > head && !rw.rule || head == 0 {
				if s := strings.TrimSpace(segsText(cell)); s != "" {
					bodyCount[c]++
					if numeric.MatchString(s) {
						numCount[c]++
					}
				}
			}
		}
	}
	// Each column costs its text plus " │ " - three columns, and one more
	// for the closing edge.
	total := func() int {
		w := 1
		for _, x := range widths {
			w += x + 3
		}
		return w
	}
	for total() > width {
		wi := 0
		for i := range widths {
			if widths[i] > widths[wi] {
				wi = i
			}
		}
		if widths[wi] <= 3 {
			break
		}
		widths[wi]--
	}
	frame := r.base().Foreground(lerp(r.p.rule, r.p.soft, 0.45))
	rule := func(l, m, rr string) {
		b := strings.Builder{}
		b.WriteString(l)
		for i, w := range widths {
			if i > 0 {
				b.WriteString(m)
			}
			b.WriteString(strings.Repeat("─", w+2))
		}
		b.WriteString(rr)
		r.emit(line{indent: indent, segs: []seg{{b.String(), frame}}})
	}
	rule("╭", "┬", "╮")
	for i, rw := range rows {
		if rw.rule {
			if i > 0 && i < len(rows)-1 {
				rule("├", "┼", "┤")
			}
			continue
		}
		segs := []seg{{"│", frame}}
		for c := 0; c < ncol; c++ {
			var cell []seg
			if c < len(rw.cells) {
				cell = rw.cells[c]
			}
			if i < head {
				for k := range cell {
					cell[k].st = cell[k].st.Bold(true).Foreground(r.p.head)
				}
			}
			cell = clip(cell, widths[c], r.base().Foreground(r.p.soft))
			pad := widths[c] - segsWidth(cell)
			segs = append(segs, seg{" ", r.base()})
			right := bodyCount[c] > 0 && numCount[c]*2 > bodyCount[c] && i >= head
			if right {
				segs = append(segs, seg{strings.Repeat(" ", pad), r.base()})
			}
			segs = append(segs, cell...)
			if !right {
				segs = append(segs, seg{strings.Repeat(" ", pad), r.base()})
			}
			segs = append(segs, seg{" │", frame})
		}
		r.emit(line{indent: indent, segs: segs})
	}
	rule("╰", "┴", "╯")
}

// ── Pictures ────────────────────────────────────────────────────────────────

// finder turns a link in the file into a path on this disk: relative to the
// file, then to each org directory, the order plugs.MediaURL tries for the
// server.
type finder struct {
	dir   string
	roots []string
}

func (f *finder) find(target string) string {
	if f == nil {
		return ""
	}
	target = strings.TrimPrefix(strings.TrimPrefix(target, "file://"), "file:")
	if strings.HasPrefix(target, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			target = filepath.Join(home, target[2:])
		}
	}
	try := []string{}
	if filepath.IsAbs(target) {
		try = append(try, target)
	} else {
		try = append(try, filepath.Join(f.dir, target))
		for _, root := range f.roots {
			try = append(try, filepath.Join(root, target))
		}
	}
	for _, p := range try {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// image draws a picture with half blocks: each cell is two pixels, the top in
// the foreground colour of `▀` and the bottom in its background, which is
// square enough in any terminal font to be the picture rather than a hint of
// it. Pure go, no terminal graphics protocol, so it works over ssh, in tmux
// and in a pipe. What cannot be drawn - an svg, a url, a missing file - is a
// labelled frame where the picture would be.
func (r *renderer) image(l org.RegularLink, indent, width int) {
	path := ""
	if !strings.Contains(l.URL, "://") || strings.HasPrefix(l.URL, "file:") {
		path = r.files.find(l.URL)
	}
	if path != "" {
		if lines, ok := r.picture(path, width); ok {
			for _, ln := range lines {
				ln.indent = indent
				ln.center = true
				r.emit(ln)
			}
			return
		}
	}
	label := "▣  " + baseName(l.URL)
	why := "not found"
	switch {
	case strings.Contains(l.URL, "://") && !strings.HasPrefix(l.URL, "file:"):
		why = "on the web"
	case strings.HasSuffix(strings.ToLower(l.URL), ".svg"):
		why = "svg"
	case path != "":
		why = "unreadable"
	}
	label += "  ·  " + why
	w := min(width, runewidth.StringWidth(label)+6)
	frame := r.base().Foreground(r.p.rule)
	r.emit(line{indent: indent, center: true, segs: []seg{{"╭" + strings.Repeat("─", w-2) + "╮", frame}}})
	in := clip([]seg{{label, r.base().Foreground(r.p.soft)}}, w-6, frame)
	r.emit(line{indent: indent, center: true, segs: append(append([]seg{{"│  ", frame}}, in...),
		seg{strings.Repeat(" ", max(0, w-4-segsWidth(in)-2)) + "  │", frame})})
	r.emit(line{indent: indent, center: true, segs: []seg{{"╰" + strings.Repeat("─", w-2) + "╯", frame}}})
}

func (r *renderer) picture(path string, width int) ([]line, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	iw, ih := float64(b.Dx()), float64(b.Dy())
	if iw < 1 || ih < 1 {
		return nil, false
	}
	// Fit the width and the height allowance; a small picture is not blown
	// up past twice its size, where pixels become tiles.
	s := min(float64(width)/iw, float64(r.maxImg*2)/ih, 2.0)
	cols := max(1, int(iw*s))
	prows := max(2, int(ih*s))
	if prows%2 == 1 {
		prows++
	}
	ground := [3]float64{14, 17, 48}
	if gr, gg, gb := r.p.bg.RGB(); gr >= 0 {
		ground = [3]float64{float64(gr), float64(gg), float64(gb)}
	} else if !r.p.dark {
		ground = [3]float64{255, 255, 255}
	} else {
		ground = [3]float64{0, 0, 0}
	}
	// Area-average each target pixel over the source pixels it covers,
	// sampling at most a 4×4 grid of them: a box filter is what keeps a
	// screenshot's text from turning to noise when it shrinks.
	px := func(tx, ty int) tcell.Color {
		x0, x1 := float64(tx)/s, float64(tx+1)/s
		y0, y1 := float64(ty)/s, float64(ty+1)/s
		var sr, sg, sb, n float64
		for j := 0; j < 4; j++ {
			y := int(y0 + (y1-y0)*(float64(j)+0.5)/4)
			for i := 0; i < 4; i++ {
				x := int(x0 + (x1-x0)*(float64(i)+0.5)/4)
				cr, cg, cb, ca := img.At(b.Min.X+min(x, b.Dx()-1), b.Min.Y+min(y, b.Dy()-1)).RGBA()
				a := float64(ca) / 0xffff
				// RGBA is premultiplied, so the ground fills what alpha left.
				sr += float64(cr)/257 + ground[0]*(1-a)
				sg += float64(cg)/257 + ground[1]*(1-a)
				sb += float64(cb)/257 + ground[2]*(1-a)
				n++
			}
		}
		return rgb(sr/n, sg/n, sb/n)
	}
	out := []line{}
	for y := 0; y < prows; y += 2 {
		segs := make([]seg, 0, cols)
		for x := 0; x < cols; x++ {
			segs = append(segs, seg{"▀", tcell.StyleDefault.Foreground(px(x, y)).Background(px(x, y+1))})
		}
		out = append(out, line{segs: segs})
	}
	return out, true
}

// ── Source code ─────────────────────────────────────────────────────────────

// source draws a source block on the code ground, coloured by chroma in the
// style the theme names - the same style its html decks get from highlight.js,
// as near as the two libraries have names in common.
func (r *renderer) source(code, lang string, numbers bool, indent, width int) {
	code = strings.TrimRight(code, "\n ")
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		r.panel(strings.Split(code, "\n"), lang, numbers, indent, width)
		return
	}
	lexer = chroma.Coalesce(lexer)
	style := styles.Get(r.p.chroma)
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		r.panel(strings.Split(code, "\n"), lang, numbers, indent, width)
		return
	}
	bg := r.base().Background(r.p.codeBg).Foreground(r.p.codeInk)
	lines := [][]seg{{}}
	for _, tok := range it.Tokens() {
		st := bg
		e := style.Get(tok.Type)
		if e.Colour.IsSet() {
			st = st.Foreground(tcell.NewRGBColor(int32(e.Colour.Red()), int32(e.Colour.Green()), int32(e.Colour.Blue())))
		}
		if e.Bold == chroma.Yes {
			st = st.Bold(true)
		}
		if e.Italic == chroma.Yes {
			st = st.Italic(true)
		}
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				lines = append(lines, []seg{})
			}
			if p != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1], seg{p, st})
			}
		}
	}
	r.panelSegs(lines, lang, numbers, indent, width)
}

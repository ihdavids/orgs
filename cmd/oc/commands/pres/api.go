package pres

// What other terminal screens may use of the slide renderer: org text to
// styled lines, in a slide theme's colours. `orgs drill` draws its cards with
// it, so a card with a table, a source block or a picture looks in the
// terminal the way it does on a slide.

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/mattn/go-runewidth"
)

// Palette is a theme, resolved to terminal colours.
type Palette = palette

// UseTemplates says where theme files are: the configured templatePath, or the
// templates folder beside the binary.
func UseTemplates(configured string) { slides.SearchPath = templateDirs(configured) }

// LoadPalette is a slide theme by name; "" is the built-in one, "term" the
// terminal's own colours. ok is false when the name was not found.
func LoadPalette(name string) (*Palette, bool) { return loadPalette(name) }

// ThemeNames is every theme there is.
func ThemeNames() []string { return themeNames() }

func (p *palette) Base() tcell.Style      { return p.base() }
func (p *palette) Ink() tcell.Color       { return p.ink }
func (p *palette) Soft() tcell.Color      { return p.soft }
func (p *palette) Head() tcell.Color      { return p.head }
func (p *palette) Accent() tcell.Color    { return p.accent }
func (p *palette) Rule() tcell.Color      { return p.rule }
func (p *palette) CodeBg() tcell.Color    { return p.codeBg }
func (p *palette) Name() string           { return p.name }
func (p *palette) Dark() bool             { return p.dark }
func Mix(a, b tcell.Color, t float64) tcell.Color { return lerp(a, b, t) }

// Span is a run of text in one style.
type Span struct {
	Text  string
	Style tcell.Style
}

// Line is one row: spans from an indent, maybe centred, maybe on a panel
// (code, examples) BandW columns wide.
type Line struct {
	Spans  []Span
	Indent int
	Center bool
	Band   tcell.Style
	BandW  int
}

func (l Line) Width() int {
	w := 0
	for _, s := range l.Spans {
		w += runewidth.StringWidth(s.Text)
	}
	return w
}

func export(ls []line) []Line {
	out := make([]Line, 0, len(ls))
	for _, l := range ls {
		o := Line{Indent: l.indent, Center: l.center, Band: l.band, BandW: l.bandW}
		for _, s := range l.segs {
			o.Spans = append(o.Spans, Span{s.s, s.st})
		}
		out = append(out, o)
	}
	return out
}

// RenderOrg draws org text - paragraphs, lists, tables, source blocks,
// quotes, pictures, headings - into lines for a column `width` wide. file is
// where the text came from, so a picture named relative to it is found; roots
// are where else to look.
func RenderOrg(text, file string, width, maxImgRows int, p *Palette, roots []string) []Line {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	doc := org.New().Parse(strings.NewReader(text+"\n"), file)
	dir := "."
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		dir = file[:i]
	}
	r := newRenderer(p, slides.Conf{Doc: doc, Prefix: "TERM"}, &finder{dir: dir, roots: roots}, maxImgRows)
	r.blocks(doc.Nodes, 0, width, false)
	r.footnotes(width)
	// A leading blank line is the renderer's gap before a first block that
	// has nothing above it here.
	for len(r.out) > 0 && r.out[0].blank() {
		r.out = r.out[1:]
	}
	return export(r.out)
}

// RenderInline draws one line of org markup - a heading - wrapped to width,
// in the given style.
func RenderInline(text string, st tcell.Style, width int, p *Palette) []Line {
	doc := org.New().Parse(strings.NewReader(text+"\n"), "")
	r := newRenderer(p, slides.Conf{Doc: doc}, nil, 4)
	nodes := []org.Node{}
	for _, n := range doc.Nodes {
		switch v := n.(type) {
		case org.Paragraph:
			nodes = append(nodes, v.Children...)
		case *org.Paragraph:
			nodes = append(nodes, v.Children...)
		}
	}
	out := []line{}
	for _, ws := range wrap(r.inline(nodes, st), width) {
		out = append(out, line{segs: ws})
	}
	return export(out)
}

// DrawLine puts a line on a screen in a column starting at left, colW wide.
func DrawLine(scr tcell.Screen, left, y, colW int, l Line) {
	w, _ := scr.Size()
	x := left + l.Indent
	if l.Center {
		x = left + l.Indent + max(0, (colW-l.Indent-max(l.Width(), l.BandW))/2)
	}
	if l.BandW > 0 {
		for i := 0; i < l.BandW && x+i < w; i++ {
			scr.SetContent(x+i, y, ' ', nil, l.Band)
		}
	}
	for _, s := range l.Spans {
		for _, r := range s.Text {
			rw := runewidth.RuneWidth(r)
			if rw == 0 {
				continue
			}
			if x+rw > w {
				return
			}
			scr.SetContent(x, y, r, nil, s.Style)
			x += rw
		}
	}
}

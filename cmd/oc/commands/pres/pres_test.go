package pres

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/mattn/go-runewidth"
)

func deckOf(t *testing.T, src string) *deck {
	t.Helper()
	doc := org.New().Parse(strings.NewReader(src), "talk.org")
	if doc.Error != nil {
		t.Fatal(doc.Error)
	}
	return newDeck(doc, "talk.org")
}

// text is a page as plain text, every row trimmed.
func text(d *deck, i, step, w, h int) string {
	c := d.compose(i, step, w, h, view{pal: builtin(), maxWidth: 88})
	rows := []string{}
	for y := 0; y < c.c.h; y++ {
		rows = append(rows, ansiRow(c.c, y, false))
	}
	return strings.Join(rows, "\n")
}

func TestWrapNeverExceedsTheWidth(t *testing.T) {
	st := builtin().base()
	segs := []seg{{"a sentence with a ", st}, {"verylongwordthatcannotfitanywhere", st.Bold(true)}, {" and more words after it", st}}
	for _, w := range []int{8, 12, 20, 40} {
		for _, l := range wrap(segs, w) {
			if got := segsWidth(l); got > w {
				t.Errorf("width %d: line %q is %d wide", w, segsText(l), got)
			}
		}
	}
	// Nothing is lost but the spaces the breaks replaced.
	all := ""
	for _, l := range wrap(segs, 12) {
		all += segsText(l)
	}
	if strings.ReplaceAll(all, " ", "") != strings.ReplaceAll(segsText(segs), " ", "") {
		t.Errorf("wrap lost text: %q", all)
	}
}

func TestFragmentsStepAndNotesStayOff(t *testing.T) {
	d := deckOf(t, `#+TITLE: T
* Slide
:PROPERTIES:
:FRAGMENT: t
:END:
- one
- two
:NOTES:
secret words
:END:
`)
	if len(d.pages) != 2 {
		t.Fatalf("want title page and one slide, got %d pages", len(d.pages))
	}
	c := d.compose(1, 0, 80, 24, view{pal: builtin(), maxWidth: 88})
	if c.steps != 2 {
		t.Fatalf("want 2 steps, got %d", c.steps)
	}
	at0, at1, at2 := text(d, 1, 0, 80, 24), text(d, 1, 1, 80, 24), text(d, 1, 2, 80, 24)
	if strings.Contains(at0, "one") || !strings.Contains(at1, "one") || strings.Contains(at1, "two") || !strings.Contains(at2, "two") {
		t.Errorf("steps wrong:\n%s\n---\n%s\n---\n%s", at0, at1, at2)
	}
	// A revealed bullet must not move what is above it.
	if strings.Index(at1, "Slide") != strings.Index(at2, "Slide") {
		t.Error("revealing a step moved the title")
	}
	if strings.Contains(at2, "secret") {
		t.Error("speaker notes reached the slide")
	}
}

func TestSectionDividerAndTitlePage(t *testing.T) {
	d := deckOf(t, `#+TITLE: Hello
#+SLIDE_LEVEL: 2
* Part
** Inside
body
`)
	if len(d.pages) != 3 {
		t.Fatalf("want 3 pages, got %d", len(d.pages))
	}
	if s := text(d, 0, 0, 80, 24); !strings.Contains(s, "┣━┫") {
		t.Errorf("title not in the display face:\n%s", s)
	}
	if s := text(d, 2, 0, 80, 24); !strings.Contains(s, "body") {
		t.Errorf("slide body missing:\n%s", s)
	}
}

func TestBigFontRefusesWhatItCannotSet(t *testing.T) {
	if _, ok := big("naïve", 80); ok {
		t.Error("set a letter the face does not have")
	}
	if _, ok := big("HELLO", 10); ok {
		t.Error("set a word wider than the slide")
	}
	rows, ok := big("ab cd", 8)
	if !ok || len(rows) != 7 {
		t.Errorf("two lines of three rows and a gap, got %d rows ok=%v", len(rows), ok)
	}
	for r := range glyphs {
		g := glyphs[r]
		w := runewidth.StringWidth(g[0])
		if runewidth.StringWidth(g[1]) != w || runewidth.StringWidth(g[2]) != w {
			t.Errorf("glyph %q has rows of different widths", r)
		}
	}
}

func TestTableFitsAndAligns(t *testing.T) {
	d := deckOf(t, `* T
| name | n |
|------+---|
| a    | 1 |
| bb   | 22 |
`)
	s := text(d, 0, 0, 60, 20)
	if !strings.Contains(s, "│ a    │  1 │") {
		t.Errorf("numbers not right-aligned:\n%s", s)
	}
	for _, row := range strings.Split(text(d, 0, 0, 24, 20), "\n") {
		if runewidth.StringWidth(row) > 24 {
			t.Errorf("row wider than the screen: %q", row)
		}
	}
}

func TestThemeFromSlideCSS(t *testing.T) {
	old := slides.SearchPath
	slides.SearchPath = filepath.Join("..", "..", "..", "..", "templates")
	defer func() { slides.SearchPath = old }()
	p, ok := loadPalette("linen")
	if !ok {
		t.Skip("no linen theme in templates")
	}
	if p.dark {
		t.Error("linen is a light theme")
	}
	if r, g, b := p.bg.RGB(); r != 0xfa || g != 0xf7 || b != 0xf1 {
		t.Errorf("bg = %d,%d,%d", r, g, b)
	}
}

func TestCSSColour(t *testing.T) {
	ground := [3]float64{0, 0, 0}
	if r, g, b, ok := cssColour("rgba(255, 255, 255, 0.5)", ground); !ok || int(r) != 127 || int(g) != 127 || int(b) != 127 {
		t.Errorf("rgba blend: %v %v %v %v", r, g, b, ok)
	}
	if _, _, b, ok := cssColour("#00f", ground); !ok || b != 255 {
		t.Errorf("short hex: %v %v", b, ok)
	}
	if got := cssString(`"\25B8"`); got != "▸" {
		t.Errorf("css escape: %q", got)
	}
}

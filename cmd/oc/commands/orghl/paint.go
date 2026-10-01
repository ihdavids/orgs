package orghl

// Drawing the spans, two ways.
//
// The palette is one table rather than two so that a role cannot be gold in a
// listing and green in a form - the two drawings of the same thing are what
// a reader compares without meaning to. The hues are the Solarized accents,
// which is what most of the html themes in this tree are built on, and they are
// all mid-tone on purpose: a terminal may be a light one, and a highlighter
// that assumes a black background is unreadable on half of them.

import (
	"strings"

	"github.com/rivo/tview"
)

type style struct {
	// 256-colour sequence, and tview's own markup.
	ansi  string
	tview string
}

const (
	aReset = "\033[0m"
	aBold  = "\033[1m"
)

func fg(n string) string { return "\033[38;5;" + n + "m" }

var palette = map[Role]style{
	Plain:          {"", "[-:-:-]"},
	Stars:          {fg("61"), "[#6c71c4]"},
	Title:          {aBold, "[-::b]"},
	TodoKeyword:    {fg("166") + aBold, "[#cb4b16::b]"},
	DoneKeyword:    {fg("106") + aBold, "[#859900::b]"},
	Priority:       {fg("125"), "[#d33682]"},
	Tag:            {fg("37"), "[#2aa198]"},
	DrawerName:     {fg("240"), "[#5f5f5f]"},
	PropKey:        {fg("33"), "[#268bd2]"},
	Planning:       {fg("136"), "[#b58900]"},
	Timestamp:      {fg("136"), "[#b58900]"},
	Link:           {fg("33") + "\033[4m", "[#268bd2::u]"},
	LinkDesc:       {fg("37"), "[#2aa198]"},
	Comment:        {fg("240"), "[#5f5f5f]"},
	Directive:      {fg("61"), "[#6c71c4]"},
	DirectiveValue: {fg("245"), "[#8a8a8a]"},
	BlockMarker:    {fg("61"), "[#6c71c4]"},
	Code:           {fg("37"), "[#2aa198]"},
	Bold:           {aBold, "[-::b]"},
	Italic:         {"\033[3m", "[-::i]"},
	Underline:      {"\033[4m", "[-::u]"},
	Strike:         {"\033[9m", "[#8a8a8a::s]"},
	Bullet:         {fg("33"), "[#268bd2]"},
	Checkbox:       {fg("106"), "[#859900]"},
	TableSep:       {fg("240"), "[#5f5f5f]"},
	SrcText:        {fg("245"), "[#8a8a8a]"},
	Hole:           {fg("125"), "[#d33682]"},
}

// ANSI draws the spans with escape codes, or draws them plainly when the caller
// says colour is not wanted - which is the same question commands.Colour()
// answers, asked by the caller rather than here, because this package has no
// business knowing what the flags said.
func ANSI(spans []Span, colour bool) string {
	var b strings.Builder
	for _, s := range spans {
		if !colour {
			b.WriteString(s.Text)
			continue
		}
		st := palette[s.Role]
		if st.ansi == "" {
			b.WriteString(s.Text)
			continue
		}
		b.WriteString(st.ansi)
		b.WriteString(s.Text)
		b.WriteString(aReset)
	}
	return b.String()
}

// Tview draws the spans as tview markup, with the text escaped.
//
// The escaping is the part to get right rather than the colours: org text is
// full of `[[links]]` and `[2026-09-30 Wed]` timestamps, and every one of those
// is a colour tag as far as a TextView with dynamic colours on is concerned -
// so an unescaped timestamp does not come out the wrong colour, it comes out
// missing.
func Tview(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		st := palette[s.Role]
		if st.tview == "" {
			st = palette[Plain]
		}
		b.WriteString(st.tview)
		b.WriteString(tview.Escape(s.Text))
	}
	b.WriteString(palette[Plain].tview)
	return b.String()
}

// ANSIText is a whole passage, coloured, with the state carried down it.
func ANSIText(text string, st *State, colour bool) string {
	lines := []string{}
	for _, spans := range Lines(text, st) {
		lines = append(lines, ANSI(spans, colour))
	}
	return strings.Join(lines, "\n")
}

// TviewText is the same for a full-screen form.
func TviewText(text string, st *State) string {
	lines := []string{}
	for _, spans := range Lines(text, st) {
		lines = append(lines, Tview(spans))
	}
	return strings.Join(lines, "\n")
}

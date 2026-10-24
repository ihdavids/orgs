package help

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"golang.org/x/term"
)

// width is what paragraphs are wrapped to: the terminal's, kept to a
// readable measure, else 80.
func width() int {
	w := 80
	if c, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && c > 20 {
		w = c
	}
	if w > 100 {
		w = 100
	}
	return w
}

type word struct {
	shown string // with its escapes
	n     int    // what it takes on screen
}

// styled is a line of org as words carrying their colour, so wrapping counts
// only what shows.
func styled(s string) []word {
	out := []word{}
	glue := false // the next word joins the last (no space between them)
	for _, sp := range inline(s) {
		on, off := "", ""
		switch sp.kind {
		case spCode:
			on, off = commands.C(commands.AnsiCyan), commands.C(commands.AnsiReset)
		case spBold:
			on, off = commands.C(commands.AnsiBold), commands.C(commands.AnsiReset)
		}
		text := sp.s
		lead := strings.HasPrefix(text, " ")
		if sp.kind != spText {
			// Code keeps its spaces: it is one piece on the line.
			text = strings.ReplaceAll(text, " ", " ")
		}
		parts := strings.Fields(text)
		for i, p := range parts {
			w := word{shown: on + strings.ReplaceAll(p, " ", " ") + off, n: utf8.RuneCountInString(p)}
			if i == 0 && glue && !lead && len(out) > 0 {
				out[len(out)-1].shown += w.shown
				out[len(out)-1].n += w.n
			} else {
				out = append(out, w)
			}
		}
		glue = len(parts) > 0 && !strings.HasSuffix(text, " ")
	}
	return out
}

// wrap lays words out at indent, the first line after first (a bullet).
func wrap(b *strings.Builder, words []word, indent int, first string, w int) {
	pad := strings.Repeat(" ", indent)
	b.WriteString(pad + first)
	col := indent + utf8.RuneCountInString(first)
	start := col
	for i, wd := range words {
		if i > 0 {
			if col+1+wd.n > w && col > start {
				b.WriteString("\n" + pad + strings.Repeat(" ", utf8.RuneCountInString(first)))
				col = start
			} else {
				b.WriteString(" ")
				col++
			}
		}
		b.WriteString(wd.shown)
		col += wd.n
	}
	b.WriteString("\n")
}

// renderTerm draws org text for a terminal, everything indented by indent.
// Headings at level one are gold, deeper ones bold.
func renderTerm(text string, indent int) string {
	b := &strings.Builder{}
	w := width()
	prev := elemKind(-1)
	for _, e := range parseOrg(text) {
		// A blank line between elements, except between items of one list.
		if prev >= 0 && !(prev == elItem && e.kind == elItem) {
			b.WriteString("\n")
		}
		switch e.kind {
		case elHeading:
			colour := commands.AnsiBold
			if e.level == 1 {
				colour = commands.AnsiGold
			}
			title := strings.Builder{}
			for i, wd := range styled(e.text) {
				if i > 0 {
					title.WriteString(" ")
				}
				// A styled word resets the colour; the heading's is put back after it.
				title.WriteString(strings.ReplaceAll(wd.shown, commands.C(commands.AnsiReset), commands.C(commands.AnsiReset)+commands.C(colour)))
			}
			b.WriteString(strings.Repeat(" ", indent+2*(e.level-1)) + commands.C(colour) + title.String() + commands.C(commands.AnsiReset) + "\n")
		case elPara:
			wrap(b, styled(e.text), indent, "", w)
		case elItem:
			wrap(b, styled(e.text), indent+2*e.level, "• ", w)
		case elPre:
			for _, l := range e.lines {
				if e.level == 1 {
					// A table reads better in the text's own colour.
					b.WriteString(strings.Repeat(" ", indent+2) + l + "\n")
				} else {
					b.WriteString(strings.Repeat(" ", indent+2) + commands.C(commands.AnsiDim) + l + commands.C(commands.AnsiReset) + "\n")
				}
			}
		}
		prev = e.kind
	}
	return b.String()
}

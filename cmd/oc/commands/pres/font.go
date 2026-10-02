package pres

// The display face: three rows of box-drawing characters, for the title slide
// and the slides that only open a section.
//
// A terminal has one size of type, and a talk's title written in it looks like
// one more line of text. These letters are drawn with the thin and heavy box
// characters every monospaced font carries, so they line up in any terminal
// without a font of their own, and three rows is short enough that a two-line
// title still leaves room on the slide.

import (
	"strings"
	"unicode/utf8"
)

var glyphs = map[rune][3]string{
	'A': {"┏━┓", "┣━┫", "╹ ╹"}, 'B': {"┏┓ ", "┣┻┓", "┗━┛"}, 'C': {"┏━╸", "┃  ", "┗━╸"},
	'D': {"╺┳┓", " ┃┃", "╺┻┛"}, 'E': {"┏━╸", "┣╸ ", "┗━╸"}, 'F': {"┏━╸", "┣╸ ", "╹  "},
	'G': {"┏━╸", "┃╺┓", "┗━┛"}, 'H': {"╻ ╻", "┣━┫", "╹ ╹"}, 'I': {"╻", "┃", "╹"},
	'J': {"  ╻", "  ┃", "┗━┛"}, 'K': {"╻┏ ", "┣┻┓", "╹ ╹"}, 'L': {"╻  ", "┃  ", "┗━╸"},
	'M': {"┏┳┓", "┃┃┃", "╹ ╹"}, 'N': {"┏┓╻", "┃┗┫", "╹ ╹"}, 'O': {"┏━┓", "┃ ┃", "┗━┛"},
	'P': {"┏━┓", "┣━┛", "╹  "}, 'Q': {"┏━┓", "┃┓┃", "┗┻┛"}, 'R': {"┏━┓", "┣┳┛", "╹┗╸"},
	'S': {"┏━┓", "┗━┓", "┗━┛"}, 'T': {"╺┳╸", " ┃ ", " ╹ "}, 'U': {"╻ ╻", "┃ ┃", "┗━┛"},
	'V': {"╻ ╻", "┃┏┛", "┗┛ "}, 'W': {"╻ ╻", "┃╻┃", "┗┻┛"}, 'X': {"╻ ╻", "┏╋┛", "╹ ╹"},
	'Y': {"╻ ╻", "┗┳┛", " ╹ "}, 'Z': {"╺━┓", "┏━┛", "┗━╸"},
	'0': {"┏━┓", "┃┃┃", "┗━┛"}, '1': {"╺┓ ", " ┃ ", "╺┻╸"}, '2': {"┏━┓", "┏━┛", "┗━╸"},
	'3': {"┏━┓", "╺━┫", "┗━┛"}, '4': {"╻ ╻", "┗━┫", "  ╹"}, '5': {"┏━╸", "┗━┓", "┗━┛"},
	'6': {"┏━┓", "┣━┓", "┗━┛"}, '7': {"┏━┓", "  ┃", "  ╹"}, '8': {"┏━┓", "┣━┫", "┗━┛"},
	'9': {"┏━┓", "┗━┫", "┗━┛"},
	' ': {"  ", "  ", "  "}, '.': {" ", " ", "╹"}, ',': {" ", " ", "┛"},
	'!': {"╻", "┃", "╹"}, '?': {"┏━┓", " ┏┛", " ╹ "}, '-': {"   ", "╺━╸", "   "},
	':': {" ", "╹", "╹"}, '\'': {"╹", " ", " "}, '&': {"┏┓ ", "┣╋╸", "┗┛ "},
	'+': {"   ", "╺╋╸", "   "}, '/': {"  ╻", " ┏┛", "╺┛ "}, '(': {"┏╸", "┃ ", "┗╸"},
	')': {"╺┓", " ┃", "╺┛"},
}

// bigWidth is how many columns a word takes in the display face, or -1 when it
// has a letter the face does not have.
func bigWidth(s string) int {
	w := 0
	n := 0
	for _, r := range strings.ToUpper(s) {
		g, ok := glyphs[r]
		if !ok {
			return -1
		}
		if n > 0 {
			w++
		}
		w += utf8.RuneCountInString(g[0])
		n++
	}
	return w
}

// big sets a title in the display face, broken into lines of whole words that
// fit the width. ok is false when it cannot be done - a letter the face lacks,
// or a word wider than the slide - and the caller sets the title in ordinary
// type instead.
func big(title string, width int) (rows []string, ok bool) {
	words := strings.Fields(title)
	if len(words) == 0 {
		return nil, false
	}
	lines := [][]string{}
	cur := []string{}
	for _, w := range words {
		ww := bigWidth(w)
		if ww < 0 || ww > width {
			return nil, false
		}
		try := strings.Join(append(append([]string{}, cur...), w), " ")
		if len(cur) > 0 && bigWidth(try) > width {
			lines = append(lines, cur)
			cur = nil
		}
		cur = append(cur, w)
	}
	lines = append(lines, cur)
	if len(lines) > 3 {
		// Four lines of display type is a paragraph, not a title.
		return nil, false
	}
	for li, words := range lines {
		if li > 0 {
			rows = append(rows, "")
		}
		var b [3]strings.Builder
		n := 0
		for _, r := range strings.ToUpper(strings.Join(words, " ")) {
			g := glyphs[r]
			for i := range b {
				if n > 0 {
					b[i].WriteByte(' ')
				}
				b[i].WriteString(g[i])
			}
			n++
		}
		for i := range b {
			rows = append(rows, b[i].String())
		}
	}
	return rows, true
}

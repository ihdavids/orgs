package help

// Just enough org to show the documentation: headings, paragraphs, lists,
// blocks and tables, with =code=, ~code~, *bold* and links inline. It is
// read into elements once and written two ways - for a terminal and as roff
// for man - so the two show the same thing.

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type elemKind int

const (
	elHeading elemKind = iota
	elPara
	elItem
	elPre
)

type elem struct {
	kind  elemKind
	level int      // a heading's level, an item's depth (0 = outermost)
	text  string   // a heading's title, a paragraph's or item's text (joined)
	lines []string // a block's or table's lines, as written
}

var (
	headLineRe  = regexp.MustCompile(`^(\*+)\s+(.*?)\s*$`)
	itemLineRe  = regexp.MustCompile(`^(\s*)([-+]|\d+[.)])\s+(.*)$`)
	beginRe     = regexp.MustCompile(`(?i)^\s*#\+begin_`)
	endRe       = regexp.MustCompile(`(?i)^\s*#\+end_`)
	imageOnlyRe = regexp.MustCompile(`(?i)^\[\[(file:)?[^\]]+\.(png|jpe?g|gif|svg|webp)\]\]$`)
)

func indentOf(l string) int {
	return len(l) - len(strings.TrimLeft(l, " \t"))
}

// parseOrg reads org text into the elements it is drawn from. Keyword lines
// (#+TITLE...) and comments are left out, and so is a paragraph that is only
// a picture, which neither a terminal nor man can show.
func parseOrg(text string) []elem {
	out := []elem{}
	lines := strings.Split(strings.ReplaceAll(text, "\t", "    "), "\n")
	itemIndents := []int{}
	for i := 0; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " ")
		t := strings.TrimSpace(l)
		last := func() *elem {
			if len(out) == 0 {
				return nil
			}
			return &out[len(out)-1]
		}
		switch {
		case t == "":
			// A blank line ends a paragraph; an item can go on after one
			// only by its indent, which is checked below.
			if e := last(); e != nil && e.kind == elPara {
				out = append(out, elem{kind: -1})
			}
		case beginRe.MatchString(l):
			block := []string{}
			for i++; i < len(lines) && !endRe.MatchString(lines[i]); i++ {
				block = append(block, strings.TrimRight(strings.ReplaceAll(lines[i], "\t", "    "), " "))
			}
			out = append(out, elem{kind: elPre, lines: dedent(block)})
		case strings.HasPrefix(t, "#+") || strings.HasPrefix(t, "# ") || t == "#":
		case headLineRe.MatchString(l) && !strings.HasPrefix(l, " "):
			m := headLineRe.FindStringSubmatch(l)
			out = append(out, elem{kind: elHeading, level: len(m[1]), text: m[2]})
			itemIndents = itemIndents[:0]
		case strings.HasPrefix(t, "|"):
			if e := last(); e != nil && e.kind == elPre && e.level == 1 {
				e.lines = append(e.lines, t)
			} else {
				out = append(out, elem{kind: elPre, level: 1, lines: []string{t}})
			}
		case itemLineRe.MatchString(l):
			m := itemLineRe.FindStringSubmatch(l)
			ind := len(m[1])
			for len(itemIndents) > 0 && itemIndents[len(itemIndents)-1] > ind {
				itemIndents = itemIndents[:len(itemIndents)-1]
			}
			if len(itemIndents) == 0 || itemIndents[len(itemIndents)-1] < ind {
				itemIndents = append(itemIndents, ind)
			}
			out = append(out, elem{kind: elItem, level: len(itemIndents) - 1, text: strings.TrimSpace(m[3])})
		default:
			e := last()
			// More text under an item, indented past its bullet.
			if e != nil && e.kind == elItem && len(itemIndents) > 0 && indentOf(l) > itemIndents[len(itemIndents)-1] {
				e.text += " " + t
				continue
			}
			itemIndents = itemIndents[:0]
			if e != nil && e.kind == elPara {
				e.text += " " + t
			} else {
				out = append(out, elem{kind: elPara, text: t})
			}
		}
	}
	// Drop the paragraph breaks used while reading, and lone pictures.
	clean := []elem{}
	for _, e := range out {
		if e.kind < 0 || (e.kind == elPara && imageOnlyRe.MatchString(e.text)) {
			continue
		}
		clean = append(clean, e)
	}
	return clean
}

func dedent(lines []string) []string {
	min := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if ind := indentOf(l); min < 0 || ind < min {
			min = ind
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= min && min > 0 {
			l = l[min:]
		}
		// org escapes a line that would read as a heading or keyword with a comma.
		if strings.HasPrefix(l, ",*") || strings.HasPrefix(l, ",#+") {
			l = l[1:]
		}
		out[i] = l
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

type spanKind int

const (
	spText spanKind = iota
	spCode
	spBold
)

type span struct {
	kind spanKind
	s    string
}

var linkRe = regexp.MustCompile(`^\[\[([^\]]+)\](?:\[([^\]]+)\])?\]`)

// Org's rule for emphasis, near enough: the marker starts after a space or
// opening punctuation, ends before a space or closing punctuation, and the
// text inside neither starts nor ends with a space.
func openOK(prev rune) bool {
	return prev == 0 || unicode.IsSpace(prev) || strings.ContainsRune(`({["'-`, prev)
}

func closeOK(next rune) bool {
	return next == 0 || unicode.IsSpace(next) || strings.ContainsRune(`.,;:!?)}]"'-`, next)
}

// inline reads a line of org into plain, code and bold spans; a link becomes
// its description (else its target).
func inline(s string) []span {
	out := []span{}
	text := strings.Builder{}
	flush := func() {
		if text.Len() > 0 {
			out = append(out, span{spText, text.String()})
			text.Reset()
		}
	}
	prev := rune(0)
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if r == '[' {
			if m := linkRe.FindStringSubmatch(s[i:]); m != nil {
				shown := m[2]
				if shown == "" {
					shown = strings.TrimPrefix(m[1], "file:")
				}
				text.WriteString(shown)
				i += len(m[0])
				prev = ']'
				continue
			}
		}
		if (r == '=' || r == '~' || r == '*') && openOK(prev) {
			if end := closeAt(s, i, r); end > 0 {
				flush()
				kind := spCode
				if r == '*' {
					kind = spBold
				}
				out = append(out, span{kind, s[i+1 : end]})
				i = end + 1
				prev = r
				continue
			}
		}
		text.WriteRune(r)
		prev = r
		i += w
	}
	flush()
	return out
}

// closeAt is where the marker opened at i closes, or -1.
func closeAt(s string, i int, marker rune) int {
	if i+1 >= len(s) || s[i+1] == ' ' {
		return -1
	}
	for j := i + 2; j < len(s); j++ {
		if rune(s[j]) != marker || s[j-1] == ' ' {
			continue
		}
		next := rune(0)
		if j+1 < len(s) {
			next, _ = utf8.DecodeRuneInString(s[j+1:])
		}
		if closeOK(next) {
			return j
		}
	}
	return -1
}

// plainText is a line of org as words, no markup.
func plainText(s string) string {
	b := strings.Builder{}
	for _, sp := range inline(s) {
		b.WriteString(sp.s)
	}
	return b.String()
}

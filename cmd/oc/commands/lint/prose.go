package lint

// The prose in an org file, as paragraphs a checker can read, each character
// remembering the line and byte column it came from.
//
// Neither checker knows org. Given a file as it stands, Harper reads
// `:PROPERTIES:` as a misspelling, LanguageTool calls `#+begin_src` a
// sentence fragment, and every link target is a spelling mistake. worg's
// writing mode reads only prose (writing.ts `proseSpans`, gathered into
// paragraphs by prose.ts `paragraphsOf`); this is the same rule in Go, so
// the terminal and the editor flag the same words. The pair drifts silently:
// a kind of line skipped on one side and read on the other shows up as a
// lint in one client and not the other, nothing more.
//
//   - Read: paragraph lines, and the insides of quote, verse and center
//     blocks. A paragraph ends at a blank line or at anything that is not
//     prose; a list item starts a paragraph of its own, without its bullet.
//   - Never read: headings (worg leaves them to the outline), drawers,
//     planning lines, `#+` keywords and comments, tables, fixed width lines,
//     and every other block.
//   - Taken out of a line: link targets (the description stays), timestamps,
//     footnote references, verbatim and code, inline src, macros, statistics
//     cookies, bare urls and mail addresses, and the markers of emphasis.
//     A hole in the middle of a line becomes one space, so the words either
//     side of it do not run together.

import (
	"regexp"
	"strings"
	"unicode/utf16"
)

// pos is a place in the file: a 0 based line and a 0 based byte column.
type pos struct{ line, col int }

type para struct {
	text []rune
	at   []pos
	// u16[i] is how many UTF-16 units come before text[i]: both checkers
	// count in those (LSP's default, and Java's chars), so an offset has to
	// be walked back to a rune before it means anything here.
	u16 []int
}

func (p *para) add(r rune, at pos) {
	p.text = append(p.text, r)
	p.at = append(p.at, at)
}

func (p *para) index() {
	p.u16 = make([]int, len(p.text)+1)
	n := 0
	for i, r := range p.text {
		p.u16[i] = n
		n += len(utf16.Encode([]rune{r}))
	}
	p.u16[len(p.text)] = n
}

// place is where a UTF-16 offset into the paragraph lands in the file.
func (p *para) place(off int) pos {
	if len(p.at) == 0 {
		return pos{}
	}
	lo, hi := 0, len(p.text)
	for lo < hi {
		mid := (lo + hi) / 2
		if p.u16[mid] < off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= len(p.at) {
		lo = len(p.at) - 1
	}
	return p.at[lo]
}

var (
	headingRe  = regexp.MustCompile(`^\*+\s`)
	drawerRe   = regexp.MustCompile(`^\s*:[\w-]+:\s*$`)
	drawerEnd  = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
	beginRe    = regexp.MustCompile(`(?i)^\s*#\+begin_(\w+)`)
	endRe      = regexp.MustCompile(`(?i)^\s*#\+end_(\w+)`)
	keywordRe  = regexp.MustCompile(`^\s*#(\+|\s|$)`)
	planningRe = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	propLineRe = regexp.MustCompile(`^\s*:[\w-]+:`)
	fixedRe    = regexp.MustCompile(`^\s*:(\s|$)`)
	tableRe    = regexp.MustCompile(`^\s*\|`)
	bulletRe   = regexp.MustCompile(`^(\s*)([-+]|\d+[.)]|\s\*)\s+(\[[ xX-]\]\s+)?`)

	// [[target][description]] and [[target]]: group 2 is the description.
	linkRe = regexp.MustCompile(`\[\[[^\]]*\](?:\[([^\]]*)\])?\]`)
	holeRes = []*regexp.Regexp{
		regexp.MustCompile(`[<\[]\d{4}-\d\d-\d\d[^>\]\n]*[>\]](?:--[<\[]\d{4}-\d\d-\d\d[^>\]\n]*[>\]])?`),
		regexp.MustCompile(`\[fn:[^\]]*\]`),
		regexp.MustCompile(`(?:^|[\s({'"])([=~])[^\s=~](?:.*?[^\s])?([=~])`),
		regexp.MustCompile(`src_\w+(?:\[[^\]]*\])?\{[^}]*\}`),
		regexp.MustCompile(`\{\{\{[^}]*\}\}\}`),
		regexp.MustCompile(`\[\d*/\d*\]|\[\d+%\]`),
		regexp.MustCompile(`\b(?:https?|ftp|file|mailto):[^\s\]\[<>"]+|\b[\w.+-]+@[\w-]+\.[\w.-]+`),
	}
	// Emphasis: only the markers go, the words inside are prose.
	emphRe = regexp.MustCompile(`(?:^|[\s({'"-])([*/_+])[^\s*/_+](?:[^*/_+\n]*?[^\s*/_+])?([*/_+])(?:$|[\s)}'".,:;!?-])`)
)

// proseBlocks are the blocks whose insides are read.
var proseBlocks = map[string]bool{"quote": true, "verse": true, "center": true}

// paragraphs reads lines [from, to) of a file.
func paragraphs(lines []string, from, to int) []*para {
	out := []*para{}
	var cur *para
	end := func() {
		if cur != nil && strings.TrimSpace(string(cur.text)) != "" {
			cur.index()
			out = append(out, cur)
		}
		cur = nil
	}
	drawer := false
	block := ""
	for i := from; i < to && i < len(lines); i++ {
		l := strings.TrimRight(lines[i], "\r")
		switch {
		case drawer:
			if drawerEnd.MatchString(l) {
				drawer = false
			}
			end()
			continue
		case block != "":
			if m := endRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[1], block) {
				block = ""
				end()
				continue
			}
			if !proseBlocks[strings.ToLower(block)] {
				end()
				continue
			}
		case strings.TrimSpace(l) == "", headingRe.MatchString(l):
			end()
			continue
		case drawerRe.MatchString(l) && !drawerEnd.MatchString(l):
			drawer = true
			end()
			continue
		}
		if m := beginRe.FindStringSubmatch(l); m != nil {
			block = m[1]
			end()
			continue
		}
		if keywordRe.MatchString(l) || planningRe.MatchString(l) || propLineRe.MatchString(l) ||
			fixedRe.MatchString(l) || tableRe.MatchString(l) {
			end()
			continue
		}
		start := len(l) - len(strings.TrimLeft(l, " \t"))
		if m := bulletRe.FindStringIndex(l); m != nil {
			end()
			start = m[1]
		}
		mask := proseMask(l, start)
		first := true
		gap := false
		for c, r := range l {
			if !mask[c] {
				if !first {
					gap = true
				}
				continue
			}
			if cur != nil && (first || gap) && (r == ' ' || r == '\t') && spaced(cur, 'x') {
				// The space after a hole, when the one before it is already
				// in: `and =code= text` reads as `and text`.
				first, gap = false, false
				continue
			}
			if cur == nil {
				cur = &para{}
			} else if (first || gap) && !spaced(cur, r) {
				// A new line, or a hole in this one: one space, placed where
				// the prose picks up again - unless there is one already
				// either side, or the checker sees two spaces the writer
				// never typed.
				cur.add(' ', pos{i, c})
			}
			first, gap = false, false
			cur.add(r, pos{i, c})
		}
	}
	end()
	return out
}

// proseMask says, for each byte of a line, whether it is prose.
func proseMask(l string, start int) []bool {
	mask := make([]bool, len(l))
	for c := start; c < len(l); c++ {
		mask[c] = true
	}
	hole := func(a, b int) {
		for c := a; c < b && c < len(l); c++ {
			mask[c] = false
		}
	}
	for _, m := range linkRe.FindAllStringSubmatchIndex(l, -1) {
		if m[2] >= 0 {
			hole(m[0], m[2])
			hole(m[3], m[1])
		} else {
			hole(m[0], m[1])
		}
	}
	for _, re := range holeRes {
		for _, m := range re.FindAllStringSubmatchIndex(l, -1) {
			if len(m) >= 6 && m[2] >= 0 {
				// The verbatim pattern takes the character before it to
				// find a word boundary; the hole starts at the marker.
				hole(m[2], m[1])
			} else {
				hole(m[0], m[1])
			}
		}
	}
	for _, m := range emphRe.FindAllStringSubmatchIndex(l, -1) {
		hole(m[2], m[3])
		hole(m[4], m[5])
	}
	return mask
}

func spaced(p *para, next rune) bool {
	if next == ' ' || next == '\t' {
		return true
	}
	if n := len(p.text); n > 0 && (p.text[n-1] == ' ' || p.text[n-1] == '\t') {
		return true
	}
	return false
}

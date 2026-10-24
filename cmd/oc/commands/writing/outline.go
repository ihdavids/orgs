package writing

// Enough of worg's outline (writing.ts headingsOf / headingTitle) to take a
// snapshot the way writing mode takes one: the heading and everything under
// it as Text, keyed by its path of titles from the top. The path is what worg
// files a snapshot under, so one taken here has to spell it the same way or
// writing mode lists it under "other headings" rather than this one's.
// Pinned by outline_test.go; change with writing.ts.

import (
	"regexp"
	"strings"
)

var (
	headingRe    = regexp.MustCompile(`^(\*+)(\s+)(.*)$`)
	blockBeginRe = regexp.MustCompile(`^(\s*)(#\+(?:BEGIN|begin)_([A-Za-z0-9_-]+))(.*)$`)
	blockEndRe   = regexp.MustCompile(`^(\s*)(#\+(?:END|end)_([A-Za-z0-9_-]+))\s*$`)
	tagsEndRe    = regexp.MustCompile(`\s+(:[\w@#%:]+:)\s*$`)
	firstWordRe  = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_@-]*)(\s+|$)`)
	commentRe    = regexp.MustCompile(`^COMMENT\s+`)
	priorityRe   = regexp.MustCompile(`^\[#[A-Za-z0-9]\]\s*`)
	// What writing mode counts as a snapshot's words (WritingParts.tsx).
	snapWordRe = regexp.MustCompile(`\p{L}[\p{L}'’]*`)
)

// Node is one heading of a file.
type Node struct {
	Line  int // zero based
	Level int
	Title string
	End   int // the subtree's last line, inclusive
	Path  string
}

// HeadingTitle is the title with stars, keyword, COMMENT, priority and tags
// taken off.
func HeadingTitle(text string, keywords map[string]bool) string {
	t := text
	if h := headingRe.FindStringSubmatch(text); h != nil {
		t = h[3]
	}
	t = tagsEndRe.ReplaceAllString(t, "")
	if m := firstWordRe.FindStringSubmatch(t); m != nil && keywords[m[1]] {
		t = t[len(m[0]):]
	}
	t = commentRe.ReplaceAllString(t, "")
	t = priorityRe.ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}

func lastLine(lines []string) int {
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		return len(lines) - 2
	}
	return len(lines) - 1
}

// Outline is every heading in file order. A line of stars in a block is not
// one.
func Outline(lines []string, keywords map[string]bool) []Node {
	type head struct{ line, level int }
	heads := []head{}
	block := ""
	for i, l := range lines {
		if block != "" {
			if e := blockEndRe.FindStringSubmatch(l); e != nil && strings.ToLower(e[3]) == block {
				block = ""
			}
			continue
		}
		if b := blockBeginRe.FindStringSubmatch(l); b != nil {
			block = strings.ToLower(b[3])
			continue
		}
		if h := headingRe.FindStringSubmatch(l); h != nil {
			heads = append(heads, head{i, len(h[1])})
		}
	}
	last := lastLine(lines)
	stack := []string{}
	out := make([]Node, 0, len(heads))
	for k, h := range heads {
		end := last
		for j := k + 1; j < len(heads); j++ {
			if heads[j].level <= h.level {
				end = heads[j].line - 1
				break
			}
		}
		if end < h.line {
			end = h.line
		}
		title := HeadingTitle(lines[h.line], keywords)
		if len(stack) > h.level-1 {
			stack = stack[:h.level-1]
		}
		for len(stack) < h.level-1 {
			stack = append(stack, "")
		}
		stack = append(stack, title)
		out = append(out, Node{Line: h.line, Level: h.level, Title: title, End: end, Path: strings.Join(stack, "\x00")})
	}
	return out
}

// SnapWords is a snapshot's word count, as writing mode counts it.
func SnapWords(text string) int { return len(snapWordRe.FindAllString(text, -1)) }

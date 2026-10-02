package common

// Clozes: the hidden words on a flashcard.
//
// A cloze is `[hidden text]` or `[hidden text||hint]`, on one line. Shared by
// the server, which wraps them in html for worg, and `orgs drill`, which draws
// them in the terminal, because both have to number them the same way: the
// card types that hide "the first" or "one at random" choose by number, and a
// client counting differently from the server would hide the wrong one.
//
// Deliberately not org-drill on one point: org-drill takes *any* bracketed
// text for a cloze, so a checkbox, a progress cookie, an inactive timestamp or
// a footnote reference on a card is hidden as if it were the answer. These are
// left alone here, as are links, code and LaTeX, and lines inside blocks,
// drawers and comments.

import (
	"regexp"
	"strings"
)

var (
	clozeRe    = regexp.MustCompile(`\[([^\[\]\n]+?)(?:\|\|([^\[\]\n]+?))?\]`)
	notClozeRe = regexp.MustCompile(`^\[(?:[ Xx-]|\d*/\d*|\d+%|fn:[^\]]*|#[A-Za-z0-9]|\d{4}-\d{2}-\d{2}[^\]]*)\]$`)
	guardedRe  = regexp.MustCompile(`\[\[[^\]]*\](?:\[[^\]]*\])?\]|=[^=\s][^=]*=|~[^~\s][^~]*~|\\\[.*?\\\]|\$[^$]+\$`)
)

// Cloze is one, as found.
type Cloze struct {
	// N is its number: from `first` in the order written, or 0 for all of
	// them when first is 0 - which is how the clozes in a card's title and
	// sides are marked "always hidden in the question".
	N    int
	Text string
	Hint string
}

// Clozes rewrites every cloze in org text through wrap, and says how many
// there were.
func Clozes(text string, first int, wrap func(Cloze) string) (string, int) {
	n := 0
	lines := strings.Split(text, "\n")
	inBlock := false
	for li, line := range lines {
		t := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(t, "#+begin_") {
			inBlock = true
		}
		if strings.HasPrefix(t, "#+end_") {
			inBlock = false
			continue
		}
		if inBlock || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ":") {
			continue
		}
		guards := guardedRe.FindAllStringIndex(line, -1)
		guarded := func(a, b int) bool {
			for _, g := range guards {
				if a < g[1] && b > g[0] {
					return true
				}
			}
			return false
		}
		b := strings.Builder{}
		last := 0
		for _, m := range clozeRe.FindAllStringSubmatchIndex(line, -1) {
			if guarded(m[0], m[1]) || notClozeRe.MatchString(line[m[0]:m[1]]) {
				continue
			}
			c := Cloze{Text: line[m[2]:m[3]]}
			if first > 0 {
				c.N = first + n
			}
			if m[4] >= 0 {
				c.Hint = line[m[4]:m[5]]
			}
			n++
			b.WriteString(line[last:m[0]])
			b.WriteString(wrap(c))
			last = m[1]
		}
		b.WriteString(line[last:])
		lines[li] = b.String()
	}
	return strings.Join(lines, "\n"), n
}

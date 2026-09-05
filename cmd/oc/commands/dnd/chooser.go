package dnd

// The list chooser used by every question in the builder.
//
// survey's own Select filters by plain substring over the whole rendered label
// - which includes the one line summary - and it cannot be swapped out, so the
// chooser is our own prompt instead. It keeps the survey keys (arrows, enter,
// space to tick, ? for help) and adds:
//
//   - fuzzy matching, so "eldbl" finds "Eldritch Blast" without typing it out,
//   - matching on the option *name* first, so a word that only appears in some
//     other option's summary does not drag it to the top,
//   - a filter you can edit: backspace a character, ctrl+w a word, escape the
//     lot, and the list widens again as you go,
//   - a match count beside the filter, and a line telling you when nothing
//     matches rather than an empty screen.
//
// Rendering is shared with the two pane view in panes.go.

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/AlecAivazis/survey.v1/core"
	"gopkg.in/AlecAivazis/survey.v1/terminal"
)

// chooser is a survey.Prompt: a filtered list, single or multiple choice.
type chooser struct {
	core.Renderer
	message  string
	options  []string // every label, in the order the server sent them
	help     string
	pageSize int
	multi    bool

	names   []string // option name only, what the filter matches first
	filter  string
	matches []int // indexes into options, in match order
	sel     int   // cursor position within matches
	checked map[string]bool
	showHlp bool
}

func newChooser(message string, options []string, help string, pageSize int,
	def string, defaults []string, multi bool, ctx *paneCtx) *chooser {
	c := &chooser{
		message: message, options: options, help: help,
		pageSize: pageSize, multi: multi, checked: map[string]bool{},
	}
	if c.pageSize <= 0 {
		c.pageSize = panePageSize()
	}
	for _, l := range options {
		name := l
		if ctx != nil {
			if o, ok := ctx.byLabel[l]; ok {
				name = o.Name
			} else {
				name = paneName(l)
			}
		}
		c.names = append(c.names, name)
	}
	for _, d := range defaults {
		c.checked[d] = true
	}
	c.refilter("")
	if def != "" {
		for i, l := range options {
			if l == def {
				c.selectOption(i)
				break
			}
		}
	}
	return c
}

// selectOption puts the cursor on an option index, if it is still matched.
func (c *chooser) selectOption(idx int) {
	for i, m := range c.matches {
		if m == idx {
			c.sel = i
			return
		}
	}
}

// current is the option index under the cursor, or -1 when nothing matches.
func (c *chooser) current() int {
	if c.sel < 0 || c.sel >= len(c.matches) {
		return -1
	}
	return c.matches[c.sel]
}

// refilter rebuilds the match list, keeping the cursor on the same option when
// that option survives the new filter.
func (c *chooser) refilter(filter string) {
	was := c.current()
	c.filter = filter
	c.matches = fuzzyMatches(filter, c.options, c.names)
	c.sel = 0
	if was >= 0 {
		c.selectOption(was)
	}
	if c.sel >= len(c.matches) {
		c.sel = len(c.matches) - 1
	}
	if c.sel < 0 {
		c.sel = 0
	}
}

func (c *chooser) move(delta int) {
	if len(c.matches) == 0 {
		return
	}
	c.sel = (c.sel + delta + len(c.matches)) % len(c.matches)
}

// dropWord removes the last word of the filter, ctrl+w style.
func dropWord(s string) string {
	s = strings.TrimRight(s, " ")
	if i := strings.LastIndex(s, " "); i >= 0 {
		return s[:i+1]
	}
	return ""
}

// dropRune removes the last rune of the filter. Doing this by rune rather than
// by byte is what stops a backspace over an accented letter leaving a stub
// behind that matches nothing.
func dropRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	return string(r[:len(r)-1])
}

func (c *chooser) Prompt() (interface{}, error) {
	c.render(false, nil)

	rr := c.NewRuneReader()
	rr.SetTermMode()
	defer rr.RestoreTermMode()
	cursor := c.NewCursor()
	cursor.Hide()
	defer cursor.Show()

	for {
		r, _, err := rr.ReadRune()
		if err != nil {
			return c.answer(), err
		}
		if done, err := c.onKey(r); done || err != nil {
			return c.answer(), err
		}
		c.render(false, nil)
	}
}

// onKey folds one keypress into the chooser. It reports whether the prompt is
// finished with, which is the only thing Prompt needs to know.
func (c *chooser) onKey(r rune) (bool, error) {
	switch r {
	case terminal.KeyInterrupt:
		return true, terminal.InterruptErr
	case terminal.KeyEnter, '\n', terminal.KeyEndTransmission:
		// With nothing matched there is nothing to accept, so hold the prompt
		// rather than answering with an empty string.
		return c.multi || c.current() >= 0, nil
	case terminal.KeyArrowUp:
		c.move(-1)
	case terminal.KeyArrowDown:
		c.move(1)
	case terminal.SpecialKeyHome:
		c.sel = 0
	case terminal.SpecialKeyEnd:
		c.sel = len(c.matches) - 1
	case terminal.KeyEscape, terminal.KeyDeleteLine:
		c.refilter("")
	case terminal.KeyDeleteWord:
		c.refilter(dropWord(c.filter))
	case terminal.KeyBackspace, terminal.KeyDelete, terminal.SpecialKeyDelete:
		c.refilter(dropRune(c.filter))
	case terminal.KeyArrowLeft, terminal.KeyArrowRight, terminal.IgnoreKey:
		// nothing to do, the filter has no cursor of its own
	case terminal.KeySpace:
		// In a multi select space is how you tick a line, so it cannot also be
		// part of the filter. Single select lists have no ticking, so there a
		// space is just another character to match on.
		if c.multi {
			c.toggle()
		} else {
			c.refilter(c.filter + " ")
		}
	case core.HelpInputRune:
		if c.filter == "" && c.help != "" {
			c.showHlp = !c.showHlp
		} else {
			c.refilter(c.filter + string(r))
		}
	default:
		if r >= terminal.KeySpace {
			c.refilter(c.filter + string(r))
		}
	}
	return false, nil
}

func (c *chooser) toggle() {
	if idx := c.current(); idx >= 0 {
		c.checked[c.options[idx]] = !c.checked[c.options[idx]]
	}
}

// answer is what survey writes into the caller's variable.
func (c *chooser) answer() interface{} {
	if c.multi {
		out := []string{}
		for _, l := range c.options {
			if c.checked[l] {
				out = append(out, l)
			}
		}
		return out
	}
	if idx := c.current(); idx >= 0 {
		return c.options[idx]
	}
	return ""
}

func (c *chooser) Cleanup(val interface{}) error {
	c.render(true, val)
	return nil
}

// filterLine is the "/eldr  (3 of 54)" that sits after the question.
func (c *chooser) filterLine() string {
	if c.filter == "" {
		return ""
	}
	return fmt.Sprintf("  /%s  (%d of %d)", c.filter, len(c.matches), len(c.options))
}

func (c *chooser) render(done bool, val interface{}) error {
	in := paneInput{
		message: c.message, filter: c.filterLine(), help: c.help,
		showHelp: c.showHlp, options: c.options, checked: c.checked, multi: c.multi,
		filtering: c.filter != "",
	}
	if done {
		in.showAnswer = true
		if s, ok := val.(string); ok {
			in.answer = s
		}
		return c.Render(chooserTemplate, in)
	}
	page, idx := pageOf(c.matches, c.sel, c.pageSize)
	for _, m := range page {
		in.entries = append(in.entries, c.options[m])
	}
	in.sel = idx
	return c.Render(chooserTemplate, in)
}

// pageOf returns the slice of matches to draw and where the cursor sits in it,
// keeping the cursor in the middle of the page once the list is long enough.
func pageOf(matches []int, sel, size int) ([]int, int) {
	if size <= 0 {
		size = 7
	}
	if len(matches) <= size {
		return matches, sel
	}
	start := sel - size/2
	if start < 0 {
		start = 0
	}
	if start+size > len(matches) {
		start = len(matches) - size
	}
	return matches[start : start+size], sel - start
}

// ----------------------------------------------------------------------------
// fuzzy matching
// ----------------------------------------------------------------------------

// fuzzyMatches returns the indexes of the options the filter matches, best
// first. An empty filter keeps the list in the order the server sent it, which
// matters because that order is meaningful - recommended options come first.
func fuzzyMatches(filter string, labels, names []string) []int {
	filter = strings.TrimSpace(filter)
	out := []int{}
	if filter == "" {
		for i := range labels {
			out = append(out, i)
		}
		return out
	}
	type hit struct {
		idx, score int
	}
	hits := []hit{}
	terms := strings.Fields(strings.ToLower(filter))
	for i := range labels {
		name := labels[i]
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		score, ok := scoreAll(terms, name, labels[i])
		if ok {
			hits = append(hits, hit{i, score})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	for _, h := range hits {
		out = append(out, h.idx)
	}
	return out
}

// scoreAll requires every term to match somewhere, so "fire bolt" and
// "bolt fire" both find the cantrip.
//
// A term matches the option's name loosely - "eldbl" is Eldritch Blast - but
// the summary that follows it only counts on a whole word. Fuzzy matching over
// a sentence matches almost anything, which would leave a filter that reads
// like it should have narrowed the list showing half of it.
func scoreAll(terms []string, name, label string) (int, bool) {
	rest := strings.ToLower(label)
	if i := strings.Index(rest, strings.ToLower(name)); i >= 0 {
		rest = rest[i+len(name):]
	}
	total := 0
	for _, t := range terms {
		if n, ok := fuzzyScore(t, name); ok {
			total += n * 2
			continue
		}
		if i := strings.Index(rest, t); i >= 0 {
			// worth having, but always below anything named for it
			total += 20 - i/8
			continue
		}
		return 0, false
	}
	return total, true
}

// fuzzyScore matches pattern against text as a subsequence and scores how good
// the match is: adjacent letters, letters that start a word and a match right
// at the front all count for more, and a big gap between letters counts for
// less. Both arguments are compared case insensitively.
func fuzzyScore(pattern, text string) (int, bool) {
	if pattern == "" {
		return 0, true
	}
	pat := []rune(strings.ToLower(pattern))
	txt := []rune(strings.ToLower(text))
	score, pi, last := 0, 0, -1
	for ti := 0; ti < len(txt) && pi < len(pat); ti++ {
		if txt[ti] != pat[pi] {
			continue
		}
		score += 10
		switch {
		case ti == 0:
			score += 20
		case last == ti-1:
			score += 12 // running on from the previous letter
		case isBoundary(txt[ti-1]):
			score += 10 // the start of a word
		}
		if last >= 0 && ti-last > 1 {
			gap := ti - last - 1
			if gap > 6 {
				gap = 6
			}
			score -= gap
		}
		last = ti
		pi++
	}
	if pi < len(pat) {
		return 0, false
	}
	// A short option that matched is more likely to be what was meant than a
	// long one that happens to contain the same letters.
	score -= len(txt) / 12
	return score, true
}

func isBoundary(r rune) bool {
	switch r {
	case ' ', '-', '_', '/', '(', ')', ',', '.', '\'', ':':
		return true
	}
	return false
}

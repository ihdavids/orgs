package github

// Reading and editing the mirror file as lines.
//
// The file is the person's as much as the plugin's: notes, clocks and child
// headings go under the items. So nothing here parses the document and writes
// it back (which re-indents every drawer and reflows every table); each change
// is a splice of the one line it is about, and a heading is found by its id
// property rather than by where it was last time.

import (
	"regexp"
	"strings"
	"time"
)

var (
	headRe     = regexp.MustCompile(`^(\*+)[ \t]+(.*)$`)
	tagsRe     = regexp.MustCompile(`[ \t]+(:[^\s]+:)[ \t]*$`)
	planRe     = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	closedRe   = regexp.MustCompile(`CLOSED:[ \t]*\[[^\]]*\][ \t]*`)
	drawerRe   = regexp.MustCompile(`^\s*:PROPERTIES:\s*$`)
	endRe      = regexp.MustCompile(`^\s*:END:\s*$`)
	propRe     = regexp.MustCompile(`^(\s*):([^:\s]+):([ \t]*)(.*)$`)
	todoLineRe = regexp.MustCompile(`(?i)^#\+(?:SEQ_|TYP_)?TODO:\s*(.*)$`)
	tagUnsafe  = regexp.MustCompile(`[^\p{L}\p{N}_@#%]+`)
	prioRe     = regexp.MustCompile(`^\[#[A-Za-z0-9]\][ \t]*`)
)

// keywords is which todo keywords a file has and which of them are done.
type keywords struct {
	all  map[string]bool
	done map[string]bool
}

// parseKeywords reads org's keyword spec: "TODO NEXT | DONE CANCELLED", one
// sequence a line; with no bar the last word is the done one. Cookies like
// NEXT(n!) are taken off.
func (k *keywords) parse(spec string) {
	for _, line := range strings.Split(spec, "\n") {
		words := strings.Fields(line)
		bar := -1
		for i, w := range words {
			if w == "|" {
				bar = i
			}
		}
		for i, w := range words {
			if w == "|" {
				continue
			}
			if p := strings.IndexByte(w, '('); p > 0 {
				w = w[:p]
			}
			k.all[w] = true
			if (bar >= 0 && i > bar) || (bar < 0 && i == len(words)-1) {
				k.done[w] = true
			}
		}
	}
}

func newKeywords() keywords {
	return keywords{all: map[string]bool{}, done: map[string]bool{}}
}

// heading is one headline and what is needed to edit it.
type heading struct {
	row, end int // headline row, and the last row of its subtree
	lvl      int
	keyword  string
	title    string
	tags     []string
	ps, pe   int // its property drawer's :PROPERTIES: and :END: rows, -1 without
	props    map[string]string
}

// splitHeadline is a headline's keyword, title and tags.
func splitHeadline(rest string, kw keywords) (keyword, title string, tags []string) {
	if loc := tagsRe.FindStringSubmatchIndex(rest); loc != nil {
		tags = strings.Split(strings.Trim(rest[loc[2]:loc[3]], ":"), ":")
		rest = rest[:loc[0]]
	}
	rest = strings.TrimSpace(rest)
	if f := strings.Fields(rest); len(f) > 0 && kw.all[f[0]] {
		keyword = f[0]
		rest = strings.TrimSpace(rest[len(f[0]):])
	}
	if p := prioRe.FindString(rest); p != "" {
		rest = strings.TrimSpace(rest[len(p):])
	}
	return keyword, rest, tags
}

// scan finds every heading in lines.
func scan(lines []string, kw keywords) []heading {
	var hs []heading
	for i, l := range lines {
		m := headRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		h := heading{row: i, lvl: len(m[1]), ps: -1, pe: -1, props: map[string]string{}}
		h.keyword, h.title, h.tags = splitHeadline(m[2], kw)
		h.end = subtreeEnd(lines, i, h.lvl)
		// The drawer is only the property drawer when it comes straight after
		// the headline and its planning line.
		j := i + 1
		for j < len(lines) && planRe.MatchString(lines[j]) {
			j++
		}
		if j < len(lines) && drawerRe.MatchString(lines[j]) {
			for k := j + 1; k <= h.end && k < len(lines); k++ {
				if endRe.MatchString(lines[k]) {
					h.ps, h.pe = j, k
					break
				}
				if pm := propRe.FindStringSubmatch(lines[k]); pm != nil {
					h.props[strings.ToUpper(pm[2])] = strings.TrimSpace(pm[4])
				}
			}
		}
		hs = append(hs, h)
	}
	return hs
}

// The last row of the subtree of the heading at row: the row before the next
// heading at its level or above, or the file's last row.
func subtreeEnd(lines []string, row, lvl int) int {
	for i := row + 1; i < len(lines); i++ {
		if m := headRe.FindStringSubmatch(lines[i]); m != nil && len(m[1]) <= lvl {
			return i - 1
		}
	}
	return len(lines) - 1
}

// findByID is the heading carrying this id in the property named key.
func findByID(lines []string, kw keywords, key, id string) (heading, bool) {
	for _, h := range scan(lines, kw) {
		if h.props[key] == id {
			return h, true
		}
	}
	return heading{}, false
}

func splice(lines []string, at int, add []string) []string {
	out := make([]string, 0, len(lines)+len(add))
	out = append(out, lines[:at]...)
	out = append(out, add...)
	return append(out, lines[at:]...)
}

// setHeadline is the headline with a new title and tags; stars, keyword and
// priority stay as written, and tags that were pushed out to org's tags
// column stay ending where they ended.
func setHeadline(line string, kw keywords, title string, tags []string) string {
	m := headRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	rest := m[2]
	oldEnd, gap := 0, 2
	if loc := tagsRe.FindStringSubmatchIndex(rest); loc != nil {
		// A run of space before the tags is org's tags column: keep them
		// ending where they ended. A space or two is just kept.
		gap = loc[2] - loc[0]
		if gap > 2 {
			oldEnd = len(strings.TrimRight(line, " \t"))
		}
		rest = rest[:loc[0]]
	}
	keyword, _, _ := splitHeadline(rest, kw)
	rest = strings.TrimSpace(rest)
	if keyword != "" {
		rest = strings.TrimSpace(rest[len(keyword):])
	}
	prio := strings.TrimSpace(prioRe.FindString(rest))
	out := m[1]
	for _, p := range []string{keyword, prio, title} {
		if p != "" {
			out += " " + p
		}
	}
	if len(tags) > 0 {
		t := ":" + strings.Join(tags, ":") + ":"
		pad := gap
		if oldEnd > 0 {
			pad = 1
			if n := oldEnd - len(out) - len(t); n > 1 {
				pad = n
			}
		}
		out += strings.Repeat(" ", pad) + t
	}
	return out
}

// setKeyword swaps a headline's keyword, and nothing else on the line.
func setKeyword(line string, kw keywords, to string) string {
	m := headRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	keyword, _, _ := splitHeadline(m[2], kw)
	rest := strings.TrimLeft(m[2], " \t")
	if keyword != "" {
		rest = strings.TrimLeft(rest[len(keyword):], " \t")
	}
	if to == "" {
		return m[1] + " " + rest
	}
	if rest == "" {
		return m[1] + " " + to
	}
	return m[1] + " " + to + " " + rest
}

// setProps writes properties into a heading's drawer, making it when there is
// none. A value already right is not touched, a changed one keeps its line's
// spacing, and a new one is aligned with the keys around it. Answers whether
// anything changed.
func setProps(lines []string, h heading, keys []string, vals map[string]string) ([]string, bool) {
	changed := false
	if h.ps < 0 {
		at := h.row + 1
		for at < len(lines) && at <= h.end && planRe.MatchString(lines[at]) {
			at++
		}
		block := []string{":PROPERTIES:"}
		width := 0
		for _, k := range keys {
			if vals[k] != "" && len(k) > width {
				width = len(k)
			}
		}
		for _, k := range keys {
			if vals[k] != "" {
				block = append(block, propLine("", k, vals[k], width))
			}
		}
		block = append(block, ":END:")
		return splice(lines, at, block), true
	}
	ind := regexp.MustCompile(`^\s*`).FindString(lines[h.ps])
	pe := h.pe
	for _, k := range keys {
		v := vals[k]
		found := false
		for i := h.ps + 1; i < pe; i++ {
			pm := propRe.FindStringSubmatch(lines[i])
			if pm == nil || !strings.EqualFold(pm[2], k) {
				continue
			}
			found = true
			if v == "" {
				// Nothing to say any more (the last label came off).
				lines = append(lines[:i:i], lines[i+1:]...)
				pe--
				changed = true
			} else if strings.TrimSpace(pm[4]) != v {
				gap := pm[3]
				if gap == "" {
					gap = " "
				}
				lines[i] = pm[1] + ":" + pm[2] + ":" + gap + v
				changed = true
			}
			break
		}
		if !found && v != "" {
			width := len(k)
			for i := h.ps + 1; i < pe; i++ {
				if pm := propRe.FindStringSubmatch(lines[i]); pm != nil && len(pm[2])+len(pm[3]) > width+1 {
					width = len(pm[2]) + len(pm[3]) - 1
				}
			}
			lines = splice(lines, pe, []string{propLine(ind, k, v, width)})
			pe++
			changed = true
		}
	}
	return lines, changed
}

func propLine(ind, key, val string, width int) string {
	pad := width - len(key) + 1
	if pad < 1 {
		pad = 1
	}
	return ind + ":" + key + ":" + strings.Repeat(" ", pad) + val
}

// closeLocally is the state change done here, for a file the server has not
// loaded (or no server, in the tests): the keyword, and a CLOSED stamp put on
// or taken off. The server's own change, when it can do it, also logs it.
func closeLocally(lines []string, h heading, kw keywords, to string, now time.Time) []string {
	lines[h.row] = setKeyword(lines[h.row], kw, to)
	plan := -1
	if h.row+1 < len(lines) && planRe.MatchString(lines[h.row+1]) {
		plan = h.row + 1
	}
	if kw.done[to] {
		stamp := "CLOSED: [" + now.Format("2006-01-02 Mon 15:04") + "]"
		switch {
		case plan >= 0 && closedRe.MatchString(lines[plan]):
			lines[plan] = closedRe.ReplaceAllString(lines[plan], stamp+" ")
			lines[plan] = strings.TrimRight(lines[plan], " ")
		case plan >= 0:
			ind := regexp.MustCompile(`^\s*`).FindString(lines[plan])
			lines[plan] = ind + stamp + " " + strings.TrimLeft(lines[plan], " \t")
		default:
			lines = splice(lines, h.row+1, []string{stamp})
		}
		return lines
	}
	if plan >= 0 && closedRe.MatchString(lines[plan]) {
		rest := strings.TrimSpace(closedRe.ReplaceAllString(lines[plan], ""))
		if rest == "" {
			lines = append(lines[:plan:plan], lines[plan+1:]...)
		} else {
			ind := regexp.MustCompile(`^\s*`).FindString(lines[plan])
			lines[plan] = ind + rest
		}
	}
	return lines
}

// labelTag is a label as an org tag: spaces and punctuation become
// underscores, since a tag cannot hold them.
func labelTag(name string) string {
	return strings.Trim(tagUnsafe.ReplaceAllString(strings.TrimSpace(name), "_"), "_")
}

// mergeTags is the heading's tags with the labels brought up to date: a tag
// that was a label last time and is not now comes off, a new label goes on,
// and every other tag - the ones somebody put there by hand - stays where it
// was. Unchanged labels give back the tags exactly as they were.
func mergeTags(old, wasLabels, labels []string) []string {
	was := map[string]bool{}
	for _, l := range wasLabels {
		was[l] = true
	}
	now := map[string]bool{}
	for _, l := range labels {
		now[l] = true
	}
	var out []string
	have := map[string]bool{}
	for _, t := range old {
		if t == "" || (was[t] && !now[t]) || have[t] {
			continue
		}
		out = append(out, t)
		have[t] = true
	}
	for _, l := range labels {
		if !have[l] {
			out = append(out, l)
			have[l] = true
		}
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

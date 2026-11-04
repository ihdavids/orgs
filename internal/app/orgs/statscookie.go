package orgs

// Statistics cookies: the [2/5] and [40%] on a heading or a list item, kept
// true when a box is ticked or a child changes keyword, as org's
// org-update-statistics-cookies does. Every server write that can change a
// count calls refreshCookiesAround (POST /checklist, /checklist/edit and
// applyStatusChange, which every keyword change goes through), so a box
// ticked on a kanban card or a task marked done from the terminal leaves the
// parent's cookie right, not only an edit in worg's editor.
//
// The counting is refreshSection in worg/src/orgcmds.ts (see Traps: logic
// said twice): a parent item's box from its children's ([X] all, [-] some,
// [ ] none), an item's cookie from its direct children, and a heading's from
// its section's top-level checkboxes or, when it has none, its child
// headings' keywords. A heading's COOKIE_DATA property chooses as org does:
// "todo" counts child headings even when there are boxes, "checkbox" counts
// boxes only, "recursive" counts every descendant heading (or every box in
// the section, at any depth) rather than the direct ones.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

var (
	scItemRe   = regexp.MustCompile(`^(\s*)([-+]|\s\*|\d+[.)])(\s+)(\[[ xX-]\]\s+)?(.*)$`)
	scCookieRe = regexp.MustCompile(`\[(\d*%|\d*/\d*)\]`)
	scBoxRe    = regexp.MustCompile(`^(\s*(?:[-+]|\s\*|\d+[.)])\s+)\[[ xX-]\]`)
	scHeadRe   = regexp.MustCompile(`^(\*+)\s+(.*)$`)
	scDataRe   = regexp.MustCompile(`(?i)^\s*:COOKIE_DATA:\s*(.*)$`)
)

// keywordSets are a file's open and finished keywords, by name. A file may
// have several #+TODO lines, each its own sequence.
func keywordSets(f *common.OrgFile) (done, active map[string]bool) {
	done, active = map[string]bool{}, map[string]bool{}
	for _, spec := range strings.Split(todoSpecFor(f), "\n") {
		a, d := parseTodoKeywords(spec)
		for _, k := range a {
			active[k.Name] = true
		}
		for _, k := range d {
			done[k.Name] = true
		}
	}
	return done, active
}

type scItem struct {
	line, indent, end int
	box               string
}

type scLines struct {
	lines []string
	mask  []bool
}

func newScLines(lines []string) *scLines {
	mask := make([]bool, len(lines))
	inside := false
	for i, l := range lines {
		switch {
		case !inside && txBlockBeginRe.MatchString(l):
			inside = true
		case inside && txBlockEndRe.MatchString(l):
			inside = false
		default:
			mask[i] = inside
		}
	}
	return &scLines{lines, mask}
}

func (s *scLines) level(i int) int {
	if i < 0 || i >= len(s.lines) || s.mask[i] {
		return 0
	}
	return txLevel(s.lines[i])
}

func (s *scLines) sectionOf(row int) int {
	if row >= len(s.lines) {
		row = len(s.lines) - 1
	}
	for i := row; i >= 0; i-- {
		if s.level(i) > 0 {
			return i
		}
	}
	return -1
}

func (s *scLines) sectionEnd(h int) int {
	for i := h + 1; i < len(s.lines); i++ {
		if s.level(i) > 0 {
			return i
		}
	}
	return len(s.lines)
}

func (s *scLines) subtreeEnd(h int) int {
	lvl := s.level(h)
	for i := h + 1; i < len(s.lines); i++ {
		if n := s.level(i); n > 0 && n <= lvl {
			return i
		}
	}
	return len(s.lines)
}

func (s *scLines) parent(h int) int {
	lvl := s.level(h)
	for i := h - 1; i >= 0; i-- {
		if n := s.level(i); n > 0 && n < lvl {
			return i
		}
	}
	return -1
}

func scIndent(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }

func (s *scLines) item(i int) *scItem {
	if s.mask[i] {
		return nil
	}
	m := scItemRe.FindStringSubmatch(s.lines[i])
	if m == nil {
		return nil
	}
	indent := len(m[1])
	if strings.HasPrefix(m[2], " ") {
		indent++
	}
	end := i + 1
	for end < len(s.lines) && strings.TrimSpace(s.lines[end]) != "" && scIndent(s.lines[end]) > indent && s.level(end) == 0 {
		end++
	}
	return &scItem{line: i, indent: indent, end: end, box: strings.TrimSpace(m[4])}
}

// children are the items directly in [from, to): those at the shallowest
// indent there.
func (s *scLines) children(from, to int) []*scItem {
	depth := -1
	for i := from; i < to; i++ {
		if it := s.item(i); it != nil && (depth < 0 || it.indent < depth) {
			depth = it.indent
		}
	}
	out := []*scItem{}
	for i := from; i < to; i++ {
		if it := s.item(i); it != nil && it.indent == depth {
			out = append(out, it)
			i = it.end - 1
		}
	}
	return out
}

func scBoxOf(l string) string {
	m := scItemRe.FindStringSubmatch(l)
	if m == nil || m[4] == "" {
		return ""
	}
	return string(strings.TrimSpace(m[4])[1])
}

func scWriteCookie(l string, done, total int) string {
	return scCookieRe.ReplaceAllStringFunc(l, func(c string) string {
		if strings.HasSuffix(c, "%]") {
			pct := 0
			if total > 0 {
				pct = 100 * done / total
			}
			return fmt.Sprintf("[%d%%]", pct)
		}
		return fmt.Sprintf("[%d/%d]", done, total)
	})
}

// cookieData is the heading's COOKIE_DATA words, read off its own drawer.
func (s *scLines) cookieData(h int) map[string]bool {
	out := map[string]bool{}
	for i := h + 1; i < s.sectionEnd(h); i++ {
		if m := scDataRe.FindStringSubmatch(s.lines[i]); m != nil {
			for _, w := range strings.Fields(strings.ToLower(m[1])) {
				out[w] = true
			}
			break
		}
	}
	return out
}

// refreshSection brings the derived state of one section up to date, in
// place: boxes and cookies of its items, then the heading's cookie. h is -1
// for the text above the first heading.
func (s *scLines) refreshSection(h int, done, active map[string]bool) {
	from, to := 0, 0
	if h < 0 {
		for to < len(s.lines) && s.level(to) == 0 {
			to++
		}
	} else {
		from, to = h+1, s.sectionEnd(h)
	}
	items := []*scItem{}
	for i := from; i < to; i++ {
		if it := s.item(i); it != nil {
			items = append(items, it)
		}
	}
	// Deepest first, so a parent is worked out from children already settled.
	sortDeepestFirst(items)
	for _, it := range items {
		kids := []*scItem{}
		for _, k := range s.children(it.line+1, it.end) {
			if k.box != "" {
				kids = append(kids, k)
			}
		}
		if len(kids) == 0 {
			continue
		}
		n := 0
		for _, k := range kids {
			if strings.EqualFold(scBoxOf(s.lines[k.line]), "x") {
				n++
			}
		}
		if scBoxOf(s.lines[it.line]) != "" {
			mark := "-"
			if n == len(kids) {
				mark = "X"
			} else if n == 0 {
				mark = " "
			}
			s.lines[it.line] = scBoxRe.ReplaceAllString(s.lines[it.line], "${1}["+mark+"]")
		}
		if scCookieRe.MatchString(s.lines[it.line]) {
			s.lines[it.line] = scWriteCookie(s.lines[it.line], n, len(kids))
		}
	}
	if h < 0 || !scCookieRe.MatchString(s.lines[h]) {
		return
	}
	data := s.cookieData(h)
	if !data["todo"] {
		boxes := []*scItem{}
		if data["recursive"] {
			for _, it := range items {
				if it.box != "" {
					boxes = append(boxes, it)
				}
			}
		} else {
			for _, k := range s.children(from, to) {
				if k.box != "" {
					boxes = append(boxes, k)
				}
			}
		}
		if len(boxes) > 0 || data["checkbox"] {
			n := 0
			for _, k := range boxes {
				if strings.EqualFold(scBoxOf(s.lines[k.line]), "x") {
					n++
				}
			}
			s.lines[h] = scWriteCookie(s.lines[h], n, len(boxes))
			return
		}
	}
	// The child headings that carry a keyword: the direct ones, or with
	// recursive every one below.
	lvl := s.level(h)
	end := s.subtreeEnd(h)
	childLvl := 0
	for i := h + 1; i < end; i++ {
		if n := s.level(i); n > 0 && (childLvl == 0 || n < childLvl) {
			childLvl = n
		}
	}
	n, total := 0, 0
	for i := h + 1; i < end; i++ {
		l := s.level(i)
		if l == 0 || l <= lvl || (!data["recursive"] && l != childLvl) {
			continue
		}
		m := scHeadRe.FindStringSubmatch(s.lines[i])
		kw := ""
		if f := strings.Fields(m[2]); len(f) > 0 {
			kw = f[0]
		}
		if done[kw] {
			n++
			total++
		} else if active[kw] {
			total++
		}
	}
	s.lines[h] = scWriteCookie(s.lines[h], n, total)
}

func sortDeepestFirst(items []*scItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].indent > items[j-1].indent; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// refreshCookiesAround is lines with the cookies of the section row is in and
// of every heading above it brought up to date. A grandparent's cookie only
// moves with COOKIE_DATA recursive, and the others come out the same.
func refreshCookiesAround(lines []string, row int, done, active map[string]bool) []string {
	s := newScLines(append([]string{}, lines...))
	h := s.sectionOf(row)
	s.refreshSection(h, done, active)
	for p := h; p >= 0; {
		p = s.parent(p)
		if p >= 0 {
			s.refreshSection(p, done, active)
		}
	}
	return s.lines
}

// refreshCookiesIn is refreshCookiesAround with the keywords of the file the
// lines are.
func refreshCookiesIn(filename string, lines []string, row int) []string {
	done, active := keywordSets(GetDb().FindByFile(filename))
	return refreshCookiesAround(lines, row, done, active)
}

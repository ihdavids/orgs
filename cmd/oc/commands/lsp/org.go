package lsp

// What is where in an org buffer: headings (from go-org's outline, so a line
// in a source block that starts with a star is not one), blocks, drawers,
// links and the names things can be linked by. Rows are 0 based, columns
// bytes; the protocol's columns are worked out at the edge (text.go).

import (
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
)

type heading struct {
	line, level int
	// The last line of its subtree.
	end      int
	keyword  string
	done     bool
	priority string
	comment  bool
	// The title as written, without stars, keyword, priority or tags: what a
	// [[*Title]] link names it by.
	title    string
	titleCol int
	tags     []string
	id       string
	customId string
	sched    string
	deadline string
}

var (
	starsRe    = regexp.MustCompile(`^(\*+)\s`)
	tagsTailRe = regexp.MustCompile(`\s+(:[\p{L}\p{N}_@#%:]+:)\s*$`)
	priorityRe = regexp.MustCompile(`^\[#[A-Za-z0-9]\]\s*`)
	cookieRe   = regexp.MustCompile(`\s*\[\d*(?:/\d*|%)\]`)
)

// headings of a document, in file order.
func (s *server) headings(d *document) []heading {
	doc := d.parsed(s.orgConfig)
	todo, done := doc.TodoKeywords()
	isDone := map[string]bool{}
	for _, k := range done {
		isDone[k] = true
	}
	_ = todo
	out := []heading{}
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, sec := range secs {
			if h := sec.Headline; h != nil && h.Pos.Row >= 0 && h.Pos.Row < len(d.lines) {
				hd := heading{line: h.Pos.Row, level: h.Lvl, keyword: h.Status, done: isDone[h.Status],
					priority: h.Priority, comment: h.IsComment, tags: h.Tags}
				if h.Properties != nil {
					hd.id, _ = h.Properties.Get("ID")
					hd.customId, _ = h.Properties.Get("CUSTOM_ID")
				}
				if h.Scheduled != nil && h.Scheduled.Date != nil {
					hd.sched = h.Scheduled.Date.ToString()
				}
				if h.Deadline != nil && h.Deadline.Date != nil {
					hd.deadline = h.Deadline.Date.ToString()
				}
				hd.title, hd.titleCol = rawTitle(d.line(h.Pos.Row), h.Status, h.IsComment)
				out = append(out, hd)
			}
			walk(sec.Children)
		}
	}
	if doc.Outline.Section != nil {
		walk(doc.Outline.Children)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].line < out[b].line })
	for i := range out {
		out[i].end = len(d.lines) - 1
		for j := i + 1; j < len(out); j++ {
			if out[j].level <= out[i].level {
				out[i].end = out[j].line - 1
				break
			}
		}
	}
	// A trailing empty line after the final newline belongs to nothing.
	for i := range out {
		for out[i].end > out[i].line && strings.TrimSpace(d.line(out[i].end)) == "" && out[i].end == len(d.lines)-1 {
			out[i].end--
		}
	}
	return out
}

// rawTitle is a heading line's title and the byte column it starts at.
func rawTitle(line, keyword string, comment bool) (string, int) {
	m := starsRe.FindStringIndex(line)
	if m == nil {
		return "", 0
	}
	col := m[1]
	for col < len(line) && (line[col] == ' ' || line[col] == '\t') {
		col++
	}
	rest := line[col:]
	if keyword != "" && strings.HasPrefix(rest, keyword) {
		n := len(keyword)
		rest = rest[n:]
		col += n
		trim := len(rest) - len(strings.TrimLeft(rest, " \t"))
		rest, col = rest[trim:], col+trim
	}
	if p := priorityRe.FindString(rest); p != "" {
		rest, col = rest[len(p):], col+len(p)
	}
	if comment && strings.HasPrefix(rest, "COMMENT") {
		rest = strings.TrimLeft(rest[len("COMMENT"):], " \t")
		col = len(line) - len(rest)
	}
	if t := tagsTailRe.FindStringIndex(rest); t != nil {
		rest = rest[:t[0]]
	}
	return strings.TrimRight(rest, " \t"), col
}

// sameTitle is whether a link's *Title names a heading: org compares the
// title without its statistics cookie, and so does this, ignoring case and
// runs of space as org's search does.
func sameTitle(a, b string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(cookieRe.ReplaceAllString(s, "")), " "))
	}
	return norm(a) == norm(b)
}

// headingAt is the heading whose own lines (not a child's) hold row, or -1.
func headingAt(hs []heading, row int) int {
	at := -1
	for i, h := range hs {
		if h.line > row {
			break
		}
		if row <= h.end {
			at = i
		}
	}
	return at
}

// ---------------------------------------------------------------------------
// Blocks and drawers
// ---------------------------------------------------------------------------

type block struct {
	kind     string // src, example, quote ... lower case
	lang     string // for src
	name     string // #+NAME: above it
	begin    int
	end      int // -1 when it has no #+END_
	nameLine int // -1 when unnamed
}

var (
	beginRe    = regexp.MustCompile(`(?i)^\s*#\+begin_(\S+)(?:\s+(\S+))?`)
	nameRe     = regexp.MustCompile(`(?i)^\s*#\+name:\s*(\S.*?)\s*$`)
	callRe     = regexp.MustCompile(`(?i)^\s*#\+call:`)
	drawerRe   = regexp.MustCompile(`^\s*:([\w-]+):\s*$`)
	drawerEndR = regexp.MustCompile(`(?i)^\s*:end:\s*$`)
	tableRowRe = regexp.MustCompile(`^\s*\|`)
	inlineRe   = regexp.MustCompile(`\b(?:src_[\w-]+(?:\[[^\]]*\])?\{|call_[\w-]+(?:\[[^\]]*\])?\()`)
)

func blocks(lines []string) []block {
	out := []block{}
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSuffix(lines[i], "\r")
		m := beginRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		b := block{kind: strings.ToLower(m[1]), begin: i, end: -1, nameLine: -1}
		if b.kind == "src" {
			b.lang = m[2]
		}
		for k := i - 1; k >= 0; k-- {
			pl := strings.TrimSuffix(lines[k], "\r")
			if nm := nameRe.FindStringSubmatch(pl); nm != nil {
				b.name, b.nameLine = nm[1], k
				break
			}
			if !strings.HasPrefix(strings.TrimSpace(pl), "#+") {
				break
			}
		}
		endRe := regexp.MustCompile(`(?i)^\s*#\+end_` + regexp.QuoteMeta(m[1]) + `\s*$`)
		for j := i + 1; j < len(lines); j++ {
			if endRe.MatchString(strings.TrimSuffix(lines[j], "\r")) {
				b.end = j
				break
			}
			if starsRe.MatchString(lines[j]) && b.kind != "src" && b.kind != "example" {
				break
			}
		}
		out = append(out, b)
		if b.end > 0 {
			i = b.end
		}
	}
	return out
}

// inBlock is the block holding row, or nil.
func inBlock(bs []block, row int) *block {
	for i := range bs {
		if bs[i].begin <= row && (row <= bs[i].end || (bs[i].end < 0 && row == bs[i].begin)) {
			return &bs[i]
		}
	}
	return nil
}

type drawer struct {
	name       string
	begin, end int // end -1 when it is never closed
}

func drawers(lines []string, bs []block) []drawer {
	out := []drawer{}
	for i := 0; i < len(lines); i++ {
		if inBlock(bs, i) != nil {
			continue
		}
		l := strings.TrimSuffix(lines[i], "\r")
		m := drawerRe.FindStringSubmatch(l)
		if m == nil || strings.EqualFold(m[1], "end") {
			continue
		}
		d := drawer{name: m[1], begin: i, end: -1}
		for j := i + 1; j < len(lines); j++ {
			lj := strings.TrimSuffix(lines[j], "\r")
			if drawerEndR.MatchString(lj) {
				d.end = j
				break
			}
			if starsRe.MatchString(lj) {
				break
			}
		}
		out = append(out, d)
		if d.end > 0 {
			i = d.end
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

type link struct {
	line       int
	start, end int // bytes in the line, the whole of [[...]]
	target     string
	desc       string
}

var (
	bracketLinkRe = regexp.MustCompile(`\[\[((?:[^\]\\]|\\.)+)\](?:\[((?:[^\]]|\][^\]])*)\])?\]`)
	plainLinkRe   = regexp.MustCompile(`\b(?:https?|ftp|mailto|file):[^\s\]\[<>"]+`)
	targetDefRe   = regexp.MustCompile(`<<([^<>\s][^<>]*?)>>`)
)

func linksIn(lines []string, bs []block) []link {
	out := []link{}
	for i, raw := range lines {
		if b := inBlock(bs, i); b != nil && b.kind != "quote" && b.kind != "verse" && b.kind != "center" {
			continue
		}
		l := strings.TrimSuffix(raw, "\r")
		taken := [][]int{}
		for _, m := range bracketLinkRe.FindAllStringSubmatchIndex(l, -1) {
			lk := link{line: i, start: m[0], end: m[1], target: strings.ReplaceAll(l[m[2]:m[3]], `\]`, "]")}
			if m[4] >= 0 {
				lk.desc = l[m[4]:m[5]]
			}
			out = append(out, lk)
			taken = append(taken, m[:2])
		}
		for _, m := range plainLinkRe.FindAllStringIndex(l, -1) {
			inside := false
			for _, t := range taken {
				if m[0] >= t[0] && m[0] < t[1] {
					inside = true
				}
			}
			if !inside {
				out = append(out, link{line: i, start: m[0], end: m[1], target: strings.TrimRight(l[m[0]:m[1]], ".,;:!?)")})
			}
		}
	}
	return out
}

// splitLink is a link target's protocol and the rest: "id", "X" for id:X,
// "" for *Heading or #custom or a bare name.
func splitLink(t string) (string, string) {
	if strings.HasPrefix(t, "./") || strings.HasPrefix(t, "../") || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~/") {
		return "file", t
	}
	if i := strings.Index(t, ":"); i > 0 && !strings.ContainsAny(t[:i], " \t*#") {
		return strings.ToLower(t[:i]), t[i+1:]
	}
	return "", t
}

// ---------------------------------------------------------------------------
// Outline and folding
// ---------------------------------------------------------------------------

func (s *server) symbols(d *document) []DocumentSymbol {
	hs := s.headings(d)
	bs := blocks(d.lines)
	e := s.enc
	rng := func(from, to int) Range {
		return Range{Start: Position{Line: from}, End: e.pos(d.lines, to, len(d.line(to)))}
	}
	type node struct {
		sym   DocumentSymbol
		level int
		kids  []*node
	}
	root := &node{level: 0}
	stack := []*node{root}
	for _, h := range hs {
		name := h.title
		if name == "" {
			name = "(untitled)"
		}
		detail := []string{}
		if h.keyword != "" {
			detail = append(detail, h.keyword)
		}
		if len(h.tags) > 0 {
			detail = append(detail, ":"+strings.Join(h.tags, ":")+":")
		}
		if h.deadline != "" {
			detail = append(detail, "DEADLINE "+h.deadline)
		} else if h.sched != "" {
			detail = append(detail, "SCHEDULED "+h.sched)
		}
		kind := symNamespace
		if h.keyword != "" {
			kind = symEvent
		}
		n := &node{level: h.level, sym: DocumentSymbol{Name: name, Detail: strings.Join(detail, " "), Kind: kind,
			Range: rng(h.line, h.end),
			SelectionRange: Range{Start: e.pos(d.lines, h.line, h.titleCol),
				End: e.pos(d.lines, h.line, h.titleCol+len(h.title))}}}
		for len(stack) > 1 && stack[len(stack)-1].level >= h.level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		parent.kids = append(parent.kids, n)
		stack = append(stack, n)
	}
	// Named blocks and tables go under the heading they are in.
	named := []struct {
		line int
		sym  DocumentSymbol
	}{}
	for _, b := range bs {
		if b.name == "" {
			continue
		}
		kind, detail := symVariable, b.kind
		if b.kind == "src" {
			kind, detail = symFunction, "src "+b.lang
		}
		end := b.end
		if end < 0 {
			end = b.begin
		}
		named = append(named, struct {
			line int
			sym  DocumentSymbol
		}{b.nameLine, DocumentSymbol{Name: b.name, Detail: detail, Kind: kind, Range: rng(b.nameLine, end),
			SelectionRange: rng(b.nameLine, b.nameLine)}})
	}
	for i := 0; i < len(d.lines); i++ {
		m := nameRe.FindStringSubmatch(d.line(i))
		if m == nil || i+1 >= len(d.lines) || !tableRowRe.MatchString(d.line(i+1)) {
			continue
		}
		end := i + 1
		for end+1 < len(d.lines) && tableRowRe.MatchString(d.line(end+1)) {
			end++
		}
		named = append(named, struct {
			line int
			sym  DocumentSymbol
		}{i, DocumentSymbol{Name: m[1], Detail: "table", Kind: symStruct, Range: rng(i, end), SelectionRange: rng(i, i)}})
	}
	var place func(n *node, line int, sym DocumentSymbol) bool
	place = func(n *node, line int, sym DocumentSymbol) bool {
		for _, k := range n.kids {
			if line >= k.sym.Range.Start.Line && line <= k.sym.Range.End.Line {
				if !place(k, line, sym) {
					k.sym.Children = append(k.sym.Children, sym)
				}
				return true
			}
		}
		return false
	}
	top := []DocumentSymbol{}
	for _, nm := range named {
		if !place(root, nm.line, nm.sym) {
			top = append(top, nm.sym)
		}
	}
	var build func(n *node) DocumentSymbol
	build = func(n *node) DocumentSymbol {
		sym := n.sym
		kids := []DocumentSymbol{}
		for _, k := range n.kids {
			kids = append(kids, build(k))
		}
		// Named things first were placed already; headings after them read
		// in file order either way, so sort by line.
		kids = append(kids, sym.Children...)
		sort.SliceStable(kids, func(a, b int) bool { return kids[a].Range.Start.Line < kids[b].Range.Start.Line })
		sym.Children = kids
		return sym
	}
	for _, k := range root.kids {
		top = append(top, build(k))
	}
	sort.SliceStable(top, func(a, b int) bool { return top[a].Range.Start.Line < top[b].Range.Start.Line })
	return top
}

func (s *server) folding(d *document) []FoldingRange {
	out := []FoldingRange{}
	for _, h := range s.headings(d) {
		if h.end > h.line {
			out = append(out, FoldingRange{StartLine: h.line, EndLine: h.end})
		}
	}
	bs := blocks(d.lines)
	for _, b := range bs {
		if b.end > b.begin {
			out = append(out, FoldingRange{StartLine: b.begin, EndLine: b.end})
		}
	}
	for _, dr := range drawers(d.lines, bs) {
		if dr.end > dr.begin {
			out = append(out, FoldingRange{StartLine: dr.begin, EndLine: dr.end})
		}
	}
	return out
}

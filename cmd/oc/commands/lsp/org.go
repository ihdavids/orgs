package lsp

// What is where in an org buffer: headings (from go-org's outline, so a line
// in a source block that starts with a star is not one), blocks, drawers,
// links and the names things can be linked by. Rows are 0 based, columns
// bytes; the protocol's columns are worked out at the edge (text.go).

import (
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/internal/common/orgcheck"
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

var starsRe = orgcheck.StarsRe

// headings of a document, in file order (orgcheck.Headings, which the
// server's /lsp/diagnose reads too, in this package's own names).
func (s *server) headings(d *document) []heading {
	hs := orgcheck.Headings(d.parsed(s.orgConfig), d.lines)
	out := make([]heading, len(hs))
	for i, h := range hs {
		out[i] = heading{line: h.Line, level: h.Level, end: h.End, keyword: h.Keyword, done: h.Done,
			priority: h.Priority, comment: h.Comment, title: h.Title, titleCol: h.TitleCol, tags: h.Tags,
			id: h.Id, customId: h.CustomId, sched: h.Sched, deadline: h.Deadline}
	}
	return out
}

// checkHeadings is the other way round, for orgcheck.Search.
func checkHeadings(hs []heading) []orgcheck.Heading {
	out := make([]orgcheck.Heading, len(hs))
	for i, h := range hs {
		out[i] = orgcheck.Heading{Line: h.line, Level: h.level, End: h.end, Title: h.title, TitleCol: h.titleCol, CustomId: h.customId}
	}
	return out
}

var sameTitle = orgcheck.SameTitle

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

// Blocks, drawers and links are orgcheck's, shared with the server.
type (
	block  = orgcheck.Block
	drawer = orgcheck.Drawer
	link   = orgcheck.Link
)

var (
	nameRe      = orgcheck.NameRe
	targetDefRe = orgcheck.TargetDefRe
	callRe      = regexp.MustCompile(`(?i)^\s*#\+call:`)
	tableRowRe  = regexp.MustCompile(`^\s*\|`)
	inlineRe    = regexp.MustCompile(`\b(?:src_[\w-]+(?:\[[^\]]*\])?\{|call_[\w-]+(?:\[[^\]]*\])?\()`)

	blocks    = orgcheck.Blocks
	inBlock   = orgcheck.InBlock
	drawers   = orgcheck.Drawers
	linksIn   = orgcheck.Links
	splitLink = orgcheck.SplitLink
)

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
		if b.Name == "" {
			continue
		}
		kind, detail := symVariable, b.Kind
		if b.Kind == "src" {
			kind, detail = symFunction, "src "+b.Lang
		}
		end := b.End
		if end < 0 {
			end = b.Begin
		}
		named = append(named, struct {
			line int
			sym  DocumentSymbol
		}{b.NameLine, DocumentSymbol{Name: b.Name, Detail: detail, Kind: kind, Range: rng(b.NameLine, end),
			SelectionRange: rng(b.NameLine, b.NameLine)}})
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
		if b.End > b.Begin {
			out = append(out, FoldingRange{StartLine: b.Begin, EndLine: b.End})
		}
	}
	for _, dr := range drawers(d.lines, bs) {
		if dr.End > dr.Begin {
			out = append(out, FoldingRange{StartLine: dr.Begin, EndLine: dr.End})
		}
	}
	return out
}

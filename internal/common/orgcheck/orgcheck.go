// Package orgcheck reads the structure of an org buffer line by line - its
// blocks, drawers, links and headings - and says what is wrong with it as
// org: a block or drawer never closed, a link to nothing, an edna property
// that will not run, a callout of no known type.
//
// It is the half of `orgs lsp`'s diagnostics that needs nothing but the text,
// moved here so the server can answer the same questions for worg's editor
// (POST /lsp/diagnose) without a second copy of the rules. What the text
// cannot say - whether a file is there, which ids exist, whether a feature is
// on - is asked of an Env, which the language server answers over REST and
// the server from its own database.
//
// Rows are 0 based and columns are bytes throughout; a client converts at the
// edge to whatever it counts in.
package orgcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

var (
	StarsRe     = regexp.MustCompile(`^(\*+)\s`)
	tagsTailRe  = regexp.MustCompile(`\s+(:[\p{L}\p{N}_@#%:]+:)\s*$`)
	priorityRe  = regexp.MustCompile(`^\[#[A-Za-z0-9]\]\s*`)
	cookieRe    = regexp.MustCompile(`\s*\[\d*(?:/\d*|%)\]`)
	beginRe     = regexp.MustCompile(`(?i)^\s*#\+begin_(\S+)(?:\s+(\S+))?`)
	NameRe      = regexp.MustCompile(`(?i)^\s*#\+name:\s*(\S.*?)\s*$`)
	drawerRe    = regexp.MustCompile(`^\s*:([\w-]+):\s*$`)
	drawerEndRe = regexp.MustCompile(`(?i)^\s*:end:\s*$`)
	ednaPropRe  = regexp.MustCompile(`(?i)^\s*:(TRIGGER|BLOCKER):[ \t]+(.*?)\s*$`)
	lineSearch  = regexp.MustCompile(`^\d+$`)

	bracketLinkRe = regexp.MustCompile(`\[\[((?:[^\]\\]|\\.)+)\](?:\[((?:[^\]]|\][^\]])*)\])?\]`)
	plainLinkRe   = regexp.MustCompile(`\b(?:https?|ftp|mailto|file):[^\s\]\[<>"]+`)
	TargetDefRe   = regexp.MustCompile(`<<([^<>\s][^<>]*?)>>`)
)

// ---------------------------------------------------------------------------
// Headings
// ---------------------------------------------------------------------------

type Heading struct {
	Line, Level int
	// The last line of its subtree.
	End      int
	Keyword  string
	Done     bool
	Priority string
	Comment  bool
	// The title as written, without stars, keyword, priority or tags: what a
	// [[*Title]] link names it by.
	Title    string
	TitleCol int
	Tags     []string
	Id       string
	CustomId string
	Sched    string
	Deadline string
}

// Headings of a parsed buffer, in file order. They come from go-org's outline
// rather than from lines starting with a star, so a star at the start of a
// line in a source block is not one.
func Headings(doc *org.Document, lines []string) []Heading {
	_, done := doc.TodoKeywords()
	isDone := map[string]bool{}
	for _, k := range done {
		isDone[k] = true
	}
	line := func(i int) string { return strings.TrimSuffix(lines[i], "\r") }
	out := []Heading{}
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, sec := range secs {
			if h := sec.Headline; h != nil && h.Pos.Row >= 0 && h.Pos.Row < len(lines) {
				hd := Heading{Line: h.Pos.Row, Level: h.Lvl, Keyword: h.Status, Done: isDone[h.Status],
					Priority: h.Priority, Comment: h.IsComment, Tags: h.Tags}
				if h.Properties != nil {
					hd.Id, _ = h.Properties.Get("ID")
					hd.CustomId, _ = h.Properties.Get("CUSTOM_ID")
				}
				if h.Scheduled != nil && h.Scheduled.Date != nil {
					hd.Sched = h.Scheduled.Date.ToString()
				}
				if h.Deadline != nil && h.Deadline.Date != nil {
					hd.Deadline = h.Deadline.Date.ToString()
				}
				hd.Title, hd.TitleCol = RawTitle(line(h.Pos.Row), h.Status, h.IsComment)
				out = append(out, hd)
			}
			walk(sec.Children)
		}
	}
	if doc.Outline.Section != nil {
		walk(doc.Outline.Children)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Line < out[b].Line })
	for i := range out {
		out[i].End = len(lines) - 1
		for j := i + 1; j < len(out); j++ {
			if out[j].Level <= out[i].Level {
				out[i].End = out[j].Line - 1
				break
			}
		}
	}
	// A trailing empty line after the final newline belongs to nothing.
	for i := range out {
		for out[i].End > out[i].Line && strings.TrimSpace(line(out[i].End)) == "" && out[i].End == len(lines)-1 {
			out[i].End--
		}
	}
	return out
}

// RawTitle is a heading line's title and the byte column it starts at.
func RawTitle(line, keyword string, comment bool) (string, int) {
	m := StarsRe.FindStringIndex(line)
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

// SameTitle is whether a link's *Title names a heading: org compares the
// title without its statistics cookie, and so does this, ignoring case and
// runs of space as org's search does.
func SameTitle(a, b string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(cookieRe.ReplaceAllString(s, "")), " "))
	}
	return norm(a) == norm(b)
}

// ---------------------------------------------------------------------------
// Blocks and drawers
// ---------------------------------------------------------------------------

type Block struct {
	Kind     string // src, example, quote ... lower case
	Lang     string // for src
	Name     string // #+NAME: above it
	Begin    int
	End      int // -1 when it has no #+END_
	NameLine int // -1 when unnamed
}

// Blocks are the #+BEGIN_ blocks of a buffer. One with no #+END_ stops at the
// next heading, except a src or example block, whose lines may start with a
// star; it then has End -1.
func Blocks(lines []string) []Block {
	out := []Block{}
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSuffix(lines[i], "\r")
		m := beginRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		b := Block{Kind: strings.ToLower(m[1]), Begin: i, End: -1, NameLine: -1}
		if b.Kind == "src" {
			b.Lang = m[2]
		}
		for k := i - 1; k >= 0; k-- {
			pl := strings.TrimSuffix(lines[k], "\r")
			if nm := NameRe.FindStringSubmatch(pl); nm != nil {
				b.Name, b.NameLine = nm[1], k
				break
			}
			if !strings.HasPrefix(strings.TrimSpace(pl), "#+") {
				break
			}
		}
		endRe := regexp.MustCompile(`(?i)^\s*#\+end_` + regexp.QuoteMeta(m[1]) + `\s*$`)
		for j := i + 1; j < len(lines); j++ {
			if endRe.MatchString(strings.TrimSuffix(lines[j], "\r")) {
				b.End = j
				break
			}
			if StarsRe.MatchString(lines[j]) && b.Kind != "src" && b.Kind != "example" {
				break
			}
		}
		out = append(out, b)
		if b.End > 0 {
			i = b.End
		}
	}
	return out
}

// InBlock is the block holding row, or nil.
func InBlock(bs []Block, row int) *Block {
	for i := range bs {
		if bs[i].Begin <= row && (row <= bs[i].End || (bs[i].End < 0 && row == bs[i].Begin)) {
			return &bs[i]
		}
	}
	return nil
}

type Drawer struct {
	Name       string
	Begin, End int // End -1 when it is never closed
}

// Drawers outside the blocks bs; one not closed before the next heading has
// End -1.
func Drawers(lines []string, bs []Block) []Drawer {
	out := []Drawer{}
	for i := 0; i < len(lines); i++ {
		if InBlock(bs, i) != nil {
			continue
		}
		l := strings.TrimSuffix(lines[i], "\r")
		m := drawerRe.FindStringSubmatch(l)
		if m == nil || strings.EqualFold(m[1], "end") {
			continue
		}
		d := Drawer{Name: m[1], Begin: i, End: -1}
		for j := i + 1; j < len(lines); j++ {
			lj := strings.TrimSuffix(lines[j], "\r")
			if drawerEndRe.MatchString(lj) {
				d.End = j
				break
			}
			if StarsRe.MatchString(lj) {
				break
			}
		}
		out = append(out, d)
		if d.End > 0 {
			i = d.End
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

type Link struct {
	Line       int
	Start, End int // bytes in the line, the whole of [[...]]
	Target     string
	Desc       string
}

// Links are the bracket links and plain urls outside blocks whose text is not
// org (quote, verse and center blocks hold org, so their links count).
func Links(lines []string, bs []Block) []Link {
	out := []Link{}
	for i, raw := range lines {
		if b := InBlock(bs, i); b != nil && b.Kind != "quote" && b.Kind != "verse" && b.Kind != "center" {
			continue
		}
		l := strings.TrimSuffix(raw, "\r")
		taken := [][]int{}
		for _, m := range bracketLinkRe.FindAllStringSubmatchIndex(l, -1) {
			lk := Link{Line: i, Start: m[0], End: m[1], Target: strings.ReplaceAll(l[m[2]:m[3]], `\]`, "]")}
			if m[4] >= 0 {
				lk.Desc = l[m[4]:m[5]]
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
				out = append(out, Link{Line: i, Start: m[0], End: m[1], Target: strings.TrimRight(l[m[0]:m[1]], ".,;:!?)")})
			}
		}
	}
	return out
}

// SplitLink is a link target's protocol and the rest: "id", "X" for id:X,
// "" for *Heading or #custom or a bare name.
func SplitLink(t string) (string, string) {
	if strings.HasPrefix(t, "./") || strings.HasPrefix(t, "../") || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~/") {
		return "file", t
	}
	if i := strings.Index(t, ":"); i > 0 && !strings.ContainsAny(t[:i], " \t*#") {
		return strings.ToLower(t[:i]), t[i+1:]
	}
	return "", t
}

// ResolveFile is a file: link's path, relative to the file it is written in.
func ResolveFile(docPath, p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(docPath), p)
	}
	return filepath.Clean(p)
}

// Search finds what a link's search part names in a buffer (its lines and
// headings): the row and the byte column to put the cursor at. A dedicated
// target, then a #+NAME:, then a heading, which is org's order.
func Search(lines []string, hs []Heading, q string) (int, int, bool) {
	q = strings.TrimSpace(q)
	if q == "" {
		return 0, 0, false
	}
	line := func(i int) string { return strings.TrimSuffix(lines[i], "\r") }
	if lineSearch.MatchString(q) {
		n, _ := strconv.Atoi(q)
		if n < 1 {
			n = 1
		}
		if n > len(lines) {
			n = len(lines)
		}
		return n - 1, 0, true
	}
	if strings.HasPrefix(q, "*") {
		t := strings.TrimSpace(q[1:])
		for _, h := range hs {
			if SameTitle(h.Title, t) {
				return h.Line, h.TitleCol, true
			}
		}
		return 0, 0, false
	}
	if strings.HasPrefix(q, "#") {
		for _, h := range hs {
			if h.CustomId == q[1:] {
				return h.Line, h.TitleCol, true
			}
		}
		return 0, 0, false
	}
	for i := range lines {
		l := line(i)
		for _, m := range TargetDefRe.FindAllStringSubmatchIndex(l, -1) {
			if strings.EqualFold(l[m[2]:m[3]], q) {
				return i, m[0], true
			}
		}
	}
	for i := range lines {
		if m := NameRe.FindStringSubmatch(line(i)); m != nil && strings.EqualFold(m[1], q) {
			return i, 0, true
		}
	}
	for _, h := range hs {
		if SameTitle(h.Title, q) {
			return h.Line, h.TitleCol, true
		}
	}
	return 0, 0, false
}

// ---------------------------------------------------------------------------
// What is wrong with a buffer
// ---------------------------------------------------------------------------

// Problem is one thing wrong, on one line: From..To are byte columns.
type Problem struct {
	Line, From, To int
	// error, warning or info.
	Severity string
	Code     string
	Message  string
}

// Env answers what the text cannot.
type Env interface {
	// FeatureOn is whether the server has an optional feature on.
	FeatureOn(name string) bool
	// File says whether path is there, and when it is not, whether that
	// should be said: a link to a folder that is not on this machine is
	// probably right somewhere else.
	File(path string) (exists, sayMissing bool)
	// LinesOf is a file's text, nil when it cannot be had.
	LinesOf(path string) []string
	// Headings of another file's lines, parsed as that file would be.
	Headings(path string, lines []string) []Heading
	// Id is whether a heading has the id; answered false means nobody could
	// be asked, and then no id is called broken.
	Id(id string) (found, answered bool)
}

// Structural is what is wrong with a buffer as org: path is where it lives
// (file: links are relative to it), hs its headings.
func Structural(path string, lines []string, hs []Heading, env Env) []Problem {
	out := []Problem{}
	line := func(i int) string { return strings.TrimSuffix(lines[i], "\r") }
	add := func(row, from, to int, sev, code, msg string) {
		out = append(out, Problem{Line: row, From: from, To: to, Severity: sev, Code: code, Message: msg})
	}
	indent := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	bs := Blocks(lines)
	for _, b := range bs {
		if b.End < 0 {
			l := line(b.Begin)
			add(b.Begin, indent(l), len(l), "error", "unclosed-block",
				fmt.Sprintf("#+BEGIN_%s with no #+END_%s", strings.ToUpper(b.Kind), strings.ToUpper(b.Kind)))
		}
	}
	for _, dr := range Drawers(lines, bs) {
		if dr.End < 0 {
			l := line(dr.Begin)
			add(dr.Begin, indent(l), len(l), "warning", "unclosed-drawer",
				fmt.Sprintf(":%s: drawer with no :END:", dr.Name))
		}
	}
	// org-edna's TRIGGER and BLOCKER, read as the server will run them -
	// only when the server has edna on, since otherwise they mean nothing.
	if env.FeatureOn("edna") {
		for row := range lines {
			l := line(row)
			m := ednaPropRe.FindStringSubmatchIndex(l)
			if m == nil {
				continue
			}
			prop, val := strings.ToUpper(l[m[2]:m[3]]), l[m[4]:m[5]]
			if err := common.ValidateEdna(prop, val); err != nil {
				from, to := m[4], m[5]
				if e, ok := err.(*common.EdnaError); ok {
					from, to = m[4]+e.Start, m[4]+max(e.End, e.Start+1)
					to = min(to, len(l))
				}
				add(row, from, to, "error", "edna", prop+": "+err.Error())
			}
		}
	}
	// Callouts of a type no row names (drawn as a note) - only with the
	// server's callouts on.
	if env.FeatureOn("callouts") {
		clean := make([]string, len(lines))
		for i := range lines {
			clean[i] = line(i)
		}
		for _, c := range common.CalloutsInLines(clean) {
			if c.Unknown != "" {
				l := clean[c.Line-1]
				add(c.Line-1, indent(l), len(l), "warning", "callout-type",
					fmt.Sprintf("no callout type %q: drawn as a note (orgs callouts types lists them)", c.Unknown))
			}
		}
	}
	for _, l := range Links(lines, bs) {
		proto, rest := SplitLink(l.Target)
		switch proto {
		case "":
			if _, _, ok := Search(lines, hs, rest); !ok {
				what := "no heading, target or #+NAME: called %q in this file"
				name := rest
				if strings.HasPrefix(rest, "*") {
					what, name = "no heading called %q in this file", strings.TrimSpace(rest[1:])
				} else if strings.HasPrefix(rest, "#") {
					what, name = "no heading with CUSTOM_ID %q in this file", rest[1:]
				}
				add(l.Line, l.Start, l.End, "warning", "broken-link", fmt.Sprintf(what, name))
			}
		case "file":
			file, search, _ := strings.Cut(rest, "::")
			p := ResolveFile(path, file)
			exists, say := env.File(p)
			if !exists {
				if say {
					add(l.Line, l.Start, l.End, "warning", "broken-link", "no file "+file)
				}
				continue
			}
			if search != "" && strings.EqualFold(filepath.Ext(p), ".org") {
				if other := env.LinesOf(p); other != nil {
					if _, _, ok := Search(other, env.Headings(p, other), search); !ok {
						add(l.Line, l.Start, l.End, "warning", "broken-link", fmt.Sprintf("%s has nothing called %q", filepath.Base(p), search))
					}
				}
			}
		case "id":
			if found, answered := env.Id(rest); answered && !found {
				add(l.Line, l.Start, l.End, "warning", "broken-link", "no heading has the id "+rest)
			}
		}
	}
	return out
}

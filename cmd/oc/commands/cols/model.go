package cols

// Org's column view, on the client side of the wire.
//
// The server reads the #+COLUMNS: line, fills each heading's cells and does
// the rollups (GET /columns); what is left is what a table needs and a server
// cannot know - which rows are folded away, which column is sorted, which
// value the rows are narrowed to - and the edits to the columns line that
// moving, resizing and the +path / +flat options make. A port of
// worg/src/columns.ts; the scenarios are pinned in model_test.go and
// columns.test.ts alike.
//
// The one rule worth stating: a cell's own value is not what the cell shows.
// On a parent that rolls up they differ, and an editor opened on the shown
// value would write a project's total effort onto the project heading.

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type (
	Spec   = common.ColumnSpec
	Cell   = common.ColumnCell
	Row    = common.ColumnRow
	Result = common.ColumnsResult
)

// Editable is whether a column can be typed into: the server already decided
// what a column is, so this asks its kind rather than keeping a list.
func Editable(s Spec) bool {
	switch s.Kind {
	case "property", "todo", "item", "tags", "priority":
		return true
	}
	return false
}

// HiddenRows is the rows under a folded heading. Folding is by hash over a
// flat list with levels, so a stack of levels does it in one pass.
func HiddenRows(rows []Row, folded map[string]bool) map[string]bool {
	hidden := map[string]bool{}
	foldAt := -1 // the level of the folded heading we are under, or -1
	for _, r := range rows {
		if foldAt >= 0 && r.Level > foldAt {
			hidden[r.Hash] = true
			continue
		}
		foldAt = -1
		if folded[r.Hash] && r.HasChildren {
			foldAt = r.Level
		}
	}
	return hidden
}

// SortWithinParents sorts siblings within their parent and walks the tree back
// out, which is what org does and the only sort that keeps the indentation
// meaning what it says. Empty cells go last either way round.
func SortWithinParents(rows []Row, col int, desc, numeric bool) []Row {
	type node struct {
		row  Row
		kids []*node
	}
	roots := []*node{}
	stack := []*node{}
	for _, r := range rows {
		n := &node{row: r}
		for len(stack) > 0 && stack[len(stack)-1].row.Level >= r.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			p := stack[len(stack)-1]
			p.kids = append(p.kids, n)
		}
		stack = append(stack, n)
	}
	cell := func(n *node) (Cell, bool) {
		if col < 0 || col >= len(n.row.Cells) {
			return Cell{}, false
		}
		return n.row.Cells[col], true
	}
	less := func(a, b *node) bool {
		ca, oka := cell(a)
		cb, okb := cell(b)
		if !oka || !okb {
			return false
		}
		ea := strings.TrimSpace(ca.Value) == ""
		eb := strings.TrimSpace(cb.Value) == ""
		if ea != eb {
			return eb
		}
		if ea {
			return false
		}
		if numeric {
			if desc {
				return ca.Number > cb.Number
			}
			return ca.Number < cb.Number
		}
		x, y := strings.ToLower(ca.Value), strings.ToLower(cb.Value)
		if desc {
			return x > y
		}
		return x < y
	}
	out := []Row{}
	var walk func(ns []*node)
	walk = func(ns []*node) {
		s := append([]*node{}, ns...)
		sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
		for _, n := range s {
			out = append(out, n.row)
			walk(n.kids)
		}
	}
	walk(roots)
	return out
}

// AncestorsOf is each heading's ancestors' titles, outermost first, worked out
// from the levels in file order (before any sort reorders the rows).
func AncestorsOf(rows []Row) map[string][]string {
	out := map[string][]string{}
	stack := []Row{}
	for _, r := range rows {
		for len(stack) > 0 && stack[len(stack)-1].Level >= r.Level {
			stack = stack[:len(stack)-1]
		}
		path := []string{}
		for _, s := range stack {
			path = append(path, s.Headline)
		}
		out[r.Hash] = path
		stack = append(stack, r)
	}
	return out
}

// RowsWithValue is the rows whose own value in col is value, and their
// ancestors, so each match keeps its place in the outline.
func RowsWithValue(rows []Row, col int, value string) map[string]bool {
	keep := map[string]bool{}
	stack := []Row{}
	for _, r := range rows {
		for len(stack) > 0 && stack[len(stack)-1].Level >= r.Level {
			stack = stack[:len(stack)-1]
		}
		if col < len(r.Cells) && strings.TrimSpace(r.Cells[col].Own) == value {
			keep[r.Hash] = true
			for _, a := range stack {
				keep[a.Hash] = true
			}
		}
		stack = append(stack, r)
	}
	return keep
}

// HoursMinutes is minutes as org's H:MM, counting past 24.
func HoursMinutes(mins float64) string {
	if mins <= 0 {
		return ""
	}
	w := int(math.Round(mins))
	return fmt.Sprintf("%d:%02d", w/60, w%60)
}

// SubtreeTotals is what each parent's subtree adds up to in a column the line
// does not sum (EFFORT with no {:}), its own value included, for showing a
// small total beside it. Only parents with something summed under them.
func SubtreeTotals(rows []Row, col int) map[string]float64 {
	type pending struct {
		level int
		total float64
		any   bool
	}
	totals := map[string]float64{}
	sums := []pending{}
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		total, any := 0.0, false
		for len(sums) > 0 && sums[len(sums)-1].level > r.Level {
			s := sums[len(sums)-1]
			sums = sums[:len(sums)-1]
			total += s.total
			any = any || s.any
		}
		childAny := any
		if col < len(r.Cells) && strings.TrimSpace(r.Cells[col].Own) != "" {
			total += r.Cells[col].Number
			any = true
		}
		if r.HasChildren && childAny {
			totals[r.Hash] = total
		}
		sums = append(sums, pending{r.Level, total, any})
	}
	return totals
}

// --- the columns line ---------------------------------------------------------------
//
// Emacs reads a #+COLUMNS: line by scanning for %... groups and skipping the
// rest, so words without a % are free for worg's options (+path, +flat) and
// harmless to save into the file.

var columnRe = regexp.MustCompile(`%(\d+)?([A-Za-z_][A-Za-z0-9_-]*)(?:\(([^)]*)\))?(?:\{([^}]*)\})?`)

type Token struct {
	Text       string
	Start, End int
	Width      int
	Prop       string
}

func Tokens(line string) []Token {
	out := []Token{}
	for _, m := range columnRe.FindAllStringSubmatchIndex(line, -1) {
		t := Token{Text: line[m[0]:m[1]], Start: m[0], End: m[1], Prop: strings.ToUpper(line[m[4]:m[5]])}
		if m[2] >= 0 {
			t.Width, _ = strconv.Atoi(line[m[2]:m[3]])
		}
		out = append(out, t)
	}
	return out
}

type Options struct {
	Flat bool
	Path int // ancestors shown before each title; 0 for none
}

var optionRe = regexp.MustCompile(`(?:^|\s)\+(flat|path)(?:=(\d+))?(?:\s|$)`)

func OptionsOf(line string) Options {
	o := Options{}
	// Spaced out so neighbouring options do not share the space between them.
	for _, m := range optionRe.FindAllStringSubmatch(strings.ReplaceAll(" "+line+" ", " ", "  "), -1) {
		switch m[1] {
		case "flat":
			o.Flat = true
		case "path":
			o.Path = 2
			if m[2] != "" {
				if n, _ := strconv.Atoi(m[2]); n >= 1 {
					o.Path = n
				}
			}
		}
	}
	return o
}

// SetOption is the line with an option set; off (false or 0) takes it away.
func SetOption(line, name string, value int) string {
	re := regexp.MustCompile(`(^|\s)\+` + name + `(?:=\d+)?(\s|$)`)
	without := line
	for re.MatchString(without) {
		without = re.ReplaceAllString(without, "$1$2")
	}
	without = strings.Join(strings.Fields(without), " ")
	if value <= 0 {
		return without
	}
	tok := "+" + name
	if name == "path" && value != 2 {
		tok = fmt.Sprintf("+path=%d", value)
	}
	return strings.TrimSpace(without + " " + tok)
}

// MoveColumn is the line with column from moved to position to. Anything that
// was not a column follows the columns in its original order.
func MoveColumn(line string, from, to int) string {
	toks := Tokens(line)
	if from < 0 || from >= len(toks) || to < 0 || to >= len(toks) || from == to {
		return line
	}
	order := []string{}
	for _, t := range toks {
		order = append(order, t.Text)
	}
	moved := order[from]
	order = append(order[:from], order[from+1:]...)
	order = append(order[:to], append([]string{moved}, order[to:]...)...)
	rest := line
	for i := len(toks) - 1; i >= 0; i-- {
		rest = rest[:toks[i].Start] + " " + rest[toks[i].End:]
	}
	return strings.Join(append(order, strings.Fields(rest)...), " ")
}

// SetWidth is the line with column index given a width in characters; 0
// takes the width off.
func SetWidth(line string, index, width int) string {
	toks := Tokens(line)
	if index < 0 || index >= len(toks) {
		return line
	}
	t := toks[index]
	bare := "%" + regexp.MustCompile(`^%\d*`).ReplaceAllString(t.Text, "")
	next := bare
	if width > 0 {
		next = fmt.Sprintf("%%%d%s", width, bare[1:])
	}
	return line[:t.Start] + next + line[t.End:]
}

// --- suggestions while the line is typed -----------------------------------------------

type Suggestion struct{ Value, Hint string }

var SpecialColumns = []Suggestion{
	{"ITEM", "the heading"}, {"TODO", "its keyword"}, {"PRIORITY", "its priority"},
	{"TAGS", "its own tags"}, {"ALLTAGS", "its tags and inherited ones"},
	{"CLOCKSUM", "time clocked, summed up the tree"}, {"CLOCKSUM_T", "today's clocked time"},
	{"DEADLINE", "its deadline"}, {"SCHEDULED", "when it is scheduled"}, {"CLOSED", "when it was closed"},
	{"CATEGORY", "its category"}, {"FILE", "its file"},
}

var SummaryOperators = []Suggestion{
	{":", "sum of times (H:MM)"}, {"+", "sum of numbers"}, {"$", "sum, as money"},
	{"min", "smallest number"}, {"max", "largest number"}, {"mean", "average number"},
	{":min", "shortest time"}, {":max", "longest time"}, {":mean", "average time"},
}

var ColumnOptions = []Suggestion{
	{"+path", "show two ancestors before each heading"}, {"+path=1", "show the parent before each heading"},
	{"+path=3", "show three ancestors before each heading"}, {"+flat", "no indentation or folding"},
}

var specWordRe = regexp.MustCompile(`^%(\d*)([A-Za-z0-9_-]*)(\([^)]*\)?)?(\{([^}]*))?$`)

// Complete is what fits the last word of the line: property names after %,
// operators inside {, options after +. It answers with the stretch of the
// line a pick replaces and the choices.
func Complete(text string, props []common.ColumnPropValues) (int, []Suggestion) {
	start := len(text)
	for start > 0 && text[start-1] != ' ' {
		start--
	}
	word := text[start:]
	if strings.HasPrefix(word, "+") {
		out := []Suggestion{}
		for _, o := range ColumnOptions {
			if strings.HasPrefix(o.Value, strings.ToLower(word)) && o.Value != word {
				out = append(out, o)
			}
		}
		return start, out
	}
	m := specWordRe.FindStringSubmatch(word)
	if m == nil {
		return len(text), nil
	}
	if m[4] != "" {
		part := m[5]
		out := []Suggestion{}
		for _, o := range SummaryOperators {
			if strings.HasPrefix(o.Value, part) && o.Value != part {
				out = append(out, o)
			}
		}
		return len(text) - len(part), out
	}
	if m[3] != "" {
		return len(text), nil
	}
	part := strings.ToUpper(m[2])
	from := start + 1 + len(m[1])
	cands := append([]Suggestion{}, SpecialColumns...)
	for _, p := range props {
		special := false
		for _, s := range SpecialColumns {
			if s.Value == p.Name {
				special = true
			}
		}
		if !special {
			n := "s"
			if p.Count == 1 {
				n = ""
			}
			cands = append(cands, Suggestion{p.Name, fmt.Sprintf("on %d heading%s", p.Count, n)})
		}
	}
	out := []Suggestion{}
	for _, c := range cands {
		if strings.HasPrefix(c.Value, part) && c.Value != part {
			out = append(out, c)
		}
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return from, out
}

// --- adding a heading in one line --------------------------------------------------------
//
// worg's quick add (worg/src/ganttquick.ts, parseQuickAdd), shared by its gantt
// chart and its column view:
//
//	NEXT Write the spec 3d @Sam #design ^fri !A RISK=high
//
// a keyword first, #tags, @person (ASSIGNED), effort, ^date (read the way
// orgs sched reads one - internal/orgdate, which is parseWhen's twin), !A
// priority, KEY=value properties; the rest is the title.

type Quick struct {
	Headline string
	Status   string
	Tags     []string
	Person   string
	Effort   string
	Start    string // YYYY-MM-DD
	After    string
	Priority string
	Props    map[string]string
}

var commonStatuses = []string{"TODO", "NEXT", "DOING", "IN-PROGRESS", "WAITING", "BLOCKED", "DONE"}

var (
	quickTokRe = regexp.MustCompile(`[@>#^]?"[^"]*"|\S+`)
	quoteRe    = regexp.MustCompile(`^([@>#^]?)"([^"]*)"$`)
	effortRe   = regexp.MustCompile(`(?i)^\d+(\.\d+)?[hdwm]$`)
	kvRe       = regexp.MustCompile(`^([A-Za-z_][\w-]*)=(.+)$`)
	prioRe     = regexp.MustCompile(`^![A-Za-z]$`)
)

func spaced(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "_", " ")) }

func ParseQuick(line string, statuses []string, now time.Time) Quick {
	q := Quick{Props: map[string]string{}}
	known := map[string]bool{}
	for _, s := range append(append([]string{}, statuses...), commonStatuses...) {
		known[s] = true
	}
	words := []string{}
	for i, raw := range quickTokRe.FindAllString(line, -1) {
		tok := quoteRe.ReplaceAllString(raw, "$1$2")
		switch {
		case i == 0 && known[tok]:
			q.Status = tok
		case strings.HasPrefix(tok, "#") && len(tok) > 1 && !strings.ContainsAny(tok[1:], " #"):
			for _, t := range strings.Split(tok[1:], ":") {
				if t != "" && !containsStr(q.Tags, t) {
					q.Tags = append(q.Tags, t)
				}
			}
		case strings.HasPrefix(tok, "@") && len(tok) > 1:
			q.Person = spaced(tok[1:])
		case strings.HasPrefix(tok, "^") && len(tok) > 1 && q.parseStart(tok[1:], now):
		case strings.HasPrefix(tok, ">") && len(tok) > 1:
			q.After = spaced(tok[1:])
		case prioRe.MatchString(tok):
			q.Priority = strings.ToUpper(tok[1:])
		case effortRe.MatchString(tok):
			q.Effort = strings.ToLower(tok)
		case kvRe.MatchString(tok):
			m := kvRe.FindStringSubmatch(tok)
			q.Props[strings.ToUpper(m[1])] = spaced(m[2])
		default:
			words = append(words, raw)
		}
	}
	q.Headline = strings.TrimSpace(strings.Join(words, " "))
	return q
}

func (q *Quick) parseStart(s string, now time.Time) bool {
	d, _, err := commands.ParseDate(s, now)
	if err != nil {
		return false
	}
	q.Start = d.Day.Format("2006-01-02")
	return true
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

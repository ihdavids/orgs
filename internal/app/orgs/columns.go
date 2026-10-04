package orgs

// Column view, and the rollup that makes it worth having.
//
// `EFFORT` was read in exactly one place - the gantt plugin, to work out how
// long a bar should be - and nowhere else. It is org's estimate property, the
// one thing in a project file that says how big the work is, and there was no
// way to see it, no way to add it up, and no way to edit it except one property
// at a time through `orgs prop`.
//
// This is org's answer to that: `#+COLUMNS:` names some properties and the
// outline is drawn as a table of them. The part that is not a table is the
// **summary operator**: `%EFFORT{:}` means a parent heading shows the total of
// the effort underneath it, so a project says how big it is without anybody
// maintaining a number that would go stale the moment a task was added.
//
// Three decisions shape the file.
//
// **Properties are read off the file's own lines, not from `Headline.Properties`.**
// go-org ends a headline's body at a drawer written in column zero, which leaves
// that field nil and hoists the rest of the heading to the top of the document -
// the trap that runs through this whole codebase. A column view whose EFFORT
// column was blank for every heading whose drawer happens to start at column
// zero would look like the properties not being set. The file is read once and
// every heading's drawer is found in those lines.
//
// **A cell carries both what is shown and what is written.** They are the same
// thing on a leaf and different on a parent, and conflating them is a data-loss
// bug waiting to happen: a project showing `12:00` summed from its tasks has no
// effort of its own, and an editor pre-filled with `12:00` writes that total
// onto the parent the first time anybody opens the cell and presses return.
//
// **The rollup is the children's total plus the heading's own value.** Org's own
// behaviour here is easy to misremember and the two readings only differ when a
// parent has an estimate *and* children with estimates - which is a parent
// carrying its own overhead. Of the two readings, this is the one that never
// silently discards a number somebody typed.

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Org's default, which is also org's: the item, its keyword, its priority and
// its tags.
//
// With one addition. `%EFFORT{:}` is on the end because the whole reason this
// view is here is to add effort up, and a default that did not show it would
// leave every file that has not declared a `#+COLUMNS:` line - which is nearly
// all of them - showing the one thing somebody opened this view to avoid
// looking for. A file that declares its own line still gets exactly that line.
//
// Without org's character widths (`%25ITEM %3PRIORITY`): those are for a
// terminal, and in worg a column with no width shares the screen - the item
// taking a good part of it - where a fixed 25 characters left the item the
// narrowest column on a wide screen.
const defaultColumns = "%ITEM %TODO %PRIORITY %TAGS %EFFORT{:} %CLOCKSUM"

// The special property names org understands, and what kind of cell each is.
//
// Anything not in here is an ordinary property, which is the editable case and
// the common one.
var specialColumns = map[string]string{
	"ITEM":       "item",
	"TODO":       "todo",
	"PRIORITY":   "priority",
	"TAGS":       "tags",
	"ALLTAGS":    "derived",
	"CLOCKSUM":   "derived",
	"CLOCKSUM_T": "derived",
	// A heading's own CATEGORY property, written like any property; what
	// is shown falls back to the file's #+CATEGORY.
	"CATEGORY":   "property",
	"FILE":       "derived",
	"DEADLINE":   "derived",
	"SCHEDULED":  "derived",
	"CLOSED":     "derived",
}

// `%25EFFORT(Estimate){:}` - an optional width, the property, an optional title
// in brackets, an optional summary operator in braces.
var columnRe = regexp.MustCompile(`%(\d+)?([A-Za-z_][A-Za-z0-9_-]*)(?:\(([^)]*)\))?(?:\{([^}]*)\})?`)

// ParseColumnSpec reads a `#+COLUMNS:` line.
//
// Anything that is not a `%...` group is ignored rather than refused: org's own
// parser is equally forgiving, and a column view that refuses to draw because
// somebody left a stray word in the line is worse than one that draws the
// columns it understood.
func ParseColumnSpec(line string) []common.ColumnSpec {
	var out []common.ColumnSpec
	for _, m := range columnRe.FindAllStringSubmatch(line, -1) {
		spec := common.ColumnSpec{Property: strings.ToUpper(m[2])}
		if m[1] != "" {
			spec.Width, _ = strconv.Atoi(m[1])
		}
		spec.Title = m[3]
		if spec.Title == "" {
			spec.Title = titleCaseProp(spec.Property)
		}
		spec.Summary = m[4]
		if kind, ok := specialColumns[spec.Property]; ok {
			spec.Kind = kind
		} else {
			spec.Kind = "property"
		}
		spec.Numbers = numberKindOf(spec)
		out = append(out, spec)
	}
	return out
}

// EFFORT -> Effort, CLOCKSUM_T -> Clocksum T. A property is written in capitals
// in the file and reads as shouting in a table header.
func titleCaseProp(p string) string {
	words := strings.FieldsFunc(p, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// How a column's values are read for summing.
//
// The `:` in `{:}` is org's way of saying "these are times", and the clock
// columns are times whether or not anybody said so. Everything else summed is a
// plain number.
func numberKindOf(spec common.ColumnSpec) string {
	switch spec.Property {
	case "CLOCKSUM", "CLOCKSUM_T":
		return "duration"
	}
	if spec.Property == "EFFORT" {
		// Effort is a duration however it is summed, because that is what the
		// property holds - `2h`, `1:30`, `3d`. Reading it as a plain number
		// would make `1:30` one and a half of nothing.
		return "duration"
	}
	if spec.Summary == "" {
		return ""
	}
	if strings.HasPrefix(spec.Summary, ":") {
		return "duration"
	}
	return "number"
}

// What this column does to its children, which is the operator it was given -
// except for the clock columns, which are a subtree total in org whether or not
// anybody wrote an operator. `%CLOCKSUM` means "the time spent under here", and
// a column view that showed only the time clocked against the parent heading
// itself would read as almost every project having no time on it.
func opFor(spec common.ColumnSpec) string {
	if op := summaryOp(spec.Summary); op != "" {
		return op
	}
	switch spec.Property {
	case "CLOCKSUM", "CLOCKSUM_T":
		return "sum"
	}
	return ""
}

// Does this operator roll up at all, and does it sum or pick?
func summaryOp(s string) string {
	// A format may be appended after a semicolon (`+;%.1f`), which changes how
	// the answer is printed and not what it is.
	op, _, _ := strings.Cut(s, ";")
	op = strings.TrimSpace(op)
	switch op {
	case "+", "$", ":":
		return "sum"
	case "min", ":min":
		return "min"
	case "max", ":max":
		return "max"
	case "mean", ":mean":
		return "mean"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Reading a file's headings
// ---------------------------------------------------------------------------

// Which `#+COLUMNS:` line applies, and where it came from.
func columnsFor(f *common.OrgFile, asked string) (string, string) {
	if strings.TrimSpace(asked) != "" {
		return strings.TrimSpace(asked), "request"
	}
	if f != nil && f.Doc != nil {
		if c := strings.TrimSpace(f.Doc.Get("COLUMNS")); c != "" {
			return c, "file"
		}
	}
	if c := Conf(); c != nil && c.Server != nil {
		if d := strings.TrimSpace(c.Server.Columns.Default); d != "" {
			return d, "config"
		}
	}
	return defaultColumns, "config"
}

// The values a property is allowed to take, org's way: a `#+PROPERTY:
// Effort_ALL 0 0:10 0:30 1:00` line, or an `:EFFORT_ALL:` property somewhere up
// the outline.
//
// This is what makes editing an estimate a matter of picking rather than typing,
// which is most of what "easily" means for a column nobody wants to think hard
// about.
func allowedValues(f *common.OrgFile, prop string) []string {
	want := strings.ToUpper(prop) + "_ALL"
	if f == nil || f.Doc == nil {
		return nil
	}
	for _, p := range strings.Split(f.Doc.Get("PROPERTY"), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), " ")
		if ok && strings.EqualFold(k, want) {
			return strings.Fields(v)
		}
	}
	return nil
}

// A heading's own property, looked up without caring how it was capitalised -
// org treats `:Effort:` and `:EFFORT:` as the same property and so must this,
// or a file written by hand shows blank cells.
func propOf(props map[string]string, key string) string {
	if v, ok := props[key]; ok {
		return v
	}
	for k, v := range props {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// Minutes clocked on this heading itself, and optionally only today's.
func clockMinutes(h *org.Headline, todayOnly bool) float64 {
	if h == nil {
		return 0
	}
	total := 0.0
	now := time.Now()
	for _, c := range h.Clocks {
		if c == nil || c.Date == nil {
			continue
		}
		if todayOnly {
			s := c.Date.Start
			if s.Year() != now.Year() || s.Month() != now.Month() || s.Day() != now.Day() {
				continue
			}
		}
		total += float64(c.Date.DurationMins)
	}
	return total
}

// ---------------------------------------------------------------------------
// The view
// ---------------------------------------------------------------------------

// A heading being turned into a row, with its place in the tree kept so the
// rollup can walk back up it.
type columnNode struct {
	sec      *org.Section
	props    map[string]string
	children []*columnNode
	row      common.ColumnRow
}

// BuildColumnView is the whole of GET /columns for one file.
func BuildColumnView(filename, asked string) (common.ColumnsResult, error) {
	res := common.ColumnsResult{File: filename}
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil {
		return res, fmt.Errorf("no file called %s", filename)
	}
	line, from := columnsFor(f, asked)
	res.Columns, res.From = line, from
	res.Spec = ParseColumnSpec(line)
	if len(res.Spec) == 0 {
		return res, fmt.Errorf("%q names no columns", line)
	}
	for i := range res.Spec {
		if res.Spec[i].Kind == "property" {
			res.Spec[i].Allowed = allowedValues(f, res.Spec[i].Property)
		}
	}

	// The file's lines once, for the property drawers. Asking go-org for them
	// would leave every heading whose drawer starts in column zero with none.
	lines := fileLines(filename)

	roots := buildColumnNodes(f, lines, res.Spec)
	for _, n := range roots {
		summarise(n, res.Spec)
	}
	var flatten func(ns []*columnNode)
	flatten = func(ns []*columnNode) {
		for _, n := range ns {
			res.Rows = append(res.Rows, n.row)
			flatten(n.children)
		}
	}
	flatten(roots)
	res.Ok = true
	return res, nil
}

// Every heading of the file as a tree of nodes, each with its own cell values
// filled in but nothing summed yet.
func buildColumnNodes(f *common.OrgFile, lines []string, spec []common.ColumnSpec) []*columnNode {
	var build func(secs []*org.Section) []*columnNode
	build = func(secs []*org.Section) []*columnNode {
		var out []*columnNode
		for _, s := range secs {
			if s == nil || s.Headline == nil {
				continue
			}
			n := &columnNode{sec: s, props: propsOfSection(lines, s)}
			n.children = build(s.Children)
			n.row = ownRow(f, s, n.props, spec, len(n.children) > 0)
			out = append(out, n)
		}
		sort.SliceStable(out, func(a, b int) bool {
			return out[a].row.LineNum < out[b].row.LineNum
		})
		return out
	}
	return build(f.Doc.Outline.Children)
}

// This heading's own values, before anything is added up.
func ownRow(f *common.OrgFile, s *org.Section, props map[string]string, spec []common.ColumnSpec, hasKids bool) common.ColumnRow {
	h := s.Headline
	var title strings.Builder
	for _, t := range h.Title {
		title.WriteString(t.String())
	}
	row := common.ColumnRow{
		Hash: s.Hash, Headline: strings.TrimSpace(title.String()),
		Level: h.Lvl, LineNum: h.Pos.Row, Status: h.Status,
		Priority: h.Priority, Tags: h.Tags, HasChildren: hasKids,
	}
	for _, c := range spec {
		row.Cells = append(row.Cells, ownCell(f, s, props, c))
	}
	return row
}

func ownCell(f *common.OrgFile, s *org.Section, props map[string]string, spec common.ColumnSpec) common.ColumnCell {
	h := s.Headline
	cell := common.ColumnCell{}
	switch spec.Property {
	case "ITEM":
		var b strings.Builder
		for _, t := range h.Title {
			b.WriteString(t.String())
		}
		cell.Value = strings.TrimSpace(b.String())
	case "TODO":
		cell.Value = h.Status
	case "PRIORITY":
		cell.Value = h.Priority
	case "TAGS":
		cell.Value = strings.Join(h.Tags, ":")
	case "ALLTAGS":
		cell.Value = strings.Join(inheritedTags(s), ":")
	case "FILE":
		cell.Value = f.Filename
	case "CATEGORY":
		cell.Value = propOf(props, "CATEGORY")
		if cell.Value == "" && f.Doc != nil {
			cell.Value = strings.TrimSpace(f.Doc.Get("CATEGORY"))
		}
	case "CLOCKSUM", "CLOCKSUM_T":
		// Only what is clocked on this heading itself. The subtree total is the
		// rollup's job, and doing it here as well would count every clock line
		// once per ancestor.
		mins := clockMinutes(h, spec.Property == "CLOCKSUM_T")
		cell.Number = mins
		if mins > 0 {
			cell.Value = hoursMinutes(mins)
		}
	case "DEADLINE":
		cell.Value = sdcString(h.Deadline)
	case "SCHEDULED":
		cell.Value = sdcString(h.Scheduled)
	case "CLOSED":
		cell.Value = sdcString(h.Closed)
	default:
		cell.Value = propOf(props, spec.Property)
	}
	cell.Own = cell.Value
	if spec.Property == "CATEGORY" {
		// The file's #+CATEGORY is shown but is not the heading's own, and an
		// editor offering it would write it onto the heading.
		cell.Own = propOf(props, "CATEGORY")
	}
	if spec.Numbers != "" {
		if n, ok := cellNumber(cell.Value, spec.Numbers); ok {
			cell.Number = n
		}
	}
	return cell
}

func sdcString(s *org.SDC) string {
	if s == nil || s.Date == nil {
		return ""
	}
	return s.Date.ToString()
}

// Every tag reaching this heading, its ancestors' included - which is what org
// means by ALLTAGS.
func inheritedTags(s *org.Section) []string {
	var out []string
	seen := map[string]bool{}
	for n := s; n != nil; n = n.Parent {
		if n.Headline == nil {
			continue
		}
		for _, t := range n.Headline.Tags {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}

// A cell's value as a number, read the way this column says to read it.
func cellNumber(v, kind string) (float64, bool) {
	if strings.TrimSpace(v) == "" {
		return 0, false
	}
	if kind == "duration" {
		if d := common.ParseDuration(v); d != nil {
			return d.Mins, true
		}
		return 0, false
	}
	// A number may be written with something after it - `3 pages`, `$4` - and
	// the leading number is what is meant.
	m := regexp.MustCompile(`-?\d+(?:\.\d+)?`).FindString(v)
	if m == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(m, 64)
	return n, err == nil
}

// Add the tree up, deepest first.
//
// A column with no summary operator is left exactly as the heading wrote it,
// which is org's rule and the reason `%EFFORT` and `%EFFORT{:}` are different
// things: the operator is how a file says it wants a total.
func summarise(n *columnNode, spec []common.ColumnSpec) {
	for _, c := range n.children {
		summarise(c, spec)
	}
	for i, s := range spec {
		op := opFor(s)
		if op == "" || len(n.children) == 0 {
			continue
		}
		var vals []float64
		// The heading's own value counts too. Of the two readings of a parent
		// that has an estimate *and* children with estimates, this is the one
		// that does not quietly throw away the number somebody typed on the
		// parent.
		if own, ok := cellNumber(n.row.Cells[i].Own, s.Numbers); ok {
			vals = append(vals, own)
		}
		for _, c := range n.children {
			if c.row.Cells[i].Value == "" {
				continue
			}
			vals = append(vals, c.row.Cells[i].Number)
		}
		if len(vals) == 0 {
			continue
		}
		total := apply(op, vals)
		n.row.Cells[i].Number = total
		n.row.Cells[i].Value = formatSummary(total, s)
		n.row.Cells[i].Summed = true
	}
}

func apply(op string, vals []float64) float64 {
	switch op {
	case "min":
		out := vals[0]
		for _, v := range vals {
			out = math.Min(out, v)
		}
		return out
	case "max":
		out := vals[0]
		for _, v := range vals {
			out = math.Max(out, v)
		}
		return out
	case "mean":
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		return sum / float64(len(vals))
	default:
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		return sum
	}
}

// How a total is written.
//
// A duration goes out as `H:MM`, which is what org's column view shows and what
// `OrgDuration.ToString` deliberately does not do - it writes `2h 30mins`, which
// is right for prose and wrong for a column of figures nobody wants to have to
// align by eye.
func formatSummary(total float64, spec common.ColumnSpec) string {
	if _, format, ok := strings.Cut(spec.Summary, ";"); ok && strings.TrimSpace(format) != "" {
		return fmt.Sprintf(strings.TrimSpace(format), total)
	}
	if spec.Numbers == "duration" {
		return hoursMinutes(total)
	}
	if total == math.Trunc(total) {
		return strconv.FormatInt(int64(total), 10)
	}
	return strconv.FormatFloat(total, 'f', 2, 64)
}

// Minutes as `H:MM`, counting past 24 rather than wrapping - a project of three
// days' effort is `72:00`, not `0:00`.
func hoursMinutes(mins float64) string {
	if mins <= 0 {
		return ""
	}
	whole := int(math.Round(mins))
	return fmt.Sprintf("%d:%02d", whole/60, whole%60)
}

// ---------------------------------------------------------------------------
// The endpoint
// ---------------------------------------------------------------------------

func columnJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// The file's lines, for reading property drawers out of them.
func fileLines(filename string) []string {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// One heading's own properties, found in lines already read.
//
// This is propsFromLines given a heading's extent rather than a whole file, and
// it is the reason the file is read here at all: `Headline.Properties` is nil for
// a drawer written in column zero, so asking go-org would leave those headings
// with no EFFORT in a view whose entire purpose is to show it.
func propsOfSection(lines []string, sec *org.Section) map[string]string {
	if sec == nil || sec.Headline == nil || len(lines) == 0 {
		return map[string]string{}
	}
	from := sec.Headline.Pos.Row
	if from < 0 || from >= len(lines) {
		return map[string]string{}
	}
	to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
	if to >= len(lines) {
		to = len(lines) - 1
	}
	return propsFromLines(lines, from, to)
}

/* SDOC: API
* GET /columns — Org Column View, With Rollups

	Answers with a file's outline as org's column view: the =#+COLUMNS:= line in
	force, the columns it names, and every heading with those properties filled
	in - including the **summed** value on any heading with children, for a
	column whose spec carries a summary operator.

	This is the only thing in orgs that adds =EFFORT= up. A =%EFFORT{:}= column
	makes a project heading show the total estimate of everything under it, which
	is a number nobody has to maintain.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Required | Description                                          |
	|-----------+----------+------------------------------------------------------|
	| =file=    | yes      | The org file to view.                                |
	| =columns= | no       | A =#+COLUMNS:= line to use instead of the file's own. |

	The line is taken from the first of these that says anything: the =columns=
	parameter, the file's own =#+COLUMNS:=, =columns.default= in the server
	config, and finally a built-in default. The answer says which in =From=.

	*Summary operators:* =+= and =$= and =:= sum, =min=, =max= and =mean= pick,
	and any of them may carry a printf format after a semicolon (=+;%.1f=). A
	=:= prefix, and the =EFFORT= and =CLOCKSUM= columns whatever their operator,
	are read as durations and summed into =H:MM=. Org's checkbox operators
	(=X=, =X/=, =X%=) and its age operators (=@min= and friends) are not
	implemented and a column carrying one simply does not roll up.

	*Response:* A =ColumnsResult=. Each cell carries =Value= (what to show,
	which is the rollup on a summed parent), =Own= (what is written on that
	heading, which is what an edit changes) and =Number= (the value as a number,
	for sorting).

	Editing is through the endpoints that already exist: =POST /property= for an
	ordinary property, =POST /status/change= for =TODO=, =POST /headline/change=
	for =ITEM=. A cell's =Kind= on the spec says which.
	EDOC */
func RequestColumns(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	if strings.TrimSpace(filename) == "" {
		columnJson(w, common.ColumnsResult{Ok: false, Msg: "no file given"})
		return
	}
	res, err := BuildColumnView(filename, r.URL.Query().Get("columns"))
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
	}
	columnJson(w, res)
}

// ---------------------------------------------------------------------------
// Writing the file's own #+COLUMNS: line
// ---------------------------------------------------------------------------

var columnsLineRe = regexp.MustCompile(`(?i)^\s*#\+COLUMNS:`)
var leadingKeywordRe = regexp.MustCompile(`^\s*(#\+[A-Za-z_]+:|#\s|\s*$)`)

// setFileColumns writes a `#+COLUMNS:` line into a file, replacing the one it
// had.
//
// It is a line edit, like everything else that writes in here: a column view is
// something people set up while looking at a project file they are in the middle
// of, and reformatting every table in it to record a display preference would be
// a poor trade.
//
// Where the line goes when the file has none is the only judgement here. It joins
// the run of `#+KEYWORD:` lines at the top, after the last of them - which is
// where org's own `C-c C-x C-c` leaves it and where anybody looking for it would
// look. Putting it at the very top would push `#+TITLE:` down, which is the one
// line in that block people expect to see first.
func setFileColumns(filename, spec string) error {
	lines := fileLines(filename)
	if len(lines) == 0 {
		return fmt.Errorf("could not read %s", filename)
	}
	want := "#+COLUMNS: " + strings.TrimSpace(spec)
	for i, l := range lines {
		if headlineRe.MatchString(l) {
			break
		}
		if columnsLineRe.MatchString(l) {
			lines[i] = want
			return writeLines(filename, lines)
		}
	}
	// After the last of the leading keyword lines, skipping trailing blanks so
	// the new line joins the block rather than floating below it.
	at := 0
	for at < len(lines) && leadingKeywordRe.MatchString(lines[at]) && !headlineRe.MatchString(lines[at]) {
		at++
	}
	for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	return writeLines(filename, splice(lines, at, []string{want}))
}

/* SDOC: API
* POST /columns/spec — Write a File's #+COLUMNS: Line

	Sets the =#+COLUMNS:= line of a file, replacing the one it had or adding one
	to the block of =#+KEYWORD:= lines at the top. This is how a column view set
	up by hand is kept: without it the spec is a thing you retype every time.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field     | Type   | Required | Description                        |
	|-----------+--------+----------+------------------------------------|
	| =File=    | string | yes      | The org file to write.             |
	| =Columns= | string | yes      | The spec, without the =#+COLUMNS:= prefix. |

	*Response:* A =ResultMsg=.
	EDOC */
func PostColumnSpec(w http.ResponseWriter, r *http.Request) {
	var args struct {
		File    string
		Columns string
	}
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if strings.TrimSpace(args.File) == "" {
		columnJson(w, common.ResultMsg{Ok: false, Msg: "no file given"})
		return
	}
	// The spec has to name at least one column, or the file would be left with a
	// line that draws nothing and no way to tell that from a broken view.
	if len(ParseColumnSpec(args.Columns)) == 0 {
		columnJson(w, common.ResultMsg{Ok: false, Msg: "that names no columns"})
		return
	}
	if GetDb().FindByFile(args.File) == nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: "no file called " + args.File})
		return
	}
	if err := setFileColumns(args.File, args.Columns); err != nil {
		columnJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	columnJson(w, common.ResultMsg{Ok: true})
}

// ---------------------------------------------------------------------------
// Writing one property
// ---------------------------------------------------------------------------

// Set or remove one property on one heading, as a line edit.
//
// This is what `POST /property` does now. It used to set the property on the
// parsed document and write the whole thing back through go-org, which
// reformatted every drawer in the file and reflowed every table in it - so
// changing one estimate re-indented the lot, and a `CLOCK:` line lost a space on
// the way past.
//
// That was tolerable while a property was something you set once from a kanban
// drag. A column view is a screen you sit in front of filling in estimates, and
// at one whole-file rewrite per cell it is not: every edit would show up as a
// hundred changed lines to anybody with the file open in an editor or under git.
//
// Setting a property to nothing takes it off, and takes the drawer with it when
// it was the last one - an empty `:PROPERTIES:` drawer is litter, and undo
// writing a previous value of "nothing" back has to leave the heading as it
// found it.
func setHeadingProperty(f *common.OrgFile, sec *org.Section, key, val string) error {
	if f == nil || sec == nil || sec.Headline == nil {
		return fmt.Errorf("no heading to change")
	}
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", f.Filename)
	}
	ind := indentOf(sec.Headline.Lvl)

	if strings.TrimSpace(val) == "" {
		ps, pe, found := drawerAt(lines, from, to, "PROPERTIES")
		if !found {
			return nil
		}
		kept := 0
		for i := ps + 1; i < pe && i < len(lines); i++ {
			if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], key) {
				lines = append(lines[:i], lines[i+1:]...)
				pe--
				i--
				continue
			}
			kept++
		}
		if kept == 0 {
			// The drawer and its END, now that nothing is in it.
			lines = append(lines[:ps], lines[pe+1:]...)
		} else {
			alignDrawer(lines, ps, pe)
		}
		return writeLines(f.Filename, lines)
	}

	var ps, pe int
	lines, ps, pe, _ = ensurePropertyDrawer(lines, from, to, ind)
	lines, _ = setPropIn(lines, ps, pe, ind, key, val)
	return writeLines(f.Filename, lines)
}

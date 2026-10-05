package orgs

/* SDOC: Editing
* Clock Table

  Time clocked under headings, summed. Two ways to get one:

  - =orgs clocktable -block thisweek= (and =GET /clockreport=) report every
    file at once, straight to the terminal;
  - a =#+BEGIN: clocktable= dynamic block writes the report into the file and
    rewrites it whenever it is refreshed. See *Dynamic Blocks* for its
    parameters.

  A range is one of:

  | Written                                  | Means                                    |
  |------------------------------------------+------------------------------------------|
  | =today=, =yesterday=, =today-N=          | a day                                    |
  | =thisweek=, =lastweek=, =thisweek-N=     | a week, Monday to Sunday                 |
  | =thismonth=, =lastmonth=, =thismonth-N=  | a month                                  |
  | =thisyear=, =lastyear=, =thisyear-N=     | a year                                   |
  | =untilnow=                               | everything ever clocked                  |
  | =2026-10-04=, =2026-10=, =2026=          | that day, month or year                  |
  | =2026-W40=, =2026-Q3=                    | that ISO week, that quarter              |

  A clock running across the edge of the range counts only the part inside it,
  and one still running counts up to now.
EDOC */

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

// ----------------------------------------------------------------------------
// Ranges
// ----------------------------------------------------------------------------

// A span of time a report covers: from is inclusive, to exclusive. A zero
// clockRange (Set false) covers everything.
type clockRange struct {
	Set   bool
	From  time.Time
	To    time.Time
	Label string
}

var relRangeRe = regexp.MustCompile(`^(today|thisweek|thismonth|thisyear)([-+]\d+)?$`)

// clockBlockRange reads a clock table's :block the way org does. now is passed
// in so it can be tested without waiting for a Monday.
func clockBlockRange(spec string, now time.Time) (clockRange, error) {
	s := strings.ToLower(strings.TrimSpace(spec))
	loc := now.Location()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	switch s {
	case "":
		return clockRange{}, nil
	case "untilnow":
		return clockRange{Set: true, From: time.Time{}, To: now, Label: "until now"}, nil
	case "yesterday":
		s = "today-1"
	case "lastweek":
		s = "thisweek-1"
	case "lastmonth":
		s = "thismonth-1"
	case "lastyear":
		s = "thisyear-1"
	}
	if m := relRangeRe.FindStringSubmatch(s); m != nil {
		n := 0
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		}
		switch m[1] {
		case "today":
			from := day.AddDate(0, 0, n)
			return clockRange{true, from, from.AddDate(0, 0, 1), from.Format("Monday, January 2, 2006")}, nil
		case "thisweek":
			monday := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
			from := monday.AddDate(0, 0, 7*n)
			y, w := from.ISOWeek()
			return clockRange{true, from, from.AddDate(0, 0, 7), fmt.Sprintf("week %d-W%02d", y, w)}, nil
		case "thismonth":
			from := time.Date(day.Year(), day.Month()+time.Month(n), 1, 0, 0, 0, 0, loc)
			return clockRange{true, from, from.AddDate(0, 1, 0), from.Format("January 2006")}, nil
		case "thisyear":
			from := time.Date(day.Year()+n, 1, 1, 0, 0, 0, 0, loc)
			return clockRange{true, from, from.AddDate(1, 0, 0), from.Format("2006")}, nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return clockRange{true, t, t.AddDate(0, 0, 1), t.Format("Monday, January 2, 2006")}, nil
	}
	if t, err := time.ParseInLocation("2006-01", s, loc); err == nil {
		return clockRange{true, t, t.AddDate(0, 1, 0), t.Format("January 2006")}, nil
	}
	if m := regexp.MustCompile(`^(\d{4})-w(\d{1,2})$`).FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		w, _ := strconv.Atoi(m[2])
		// The fourth of January is always in week one.
		jan4 := time.Date(y, 1, 4, 0, 0, 0, 0, loc)
		from := jan4.AddDate(0, 0, -((int(jan4.Weekday())+6)%7)+7*(w-1))
		return clockRange{true, from, from.AddDate(0, 0, 7), fmt.Sprintf("week %d-W%02d", y, w)}, nil
	}
	if m := regexp.MustCompile(`^(\d{4})-q([1-4])$`).FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		q, _ := strconv.Atoi(m[2])
		from := time.Date(y, time.Month(3*(q-1)+1), 1, 0, 0, 0, 0, loc)
		return clockRange{true, from, from.AddDate(0, 3, 0), fmt.Sprintf("%d-Q%d", y, q)}, nil
	}
	if regexp.MustCompile(`^\d{4}$`).MatchString(s) {
		y, _ := strconv.Atoi(s)
		from := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		return clockRange{true, from, from.AddDate(1, 0, 0), s}, nil
	}
	return clockRange{}, fmt.Errorf("%q is not a range a clock table knows (today, thisweek, lastmonth, 2026-W40, 2026-Q3, ...)", spec)
}

// The range a clock table's parameters ask for: :block, narrowed or replaced
// by :tstart and :tend.
func clockRangeOf(params map[string]string, now time.Time) (clockRange, error) {
	r, err := clockBlockRange(params["block"], now)
	if err != nil {
		return r, err
	}
	edge := func(key string) (time.Time, bool, error) {
		v := strings.TrimSpace(params[key])
		if v == "" || v == "nil" {
			return time.Time{}, false, nil
		}
		d, ok, err := orgdate.ParseDate(v, now)
		if err != nil || !ok {
			return time.Time{}, false, fmt.Errorf(":%s %q is not a date", key, v)
		}
		return d.Day, true, nil
	}
	from, hasFrom, err := edge("tstart")
	if err != nil {
		return r, err
	}
	to, hasTo, err := edge("tend")
	if err != nil {
		return r, err
	}
	if hasFrom || hasTo {
		if !r.Set {
			r = clockRange{Set: true, To: now.AddDate(100, 0, 0)}
		}
		if hasFrom {
			r.From = from
		}
		if hasTo {
			r.To = to
		}
		r.Label = r.From.Format("2006-01-02") + " to " + r.To.Format("2006-01-02")
		if !hasFrom {
			r.Label = "until " + r.To.Format("2006-01-02")
		} else if !hasTo {
			r.Label = "since " + r.From.Format("2006-01-02")
		}
	}
	return r, nil
}

// Minutes of one clock line that fall in the range. Without a range it is the
// duration the line was written with, so a report agrees with the file to the
// minute; a clock still running counts up to now.
func clockMinsIn(clk *org.Clock, r clockRange, now time.Time) float64 {
	if clk == nil || clk.Date == nil || clk.Date.Start.IsZero() {
		return 0
	}
	start, end := clk.Date.Start, clk.Date.End
	running := end.IsZero()
	if running {
		end = now
	}
	if !r.Set {
		if running {
			return end.Sub(start).Minutes()
		}
		return float64(clk.Date.DurationMins)
	}
	if start.Before(r.From) {
		start = r.From
	}
	if end.After(r.To) {
		end = r.To
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start).Minutes()
}

func headlineClockMins(h *org.Headline, r clockRange, now time.Time) float64 {
	if h == nil {
		return 0
	}
	total := 0.0
	for _, c := range h.Clocks {
		total += clockMinsIn(c, r, now)
	}
	return total
}

// A heading's clock lines cut to the range, oldest first.
func clockSessionsIn(h *org.Headline, r clockRange, now time.Time) []common.ClockSpan {
	out := []common.ClockSpan{}
	for _, c := range h.Clocks {
		mins := clockMinsIn(c, r, now)
		if mins <= 0 {
			continue
		}
		start, end := c.Date.Start, c.Date.End
		running := end.IsZero()
		if running {
			end = now
		}
		if r.Set && start.Before(r.From) {
			start = r.From
		}
		if r.Set && end.After(r.To) {
			end = r.To
		}
		span := common.ClockSpan{Start: start.Format("2006-01-02T15:04"), Mins: mins}
		if !running {
			span.End = end.Format("2006-01-02T15:04")
		}
		out = append(out, span)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out
}

// H:MM, the way org writes a duration in a clock table.
func clockHM(mins float64) string {
	m := int(mins + 0.5)
	return fmt.Sprintf("%d:%02d", m/60, m%60)
}

// ----------------------------------------------------------------------------
// The report across every file
// ----------------------------------------------------------------------------

func collectClockEntries(sec *org.Section, r clockRange, now time.Time, filename string, entries *[]common.ClockEntry) {
	if mins := headlineClockMins(sec.Headline, r, now); mins > 0 {
		*entries = append(*entries, common.ClockEntry{
			Headline: common.GetSectionTitle(sec),
			Filename: filename,
			Level:    sec.Headline.Lvl,
			Mins:     mins,
			Sessions: clockSessionsIn(sec.Headline, r, now),
		})
	}
	for _, c := range sec.Children {
		collectClockEntries(c, r, now, filename, entries)
	}
}

// GenerateClockReport builds a ClockReport across all files for a given block range.
func GenerateClockReport(blockName string) *common.ClockReport {
	report := &common.ClockReport{Block: blockName}
	now := time.Now()
	r, err := clockBlockRange(blockName, now)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	for _, fname := range GetDb().GetFiles() {
		ofile := GetDb().GetFile(fname)
		if ofile == nil || ofile.Doc == nil {
			continue
		}
		for _, sec := range ofile.Doc.Outline.Children {
			collectClockEntries(sec, r, now, fname, &report.Entries)
		}
	}
	for _, e := range report.Entries {
		report.TotalMin += e.Mins
	}
	return report
}

// ----------------------------------------------------------------------------
// The clocktable dynamic block
// ----------------------------------------------------------------------------

type ctRow struct {
	level int
	title string
	mins  float64
}

type ctFile struct {
	name  string
	total float64
	rows  []ctRow
}

// What a clock table counts: the range, and which headings may contribute
// their own time.
type ctFilter struct {
	r     clockRange
	now   time.Time
	match *MatchExpr
	query *Expr
}

func (c *ctFilter) counts(f *common.OrgFile, s *org.Section) bool {
	if c.match != nil {
		ok := c.match.EvalSection(f, s)
		c.match.Reset()
		if !ok {
			return false
		}
	}
	if c.query != nil && !EvalString(c.query, s, f) {
		return false
	}
	return true
}

// Rows for a subtree, deepest last, each with the time of everything under it.
// A heading with nothing clocked in range is left out, as org leaves it out.
// level is the heading's depth in the table, which starts at one.
func (c *ctFilter) walk(f *common.OrgFile, s *org.Section, level, maxLevel int, rows *[]ctRow) float64 {
	if s == nil || s.Headline == nil {
		return 0
	}
	at := len(*rows)
	*rows = append(*rows, ctRow{level: level, title: common.GetSectionTitle(s)})
	total := 0.0
	if c.counts(f, s) {
		total = headlineClockMins(s.Headline, c.r, c.now)
	}
	for _, ch := range s.Children {
		total += c.walk(f, ch, level+1, maxLevel, rows)
	}
	// Nothing in range, or too deep to list: its row comes back out, and
	// with it any its children added. The time still counts upwards.
	if total <= 0 || (maxLevel > 0 && level > maxLevel) {
		*rows = (*rows)[:at]
		return total
	}
	(*rows)[at].mins = total
	return total
}

func dynClocktable(c *dynCtx) ([]string, error) {
	r, err := clockRangeOf(c.Block.Params, c.Now)
	if err != nil {
		return nil, err
	}
	filter := &ctFilter{r: r, now: c.Now}
	if m := c.param("match", ""); m != "" && m != "nil" {
		filter.match = NewMatchExpr(m)
	}
	if q := c.param("query", ""); q != "" && q != "nil" {
		exp, err := ParseString(&common.StringQuery{Query: expandQueryFilters(q)})
		if err != nil {
			return nil, fmt.Errorf(":query does not parse: %v", err)
		}
		filter.query = exp
	}
	maxLevel, err := strconv.Atoi(c.param("maxlevel", "3"))
	if err != nil {
		return nil, fmt.Errorf(":maxlevel must be a number")
	}
	indent := dynTrue(c.param("indent", "t"))

	// What is reported: the roots of each file's part of it.
	scope := c.param("scope", "file")
	type part struct {
		f     *common.OrgFile
		roots []*org.Section
	}
	parts := []part{}
	whole := func(f *common.OrgFile) part { return part{f, f.Doc.Outline.Children} }
	treeAt := func(level int) error {
		if c.Owner == nil {
			return fmt.Errorf(":scope %s needs the block to be under a heading", scope)
		}
		s := c.Owner
		for s.Parent != nil && s.Parent.Headline != nil && s.Headline.Lvl > level {
			s = s.Parent
		}
		parts = append(parts, part{c.File, []*org.Section{s}})
		return nil
	}
	switch {
	case scope == "file" || scope == "nil":
		parts = append(parts, whole(c.File))
	case scope == "subtree":
		if c.Owner == nil {
			return nil, fmt.Errorf(":scope subtree needs the block to be under a heading")
		}
		parts = append(parts, part{c.File, []*org.Section{c.Owner}})
	case scope == "tree":
		if err := treeAt(1); err != nil {
			return nil, err
		}
	case strings.HasPrefix(scope, "tree"):
		n, err := strconv.Atoi(strings.TrimPrefix(scope, "tree"))
		if err != nil {
			return nil, fmt.Errorf(":scope %q is not one a clock table knows", scope)
		}
		if err := treeAt(n); err != nil {
			return nil, err
		}
	case scope == "agenda" || scope == "agenda-with-archives" || scope == "all":
		for _, name := range GetDb().GetFiles() {
			if f := GetDb().GetFile(name); f != nil && f.Doc != nil {
				parts = append(parts, whole(f))
			}
		}
	case strings.HasPrefix(scope, "("):
		for _, name := range dynList(scope) {
			if !filepath.IsAbs(name) {
				name = filepath.Join(filepath.Dir(c.File.Doc.Path), name)
			}
			f := GetDb().FindByFile(name)
			if f == nil || f.Doc == nil {
				return nil, fmt.Errorf("no file called %q", name)
			}
			parts = append(parts, whole(f))
		}
	default:
		return nil, fmt.Errorf(":scope %q is not one a clock table knows (file, subtree, tree, treeN, agenda, or a list of files)", scope)
	}

	files := []ctFile{}
	grand := 0.0
	deepest := 1
	for _, p := range parts {
		cf := ctFile{name: relativeName(c.File.Doc.Path, p.f.Doc.Path)}
		for _, root := range p.roots {
			cf.total += filter.walk(p.f, root, 1, maxLevel, &cf.rows)
		}
		for _, row := range cf.rows {
			if row.level > deepest {
				deepest = row.level
			}
		}
		grand += cf.total
		files = append(files, cf)
	}

	// Each root's level in the table is one, whatever its depth in the file -
	// a subtree's report reads the same wherever the subtree is.
	timeCols := deepest
	if maxLevel > 0 && timeCols > maxLevel {
		timeCols = maxLevel
	}
	several := len(parts) > 1
	skipEmptyFiles := dynTrue(c.param("fileskip0", ""))

	header := []string{"Headline", "Time"}
	for i := 1; i < timeCols; i++ {
		header = append(header, "")
	}
	timeAt := func(level int, v string) []string {
		cells := make([]string, timeCols)
		if level < 1 {
			level = 1
		}
		if level > timeCols {
			level = timeCols
		}
		cells[level-1] = v
		return cells
	}
	if several {
		header = append([]string{"File"}, header...)
	}
	table := [][]string{header, nil}
	totalLabel := "*Total time*"
	if several {
		totalLabel = "ALL *Total time*"
	}
	total := append([]string{totalLabel}, timeAt(1, "*"+clockHM(grand)+"*")...)
	if several {
		total = append([]string{""}, total...)
	}
	table = append(table, total, nil)
	for _, cf := range files {
		if several {
			if skipEmptyFiles && cf.total <= 0 {
				continue
			}
			table = append(table, append([]string{cf.name, "*File time*"}, timeAt(1, "*"+clockHM(cf.total)+"*")...))
		}
		for _, row := range cf.rows {
			title := cellText(row.title)
			if indent {
				title = levelIndent(row.level) + title
			}
			cells := append([]string{title}, timeAt(row.level, clockHM(row.mins))...)
			if several {
				cells = append([]string{""}, cells...)
			}
			table = append(table, cells)
		}
		if several {
			table = append(table, nil)
		}
	}
	if several && table[len(table)-1] == nil {
		table = table[:len(table)-1]
	}

	caption := "#+CAPTION: Clock summary at [" + c.Now.Format("2006-01-02 Mon 15:04") + "]"
	if r.Set && r.Label != "" {
		caption += ", for " + r.Label + "."
	}
	return append([]string{caption}, orgTableLines(table)...), nil
}

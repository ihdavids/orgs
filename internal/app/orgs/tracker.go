package orgs

// Trackers (features.tracker), in org's own spelling. A #+TRACKER: line anywhere in a file says
// what to follow and how to draw it, in org-plot's key:value words:
//
//	#+TRACKER: type:heatmap source:property:MOOD from:-1y
//	#+TRACKER: source:query:"IsDone()" type:bar from:-8w
//	#+TRACKER: source:habit:Run color:green goal:1
//	#+TRACKER: source:clock:Writing type:line
//
// and a #+BEGIN: tracker dynamic block takes the same words and writes the
// days as an org table with summary rows. A series is one value a calendar
// day:
//
//   - property:NAME - the property on day pages (a file-level drawer or
//     #+PROPERTY: line in a day page, or a heading on one) and on any heading
//     with a day: a datetree title, a heading under one, CLOSED, CREATED,
//     SCHEDULED, DEADLINE, a timestamp (date: picks one).
//   - tag:NAME, query:"..." - how many headings a day.
//   - habit:Title - the habit's completions (parseHabitCompletions).
//   - clock:Title (or a query, or *) - minutes clocked a day in the subtree.
//   - words - words written a day (the writing log).
//   - table:NAME - a #+NAME:d table's date column and a value column.
//
// The series is computed when it is asked for; nothing is stored. While the
// feature is on the export copy (exportLinesFor) has each #+TRACKER: line
// replaced with its chart: an inline SVG for html, TikZ for LaTeX, the SVG
// inline for Typst and a sparkline in plain text. A file never changes for it.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

const trackerFeature = "tracker"

func init() {
	dynGenerators["tracker"] = dynTracker
}

var (
	trackerLineRe  = regexp.MustCompile(`(?i)^(\s*)#\+TRACKER:\s*(.*)$`)
	trackerBeginRe = regexp.MustCompile(`(?i)^\s*#\+BEGIN:\s+tracker\b\s*(.*)$`)
	// A date at the start of a heading's title: a datetree day, or a stamp.
	trackerTitleDateRe = regexp.MustCompile(`^\s*[<\[]?(\d{4}-\d{2}-\d{2})`)
	trackerAnyDateRe   = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)
	// Day page names: orgs's Mon_2006_01_02, org-roam's 2006-01-02, org-journal's 20060102.
	dayPageNameRes = []*regexp.Regexp{
		regexp.MustCompile(`^[A-Za-z]{3}_(\d{4})_(\d{2})_(\d{2})$`),
		regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`),
		regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`),
	}
	trackerPropKwRe = regexp.MustCompile(`(?i)^\s*#\+PROPERTY:\s*(\S+)\s+(.*)$`)
	trackerDateKwRe = regexp.MustCompile(`(?i)^\s*#\+DATE:\s*(.*)$`)
	trackerNameKwRe = regexp.MustCompile(`(?i)^\s*#\+NAME:\s*(.+?)\s*$`)
)

var trackerWeekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// ----------------------------------------------------------------------------
// A tracker's options
// ----------------------------------------------------------------------------

type trackerSpec struct {
	Line       string
	Opts       []common.TrackerOption
	Kind, Arg  string
	Type, Agg  string
	Title      string
	Color      string
	Unit       string
	DateFrom   string // date: which date of a heading counts
	FileFilter string
	From, To   time.Time
	Goal       *float64
	Min, Max   *float64
	DateCol    string
	ValCol     string
	// Bare leaves the title and summary off the picture, for a client
	// that shows them itself (worg's cards).
	Bare bool
}

func (s trackerSpec) get(key, def string) string { return common.TrackerGet(s.Opts, key, def) }

// zeroFill is whether a day with nothing is a zero (a count) rather than a
// gap (a property nobody wrote down that day).
func (s trackerSpec) zeroFill() bool {
	switch s.Kind {
	case "tag", "query", "habit", "clock", "words":
		return true
	}
	return s.Agg == "count"
}

func trackerDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// parseTrackerSpec reads a #+TRACKER: line's options.
func parseTrackerSpec(line string, now time.Time) (trackerSpec, error) {
	opts := common.ParseTrackerOptions(line)
	s := trackerSpec{Line: strings.TrimSpace(line), Opts: opts}
	kind, arg, ok := common.TrackerSourceOf(opts)
	if !ok {
		if kind == "" {
			return s, fmt.Errorf("a tracker needs a source: source:property:MOOD, tag:NAME, query:\"...\", habit:NAME, clock:NAME, words or table:NAME")
		}
		return s, fmt.Errorf("%q is not a source a tracker knows (%s)", kind, strings.Join(common.TrackerSources, ", "))
	}
	s.Kind, s.Arg = kind, arg
	if kind != "words" && kind != "clock" && strings.TrimSpace(arg) == "" {
		return s, fmt.Errorf("source:%s needs a name after it: source:%s:NAME", kind, kind)
	}
	s.Type = strings.ToLower(s.get("type", "heatmap"))
	switch s.Type {
	case "heatmap", "calendar":
		s.Type = "heatmap"
	case "line", "lines", "2d":
		s.Type = "line"
	case "bar", "bars", "histogram", "histograms", "column":
		s.Type = "bar"
	case "sparkline", "spark":
		s.Type = "sparkline"
	default:
		return s, fmt.Errorf("type:%s is not a chart a tracker draws (heatmap, line, bar, sparkline)", s.Type)
	}
	defAgg := "sum"
	if kind == "property" || kind == "table" {
		defAgg = "mean"
	}
	s.Agg = strings.ToLower(s.get("agg", s.get("aggregate", defAgg)))
	switch s.Agg {
	case "avg", "average":
		s.Agg = "mean"
	case "total":
		s.Agg = "sum"
	}
	if !trackerKnown(common.TrackerAggs, s.Agg) {
		return s, fmt.Errorf("agg:%s is not one a tracker knows (%s)", s.Agg, strings.Join(common.TrackerAggs, ", "))
	}
	s.Title = s.get("title", "")
	if s.Title == "" {
		s.Title = trackerDefaultTitle(kind, arg)
	}
	s.Color = s.get("color", s.get("colour", ""))
	s.Unit = s.get("unit", "")
	s.DateFrom = strings.ToLower(s.get("date", "auto"))
	switch s.DateFrom {
	case "auto", "title", "page", "closed", "created", "scheduled", "deadline", "timestamp":
	default:
		return s, fmt.Errorf("date:%s is not a date a tracker reads (auto, title, page, closed, created, scheduled, deadline, timestamp)", s.DateFrom)
	}
	s.FileFilter = s.get("file", "")
	s.DateCol = s.get("datecol", "")
	s.ValCol = s.get("col", s.get("valcol", ""))
	for _, k := range []struct {
		key string
		dst **float64
	}{{"goal", &s.Goal}, {"min", &s.Min}, {"max", &s.Max}} {
		if v := s.get(k.key, ""); v != "" {
			n, _, ok := common.TrackerValue(v)
			if !ok {
				return s, fmt.Errorf("%s:%s is not a number", k.key, v)
			}
			*k.dst = &n
		}
	}
	today := trackerDay(now)
	s.To = today
	if v := s.get("to", ""); v != "" {
		d, ok, err := orgdate.ParseDate(v, now)
		if err != nil || !ok {
			return s, fmt.Errorf("to:%s is not a date", v)
		}
		s.To = trackerDay(d.Day)
	}
	defFrom := "-30d"
	if s.Type == "heatmap" {
		defFrom = "-1y"
	}
	v := s.get("from", defFrom)
	d, ok, err := orgdate.ParseDate(v, s.To.Add(12*time.Hour))
	if err != nil || !ok {
		return s, fmt.Errorf("from:%s is not a date", v)
	}
	s.From = trackerDay(d.Day)
	if s.Type == "heatmap" && s.get("from", "") == "" {
		// A year back reads as 53 columns, starting the day after.
		s.From = s.From.AddDate(0, 0, 1)
	}
	if s.From.After(s.To) {
		return s, fmt.Errorf("from: is after to:")
	}
	if s.To.Sub(s.From) > 3700*24*time.Hour {
		return s, fmt.Errorf("a tracker covers ten years at most")
	}
	return s, nil
}

func trackerKnown(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func trackerDefaultTitle(kind, arg string) string {
	switch kind {
	case "words":
		return "Words written"
	case "clock":
		if arg == "" || arg == "*" {
			return "Time clocked"
		}
		return arg + " (clocked)"
	case "query":
		return arg
	case "tag":
		return ":" + arg + ":"
	}
	return arg
}

// ----------------------------------------------------------------------------
// The day a thing happened
// ----------------------------------------------------------------------------

// dayPageDate is the day a file's name says it is for, if it is a day page.
func dayPageDate(filename string) (time.Time, bool) {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	for _, re := range dayPageNameRes {
		if m := re.FindStringSubmatch(base); m != nil {
			y, _ := strconv.Atoi(m[1])
			mo, _ := strconv.Atoi(m[2])
			d, _ := strconv.Atoi(m[3])
			t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.Local)
			if t.Month() == time.Month(mo) && t.Day() == d && y > 1900 {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func parseTrackerDate(s string) (time.Time, bool) {
	m := trackerAnyDateRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", m[1], time.Local)
	return t, err == nil
}

func titleDay(sec *org.Section) (time.Time, bool) {
	if m := trackerTitleDateRe.FindStringSubmatch(headlineTitle(sec)); m != nil {
		return parseTrackerDate(m[1])
	}
	return time.Time{}, false
}

// weekdayOf reads a heading titled for a day of the week: Mon, Monday.
func weekdayOf(title string) (time.Weekday, bool) {
	t := strings.ToLower(strings.TrimSpace(title))
	if len(t) < 3 {
		return 0, false
	}
	for i, d := range trackerWeekdays {
		if t == d || t == d[:3] {
			return time.Weekday(i), true
		}
	}
	return 0, false
}

// pageDay is the day a heading on a day page is for: the page's day, or on a
// week page the day its Mon/Tue heading names.
func pageDay(f *common.OrgFile, sec *org.Section) (time.Time, bool) {
	page, ok := dayPageDate(f.Filename)
	if !ok {
		return time.Time{}, false
	}
	for s := sec; s != nil && s.Headline != nil; s = s.Parent {
		if wd, ok := weekdayOf(headlineTitle(s)); ok {
			return page.AddDate(0, 0, (int(wd)-int(page.Weekday())+7)%7), true
		}
	}
	return page, true
}

func sdcDay(s *org.SDC) (time.Time, bool) {
	if s == nil || s.Date == nil || s.Date.Start.IsZero() {
		return time.Time{}, false
	}
	return trackerDay(s.Date.Start), true
}

// sectionDay is the day a heading counts on. auto takes, in order: a date
// starting its title (a datetree day), a heading above it that is one, the
// day page it is on, CLOSED, CREATED, SCHEDULED, DEADLINE, its timestamp.
func sectionDay(f *common.OrgFile, sec *org.Section, mode string) (time.Time, bool) {
	h := sec.Headline
	try := func(m string) (time.Time, bool) {
		switch m {
		case "title":
			return titleDay(sec)
		case "page":
			for s := sec.Parent; s != nil && s.Headline != nil; s = s.Parent {
				if t, ok := titleDay(s); ok {
					return t, true
				}
			}
			return pageDay(f, sec)
		case "closed":
			return sdcDay(h.Closed)
		case "scheduled":
			return sdcDay(h.Scheduled)
		case "deadline":
			return sdcDay(h.Deadline)
		case "created":
			if v, ok := ownProp(sec, "CREATED"); ok {
				return parseTrackerDate(v)
			}
		case "timestamp":
			for _, ts := range h.Timestamps {
				if ts != nil && ts.Time != nil && !ts.Time.Start.IsZero() {
					return trackerDay(ts.Time.Start), true
				}
			}
			return parseTrackerDate(headlineTitle(sec))
		}
		return time.Time{}, false
	}
	if mode != "auto" {
		return try(mode)
	}
	for _, m := range []string{"title", "page", "closed", "created", "scheduled", "deadline", "timestamp"} {
		if t, ok := try(m); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// ----------------------------------------------------------------------------
// The values
// ----------------------------------------------------------------------------

type trackerSample struct {
	day  string
	v    float64
	kind string
	file string
	hash string
	line int
}

type trackerWalk struct {
	spec    trackerSpec
	user    string
	now     time.Time
	cache   lineCache
	samples []trackerSample
	kinds   map[string]int
}

func (w *trackerWalk) add(day time.Time, v float64, kind, file, hash string, line int) {
	if day.Before(w.spec.From) || day.After(w.spec.To) {
		return
	}
	w.samples = append(w.samples, trackerSample{day.Format("2006-01-02"), v, kind, file, hash, line})
	w.kinds[kind]++
}

func (w *trackerWalk) fileWanted(name string) bool {
	ff := w.spec.FileFilter
	if ff == "" {
		return true
	}
	if ok, _ := filepath.Match(ff, filepath.Base(name)); ok {
		return true
	}
	return strings.Contains(filepath.ToSlash(name), ff)
}

func (w *trackerWalk) sections(fn func(f *common.OrgFile, sec *org.Section)) {
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if w.fileWanted(f.Filename) {
			fn(f, sec)
		}
	})
}

// titleMatches finds the headings a habit:/clock: names: the same title, or
// failing that every title containing it.
func (w *trackerWalk) titled(name string) []trackerHit {
	exact, partial := []trackerHit{}, []trackerHit{}
	want := strings.ToLower(strings.TrimSpace(name))
	w.sections(func(f *common.OrgFile, sec *org.Section) {
		t := strings.ToLower(strings.TrimSpace(common.GetSectionTitle(sec)))
		if t == want {
			exact = append(exact, trackerHit{f, sec})
		} else if strings.Contains(t, want) {
			partial = append(partial, trackerHit{f, sec})
		}
	})
	if len(exact) > 0 {
		return exact
	}
	return partial
}

type trackerHit struct {
	f   *common.OrgFile
	sec *org.Section
}

func (w *trackerWalk) queried(q string) ([]trackerHit, error) {
	out := []trackerHit{}
	err := queryEachSection(&common.StringQuery{Query: q}, func(sec *org.Section, f *common.OrgFile) {
		if w.fileWanted(f.Filename) {
			out = append(out, trackerHit{f, sec})
		}
	})
	return out, err
}

func (w *trackerWalk) run() error {
	s := w.spec
	switch s.Kind {
	case "property":
		w.property(s.Arg)
	case "tag":
		w.sections(func(f *common.OrgFile, sec *org.Section) {
			if ednaHasTag(sec, s.Arg) {
				if d, ok := sectionDay(f, sec, s.DateFrom); ok {
					w.add(d, 1, "count", f.Filename, sec.Hash, sec.Headline.Pos.Row)
				}
			}
		})
	case "query":
		hits, err := w.queried(s.Arg)
		if err != nil {
			return fmt.Errorf("query %q: %v", s.Arg, err)
		}
		for _, h := range hits {
			if d, ok := sectionDay(h.f, h.sec, s.DateFrom); ok {
				w.add(d, 1, "count", h.f.Filename, h.sec.Hash, h.sec.Headline.Pos.Row)
			}
		}
	case "habit":
		hits := w.titled(s.Arg)
		if len(hits) == 0 {
			return fmt.Errorf("no heading called %q to follow as a habit", s.Arg)
		}
		for _, h := range hits {
			seen := map[string]bool{}
			for _, c := range parseHabitCompletions(h.sec) {
				if seen[c] {
					continue // a day is a day, however many ticks it had
				}
				seen[c] = true
				if d, err := time.ParseInLocation("2006-01-02", c, time.Local); err == nil {
					w.add(d, 1, "count", h.f.Filename, h.sec.Hash, h.sec.Headline.Pos.Row)
				}
			}
		}
	case "clock":
		return w.clock(s.Arg)
	case "words":
		w.words()
	case "table":
		return w.table(s.Arg)
	}
	return nil
}

// property: on headings with a day, and in day pages' own drawers and
// #+PROPERTY: lines.
func (w *trackerWalk) property(name string) {
	w.sections(func(f *common.OrgFile, sec *org.Section) {
		v, ok := ownProp(sec, name)
		if !ok {
			return
		}
		n, kind, ok := common.TrackerValue(v)
		if !ok {
			return
		}
		if d, ok := sectionDay(f, sec, w.spec.DateFrom); ok {
			w.add(d, n, kind, f.Filename, sec.Hash, sec.Headline.Pos.Row)
		}
	})
	if w.spec.DateFrom != "auto" && w.spec.DateFrom != "page" {
		return
	}
	for _, name0 := range GetDb().GetFiles() {
		if !w.fileWanted(name0) {
			continue
		}
		lines := w.cache.of(name0)
		day, isPage := dayPageDate(name0)
		val, row, found := fileLevelProp(lines, name)
		if !found {
			continue
		}
		if !isPage {
			// A file that is not a day page counts on its #+DATE:.
			for _, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), "*") {
					break
				}
				if m := trackerDateKwRe.FindStringSubmatch(l); m != nil {
					day, isPage = parseTrackerDate(m[1])
				}
			}
		}
		if !isPage {
			continue
		}
		if n, kind, ok := common.TrackerValue(val); ok {
			w.add(day, n, kind, name0, "", row)
		}
	}
}

// fileLevelProp is a property above a file's first heading: in its property
// drawer, or on a #+PROPERTY: line.
func fileLevelProp(lines []string, name string) (string, int, bool) {
	inDrawer := false
	val, row, found := "", 0, false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(l, "*") && headingLineRe.MatchString(l) {
			break
		}
		switch {
		case strings.EqualFold(t, ":PROPERTIES:"):
			inDrawer = true
		case strings.EqualFold(t, ":END:"):
			inDrawer = false
		case inDrawer:
			if m := propLineRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[2], name) {
				val, row, found = strings.TrimSpace(m[4]), i, true
			}
		default:
			if m := trackerPropKwRe.FindStringSubmatch(l); m != nil && strings.EqualFold(m[1], name) {
				val, row, found = strings.TrimSpace(m[2]), i, true
			}
		}
	}
	return val, row, found
}

// clock: minutes a day under the headings named (every heading for * or
// nothing), each clock line cut at midnight.
func (w *trackerWalk) clock(arg string) error {
	var roots []trackerHit
	switch {
	case arg == "" || arg == "*":
		w.sections(func(f *common.OrgFile, sec *org.Section) {
			w.clockLines(f, sec)
		})
		return nil
	case strings.Contains(arg, "("):
		hits, err := w.queried(arg)
		if err != nil {
			return fmt.Errorf("query %q: %v", arg, err)
		}
		roots = hits
	default:
		roots = w.titled(arg)
		if len(roots) == 0 {
			return fmt.Errorf("no heading called %q to add up the clock under", arg)
		}
	}
	seen := map[*org.Section]bool{}
	var walk func(f *common.OrgFile, sec *org.Section)
	walk = func(f *common.OrgFile, sec *org.Section) {
		if seen[sec] {
			return
		}
		seen[sec] = true
		w.clockLines(f, sec)
		for _, c := range sec.Children {
			walk(f, c)
		}
	}
	for _, r := range roots {
		walk(r.f, r.sec)
	}
	return nil
}

func (w *trackerWalk) clockLines(f *common.OrgFile, sec *org.Section) {
	if sec.Headline == nil {
		return
	}
	for _, c := range sec.Headline.Clocks {
		if c == nil || c.Date == nil || c.Date.Start.IsZero() || c.Date.End.IsZero() {
			continue
		}
		for d := trackerDay(c.Date.Start); !d.After(c.Date.End); d = d.AddDate(0, 0, 1) {
			if d.Before(w.spec.From) || d.After(w.spec.To) {
				continue
			}
			r := clockRange{Set: true, From: d, To: d.AddDate(0, 0, 1)}
			if m := clockMinsIn(c, r, w.now); m > 0 {
				w.add(d, m, "duration", f.Filename, sec.Hash, sec.Headline.Pos.Row)
			}
		}
	}
}

// words: the writing log, this user's (everybody's without one).
func (w *trackerWalk) words() {
	ext := GetExtensions()
	if ext == nil {
		return
	}
	ext.mu.RLock()
	defer ext.mu.RUnlock()
	for name, u := range ext.Users {
		if w.user != "" && name != w.user {
			continue
		}
		for file, days := range u.WritingLog {
			if !w.fileWanted(file) {
				continue
			}
			for date, wd := range days {
				d, err := time.ParseInLocation("2006-01-02", date, time.Local)
				if err != nil || wd.End-wd.Start <= 0 {
					continue
				}
				w.add(d, float64(wd.End-wd.Start), "number", file, "", 0)
			}
		}
	}
	// One file's words a sample; sum them to the day.
	if w.spec.Agg == "mean" {
		w.spec.Agg = "sum"
	}
}

// table: a #+NAME:d table, a row a sample.
func (w *trackerWalk) table(name string) error {
	found := false
	for _, fn := range GetDb().GetFiles() {
		if !w.fileWanted(fn) {
			continue
		}
		lines := w.cache.of(fn)
		for i := 0; i < len(lines); i++ {
			m := trackerNameKwRe.FindStringSubmatch(lines[i])
			if m == nil || !strings.EqualFold(m[1], name) {
				continue
			}
			start := i + 1
			for start < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[start]), "#+") {
				start++
			}
			if start >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[start]), "|") {
				continue
			}
			found = true
			rows, names, _ := plotTableRows(lines, start)
			dc, vc := trackerColumns(rows, names, w.spec.DateCol, w.spec.ValCol)
			if dc < 0 || vc < 0 {
				return fmt.Errorf("table %s: no date column and value column (datecol:1 col:2)", name)
			}
			for _, r := range rows {
				if dc >= len(r) || vc >= len(r) {
					continue
				}
				d, ok := parseTrackerDate(r[dc])
				if !ok {
					continue
				}
				if v, kind, ok := common.TrackerValue(r[vc]); ok {
					w.add(d, v, kind, fn, "", start)
				}
			}
		}
	}
	if !found {
		return fmt.Errorf("no table named %s (a #+NAME: %s line above it)", name, name)
	}
	return nil
}

// trackerColumns picks a table's date and value columns: by number (1 is the
// first) or header name, else the first column of dates and the first after
// it with values.
func trackerColumns(rows [][]string, names []string, dateCol, valCol string) (int, int) {
	pick := func(spec string) int {
		if spec == "" {
			return -1
		}
		if n, err := strconv.Atoi(spec); err == nil {
			return n - 1
		}
		for i, h := range names {
			if strings.EqualFold(strings.TrimSpace(h), spec) {
				return i
			}
		}
		return -2
	}
	dc, vc := pick(dateCol), pick(valCol)
	if dc == -2 || vc == -2 {
		return -1, -1
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if dc < 0 {
		for c := 0; c < cols && dc < 0; c++ {
			for _, r := range rows {
				if c < len(r) && strings.TrimSpace(r[c]) != "" {
					if _, ok := parseTrackerDate(r[c]); ok {
						dc = c
					}
					break
				}
			}
		}
	}
	if vc < 0 {
		for c := 0; c < cols && vc < 0; c++ {
			if c == dc {
				continue
			}
			for _, r := range rows {
				if c < len(r) && strings.TrimSpace(r[c]) != "" {
					if _, _, ok := common.TrackerValue(r[c]); ok {
						vc = c
					}
					break
				}
			}
		}
	}
	return dc, vc
}

// trackerSeries is a tracker's days and summary.
func trackerSeries(spec trackerSpec, user string, now time.Time) (common.TrackerSeries, error) {
	w := &trackerWalk{spec: spec, user: user, now: now, cache: lineCache{}, kinds: map[string]int{}}
	out := common.TrackerSeries{Line: spec.Line, Title: spec.Title, Type: spec.Type, Source: spec.Kind + ":" + spec.Arg,
		Agg: spec.Agg, Unit: spec.Unit, Color: spec.Color, From: spec.From.Format("2006-01-02"), To: spec.To.Format("2006-01-02"),
		Goal: spec.Goal}
	if spec.Kind == "words" {
		out.Source = "words"
		if out.Unit == "" {
			out.Unit = "words"
		}
	}
	if err := w.run(); err != nil {
		return out, err
	}
	spec = w.spec
	out.Agg = spec.Agg
	vals := map[string][]float64{}
	first := map[string]trackerSample{}
	for _, s := range w.samples {
		if _, ok := first[s.day]; !ok {
			first[s.day] = s
		}
		vals[s.day] = append(vals[s.day], s.v)
	}
	out.Days = common.TrackerDays(spec.From, spec.To, vals, spec.Agg)
	for i := range out.Days {
		if s, ok := first[out.Days[i].Date]; ok {
			out.Days[i].File, out.Days[i].Hash, out.Days[i].Line = s.file, s.hash, s.line
		}
	}
	// The kind the values mostly read as.
	best := 0
	for k, n := range w.kinds {
		if n > best || (n == best && k < out.Kind) {
			out.Kind, best = k, n
		}
	}
	if out.Kind == "" {
		out.Kind = "number"
		if spec.Kind == "clock" {
			out.Kind = "duration"
		} else if spec.zeroFill() {
			out.Kind = "count"
		}
	}
	if spec.Agg == "count" {
		out.Kind = "count"
	}
	out.Summary = common.SummarizeTracker(out.Days, spec.Goal, trackerDay(now).Format("2006-01-02"))
	out.Ok = true
	return out, nil
}

// trackerFor is the series a line describes, and the spec read from it.
func trackerFor(line, user string, now time.Time) (common.TrackerSeries, trackerSpec, error) {
	spec, err := parseTrackerSpec(line, now)
	if err != nil {
		return common.TrackerSeries{Line: strings.TrimSpace(line)}, spec, err
	}
	s, err := trackerSeries(spec, user, now)
	return s, spec, err
}

// ----------------------------------------------------------------------------
// Every #+TRACKER: line
// ----------------------------------------------------------------------------

// trackerLinesIn is the trackers in one file's lines, skipping those inside
// a source or example block (a guide showing one).
func trackerLinesIn(lines []string) []common.TrackerLine {
	out := []common.TrackerLine{}
	var inBlock string
	for i, l := range lines {
		t := strings.ToLower(strings.TrimSpace(l))
		if inBlock != "" {
			if strings.HasPrefix(t, "#+end_"+inBlock) {
				inBlock = ""
			}
			continue
		}
		if strings.HasPrefix(t, "#+begin_") {
			inBlock = strings.Fields(strings.TrimPrefix(t, "#+begin_") + " ")[0]
			continue
		}
		if m := trackerLineRe.FindStringSubmatch(l); m != nil {
			out = append(out, common.TrackerLine{Line: i, Options: strings.TrimSpace(m[2])})
		} else if m := trackerBeginRe.FindStringSubmatch(l); m != nil {
			out = append(out, common.TrackerLine{Line: i, Options: strings.TrimSpace(m[1]), Block: true})
		}
	}
	return out
}

func trackerList() []common.TrackerLine {
	out := []common.TrackerLine{}
	cache := lineCache{}
	for _, name := range GetDb().GetFiles() {
		lines := cache.of(name)
		if !strings.Contains(strings.ToLower(strings.Join(lines, "\n")), "tracker") {
			continue
		}
		f := GetDb().FindByFile(name)
		for _, t := range trackerLinesIn(lines) {
			t.Filename = name
			t.Title = common.TrackerGet(common.ParseTrackerOptions(t.Options), "title", "")
			if t.Title == "" {
				if k, a, ok := common.TrackerSourceOf(common.ParseTrackerOptions(t.Options)); ok {
					t.Title = trackerDefaultTitle(k, a)
				}
			}
			if f != nil {
				if sec := sectionAtRow(f, t.Line); sec != nil && sec.Headline != nil {
					t.Hash, t.Headline = sec.Hash, strings.TrimSpace(headlineTitle(sec))
				}
			}
			out = append(out, t)
		}
	}
	common.SortTrackerLines(out)
	return out
}

// ----------------------------------------------------------------------------
// Logging a value on a day's page
// ----------------------------------------------------------------------------

var trackerPropNameRe = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

// logTracker writes :NAME: value on the day page for day: in a day-a-page
// setup into the page's own property drawer, on a week page into the day's
// Mon/Tue heading (made at the end of the page when it is not there).
func logTracker(prop, val string, day time.Time) (common.TrackerLogResult, error) {
	res := common.TrackerLogResult{Date: day.Format("2006-01-02")}
	if !trackerPropNameRe.MatchString(prop) {
		return res, fmt.Errorf("%q is not a property name (letters, digits, - and _)", prop)
	}
	val = strings.TrimSpace(val)
	if val == "" || strings.ContainsAny(val, "\n\r") {
		return res, fmt.Errorf("give a value to write")
	}
	filename, title := getDayPageFilename(day)
	if !filepath.IsAbs(filename) {
		if abs, err := filepath.Abs(filename); err == nil {
			filename = abs
		}
	}
	if _, err := insideOrgDirs(filename); !insideOrgDirsOk(filename) {
		_ = err
		return res, fmt.Errorf("the day page %s is not under the org directories (set dayPagePath)", filename)
	}
	if _, err := os.Stat(filename); err != nil {
		res.Created = true
		made := false
		if trackerDay(day).Equal(trackerDay(time.Now())) && Conf().Server.DayPageTemplate != "" {
			if _, err := CreateDayPage(); err == nil {
				made = true
			}
		}
		if !made {
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				return res, err
			}
			if err := os.WriteFile(filename, []byte("#+TITLE: "+title+"\n"), 0o644); err != nil {
				return res, err
			}
		}
		GetDb().LoadFile(filename)
	}
	lines, err := readFileLines(filename)
	if err != nil {
		return res, err
	}
	res.Filename = filename
	week := Conf().Server.DayPageMode == "week"
	if !week {
		lines, row := setFileLevelProp(lines, prop, val)
		res.Line = row
		return res, writeTrackerLog(filename, lines)
	}
	// The weekday heading.
	want := day.Weekday()
	for i, l := range lines {
		m := subtreeHeadingRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(l, "* "))
		if wd, ok := weekdayOf(title); (ok && wd == want) || strings.HasPrefix(title, day.Format("2006-01-02")) {
			end := i + 1
			for end < len(lines) && !headingLineRe.MatchString(lines[end]) {
				end++
			}
			ind := indentOf(len(m[1]))
			var ps, pe int
			lines, ps, pe, _ = ensurePropertyDrawer(lines, i, end-1, ind)
			lines, _ = setPropIn(lines, ps, pe, ind, prop, val)
			res.Line = i
			return res, writeTrackerLog(filename, lines)
		}
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	res.Line = len(lines)
	lines = append(lines, "* "+day.Format("Mon"), ":PROPERTIES:", propLine("  ", prop, val, 0), ":END:")
	return res, writeTrackerLog(filename, lines)
}

func insideOrgDirsOk(p string) bool {
	_, ok := insideOrgDirs(p)
	return ok
}

// setFileLevelProp sets a property in the drawer above a file's first
// heading, making the drawer at the top of the file (where org reads one).
func setFileLevelProp(lines []string, key, val string) ([]string, int) {
	first := len(lines)
	for i, l := range lines {
		if headingLineRe.MatchString(l) {
			first = i
			break
		}
	}
	for i := 0; i < first; i++ {
		if strings.EqualFold(strings.TrimSpace(lines[i]), ":PROPERTIES:") {
			for j := i + 1; j < first; j++ {
				if strings.EqualFold(strings.TrimSpace(lines[j]), ":END:") {
					out, _ := setPropIn(lines, i, j, "", key, val)
					return out, i
				}
			}
		}
	}
	return splice(lines, 0, []string{":PROPERTIES:", propLine("  ", key, val, 0), ":END:"}), 0
}

func writeTrackerLog(filename string, lines []string) error {
	if err := writeLines(filename, lines); err != nil {
		return err
	}
	if GetDb().FindByFile(filename) == nil {
		GetDb().LoadFile(filename)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Endpoints
// ----------------------------------------------------------------------------

func registerTrackerRoutes(api *mux.Router) {
	api.HandleFunc("/tracker/series", RequestTrackerSeries).Methods("GET")
	api.HandleFunc("/tracker/list", RequestTrackerList).Methods("GET")
	api.HandleFunc("/tracker/svg", RequestTrackerSvg).Methods("GET")
	api.HandleFunc("/tracker/log", PostTrackerLog).Methods("POST")
	api.HandleFunc("/tracker/insert", PostTrackerInsert).Methods("POST")
}

// trackerLineOf is a request's tracker: line= (a #+TRACKER: line's options),
// else the same words as parameters (source=property:MOOD&type=line).
func trackerLineOf(r *http.Request) string {
	q := r.URL.Query()
	if l := strings.TrimSpace(q.Get("line")); l != "" {
		if m := trackerLineRe.FindStringSubmatch(l); m != nil {
			return m[2]
		}
		return l
	}
	opts := []common.TrackerOption{}
	for _, k := range []string{"source", "type", "agg", "from", "to", "title", "color", "goal", "min", "max", "unit", "date", "file", "datecol", "col",
		"property", "tag", "query", "habit", "clock", "words", "table"} {
		if v := q.Get(k); v != "" {
			opts = append(opts, common.TrackerOption{Key: k, Value: v})
		}
	}
	return common.FormatTrackerOptions(opts)
}

/* SDOC: API
* GET /tracker/series — A Tracker's Days
* GET /tracker/list — Every #+TRACKER: Line
* GET /tracker/svg — A Tracker As A Picture
* POST /tracker/log — Write Today's Value On The Day Page
* POST /tracker/insert — Put A #+TRACKER: Line In A File

	#+BEGIN_EXAMPLE
	GET  /tracker/series?line=source:property:MOOD type:heatmap from:-1y
	     {"Ok": true, "Line": "...", "Title": "MOOD", "Type": "heatmap",
	      "Source": "property:MOOD", "Kind": "number", "Agg": "mean",
	      "From": "2025-11-05", "To": "2026-11-04",
	      "Days": [{"Date": "2026-11-04", "Value": 4, "N": 1, "File": "...", "Hash": "...", "Line": 3}, ...],
	      "Summary": {Days, Total, Mean, Min, Max, Last, LastDate, Current, Longest, LongestEnd}}
	GET  /tracker/list   {"Ok": true, "Trackers": [{Filename, Line, Options, Block, Title, Hash, Headline}]}
	GET  /tracker/svg?line=...&width=720&bare=t     image/svg+xml (bare: no title or summary)
	POST /tracker/log    {"Property": "MOOD", "Value": "4", "Date": "yesterday"}
	     {"Ok": true, "Filename": ".../Wed_2026_11_04.org", "Line": 0, "Date": "2026-11-04", "Created": false}
	POST /tracker/insert {"Filename": ".../life.org", "Options": "source:property:MOOD", "Line": 12}
	     {"Ok": true, "Line": 13}     (Line -1, or past the end: at the end of the file)
	#+END_EXAMPLE

	=line= is what follows =#+TRACKER:= (the keyword itself may be left
	on); the same words may come as parameters instead
	(=source=property:MOOD&type=line=). =Days= has every day from =From= to
	=To=, =N= 0 on a day with nothing; =Value= is the day's values put
	together by =agg=. =Kind= is what the values read as: =number=,
	=duration= (minutes), =bool= (1 or 0), =rating= (stars) or =count=.
	=Current= is the run of days meeting =goal:= (any value above zero
	without one) up to today - a run up to yesterday still counts, today not
	being over. A heading's =File=/=Hash=/=Line= is where the day's first
	value came from. An error is ={"Ok": false, "Msg": ...}= (and for the
	svg, a picture of the message). =/tracker/log= writes =:MOOD: 4= on the
	day page for =Date= (today by default): into its property drawer, or on
	a week page (=dayPageMode: week=) into the day's =* Wed= heading, made
	when there is none. The day page is made when it is not there yet (from
	=dayPageTemplate= for today). Needs =features: tracker:=.
EDOC */
func RequestTrackerSeries(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, trackerFeature) {
		return
	}
	s, _, err := trackerFor(trackerLineOf(r), GetUsername(r), time.Now())
	if err != nil {
		s.Ok, s.Msg = false, err.Error()
	}
	dynJson(w, s)
}

func RequestTrackerList(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, trackerFeature) {
		return
	}
	dynJson(w, common.TrackerList{Ok: true, Trackers: trackerList()})
}

func RequestTrackerSvg(w http.ResponseWriter, r *http.Request) {
	if !featureOn(trackerFeature) {
		w.Header().Set("Content-Type", "image/svg+xml")
		io.WriteString(w, trackerErrorSvg("Trackers are turned off: orgs features tracker on"))
		return
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	s, spec, err := trackerFor(trackerLineOf(r), GetUsername(r), time.Now())
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		io.WriteString(w, trackerErrorSvg(err.Error()))
		return
	}
	if dynTrue(r.URL.Query().Get("bare")) {
		spec.Bare, s.Title = true, ""
	}
	io.WriteString(w, trackerSvg(s, spec, width, time.Now()))
}

func PostTrackerLog(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, trackerFeature) {
		return
	}
	var req common.TrackerLog
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &req); err != nil {
		dynJson(w, common.TrackerLogResult{Msg: "could not read the request: " + err.Error()})
		return
	}
	now := time.Now()
	day := now
	if v := strings.TrimSpace(req.Date); v != "" {
		d, ok, err := orgdate.ParseDate(v, now)
		if err != nil || !ok {
			dynJson(w, common.TrackerLogResult{Msg: fmt.Sprintf("%q is not a date", v)})
			return
		}
		day = d.Day
	}
	res, err := logTracker(strings.TrimSpace(req.Property), req.Value, trackerDay(day))
	if err != nil {
		res.Msg = err.Error()
	} else {
		res.Ok = true
		res.Msg = fmt.Sprintf(":%s: %s on %s", req.Property, strings.TrimSpace(req.Value), filepath.Base(res.Filename))
	}
	dynJson(w, res)
}

type trackerInsertReq struct {
	Filename string
	Options  string
	Line     int // the 0-based line to go after; -1 the end of the file
}

func PostTrackerInsert(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, trackerFeature) {
		return
	}
	var req trackerInsertReq
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &req); err != nil {
		dynJson(w, common.ResultMsg{Msg: "could not read the request: " + err.Error()})
		return
	}
	row, err := insertTrackerLine(req.Filename, req.Options, req.Line, time.Now())
	if err != nil {
		dynJson(w, common.ResultMsg{Msg: err.Error()})
		return
	}
	dynJson(w, map[string]any{"Ok": true, "Line": row, "Msg": "#+TRACKER: line added"})
}

// insertTrackerLine puts a #+TRACKER: line into a file the server reads,
// after line (the end for -1), once its options read as a tracker.
func insertTrackerLine(filename, options string, line int, now time.Time) (int, error) {
	options = strings.TrimSpace(options)
	if m := trackerLineRe.FindStringSubmatch(options); m != nil {
		options = strings.TrimSpace(m[2])
	}
	if strings.ContainsAny(options, "\n\r") {
		return 0, fmt.Errorf("a tracker is one line")
	}
	if _, err := parseTrackerSpec(options, now); err != nil {
		return 0, err
	}
	f := GetDb().FindByFile(filename)
	if f == nil || !insideOrgDirsOk(f.Filename) {
		return 0, fmt.Errorf("no org file called %s", filename)
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		return 0, err
	}
	at := line + 1
	if line < 0 || at > len(lines) {
		at = len(lines)
		for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
			at--
		}
		lines = lines[:at]
		if at > 0 {
			lines = append(lines, "")
			at++
		}
	}
	lines = splice(lines, at, []string{"#+TRACKER: " + options})
	return at, writeLines(f.Filename, lines)
}

// ----------------------------------------------------------------------------
// #+BEGIN: tracker - the days as an org table
// ----------------------------------------------------------------------------

func dynTracker(c *dynCtx) ([]string, error) {
	if !featureOn(trackerFeature) {
		return nil, fmt.Errorf("trackers are turned off: `orgs features tracker on`")
	}
	s, spec, err := trackerFor(c.Block.Header, "", c.Now)
	if err != nil {
		return nil, err
	}
	all := dynTrue(spec.get("empty", "nil"))
	unit := s.Unit
	header := []string{"Date", s.Title}
	if unit != "" {
		header[1] += " (" + unit + ")"
	}
	table := [][]string{header, nil}
	shown := 0
	for i := len(s.Days) - 1; i >= 0; i-- {
		d := s.Days[i]
		if d.N == 0 && !all {
			continue
		}
		v := ""
		if d.N > 0 || spec.zeroFill() {
			v = common.TrackerFormat(d.Value, s.Kind)
		}
		t, _ := time.ParseInLocation("2006-01-02", d.Date, time.Local)
		table = append(table, []string{"[" + t.Format("2006-01-02 Mon") + "]", v})
		shown++
	}
	if limit, err := strconv.Atoi(spec.get("rows", "")); err == nil && limit > 0 && limit < shown {
		table = table[:2+limit]
	}
	sum := s.Summary
	kind := s.Kind
	if kind == "rating" || kind == "bool" {
		kind = "number"
	}
	table = append(table, nil,
		[]string{"Total", common.TrackerFormat(sum.Total, kind)},
		[]string{"Mean", common.TrackerFormat(sum.Mean, kind)},
		[]string{"Days", strconv.Itoa(sum.Days)},
		[]string{"Streak", fmt.Sprintf("%d (longest %d)", sum.Current, sum.Longest)})
	caption := fmt.Sprintf("#+CAPTION: %s, %s to %s, at [%s].", s.Title, s.From, s.To, c.Now.Format("2006-01-02 Mon 15:04"))
	return append([]string{caption}, orgTableLines(table)...), nil
}

// ----------------------------------------------------------------------------
// The export copy: each #+TRACKER: line becomes its chart
// ----------------------------------------------------------------------------

func trackerMayApply(text string) bool {
	return featureOn(trackerFeature) && strings.Contains(strings.ToLower(text), "#+tracker:")
}

// trackerBackendKind is how a backend takes a picture: html, latex, typst or
// text.
func trackerBackendKind(backend string) string {
	b := strings.ToLower(backend)
	switch {
	case b == "latex" || b == "beamer" || b == "pdf" || strings.Contains(b, "latex"):
		return "latex"
	case b == "typst" || strings.HasPrefix(b, "typst"):
		return "typst"
	case b == "txt" || b == "ascii" || b == "text" || b == "markdown" || b == "md" || strings.HasPrefix(b, "pandoc"):
		return "text"
	}
	return "html"
}

// trackerExportLines replaces each #+TRACKER: line with the chart it
// describes, as the backend can take it.
func trackerExportLines(path string, lines []string, backend string) ([]string, bool) {
	kind := trackerBackendKind(backend)
	out := make([]string, 0, len(lines))
	changed, tikz := false, false
	found := trackerLinesIn(lines)
	at := map[int]common.TrackerLine{}
	for _, t := range found {
		if !t.Block {
			at[t.Line] = t
		}
	}
	now := time.Now()
	for i, l := range lines {
		t, ok := at[i]
		if !ok {
			out = append(out, l)
			continue
		}
		ind := trackerLineRe.FindStringSubmatch(l)[1]
		s, spec, err := trackerFor(t.Options, "", now)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tracker: %s:%d: %v\n", filepath.Base(path), i+1, err)
			out = append(out, ind+"/Tracker: "+strings.ReplaceAll(err.Error(), "/", "")+"/")
			changed = true
			continue
		}
		changed = true
		switch kind {
		case "html":
			out = append(out, ind+"#+BEGIN_EXPORT html", `<figure class="org-tracker">`+trackerSvg(s, spec, 0, now)+`</figure>`, ind+"#+END_EXPORT")
		case "latex":
			tikz = true
			out = append(out, ind+"#+BEGIN_EXPORT latex")
			out = append(out, trackerTikz(s, spec, now)...)
			out = append(out, ind+"#+END_EXPORT")
		case "typst":
			// The svg inline (typst 0.13 and on read an image from bytes), so
			// nothing has to be written beside the org file or copied.
			svg := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(trackerSvg(s, spec, 0, now))
			out = append(out, ind+"#+BEGIN_EXPORT typst", `#figure(image(bytes("`+svg+`"), format: "svg", width: 100%))`, ind+"#+END_EXPORT")
		default:
			out = append(out, ind+trackerTextLine(s))
		}
	}
	if tikz {
		out = append([]string{`#+LATEX_HEADER: \usepackage{tikz}`}, out...)
	}
	return out, changed
}

// trackerTextLine is a tracker in a line of text: its title, the last
// month as a sparkline, the summary.
func trackerTextLine(s common.TrackerSeries) string {
	days := s.Days
	if len(days) > 60 {
		days = days[len(days)-60:]
	}
	return fmt.Sprintf("%s: =%s= %s", s.Title, common.Sparkline(days), trackerSummaryText(s))
}

// trackerSummaryText is the summary in words.
func trackerSummaryText(s common.TrackerSeries) string {
	sum := s.Summary
	if sum.Days == 0 {
		return "nothing from " + s.From + " to " + s.To
	}
	kind := s.Kind
	if kind == "bool" || kind == "rating" {
		kind = "number"
	}
	parts := []string{fmt.Sprintf("%d day%s", sum.Days, plural(sum.Days))}
	written := strings.HasPrefix(s.Source, "property:") || strings.HasPrefix(s.Source, "table:")
	switch {
	case !written || s.Agg == "sum" || s.Agg == "count":
		parts = append(parts, "total "+common.TrackerFormat(sum.Total, kind))
	default:
		parts = append(parts, "mean "+common.TrackerFormat(sum.Mean, kind))
	}
	if sum.Current > 0 {
		parts = append(parts, fmt.Sprintf("streak %d", sum.Current))
	}
	if sum.Longest > 1 {
		parts = append(parts, fmt.Sprintf("longest %d", sum.Longest))
	}
	return strings.Join(parts, " · ")
}

package common

// Trackers (features.tracker), in org's own spelling: a #+TRACKER: line whose options are org-plot's
// key:value words, and a #+BEGIN: tracker dynamic block that takes the same
// words. This file is the pure half - the options, reading a value as written
// in a property or a table cell, a day's aggregate, the summary and streaks -
// so the server and the command line read a line the same way.
//
//	#+TRACKER: type:heatmap source:property:MOOD from:-1y
//	#+TRACKER: source:query:"IsDone()" type:bar from:-8w
//	#+TRACKER: source:habit:Run color:green
//	#+TRACKER: source:clock:"Writing" type:line agg:sum

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TrackerDay is one calendar day of a series. N is how many values went into
// it (0: nothing that day); File/Hash/Line are where the first one came from,
// for a client to open.
type TrackerDay struct {
	Date  string
	Value float64
	N     int
	File  string `json:",omitempty"`
	Hash  string `json:",omitempty"`
	Line  int    `json:",omitempty"`
}

// TrackerSummary is a series' numbers over its range.
type TrackerSummary struct {
	Days     int     // days with a value
	Total    float64 // sum of the days' values
	Mean     float64 // mean over the days with a value
	Min      float64
	Max      float64
	Last     float64
	LastDate string
	Current  int // the run of days meeting the goal, ending today (or yesterday, today not being over)
	Longest  int
	// The day the longest run ended.
	LongestEnd string `json:",omitempty"`
}

// TrackerSeries is GET /tracker/series's answer.
type TrackerSeries struct {
	Ok    bool
	Off   bool   `json:",omitempty"`
	Msg   string `json:",omitempty"`
	Line  string // the options, as given
	Title string
	Type  string // heatmap, line, bar, sparkline
	// Source is kind:arg ("property:MOOD"); Kind what the values read as:
	// number, duration (minutes), bool, rating, count.
	Source  string
	Kind    string
	Agg     string
	Unit    string `json:",omitempty"`
	Color   string `json:",omitempty"`
	From    string
	To      string
	Goal    *float64 `json:",omitempty"`
	Days    []TrackerDay
	Summary TrackerSummary
}

// TrackerLine is one #+TRACKER: line (or #+BEGIN: tracker block) in a file.
type TrackerLine struct {
	Filename string
	Line     int    // 0-based row
	Options  string // what follows the keyword
	Block    bool   `json:",omitempty"` // a #+BEGIN: tracker block, not a keyword
	Title    string `json:",omitempty"`
	Hash     string `json:",omitempty"` // the heading it sits under
	Headline string `json:",omitempty"`
}

type TrackerList struct {
	Ok       bool
	Off      bool   `json:",omitempty"`
	Msg      string `json:",omitempty"`
	Trackers []TrackerLine
}

// TrackerLog is POST /tracker/log: a value for a property on a day's page.
type TrackerLog struct {
	Property string
	Value    string
	Date     string `json:",omitempty"` // a typed date; today when empty
}

type TrackerLogResult struct {
	Ok       bool
	Off      bool   `json:",omitempty"`
	Msg      string `json:",omitempty"`
	Filename string `json:",omitempty"`
	Line     int    `json:",omitempty"`
	Date     string `json:",omitempty"`
	Created  bool   `json:",omitempty"` // the day page was made for it
}

// TrackerSources are the kinds of thing a tracker can follow.
var TrackerSources = []string{"property", "tag", "query", "habit", "clock", "words", "table"}

// TrackerAggs are how a day's values become one.
var TrackerAggs = []string{"sum", "mean", "max", "min", "last", "count"}

// TrackerTypes are the charts.
var TrackerTypes = []string{"heatmap", "line", "bar", "sparkline"}

// TrackerOption is one key:value word, in the order written.
type TrackerOption struct {
	Key, Value string
}

// ParseTrackerOptions reads a #+TRACKER: line's words: key:value, the value a
// word, "quoted" (wholly or after a prefix, source:query:"IsDone()"), or a
// (list). A dynamic block's org spelling, :key value, is read too. Keys are
// lower case; a key given twice keeps both, the last one winning in Get.
func ParseTrackerOptions(s string) []TrackerOption {
	out := []TrackerOption{}
	rs := []rune(s)
	i := 0
	// word reads to the next unquoted, unbracketed space, dropping the quotes.
	word := func() string {
		var b strings.Builder
		quoted, depth := false, 0
		for i < len(rs) {
			c := rs[i]
			switch {
			case quoted && c == '\\' && i+1 < len(rs) && rs[i+1] == '"':
				b.WriteRune('"')
				i += 2
				continue
			case c == '"':
				quoted = !quoted
				i++
				continue
			case !quoted && c == '(':
				depth++
			case !quoted && c == ')' && depth > 0:
				depth--
			case !quoted && depth == 0 && (c == ' ' || c == '\t'):
				return b.String()
			}
			b.WriteRune(c)
			i++
		}
		return b.String()
	}
	skip := func() {
		for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
			i++
		}
	}
	for {
		skip()
		if i >= len(rs) {
			break
		}
		if rs[i] == ':' {
			// :key value
			i++
			key := strings.ToLower(word())
			skip()
			if i >= len(rs) || rs[i] == ':' {
				out = append(out, TrackerOption{key, "t"})
				continue
			}
			out = append(out, TrackerOption{key, word()})
			continue
		}
		start := i
		// The key is up to the first colon; a word with none is skipped.
		for i < len(rs) && rs[i] != ':' && rs[i] != ' ' && rs[i] != '\t' && rs[i] != '"' {
			i++
		}
		if i >= len(rs) || rs[i] != ':' || i == start {
			i = start
			word()
			continue
		}
		key := strings.ToLower(string(rs[start:i]))
		i++
		out = append(out, TrackerOption{key, word()})
	}
	return out
}

// TrackerGet is the last value given for key, or def.
func TrackerGet(opts []TrackerOption, key, def string) string {
	for i := len(opts) - 1; i >= 0; i-- {
		if opts[i].Key == key {
			return opts[i].Value
		}
	}
	return def
}

var trackerPlainRe = regexp.MustCompile(`^[A-Za-z0-9_.+\-/@#%*]+$`)

// trackerQuote is a value as a #+TRACKER: line writes it: bare when it is one
// plain word, "quoted" otherwise.
func trackerQuote(v string) string {
	if v != "" && trackerPlainRe.MatchString(v) {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}

// FormatTrackerOptions writes options back as a #+TRACKER: line's words. A
// source keeps its kind bare and quotes only what follows (source:query:"IsDone()").
func FormatTrackerOptions(opts []TrackerOption) string {
	parts := []string{}
	for _, o := range opts {
		if o.Key == "" {
			continue
		}
		v := o.Value
		if o.Key == "source" {
			if k, a, ok := strings.Cut(v, ":"); ok {
				parts = append(parts, "source:"+k+":"+trackerQuote(a))
				continue
			}
		}
		parts = append(parts, o.Key+":"+trackerQuote(v))
	}
	return strings.Join(parts, " ")
}

// TrackerSourceOf is a line's source as kind and argument: source:property:MOOD,
// or the kind given as its own key (property:MOOD, habit:Run, words:t).
func TrackerSourceOf(opts []TrackerOption) (kind, arg string, ok bool) {
	if v := TrackerGet(opts, "source", ""); v != "" {
		k, a, _ := strings.Cut(v, ":")
		k = strings.ToLower(strings.TrimSpace(k))
		for _, s := range TrackerSources {
			if s == k {
				return k, strings.TrimSpace(a), true
			}
		}
		if k == "prop" {
			return "property", strings.TrimSpace(a), true
		}
		return k, a, false
	}
	for i := len(opts) - 1; i >= 0; i-- {
		for _, s := range TrackerSources {
			if opts[i].Key == s || (opts[i].Key == "prop" && s == "property") {
				return s, opts[i].Value, true
			}
		}
	}
	return "", "", false
}

// ---------------------------------------------------------------------------
// Values
// ---------------------------------------------------------------------------

var (
	trackerHMRe   = regexp.MustCompile(`^(\d+):([0-5]\d)(?::([0-5]\d))?$`)
	trackerSpanRe = regexp.MustCompile(`^(?:(\d+(?:\.\d+)?)\s*h)?\s*(?:(\d+(?:\.\d+)?)\s*m(?:in)?)?$`)
	trackerNumRe  = regexp.MustCompile(`^([-+]?\d[\d,]*(?:\.\d+)?|[-+]?\.\d+)\s*([A-Za-z%°]*)$`)
	trackerFracRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*/\s*(\d+(?:\.\d+)?)$`)
)

// TrackerValue reads a value as somebody writes one in a property or a cell:
//
//	4, 72.5kg, 8,000, -3      number (a unit after it is dropped)
//	1:30, 1h30m, 45m, 2h      duration, in minutes
//	t, yes, y, true, on, done, [X]; nil, no, n, false, off, [ ]   bool, 1 or 0
//	★★★☆☆, ⭐⭐, 3/5             rating (the stars filled; a fraction's top)
//
// ok is false for anything else, and for an empty value.
func TrackerValue(s string) (v float64, kind string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", false
	}
	low := strings.ToLower(s)
	switch low {
	case "t", "yes", "y", "true", "on", "done", "[x]", "x", "✓", "✔":
		return 1, "bool", true
	case "nil", "no", "n", "false", "off", "[ ]", "[]", "✗", "✘":
		return 0, "bool", true
	case "[-]":
		return 0.5, "bool", true
	}
	stars, other := 0, 0
	for _, r := range s {
		switch r {
		case '★', '⭐':
			stars++
		case '☆', ' ', '️':
		default:
			other++
		}
	}
	if other == 0 {
		return float64(stars), "rating", true
	}
	if m := trackerFracRe.FindStringSubmatch(s); m != nil {
		a, _ := strconv.ParseFloat(m[1], 64)
		return a, "rating", true
	}
	if m := trackerHMRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		sec := 0
		if m[3] != "" {
			sec, _ = strconv.Atoi(m[3])
		}
		return float64(h*60+mm) + float64(sec)/60, "duration", true
	}
	if m := trackerNumRe.FindStringSubmatch(s); m != nil {
		unit := strings.ToLower(m[2])
		n, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err != nil {
			return 0, "", false
		}
		switch unit {
		case "h", "hr", "hrs", "hour", "hours":
			return n * 60, "duration", true
		case "m", "min", "mins", "minute", "minutes":
			return n, "duration", true
		}
		return n, "number", true
	}
	if m := trackerSpanRe.FindStringSubmatch(low); m != nil && (m[1] != "" || m[2] != "") {
		mins := 0.0
		if m[1] != "" {
			h, _ := strconv.ParseFloat(m[1], 64)
			mins += h * 60
		}
		if m[2] != "" {
			mm, _ := strconv.ParseFloat(m[2], 64)
			mins += mm
		}
		return mins, "duration", true
	}
	return 0, "", false
}

// TrackerAggregate is a day's values as one: sum, mean, max, min, last
// (the last given) or count.
func TrackerAggregate(vals []float64, agg string) float64 {
	if len(vals) == 0 {
		return 0
	}
	switch agg {
	case "count":
		return float64(len(vals))
	case "mean", "avg", "average":
		t := 0.0
		for _, v := range vals {
			t += v
		}
		return t / float64(len(vals))
	case "max":
		m := vals[0]
		for _, v := range vals {
			m = math.Max(m, v)
		}
		return m
	case "min":
		m := vals[0]
		for _, v := range vals {
			m = math.Min(m, v)
		}
		return m
	case "last":
		return vals[len(vals)-1]
	}
	t := 0.0
	for _, v := range vals {
		t += v
	}
	return t
}

// TrackerHit is whether a day counts toward a streak: it has a value, and
// that value is at least the goal (above zero when there is no goal).
func TrackerHit(d TrackerDay, goal *float64) bool {
	if d.N == 0 {
		return false
	}
	if goal != nil {
		return d.Value >= *goal
	}
	return d.Value > 0
}

// SummarizeTracker is the numbers over a run of days (oldest first, one a
// day). today is the day a run may still be going on: a run that ends the day
// before is still current, since today is not over.
func SummarizeTracker(days []TrackerDay, goal *float64, today string) TrackerSummary {
	s := TrackerSummary{}
	first := true
	run := 0
	for _, d := range days {
		if d.N > 0 {
			s.Days++
			s.Total += d.Value
			if first {
				s.Min, s.Max, first = d.Value, d.Value, false
			}
			s.Min = math.Min(s.Min, d.Value)
			s.Max = math.Max(s.Max, d.Value)
			s.Last, s.LastDate = d.Value, d.Date
		}
		if TrackerHit(d, goal) {
			run++
			if run > s.Longest {
				s.Longest, s.LongestEnd = run, d.Date
			}
		} else if d.Date != today {
			run = 0
		}
	}
	if s.Days > 0 {
		s.Mean = s.Total / float64(s.Days)
	}
	// The current run counts back from today, or from yesterday when today
	// has nothing yet.
	i := len(days) - 1
	for i >= 0 && days[i].Date > today {
		i--
	}
	if i >= 0 && days[i].Date == today && !TrackerHit(days[i], goal) {
		i--
	}
	for ; i >= 0 && TrackerHit(days[i], goal); i-- {
		s.Current++
	}
	return s
}

// TrackerDays is every day from..to (inclusive) with the values that fell on
// it aggregated, oldest first.
func TrackerDays(from, to time.Time, vals map[string][]float64, agg string) []TrackerDay {
	out := []TrackerDay{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		day := TrackerDay{Date: k}
		if v := vals[k]; len(v) > 0 {
			day.N = len(v)
			day.Value = TrackerAggregate(v, agg)
		}
		out = append(out, day)
	}
	return out
}

// TrackerLevel is a day's step on a heatmap: 0 nothing, 1-4 from lo to hi.
func TrackerLevel(v float64, n int, lo, hi float64) int {
	if n == 0 || (v <= 0 && lo >= 0) {
		return 0
	}
	if hi <= lo {
		return 4
	}
	t := (v - lo) / (hi - lo)
	if t <= 0 {
		return 1
	}
	if t >= 1 {
		return 4
	}
	return 1 + int(math.Round(t*3))
}

// TrackerFormat is a value as its kind is written: H:MM for durations, stars
// for ratings, yes/no for bools, else the number trimmed.
func TrackerFormat(v float64, kind string) string {
	switch kind {
	case "duration":
		m := int(math.Round(v))
		sign := ""
		if m < 0 {
			sign, m = "-", -m
		}
		return sign + strconv.Itoa(m/60) + ":" + leftPad2(m%60)
	case "rating":
		if v == math.Trunc(v) && v >= 0 && v <= 10 {
			return strings.Repeat("★", int(v))
		}
	case "bool":
		if v == 1 {
			return "yes"
		}
		if v == 0 {
			return "no"
		}
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func leftPad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// Sparkline is the values as block glyphs, ▁ to █; a day with nothing is a
// space.
func Sparkline(days []TrackerDay) string {
	marks := []rune("▁▂▃▄▅▆▇█")
	lo, hi, first := 0.0, 0.0, true
	for _, d := range days {
		if d.N == 0 {
			continue
		}
		if first {
			lo, hi, first = d.Value, d.Value, false
		}
		lo, hi = math.Min(lo, d.Value), math.Max(hi, d.Value)
	}
	if lo > 0 {
		lo = 0
	}
	var b strings.Builder
	for _, d := range days {
		if d.N == 0 {
			b.WriteRune(' ')
			continue
		}
		i := len(marks) - 1
		if hi > lo {
			i = int((d.Value - lo) / (hi - lo) * float64(len(marks)-1))
		}
		b.WriteRune(marks[max(0, min(i, len(marks)-1))])
	}
	return b.String()
}

// SortTrackerLines orders lines by file, then row.
func SortTrackerLines(ls []TrackerLine) {
	sort.SliceStable(ls, func(a, b int) bool {
		if ls[a].Filename != ls[b].Filename {
			return ls[a].Filename < ls[b].Filename
		}
		return ls[a].Line < ls[b].Line
	})
}

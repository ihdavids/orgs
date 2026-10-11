package orgs

// The agenda engine: what lands on each day. GET /agenda and GET
// /agenda/nodes; the wire types and the rules are in internal/common/agenda.go.
//
// The engine is split so the rules can be tested on a parsed file without a
// server: agendaOccurrences works on one headline and a set of day numbers,
// agendaNodeOf on one section. The handlers only find the headings (through
// the same query path as /search) and assemble the answer.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// Days
// ---------------------------------------------------------------------------

// A day is a number: days since 1970-01-01, counted on the calendar rather
// than in instants, so a daylight saving change never makes a day 23 hours
// long and a date never slips across midnight between time zones.
func agendaDayNum(t time.Time) int {
	y, m, d := t.Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

func agendaDayYMD(y int, m time.Month, d int) int {
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

func agendaDayTime(n int) time.Time {
	return time.Unix(int64(n)*86400, 0).UTC()
}

func agendaDayStr(n int) string {
	return agendaDayTime(n).Format("2006-01-02")
}

// Reads YYYY-MM-DD as a day number.
func agendaParseDay(s string) (int, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("not a date (want YYYY-MM-DD): %q", s)
	}
	return agendaDayNum(t), nil
}

func agendaDaysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// n months on from a date, kept inside the month it lands in: Jan 31 +1m is
// Feb 28 (29 in a leap year), and +2m from Jan 31 is Mar 31 - each repeat is
// counted from the base date, so a short month does not pull every later
// repeat back. Feb 29 +1y is Feb 28 in a year with no 29th.
func agendaAddMonths(y int, m time.Month, d int, months int) int {
	total := int(m) - 1 + months
	ny := y + total/12
	mi := total % 12
	if mi < 0 {
		mi += 12
		ny--
	}
	nm := time.Month(mi + 1)
	if last := agendaDaysIn(ny, nm); d > last {
		d = last
	}
	return agendaDayYMD(ny, nm, d)
}

// The rules a heading is laid out by.
type agendaRules struct {
	From, To int // the days asked about, inclusive
	Today    int
	Closed   bool // CLOSED lands on its day too (the logbook view)
	// Headings in an archived tree (tagged ARCHIVE, here or above) are left
	// out, as org's agenda does (org-agenda-skip-archived-trees), unless
	// asked for: an overdue item filed away years ago is not today's problem.
	Archived bool
	WarnDays int // a deadline's warning period when the stamp names none
}

// One heading landing on one day, before it knows its index into Nodes.
type agendaOcc struct {
	Day   int
	Entry common.AgendaEntry
}

// agenda.deadlineWarningDays, else org's 14. Zero means a deadline warns on
// its own day only.
func agendaWarnDays() int {
	if c := Conf(); c != nil && c.Server != nil && c.Server.Agenda.DeadlineWarningDays != nil {
		if n := *c.Server.Agenda.DeadlineWarningDays; n >= 0 {
			return n
		}
		return 0
	}
	return 14
}

// The warning period a stamp names (-3d, -1w), in days, or -1 for none.
func agendaStampWarn(d *org.OrgDate) int {
	if d == nil || d.WarnDWMY == "" || (d.WarnRule == nil && d.WarnNum == "") {
		return -1
	}
	// The number as written: a rule cannot hold the 0 of -0d (no warning
	// before the deadline's own day).
	n := 1
	if d.WarnNum != "" {
		if v, err := strconv.Atoi(d.WarnNum); err == nil && v >= 0 {
			n = v
		}
	} else if d.WarnRule.Options.Interval > 0 {
		n = d.WarnRule.Options.Interval
	}
	switch d.WarnDWMY {
	case "d":
		return n
	case "w":
		return 7 * n
	case "m":
		return 30 * n
	case "y":
		return 365 * n
	}
	return -1
}

// The start day of every repeat of a stamp whose span (start day to end
// day) can touch [from, to]: the base day and every interval after it, never
// before it. k is 0 for the base day.
func agendaRepeats(d *org.OrgDate, span, from, to int, each func(k, start int)) {
	base := agendaDayNum(d.Start)
	if d.RepeatRule == nil || d.RepeatDWMY == "" {
		if base <= to && base+span >= from {
			each(0, base)
		}
		return
	}
	n := d.RepeatRule.Options.Interval
	if n <= 0 {
		n = 1
	}
	const most = 20000 // a guard, not a limit anyone reaches: 20000 repeats
	switch d.RepeatDWMY {
	case "h", "d", "w":
		// An hourly repeater comes round every day from its base: the agenda
		// places days, and the stamp's time stands for the first of them.
		step := n
		switch d.RepeatDWMY {
		case "w":
			step = 7 * n
		case "h":
			step = 1
		}
		k := 0
		if gap := from - span - base; gap > 0 {
			k = gap / step
		}
		for i := 0; i < most; i, k = i+1, k+1 {
			s := base + k*step
			if s > to {
				return
			}
			if s+span >= from {
				each(k, s)
			}
		}
	case "m", "y":
		months := n
		if d.RepeatDWMY == "y" {
			months = 12 * n
		}
		y, m, dd := d.Start.Date()
		k := 0
		if gap := from - span - base; gap > 0 {
			// Months are at least 28 days, so this never overshoots.
			k = gap / (31 * months)
		}
		for i := 0; i < most; i, k = i+1, k+1 {
			s := agendaAddMonths(y, m, dd, k*months)
			if s > to {
				return
			}
			if s+span >= from {
				each(k, s)
			}
		}
	default:
		// A unit this does not know still names its base day.
		if base <= to && base+span >= from {
			each(0, base)
		}
	}
}

// The entries a stamp gives each day of one occurrence starting on `start`:
// a point or a time span within a day lands on that day; a range across days
// lands on every day it spans, with the times written there on its first and
// last day and as all day on the days between.
func agendaSpread(d *org.OrgDate, kind string, k, start int, r agendaRules, out []agendaOcc) []agendaOcc {
	span := 0
	if d.HasEnd() {
		span = agendaDayNum(d.End) - agendaDayNum(d.Start)
		if span < 0 {
			span = 0
		}
	}
	clock := func(day int, t time.Time) string {
		return agendaDayStr(day) + "T" + t.Format("15:04")
	}
	for day := start; day <= start+span; day++ {
		if day < r.From || day > r.To {
			continue
		}
		e := common.AgendaEntry{Kind: kind, Start: agendaDayStr(day), Repeat: k > 0, DueIn: -1}
		if d.HaveTime {
			switch {
			case span == 0:
				e.HasTime = true
				e.Start = clock(day, d.Start)
				if d.HasEnd() {
					e.End = clock(day, d.End)
				}
			case day == start:
				e.HasTime = true
				e.Start = clock(day, d.Start)
				// Clipped to the day, as the last day is from midnight: a
				// client drawing hours needs to know it runs on past them.
				e.End = agendaDayStr(day) + "T23:59"
				if !d.FirstEnd.IsZero() {
					e.End = clock(day, d.FirstEnd)
				}
			case day == start+span:
				e.HasTime = true
				e.Start = agendaDayStr(day) + "T00:00"
				if !d.LastStart.IsZero() {
					e.Start = clock(day, d.LastStart)
				}
				e.End = clock(day, d.End)
			}
		}
		out = append(out, agendaOcc{Day: day, Entry: e})
	}
	return out
}

func agendaStampSpan(d *org.OrgDate) int {
	if d == nil || !d.HasEnd() {
		return 0
	}
	if s := agendaDayNum(d.End) - agendaDayNum(d.Start); s > 0 {
		return s
	}
	return 0
}

// Every day a heading lands on in [r.From, r.To], following org's agenda (the
// rules are written out in internal/common/agenda.go). done is whether the
// heading's keyword is a finished one of its own file's: a finished heading
// lands only on the days its dates name, with no carrying forward and no
// warnings.
func agendaOccurrences(h *org.Headline, done bool, r agendaRules) []agendaOcc {
	var out []agendaOcc
	if h == nil || r.To < r.From {
		return out
	}
	todayIn := r.Today >= r.From && r.Today <= r.To
	plain := func(d *org.OrgDate, kind string) {
		if d == nil || d.Start.IsZero() {
			return
		}
		// One entry a day per stamp: a repeating range longer than its
		// repeat overlaps itself, and the earliest occurrence speaks for the day.
		seen := map[int]bool{}
		agendaRepeats(d, agendaStampSpan(d), r.From, r.To, func(k, start int) {
			before := len(out)
			out = agendaSpread(d, kind, k, start, r, out)
			kept := out[:before]
			for _, o := range out[before:] {
				if !seen[o.Day] {
					seen[o.Day] = true
					kept = append(kept, o)
				}
			}
			out = kept
		})
	}

	// Plain active timestamps: their days, nothing more.
	for _, ts := range h.Timestamps {
		if ts != nil && ts.Time != nil && ts.Time.TimestampType == org.Active {
			plain(ts.Time, common.AgendaTimestamp)
		}
	}

	// SCHEDULED: its day and each repeat; not done and past, today as well.
	if h.Scheduled != nil && h.Scheduled.Date != nil && !h.Scheduled.Date.Start.IsZero() {
		d := h.Scheduled.Date
		base := agendaDayNum(d.Start)
		late := !done && base < r.Today
		mark := len(out)
		plain(d, common.AgendaScheduled)
		if late && todayIn {
			// The carried-forward entry is today's; a repeat that also falls
			// today would only say the same thing twice.
			out = agendaDrop(out, mark, r.Today)
			out = append(out, agendaOcc{Day: r.Today, Entry: common.AgendaEntry{
				Kind: common.AgendaScheduled, Start: agendaDayStr(r.Today),
				Late: r.Today - base, DueIn: -1}})
		}
	}

	// DEADLINE: its day and each repeat; not done, today as well while today
	// is inside the warning period or past the day.
	if h.Deadline != nil && h.Deadline.Date != nil && !h.Deadline.Date.Start.IsZero() {
		d := h.Deadline.Date
		base := agendaDayNum(d.Start)
		warn := agendaStampWarn(d)
		if warn < 0 {
			warn = r.WarnDays
		}
		mark := len(out)
		plain(d, common.AgendaDeadline)
		if !done {
			left := base - r.Today
			// The deadline's own day says how long is left, while it warns.
			if left >= 0 && left <= warn {
				for i := mark; i < len(out); i++ {
					if out[i].Day == base {
						out[i].Entry.DueIn = left
					}
				}
			}
			if todayIn && left != 0 && (left < 0 || left <= warn) {
				out = agendaDrop(out, mark, r.Today)
				e := common.AgendaEntry{Kind: common.AgendaDeadline, Start: agendaDayStr(r.Today), DueIn: -1}
				if left < 0 {
					e.Late = -left
				} else {
					e.DueIn = left
				}
				out = append(out, agendaOcc{Day: r.Today, Entry: e})
			}
		}
	}

	// CLOSED: its day, when the request asks.
	if r.Closed && h.Closed != nil && h.Closed.Date != nil && !h.Closed.Date.Start.IsZero() {
		d := h.Closed.Date
		day := agendaDayNum(d.Start)
		if day >= r.From && day <= r.To {
			e := common.AgendaEntry{Kind: common.AgendaClosed, Start: agendaDayStr(day), DueIn: -1}
			if d.HaveTime {
				e.HasTime = true
				e.Start = agendaDayStr(day) + "T" + d.Start.Format("15:04")
			}
			out = append(out, agendaOcc{Day: day, Entry: e})
		}
	}

	for i := range out {
		out[i].Entry.Done = done
	}
	return out
}

// Drops the entries from mark on that fall on day.
func agendaDrop(out []agendaOcc, mark, day int) []agendaOcc {
	keep := out[:mark]
	for _, o := range out[mark:] {
		if o.Day != day {
			keep = append(keep, o)
		}
	}
	return keep
}

// ---------------------------------------------------------------------------
// Sorting within a day
// ---------------------------------------------------------------------------

var agendaKindOrder = map[string]int{
	common.AgendaDeadline:  0,
	common.AgendaScheduled: 1,
	common.AgendaTimestamp: 2,
	common.AgendaClosed:    3,
}

// No priority cookie counts as org's default, B.
func agendaPriority(p string) string {
	if p == "" {
		return "B"
	}
	return p
}

// Timed entries by time, then all day ones by kind (deadline, scheduled,
// timestamp), then priority, then title; file and line keep it stable.
func agendaSortDay(entries []common.AgendaEntry, nodes []common.AgendaNode) {
	sort.SliceStable(entries, func(a, b int) bool {
		ea, eb := entries[a], entries[b]
		if ea.HasTime != eb.HasTime {
			return ea.HasTime
		}
		if ea.HasTime && ea.Start != eb.Start {
			return ea.Start < eb.Start
		}
		if ka, kb := agendaKindOrder[ea.Kind], agendaKindOrder[eb.Kind]; ka != kb {
			return ka < kb
		}
		na, nb := nodes[ea.Node], nodes[eb.Node]
		if pa, pb := agendaPriority(na.Priority), agendaPriority(nb.Priority); pa != pb {
			return pa < pb
		}
		if na.Headline != nb.Headline {
			return na.Headline < nb.Headline
		}
		if na.Filename != nb.Filename {
			return na.Filename < nb.Filename
		}
		return na.LineNum < nb.LineNum
	})
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

func agendaDateOf(d *org.OrgDate) *common.AgendaDate {
	if d == nil || d.Start.IsZero() {
		return nil
	}
	layout := "2006-01-02"
	if d.HaveTime {
		layout = "2006-01-02T15:04"
	}
	out := &common.AgendaDate{
		Start:   d.Start.Format(layout),
		HasTime: d.HaveTime,
		Active:  d.TimestampType == org.Active,
		Repeat:  d.RepeatString(),
		Warn:    d.WarnString(),
		Text:    d.ToString(),
	}
	if d.HasEnd() {
		out.End = d.End.Format(layout)
	}
	return out
}

func agendaSdcDate(s *org.SDC) *common.AgendaDate {
	if s == nil {
		return nil
	}
	return agendaDateOf(s.Date)
}

// A file's keywords, active and finished, read once per answer.
type agendaKeys struct {
	cache map[*common.OrgFile][2][]string
}

func (self *agendaKeys) of(f *common.OrgFile) ([]string, []string) {
	if self.cache == nil {
		self.cache = map[*common.OrgFile][2][]string{}
	}
	if k, ok := self.cache[f]; ok {
		return k[0], k[1]
	}
	active, done := ValidStatusFromFile(f)
	if active == nil {
		active = []string{}
	}
	if done == nil {
		done = []string{}
	}
	self.cache[f] = [2][]string{active, done}
	return active, done
}

// Is the heading on a finished keyword of its own file's?
func (self *agendaKeys) done(sec *org.Section, f *common.OrgFile) bool {
	if sec == nil || sec.Headline == nil || sec.Headline.Status == "" {
		return false
	}
	_, done := self.of(f)
	return contains(done, sec.Headline.Status)
}

// The heading's tags and every tag above it, #+FILETAGS included: what
// HasTags matches against (HasTag walks the same way).
func agendaAllTags(sec *org.Section, doc *org.Document) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, t := range sec.Headline.Tags {
		add(t)
	}
	if doc != nil {
		for _, t := range GetParentTags(sec, doc) {
			add(t)
		}
	}
	return out
}

// A heading with everything the agenda reads about it.
func agendaNodeOf(sec *org.Section, f *common.OrgFile, keys *agendaKeys, now time.Time) common.AgendaNode {
	h := sec.Headline
	n := common.AgendaNode{}
	if t := SectionToTodo(sec, f); t != nil {
		n.Todo = *t
	}
	n.Olp = outlinePath(sec)
	n.Scheduled = agendaSdcDate(h.Scheduled)
	n.Deadline = agendaSdcDate(h.Deadline)
	n.Closed = agendaSdcDate(h.Closed)
	n.Timestamps = []common.AgendaDate{}
	for _, ts := range h.Timestamps {
		if ts == nil {
			continue
		}
		if d := agendaDateOf(ts.Time); d != nil {
			n.Timestamps = append(n.Timestamps, *d)
		}
	}
	n.Clocks = []common.AgendaClock{}
	for _, c := range clockSpans(h) {
		n.Clocks = append(n.Clocks, common.AgendaClock{Start: c.Start, End: c.End, Mins: c.Mins})
		n.ClockMins += c.Mins
	}
	active, done := keys.of(f)
	n.TodoKeys, n.DoneKeys = active, done
	n.IsTodo = h.Status != "" && contains(active, h.Status)
	n.IsDone = h.Status != "" && contains(done, h.Status)
	var doc *org.Document
	if f != nil {
		doc = f.Doc
	}
	if doc != nil {
		n.IsArchived = IsArchived(sec, doc)
	}
	n.IsHabit = IsHabitSection(sec)
	n.AllTags = agendaAllTags(sec, doc)
	n.Children.States = []string{}
	for _, c := range sec.Children {
		if c == nil || c.Headline == nil {
			continue
		}
		st := c.Headline.Status
		n.Children.Count++
		n.Children.States = append(n.Children.States, st)
		switch {
		case st != "" && contains(done, st):
			n.Children.Done++
		case st != "" && contains(active, st):
			n.Children.Todo++
		}
	}
	if n.IsHabit {
		completed := habitCompletionDays(sec) // newest first
		n.DoneDates = make([]string, 0, len(completed))
		for i := len(completed) - 1; i >= 0; i-- {
			n.DoneDates = append(n.DoneDates, completed[i].Format("2006-01-02"))
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Finding the headings
// ---------------------------------------------------------------------------

// Every heading a query finds, through /search's own path (files the query
// cannot match skipped, sections registered so their hashes answer /hash and
// the edit endpoints). The query's filters are already expanded, because the
// expanded query is what the answer is cached under. An empty query is every
// heading.
func agendaSections(query string, visit func(*org.Section, *common.OrgFile)) error {
	if strings.TrimSpace(query) == "" {
		registerAllSections()
		for _, name := range GetDb().GetFiles() {
			f := GetDb().GetFile(name)
			if f == nil || f.Doc == nil {
				continue
			}
			for _, sec := range flattenSections(f) {
				if sec.Headline != nil {
					visit(sec, f)
				}
			}
		}
		return nil
	}
	return evalEachSection(query, visit)
}

// ---------------------------------------------------------------------------
// The answers
// ---------------------------------------------------------------------------

// The longest span one request may ask for. A client paging a year view asks
// for a year; nobody needs thirty.
const agendaMaxDays = 1000

func buildAgenda(query string, r agendaRules, now time.Time) (common.AgendaResult, error) {
	res := common.AgendaResult{
		Ok: true, From: agendaDayStr(r.From), To: agendaDayStr(r.To), Today: agendaDayStr(r.Today),
		Nodes: []common.AgendaNode{}, Days: []common.AgendaDay{},
	}
	if r.To < r.From {
		return res, fmt.Errorf("to (%s) is before from (%s)", res.To, res.From)
	}
	if r.To-r.From >= agendaMaxDays {
		return res, fmt.Errorf("%d days asked for; at most %d at a time", r.To-r.From+1, agendaMaxDays)
	}
	for d := r.From; d <= r.To; d++ {
		res.Days = append(res.Days, common.AgendaDay{Date: agendaDayStr(d), Entries: []common.AgendaEntry{}})
	}
	keys := &agendaKeys{}
	err := agendaSections(query, func(sec *org.Section, f *common.OrgFile) {
		if !r.Archived && f != nil && f.Doc != nil && IsArchived(sec, f.Doc) {
			return
		}
		if agendaCommented(sec) {
			return
		}
		occs := agendaOccurrences(sec.Headline, keys.done(sec, f), r)
		if len(occs) == 0 {
			return
		}
		idx := len(res.Nodes)
		res.Nodes = append(res.Nodes, agendaNodeOf(sec, f, keys, now))
		for _, o := range occs {
			o.Entry.Node = idx
			day := &res.Days[o.Day-r.From]
			day.Entries = append(day.Entries, o.Entry)
		}
	})
	if err != nil {
		return res, err
	}
	for i := range res.Days {
		agendaSortDay(res.Days[i].Entries, res.Nodes)
	}
	return res, nil
}

func buildAgendaNodes(query string, now time.Time) (common.AgendaNodes, error) {
	res := common.AgendaNodes{Ok: true, Nodes: []common.AgendaNode{}}
	keys := &agendaKeys{}
	err := agendaSections(query, func(sec *org.Section, f *common.OrgFile) {
		res.Nodes = append(res.Nodes, agendaNodeOf(sec, f, keys, now))
	})
	return res, err
}

// Answers are kept against the database's version (see Traps: caches), the
// expanded query and every parameter - today among them, since it changes
// the answer - and the minute, since a running clock's minutes move.
type agendaCacheT struct {
	sync.Mutex
	reload uint64
	items  map[string]interface{}
}

var agendaCache agendaCacheT

func (self *agendaCacheT) get(key string, build func() (interface{}, error)) (interface{}, error) {
	reload := GetDb().ReloadIndex
	self.Lock()
	if self.items == nil || self.reload != reload {
		self.items = map[string]interface{}{}
		self.reload = reload
	}
	if v, ok := self.items[key]; ok {
		self.Unlock()
		return v, nil
	}
	self.Unlock()
	v, err := build()
	if err != nil {
		return v, err
	}
	self.Lock()
	if self.reload == reload {
		if len(self.items) > 64 {
			self.items = map[string]interface{}{}
		}
		self.items[key] = v
	}
	self.Unlock()
	return v, nil
}

func agendaAnswer(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

/* SDOC: API
* GET /agenda — What Lands on Each Day

	The agenda worked out on the server: every heading that lands on each day
	of a range, with repeaters expanded, ranges spread over the days they span,
	overdue SCHEDULED items carried forward to today, and DEADLINE warnings.
	A client only draws it. The rules follow org's agenda:

	- A plain active timestamp (in the headline or the heading's own body, not
	  a child's, not in a drawer) lands on its day. A repeater (=+1w=, =++1w=,
	  =.+1w=) lands on its base day and every interval after it; months and
	  years keep to the calendar (Jan 31 =+1m= is Feb 28).
	- A range =<a>--<b>= lands on every day it spans, with the times written
	  there on its first and last day and as all day between.
	- SCHEDULED lands on its day; not done and past, it also lands on *today*
	  only, with =Late= days.
	- DEADLINE lands on its day; not done, it also lands on today inside its
	  warning period (=-Nd= on the stamp, else =agenda.deadlineWarningDays=,
	  default 14) with =DueIn= days, or past it with =Late= days.
	- A finished heading lands only on the days its dates name.
	- CLOSED lands on its day when =closed=t=.
	- A heading in an archived tree (tag ARCHIVE, here or above) is left out
	  unless =archived=t=, as org's agenda skips archived trees.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                            |
	|-----------+--------+----------+--------------------------------------------------------|
	| =from=    | date   | no       | First day, =YYYY-MM-DD=. Defaults to today.             |
	| =to=      | date   | no       | Last day, inclusive. Defaults to =from=.                |
	| =today=   | date   | no       | The day to count late and due from. The server's date. |
	| =query=   | string | no       | Narrows the headings, as =/search=. Empty is all.      |
	| =closed=  | bool   | no       | =t= adds CLOSED dates (the logbook view).              |
	| =archived= | bool  | no       | =t= keeps headings in archived trees.                  |

	*Response:* An =AgendaResult=: ={"Ok": true, "From", "To", "Today",
	"Nodes": [...], "Days": [{"Date", "Entries": [{"Node", "Kind", "Start",
	"End", "HasTime", "Repeat", "Late", "DueIn", "Done"}]}]}=. =Node= is an
	index into =Nodes=, so a heading on five days is sent once. Every day of
	the range is listed, empty or not. A bad date or query answers =Ok: false=
	with the reason in =Msg=.

* GET /agenda/nodes — Headings With Everything the Agenda Reads

	The headings a query finds (all of them with no query) as =AgendaNode=s -
	planning dates, timestamps, clocks, inherited tags, a summary of the
	children, the file's keywords - for list views that are not laid out by
	day: todo lists, projects, the clocked view.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                               |
	|-----------+--------+----------+-------------------------------------------|
	| =query=   | string | no       | The query expression, as for =/search=.   |

	*Response:* ={"Ok": true, "Nodes": [...]}=, or =Ok: false= and =Msg=.
	EDOC */

func RequestAgenda(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now()
	fail := func(msg string) {
		agendaAnswer(w, common.AgendaResult{Ok: false, Msg: msg, Nodes: []common.AgendaNode{}, Days: []common.AgendaDay{}})
	}
	rules := agendaRules{Today: agendaDayNum(now), WarnDays: agendaWarnDays()}
	var err error
	if s := q.Get("today"); s != "" {
		if rules.Today, err = agendaParseDay(s); err != nil {
			fail("agenda: today: " + err.Error())
			return
		}
	}
	rules.From = rules.Today
	if s := q.Get("from"); s != "" {
		if rules.From, err = agendaParseDay(s); err != nil {
			fail("agenda: from: " + err.Error())
			return
		}
	}
	rules.To = rules.From
	if s := q.Get("to"); s != "" {
		if rules.To, err = agendaParseDay(s); err != nil {
			fail("agenda: to: " + err.Error())
			return
		}
	}
	switch strings.ToLower(q.Get("closed")) {
	case "t", "true", "1", "yes":
		rules.Closed = true
	}
	switch strings.ToLower(q.Get("archived")) {
	case "t", "true", "1", "yes":
		rules.Archived = true
	}
	query := expandQueryFilters(q.Get("query"))
	key := fmt.Sprintf("a\x00%s\x00%d\x00%d\x00%d\x00%v\x00%v\x00%d\x00%s", query, rules.From, rules.To, rules.Today, rules.Closed, rules.Archived, rules.WarnDays, now.Format("200601021504"))
	v, err := agendaCache.get(key, func() (interface{}, error) {
		return buildAgenda(query, rules, now)
	})
	if err != nil {
		fail("agenda: " + err.Error())
		return
	}
	agendaAnswer(w, v)
}

func RequestAgendaNodes(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	query := expandQueryFilters(r.URL.Query().Get("query"))
	key := fmt.Sprintf("n\x00%s\x00%s", query, now.Format("200601021504"))
	v, err := agendaCache.get(key, func() (interface{}, error) {
		return buildAgendaNodes(query, now)
	})
	if err != nil {
		agendaAnswer(w, common.AgendaNodes{Ok: false, Msg: "agenda: " + err.Error(), Nodes: []common.AgendaNode{}})
		return
	}
	agendaAnswer(w, v)
}

// ---------------------------------------------------------------------------
// The query language's date functions, on the same rules
// ---------------------------------------------------------------------------

// Does the heading land on day t? Counted from the real today, so on today it
// includes what is carried forward to it (overdue SCHEDULED, warning and
// overdue DEADLINEs) - what a day view built from OnDate() wants.
func IsOn(p *org.Section, f *common.OrgFile, t time.Time) bool {
	d := agendaDayNum(t)
	return agendaLands(p, f, d, d)
}

// Does the heading land on any day from start up to (not including) end?
func IsIn(p *org.Section, f *common.OrgFile, start time.Time, end time.Time) bool {
	if !end.After(start) {
		return false
	}
	return agendaLands(p, f, agendaDayNum(start), agendaDayNum(end.Add(-time.Nanosecond)))
}

// A COMMENT heading, or one under it, is not in the agenda (org skips
// commented subtrees, as the exporter does).
func agendaCommented(sec *org.Section) bool {
	for s := sec; s != nil; s = s.Parent {
		if s.Headline != nil && s.Headline.IsComment {
			return true
		}
	}
	return false
}

func agendaLands(p *org.Section, f *common.OrgFile, from, to int) bool {
	if p == nil || p.Headline == nil || agendaCommented(p) {
		return false
	}
	if f == nil {
		f = GetDb().FileFromSection(p)
	}
	keys := &agendaKeys{}
	r := agendaRules{From: from, To: to, Today: agendaDayNum(time.Now()), WarnDays: agendaWarnDays()}
	return len(agendaOccurrences(p.Headline, keys.done(p, f), r)) > 0
}

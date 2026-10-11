package orgs

// Time-blocking: the day planner. GET /plan/day, POST /plan/block, POST
// /plan/unblock; wire types in internal/common/plan.go, pinned by
// planner_test.go. Clients: worg's Agenda day view, `orgs plan`.
//
// Placing a task on the day writes SCHEDULED: <2026-10-12 Mon 10:00-11:00>
// on it. That is all a block is - there is no planner file and no second
// copy of the day - so Emacs, the agenda and every other client see the plan
// without knowing a planner exists.
//
// /date/change could already write a stamp like that, but a client using it
// would have to know three things only the file knows:
//
//  1. **What else the old stamp says.** A repeater (+1w) and a warning (-2d)
//     on SCHEDULED belong to the task, not to the hour it was given, so a
//     block keeps them as they are spelled. Dropping a weekly review onto
//     10:00 must not turn it into a one-off.
//  2. **How long the task is.** A drop with no end is sized by EFFORT, else
//     the default hour, and "the next free hour" needs every other block on
//     the day. That is the server's answer, so `orgs plan task` and a
//     keyboard placement in worg land on the same slot.
//  3. **What unplanning means.** Taking a block off removes SCHEDULED, except
//     on a repeating task, where only the time comes off: a drag back to the
//     list must never end a series.
//
// Every write is a line edit of the planning line (changeDateLines, the same
// edit /date/change makes), so DEADLINE and CLOSED on the same line and the
// rest of the file are untouched (see Traps: line edits).
//
// Which tasks are offered: the query (default IsTask()), less anything done,
// archived, commented out, a habit, already on the day with a time, or
// scheduled after the day. A task scheduled on or before the day without a
// time is offered first - it is due and has no hour yet - with how late it is.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// registerPlannerRoutes adds the endpoints for time-blocking / the day planner. Called from RestApi.
func registerPlannerRoutes(api *mux.Router) {
	api.HandleFunc("/plan/day", RequestPlanDay).Methods("GET")
	api.HandleFunc("/plan/block", PostPlanBlock).Methods("POST")
	api.HandleFunc("/plan/unblock", PostPlanUnblock).Methods("POST")
}

const (
	planDefaultStart = 9 * 60
	planDefaultEnd   = 17 * 60
	// The length of a task with no EFFORT, and of an appointment with no end.
	planDefaultMins = 60
	planDefaultQuery = "IsTask()"
	planDayMins      = 24 * 60
)

// ---------------------------------------------------------------------------
// Minutes of the day
// ---------------------------------------------------------------------------

var planClockRe = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)

// "9:05" or "09:05" as minutes after midnight. 24:00 is allowed as the end
// of the day; nothing later is.
func planParseClock(s string) (int, error) {
	m := planClockRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("%q is not a time of day (HH:MM)", s)
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	if mi > 59 || h > 24 || (h == 24 && mi != 0) {
		return 0, fmt.Errorf("%q is not a time of day", s)
	}
	return h*60 + mi, nil
}

func planClock(m int) string {
	if m < 0 {
		m = 0
	}
	if m > planDayMins {
		m = planDayMins
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// The window from the request, defaulting each end on its own.
func planWindow(start, end string) (int, int, error) {
	ds, de := planDefaultStart, planDefaultEnd
	var err error
	if strings.TrimSpace(start) != "" {
		if ds, err = planParseClock(start); err != nil {
			return 0, 0, fmt.Errorf("dayStart: %v", err)
		}
	}
	if strings.TrimSpace(end) != "" {
		if de, err = planParseClock(end); err != nil {
			return 0, 0, fmt.Errorf("dayEnd: %v", err)
		}
	}
	if de <= ds {
		return 0, 0, fmt.Errorf("the working day ends (%s) before it starts (%s)", planClock(de), planClock(ds))
	}
	return ds, de, nil
}

// ---------------------------------------------------------------------------
// Load, overlaps and free time. Said again in worg/src/planner.ts (dayLoad,
// overlapPairs, freeSlot), with the same cases in planner.test.ts.
// ---------------------------------------------------------------------------

type planSpan struct{ Start, End int }

// How full the window [ds, de) is: minutes covered once or more, twice or
// more, and covered outside it. Counted a minute at a time, because a day has
// only 1440 of them and the rule then cannot disagree with itself.
func planLoad(spans []planSpan, ds, de int) (planned, double, outside int) {
	var cover [planDayMins]int
	for _, s := range spans {
		a, b := s.Start, s.End
		if a < 0 {
			a = 0
		}
		if b > planDayMins {
			b = planDayMins
		}
		for m := a; m < b; m++ {
			cover[m]++
		}
	}
	for m := 0; m < planDayMins; m++ {
		in := m >= ds && m < de
		switch {
		case in && cover[m] >= 2:
			planned++
			double++
		case in && cover[m] == 1:
			planned++
		case !in && cover[m] >= 1:
			outside++
		}
	}
	return
}

// Pairs of spans that share at least a minute. A moment (a deadline) never
// overlaps anything.
func planOverlaps(spans []planSpan) [][2]int {
	out := [][2]int{}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			a, b := spans[i], spans[j]
			if a.End > a.Start && b.End > b.Start && a.Start < b.End && b.Start < a.End {
				out = append(out, [2]int{i, j})
			}
		}
	}
	return out
}

// The first gap of at least length minutes inside [ds, de), from `from` on.
func planFreeSlot(spans []planSpan, ds, de, from, length int) (int, bool) {
	sorted := append([]planSpan(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	start := ds
	if from > start {
		start = from
	}
	for _, s := range sorted {
		if s.End <= s.Start || s.End <= start {
			continue
		}
		if s.Start-start >= length {
			break
		}
		start = s.End
	}
	if start+length <= de {
		return start, true
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// The stamp
// ---------------------------------------------------------------------------

// A repeater (+1w, ++1d, .+2d/4d) or a warning (-2d, --2d) inside a stamp.
var planCookieRe = regexp.MustCompile(`^(\.\+|\+\+|\+|--|-)\d+[hdwmy](/\d+[hdwmy])?$`)

// The repeater and warning written inside a stamp, in their order: what a
// new date for the same task has to carry over.
func planCookies(stamp string) []string {
	s := strings.TrimSpace(stamp)
	if i := strings.Index(s, "--<"); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(s, "<>[]")
	out := []string{}
	for _, w := range strings.Fields(s) {
		if planCookieRe.MatchString(w) {
			out = append(out, w)
		}
	}
	return out
}

// <2026-10-12 Mon 10:00-11:00 +1w>; with start < 0 the date alone.
func planStamp(day time.Time, start, end int, cookies []string) string {
	parts := []string{day.Format("2006-01-02"), day.Format("Mon")}
	if start >= 0 {
		t := planClock(start)
		if end > start {
			t += "-" + planClock(end)
		}
		parts = append(parts, t)
	}
	parts = append(parts, cookies...)
	return "<" + strings.Join(parts, " ") + ">"
}

// The SCHEDULED stamp written in a heading's planning line, or "".
func planScheduledText(lines []string, from int) string {
	if from+1 >= len(lines) || !planningRe.MatchString(lines[from+1]) {
		return ""
	}
	m := planningKeyRe("SCHEDULED").FindString(lines[from+1])
	if m == "" {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(m, "SCHEDULED:"))
}

// The edit that places a heading at [start, end) on day: SCHEDULED set to
// that, keeping the old stamp's repeater and warning.
func planBlockLines(lines []string, from, lvl int, day time.Time, start, end int) ([]string, string, error) {
	if start < 0 || end > planDayMins-1 || end <= start {
		return nil, "", fmt.Errorf("a block runs from %s to %s: it has to start before it ends, within the day (by 23:59)", planClock(start), planClock(end))
	}
	stamp := planStamp(day, start, end, planCookies(planScheduledText(lines, from)))
	if d, _, _ := org.ParseTimestamp(stamp); d == nil {
		return nil, "", fmt.Errorf("%s is not a stamp org can read", stamp)
	}
	out, err := changeDateLines(lines, from, lvl, "SCHEDULED", stamp)
	return out, stamp, err
}

// The edit that takes a heading off the day. Returns the stamp left behind
// ("" when SCHEDULED went altogether).
func planUnblockLines(lines []string, from, lvl int, keepDate bool) ([]string, string, error) {
	old := planScheduledText(lines, from)
	if old == "" {
		return nil, "", fmt.Errorf("it is not scheduled")
	}
	cookies := planCookies(old)
	repeats := false
	for _, c := range cookies {
		if strings.HasPrefix(c, "+") || strings.HasPrefix(c, ".+") {
			repeats = true
		}
	}
	if !keepDate && !repeats {
		out, err := changeDateLines(lines, from, lvl, "SCHEDULED", "")
		return out, "", err
	}
	d, _, _ := org.ParseTimestamp(old)
	if d == nil {
		return nil, "", fmt.Errorf("could not read the SCHEDULED stamp %s", old)
	}
	day := time.Date(d.Start.Year(), d.Start.Month(), d.Start.Day(), 0, 0, 0, 0, time.Local)
	stamp := planStamp(day, -1, -1, cookies)
	out, err := changeDateLines(lines, from, lvl, "SCHEDULED", stamp)
	return out, stamp, err
}

// ---------------------------------------------------------------------------
// The day
// ---------------------------------------------------------------------------

func planEffortMins(props map[string]string) (string, int) {
	e := reviewEffortOf(props)
	if e == "" {
		return "", 0
	}
	if d := common.ParseDuration(e); d != nil && d.Mins > 0 {
		return e, int(d.Mins + 0.5)
	}
	return e, 0
}

// "2026-10-12T10:00" → minutes, or -1.
func planEntryMins(s string) int {
	i := strings.Index(s, "T")
	if i < 0 {
		return -1
	}
	m, err := planParseClock(s[i+1:])
	if err != nil {
		return -1
	}
	return m
}

func planPriorityRank(p string) int {
	if p == "" {
		p = "B"
	}
	return int(p[0])
}

type planDayArgs struct {
	Date, Today int
	Query       string
	DayStart    int
	DayEnd      int
	Now         time.Time
}

func buildPlanDay(a planDayArgs) (common.PlanDay, error) {
	res := common.PlanDay{
		Ok: true, Date: agendaDayStr(a.Date), Today: agendaDayStr(a.Today),
		DayStart: planClock(a.DayStart), DayEnd: planClock(a.DayEnd), Query: a.Query,
		Blocks: []common.PlanBlock{}, AllDay: []common.PlanBlock{}, Tasks: []common.PlanTask{},
		Overlaps: [][2]int{}, DefaultMins: planDefaultMins,
	}
	if res.Query == "" {
		res.Query = planDefaultQuery
	}
	ag, err := buildAgenda("", agendaRules{From: a.Date, To: a.Date, Today: a.Today, WarnDays: agendaWarnDays()}, a.Now)
	if err != nil {
		return res, err
	}
	onDay := map[string]bool{}
	untimed := []common.PlanBlock{}
	for _, d := range ag.Days {
		for _, e := range d.Entries {
			n := ag.Nodes[e.Node]
			_, effortMins := planEffortMins(n.Props)
			// A stamp in the headline is part of its title (as /search says),
			// but on a block it only repeats the time the block is drawn at.
			title := strings.TrimSpace(markerStampRe.ReplaceAllString(n.Headline, ""))
			if title == "" {
				title = n.Headline
			}
			b := common.PlanBlock{
				Hash: n.Hash, Headline: title, Filename: n.Filename, LineNum: n.LineNum,
				Status: n.Status, Priority: n.Priority, Tags: n.Tags, Kind: e.Kind,
				Repeat: e.Repeat, Done: e.Done, EffortMins: effortMins, DueIn: e.DueIn, Late: e.Late,
			}
			if b.Tags == nil {
				b.Tags = []string{}
			}
			switch e.Kind {
			case common.AgendaScheduled:
				if n.Scheduled != nil {
					b.Stamp, b.Repeater = n.Scheduled.Text, n.Scheduled.Repeat
				}
			case common.AgendaDeadline:
				if n.Deadline != nil {
					b.Stamp, b.Repeater = n.Deadline.Text, n.Deadline.Repeat
				}
			case common.AgendaTimestamp:
				if len(n.Timestamps) == 1 {
					b.Stamp, b.Repeater = n.Timestamps[0].Text, n.Timestamps[0].Repeat
				}
			}
			start := planEntryMins(e.Start)
			if !e.HasTime || start < 0 {
				untimed = append(untimed, b)
				continue
			}
			end := planEntryMins(e.End)
			switch {
			case e.Kind == common.AgendaDeadline:
				end = start
			case end < 0 || e.End == "":
				b.Implied = true
				l := effortMins
				if l <= 0 {
					l = planDefaultMins
				}
				end = start + l
			case strings.HasSuffix(e.End, "T23:59"):
				// A range running on past midnight is clipped at 23:59 by the
				// agenda; the day loses the last minute too.
				end = planDayMins
			}
			if end > planDayMins {
				end = planDayMins
			}
			b.Start, b.End, b.Mins = planClock(start), planClock(end), end-start
			// One day's SCHEDULED, not done, is the planner's to move. A
			// SCHEDULED range over several days is not: a drag would fold it
			// into one.
			b.Movable = e.Kind == common.AgendaScheduled && !e.Done && n.Scheduled != nil &&
				(n.Scheduled.End == "" || n.Scheduled.End[:10] == n.Scheduled.Start[:10])
			res.Blocks = append(res.Blocks, b)
			onDay[b.Hash] = true
		}
	}

	// The tasks that could go on the day.
	keys := &agendaKeys{}
	err = agendaSections(expandQueryFilters(res.Query), func(sec *org.Section, f *common.OrgFile) {
		h := sec.Headline
		if h == nil || onDay[sec.Hash] {
			return
		}
		if f == nil || f.Doc == nil || IsArchived(sec, f.Doc) || agendaCommented(sec) || IsHabitSection(sec) {
			return
		}
		if keys.done(sec, f) {
			return
		}
		t := SectionToTodo(sec, f)
		if t == nil {
			return
		}
		task := common.PlanTask{
			Hash: t.Hash, Headline: t.Headline, Filename: t.Filename, LineNum: t.LineNum,
			Status: t.Status, Priority: t.Priority, Tags: t.Tags, Late: -1,
		}
		if task.Tags == nil {
			task.Tags = []string{}
		}
		task.Effort, task.EffortMins = planEffortMins(t.Props)
		if h.Scheduled != nil && h.Scheduled.Date != nil && !h.Scheduled.Date.Start.IsZero() {
			sd := h.Scheduled.Date
			day := agendaDayNum(sd.Start)
			if day > a.Date || (day == a.Date && sd.HaveTime) {
				return
			}
			task.Scheduled = agendaDayStr(day)
			task.Late = a.Date - day
			task.Repeater = sd.RepeatString()
		}
		if h.Deadline != nil && h.Deadline.Date != nil && !h.Deadline.Date.Start.IsZero() {
			day := agendaDayNum(h.Deadline.Date.Start)
			task.Deadline = agendaDayStr(day)
			task.DueIn = day - a.Date
		}
		res.Tasks = append(res.Tasks, task)
	})
	if err != nil {
		return res, err
	}
	// Due first (the most overdue at the top), then by deadline, priority,
	// and where it is in the files.
	sort.SliceStable(res.Tasks, func(i, j int) bool {
		x, y := res.Tasks[i], res.Tasks[j]
		if (x.Late >= 0) != (y.Late >= 0) {
			return x.Late >= 0
		}
		if x.Late != y.Late {
			return x.Late > y.Late
		}
		if (x.Deadline != "") != (y.Deadline != "") {
			return x.Deadline != ""
		}
		if x.Deadline != y.Deadline {
			return x.Deadline < y.Deadline
		}
		if pr, qr := planPriorityRank(x.Priority), planPriorityRank(y.Priority); pr != qr {
			return pr < qr
		}
		if x.Filename != y.Filename {
			return x.Filename < y.Filename
		}
		return x.LineNum < y.LineNum
	})
	offered := map[string]bool{}
	for _, t := range res.Tasks {
		offered[t.Hash] = true
		if t.EffortMins > 0 {
			res.Load.BacklogMins += t.EffortMins
		} else {
			res.Load.BacklogNoEst++
		}
	}
	for _, b := range untimed {
		if !offered[b.Hash] {
			res.AllDay = append(res.AllDay, b)
		}
	}

	spans := planSpans(res.Blocks)
	res.Load.WindowMins = a.DayEnd - a.DayStart
	res.Load.PlannedMins, res.Load.DoubleMins, res.Load.OutsideMins = planLoad(spans, a.DayStart, a.DayEnd)
	res.Load.FreeMins = res.Load.WindowMins - res.Load.PlannedMins
	res.Overlaps = planOverlaps(spans)
	return res, nil
}

func planSpans(blocks []common.PlanBlock) []planSpan {
	out := make([]planSpan, len(blocks))
	for i, b := range blocks {
		s, _ := planParseClock(b.Start)
		e, _ := planParseClock(b.End)
		out[i] = planSpan{s, e}
	}
	return out
}

/* SDOC: API
* GET /plan/day — The Day Planner: Blocks, Tasks to Place, and Load

	One day laid out for time-blocking: the timed things already on it, the
	open tasks that could go on it, and how full it is inside the working
	window. worg's Agenda day view and =orgs plan= draw this.

	- =Blocks= are the day's timed agenda entries (a SCHEDULED, a plain
	  stamp, a deadline with a time), each with =Start=/=End= as =HH:MM=. A
	  stamp with no end is given its EFFORT, else an hour (=Implied=). A
	  deadline is a moment (=Mins= 0). =Movable= is true for a one-day
	  SCHEDULED that is not done: the planner's to move, resize and unplan.
	- =Tasks= are what the query finds (default =IsTask()=), less anything
	  done, archived, commented, a habit, already on the day with a time, or
	  scheduled after the day. Scheduled on or before the day without a time
	  comes first, with =Late= days. =EffortMins= sizes a drop.
	- =AllDay= is the rest of the day's entries without a time.
	- =Load= counts minutes inside the window covered by a block
	  (=PlannedMins=), left (=FreeMins=), covered twice (=DoubleMins=), and
	  the backlog's EFFORT. =Overlaps= are pairs of indexes into =Blocks=.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                          |
	|------------+--------+----------+------------------------------------------------------|
	| =date=     | date   | no       | The day, =YYYY-MM-DD=. Defaults to today.             |
	| =query=    | string | no       | The tasks to offer, as =/search=. Default =IsTask()=. |
	| =dayStart= | time   | no       | The working window's start, =HH:MM=. Default 09:00.   |
	| =dayEnd=   | time   | no       | Its end. Default 17:00.                               |
	| =today=    | date   | no       | The day to count late from. The server's date.        |

	*Response:* A =PlanDay=: ={"Ok": true, "Date", "Today", "DayStart",
	"DayEnd", "Query", "Blocks": [...], "AllDay": [...], "Tasks": [...],
	"Load": {...}, "Overlaps": [[i, j]], "DefaultMins": 60}=, or =Ok: false=
	with =Msg=.

* POST /plan/block — Place a Heading on the Day

	Writes =SCHEDULED: <2026-10-12 Mon 10:00-11:00>= on the heading, as a
	line edit of its planning line. A repeater or warning already on its
	SCHEDULED is kept. Moving and resizing a block is the same call.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field      | Type   | Required | Description                                                     |
	|------------+--------+----------+-----------------------------------------------------------------|
	| =Hash=     | string | yes      | The heading.                                                    |
	| =Date=     | string | no       | =YYYY-MM-DD=; today when empty.                                 |
	| =Start=    | string | no       | =HH:MM=. Empty: the first free gap long enough (from now today). |
	| =End=      | string | no       | =HH:MM=. Empty: Start plus EFFORT, else an hour.                |
	| =DayStart= | string | no       | The window the free gap is looked for in. Default 09:00.        |
	| =DayEnd=   | string | no       | Default 17:00.                                                  |

	*Response:* A =PlanWrite=: ={"Ok": true, "Hash", "Stamp", "Start", "End"}=,
	or =Ok: false= with =Msg=.

* POST /plan/unblock — Take a Heading off the Day

	Removes SCHEDULED from the heading. On a repeating SCHEDULED, or with
	=KeepDate=, only the time comes off: the date and repeater stay.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field      | Type   | Required | Description                         |
	|------------+--------+----------+-------------------------------------|
	| =Hash=     | string | yes      | The heading.                        |
	| =KeepDate= | bool   | no       | Keep the date, take off the time.   |

	*Response:* A =PlanWrite=; =Stamp= is what is left, empty when SCHEDULED was removed.
	EDOC */

func RequestPlanDay(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now()
	fail := func(msg string) {
		agendaAnswer(w, common.PlanDay{Ok: false, Msg: "plan: " + msg, Blocks: []common.PlanBlock{},
			AllDay: []common.PlanBlock{}, Tasks: []common.PlanTask{}, Overlaps: [][2]int{}})
	}
	a := planDayArgs{Today: agendaDayNum(now), Query: q.Get("query"), Now: now}
	var err error
	if s := q.Get("today"); s != "" {
		if a.Today, err = agendaParseDay(s); err != nil {
			fail("today: " + err.Error())
			return
		}
	}
	a.Date = a.Today
	if s := q.Get("date"); s != "" {
		if a.Date, err = agendaParseDay(s); err != nil {
			fail("date: " + err.Error())
			return
		}
	}
	if a.DayStart, a.DayEnd, err = planWindow(q.Get("dayStart"), q.Get("dayEnd")); err != nil {
		fail(err.Error())
		return
	}
	res, err := buildPlanDay(a)
	if err != nil {
		fail(err.Error())
		return
	}
	agendaAnswer(w, res)
}

// The heading, its file and its lines, found afresh (see Traps: hashes).
func planTarget(hash string) (*org.Section, string, []string, int, error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return nil, "", nil, 0, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil || f.Doc == nil {
		return nil, "", nil, 0, fmt.Errorf("could not find the file that heading is in")
	}
	lines, from, _, ok := recordLines(f.Doc.Path, sec)
	if !ok {
		return nil, "", nil, 0, fmt.Errorf("could not read %s", f.Doc.Path)
	}
	return sec, f.Doc.Path, lines, from, nil
}

// The hash of the heading at row after a write: a planning edit does not
// change it today, but the answer should not depend on that.
func planHashAt(path string, row int, fallback string) string {
	if f := GetDb().FindByFile(path); f != nil {
		registerAllSections()
		if s := sectionAtRow(f, row); s != nil && s.Hash != "" {
			return s.Hash
		}
	}
	return fallback
}

func planBlock(req common.PlanBlockReq, now time.Time) (common.PlanWrite, error) {
	out := common.PlanWrite{}
	dayNum := agendaDayNum(now)
	if strings.TrimSpace(req.Date) != "" {
		var err error
		if dayNum, err = agendaParseDay(strings.TrimSpace(req.Date)); err != nil {
			return out, fmt.Errorf("date: %v", err)
		}
	}
	day := agendaDayTime(dayNum)
	sec, path, lines, from, err := planTarget(req.Hash)
	if err != nil {
		return out, err
	}
	start, end := -1, -1
	if strings.TrimSpace(req.Start) != "" {
		if start, err = planParseClock(req.Start); err != nil {
			return out, fmt.Errorf("start: %v", err)
		}
	}
	if strings.TrimSpace(req.End) != "" {
		if end, err = planParseClock(req.End); err != nil {
			return out, fmt.Errorf("end: %v", err)
		}
	}
	length := planDefaultMins
	if t := SectionToTodo(sec, nil); t != nil {
		if _, m := planEffortMins(t.Props); m > 0 {
			length = m
		}
	}
	if start < 0 && end >= 0 {
		start = end - length
	}
	if start < 0 {
		// The first free gap: from now on today (to the next quarter hour),
		// from the start of the window on any other day.
		ds, de, werr := planWindow(req.DayStart, req.DayEnd)
		if werr != nil {
			return out, werr
		}
		fromMin := ds
		if dayNum == agendaDayNum(now) {
			n := now.Hour()*60 + now.Minute()
			n = (n + 14) / 15 * 15
			if n > fromMin {
				fromMin = n
			}
		}
		pd, perr := buildPlanDay(planDayArgs{Date: dayNum, Today: agendaDayNum(now), DayStart: ds, DayEnd: de, Now: now})
		if perr != nil {
			return out, perr
		}
		others := []common.PlanBlock{}
		for _, b := range pd.Blocks {
			if b.Hash != req.Hash {
				others = append(others, b)
			}
		}
		s, ok := planFreeSlot(planSpans(others), ds, de, fromMin, length)
		if !ok {
			return out, fmt.Errorf("no free %d minutes left between %s and %s on %s", length, planClock(fromMin), planClock(de), agendaDayStr(dayNum))
		}
		start = s
	}
	if end < 0 {
		end = start + length
	}
	lines, stamp, err := planBlockLines(lines, from, sec.Headline.Lvl, day, start, end)
	if err != nil {
		return out, err
	}
	if err := writeLines(path, lines); err != nil {
		return out, err
	}
	return common.PlanWrite{Ok: true, Hash: planHashAt(path, from, req.Hash), Stamp: stamp,
		Start: planClock(start), End: planClock(end)}, nil
}

func planUnblock(req common.PlanUnblockReq) (common.PlanWrite, error) {
	out := common.PlanWrite{}
	sec, path, lines, from, err := planTarget(req.Hash)
	if err != nil {
		return out, err
	}
	lines, stamp, err := planUnblockLines(lines, from, sec.Headline.Lvl, req.KeepDate)
	if err != nil {
		return out, err
	}
	if err := writeLines(path, lines); err != nil {
		return out, err
	}
	out = common.PlanWrite{Ok: true, Hash: planHashAt(path, from, req.Hash), Stamp: stamp}
	if stamp != "" {
		out.Msg = "the time came off; the date stays"
		if !req.KeepDate {
			out.Msg = "it repeats, so the date and repeater stay and only the time came off"
		}
	}
	return out, nil
}

func PostPlanBlock(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		answerErr(w, err)
		return
	}
	var req common.PlanBlockReq
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	res, err := planBlock(req, time.Now())
	if err != nil {
		agendaAnswer(w, common.PlanWrite{Ok: false, Msg: err.Error()})
		return
	}
	agendaAnswer(w, res)
}

func PostPlanUnblock(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		answerErr(w, err)
		return
	}
	var req common.PlanUnblockReq
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	res, err := planUnblock(req)
	if err != nil {
		agendaAnswer(w, common.PlanWrite{Ok: false, Msg: err.Error()})
		return
	}
	agendaAnswer(w, res)
}

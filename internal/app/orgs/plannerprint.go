package orgs

// GET /agenda/print: the agenda as paper - a week on one landscape sheet, a
// day on a portrait one, a month as a calendar grid.
//
// What lands on each day is buildAgenda's answer, the same one GET /agenda
// gives, so a printed week and the agenda on screen cannot disagree. This
// file only decides where things go on the sheet: an overdue item (Late, which
// the agenda carries forward to today) is lifted out of today's column into
// the Overdue panel, because a carried item is a today problem the page should
// show once, not a thing happening at a time.
//
// The page is one html/template (planner/planner.html), a layout stylesheet
// (planner/base.css) and a theme (planner/themes/<name>.css), all inlined so
// the page stands alone and prints the same from a browser or a headless
// Chrome. A theme is one file: the listing is a glob, as invoice themes are.

import (
	"fmt"
	"html/template"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/common"
)

// registerPlannerPrintRoutes adds GET /agenda/print. Called from RestApi.
func registerPlannerPrintRoutes(api *mux.Router) {
	api.HandleFunc("/agenda/print", RequestAgendaPrint).Methods("GET")
}

// ---------------------------------------------------------------------------
// The model the template draws
// ---------------------------------------------------------------------------

// One thing on the page.
type plannerEntry struct {
	Time    string // "09:00" for a timed entry, "" otherwise
	End     string // "10:30", or ""
	Start   int    // minutes after midnight, for placing
	Stop    int
	Title   string
	Kind    string // timestamp, scheduled, deadline
	Keyword string
	// A box to tick: the heading has a keyword. Ticked when it is done.
	Box      bool
	Done     bool
	Priority string
	// A word beside the title, and its class: "due", "in 3d", "late 2d".
	Mark      string
	MarkClass string
	node      int
}

// A timed entry placed on the day view's hour grid.
type plannerSlot struct {
	Entry plannerEntry
	// Percentages of the grid's height, and of the width beside the hours.
	Top, Height, Left, Width float64
	Style                    template.CSS
}

type plannerDay struct {
	Date    time.Time
	Key     string // YYYY-MM-DD
	Today   bool
	Weekend bool
	// A day of the month grid that belongs to the month before or after.
	Outside bool
	AllDay  []plannerEntry
	Timed   []plannerEntry
	// Month view: what fits in a cell, and how many did not.
	Shown []plannerEntry
	More  int
}

type plannerHabit struct {
	Title string
	// One per day of the sheet: "done", "miss" or "open".
	Marks []string
}

type plannerSheet struct {
	View     string // week, day, month
	Title    string
	Subtitle string
	Days     []plannerDay   // week: 7, day: 1
	Weeks    [][]plannerDay // month
	Weekdays []string       // month grid headings
	// Week view: the first four days on the left page, the rest on the right.
	Left, Right []plannerDay
	Overdue     []plannerEntry
	Tasks       []plannerEntry
	Habits      []plannerHabit
	HabitDays   []string
	// Day view's grid.
	Hours []string
	Slots []plannerSlot
}

type plannerPage struct {
	Title     string
	View      string
	Theme     string
	Paper     string // CSS page size: "A4 landscape"
	SheetW    string // "279mm"
	SheetH    string
	Margin    string
	Base      template.CSS
	CSS       template.CSS
	Sheets    []plannerSheet
	Generated string
}

// ---------------------------------------------------------------------------
// Laying out
// ---------------------------------------------------------------------------

// weekStart is the first day of the week holding t: back to start (Monday or
// Sunday), never forward.
func weekStart(t time.Time, start time.Weekday) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	back := (int(t.Weekday()) - int(start) + 7) % 7
	return t.AddDate(0, 0, -back)
}

// monthGrid is the days of the calendar page for the month holding t: whole
// weeks, from the week of the 1st to the week of the last day.
func monthGrid(t time.Time, start time.Weekday) [][]time.Time {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	d := weekStart(first, start)
	weeks := [][]time.Time{}
	for !d.After(last) {
		w := make([]time.Time, 7)
		for i := range w {
			w[i] = d
			d = d.AddDate(0, 0, 1)
		}
		weeks = append(weeks, w)
	}
	return weeks
}

// gridHours is the span of the day view's hour grid: lo to hi, widened to
// take in anything that starts earlier or ends later, in whole hours.
func gridHours(es []plannerEntry, lo, hi int) (int, int) {
	for _, e := range es {
		if h := e.Start / 60; h < lo {
			lo = h
		}
		stop := e.Stop
		if stop <= e.Start {
			stop = e.Start + 30
		}
		if h := int(math.Ceil(float64(stop) / 60)); h > hi {
			hi = h
		}
	}
	if lo < 0 {
		lo = 0
	}
	if hi > 24 {
		hi = 24
	}
	return lo, hi
}

// placeTimed puts timed entries on a grid running from hour lo to hour hi.
// Something with no end is drawn half an hour long. Entries that overlap
// share the width: each run of overlapping entries is as many lanes wide as
// it needs, each entry in the first lane free when it starts.
func placeTimed(es []plannerEntry, lo, hi int) []plannerSlot {
	from, to := lo*60, hi*60
	total := float64(to - from)
	if total <= 0 {
		return nil
	}
	sorted := append([]plannerEntry{}, es...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	slots := []plannerSlot{}
	type placed struct {
		slot int
		lane int
		stop int
	}
	var run []placed
	runEnd := -1
	finish := func() {
		lanes := 0
		for _, p := range run {
			if p.lane+1 > lanes {
				lanes = p.lane + 1
			}
		}
		for _, p := range run {
			s := &slots[p.slot]
			s.Width = 100 / float64(lanes)
			s.Left = float64(p.lane) * s.Width
		}
		run = nil
	}
	for _, e := range sorted {
		start, stop := e.Start, e.Stop
		if stop <= start {
			stop = start + 30
		}
		if stop <= from || start >= to {
			continue
		}
		if start < from {
			start = from
		}
		if stop > to {
			stop = to
		}
		if len(run) > 0 && start >= runEnd {
			finish()
		}
		lane := 0
		for {
			free := true
			for _, p := range run {
				if p.lane == lane && p.stop > start {
					free = false
					break
				}
			}
			if free {
				break
			}
			lane++
		}
		slots = append(slots, plannerSlot{
			Entry:  e,
			Top:    float64(start-from) / total * 100,
			Height: float64(stop-start) / total * 100,
		})
		run = append(run, placed{slot: len(slots) - 1, lane: lane, stop: stop})
		if len(run) == 1 || stop > runEnd {
			runEnd = stop
		}
	}
	if len(run) > 0 {
		finish()
	}
	for i := range slots {
		s := &slots[i]
		s.Style = template.CSS(fmt.Sprintf("top:%.3f%%;height:%.3f%%;left:%.3f%%;width:%.3f%%",
			s.Top, s.Height, s.Left, s.Width))
	}
	return slots
}

// A heading's title as a person reads it on paper: no timestamps (a stamp in
// the headline is part of /search's title), links as their description.
var (
	plannerStampRe = regexp.MustCompile(`[<\[]\d{4}-\d{2}-\d{2}[^>\]]*[>\]](--[<\[]\d{4}-\d{2}-\d{2}[^>\]]*[>\]])?`)
	plannerLinkRe  = regexp.MustCompile(`\[\[([^\]]+)\](?:\[([^\]]+)\])?\]`)
	plannerCookie  = regexp.MustCompile(`\[\d*[/%]\d*\]`)
)

func plannerTitle(s string) string {
	s = plannerLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := plannerLinkRe.FindStringSubmatch(m)
		if sub[2] != "" {
			return sub[2]
		}
		return strings.TrimPrefix(sub[1], "file:")
	})
	s = plannerStampRe.ReplaceAllString(s, "")
	s = plannerCookie.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

func plannerMinutes(stamp string) (string, int) {
	i := strings.Index(stamp, "T")
	if i < 0 {
		return "", 0
	}
	hm := stamp[i+1:]
	t, err := time.Parse("15:04", hm)
	if err != nil {
		return "", 0
	}
	return hm, t.Hour()*60 + t.Minute()
}

// plannerEntryOf is one agenda entry as the page draws it, on day (YYYY-MM-DD).
func plannerEntryOf(e common.AgendaEntry, n common.AgendaNode, day string) plannerEntry {
	pe := plannerEntry{
		Title:    plannerTitle(n.Headline),
		Kind:     e.Kind,
		Keyword:  n.Status,
		Box:      n.IsTodo || n.IsDone,
		Done:     e.Done || n.IsDone,
		Priority: n.Priority,
		node:     e.Node,
	}
	if e.HasTime {
		pe.Time, pe.Start = plannerMinutes(e.Start)
		if e.End != "" {
			// A range running past midnight is clipped to the day by the
			// agenda, which ends it at 23:59.
			pe.End, pe.Stop = plannerMinutes(e.End)
		}
	}
	switch {
	case e.Late > 0:
		pe.Mark, pe.MarkClass = fmt.Sprintf("late %dd", e.Late), "late"
	// The deadline's own day carries DueIn too, while it is warning; there it
	// is simply due.
	case e.Kind == common.AgendaDeadline && e.DueIn > 0 && !(n.Deadline != nil && strings.HasPrefix(n.Deadline.Start, day)):
		pe.Mark, pe.MarkClass = fmt.Sprintf("in %dd", e.DueIn), "warn"
	case e.Kind == common.AgendaDeadline:
		pe.Mark, pe.MarkClass = "due", "due"
	}
	return pe
}

// plannerDayOf splits a day's entries into all day and timed, keeping the
// agenda's order (timed by time, then all day by kind and priority).
// Carried-forward overdue entries are left out: they go in the Overdue panel.
// So are habits when noHabits says so: a daily habit is a line on every day of
// the sheet, and the habits strip already has a box for each.
func plannerDayOf(d common.AgendaDay, res *common.AgendaResult, today string, noHabits bool) plannerDay {
	t, _ := time.ParseInLocation("2006-01-02", d.Date, time.Local)
	pd := plannerDay{Date: t, Key: d.Date, Today: d.Date == today,
		Weekend: t.Weekday() == time.Saturday || t.Weekday() == time.Sunday}
	for _, e := range d.Entries {
		if e.Late > 0 || (noHabits && res.Nodes[e.Node].IsHabit) {
			continue
		}
		pe := plannerEntryOf(e, res.Nodes[e.Node], d.Date)
		if pe.Time != "" {
			pd.Timed = append(pd.Timed, pe)
		} else {
			pd.AllDay = append(pd.AllDay, pe)
		}
	}
	return pd
}

// The overdue entries of today's agenda, once each (habits left out with
// noHabits: a missed habit shows as a missed box on the strip).
func plannerOverdue(res *common.AgendaResult, noHabits bool) []plannerEntry {
	out := []plannerEntry{}
	seen := map[int]bool{}
	for _, d := range res.Days {
		for _, e := range d.Entries {
			if e.Late <= 0 || seen[e.Node] || (noHabits && res.Nodes[e.Node].IsHabit) {
				continue
			}
			seen[e.Node] = true
			out = append(out, plannerEntryOf(e, res.Nodes[e.Node], d.Date))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return plannerLate(out[i].Mark) > plannerLate(out[j].Mark)
	})
	return out
}

func plannerLate(mark string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(mark, "late "), "d"))
	return n
}

// Under way or next up: the tasks a week is planned around. (Todo.IsActive
// is not this: it is true of every open keyword.)
func plannerActive(n common.AgendaNode) bool {
	switch strings.ToUpper(n.Status) {
	case "NEXT", "STARTED", "DOING", "INPROGRESS", "IN-PROGRESS", "ACTIVE":
		return true
	}
	return false
}

func plannerPriority(p string) int {
	switch p {
	case "A":
		return 0
	case "", "B":
		return 1
	case "C":
		return 2
	}
	return 3
}

// plannerTasks is the panel of things worth doing that have no day: the
// active ones first (NEXT and friends), then by priority, then in file order.
func plannerTasks(nodes []common.AgendaNode, limit int) []plannerEntry {
	idx := []int{}
	for i, n := range nodes {
		if n.IsTodo && !n.IsHabit {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		na, nb := nodes[idx[a]], nodes[idx[b]]
		if aa, ab := plannerActive(na), plannerActive(nb); aa != ab {
			return aa
		}
		return plannerPriority(na.Priority) < plannerPriority(nb.Priority)
	})
	out := []plannerEntry{}
	for _, i := range idx {
		if len(out) == limit {
			break
		}
		n := nodes[i]
		out = append(out, plannerEntry{Title: plannerTitle(n.Headline), Keyword: n.Status, Box: true, Priority: n.Priority})
	}
	return out
}

// plannerHabits is a row per habit with a mark per day: kept, missed (a day
// gone by with no completion) or open (today and after, to tick by hand).
func plannerHabits(nodes []common.AgendaNode, days []time.Time, today string, limit int) []plannerHabit {
	out := []plannerHabit{}
	for _, n := range nodes {
		if !n.IsHabit {
			continue
		}
		if len(out) == limit {
			break
		}
		done := map[string]bool{}
		for _, d := range n.DoneDates {
			done[d] = true
		}
		h := plannerHabit{Title: plannerTitle(n.Headline)}
		for _, d := range days {
			k := d.Format("2006-01-02")
			switch {
			case done[k]:
				h.Marks = append(h.Marks, "done")
			case k < today:
				h.Marks = append(h.Marks, "miss")
			default:
				h.Marks = append(h.Marks, "open")
			}
		}
		out = append(out, h)
	}
	return out
}

// ---------------------------------------------------------------------------
// The request
// ---------------------------------------------------------------------------

type plannerOpts struct {
	View      string
	From      time.Time
	Today     time.Time
	Pages     int
	Query     string
	Tasks     string // "" for the default, "none" for no panel
	Habits    bool
	Theme     string
	Paper     string
	WeekStart time.Weekday
}

// The panel of undated tasks when nobody names one: open tasks with no date
// of any kind, which is what a weekly plan is made from.
const plannerDefaultTasks = `IsTask() && !HasAnyDate()`

// Paper sizes in millimetres, portrait.
var plannerPapers = map[string][2]float64{"a4": {210, 297}, "letter": {215.9, 279.4}}

const plannerMargin = 9.0

func plannerAgenda(query string, from, to, today time.Time) (common.AgendaResult, error) {
	r := agendaRules{From: agendaDayNum(from), To: agendaDayNum(to), Today: agendaDayNum(today), WarnDays: agendaWarnDays()}
	return buildAgenda(query, r, time.Now())
}

// PlannerThemes is every stylesheet in planner/themes on the template path.
func PlannerThemes() []string {
	names := []string{}
	for _, n := range assets.GlobTemplates("planner/themes/*.css", Conf().Server.TemplatePath) {
		names = append(names, strings.TrimSuffix(filepath.Base(n), ".css"))
	}
	return names
}

func plannerSheets(o plannerOpts) ([]plannerSheet, error) {
	today := o.Today.Format("2006-01-02")
	query := expandQueryFilters(o.Query)
	var overdue []plannerEntry
	var tasks []common.AgendaNode
	var habits []common.AgendaNode
	if o.View != "month" {
		res, err := plannerAgenda(query, o.Today, o.Today, o.Today)
		if err != nil {
			return nil, err
		}
		overdue = plannerOverdue(&res, o.Habits)
		if strings.ToLower(strings.TrimSpace(o.Tasks)) != "none" {
			tq := o.Tasks
			if strings.TrimSpace(tq) == "" {
				tq = plannerDefaultTasks
			}
			n, err := buildAgendaNodes(expandQueryFilters(tq), time.Now())
			if err != nil {
				return nil, fmt.Errorf("tasks: %v", err)
			}
			tasks = n.Nodes
		}
		if o.Habits {
			n, err := buildAgendaNodes("IsHabit()", time.Now())
			if err == nil {
				habits = n.Nodes
			}
		}
	}
	sheets := []plannerSheet{}
	for p := 0; p < o.Pages; p++ {
		switch o.View {
		case "day":
			day := o.From.AddDate(0, 0, p)
			res, err := plannerAgenda(query, day, day, o.Today)
			if err != nil {
				return nil, err
			}
			pd := plannerDayOf(res.Days[0], &res, today, o.Habits)
			lo, hi := gridHours(pd.Timed, 7, 21)
			s := plannerSheet{View: "day", Days: []plannerDay{pd},
				Title:    day.Format("Monday"),
				Subtitle: fmt.Sprintf("%s · week %d", day.Format("2 January 2006"), isoWeek(day)),
				Slots:    placeTimed(pd.Timed, lo, hi),
				Tasks:    plannerTasks(tasks, 8),
			}
			for h := lo; h < hi; h++ {
				s.Hours = append(s.Hours, fmt.Sprintf("%02d:00", h))
			}
			if pd.Key >= today {
				s.Overdue = overdue
			}
			s.Habits = plannerHabits(habits, []time.Time{day}, today, 6)
			sheets = append(sheets, s)
		case "month":
			first := time.Date(o.From.Year(), o.From.Month()+time.Month(p), 1, 0, 0, 0, 0, time.Local)
			grid := monthGrid(first, o.WeekStart)
			last := grid[len(grid)-1][6]
			res, err := plannerAgenda(query, grid[0][0], last, o.Today)
			if err != nil {
				return nil, err
			}
			s := plannerSheet{View: "month", Title: first.Format("January"), Subtitle: first.Format("2006")}
			for i := 0; i < 7; i++ {
				s.Weekdays = append(s.Weekdays, grid[0][i].Format("Monday"))
			}
			k := 0
			for _, w := range grid {
				row := []plannerDay{}
				for range w {
					pd := plannerDayOf(res.Days[k], &res, today, o.Habits)
					pd.Outside = pd.Date.Month() != first.Month()
					all := append(append([]plannerEntry{}, pd.AllDay...), pd.Timed...)
					max := 7
					if len(grid) > 5 {
						max = 5
					}
					if len(all) > max {
						pd.Shown, pd.More = all[:max-1], len(all)-(max-1)
					} else {
						pd.Shown = all
					}
					row = append(row, pd)
					k++
				}
				s.Weeks = append(s.Weeks, row)
			}
			sheets = append(sheets, s)
		default:
			start := weekStart(o.From, o.WeekStart).AddDate(0, 0, 7*p)
			end := start.AddDate(0, 0, 6)
			res, err := plannerAgenda(query, start, end, o.Today)
			if err != nil {
				return nil, err
			}
			s := plannerSheet{View: "week", Title: plannerWeekTitle(start, end),
				Subtitle: fmt.Sprintf("Week %d", isoWeek(start.AddDate(0, 0, 3)))}
			days := []time.Time{}
			for i := range res.Days {
				pd := plannerDayOf(res.Days[i], &res, today, o.Habits)
				s.Days = append(s.Days, pd)
				days = append(days, pd.Date)
				s.HabitDays = append(s.HabitDays, pd.Date.Format("Mon")[:1])
			}
			s.Left, s.Right = s.Days[:4], s.Days[4:]
			if end.Format("2006-01-02") >= today {
				s.Overdue = overdue
			}
			s.Tasks = plannerTasks(tasks, 10)
			s.Habits = plannerHabits(habits, days, today, 6)
			sheets = append(sheets, s)
		}
	}
	return sheets, nil
}

func isoWeek(t time.Time) int {
	_, w := t.ISOWeek()
	return w
}

// "October 2026", or "28 September – 4 October 2026" across a month.
func plannerWeekTitle(start, end time.Time) string {
	switch {
	case start.Month() == end.Month():
		return start.Format("January 2006")
	case start.Year() == end.Year():
		return start.Format("January") + " – " + end.Format("January 2006")
	}
	return start.Format("January 2006") + " – " + end.Format("January 2006")
}

var plannerFuncs = template.FuncMap{
	"day":   func(t time.Time) string { return strconv.Itoa(t.Day()) },
	"dow":   func(t time.Time) string { return t.Format("Mon") },
	"lines": func(n int) []int { return make([]int, n) },
}

// PlannerHTML renders the sheets as one page.
func PlannerHTML(o plannerOpts) ([]byte, error) {
	if o.Theme == "" {
		o.Theme = "classic"
	}
	tplPath := assets.FindTemplate("planner/planner.html", Conf().Server.TemplatePath)
	if tplPath == "" {
		return nil, fmt.Errorf("no planner/planner.html on the template path")
	}
	cssPath := assets.FindTemplate("planner/themes/"+filepath.Base(o.Theme)+".css", Conf().Server.TemplatePath)
	if cssPath == "" {
		return nil, fmt.Errorf("no planner theme %q (themes: %s)", o.Theme, strings.Join(PlannerThemes(), ", "))
	}
	css, err := os.ReadFile(cssPath)
	if err != nil {
		return nil, err
	}
	base := []byte{}
	if p := assets.FindTemplate("planner/base.css", Conf().Server.TemplatePath); p != "" {
		if base, err = os.ReadFile(p); err != nil {
			return nil, err
		}
	}
	size, ok := plannerPapers[strings.ToLower(o.Paper)]
	if !ok {
		return nil, fmt.Errorf("paper %q: a4 or letter", o.Paper)
	}
	w, h := size[0], size[1]
	orient := "portrait"
	if o.View != "day" {
		w, h, orient = h, w, "landscape"
	}
	sheets, err := plannerSheets(o)
	if err != nil {
		return nil, err
	}
	tpl, err := template.New(filepath.Base(tplPath)).Funcs(plannerFuncs).ParseFiles(tplPath)
	if err != nil {
		return nil, err
	}
	title := "Planner"
	if len(sheets) > 0 {
		title = sheets[0].Title
		if o.View == "week" {
			title = sheets[0].Subtitle + " · " + sheets[0].Title
		}
	}
	page := plannerPage{
		Title: title, View: o.View, Theme: filepath.Base(o.Theme),
		Paper: strings.ToUpper(o.Paper[:1]) + o.Paper[1:] + " " + orient,
		// A hair under the page, or Chrome's rounding starts a blank one.
		SheetW:    fmt.Sprintf("%.1fmm", w-2*plannerMargin-0.5),
		SheetH:    fmt.Sprintf("%.1fmm", h-2*plannerMargin-0.5),
		Margin:    fmt.Sprintf("%.0fmm", plannerMargin),
		Base:      template.CSS(base),
		CSS:       template.CSS(css),
		Sheets:    sheets,
		Generated: time.Now().Format("2 Jan 2006 15:04"),
	}
	if strings.EqualFold(o.Paper, "a4") {
		page.Paper = "A4 " + orient
	}
	sb := strings.Builder{}
	if err := tpl.Execute(&sb, page); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

/* SDOC: API
* GET /agenda/print — The Agenda as a Printable Planner

	A self-contained html page to print (or to hand a headless Chrome, which is
	what =orgs print -f pdf= does): a week on one landscape sheet, a day on a
	portrait one, or a month as a calendar. What lands on each day is the
	=/agenda= answer for the same query, so paper and screen agree.

	- *week*: a spread - four days on the left page, three and a panel on the
	  right. Each day lists its all day items (deadlines marked =due=, or =in
	  3d= while warning) and its timed ones, done items struck through, and
	  ruled lines to write on. The panel has *Overdue* (what the agenda carries
	  to today, late first), *Tasks* (undated open tasks, active ones first),
	  a *Habits* strip with a box a day and *Notes*.
	- *day*: an hour grid (07:00 to 21:00, wider when something is earlier or
	  later) with overlapping entries side by side, then the day's list, tasks,
	  overdue and notes.
	- *month*: a calendar grid, a few entries a day and how many more.

	*Query Parameters:*
	| Parameter   | Description                                                  |
	|-------------+--------------------------------------------------------------|
	| =view=      | =week= (default), =day= or =month=                           |
	| =from=      | a day in the first sheet, =YYYY-MM-DD=; today when empty     |
	| =pages=     | sheets: weeks, days or months (default 1)                    |
	| =days=      | instead of pages: days to cover (week view: rounded up to weeks) |
	| =query=     | narrows the headings, as =/agenda=                           |
	| =tasks=     | the tasks panel's query; =none= for no panel                 |
	| =habits=    | =f= leaves the habits strip off                              |
	| =theme=     | =classic= (default), =minimal=, =bullet=, =modern=, or yours |
	| =paper=     | =a4= (default) or =letter=                                   |
	| =weekstart= | =mon= (default) or =sun=                                     |
	| =today=     | the day to count overdue from; the server's date            |
	| =list=      | =t= answers the theme names, one a line                      |

	A theme is one stylesheet in =planner/themes/= on the template path; the
	layout is =planner/base.css= and the page =planner/planner.html=, and a copy
	of either in your template folder replaces it.

	*Response:* =text/html=, or a 400 with the reason as text.
EDOC */

// RequestAgendaPrint is GET /agenda/print (the SDOC above is its documentation;
// a blank line keeps gofmt from reflowing it as a doc comment).
func RequestAgendaPrint(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	bad := func(format string, args ...interface{}) {
		http.Error(w, fmt.Sprintf(format, args...), http.StatusBadRequest)
	}
	if q.Get("list") == "t" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, strings.Join(PlannerThemes(), "\n"))
		return
	}
	o := plannerOpts{View: strings.ToLower(q.Get("view")), Query: q.Get("query"), Tasks: q.Get("tasks"),
		Theme: q.Get("theme"), Paper: strings.ToLower(q.Get("paper")), Habits: q.Get("habits") != "f", Pages: 1}
	switch o.View {
	case "":
		o.View = "week"
	case "week", "day", "month":
	default:
		bad("view %q: week, day or month", o.View)
		return
	}
	if o.Paper == "" {
		o.Paper = "a4"
	}
	switch strings.ToLower(q.Get("weekstart")) {
	case "", "mon", "monday":
		o.WeekStart = time.Monday
	case "sun", "sunday":
		o.WeekStart = time.Sunday
	default:
		bad("weekstart %q: mon or sun", q.Get("weekstart"))
		return
	}
	now := time.Now()
	o.Today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if s := q.Get("today"); s != "" {
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			bad("today %q: YYYY-MM-DD", s)
			return
		}
		o.Today = t
	}
	o.From = o.Today
	if s := q.Get("from"); s != "" {
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			bad("from %q: YYYY-MM-DD", s)
			return
		}
		o.From = t
	}
	if s := q.Get("pages"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			bad("pages %q: a number from 1", s)
			return
		}
		o.Pages = n
	} else if s := q.Get("days"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			bad("days %q: a number from 1", s)
			return
		}
		switch o.View {
		case "week":
			o.Pages = (n + 6) / 7
		case "day":
			o.Pages = n
		case "month":
			o.Pages = (n + 30) / 31
		}
	}
	// A planner for the year is 53 sheets; more than that is a mistake.
	if o.Pages > 60 {
		bad("%d sheets asked for; at most 60 at a time", o.Pages)
		return
	}
	page, err := PlannerHTML(o)
	if err != nil {
		bad("%v", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

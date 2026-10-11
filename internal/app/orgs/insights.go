package orgs

// The Insights tab: two questions a plan cannot answer about itself.
//
// /insights/estimates is "how far off are my estimates": EFFORT against the
// clock, per tag, project, file or person, with the middle and the spread of
// the per-task misses and a multiplier to plan with.
//
// /insights/deps is "what in my dependencies is broken": chains held up by one
// task, things still WAITING on something finished, AFTER ids that name
// nothing, ORDERED projects stuck at a step, and cycles.
//
// Dependencies are resolved with the gantt's rules (plugs/mermaid's After): an
// AFTER names a heading by hash, ID or CUSTOM_ID, exactly as written; without
// one, the nearest ORDERED ancestor chains a heading behind its previous
// sibling; NOBLOCKING takes it out of that chain. They are re-implemented here
// over the outline rather than called, because After hands back one answer and
// a health check needs to know *why* - which link is AFTER and which ORDERED,
// which id was missing - and must keep walking past a cycle to report it,
// where the chart only needs to stop.

import (
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/mermaid"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

// registerInsightRoutes adds the endpoints for effort vs actual analytics and dependency health. Called from RestApi.
func registerInsightRoutes(api *mux.Router) {
	api.HandleFunc("/insights/estimates", RequestInsightEstimates).Methods("GET")
	api.HandleFunc("/insights/deps", RequestInsightDeps).Methods("GET")
}

// ---------------------------------------------------------------------------
// Small pieces shared by both
// ---------------------------------------------------------------------------

func insightProp(sec *org.Section, name string) string {
	if sec == nil || sec.Headline == nil || sec.Headline.Properties == nil {
		return ""
	}
	for _, p := range sec.Headline.Properties.Properties {
		if strings.EqualFold(p[0], name) {
			return strings.TrimSpace(p[1])
		}
	}
	return ""
}

func insightTitle(sec *org.Section) string {
	if sec == nil || sec.Headline == nil {
		return ""
	}
	t := strings.TrimSpace(common.GetSectionTitle(sec))
	if t == "" {
		return "(untitled)"
	}
	return t
}

func insightDone(sec *org.Section, f *common.OrgFile) bool {
	if sec == nil || sec.Headline == nil || sec.Headline.Status == "" {
		return false
	}
	_, done := ValidStatusFromFile(f)
	return contains(done, sec.Headline.Status)
}

func insightOpen(sec *org.Section, f *common.OrgFile) bool {
	return sec != nil && sec.Headline != nil && sec.Headline.Status != "" && !insightDone(sec, f)
}

// Keywords that say "this is held up by something", as opposed to "this is
// not started". A task in one of these whose dependency is finished has been
// forgotten.
var waitingKeywords = map[string]bool{
	"WAITING": true, "WAIT": true, "BLOCKED": true, "HOLD": true, "ON-HOLD": true,
	"ONHOLD": true, "DEFERRED": true, "PENDING": true,
}

func isWaitingKeyword(status string) bool {
	return waitingKeywords[strings.ToUpper(strings.TrimSpace(status))]
}

// Finished, but not because the work was done. Their clock says nothing about
// how long the work takes.
func isCancelledKeyword(status string) bool {
	switch strings.ToUpper(status) {
	case "CANCELLED", "CANCELED", "ABANDONED", "DROPPED", "KILL", "KILLED", "WONTDO", "WONT-DO":
		return true
	}
	return false
}

// The properties that say who a task is for, in the order worg believes them
// (ASSIGNEE_PROPS and personOf in worg's ganttplan.ts; keep the two in step).
var assigneeProps = []string{"ASSIGNEE", "ASSIGNED", "RID", "RESOURCEID"}

var personLinkRe = regexp.MustCompile(`\[[^\]]*\]`)

func assigneeOf(sec *org.Section) string {
	for _, p := range assigneeProps {
		v := personLinkRe.ReplaceAllString(insightProp(sec, p), "")
		v = strings.Join(strings.Fields(v), " ")
		if v != "" {
			return v
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Estimates: the arithmetic
// ---------------------------------------------------------------------------

var effortDayRe = regexp.MustCompile(`([0-9.]+)\s*(y|w|d)\b`)

// effortMins reads an EFFORT in minutes, with a day standing for dayHours
// hours. Org's own day is 24 hours (org-effort-durations), and so is
// common.ParseDuration's, which also makes a week five of them; a working day
// of 8 changes every d, w and y by the same factor. dayUnits says whether the
// text named any of them, which is when the choice matters.
func effortMins(text string, dayHours float64) (mins float64, dayUnits bool, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false, false
	}
	d := common.ParseDuration(text)
	if d == nil || d.Mins <= 0 {
		return 0, false, false
	}
	mins = d.Mins
	dayEq := 0.0
	for _, m := range effortDayRe.FindAllStringSubmatch(strings.ToLower(text), -1) {
		x, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		switch m[2] {
		case "y":
			dayEq += x * 365
		case "w":
			dayEq += x * 5
		case "d":
			dayEq += x
		}
	}
	if dayEq > 0 {
		dayUnits = true
		if dayHours > 0 && dayHours != 24 {
			mins += dayEq * (dayHours - 24) * 60
		}
	}
	return mins, dayUnits, mins > 0
}

// quantile of sorted values, interpolating between neighbours (the usual
// "type 7" definition, so the median of an even count is the mean of the two
// middle values).
func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	pos := q * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	frac := pos - float64(lo)
	return sorted[lo] + (sorted[hi]-sorted[lo])*frac
}

// The middle and the middle half of a set of ratios.
func ratioStats(ratios []float64) (median, p25, p75 float64) {
	s := append([]float64(nil), ratios...)
	sort.Float64s(s)
	return quantile(s, 0.5), quantile(s, 0.25), quantile(s, 0.75)
}

// Fewer than this many measured tasks and a multiplier is noise.
const minTasksForMultiplier = 3

func round1(x float64) float64 { return math.Round(x*10) / 10 }

func timesStr(x float64) string {
	return strconv.FormatFloat(round1(x), 'f', -1, 64) + "×"
}

// The sentence a group's numbers come down to.
func estimateSentence(label string, g common.EstimateGroup) string {
	measured := g.Tasks
	if measured == 0 {
		return ""
	}
	if measured < minTasksForMultiplier {
		where := " in " + label
		if label == "" {
			where = ""
		}
		return fmt.Sprintf("Only %d task%s measured%s: too few to say.", measured, plural(measured), where)
	}
	m := round1(g.Median)
	in := " in " + label
	if label == "" {
		in = ""
	}
	var s string
	switch {
	case m >= 0.9 && m <= 1.1:
		s = fmt.Sprintf("Your estimates%s are about right (median %s).", in, timesStr(g.Median))
	case m > 1.1:
		s = fmt.Sprintf("Work%s takes %s as long as estimated: multiply estimates by %s.", in, timesStr(g.Median), strconv.FormatFloat(m, 'f', -1, 64))
	default:
		s = fmt.Sprintf("Work%s takes %s the estimate: estimates are padded.", in, timesStr(g.Median))
	}
	// A spread of more than three times between the quartiles means the
	// median is the middle of a scatter, not a habit.
	if g.P25 > 0 && g.P75/g.P25 > 3 {
		s += fmt.Sprintf(" They vary widely (%s to %s).", timesStr(g.P25), timesStr(g.P75))
	}
	return s
}

// How a group is named in a sentence.
func estimateLabel(g common.EstimateGroup) string {
	switch g.Kind {
	case "tag":
		if strings.HasPrefix(g.Name, "(") {
			return "untagged work"
		}
		return ":" + g.Name + ":"
	case "all":
		return ""
	case "person":
		return g.Name + "'s work"
	}
	return g.Name
}

// The size of a miss, in minutes either way. The worst lists are ranked by it
// rather than by ratio, because a five minute task that took thirty is six
// times over and matters to nobody's plan.
func missMins(t common.EstimateTask) float64 { return math.Abs(t.ActualMins - t.EstMins) }

func worstOf(tasks []common.EstimateTask, n int, keep func(common.EstimateTask) bool) []common.EstimateTask {
	out := []common.EstimateTask{}
	for _, t := range tasks {
		if keep == nil || keep(t) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return missMins(out[a]) > missMins(out[b]) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// summarize fills in a group's totals and statistics from its tasks.
func summarize(g common.EstimateGroup, tasks []common.EstimateTask) common.EstimateGroup {
	ratios := []float64{}
	for _, t := range tasks {
		g.Tasks++
		if t.InProgress {
			g.InProgress++
		}
		g.EstMins += t.EstMins
		g.ActualMins += t.ActualMins
		if t.Ratio > 0 {
			ratios = append(ratios, t.Ratio)
		}
	}
	if g.EstMins > 0 {
		g.Ratio = g.ActualMins / g.EstMins
	}
	g.Median, g.P25, g.P75 = ratioStats(ratios)
	if g.Tasks >= minTasksForMultiplier {
		g.Multiplier = round1(g.Median)
	}
	g.Sentence = estimateSentence(estimateLabel(g), g)
	g.Worst = worstOf(tasks, 3, func(t common.EstimateTask) bool { return t.Ratio > 0 })
	return g
}

// groupEstimates splits measured tasks by tag, project, file or person and
// summarizes each. A task with several tags is counted once in each, so tag
// rows do not add up to the total. Biggest by actual time first.
func groupEstimates(tasks []common.EstimateTask, by string) []common.EstimateGroup {
	type bucket struct {
		g     common.EstimateGroup
		tasks []common.EstimateTask
	}
	buckets := map[string]*bucket{}
	order := []string{}
	add := func(key string, g common.EstimateGroup, t common.EstimateTask) {
		b := buckets[key]
		if b == nil {
			b = &bucket{g: g}
			buckets[key] = b
			order = append(order, key)
		}
		b.tasks = append(b.tasks, t)
	}
	for _, t := range tasks {
		switch by {
		case "tag":
			if len(t.Tags) == 0 {
				add("", common.EstimateGroup{Name: "(untagged)", Kind: "tag"}, t)
			}
			for _, tag := range t.Tags {
				add(tag, common.EstimateGroup{Name: tag, Kind: "tag"}, t)
			}
		case "file":
			add(t.Filename, common.EstimateGroup{Name: filepath.Base(t.Filename), Kind: "file", Filename: t.Filename}, t)
		case "person":
			who := t.Person
			if who == "" {
				who = "(nobody)"
			}
			add(who, common.EstimateGroup{Name: who, Kind: "person"}, t)
		default:
			add(t.Project, common.EstimateGroup{Name: t.Project, Kind: "project"}, t)
		}
	}
	out := []common.EstimateGroup{}
	for _, k := range order {
		out = append(out, summarize(buckets[k].g, buckets[k].tasks))
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].ActualMins > out[b].ActualMins })
	return out
}

// ---------------------------------------------------------------------------
// Estimates: reading the files
// ---------------------------------------------------------------------------

type estimateArgs struct {
	Query    string
	By       string
	Rng      clockRange
	DayHours float64
	Open     bool
	Now      time.Time
}

// The time span asked for: a clock-table block (thismonth, lastweek,
// 2026-Q3, ...) and/or from and to, each a date as typed anywhere else
// (2026-09-01, -30d, mon). To is inclusive: to=2026-09-30 counts that day.
func insightRange(block, from, to string, now time.Time) (clockRange, error) {
	r, err := clockBlockRange(block, now)
	if err != nil {
		return r, err
	}
	edge := func(name, v string) (time.Time, bool, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Time{}, false, nil
		}
		d, ok, err := orgdate.ParseDate(v, now)
		if err != nil || !ok {
			return time.Time{}, false, fmt.Errorf("%s %q is not a date", name, v)
		}
		return d.Day, true, nil
	}
	f, hasF, err := edge("from", from)
	if err != nil {
		return r, err
	}
	t, hasT, err := edge("to", to)
	if err != nil {
		return r, err
	}
	if hasF || hasT {
		if !r.Set {
			r = clockRange{Set: true, To: now.AddDate(100, 0, 0)}
		}
		if hasF {
			r.From = f
		}
		if hasT {
			r.To = t.AddDate(0, 0, 1)
		}
		switch {
		case hasF && hasT:
			r.Label = f.Format("2006-01-02") + " to " + t.Format("2006-01-02")
		case hasF:
			r.Label = "since " + f.Format("2006-01-02")
		default:
			r.Label = "until " + t.Format("2006-01-02")
		}
	}
	if r.Label == "" {
		r.Label = "all time"
	}
	return r, nil
}

func inRange(r clockRange, t time.Time) bool {
	if !r.Set {
		return true
	}
	return !t.Before(r.From) && t.Before(r.To)
}

// When a task was finished: its CLOSED, else the end of its last clock.
func finishedAt(h *org.Headline) (time.Time, bool) {
	if h.Closed != nil && h.Closed.Date != nil && !h.Closed.Date.Start.IsZero() {
		return h.Closed.Date.Start, true
	}
	var last time.Time
	for _, c := range h.Clocks {
		if c == nil || c.Date == nil {
			continue
		}
		end := c.Date.End
		if end.IsZero() {
			end = c.Date.Start
		}
		if end.After(last) {
			last = end
		}
	}
	return last, !last.IsZero()
}

// Clocked minutes of a heading and of every heading under it that is not
// estimated in its own right. A subtask with its own EFFORT is measured on its
// own and so is not this heading's time as well.
func unestimatedSubtreeMins(sec *org.Section, now time.Time) float64 {
	total := headlineClockMins(sec.Headline, clockRange{}, now)
	for _, c := range sec.Children {
		if c.Headline == nil || insightProp(c, "EFFORT") != "" {
			continue
		}
		total += unestimatedSubtreeMins(c, now)
	}
	return total
}

func hasEstimatedDescendant(sec *org.Section) bool {
	for _, c := range sec.Children {
		if insightProp(c, "EFFORT") != "" || hasEstimatedDescendant(c) {
			return true
		}
	}
	return false
}

// buildEstimates measures every task the query finds (every heading with a
// keyword when there is no query).
//
// Measured: a DONE task (finished in the range, by CLOSED or else its last
// clock) with both an EFFORT and clocked time; with Open, also an open task
// with both that was clocked in the range, flagged InProgress. Its actual is
// every clock it has - not only those in the range, since the estimate was
// for the whole task - plus the clocks of headings under it that carry no
// EFFORT of their own. A heading with an EFFORT over other estimated headings
// is a rollup and is counted, not measured.
func buildEstimates(a estimateArgs) (common.EstimateReport, error) {
	res := common.EstimateReport{
		Ok: true, Query: a.Query, By: a.By, DayHours: a.DayHours, Label: a.Rng.Label,
		Groups: []common.EstimateGroup{}, Over: []common.EstimateTask{}, Under: []common.EstimateTask{},
		Unestimated: []common.EstimateTask{},
	}
	if a.Rng.Set {
		res.From = a.Rng.From.Format("2006-01-02")
		if a.Rng.To.Year() < a.Now.Year()+50 {
			res.To = a.Rng.To.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	registerAllSections()

	var match map[*org.Section]bool
	if strings.TrimSpace(a.Query) != "" {
		match = map[*org.Section]bool{}
		err := queryEachSection(&common.StringQuery{Query: a.Query}, func(v *org.Section, f *common.OrgFile) {
			match[v] = true
		})
		if err != nil {
			return res, err
		}
	}

	tasks := []common.EstimateTask{}
	unest := []common.EstimateTask{}
	var walk func(s *org.Section, f *common.OrgFile, project *org.Section, estimatedAbove bool)
	walk = func(s *org.Section, f *common.OrgFile, project *org.Section, estimatedAbove bool) {
		if s.Headline == nil {
			return
		}
		h := s.Headline
		effort := insightProp(s, "EFFORT")
		inQuery := match == nil || match[s]
		if inQuery && h.Status != "" && !isCancelledKeyword(h.Status) {
			done := insightDone(s, f)
			t := common.EstimateTask{
				Hash: s.Hash, Headline: insightTitle(s), Filename: f.Filename, LineNum: h.Pos.Row,
				Status: h.Status, Done: done, Effort: effort, Person: assigneeOf(s),
			}
			t.Tags = []string{}
			for _, tag := range inheritedTags(s) {
				if strings.EqualFold(tag, "archive") || (Conf().Server.UseTagForProjects && strings.EqualFold(tag, "project")) {
					continue
				}
				t.Tags = append(t.Tags, tag)
			}
			if project != nil {
				t.Project = insightTitle(project)
			} else {
				t.Project = filepath.Base(f.Filename) + " (no project)"
			}
			if h.Closed != nil && h.Closed.Date != nil && !h.Closed.Date.Start.IsZero() {
				t.Closed = h.Closed.Date.Start.Format("2006-01-02")
			}
			selected := false
			if done {
				if at, ok := finishedAt(h); ok {
					selected = inRange(a.Rng, at)
				} else {
					selected = !a.Rng.Set
				}
			} else {
				// Open: clocked in the range (a zero range covers everything).
				selected = headlineClockMins(h, a.Rng, a.Now) > 0
			}
			switch {
			case !selected:
			case effort != "":
				est, dayUnits, ok := effortMins(effort, a.DayHours)
				if !ok {
					break
				}
				if hasEstimatedDescendant(s) {
					res.Rollups++
					break
				}
				t.EstMins = est
				t.ActualMins = unestimatedSubtreeMins(s, a.Now)
				if t.ActualMins <= 0 {
					if done {
						res.EffortNoClock++
					}
					break
				}
				if !done && !a.Open {
					break
				}
				t.InProgress = !done
				t.Ratio = t.ActualMins / t.EstMins
				if dayUnits {
					res.DayEfforts++
				}
				tasks = append(tasks, t)
			case !estimatedAbove && !hasEstimatedDescendant(s):
				// Time with no estimate anywhere above or below it.
				mins := headlineClockMins(h, a.Rng, a.Now)
				if mins > 0 {
					res.ClockNoEffort++
					res.ClockNoEffortMins += mins
					t.ActualMins = mins
					unest = append(unest, t)
				}
			}
		}
		if IsProject(s, f) {
			project = s
		}
		for _, c := range s.Children {
			walk(c, f, project, estimatedAbove || effort != "")
		}
	}
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, s := range f.Doc.Outline.Children {
			walk(s, f, nil, false)
		}
	}

	res.All = summarize(common.EstimateGroup{Name: "all", Kind: "all"}, tasks)
	res.Groups = groupEstimates(tasks, a.By)
	res.Over = worstOf(tasks, 10, func(t common.EstimateTask) bool { return t.ActualMins > t.EstMins })
	res.Under = worstOf(tasks, 10, func(t common.EstimateTask) bool { return t.ActualMins < t.EstMins })
	sort.SliceStable(unest, func(x, y int) bool { return unest[x].ActualMins > unest[y].ActualMins })
	if len(unest) > 10 {
		unest = unest[:10]
	}
	res.Unestimated = unest
	return res, nil
}

/* SDOC: API
* GET /insights/estimates — How Far Off Are the Estimates
	EFFORT against the clock. Every finished task with both an EFFORT and
	clocked time is measured: its actual (every clock it has, plus the clocks
	of headings under it with no EFFORT of their own) over its estimate. The
	tasks are then grouped, and each group gets its totals, the median and the
	middle half (25th to 75th percentile) of the per-task ratios, its worst
	misses, and a sentence: "Work in :backend: takes 1.6× as long as estimated:
	multiply estimates by 1.6." The multiplier is the median rounded to a
	tenth, and is only given for three tasks or more.

	A day of EFFORT is 24 hours unless =day= says otherwise - org's own
	reading, and what the weekly review and the column view use. Somebody who
	writes =1d= for a working day passes =day=8=; =DayEfforts= says how many
	estimates are written in days, which is when it matters. A week is five
	days, as =common.ParseDuration= has it.

	What cannot be measured is counted, not dropped: =ClockNoEffort= (a task
	with time on it and no estimate above or below it, with the ten biggest in
	=Unestimated=), =EffortNoClock= (finished with an estimate and no clock)
	and =Rollups= (an EFFORT over other estimated headings - a sum, which
	would count the same work twice). Cancelled tasks are never measured.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                                       |
	|-----------+--------+----------+-------------------------------------------------------------------|
	| =query=   | string | no       | Only the tasks this finds. Default: every heading with a keyword. |
	| =by=      | string | no       | =tag= (default), =project=, =file= or =person= (ASSIGNEE...).    |
	| =block=   | string | no       | A clock table block: =thismonth=, =lastweek=, =2026-Q3=...        |
	| =from=    | string | no       | First day, as typed anywhere (=2026-09-01=, =-90d=).              |
	| =to=      | string | no       | Last day, inclusive.                                              |
	| =day=     | number | no       | Hours in a day of EFFORT. Default 24.                             |
	| =open=    | bool   | no       | =t= also measures open tasks clocked in the range, flagged.       |

	A finished task is in the range when its CLOSED is (or, with no CLOSED,
	the end of its last clock); an open one when it was clocked in it.

	*Response:* An =EstimateReport=: =All= (every measured task as one group),
	=Groups= (biggest by actual time first), =Over= and =Under= (the ten worst
	misses each way, by minutes), the counts above, =DayHours=, =From=, =To=,
	=Label=. Each group: =Name=, =Kind=, =Tasks=, =InProgress=, =EstMins=,
	=ActualMins=, =Ratio= (total over total), =Median=, =P25=, =P75=,
	=Multiplier=, =Sentence=, =Worst=. Each task carries its =Hash=, file and
	line.
	EDOC */
func RequestInsightEstimates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now()
	by := strings.ToLower(strings.TrimSpace(q.Get("by")))
	switch by {
	case "":
		by = "tag"
	case "tag", "tags":
		by = "tag"
	case "project", "projects":
		by = "project"
	case "file", "files":
		by = "file"
	case "person", "people", "assignee", "who":
		by = "person"
	default:
		answerErr(w, fmt.Errorf("by=%q: group by tag, project, file or person", by))
		return
	}
	rng, err := insightRange(q.Get("block"), q.Get("from"), q.Get("to"), now)
	if err != nil {
		answerErr(w, err)
		return
	}
	day := 24.0
	if s := strings.TrimSpace(q.Get("day")); s != "" {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "h"), 64)
		if err != nil || v <= 0 || v > 24 {
			answerErr(w, fmt.Errorf("day=%q: hours in a day of effort, 1 to 24", s))
			return
		}
		day = v
	}
	open := false
	switch strings.ToLower(strings.TrimSpace(q.Get("open"))) {
	case "t", "true", "1", "yes":
		open = true
	}
	res, err := buildEstimates(estimateArgs{Query: q.Get("query"), By: by, Rng: rng, DayHours: day, Open: open, Now: now})
	if err != nil {
		answerErr(w, err)
		return
	}
	reviewJson(w, res)
}

// ---------------------------------------------------------------------------
// Dependencies
// ---------------------------------------------------------------------------

type depNode struct {
	sec      *org.Section
	f        *common.OrgFile
	parent   *depNode
	kids     []*depNode
	index    int // among its parent's kids
	open     bool
	done     bool
	waiting  bool
	archived bool
	after    string
	noBlock  string
	ordered  bool
}

type depEdge struct {
	to      *depNode
	via     string // AFTER, ORDERED
	missing string // an AFTER naming nothing
	// A missing AFTER that would have resolved spelled differently ("id:x").
	hint string
}

type depGraph struct {
	nodes []*depNode
	byKey map[string]*depNode
	edges map[*depNode]depEdge
}

func (n *depNode) heading() common.DepHeading {
	return common.DepHeading{
		Hash: n.sec.Hash, Headline: insightTitle(n.sec), Filename: n.f.Filename,
		LineNum: n.sec.Headline.Pos.Row, Status: n.sec.Headline.Status, Done: n.done,
		After: n.after, NoBlocking: n.noBlock,
	}
}

// buildDepGraph reads every heading of every file into nodes with outline
// links of their own, so that no walk depends on the hash of a parent (an
// untitled heading once shared its parent's and made it its own parent).
func buildDepGraph() *depGraph {
	registerAllSections()
	g := &depGraph{byKey: map[string]*depNode{}, edges: map[*depNode]depEdge{}}
	var walk func(secs []*org.Section, f *common.OrgFile, parent *depNode, archived bool)
	walk = func(secs []*org.Section, f *common.OrgFile, parent *depNode, archived bool) {
		for _, s := range secs {
			if s.Headline == nil {
				continue
			}
			n := &depNode{sec: s, f: f, parent: parent}
			if parent != nil {
				n.index = len(parent.kids)
			}
			n.archived = archived || ownArchiveTag(s)
			n.open = insightOpen(s, f)
			n.done = insightDone(s, f)
			n.waiting = n.open && isWaitingKeyword(s.Headline.Status)
			n.after = insightProp(s, "AFTER")
			n.noBlock = insightProp(s, "NOBLOCKING")
			_, n.ordered = insightProps(s)["ORDERED"]
			if parent != nil {
				parent.kids = append(parent.kids, n)
			}
			g.nodes = append(g.nodes, n)
			for _, key := range []string{s.Hash, insightProp(s, "ID"), insightProp(s, "CUSTOM_ID")} {
				if key != "" {
					if _, taken := g.byKey[key]; !taken {
						g.byKey[key] = n
					}
				}
			}
			walk(s.Children, f, n, n.archived)
		}
	}
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		walk(f.Doc.Outline.Children, f, nil, false)
	}
	for _, n := range g.nodes {
		g.edges[n] = g.resolve(n, map[*depNode]bool{})
	}
	return g
}

// A heading's own ARCHIVE tag, not an inherited one: the walk carries the
// inheritance itself.
func ownArchiveTag(s *org.Section) bool {
	for _, t := range s.Headline.Tags {
		if strings.EqualFold(t, "archive") || strings.EqualFold(t, "archived") {
			return true
		}
	}
	return false
}

func insightProps(s *org.Section) map[string]string {
	out := map[string]string{}
	if s.Headline != nil && s.Headline.Properties != nil {
		for _, p := range s.Headline.Properties.Properties {
			out[strings.ToUpper(p[0])] = p[1]
		}
	}
	return out
}

func noBlocking(n *depNode) bool {
	return mermaid.NoBlocking(&common.Todo{Props: map[string]string{"NOBLOCKING": n.noBlock}})
}

// resolve is what n waits on, by the gantt's rules. seen guards the walk up
// through ORDERED first children, which asks the parent what *it* waits on.
func (g *depGraph) resolve(n *depNode, seen map[*depNode]bool) depEdge {
	if seen[n] {
		return depEdge{}
	}
	seen[n] = true
	if n.after != "" {
		if to := g.byKey[n.after]; to != nil {
			return depEdge{to: to, via: "AFTER"}
		}
		e := depEdge{via: "AFTER", missing: n.after}
		bare := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(n.after, "id:"), "#"))
		if bare != n.after && g.byKey[bare] != nil {
			e.hint = fmt.Sprintf("write AFTER as %q: the gantt looks the value up exactly as written", bare)
		} else if len(strings.Fields(n.after)) > 1 {
			e.hint = "AFTER holds one id; the gantt reads the whole value as a single id"
		}
		return e
	}
	return g.chained(n, seen)
}

// What an ORDERED ancestor chains n behind, AFTER aside: the previous sibling
// of the child of the nearest ORDERED ancestor on the way up, or, for the
// first child, whatever that ancestor itself waits on.
func (g *depGraph) chained(n *depNode, seen map[*depNode]bool) depEdge {
	if noBlocking(n) {
		return depEdge{}
	}
	for cur := n; cur.parent != nil; cur = cur.parent {
		par := cur.parent
		if !par.ordered {
			continue
		}
		if cur.index > 0 && cur.index <= len(par.kids) {
			return depEdge{to: par.kids[cur.index-1], via: "ORDERED"}
		}
		e := g.resolve(par, seen)
		if e.to != nil || e.missing != "" {
			e.via = "ORDERED"
		}
		return e
	}
	return depEdge{}
}

func (g *depGraph) isAncestor(a, n *depNode) bool {
	for p := n.parent; p != nil; p = p.parent {
		if p == a {
			return true
		}
	}
	return false
}

// The heading with the via and the fix fields set for the link it waits by.
func (g *depGraph) link(n *depNode) common.DepHeading {
	h := n.heading()
	if e, ok := g.edges[n]; ok {
		h.Via = e.via
	}
	h.Chained = g.chainedIgnoringAfter(n)
	return h
}

// Whether an ORDERED parent would still chain n once its AFTER is cleared -
// what worg's unlinkOp needs to know to take a wait away for good.
func (g *depGraph) chainedIgnoringAfter(n *depNode) bool {
	for cur := n; cur.parent != nil; cur = cur.parent {
		par := cur.parent
		if !par.ordered {
			continue
		}
		if cur.index > 0 {
			return true
		}
		e := g.resolve(par, map[*depNode]bool{})
		return e.to != nil
	}
	return false
}

// Headings given in work order, as a chain: the first waits on nothing in it,
// so its Via is left empty.
func (g *depGraph) chain(ns ...*depNode) []common.DepHeading {
	out := make([]common.DepHeading, 0, len(ns))
	for _, n := range ns {
		out = append(out, g.link(n))
	}
	if len(out) > 0 {
		out[0].Via = ""
	}
	return out
}

// A path of waits (each waiting on the next) turned into work order.
func (g *depGraph) chainOf(path []*depNode) []common.DepHeading {
	rev := make([]*depNode, 0, len(path))
	for i := len(path) - 1; i >= 0; i-- {
		rev = append(rev, path[i])
	}
	return g.chain(rev...)
}

type depArgs struct {
	Query    string
	MinChain int
}

var severityRank = map[string]int{"error": 0, "warn": 1, "info": 2}

// analyzeDeps finds every problem in the graph. scope, when not nil, keeps a
// problem only if one of the headings in it is in scope.
func analyzeDeps(g *depGraph, scope map[*depNode]bool, minChain int) common.DepReport {
	res := common.DepReport{Ok: true, Problems: []common.DepProblem{}, Counts: map[string]int{}}
	if minChain < 2 {
		minChain = 3
	}
	inScope := func(ns ...*depNode) bool {
		if scope == nil {
			return true
		}
		for _, n := range ns {
			if scope[n] {
				return true
			}
		}
		return false
	}
	add := func(p common.DepProblem, ns ...*depNode) {
		if !inScope(ns...) {
			return
		}
		res.Problems = append(res.Problems, p)
		res.Counts[p.Kind]++
	}
	for _, n := range g.nodes {
		if n.open || n.after != "" || n.ordered {
			res.Checked++
		}
	}

	// Cycles. Every heading waits on at most one other, so following the links
	// from each heading either ends or comes back to a heading already on the
	// path; colouring the headings (on the path now, finished) visits each once
	// and finds each cycle exactly once, from whichever heading reaches it first.
	state := map[*depNode]int{}
	inCycle := map[*depNode]bool{}
	for _, start := range g.nodes {
		if state[start] != 0 {
			continue
		}
		path := []*depNode{}
		at := map[*depNode]int{}
		for n := start; n != nil; n = g.edges[n].to {
			if state[n] == 2 {
				break
			}
			if state[n] == 1 {
				loop := path[at[n]:]
				for _, m := range loop {
					inCycle[m] = true
				}
				// Work order, back round to where it started; the repeat at
				// the end carries the link that closes the circle.
				chain := g.chainOf(loop)
				chain = append(chain, g.link(loop[len(loop)-1]))
				sev := "warn"
				for _, m := range loop {
					if m.open {
						sev = "error"
					}
				}
				title := fmt.Sprintf("%d headings wait on each other in a circle", len(loop))
				if len(loop) == 1 {
					title = fmt.Sprintf("%s waits on itself", insightTitle(loop[0].sec))
				}
				add(common.DepProblem{
					Kind: "cycle", Severity: sev, Title: title, Chain: chain, Length: len(loop),
					Suggestion: "None of them can ever start. Remove one of the links (an AFTER, or NOBLOCKING out of an ORDERED chain).",
				}, loop...)
				break
			}
			state[n] = 1
			at[n] = len(path)
			path = append(path, n)
		}
		for _, n := range path {
			state[n] = 2
		}
	}

	for _, n := range g.nodes {
		e := g.edges[n]
		// Waiting on its own ancestor: not a loop of links, but a deadlock all
		// the same, since a parent is not finished until its children are.
		if e.to != nil && e.via == "AFTER" && n.open && e.to.open && g.isAncestor(e.to, n) {
			add(common.DepProblem{
				Kind: "cycle", Severity: "error",
				Title:      fmt.Sprintf("%s comes AFTER %s, which it is part of", insightTitle(n.sec), insightTitle(e.to.sec)),
				Chain:      g.chain(e.to, n),
				Length:     2,
				Suggestion: "A heading cannot finish before the subtree under it does. Clear the AFTER, or point it at a sibling.",
			}, n, e.to)
		}
		// An outline loop: a heading with its parent's hash. Anything that
		// walks up the outline by hash goes round for ever.
		if n.parent != nil && n.sec.Hash != "" && n.sec.Hash == n.parent.sec.Hash {
			add(common.DepProblem{
				Kind: "cycle", Severity: "error",
				Title:      fmt.Sprintf("%s has the same hash as its parent %s", insightTitle(n.sec), insightTitle(n.parent.sec)),
				Chain:      []common.DepHeading{n.parent.heading(), n.heading()},
				Length:     2,
				Suggestion: "Give the heading a title: an untitled heading used to take its parent's hash, and anything following the outline by hash loops.",
			}, n)
		}
		// Dangling and archived AFTERs.
		if e.missing != "" && e.via == "AFTER" && n.after != "" && !n.archived {
			sev := "info"
			if n.open {
				sev = "warn"
			}
			sug := "Clear the AFTER, or point it at the heading meant: no heading has that ID, CUSTOM_ID or hash."
			if e.hint != "" {
				sug = strings.ToUpper(e.hint[:1]) + e.hint[1:] + "."
			}
			add(common.DepProblem{
				Kind: "dangling", Severity: sev, Missing: e.missing,
				Title:      fmt.Sprintf("%s comes AFTER %q, which names no heading", insightTitle(n.sec), e.missing),
				Chain:      []common.DepHeading{g.link(n)},
				Suggestion: sug,
			}, n)
		} else if e.to != nil && e.via == "AFTER" && e.to.archived && !n.archived {
			sev := "info"
			if n.open && e.to.open {
				sev = "warn"
			}
			add(common.DepProblem{
				Kind: "dangling", Severity: sev, Missing: n.after,
				Title:      fmt.Sprintf("%s comes AFTER %s, which is archived", insightTitle(n.sec), insightTitle(e.to.sec)),
				Chain:      g.chain(e.to, n),
				Suggestion: "Archiving to another file will break this link. Clear the AFTER if the wait is over.",
			}, n, e.to)
		}
		// Still WAITING on something finished. Only through an AFTER: a WAITING
		// step of an ORDERED project is usually waiting on somebody outside,
		// not on the step before it, and ordered-stuck reports it as stuck.
		if n.waiting && e.to != nil && e.via == "AFTER" && e.to.done && !n.archived {
			add(common.DepProblem{
				Kind: "waiting-on-done", Severity: "warn",
				Title: fmt.Sprintf("%s is %s, but %s is %s", insightTitle(n.sec), n.sec.Headline.Status,
					insightTitle(e.to.sec), e.to.sec.Headline.Status),
				Chain:      g.chain(e.to, n),
				Suggestion: fmt.Sprintf("What it waited on is finished: make %s TODO or NEXT.", insightTitle(n.sec)),
			}, n, e.to)
		}
	}

	// ORDERED projects stuck at a step. Done before the blocked chains, which
	// leave out a short chain these already explain.
	stuckStep := map[*depNode]bool{}
	for _, p := range g.nodes {
		if !p.ordered || p.archived || p.done {
			continue
		}
		steps := []*depNode{}
		for _, k := range p.kids {
			if k.sec.Headline.Status != "" && !noBlocking(k) {
				steps = append(steps, k)
			}
		}
		cur := -1
		for i, k := range steps {
			if k.open {
				cur = i
				break
			}
		}
		if cur < 0 {
			continue
		}
		step := steps[cur]
		if step.waiting {
			stuckStep[step] = true
			// The step, and the open steps held behind it.
			held := []*depNode{step}
			for _, k := range steps[cur+1:] {
				if k.open {
					held = append(held, k)
				}
			}
			add(common.DepProblem{
				Kind: "ordered-stuck", Severity: "warn",
				Title: fmt.Sprintf("%s is stuck at step %d of %d: %s is %s", insightTitle(p.sec), cur+1, len(steps),
					insightTitle(step.sec), step.sec.Headline.Status),
				Chain:      g.chain(held...),
				Length:     len(held),
				Suggestion: "Nothing after this step can start. Chase it, or mark the steps that do not need it NOBLOCKING.",
			}, p, step)
		}
		for _, later := range steps[cur+1:] {
			if later.done {
				add(common.DepProblem{
					Kind: "ordered-stuck", Severity: "info",
					Title: fmt.Sprintf("In %s, %s is %s ahead of %s", insightTitle(p.sec), insightTitle(later.sec),
						later.sec.Headline.Status, insightTitle(step.sec)),
					Chain:      g.chain(step, later),
					Suggestion: "The ORDERED sequence and the work disagree. Move the step up, or mark it NOBLOCKING.",
				}, p, later)
				break
			}
		}
	}

	// Blocked chains. A heading is blocked when what it waits on is open; its
	// root blocker is the first heading down the links that is not. Chains are
	// reported per root: the longest path into it and how many it holds up.
	blockedBy := func(n *depNode) *depNode {
		e := g.edges[n]
		if e.to != nil && e.to.open && !inCycle[n] && !inCycle[e.to] && !g.isAncestor(e.to, n) {
			return e.to
		}
		return nil
	}
	type rootInfo struct {
		held    int
		longest []*depNode
		members []*depNode
	}
	roots := map[*depNode]*rootInfo{}
	rootOrder := []*depNode{}
	for _, n := range g.nodes {
		if !n.open || n.archived || blockedBy(n) == nil {
			continue
		}
		path := []*depNode{n}
		seen := map[*depNode]bool{n: true}
		cur := n
		for {
			nx := blockedBy(cur)
			if nx == nil || seen[nx] {
				break
			}
			seen[nx] = true
			path = append(path, nx)
			cur = nx
		}
		root := path[len(path)-1]
		ri := roots[root]
		if ri == nil {
			ri = &rootInfo{}
			roots[root] = ri
			rootOrder = append(rootOrder, root)
		}
		ri.held++
		ri.members = append(ri.members, n)
		if len(path) > len(ri.longest) {
			ri.longest = path
		}
	}
	for _, root := range rootOrder {
		ri := roots[root]
		length := len(ri.longest)
		if length < minChain && !root.waiting {
			continue
		}
		// A WAITING step of an ORDERED project holding up only the steps
		// after it is the ordered-stuck problem already reported.
		if stuckStep[root] {
			siblings := true
			for _, m := range ri.members {
				if m.parent != root.parent {
					siblings = false
				}
			}
			if siblings {
				continue
			}
		}
		sev := "info"
		if root.waiting {
			sev = "warn"
		}
		title := fmt.Sprintf("%d heading%s held up behind %s", ri.held, plural(ri.held), insightTitle(root.sec))
		sug := fmt.Sprintf("Everything here waits on %s. Finishing it, or splitting the chain, frees %d heading%s.", insightTitle(root.sec), ri.held, plural(ri.held))
		if root.waiting {
			sug = fmt.Sprintf("The whole chain is stuck on %s, which is %s: chase it, or take the chain off it.", insightTitle(root.sec), root.sec.Headline.Status)
		}
		add(common.DepProblem{
			Kind: "blocked-chain", Severity: sev, Title: title, Suggestion: sug,
			Chain: g.chainOf(ri.longest), Length: length, Held: ri.held,
		}, append(ri.members, root)...)
	}

	sort.SliceStable(res.Problems, func(a, b int) bool {
		pa, pb := res.Problems[a], res.Problems[b]
		if severityRank[pa.Severity] != severityRank[pb.Severity] {
			return severityRank[pa.Severity] < severityRank[pb.Severity]
		}
		return pa.Length > pb.Length
	})
	return res
}

func buildDeps(a depArgs) (common.DepReport, error) {
	g := buildDepGraph()
	var scope map[*depNode]bool
	if strings.TrimSpace(a.Query) != "" {
		bySec := map[*org.Section]*depNode{}
		for _, n := range g.nodes {
			bySec[n.sec] = n
		}
		scope = map[*depNode]bool{}
		err := queryEachSection(&common.StringQuery{Query: a.Query}, func(v *org.Section, f *common.OrgFile) {
			if n := bySec[v]; n != nil {
				scope[n] = true
			}
		})
		if err != nil {
			return common.DepReport{Ok: false, Msg: err.Error(), Query: a.Query}, err
		}
	}
	res := analyzeDeps(g, scope, a.MinChain)
	res.Query = a.Query
	return res, nil
}

/* SDOC: API
* GET /insights/deps — Dependency Health
	What is wrong with the AFTER and ORDERED links between headings, read with
	the gantt's own rules (an AFTER names a heading by hash, ID or CUSTOM_ID as
	written; an ORDERED parent chains each child behind the one before;
	NOBLOCKING takes a child out). Each problem is one of:

	| Kind              | What it means                                                                 |
	|-------------------+-------------------------------------------------------------------------------|
	| =cycle=           | Headings that wait on each other in a circle (each reported once), a heading  |
	|                   | AFTER its own ancestor, or one sharing its parent's hash.                     |
	| =waiting-on-done= | WAITING (or BLOCKED, HOLD...) on a heading that is finished.                 |
	| =blocked-chain=   | Open headings held up behind one open root blocker: reported when the longest |
	|                   | chain is =min= long or more, or when the root is itself WAITING.             |
	| =dangling=        | An AFTER naming no heading, or an archived one.                               |
	| =ordered-stuck=   | An ORDERED project whose current step is WAITING, or with a later step done   |
	|                   | ahead of it.                                                                  |

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                                     |
	|-----------+--------+----------+-----------------------------------------------------------------|
	| =query=   | string | no       | Only problems involving a heading this finds. Links are always |
	|           |        |          | followed through the whole database.                            |
	| =min=     | int    | no       | Shortest blocked chain worth reporting. Default 3.              |

	*Response:* A =DepReport=: =Problems= (errors first, then warnings, then
	notes), =Counts= per kind and =Checked=. Each problem has =Kind=,
	=Severity=, =Title=, =Suggestion=, =Length=, =Held= (blocked chains),
	=Missing= (dangling) and =Chain=: the headings in work order, what is
	waited on first (a cycle ends where it began). Each heading carries its
	hash, file, line, keyword, =Via= (=AFTER= or =ORDERED=: how it waits on the
	one before) and =After=, =NoBlocking= and =Chained=, which are what a client
	needs to take that wait away.
	EDOC */
func RequestInsightDeps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	min := 3
	if s := strings.TrimSpace(q.Get("min")); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 2 {
			answerErr(w, fmt.Errorf("min=%q: the shortest chain to report, 2 or more", s))
			return
		}
		min = v
	}
	res, err := buildDeps(depArgs{Query: q.Get("query"), MinChain: min})
	if err != nil {
		answerErr(w, err)
		return
	}
	reviewJson(w, res)
}

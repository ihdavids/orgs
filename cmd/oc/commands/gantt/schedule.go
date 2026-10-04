package gantt

// The plan's dates: when each heading /gantt/tasks hands back starts and ends.
//
// The server says what a heading knows about itself - a date, an EFFORT, what
// it comes AFTER - and leaves the arithmetic to whoever draws it, so this is a
// port of worg's resolveSchedule / ganttFromTasks (worg/src/gantt.ts). The two
// have to agree: a bar on a different day in the terminal than in the browser
// is a chart that is wrong in one of them, and nothing notices. The same
// scenarios are pinned in schedule_test.go and gantt.test.ts.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

const day = 24 * time.Hour

// DefaultSection is the lane a heading with nothing to say about one lands in,
// as worg names it.
const DefaultSection = "main"

type Task struct {
	// The heading's hash, which is what After refers to.
	ID      string
	Name    string
	Section string
	Start   time.Time
	End     time.Time
	// default, active, done or crit - crit wins over active wins over done,
	// the order mermaid paints them in.
	State     string
	Milestone bool
	Mark      bool
	// Calendar days the bar covers, and the EFFORT somebody estimated (zero
	// for none). Not the same whenever the bar steps over a weekend.
	Days        float64
	PlannedDays float64
	After       []string
	// False when the heading has no date of its own and sits where it does
	// only because of what it comes after.
	Dated bool
	Src   common.GanttTask
}

type Section struct {
	Name  string
	Tasks []*Task
}

type Model struct {
	Tasks    []*Task
	Sections []Section
	// The span the chart has to cover, widened to whole days.
	Start, End      time.Time
	ExcludeWeekends bool
	// Dependencies that point at nothing and chains that never resolve.
	// Shown rather than fatal: one bad heading should not cost the chart.
	Errors []string
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func addDays(t time.Time, n int) time.Time {
	return t.AddDate(0, 0, n)
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// Local time throughout: a bar is read against the calendar on the wall.
func parseDay(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, f := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func excluded(t time.Time, weekends bool) bool {
	wd := t.Weekday()
	return weekends && (wd == time.Saturday || wd == time.Sunday)
}

// addWorkingTime walks a duration forward over the days that count, so two
// days of work started on a Friday ends on Tuesday.
func addWorkingTime(start time.Time, d time.Duration, weekends bool) time.Time {
	if d <= 0 {
		return start
	}
	cur, left := start, d
	for guard := 0; guard < 20000 && left > 0; guard++ {
		next := addDays(startOfDay(cur), 1)
		if excluded(cur, weekends) {
			cur = next
			continue
		}
		take := next.Sub(cur)
		if left < take {
			take = left
		}
		cur = cur.Add(take)
		left -= take
	}
	return cur
}

// A task that would begin on a day nobody works begins on the next one that
// somebody does. Only real spans: a milestone keeps its day.
func pushToWorkingDay(t time.Time, weekends bool) time.Time {
	for guard := 0; guard < 20000 && excluded(t, weekends); guard++ {
		t = addDays(startOfDay(t), 1)
	}
	return t
}

// The lane a heading with no value for the lane property lands in, named after
// the property so the chart says what is missing.
func noLane(prop string) string { return "(no " + prop + ")" }

func laneOf(r *common.GanttTask, prop string) string {
	if prop == "" {
		if r.Section != "" {
			return r.Section
		}
		return DefaultSection
	}
	if v := strings.TrimSpace(propOf(r.Props, prop)); v != "" {
		return v
	}
	return noLane(prop)
}

// Properties come back in whatever case the file wrote them.
func propOf(props map[string]string, name string) string {
	if v, ok := props[name]; ok {
		return v
	}
	for k, v := range props {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// sortServerTasks is the row order: lanes in the order the query walked into
// them (by value when split by a property, the empty lane last), GANTT_ORDER
// within a lane, then file order.
func sortServerTasks(rows []common.GanttTask, prop string) []common.GanttTask {
	names := []string{}
	seen := map[string]bool{}
	for i := range rows {
		n := laneOf(&rows[i], prop)
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if prop != "" {
		none := noLane(prop)
		sort.SliceStable(names, func(i, j int) bool {
			if names[i] == none || names[j] == none {
				return names[j] == none && names[i] != none
			}
			return naturalLess(names[i], names[j])
		})
	}
	lane := map[string]int{}
	for i, n := range names {
		lane[n] = i
	}
	out := append([]common.GanttTask{}, rows...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := &out[i], &out[j]
		la, lb := lane[laneOf(a, prop)], lane[laneOf(b, prop)]
		if la != lb {
			return la < lb
		}
		oa, ob := 1e300, 1e300
		if a.HasOrder {
			oa = a.Order
		}
		if b.HasOrder {
			ob = b.Order
		}
		if oa != ob {
			return oa < ob
		}
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		return a.LineNum < b.LineNum
	})
	return out
}

// naturalLess compares without regard to case and with runs of digits as
// numbers, which is what localeCompare's numeric option does: "sprint 9"
// before "sprint 10".
func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da != "" && db != "" {
			na, nb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

type raw struct {
	t         *Task
	startDate time.Time
	hasStart  bool
	endDate   time.Time
	hasEnd    bool
	duration  time.Duration // -1 for none
	resolved  bool
}

// Build lays the server's rows out on the calendar.
func Build(rows []common.GanttTask, now time.Time, excludeWeekends bool, laneProp string) *Model {
	laneProp = strings.TrimSpace(laneProp)
	known := map[string]bool{}
	for _, r := range rows {
		known[r.Hash] = true
	}
	errors := []string{}

	raws := []*raw{}
	byID := map[string]*raw{}
	for _, r := range sortServerTasks(rows, laneProp) {
		t := &Task{
			ID:        r.Hash,
			Name:      r.Headline,
			Section:   laneOf(&r, laneProp),
			Milestone: r.Milestone,
			Mark:      r.Mark,
			Dated:     r.Start != "",
			Src:       r,
		}
		// crit wins over active wins over done.
		t.State = "default"
		if r.Done {
			t.State = "done"
		}
		if r.Active {
			t.State = "active"
		}
		if r.Crit {
			t.State = "crit"
		}
		// A dependency the chart does not hold cannot be waited on, and saying
		// so is better than quietly starting the task today.
		if r.After != "" {
			if known[r.After] {
				t.After = []string{r.After}
			} else {
				errors = append(errors, fmt.Sprintf("%q comes after a heading that is not in this chart", r.Headline))
			}
		}
		x := &raw{t: t, duration: -1}
		x.startDate, x.hasStart = parseDay(r.Start)
		x.endDate, x.hasEnd = parseDay(r.End)
		if r.EffortDays > 0 {
			x.duration = time.Duration(r.EffortDays * float64(day))
		}
		raws = append(raws, x)
		byID[t.ID] = x
	}

	// Resolve until nothing moves: a task waits on what it comes after, and a
	// task with nothing to say about its start follows the one above it. What
	// is left after that is a cycle, and is placed rather than dropped.
	resolveOne := func(idx int) bool {
		r := raws[idx]
		var start time.Time
		have := false
		if r.hasStart {
			start, have = r.startDate, true
		} else if len(r.t.After) > 0 {
			for _, id := range r.t.After {
				dep := byID[id]
				if dep == nil {
					continue
				}
				if !dep.resolved {
					return false
				}
				if !have || dep.t.End.After(start) {
					start, have = dep.t.End, true
				}
			}
		}
		if !have {
			if idx > 0 {
				prev := raws[idx-1]
				if !prev.resolved {
					return false
				}
				start = prev.t.End
			} else {
				start = startOfDay(now)
			}
		}
		moment := r.t.Milestone || r.t.Mark
		if !moment {
			start = pushToWorkingDay(start, excludeWeekends)
		}
		var end time.Time
		switch {
		case moment:
			end = start
		case r.hasEnd && r.endDate.After(start):
			end = r.endDate
		default:
			d := r.duration
			if d < 0 {
				d = day
			}
			end = addWorkingTime(start, d, excludeWeekends)
		}
		r.t.Start, r.t.End, r.resolved = start, end, true
		return true
	}

	pending := []int{}
	for i := range raws {
		pending = append(pending, i)
	}
	for pass := 0; pass < len(raws)+1 && len(pending) > 0; pass++ {
		before := len(pending)
		left := pending[:0:0]
		for _, i := range pending {
			if !resolveOne(i) {
				left = append(left, i)
			}
		}
		pending = left
		if len(pending) == before {
			break
		}
	}
	for _, i := range pending {
		r := raws[i]
		errors = append(errors, fmt.Sprintf("%q depends on something that never resolves", r.t.Name))
		d := r.duration
		if d < 0 {
			d = day
		}
		r.t.Start = startOfDay(now)
		r.t.End = addWorkingTime(r.t.Start, d, excludeWeekends)
		r.resolved = true
	}

	m := &Model{ExcludeWeekends: excludeWeekends}
	for _, r := range raws {
		t := r.t
		if !t.Milestone && !t.Mark {
			t.Days = t.End.Sub(t.Start).Hours() / 24
			if r.duration >= 0 {
				t.PlannedDays = r.duration.Hours() / 24
			}
		}
		m.Tasks = append(m.Tasks, t)
	}
	for _, t := range m.Tasks {
		n := len(m.Sections)
		if n == 0 || m.Sections[n-1].Name != t.Section {
			m.Sections = append(m.Sections, Section{Name: t.Section})
			n++
		}
		m.Sections[n-1].Tasks = append(m.Sections[n-1].Tasks, t)
	}

	// A chart with nothing on it still needs an axis, and one whose last bar
	// ends at midnight gets a day of room after it.
	min, max := now, now
	for i, t := range m.Tasks {
		if i == 0 || t.Start.Before(min) {
			min = t.Start
		}
		if i == 0 || t.End.After(max) {
			max = t.End
		}
	}
	m.Start = startOfDay(min)
	m.End = addDays(startOfDay(max), 1)

	// A task that cannot resolve is looked at once per pass; it is one problem
	// and reads as one.
	seen := map[string]bool{}
	for _, e := range errors {
		if !seen[e] {
			seen[e] = true
			m.Errors = append(m.Errors, e)
		}
	}
	return m
}

// CriticalPath is the tasks with no room to move: a backward pass from the
// latest end, where whatever has less than half a day between when it must
// finish and when it does is critical.
func CriticalPath(m *Model) map[string]bool {
	out := map[string]bool{}
	lf := latestFinish(m)
	for _, t := range m.Tasks {
		if t.Mark || t.State == "done" || t.Src.Implied {
			continue
		}
		if v, ok := lf[t.ID]; ok && v.Sub(t.End) < day/2 {
			out[t.ID] = true
		}
	}
	return out
}

func latestFinish(m *Model) map[string]time.Time {
	latest := map[string]time.Time{}
	byID := map[string]*Task{}
	tasks := []*Task{}
	for _, t := range m.Tasks {
		if !t.Mark {
			tasks = append(tasks, t)
			byID[t.ID] = t
		}
	}
	if len(tasks) == 0 {
		return latest
	}
	waiters := map[string][]*Task{}
	for _, t := range tasks {
		for _, a := range t.After {
			if byID[a] != nil {
				waiters[a] = append(waiters[a], t)
			}
		}
	}
	end := tasks[0].End
	for _, t := range tasks {
		if t.End.After(end) {
			end = t.End
		}
	}
	visiting := map[string]bool{}
	var lf func(t *Task) time.Time
	lf = func(t *Task) time.Time {
		if v, ok := latest[t.ID]; ok {
			return v
		}
		if visiting[t.ID] {
			return end
		}
		visiting[t.ID] = true
		v := end
		for _, w := range waiters[t.ID] {
			if c := lf(w).Add(-w.End.Sub(w.Start)); c.Before(v) {
				v = c
			}
		}
		delete(visiting, t.ID)
		latest[t.ID] = v
		return v
	}
	for _, t := range tasks {
		lf(t)
	}
	return latest
}

// Health says whether a task is in trouble. Overdue: not finished and its last
// day has gone. Slipping: it records its progress and is more than a fifth
// behind where that should be by now. A finished task is never in trouble.
// worg also reads slipping against a baseline; the terminal has none.
func Health(t *Task, now time.Time) (kind, note string) {
	if t.Mark || t.State == "done" || t.Src.Implied {
		return "", ""
	}
	today := startOfDay(now)
	lastDay := startOfDay(t.End.Add(-time.Nanosecond))
	if lastDay.Before(today) {
		days := int(today.Sub(lastDay).Hours()/24 + 0.5)
		return "overdue", fmt.Sprintf("overdue %dd", days)
	}
	if !tracked(t) {
		return "", ""
	}
	span := t.End.Sub(t.Start)
	if !t.Start.After(now) && span > 0 {
		expected := float64(now.Sub(t.Start)) / float64(span) * 100
		if expected > 100 {
			expected = 100
		}
		if expected-float64(t.Src.Percent) > 20 {
			return "slipping", fmt.Sprintf("%d%% done, %d%% due", t.Src.Percent, int(expected+0.5))
		}
	}
	return "", ""
}

// Only a task that records its progress can be behind on it: most headings say
// nothing about percent done, and reading that silence as 0% would call every
// task that has started slipping.
func tracked(t *Task) bool {
	if t.Src.Percent > 0 {
		return true
	}
	for k := range t.Src.Props {
		if strings.EqualFold(k, "PERCENTDONE") {
			return true
		}
	}
	return false
}

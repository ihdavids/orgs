package orgs

// GET /recap - what was finished, clocked and written over a span of days,
// what is on today, and what is held up: the material of `orgs standup` and
// `orgs recap`, gathered in one walk so the two cannot disagree with each
// other or with the views each part comes from.
//
// Nothing here has rules of its own. Finished is CLOSED plus the logbook's
// `State "DONE"` lines (parseHabitCompletions), so a repeating task - which
// org puts straight back to TODO and never leaves CLOSED on - still counts.
// Clocked is clockMinsIn over the range, put against the nearest project the
// way the weekly review's time step does. Today is buildAgenda for today.
// Waiting is waitingItems, the weekly review's list. Held up is analyzeDeps.
// Words are the writing log's End - Start per day, as worg sums them.

import (
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// The title as somebody reads it aloud in a standup: emphasis gone, a link as
// its description (else its target), timestamps and progress cookies left
// out - a meeting's `<2026-10-12 Mon 10:00>` is not part of what it is called.
func recapTitle(h *org.Headline) string {
	if h == nil {
		return ""
	}
	b := strings.Builder{}
	var walk func(ns []org.Node)
	walk = func(ns []org.Node) {
		for _, n := range ns {
			switch v := n.(type) {
			case org.Text:
				b.WriteString(v.Content)
			case org.Emphasis:
				walk(v.Content)
			case org.RegularLink:
				if len(v.Description) > 0 {
					walk(v.Description)
				} else {
					b.WriteString(strings.TrimPrefix(v.URL, "file:"))
				}
			case org.Timestamp, org.StatisticToken, org.FootnoteLink:
			default:
				b.WriteString(n.String())
			}
		}
	}
	walk(h.Title)
	return strings.Join(strings.Fields(b.String()), " ")
}

// What the walk remembers about a heading, so the parts built elsewhere
// (agenda, waiting, deps) can be told in the same words.
type recapHead struct {
	title, project, file string
	line                 int
}

// The clock orgs itself is running: it writes no CLOCK line until it stops,
// so the file cannot say. A variable so the tests do not read the real one.
var recapActiveClock = func() (*org.Section, time.Time) {
	c := Clock()
	if !c.IsClockActive() || c.GetTime() == nil {
		return nil, time.Time{}
	}
	t := *c.GetTarget()
	_, sec := GetDb().GetFromTarget(&t, false)
	return sec, c.GetTime().Start
}

// An open `CLOCK: [stamp]` line, which is how Emacs keeps a running clock.
// go-org reads only a closed one (its pattern needs the `--`).
var recapOpenClockRe = regexp.MustCompile(`^\s*CLOCK:\s*(\[[^\]]+\])\s*$`)

func recapOpenClocks(lines []string, from int) []time.Time {
	out := []time.Time{}
	if from < 0 || from >= len(lines) {
		return out
	}
	for i := from + 1; i <= ownEnd(lines, from); i++ {
		if m := recapOpenClockRe.FindStringSubmatch(lines[i]); m != nil {
			if t, ok := reviewStamp(m[1]); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

// Minutes of a clock from start to now that fall in the range.
func recapRunningMins(start time.Time, rng clockRange, now time.Time) float64 {
	if start.Before(rng.From) {
		start = rng.From
	}
	end := now
	if end.After(rng.To) {
		end = rng.To
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start).Minutes()
}

func recapDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func recapRepeats(sdc *org.SDC) bool {
	return sdc != nil && sdc.Date != nil && sdc.Date.RepeatDWMY != ""
}

// buildRecap covers the days from..to (inclusive, local days); now is the
// moment today's parts and running clocks are measured against.
func buildRecap(from, to, now time.Time, user string) common.Recap {
	from, to = recapDay(from), recapDay(to)
	rng := clockRange{Set: true, From: from, To: to.AddDate(0, 0, 1)}
	fromS, toS := from.Format("2006-01-02"), to.Format("2006-01-02")
	days := int(to.Sub(from).Hours()/24+0.5) + 1
	res := common.Recap{
		Ok: true, From: fromS, To: toS, Today: now.Format("2006-01-02"),
		Closed: []common.RecapItem{}, Clocked: []common.RecapGroup{}, Agenda: []common.RecapItem{},
		Waiting: []common.RecapItem{}, Blocked: []common.DepProblem{}, Habits: []common.RecapHabit{},
	}
	heads := map[string]recapHead{}
	item := func(s *org.Section, f *common.OrgFile, project *org.Section) common.RecapItem {
		it := common.RecapItem{Headline: recapTitle(s.Headline), Hash: s.Hash, Filename: f.Filename,
			LineNum: s.Headline.Pos.Row, Status: s.Headline.Status, Tags: s.Headline.Tags}
		if project != nil && project != s {
			it.Project = recapTitle(project.Headline)
		}
		return it
	}
	groups := map[string]*common.RecapGroup{}
	next := []common.RecapItem{}
	cache := lineCache{}
	activeSec, activeStart := recapActiveClock()

	registerAllSections()
	var walk func(s *org.Section, f *common.OrgFile, project *org.Section, done []string)
	walk = func(s *org.Section, f *common.OrgFile, project *org.Section, done []string) {
		if s.Headline == nil || s.Headline.IsComment || IsArchived(s, f.Doc) {
			return
		}
		if IsProject(s, f) {
			project = s
		}
		it := item(s, f, project)
		heads[s.Hash] = recapHead{title: it.Headline, project: it.Project, file: f.Filename, line: it.LineNum}
		isDone := contains(done, s.Headline.Status)
		habit := IsHabitSection(s)

		// Finished: the days it was, CLOSED and the logbook's DONE lines
		// together, a day counted once (orgs writes both when it closes).
		finished := map[string]bool{}
		if c := s.Headline.Closed; isDone && c != nil && c.Date != nil && !c.Date.Start.IsZero() {
			if d := c.Date.Start.Format("2006-01-02"); d >= fromS && d <= toS {
				finished[d] = true
			}
		}
		for _, d := range parseHabitCompletions(s) {
			if d >= fromS && d <= toS {
				finished[d] = true
			}
		}
		if len(finished) > 0 {
			if habit {
				res.Habits = append(res.Habits, common.RecapHabit{Headline: it.Headline, Hash: s.Hash,
					Filename: f.Filename, Done: len(finished), Days: days})
			} else {
				c := it
				for d := range finished {
					if d > c.When {
						c.When = d
					}
				}
				c.Count = len(finished)
				c.Repeat = !isDone || recapRepeats(s.Headline.Scheduled) || recapRepeats(s.Headline.Deadline)
				c.Cancelled = isDone && isCancelledKeyword(s.Headline.Status)
				res.Closed = append(res.Closed, c)
			}
		}

		// Clocked, against the project (or the file), and a clock still
		// running: an open CLOCK line in the file (Emacs), or orgs's own.
		mins := headlineClockMins(s.Headline, rng, now)
		running := recapOpenClocks(cache.of(f.Filename), s.Headline.Pos.Row)
		if activeSec == s {
			running = append(running, activeStart)
		}
		for _, start := range running {
			mins += recapRunningMins(start, rng, now)
			r := it
			r.Mins = now.Sub(start).Minutes()
			r.When = start.Format("15:04")
			res.Running = &r
		}
		if mins > 0 {
			key, g := "f:"+f.Filename, (*common.RecapGroup)(nil)
			if project != nil {
				key = "p:" + project.Hash
			}
			if g = groups[key]; g == nil {
				g = &common.RecapGroup{Name: filepath.Base(f.Filename), Kind: "file", Filename: f.Filename}
				if project != nil {
					g.Name, g.Kind, g.Hash = recapTitle(project.Headline), "project", project.Hash
				}
				groups[key] = g
			}
			c := it
			c.Mins = mins
			g.Items = append(g.Items, c)
			g.Mins += mins
			res.ClockedMins += mins
		}

		if !isDone && !habit && IsNextTask(s, f) {
			n := it
			n.Kind = "next"
			next = append(next, n)
		}
		for _, c := range s.Children {
			walk(c, f, project, done)
		}
	}
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		_, done := ValidStatusFromFile(f)
		for _, s := range f.Doc.Outline.Children {
			walk(s, f, nil, done)
		}
	}

	sort.SliceStable(res.Closed, func(a, b int) bool {
		if res.Closed[a].When != res.Closed[b].When {
			return res.Closed[a].When > res.Closed[b].When
		}
		return res.Closed[a].Headline < res.Closed[b].Headline
	})
	for _, g := range groups {
		sort.SliceStable(g.Items, func(a, b int) bool { return g.Items[a].Mins > g.Items[b].Mins })
		res.Clocked = append(res.Clocked, *g)
	}
	sort.SliceStable(res.Clocked, func(a, b int) bool { return res.Clocked[a].Mins > res.Clocked[b].Mins })
	sort.SliceStable(res.Habits, func(a, b int) bool { return res.Habits[a].Headline < res.Habits[b].Headline })

	res.Agenda = recapToday(now, heads, next)
	res.Waiting = recapWaiting(now, heads)
	res.Blocked = recapBlocked(heads)
	res.Writing = recapWriting(user, fromS, toS, days)
	return res
}

// Today: what the agenda puts on today that is not finished and not a habit
// (the habit tracker's business), then NEXT tasks not already there.
func recapToday(now time.Time, heads map[string]recapHead, next []common.RecapItem) []common.RecapItem {
	out := []common.RecapItem{}
	today := agendaDayNum(now)
	ag, err := buildAgenda("", agendaRules{From: today, To: today, Today: today, WarnDays: agendaWarnDays()}, now)
	seen := map[string]bool{}
	if err == nil && len(ag.Days) == 1 {
		for _, e := range ag.Days[0].Entries {
			n := ag.Nodes[e.Node]
			if e.Done || n.IsDone || n.IsHabit || e.Kind == common.AgendaClosed || seen[n.Hash] {
				continue
			}
			seen[n.Hash] = true
			it := common.RecapItem{Headline: n.Headline, Hash: n.Hash, Filename: n.Filename, LineNum: n.LineNum,
				Status: n.Status, Tags: n.Tags, Kind: e.Kind, Late: e.Late}
			if h, ok := heads[n.Hash]; ok {
				it.Headline, it.Project = h.title, h.project
			}
			if e.Kind == common.AgendaScheduled && e.Late > 0 {
				it.Kind = "overdue"
			}
			if e.Kind == common.AgendaDeadline && e.Late == 0 && e.DueIn > 0 {
				it.DueIn = e.DueIn
			}
			if e.HasTime {
				if i := strings.Index(e.Start, "T"); i >= 0 && len(e.Start) >= i+6 {
					it.When = e.Start[i+1 : i+6]
				}
			}
			out = append(out, it)
		}
	}
	for _, n := range next {
		if !seen[n.Hash] {
			seen[n.Hash] = true
			out = append(out, n)
		}
	}
	return out
}

// Waiting: the weekly review's list, every one of them however short the
// wait - in a standup, "still waiting on the quote, three days" is the point.
func recapWaiting(now time.Time, heads map[string]recapHead) []common.RecapItem {
	out := []common.RecapItem{}
	for _, w := range waitingItems(lineCache{}, 0, now) {
		it := common.RecapItem{Headline: w.Headline, Hash: w.Hash, Filename: w.Filename, LineNum: w.LineNum,
			Status: w.Status, Tags: w.Tags, Days: w.Days, SinceFrom: w.SinceFrom}
		h, ok := heads[w.Hash]
		if !ok {
			// Archived or under a COMMENT heading: not in the walk, not news.
			continue
		}
		it.Headline, it.Project = h.title, h.project
		out = append(out, it)
	}
	return out
}

// Held up: the dependency problems that mean work cannot move - a blocked
// chain, an ORDERED project stuck on a step, something waiting on a heading
// already done. Cycles and dangling AFTERs are mistakes in the file, which
// `orgs deps` reports; they are not news for a standup.
func recapBlocked(heads map[string]recapHead) []common.DepProblem {
	out := []common.DepProblem{}
	rep := analyzeDeps(buildDepGraph(), nil, 3)
	for _, p := range rep.Problems {
		switch p.Kind {
		case "blocked-chain", "ordered-stuck", "waiting-on-done":
		default:
			continue
		}
		for i, c := range p.Chain {
			if h, ok := heads[c.Hash]; ok {
				p.Chain[i].Headline = h.title
			}
		}
		out = append(out, p)
	}
	return out
}

// Words written from..to in writing mode, with the goal that fits the span.
func recapWriting(user, from, to string, days int) *common.RecapWriting {
	ext := GetExtensions()
	if ext == nil {
		return nil
	}
	ext.mu.RLock()
	defer ext.mu.RUnlock()
	u, ok := ext.Users[user]
	if !ok || (len(u.WritingLog) == 0 && u.WritingGoals == nil) {
		return nil
	}
	w := &common.RecapWriting{Files: []common.RecapWords{}}
	perDay := map[string]int{}
	for file, log := range u.WritingLog {
		n := 0
		for d, v := range log {
			if d >= from && d <= to {
				n += v.End - v.Start
				perDay[d] += v.End - v.Start
			}
		}
		if n != 0 {
			w.Files = append(w.Files, common.RecapWords{File: file, Words: n})
			w.Words += n
		}
	}
	if g := u.WritingGoals; g != nil {
		switch {
		case days >= 28 && days <= 31 && g.Monthly > 0:
			w.Goal, w.GoalKind = g.Monthly, "monthly"
		case days == 7 && g.Weekly > 0:
			w.Goal, w.GoalKind = g.Weekly, "weekly"
		case g.Daily > 0:
			w.Goal, w.GoalKind = g.Daily*days, "daily"
		}
	}
	for _, n := range perDay {
		if n > 0 {
			w.Days++
			if u.WritingGoals != nil && u.WritingGoals.Daily > 0 && n >= u.WritingGoals.Daily {
				w.GoalDays++
			}
		}
	}
	sort.SliceStable(w.Files, func(a, b int) bool { return w.Files[a].Words > w.Files[b].Words })
	if w.Words == 0 && w.Goal == 0 {
		return nil
	}
	return w
}

/* SDOC: API
* GET /recap — What Happened, and What Is in the Way
	The material of =orgs standup= and =orgs recap=, in one walk over every
	heading that is not archived or under a COMMENT heading.

	*Parameters:* =from= and =to= (YYYY-MM-DD, inclusive), or =block= (a clock
	table range: =today=, =yesterday=, =thisweek=, =lastweek=, =thismonth=,
	=2026-W40=...). Without them, the last seven days, today included.
	=today= (YYYY-MM-DD) moves "today" for the today-shaped parts.

	*Response:* A =Recap=:
	- =Closed=: headings finished in the range, newest first: a =CLOSED= in it
	  on a finished keyword, or a =State "DONE"= logbook line in it (so a
	  repeating task, put back to TODO the moment it is done, still counts).
	  =Count= is the distinct days, =Repeat= marks a repeating one,
	  =Cancelled= a cancelling keyword. Habits go to =Habits= instead.
	- =Clocked=: minutes of every clock line inside the range (a running one to
	  now), by the nearest project at or above the heading, else by file;
	  =ClockedMins= the whole of it. =Running= is a clock with no end.
	- =Agenda=: today's agenda, unfinished and not habits (=Kind= scheduled,
	  deadline, timestamp, overdue), then NEXT tasks not already in it (=next=).
	- =Waiting=: the weekly review's waiting list, longest first, with =Days=
	  and =SinceFrom= read the same way.
	- =Blocked=: the =blocked-chain=, =ordered-stuck= and =waiting-on-done=
	  problems of =/insights/deps=.
	- =Habits=: each habit with the days it was done in the range.
	- =Writing=: words written in writing mode (=End - Start= per day of the
	  writing log), the days with words, and the goal that fits the span
	  (weekly for seven days, monthly for a month, else daily times days).
	Headlines are plain text: emphasis markers gone, a link as its
	description, timestamps and cookies left out.
	EDOC */
func RequestRecap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now()
	fail := func(msg string) { writingJson(w, common.Recap{Ok: false, Msg: msg}) }
	if s := strings.TrimSpace(q.Get("today")); s != "" {
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			fail("today: " + err.Error())
			return
		}
		now = t.Add(now.Sub(recapDay(now)))
	}
	day := recapDay(now)
	from, to := day.AddDate(0, 0, -6), day
	if b := strings.TrimSpace(q.Get("block")); b != "" {
		rng, err := clockBlockRange(b, now)
		if err != nil || !rng.Set || rng.From.IsZero() {
			fail("block: " + b + " is not a range (today, yesterday, thisweek, lastweek, thismonth, 2026-W40...)")
			return
		}
		from, to = rng.From, rng.To.AddDate(0, 0, -1)
	}
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if s := strings.TrimSpace(q.Get(key)); s != "" {
			t, err := time.ParseInLocation("2006-01-02", s, time.Local)
			if err != nil {
				fail(key + ": " + err.Error())
				return
			}
			*dst = t
		}
	}
	if to.Before(from) {
		fail("to is before from")
		return
	}
	if to.Sub(from) > 400*24*time.Hour {
		fail("at most a year at a time")
		return
	}
	writingJson(w, buildRecap(from, to, now, GetUsername(r)))
}

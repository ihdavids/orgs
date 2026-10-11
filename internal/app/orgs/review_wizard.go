//lint:file-ignore ST1006 allow the use of self
package orgs

// Inbox triage, the someday sweep and the weekly review wizard.
//
// `orgs review` (the nine checks) answers "what wants a decision"; this file is
// what the decisions are made with. Three lists - the inbox, the someday pile,
// the weekly review's middle steps - and one way of acting on a heading that
// can be taken back.
//
// Why the actions have an endpoint of their own when /move, /date/change,
// /heading/parts and /status/change already do every one of them: **undo**.
// Triage is fast, one keypress per decision, and a fast tool needs a way back
// from the wrong key. Undoing a refile or an archive from the client would mean
// finding the heading again in a file it has only just landed in (an archive
// file the database may not even load), by a hash the move has changed. So
// POST /review/act calls the same functions those endpoints call, and around
// them takes the text of every file the action can touch, before and after.
// Undo puts the "before" back - only if the file still reads exactly as the
// action left it, so an undo can never throw away an edit made since. Writing
// back bytes that were read is not the document rewrite the root CLAUDE.md
// warns about: nothing is re-serialised.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/orgdate"
)

// registerReviewRoutes adds the endpoints for the weekly review wizard and inbox triage. Called from RestApi.
func registerReviewRoutes(api *mux.Router) {
	api.HandleFunc("/review/inbox", RequestReviewInbox).Methods("GET")
	api.HandleFunc("/review/someday", RequestReviewSomeday).Methods("GET")
	api.HandleFunc("/review/weekly", RequestReviewWeekly).Methods("GET")
	api.HandleFunc("/review/act", PostReviewAct).Methods("POST")
	api.HandleFunc("/review/undo", PostReviewUndo).Methods("POST")
	api.HandleFunc("/review/last", RequestReviewLast).Methods("GET")
	api.HandleFunc("/review/step", PostReviewStep).Methods("POST")
	api.HandleFunc("/review/done", PostReviewDone).Methods("POST")
}

func reviewSettings() *common.ReviewSettings {
	if c := Conf(); c != nil && c.Server != nil {
		return &c.Server.Review
	}
	return &common.ReviewSettings{}
}

// ---------------------------------------------------------------------------
// Reading a heading's own lines
// ---------------------------------------------------------------------------

var (
	reviewStampRe = regexp.MustCompile(`[\[<](\d{4}-\d{2}-\d{2})(?:\s+[^\]>\s\d]+)?(?:\s+(\d{1,2}:\d{2}))?`)
	reviewStateRe = regexp.MustCompile(`^\s*-\s+State\s+"([^"]+)"\s+from\b.*?(\[[^\]]+\])`)
	reviewNoteRe  = regexp.MustCompile(`^\s*-\s+Note taken on\s+(\[[^\]]+\])\s*(?:\\\\)?\s*(.*)$`)
	reviewClockRe = regexp.MustCompile(`^\s*CLOCK:\s*(\[[^\]]+\])`)
	reviewDrawerEndRe = regexp.MustCompile(`^\s*:END:\s*$`)
)

// The start of a stamp - its day, and its time when it has one - in local time.
func reviewStamp(s string) (time.Time, bool) {
	m := reviewStampRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	layout, text := "2006-01-02", m[1]
	if m[2] != "" {
		layout, text = "2006-01-02 15:04", m[1]+" "+m[2]
	}
	t, err := time.ParseInLocation(layout, text, time.Local)
	return t, err == nil
}

func isHeadingLine(line string) bool {
	stars := 0
	for stars < len(line) && line[stars] == '*' {
		stars++
	}
	return stars > 0 && stars < len(line) && line[stars] == ' '
}

// The last line a heading's own text runs to: up to the next heading of any
// level, so a child's logbook is never read as the parent's.
func ownEnd(lines []string, from int) int {
	for i := from + 1; i < len(lines); i++ {
		if isHeadingLine(lines[i]) {
			return i - 1
		}
	}
	return len(lines) - 1
}

// The prose under a heading: what someone triaging it reads to decide. The
// planning line and every drawer come off (they are shown, or not wanted), and
// a long note is cut rather than sent whole.
func bodyPreview(lines []string, from, end int) string {
	out := []string{}
	inDrawer := false
	for i := from + 1; i <= end && i < len(lines); i++ {
		l := lines[i]
		switch {
		case inDrawer:
			if reviewDrawerEndRe.MatchString(l) {
				inDrawer = false
			}
			continue
		case drawerOpenRe.MatchString(l) && !reviewDrawerEndRe.MatchString(l):
			inDrawer = true
			continue
		case planningRe.MatchString(l) && len(out) == 0:
			continue
		}
		out = append(out, strings.TrimRight(l, " \t"))
	}
	// No blank lines at either end, and no common indentation: the body of a
	// level three heading is indented four spaces by org and none of that is
	// what was written.
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	common := -1
	for _, l := range out {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	for i, l := range out {
		if common > 0 && len(l) >= common {
			out[i] = l[common:]
		}
	}
	const maxLines = 40
	if len(out) > maxLines {
		out = append(out[:maxLines], "…")
	}
	return strings.Join(out, "\n")
}

// When a heading started waiting, read out of its own lines.
//
// The best witness is org's own state change line, `State "WAITING" from
// "NEXT" [stamp]`, written when the keyword was set - the newest one for the
// keyword it is on now, since it may have waited, moved on and waited again.
// Without one, a CREATED (or ADDED) property says when the heading was made,
// which is an upper bound on how long it has waited. Without that, the
// heading's own date, if it has one. The answer says which, so a client can
// say "about" when it is not the logbook.
func waitingSince(lines []string, from, end int, status string, props map[string]string) (time.Time, string) {
	var best time.Time
	for i := from + 1; i <= end && i < len(lines); i++ {
		m := reviewStateRe.FindStringSubmatch(lines[i])
		if m == nil || !strings.EqualFold(m[1], status) {
			continue
		}
		if t, ok := reviewStamp(m[2]); ok && t.After(best) {
			best = t
		}
	}
	if !best.IsZero() {
		return best, "logbook"
	}
	for _, k := range []string{"CREATED", "ADDED", "Created", "Added"} {
		if v, ok := props[k]; ok {
			if t, ok := reviewStamp(v); ok {
				return t, "created"
			}
		}
	}
	return time.Time{}, ""
}

// The last time a waiting heading was chased: a note (org's `- Note taken on`)
// whose text starts with "chased", which is what POST /review/act's chase
// writes.
func lastChased(lines []string, from, end int) time.Time {
	var best time.Time
	for i := from + 1; i <= end && i < len(lines); i++ {
		m := reviewNoteRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[2])
		if text == "" && i+1 < len(lines) {
			text = strings.TrimSpace(lines[i+1])
		}
		if !strings.HasPrefix(strings.ToLower(text), "chased") {
			continue
		}
		if t, ok := reviewStamp(m[1]); ok && t.After(best) {
			best = t
		}
	}
	return best
}

// The last time anything happened to a heading that left a mark in it: a
// clock, a state change or a note. What "nobody has touched it" is measured
// against.
func lastActivity(lines []string, from, end int) time.Time {
	var best time.Time
	for i := from + 1; i <= end && i < len(lines); i++ {
		var stamp string
		if m := reviewStateRe.FindStringSubmatch(lines[i]); m != nil {
			stamp = m[2]
		} else if m := reviewNoteRe.FindStringSubmatch(lines[i]); m != nil {
			stamp = m[1]
		} else if m := reviewClockRe.FindStringSubmatch(lines[i]); m != nil {
			stamp = m[1]
		}
		if stamp == "" {
			continue
		}
		if t, ok := reviewStamp(stamp); ok && t.After(best) {
			best = t
		}
	}
	return best
}

// Whole days from one moment to another, counted in calendar days: something
// that started waiting at 23:00 yesterday has waited one day this morning.
func reviewDaysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.Local)
	return int(b.Sub(a).Hours()/24 + 0.5)
}

// ---------------------------------------------------------------------------
// Building the lists
// ---------------------------------------------------------------------------

// The lines of each file a list touches, read once per request.
type lineCache map[string][]string

func (self lineCache) of(filename string) []string {
	if l, ok := self[filename]; ok {
		return l
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		self[filename] = nil
		return nil
	}
	l := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	self[filename] = l
	return l
}

func countDescendants(sec *org.Section) int {
	n := 0
	for _, c := range sec.Children {
		n += 1 + countDescendants(c)
	}
	return n
}

func reviewSectionProps(sec *org.Section) map[string]string {
	props := map[string]string{}
	if sec.Headline != nil && sec.Headline.Properties != nil {
		for _, p := range sec.Headline.Properties.Properties {
			props[p[0]] = p[1]
		}
	}
	return props
}

func reviewEffortOf(props map[string]string) string {
	for k, v := range props {
		if strings.EqualFold(k, "EFFORT") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func reviewItemOf(sec *org.Section, f *common.OrgFile, cache lineCache) *common.ReviewItem {
	t := SectionToTodo(sec, f)
	if t == nil {
		return nil
	}
	item := &common.ReviewItem{Todo: *t, Days: -1}
	olp := outlineOf(sec)
	if len(olp) > 0 {
		item.Olp = olp[:len(olp)-1]
	}
	item.Children = countDescendants(sec)
	item.Effort = reviewEffortOf(t.Props)
	if sec.Headline.Scheduled != nil && sec.Headline.Scheduled.Date != nil && !sec.Headline.Scheduled.Date.Start.IsZero() {
		item.Scheduled = sec.Headline.Scheduled.Date.Start.Format("2006-01-02")
	}
	if lines := cache.of(f.Filename); lines != nil {
		from := sec.Headline.Pos.Row
		if from >= 0 && from < len(lines) {
			item.Body = bodyPreview(lines, from, ownEnd(lines, from))
		}
	}
	return item
}

// The headings a query finds, outermost only: a heading under one already
// found is part of it - a project in the inbox is one item, not one per task,
// and a :someday: tag (which every child inherits) makes one item of a tree.
func outermostMatches(query string) ([]*org.Section, []*common.OrgFile, error) {
	secs := []*org.Section{}
	files := []*common.OrgFile{}
	found := map[*org.Section]bool{}
	err := queryEachSection(&common.StringQuery{Query: query}, func(v *org.Section, f *common.OrgFile) {
		for p := v.Parent; p != nil; p = p.Parent {
			if found[p] {
				return
			}
		}
		found[v] = true
		secs = append(secs, v)
		files = append(files, f)
	})
	return secs, files, err
}

func reviewList(query string) common.ReviewList {
	res := common.ReviewList{Query: query, Items: []common.ReviewItem{}}
	secs, files, err := outermostMatches(query)
	if err != nil {
		res.Msg = err.Error()
		return res
	}
	cache := lineCache{}
	for i, s := range secs {
		if item := reviewItemOf(s, files[i], cache); item != nil {
			res.Items = append(res.Items, *item)
		}
	}
	res.Ok = true
	return res
}

func reviewJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

/* SDOC: API
* GET /review/inbox — The Inbox, One Item Per Heading
	What inbox triage works through: the headings the =review.inbox= query
	finds (by default the top level of every file called =inbox.org= that is
	neither archived nor done). Only the outermost heading a query finds is an
	item - what is under it goes wherever it goes, and is counted in =Children=.

	*Method:* =GET=

	*Response:* A =ReviewList=: =Ok=, =Msg=, the =Query= used, and =Items=, each
	a =Todo= (headline, tags, hash, keyword, file, line...) plus =Olp= (the
	outline path above it), =Body= (the prose under it, drawers and planning
	taken off, cut at forty lines), =Children=, =Effort= and =Scheduled=.
	EDOC */
func RequestReviewInbox(w http.ResponseWriter, r *http.Request) {
	q := reviewSettings().InboxQuery()
	if s := strings.TrimSpace(r.URL.Query().Get("query")); s != "" {
		q = s
	}
	reviewJson(w, reviewList(q))
}

/* SDOC: API
* GET /review/someday — The Someday Pile
	What the someday sweep works through: every heading on one of
	=review.somedayKeywords= (SOMEDAY, MAYBE) or carrying one of
	=review.somedayTags= (someday, maybe), not archived and not done. Outermost
	only, as for =/review/inbox=, so a tagged project is one item.

	*Method:* =GET=

	*Response:* A =ReviewList=, as =/review/inbox=.
	EDOC */
func RequestReviewSomeday(w http.ResponseWriter, r *http.Request) {
	reviewJson(w, reviewList(reviewSettings().SomedayQuery()))
}

// ---------------------------------------------------------------------------
// The weekly review's middle steps
// ---------------------------------------------------------------------------

// Projects with nothing to do next, each with the open tasks directly under
// it - one of which is usually the next action, and only needs saying so.
func stuckProjects(cache lineCache) []common.StuckProject {
	out := []common.StuckProject{}
	queryEachSection(&common.StringQuery{Query: "IsBlockedProject() && !IsArchived() && !IsDone()"},
		func(v *org.Section, f *common.OrgFile) {
			item := reviewItemOf(v, f, cache)
			if item == nil {
				return
			}
			sp := common.StuckProject{Project: *item, Open: []common.ReviewItem{}}
			for _, c := range v.Children {
				if c.Headline == nil || !IsTodoStatus(c, f) {
					continue
				}
				if ci := reviewItemOf(c, f, cache); ci != nil {
					sp.Open = append(sp.Open, *ci)
				}
			}
			out = append(out, sp)
		})
	return out
}

// Waiting-for items that have waited at least days (counted from when they
// started waiting, or from the last chase if that is later). One whose start
// cannot be read at all is included and says so (Days -1): a waiting item with
// no date is exactly the one that has been forgotten.
func waitingItems(cache lineCache, days int, now time.Time) []common.ReviewItem {
	out := []common.ReviewItem{}
	queryEachSection(&common.StringQuery{Query: reviewSettings().WaitingQuery()},
		func(v *org.Section, f *common.OrgFile) {
			item := reviewItemOf(v, f, cache)
			if item == nil {
				return
			}
			lines := cache.of(f.Filename)
			from := v.Headline.Pos.Row
			if lines == nil || from < 0 || from >= len(lines) {
				return
			}
			end := ownEnd(lines, from)
			since, how := waitingSince(lines, from, end, v.Headline.Status, item.Props)
			if since.IsZero() {
				if d := daysOld(v); d >= 0 {
					since, how = now.AddDate(0, 0, -d), "date"
				}
			}
			counted := since
			if chased := lastChased(lines, from, end); !chased.IsZero() {
				item.Chased = chased.Format("2006-01-02")
				if chased.After(counted) {
					counted = chased
				}
			}
			if !since.IsZero() {
				item.Since = since.Format("2006-01-02")
				item.SinceFrom = how
			}
			if !counted.IsZero() {
				item.Days = reviewDaysBetween(counted, now)
				if item.Days < days {
					return
				}
			}
			out = append(out, *item)
		})
	// Longest waiting first, the unknown ones last.
	sort.SliceStable(out, func(a, b int) bool {
		da, db := out[a].Days, out[b].Days
		if da < 0 {
			da = -1 << 30
		}
		if db < 0 {
			db = -1 << 30
		}
		return da > db
	})
	return out
}

// Open headings scheduled before today that nothing has happened to since the
// day they were scheduled for: no clock, no state change, no note. Habits are
// left out (a missed habit is the habit tracker's business), and so is
// anything already on the waiting list.
func staleItems(cache lineCache, skip map[string]bool, now time.Time) []common.ReviewItem {
	out := []common.ReviewItem{}
	queryEachSection(&common.StringQuery{Query: "IsTodo() && !IsArchived() && ScheduledPast() && !IsHabit()"},
		func(v *org.Section, f *common.OrgFile) {
			if skip[v.Hash] || v.Headline.Scheduled == nil || v.Headline.Scheduled.Date == nil {
				return
			}
			sched := v.Headline.Scheduled.Date.Start
			lines := cache.of(f.Filename)
			from := v.Headline.Pos.Row
			if lines == nil || from < 0 || from >= len(lines) {
				return
			}
			day := time.Date(sched.Year(), sched.Month(), sched.Day(), 0, 0, 0, 0, time.Local)
			if last := lastActivity(lines, from, ownEnd(lines, from)); !last.Before(day) {
				return
			}
			item := reviewItemOf(v, f, cache)
			if item == nil {
				return
			}
			item.Days = reviewDaysBetween(sched, now)
			out = append(out, *item)
		})
	sort.SliceStable(out, func(a, b int) bool { return out[a].Days > out[b].Days })
	return out
}

// Last week's time: what was clocked, against what was planned.
//
// Clocked is every clock line inside the week, on any heading, cut to the
// week. Planned is the EFFORT of every task scheduled or due inside the week.
// Both are put against the project the heading is in (the nearest heading
// above it, or itself, that is a project), or against its file when it is in
// no project; and against each of its tags, inherited ones included, so a
// heading is counted once per tag and the tag rows do not add up to the total.
func timeReview(week string, now time.Time) (common.ReviewTime, error) {
	rng, err := clockBlockRange(week, now)
	if err != nil {
		return common.ReviewTime{}, err
	}
	res := common.ReviewTime{
		From:     rng.From.Format("2006-01-02"),
		To:       rng.To.AddDate(0, 0, -1).Format("2006-01-02"),
		Label:    rng.Label,
		Projects: []common.ReviewTimeRow{},
		Tags:     []common.ReviewTimeRow{},
	}
	projects := map[string]*common.ReviewTimeRow{}
	tags := map[string]*common.ReviewTimeRow{}
	inWeek := func(sdc *org.SDC) bool {
		if sdc == nil || sdc.Date == nil || sdc.Date.Start.IsZero() {
			return false
		}
		t := sdc.Date.Start
		return !t.Before(rng.From) && t.Before(rng.To)
	}
	add := func(row *common.ReviewTimeRow, clocked, planned float64, planned1, done1 bool) {
		row.Clocked += clocked
		row.Planned += planned
		if planned1 {
			row.Tasks++
			if done1 {
				row.Done++
			}
		}
	}

	registerAllSections()
	var walk func(s *org.Section, f *common.OrgFile, project *org.Section)
	walk = func(s *org.Section, f *common.OrgFile, project *org.Section) {
		if s.Headline == nil || IsArchived(s, f.Doc) {
			return
		}
		if IsProject(s, f) {
			project = s
		}
		clocked := headlineClockMins(s.Headline, rng, now)
		planned, isPlanned, isDone := 0.0, false, false
		if s.Headline.Status != "" && (inWeek(s.Headline.Scheduled) || inWeek(s.Headline.Deadline)) {
			isPlanned = true
			_, done := ValidStatusFromFile(f)
			isDone = contains(done, s.Headline.Status)
			if d := common.ParseDuration(reviewEffortOf(reviewSectionProps(s))); d != nil {
				planned = d.Mins
			}
		}
		if clocked > 0 || isPlanned {
			key, row := "", (*common.ReviewTimeRow)(nil)
			if project != nil {
				key = "p:" + project.Hash
				if row = projects[key]; row == nil {
					row = &common.ReviewTimeRow{Name: common.GetSectionTitle(project), Kind: "project", Hash: project.Hash, Filename: f.Filename}
				}
			} else {
				key = "f:" + f.Filename
				if row = projects[key]; row == nil {
					row = &common.ReviewTimeRow{Name: filepath.Base(f.Filename), Kind: "file", Filename: f.Filename}
				}
			}
			projects[key] = row
			add(row, clocked, planned, isPlanned, isDone)
			// The :project: tag is how a project is marked, not what it is
			// about; as a row it would only repeat the project rows' total.
			own := []string{}
			for _, t := range inheritedTags(s) {
				if !(Conf().Server.UseTagForProjects && strings.EqualFold(t, "project")) {
					own = append(own, t)
				}
			}
			if len(own) == 0 {
				own = []string{""}
			}
			for _, t := range own {
				row := tags[t]
				if row == nil {
					name := t
					if name == "" {
						name = "(untagged)"
					}
					row = &common.ReviewTimeRow{Name: name, Kind: "tag"}
					tags[t] = row
				}
				add(row, clocked, planned, isPlanned, isDone)
			}
			res.Clocked += clocked
			res.Planned += planned
		}
		for _, c := range s.Children {
			walk(c, f, project)
		}
	}
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, s := range f.Doc.Outline.Children {
			walk(s, f, nil)
		}
	}
	for _, r := range projects {
		res.Projects = append(res.Projects, *r)
	}
	for _, r := range tags {
		res.Tags = append(res.Tags, *r)
	}
	// Biggest first, by whichever of the two is bigger: a project planned at
	// ten hours and never touched matters as much as one that ate ten.
	big := func(r common.ReviewTimeRow) float64 {
		if r.Clocked > r.Planned {
			return r.Clocked
		}
		return r.Planned
	}
	sort.SliceStable(res.Projects, func(a, b int) bool { return big(res.Projects[a]) > big(res.Projects[b]) })
	sort.SliceStable(res.Tags, func(a, b int) bool { return big(res.Tags[a]) > big(res.Tags[b]) })
	return res, nil
}

/* SDOC: API
* GET /review/weekly — The Weekly Review's Lists
	Everything the weekly review wizard asks about between emptying the inbox
	and the someday sweep, in one answer:

	- =Stuck=: projects with no NEXT action (=IsBlockedProject()=), each with
	  the open tasks directly under it, any of which could be made NEXT.
	- =Waiting=: the =review.waiting= items that have waited at least
	  =waitingDays= days. How long is read from org's own logbook line
	  (=State "WAITING" from ... [stamp]=), else the heading's CREATED property,
	  else its date; =SinceFrom= says which, and =Days= is -1 when nothing
	  does. A chase (a note starting "Chased") restarts the count.
	- =Time=: the week's clocked minutes against its planned minutes (the
	  EFFORT of tasks scheduled or due in it), per project (or file, outside
	  any project) and per tag.
	- =Stale=: open headings scheduled before today with no clock, state change
	  or note since the day they were scheduled for. Habits and waiting items
	  are left out.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter     | Required | Description                                            |
	|---------------+----------+--------------------------------------------------------|
	| =waitingDays= | no       | Override =review.waitingDays= (7).                      |
	| =week=        | no       | The week to compare, as a clock table block: =lastweek= (default), =thisweek=, =2026-W40=. |

	*Response:* A =WeeklyReview=.
	EDOC */
func RequestReviewWeekly(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	days := reviewSettings().WaitDays()
	if s := r.URL.Query().Get("waitingDays"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			days = n
		}
	}
	week := r.URL.Query().Get("week")
	if week == "" {
		week = "lastweek"
	}
	cache := lineCache{}
	res := common.WeeklyReview{WaitingDays: days}
	tm, err := timeReview(week, now)
	if err != nil {
		answerErr(w, err)
		return
	}
	res.Time = tm
	res.Stuck = stuckProjects(cache)
	// Every waiting item, not just the old ones, is kept off the stale list:
	// an item waiting on someone is not stale for being untouched.
	all := waitingItems(cache, 0, now)
	skip := map[string]bool{}
	res.Waiting = []common.ReviewItem{}
	for _, it := range all {
		skip[it.Hash] = true
		if it.Days < 0 || it.Days >= days {
			res.Waiting = append(res.Waiting, it)
		}
	}
	res.Stale = staleItems(cache, skip, now)
	res.Ok = true
	reviewJson(w, res)
}

// ---------------------------------------------------------------------------
// Acting, and taking it back
// ---------------------------------------------------------------------------

// One file as an action found it and left it.
type reviewFile struct {
	path    string
	existed bool
	before  []byte
	// Whether the action left the file there at all (an archive can make
	// one), and what it left in it.
	left  bool
	after []byte
}

type reviewUndo struct {
	token string
	did   string
	files []reviewFile
}

var (
	// One action at a time: the before and after of two interleaved actions
	// would each contain half of the other.
	reviewMu sync.Mutex
	// Per user, newest last. Kept in memory: an undo is for the wrong key
	// pressed a moment ago, not for last week.
	reviewUndos = map[string][]reviewUndo{}
)

const reviewUndoDepth = 50

func reviewToken() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// A path the server can read for a filename the database or a target used.
func reviewPath(name string) string {
	if name == "" {
		return ""
	}
	if _, err := os.Stat(name); err == nil {
		if abs, err := filepath.Abs(name); err == nil {
			return abs
		}
		return name
	}
	p := GetDb().GetFilepath(name)
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func snapshot(paths []string) []reviewFile {
	seen := map[string]bool{}
	out := []reviewFile{}
	for _, p := range paths {
		p = reviewPath(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		b, err := os.ReadFile(p)
		out = append(out, reviewFile{path: p, existed: err == nil, before: b})
	}
	return out
}

// Whether an action changed anything, and the files as it left them.
func (self *reviewUndo) settle() bool {
	changed := false
	for i := range self.files {
		f := &self.files[i]
		b, err := os.ReadFile(f.path)
		f.left = err == nil
		if err == nil {
			f.after = b
		}
		if f.left != f.existed || string(f.after) != string(f.before) {
			changed = true
			GetDb().ReloadFile(f.path)
		}
	}
	return changed
}

// The heading a request names, by hash, with its file.
func reviewHeading(hash string) (*org.Section, *common.OrgFile, error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return nil, nil, fmt.Errorf("no heading with that hash - it may have moved; refresh and try again")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		return nil, nil, fmt.Errorf("could not find the file that heading is in")
	}
	return sec, f, nil
}

// A date as typed - "fri", "+2w", "tomorrow 10:00" - or already an org stamp.
func reviewWhen(s string, now time.Time) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "<") || strings.HasPrefix(s, "[") {
		return s, nil
	}
	return orgdate.ParseDateToOrg(s, now)
}

func setOwnTags(f *common.OrgFile, sec *org.Section, tags []string) error {
	lines, from, _, ok := recordLines(f.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", f.Filename)
	}
	if tags == nil {
		tags = []string{}
	}
	line := setHeadlineParts(lines[from], sec.Headline.Status, nil, &tags)
	if line == lines[from] {
		return nil
	}
	lines[from] = line
	return writeLines(f.Filename, lines)
}

// The heading again after a write to its file, found by its outline path -
// its hash may have changed with the file.
func refind(filename string, olp []string) (*org.Section, *common.OrgFile) {
	f := GetDb().ReloadFile(filename)
	if f == nil {
		return nil, nil
	}
	t := common.Target{Type: "file+olp", Filename: filename, Id: olpString(olp)}
	file, sec := GetDb().GetFromTarget(&t, false)
	if sec == nil || sec.Headline == nil {
		return nil, nil
	}
	return sec, file
}

// The action itself, with the files it may touch named first so they can be
// read before anything is written.
func reviewAction(req *common.ReviewAct, sec *org.Section, f *common.OrgFile, now time.Time) (touch []string, run func() (string, error), err error) {
	title := common.GetSectionTitle(sec)
	from := common.Target{Type: "hash", Id: req.Hash}
	touch = []string{f.Filename}
	switch strings.ToLower(req.Action) {
	case "refile":
		if req.To == nil {
			return nil, nil, fmt.Errorf("refile needs somewhere to go")
		}
		touch = append(touch, destFile(db, req.To))
		// Named before the move: a hash names nothing once its file is written.
		where := describeTarget(req.To)
		if _, to := db.GetFromTarget(req.To, false); to != nil && to.Headline != nil {
			where = common.GetSectionTitle(to)
		} else if req.To.Filename != "" {
			where = filepath.Base(req.To.Filename)
		}
		return touch, func() (string, error) {
			res, err := Move(db, &common.MoveRequest{Op: common.MoveRefile, From: []common.Target{from}, To: *req.To})
			if err == nil && !res.Ok {
				err = fmt.Errorf("%s", moveFailure(res))
			}
			return fmt.Sprintf("refiled %q to %s", title, where), err
		}, nil
	case "archive":
		if at := FindArchiveTarget(db, &from); at != nil {
			touch = append(touch, at.Filename)
		}
		return touch, func() (string, error) {
			res, err := Move(db, &common.MoveRequest{Op: common.MoveArchive, From: []common.Target{from}})
			if err == nil && !res.Ok {
				err = fmt.Errorf("%s", moveFailure(res))
			}
			return fmt.Sprintf("archived %q", title), err
		}, nil
	case "schedule", "deadline", "unschedule":
		name, when := "SCHEDULED", req.When
		if strings.ToLower(req.Action) == "deadline" {
			name = "DEADLINE"
		}
		if strings.ToLower(req.Action) == "unschedule" {
			when = ""
		}
		stamp, err := reviewWhen(when, now)
		if err != nil {
			return nil, nil, err
		}
		return touch, func() (string, error) {
			_, err := ChangeDate(&common.TodoDateChange{Hash: req.Hash, Name: name, Value: stamp})
			if stamp == "" {
				if name == "DEADLINE" {
					return fmt.Sprintf("took the deadline off %q", title), err
				}
				return fmt.Sprintf("unscheduled %q", title), err
			}
			return fmt.Sprintf("%s %q for %s", map[string]string{"SCHEDULED": "scheduled", "DEADLINE": "set a deadline on"}[name], title, stamp), err
		}, nil
	case "tags":
		return touch, func() (string, error) {
			err := setOwnTags(f, sec, req.Tags)
			if len(req.Tags) == 0 {
				return fmt.Sprintf("took the tags off %q", title), err
			}
			return fmt.Sprintf("tagged %q :%s:", title, strings.Join(req.Tags, ":")), err
		}, nil
	case "status", "done", "promote":
		kw := req.Keyword
		if strings.ToLower(req.Action) == "done" && kw == "" {
			kw = "DONE"
		}
		if kw == "" {
			return nil, nil, fmt.Errorf("%s needs a keyword", req.Action)
		}
		olp := outlineOf(sec)
		promote := strings.ToLower(req.Action) == "promote"
		return touch, func() (string, error) {
			res, err := ChangeStatus(&common.TodoItemChange{Hash: req.Hash, Value: kw})
			if err != nil {
				return "", err
			}
			did := fmt.Sprintf("%q is %s", title, kw)
			if res.Msg != "" {
				did = res.Msg
			}
			if !promote {
				return did, nil
			}
			// Off the someday pile for good: a :someday: tag left on it would
			// put it straight back, whatever its keyword.
			s2, f2 := refind(f.Filename, olp)
			if s2 == nil {
				return did, nil
			}
			drop := map[string]bool{}
			for _, t := range reviewSettings().Tags() {
				drop[strings.ToLower(t)] = true
			}
			keep, changed := []string{}, false
			for _, t := range s2.Headline.Tags {
				if drop[strings.ToLower(t)] {
					changed = true
					continue
				}
				keep = append(keep, t)
			}
			if changed {
				if err := setOwnTags(f2, s2, keep); err != nil {
					return did, err
				}
			}
			return fmt.Sprintf("promoted %q to %s", title, kw), nil
		}, nil
	case "next":
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return nil, nil, fmt.Errorf("a next action needs some words")
		}
		kw := req.Keyword
		if kw == "" {
			kw = "NEXT"
		}
		return touch, func() (string, error) {
			lines, at, end, ok := recordLines(f.Filename, sec)
			if !ok {
				return "", fmt.Errorf("could not read %s", f.Filename)
			}
			// Last under the project, before any blank lines that close its
			// subtree, so the gap before the next heading stays where it was.
			for end > at && strings.TrimSpace(lines[end]) == "" {
				end--
			}
			head := strings.Repeat("*", sec.Headline.Lvl+1) + " " + kw + " " + text
			lines = splice(lines, end+1, []string{head})
			return fmt.Sprintf("added %s %q under %q", kw, text, title), writeLines(f.Filename, lines)
		}, nil
	case "chase":
		note := "Chased"
		if t := strings.TrimSpace(req.Text); t != "" {
			note += ": " + t
		}
		return touch, func() (string, error) {
			lines, at, end, ok := recordLines(f.Filename, sec)
			if !ok {
				return "", fmt.Errorf("could not read %s", f.Filename)
			}
			ind := indentOf(sec.Headline.Lvl)
			set := applyHeadingLogProps(fileLogSettings(f), propsFromLines(lines, at, end))
			entry := []string{ind + "- Note taken on " + orgStamp(now) + " \\\\", ind + "  " + note}
			lines = logInto(lines, at, end, ind, set.drawer, entry)
			return fmt.Sprintf("chased %q", title), writeLines(f.Filename, lines)
		}, nil
	}
	return nil, nil, fmt.Errorf("there is no review action called %q", req.Action)
}

func moveFailure(res common.MoveResponse) string {
	for _, r := range res.Results {
		if !r.Ok && r.Msg != "" {
			return r.Msg
		}
	}
	if res.Msg != "" {
		return res.Msg
	}
	return "nothing moved"
}

/* SDOC: API
* POST /review/act — Decide About One Heading, Undoably
	The one write behind inbox triage, the someday sweep and the weekly review.
	It calls what =/move=, =/date/change=, =/heading/parts=, =/status/change= and
	capture call, and around that keeps the text of every file it touched as it
	was before and after, so =/review/undo= can put it back.

	*Method:* =POST=

	*Request Body (JSON):* a =ReviewAct=:
	| Field     | Description                                                              |
	|-----------+--------------------------------------------------------------------------|
	| =Hash=    | The heading.                                                             |
	| =Action=  | =refile=, =archive=, =schedule=, =deadline=, =unschedule=, =tags=, =status=, =done=, =promote=, =next=, =chase=. |
	| =To=      | refile: a =Target= (as =/move= takes, e.g. ={"Type":"hash","Id":...}=).   |
	| =When=    | schedule/deadline: typed (=fri=, =+2w=, =tomorrow 10:00=) or an org stamp; empty clears. |
	| =Tags=    | tags: the heading's whole tag list.                                       |
	| =Keyword= | status/promote: the keyword; next: the new child's keyword (NEXT).       |
	| =Text=    | next: the new child's title; chase: a note to keep with the chase.       |

	=promote= sets the keyword and takes the someday tags off. =chase= writes
	org's own note line (=- Note taken on [stamp] \\= / =Chased=) into the
	logbook, which restarts the waiting count.

	*Response:* =ReviewActResult=: =Ok=, =Msg=, =Did= (what happened, in words)
	and =Undo=, the token for =/review/undo=.
	EDOC */
func PostReviewAct(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.ReviewAct
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	reviewMu.Lock()
	defer reviewMu.Unlock()
	sec, f, err := reviewHeading(req.Hash)
	if err != nil {
		reviewJson(w, common.ReviewActResult{Ok: false, Msg: err.Error()})
		return
	}
	touch, run, err := reviewAction(&req, sec, f, time.Now())
	if err != nil {
		reviewJson(w, common.ReviewActResult{Ok: false, Msg: err.Error()})
		return
	}
	u := reviewUndo{token: reviewToken(), files: snapshot(touch)}
	did, err := run()
	changed := u.settle()
	if err != nil {
		// A failure may still have written something (a refile that wrote the
		// destination and could not delete the source); it can be undone too.
		res := common.ReviewActResult{Ok: false, Msg: err.Error()}
		if changed {
			res.Undo = u.token
			pushUndo(GetUsername(r), u)
		}
		reviewJson(w, res)
		return
	}
	u.did = did
	res := common.ReviewActResult{Ok: true, Did: did}
	if changed {
		res.Undo = u.token
		pushUndo(GetUsername(r), u)
	}
	reviewJson(w, res)
}

func pushUndo(user string, u reviewUndo) {
	list := append(reviewUndos[user], u)
	if len(list) > reviewUndoDepth {
		list = list[len(list)-reviewUndoDepth:]
	}
	reviewUndos[user] = list
}

/* SDOC: API
* POST /review/undo — Take a Review Action Back
	Puts back every file a =/review/act= changed, as it was before. Refused,
	with nothing written, when any of those files has been changed since by
	anything else: an undo never throws away somebody's later edit.

	*Method:* =POST=

	*Request Body (JSON):* ={"Token": "..."}= from the action's =Undo=; empty
	takes back the most recent action.

	*Response:* =ReviewActResult= with =Did= saying what was taken back.
	EDOC */
func PostReviewUndo(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.ReviewUndo
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	reviewMu.Lock()
	defer reviewMu.Unlock()
	user := GetUsername(r)
	list := reviewUndos[user]
	at := len(list) - 1
	if req.Token != "" {
		for at >= 0 && list[at].token != req.Token {
			at--
		}
	}
	if at < 0 {
		reviewJson(w, common.ReviewActResult{Ok: false, Msg: "nothing to undo"})
		return
	}
	u := list[at]
	for _, f := range u.files {
		now, err := os.ReadFile(f.path)
		if (err == nil) != f.left || string(now) != string(f.after) {
			reviewJson(w, common.ReviewActResult{Ok: false,
				Msg: fmt.Sprintf("%s has changed since, so it was left alone - undo that by hand", filepath.Base(f.path))})
			return
		}
	}
	for _, f := range u.files {
		var err error
		if f.existed {
			err = os.WriteFile(f.path, f.before, 0644)
		} else {
			err = os.Remove(f.path)
		}
		if err != nil {
			answerErr(w, err)
			return
		}
		GetDb().ReloadFile(f.path)
	}
	reviewUndos[user] = append(list[:at:at], list[at+1:]...)
	did := u.did
	if did == "" {
		did = "the last action"
	}
	reviewJson(w, common.ReviewActResult{Ok: true, Did: "undid: " + did})
}

// ---------------------------------------------------------------------------
// When the review was last done
// ---------------------------------------------------------------------------

func (self *ExtensionsConfig) reviewState(username string) common.ReviewState {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if u, ok := self.Users[username]; ok && u.Review != nil {
		return *u.Review
	}
	return common.ReviewState{}
}

func (self *ExtensionsConfig) updateReviewState(username string, change func(*common.ReviewState)) (common.ReviewState, error) {
	self.mu.Lock()
	defer self.mu.Unlock()
	u := self.getUser(username)
	if u.Review == nil {
		u.Review = &common.ReviewState{}
	}
	change(u.Review)
	return *u.Review, self.save()
}

func reviewLast(st common.ReviewState, now time.Time) common.ReviewLast {
	res := common.ReviewLast{Ok: true, State: st, DaysAgo: -1}
	if t, err := time.Parse(time.RFC3339, st.Last); err == nil {
		res.DaysAgo = reviewDaysBetween(t.Local(), now)
	}
	return res
}

/* SDOC: API
* GET /review/last — When the Weekly Review Was Last Done
	The user's review record: =State.last= (when the last review was finished,
	RFC 3339), =State.step= and =State.started= (a review in progress, so it can
	be resumed where it was left, from worg or the terminal), and
	=State.history= (the last twelve, each with its summary). =DaysAgo= is
	whole days since the last one, -1 for never. Kept in the per-user
	extensions file.

	*Method:* =GET=

	*Response:* =ReviewLast=.
	EDOC */
func RequestReviewLast(w http.ResponseWriter, r *http.Request) {
	reviewJson(w, reviewLast(GetExtensions().reviewState(GetUsername(r)), time.Now()))
}

/* SDOC: API
* POST /review/step — Remember Where a Review Is
	Records the step a weekly review in progress is on, so leaving and coming
	back (or switching between worg and =orgs review week=) resumes it, and
	optionally what it has done so far (=Tally=, kept until the review is
	finished). =Step= 0 forgets the review in progress.

	*Method:* =POST=

	*Request Body (JSON):* ={"Step": 3, "Tally": {"refiled": 4}}=

	*Response:* =ReviewLast=.
	EDOC */
func PostReviewStep(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Step  int
		Tally map[string]int
	}
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	st, err := GetExtensions().updateReviewState(GetUsername(r), func(s *common.ReviewState) {
		if req.Step <= 0 {
			s.Step, s.Started, s.Tally = 0, "", nil
			return
		}
		if s.Step == 0 || s.Started == "" {
			s.Started = time.Now().Format(time.RFC3339)
		}
		s.Step = req.Step
		if req.Tally != nil {
			s.Tally = req.Tally
		}
	})
	if err != nil {
		answerErr(w, err)
		return
	}
	reviewJson(w, reviewLast(st, time.Now()))
}

/* SDOC: API
* POST /review/done — Finish the Weekly Review
	Records the review as done now, with whatever the client counted along the
	way (=Summary=, e.g. ={"refiled": 4, "archived": 2}=), and forgets the step
	it was on.

	*Method:* =POST=

	*Request Body (JSON):* ={"Summary": {"refiled": 4}}= (optional).

	*Response:* =ReviewLast=.
	EDOC */
func PostReviewDone(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct{ Summary map[string]int }
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	now := time.Now()
	st, err := GetExtensions().updateReviewState(GetUsername(r), func(s *common.ReviewState) {
		s.Last = now.Format(time.RFC3339)
		s.Step, s.Started, s.Tally = 0, "", nil
		s.History = append([]common.ReviewRecord{{Done: s.Last, Summary: req.Summary}}, s.History...)
		if len(s.History) > 12 {
			s.History = s.History[:12]
		}
	})
	if err != nil {
		answerErr(w, err)
		return
	}
	reviewJson(w, reviewLast(st, now))
}

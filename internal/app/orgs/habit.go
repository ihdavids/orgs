package orgs

// Habits, and asking questions about them.
//
// A habit is a heading with `:STYLE: habit` and a repeating schedule: `every two
// days, and definitely overdue after four` is `SCHEDULED: <... .+2d/4d>`. The
// agenda has drawn a little graph of one for as long as it has existed, built by
// reading `State "DONE"` lines back out of the heading's `LOGBOOK` - lines that
// nothing in orgs had ever written, so the graph could only ever be filled in by
// somebody who also used Emacs. Now that marking something done writes one, a
// habit's history is orgs' own, and these are the questions worth asking of it.
//
// Two things about the counting are deliberate.
//
// **A day is the unit, not an instant.** A habit done at eleven at night and
// again at one in the morning is two days' worth, and one done twice in an
// afternoon is one. So completions are folded to distinct days before anything
// is counted, which also makes a double-click on a kanban card harmless.
//
// **The cadence comes from the heading's own repeater**, including org-habit's
// slack half where it has one. A habit that says `.+2d/4d` is asking to be
// judged on four days, not on two, and using the wrong half of that would report
// a broken streak on a habit its owner considers perfectly kept.

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Is this heading a habit?
func IsHabitSection(sec *org.Section) bool {
	if sec == nil || sec.Headline == nil {
		return false
	}
	if props := sec.Headline.Properties; props != nil {
		if v, ok := props.Get("STYLE"); ok {
			return strings.EqualFold(strings.TrimSpace(v), "habit")
		}
	}
	return false
}

// The distinct days this habit was completed on, newest first.
func habitCompletionDays(sec *org.Section) []time.Time {
	seen := map[string]bool{}
	var days []time.Time
	for _, s := range parseHabitCompletions(sec) {
		if seen[s] {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			continue
		}
		seen[s] = true
		days = append(days, t)
	}
	sort.Slice(days, func(a, b int) bool { return days[a].After(days[b]) })
	return days
}

// How many days this habit allows between completions before the run is broken.
//
// The slack half of `.+2d/4d` when it has one, the interval itself when it does
// not - which is org's own reading of a bare repeater. Zero means there is no
// cadence to judge against, which is the honest answer for a habit whose
// schedule does not repeat.
func habitSlackDays(sec *org.Section) int {
	if sec == nil || sec.Headline == nil || sec.Headline.Scheduled == nil {
		return 0
	}
	d := sec.Headline.Scheduled.Date
	if d == nil || d.RepeatRule == nil || d.RepeatDWMY == "" {
		return 0
	}
	if n, unit := d.RepeatMaxNum, d.RepeatMaxDWMY; n != "" && unit != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			return v * daysPerUnit(unit)
		}
	}
	return d.RepeatRule.Options.Interval * daysPerUnit(strings.TrimSpace(d.RepeatDWMY))
}

// A month is counted as 31 days and a year as 366, because this number is only
// ever used as "has too long gone by" - and rounding it the generous way means
// erring towards saying a habit is still on track, which is the direction to be
// wrong in when the alternative is telling somebody they broke a streak they
// did not.
func daysPerUnit(unit string) int {
	switch unit {
	case "h":
		return 1
	case "d":
		return 1
	case "w":
		return 7
	case "m":
		return 31
	case "y":
		return 366
	}
	return 1
}

// How many times in a row this habit has been kept, counted back from the most
// recent completion.
//
// Zero when it has never been done, and zero when the run is already broken as
// of today: a streak is a thing you are currently on, and reporting the length
// of one that ended in March is the sort of number that reads as encouragement
// and is not.
func HabitStreak(sec *org.Section, today time.Time) int {
	days := habitCompletionDays(sec)
	if len(days) == 0 {
		return 0
	}
	slack := habitSlackDays(sec)
	if slack <= 0 {
		// No cadence to judge against, so every completion there has ever been
		// is as much of a run as can honestly be claimed.
		return len(days)
	}
	today = dayOf(today)
	if int(today.Sub(dayOf(days[0])).Hours()/24) > slack {
		return 0
	}
	streak := 1
	for i := 1; i < len(days); i++ {
		gap := int(dayOf(days[i-1]).Sub(dayOf(days[i])).Hours() / 24)
		if gap > slack {
			break
		}
		streak++
	}
	return streak
}

// Has this habit been left too long?
//
// True when more days have gone by since the last completion than the habit
// allows, and true for a habit that has never been done at all once its
// scheduled date is past. A habit with no repeater is never missed, because
// there is nothing saying how often it was meant to happen.
func MissedHabit(sec *org.Section, today time.Time) bool {
	slack := habitSlackDays(sec)
	if slack <= 0 {
		return false
	}
	today = dayOf(today)
	days := habitCompletionDays(sec)
	if len(days) == 0 {
		// Never done. Missed only once the day it was asked for has gone by.
		if sec.Headline.Scheduled == nil || sec.Headline.Scheduled.Date == nil {
			return false
		}
		return dayOf(sec.Headline.Scheduled.Date.Start).Before(today)
	}
	return int(today.Sub(dayOf(days[0])).Hours()/24) > slack
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// ---------------------------------------------------------------------------
// The tracker
// ---------------------------------------------------------------------------

// How many days of history a tracker gets when it does not say.
//
// Eight weeks: long enough that a run is visibly a run and a gap is visibly a
// gap, short enough to draw as one row of squares beside a heading without
// becoming a chart somebody has to study.
const defaultHabitWindow = 56

// The longest run this habit has ever been on.
//
// Every completion, not just the ones in the window - a tracker's whole job is
// to say "you have done better than this", and a best that only looked at the
// last eight weeks would quietly forget the winter you kept it for three months.
func bestHabitStreak(days []time.Time, slack int) int {
	if len(days) == 0 {
		return 0
	}
	if slack <= 0 {
		return len(days)
	}
	// habitCompletionDays hands them back newest first; runs are easier to read
	// walking forwards.
	oldest := make([]time.Time, len(days))
	for i, d := range days {
		oldest[len(days)-1-i] = dayOf(d)
	}
	best, run := 1, 1
	for i := 1; i < len(oldest); i++ {
		if int(oldest[i].Sub(oldest[i-1]).Hours()/24) <= slack {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
	}
	return best
}

// One square per day, oldest first, up to and including today.
//
// The three states are the point. "Not done" means two different things: a daily
// habit not done yesterday is a broken run, and a habit with three days of slack
// not done yesterday is a day you had in hand. Drawing both as a gap makes a
// well-kept habit look like a failing one, which is the opposite of what a
// tracker is for.
//
// A day before the habit's first ever completion is `ok` rather than `miss` -
// there was nothing to miss yet, and a new habit that opened with eight weeks of
// red would be a tracker arguing with the person who just started it.
func habitWindow(done []time.Time, slack, window int, today time.Time) []HabitDayState {
	onDay := map[string]bool{}
	var first time.Time
	for _, d := range done {
		onDay[dayOf(d).Format("2006-01-02")] = true
		if first.IsZero() || d.Before(first) {
			first = dayOf(d)
		}
	}
	out := make([]HabitDayState, 0, window)
	start := dayOf(today).AddDate(0, 0, -(window - 1))
	// The last completion at or before the day being drawn, walked forward so it
	// is one pass rather than a search per day.
	var last time.Time
	for _, d := range done {
		if day := dayOf(d); day.Before(start) && (last.IsZero() || day.After(last)) {
			last = day
		}
	}
	for i := 0; i < window; i++ {
		day := start.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		state := "ok"
		switch {
		case onDay[key]:
			state = "done"
			last = day
		case first.IsZero() || day.Before(first):
			// Nothing had started yet, so there was nothing to miss.
		case slack <= 0:
			// No cadence to be late against.
		case last.IsZero():
			state = "miss"
		case int(day.Sub(last).Hours()/24) > slack:
			state = "miss"
		}
		out = append(out, HabitDayState{Date: key, State: state})
	}
	return out
}

// HabitDayState is common.HabitDay under a local name, so this file does not
// have to import the wire package to talk about a day.
type HabitDayState = common.HabitDay

// Everything a tracker needs about every habit in the database.
//
// Cached per file like the other indexes: a habit's history lives in its own
// heading, so one saved file costs one file rather than a walk of all of them.
var habitParts = NewFileParts[[]common.Habit]()

func allHabits(window int, today time.Time) []common.Habit {
	parts := habitParts.All(func(f *common.OrgFile) []common.Habit {
		var out []common.Habit
		_, doneStates := ValidStatusFromFile(f)
		doneKeyword := ""
		if len(doneStates) > 0 {
			doneKeyword = doneStates[0]
		}
		for _, sec := range flattenSections(f) {
			if !IsHabitSection(sec) {
				continue
			}
			out = append(out, habitOf(f, sec, doneKeyword))
		}
		return out
	})
	var out []common.Habit
	for _, p := range parts {
		out = append(out, p...)
	}
	// The window and the streaks depend on what day it is, so they are worked
	// out here rather than cached against a file that has not changed since
	// yesterday. Everything expensive - finding the habits, reading their
	// logbooks - is what the cache holds.
	for i := range out {
		fill := &out[i]
		days := parseDayList(fill.Days)
		fill.Streak = streakFrom(days, fill.Slack, today)
		fill.Best = bestHabitStreak(days, fill.Slack)
		fill.Days = habitWindow(days, fill.Slack, window, today)
		fill.DoneToday = len(days) > 0 && dayOf(days[0]).Equal(dayOf(today))
		fill.Missed = missedFrom(days, fill.Slack, fill.Scheduled, today)
		fill.Due = fill.Missed || dueToday(fill.Scheduled, today)
		fill.Rate = habitRate(fill.Days, fill.Every, window)
	}
	sort.SliceStable(out, func(a, b int) bool {
		// Whatever wants doing first. A tracker read at breakfast should open on
		// the things still to do, not on a list of ticks.
		if out[a].DoneToday != out[b].DoneToday {
			return !out[a].DoneToday
		}
		if out[a].Due != out[b].Due {
			return out[a].Due
		}
		return out[a].Headline < out[b].Headline
	})
	return out
}

// One habit, with everything that does not depend on today's date.
//
// The completion days are parked in Days as bare dates and turned into the
// window by the caller - which is what lets the expensive half of this be cached
// against the file while the half that changes at midnight is not.
func habitOf(f *common.OrgFile, sec *org.Section, doneKeyword string) common.Habit {
	var title strings.Builder
	for _, t := range sec.Headline.Title {
		title.WriteString(t.String())
	}
	h := common.Habit{
		Hash: sec.Hash, Headline: strings.TrimSpace(title.String()),
		Filename: f.Filename, LineNum: sec.Headline.Pos.Row,
		Status: sec.Headline.Status, Tags: sec.Headline.Tags,
		Slack: habitSlackDays(sec), DoneKeyword: doneKeyword,
	}
	if s := sec.Headline.Scheduled; s != nil && s.Date != nil {
		h.Scheduled = s.Date.Start.Format("2006-01-02")
		h.Repeater = repeaterText(s.Date)
		if s.Date.RepeatRule != nil {
			h.Every = s.Date.RepeatRule.Options.Interval *
				daysPerUnit(strings.TrimSpace(s.Date.RepeatDWMY))
		}
	}
	if h.Every <= 0 {
		h.Every = 1
	}
	done := habitCompletionDays(sec)
	h.Total = len(done)
	if len(done) > 0 {
		h.LastDone = dayOf(done[0]).Format("2006-01-02")
	}
	for _, d := range done {
		h.Days = append(h.Days, common.HabitDay{Date: dayOf(d).Format("2006-01-02"), State: "done"})
	}
	return h
}

// The repeater as it was written, rebuilt from what the parser kept of it.
func repeaterText(d *org.OrgDate) string {
	if d == nil || d.RepeatRule == nil || d.RepeatDWMY == "" {
		return ""
	}
	out := strings.TrimSpace(d.RepeatPre) +
		strconv.Itoa(d.RepeatRule.Options.Interval) +
		strings.TrimSpace(d.RepeatDWMY)
	if d.RepeatMaxNum != "" && d.RepeatMaxDWMY != "" {
		out += "/" + d.RepeatMaxNum + d.RepeatMaxDWMY
	}
	return out
}

func parseDayList(days []common.HabitDay) []time.Time {
	var out []time.Time
	for _, d := range days {
		if t, err := time.ParseInLocation("2006-01-02", d.Date, time.Local); err == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].After(out[b]) })
	return out
}

// The same sums HabitStreak and MissedHabit do, over days already read.
//
// They exist twice because the query functions are handed a section and these
// are handed a list - and having the list already is the whole reason the
// tracker is one walk rather than one per habit per question.
func streakFrom(days []time.Time, slack int, today time.Time) int {
	if len(days) == 0 {
		return 0
	}
	if slack <= 0 {
		return len(days)
	}
	today = dayOf(today)
	if int(today.Sub(dayOf(days[0])).Hours()/24) > slack {
		return 0
	}
	streak := 1
	for i := 1; i < len(days); i++ {
		if int(dayOf(days[i-1]).Sub(dayOf(days[i])).Hours()/24) > slack {
			break
		}
		streak++
	}
	return streak
}

func missedFrom(days []time.Time, slack int, scheduled string, today time.Time) bool {
	if slack <= 0 {
		return false
	}
	today = dayOf(today)
	if len(days) == 0 {
		if scheduled == "" {
			return false
		}
		s, err := time.ParseInLocation("2006-01-02", scheduled, time.Local)
		return err == nil && s.Before(today)
	}
	return int(today.Sub(dayOf(days[0])).Hours()/24) > slack
}

// Is it asking to be done today? Scheduled for today or already past.
func dueToday(scheduled string, today time.Time) bool {
	if scheduled == "" {
		return false
	}
	s, err := time.ParseInLocation("2006-01-02", scheduled, time.Local)
	return err == nil && !s.After(dayOf(today))
}

// How much of what the cadence asked for actually happened.
//
// Counted from the day the habit first appears in the window, not from the edge
// of the window. A habit started on Tuesday and kept on Tuesday and Wednesday
// has been kept every time it has ever been asked for - and judging it against
// eight weeks it did not exist for reports 4%, which is a tracker telling
// somebody who has just started that they are failing. That is the single
// fastest way to make a tracker not get looked at again.
//
// Capped at one. A daily habit done twice on some days would otherwise report
// better than perfect, which reads as a bug rather than as enthusiasm.
func habitRate(days []common.HabitDay, every, window int) float64 {
	if every <= 0 {
		every = 1
	}
	first := -1
	done := 0
	for i, d := range days {
		if d.State != "done" {
			continue
		}
		if first < 0 {
			first = i
		}
		done++
	}
	if first < 0 {
		return 0
	}
	// The days from the first completion to the end of the window, inclusive.
	live := len(days) - first
	expected := live / every
	if expected < 1 {
		expected = 1
	}
	rate := float64(done) / float64(expected)
	if rate > 1 {
		rate = 1
	}
	return rate
}

// ---------------------------------------------------------------------------
// The endpoint
// ---------------------------------------------------------------------------

func habitJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

/* SDOC: API
* GET /habits — Every Habit, And How It Is Going

	Answers with every heading carrying =:STYLE: habit=, its cadence, and its
	recent history: the current run, the best run it has ever been on, how much
	of what the cadence asked for actually happened, and one entry per day for
	the last few weeks.

	The history is read out of the =State "DONE"= lines in each heading's
	logbook - the ones =POST /status/change= writes. Before orgs wrote them this
	endpoint could only have answered about habits kept in Emacs.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Required | Description                                     |
	|-----------+----------+-------------------------------------------------|
	| =days=    | no       | How many days of history, 7 to 371. Default 56. |

	*Response:* A =HabitsResult=. Each habit's =Days= runs oldest first and ends
	on today, with a =State= of =done=, =miss= or =ok=:

	- =done= — a completion is recorded that day.
	- =miss= — as of that day, more time had passed than the habit allows.
	- =ok= — neither: inside the grace the habit's own repeater grants.

	Three states rather than two because "not done" means two different things. A
	daily habit not done yesterday is a broken run; a habit written =.+1d/3d= not
	done yesterday is a day you had in hand. Drawing both the same way makes a
	well-kept habit look like a failing one. For the same reason the day a habit
	comes round is never a miss - you have that day to do it - and no day before
	the habit's first ever completion is one either.

	=Rate= is counted from the habit's first completion rather than from the edge
	of the window, so a habit started on Tuesday and kept since reads as kept
	rather than as 4%.

	=DoneKeyword= is the first finished keyword of that heading's own file, so a
	tracker can offer a button to tick the habit off without a request per habit.
	Ticking it off is =POST /status/change= as usual, which moves the repeater on
	and writes the logbook line that this endpoint reads back.
	EDOC */
func RequestHabits(w http.ResponseWriter, r *http.Request) {
	window := defaultHabitWindow
	if d := strings.TrimSpace(r.URL.Query().Get("days")); d != "" {
		if n, err := strconv.Atoi(d); err == nil {
			// Bounded rather than trusted: the window is one square per day in
			// somebody's browser, and a year of them is already more than a
			// glance. Clamped rather than refused - a tracker asking for too
			// much should get a sensible answer, not an error to handle.
			if n < 7 {
				n = 7
			}
			if n > 371 {
				n = 371
			}
			window = n
		}
	}
	now := time.Now()
	res := common.HabitsResult{Ok: true, Days: window, Today: dayOf(now).Format("2006-01-02")}
	res.Habits = allHabits(window, now)
	for _, h := range res.Habits {
		if h.Due {
			res.DueToday++
		}
		if h.DoneToday {
			res.DoneToday++
		}
		if h.Streak > res.BestStreak {
			res.BestStreak = h.Streak
		}
	}
	habitJson(w, res)
}

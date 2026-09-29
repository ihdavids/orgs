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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
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

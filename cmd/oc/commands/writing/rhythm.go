package writing

// The writing goals arithmetic, the same as worg's rhythm.ts (the goals view
// in writing mode): words per day summed over every file in the log, sums for
// the day, week (Monday on) and month, the streak and pace. Said twice on
// purpose - the CLI only reads the log, so asking the server for a number
// worg works out in the browser would be a third answer. The scenarios in
// rhythm_test.go are the ones in rhythm.test.ts; change both together.
//
// The counts themselves are not worked out here. worg counts the words of
// the file it is editing (writing.ts countWords: prose only, no drawers,
// code or urls) and posts them to /writing/log; a terminal has no way to
// count the same way without a second copy of the prose rules, so this side
// never posts a count.

import (
	"sort"
	"time"
)

// Day is a file's word count at the first and last count of a day.
type Day struct {
	Start int
	End   int
}

// Goals are what POST /writing/goals keeps; zero is no goal.
type Goals struct {
	Daily    int
	Weekly   int
	Monthly  int
	RestDays int
}

// DailyWords is words written each day over every file given.
func DailyWords(files map[string]map[string]Day) map[string]int {
	out := map[string]int{}
	for _, days := range files {
		for d, v := range days {
			out[d] += v.End - v.Start
		}
	}
	return out
}

func IsoDay(t time.Time) string { return t.Format("2006-01-02") }

func dayOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// By calendar fields, not by 24 hours, so a day with a clock change in it is
// still one day.
func addDays(t time.Time, n int) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+n, 0, 0, 0, 0, t.Location())
}

// WeekStart is Monday of t's week.
func WeekStart(t time.Time) time.Time {
	x := dayOnly(t)
	return addDays(x, -((int(x.Weekday()) + 6) % 7))
}

// Whether a day keeps a streak going: the daily goal met, or any words when
// there is no goal.
func counts(n int, g Goals) bool {
	if g.Daily > 0 {
		return n >= g.Daily
	}
	return n > 0
}

func firstDay(byDay map[string]int, loc *time.Location) (time.Time, bool) {
	first := ""
	for k, v := range byDay {
		if v != 0 && (first == "" || k < first) {
			first = k
		}
	}
	if first == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", first, loc)
	return t, err == nil
}

// GoalStreak is days in a row that counted, back from today; a day not yet
// counted today does not break it. Up to RestDays missed days a week (Monday
// on) are forgiven: they do not break the run, nor add to it.
func GoalStreak(byDay map[string]int, today time.Time, g Goals) int {
	t := dayOnly(today)
	earliest, ok := firstDay(byDay, t.Location())
	if !ok {
		return 0
	}
	d := t
	if !counts(byDay[IsoDay(t)], g) {
		d = addDays(t, -1)
	}
	n := 0
	missed := map[string]int{}
	for !d.Before(earliest) {
		if counts(byDay[IsoDay(d)], g) {
			n++
		} else {
			wk := IsoDay(WeekStart(d))
			m := missed[wk] + 1
			if m > g.RestDays {
				break
			}
			missed[wk] = m
		}
		d = addDays(d, -1)
	}
	return n
}

// LongestStreak is the longest such run in the whole log.
func LongestStreak(byDay map[string]int, today time.Time, g Goals) int {
	t := dayOnly(today)
	earliest, ok := firstDay(byDay, t.Location())
	if !ok {
		return 0
	}
	best, n := 0, 0
	missed := map[string]int{}
	for d := earliest; !d.After(t); d = addDays(d, 1) {
		if counts(byDay[IsoDay(d)], g) {
			n++
			if n > best {
				best = n
			}
			continue
		}
		if IsoDay(d) == IsoDay(t) { // today is not over
			break
		}
		wk := IsoDay(WeekStart(d))
		m := missed[wk] + 1
		if m > g.RestDays {
			n = 0
			missed = map[string]int{}
			continue
		}
		missed[wk] = m
	}
	return best
}

func sumRange(byDay map[string]int, from, to time.Time) int {
	s := 0
	for d := dayOnly(from); !d.After(to); d = addDays(d, 1) {
		s += byDay[IsoDay(d)]
	}
	return s
}

// Best is the busiest day in the log.
type Best struct {
	Day   string
	Words int
}

// Rhythm is rhythm.ts's Rhythm, minus the year-long sums nothing here shows.
type Rhythm struct {
	Today, Week, Month, Year int
	Last7, Last30            int
	Average                  int // per day written, over the last 30 days
	DaysWritten30            int
	Best                     *Best `json:",omitempty"`
	Streak, Longest          int
	GoalDays30               int // days in the last 30 the daily goal was met
	WeekLeft                 int // days left in the week, today included
	MonthLeft                int
}

func Compute(byDay map[string]int, today time.Time, g Goals) Rhythm {
	t := dayOnly(today)
	ws := WeekStart(t)
	ms := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	ys := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	last30 := make([]int, 0, 30)
	for k := 0; k < 30; k++ {
		last30 = append(last30, byDay[IsoDay(addDays(t, -k))])
	}
	written, sum30, met := 0, 0, 0
	for _, x := range last30 {
		sum30 += x
		if x > 0 {
			written++
		}
		if g.Daily > 0 && x >= g.Daily {
			met++
		}
	}
	var best *Best
	// Ties go to the earlier day, so the answer does not depend on map order.
	keys := make([]string, 0, len(byDay))
	for k := range byDay {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, d := range keys {
		if n := byDay[d]; n > 0 && (best == nil || n > best.Words) {
			best = &Best{d, n}
		}
	}
	monthEnd := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location())
	r := Rhythm{
		Today:         byDay[IsoDay(t)],
		Week:          sumRange(byDay, ws, t),
		Month:         sumRange(byDay, ms, t),
		Year:          sumRange(byDay, ys, t),
		Last7:         sumRange(byDay, addDays(t, -6), t),
		Last30:        sum30,
		DaysWritten30: written,
		Best:          best,
		Streak:        GoalStreak(byDay, t, g),
		Longest:       LongestStreak(byDay, t, g),
		GoalDays30:    written,
		WeekLeft:      7 - daysBetween(ws, t),
		MonthLeft:     monthEnd.Day() - t.Day() + 1,
	}
	if written > 0 {
		r.Average = int(float64(sum30)/float64(written) + 0.5)
	}
	if g.Daily > 0 {
		r.GoalDays30 = met
	}
	return r
}

func daysBetween(a, b time.Time) int {
	n := 0
	for d := a; d.Before(b); d = addDays(d, 1) {
		n++
	}
	return n
}

// Pace is what is still to write to reach a goal by the end of its period,
// spread over the days left: "620 a day to make the week".
func Pace(done, goal, daysLeft int) int {
	if goal <= 0 || done >= goal {
		return 0
	}
	if daysLeft < 1 {
		daysLeft = 1
	}
	return (goal - done + daysLeft - 1) / daysLeft
}

// Entry is one file written in on one day.
type Entry struct {
	File  string
	Words int
}

// EntriesOn is the files written in on one day, most words first.
func EntriesOn(files map[string]map[string]Day, day string) []Entry {
	out := []Entry{}
	for f, days := range files {
		if v, ok := days[day]; ok && v.End != v.Start {
			out = append(out, Entry{f, v.End - v.Start})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Words != out[j].Words {
			return out[i].Words > out[j].Words
		}
		return out[i].File < out[j].File
	})
	return out
}

package writing

// The same scenarios as worg's rhythm.test.ts.

import (
	"reflect"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t
}

func TestWeekStartsMonday(t *testing.T) {
	if got := IsoDay(WeekStart(day("2026-10-25"))); got != "2026-10-19" {
		t.Fatalf("Sunday's week starts %s", got)
	}
	if got := IsoDay(WeekStart(day("2026-10-19"))); got != "2026-10-19" {
		t.Fatalf("Monday's week starts %s", got)
	}
}

func TestStreakForgivesRestDays(t *testing.T) {
	goals := Goals{Daily: 100}
	// Mon 12 to Tue 20 Oct; Thu 15 short of the goal.
	l := map[string]int{"2026-10-12": 200, "2026-10-13": 200, "2026-10-14": 200, "2026-10-15": 50,
		"2026-10-16": 200, "2026-10-17": 200, "2026-10-18": 200, "2026-10-19": 200, "2026-10-20": 200}
	wed := day("2026-10-21") // nothing yet today: not broken
	rest := goals
	rest.RestDays = 1
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"streak", GoalStreak(l, wed, goals), 5},
		{"streak with a rest day", GoalStreak(l, wed, rest), 8},
		{"streak with no goal", GoalStreak(l, wed, Goals{}), 9},
		{"longest", LongestStreak(l, wed, goals), 5},
		{"longest with a rest day", LongestStreak(l, wed, rest), 8},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestSumsPaceAndEntries(t *testing.T) {
	l := map[string]int{"2026-10-19": 300, "2026-10-21": 200, "2026-09-30": 1000}
	r := Compute(l, day("2026-10-21"), Goals{Daily: 250})
	want := map[string][2]int{
		"today": {r.Today, 200}, "week": {r.Week, 500}, "month": {r.Month, 500}, "year": {r.Year, 1500},
		"weekLeft": {r.WeekLeft, 5}, "monthLeft": {r.MonthLeft, 11}, "goalDays30": {r.GoalDays30, 2},
	}
	for k, v := range want {
		if v[0] != v[1] {
			t.Errorf("%s: %d, want %d", k, v[0], v[1])
		}
	}
	if r.Best == nil || *r.Best != (Best{"2026-09-30", 1000}) {
		t.Errorf("best: %+v", r.Best)
	}
	if Pace(500, 2000, 5) != 300 || Pace(2500, 2000, 5) != 0 {
		t.Errorf("pace")
	}
	got := EntriesOn(map[string]map[string]Day{
		"a": {"2026-10-21": {10, 50}}, "b": {"2026-10-21": {0, 300}}, "c": {},
	}, "2026-10-21")
	if !reflect.DeepEqual(got, []Entry{{"b", 300}, {"a", 40}}) {
		t.Errorf("entries: %+v", got)
	}
}

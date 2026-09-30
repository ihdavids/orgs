package orgs

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// Newest first, the way habitCompletionDays hands them back.
func days(ss ...string) []time.Time {
	out := make([]time.Time, len(ss))
	for i, s := range ss {
		out[i] = day(s)
	}
	return out
}

func states(w []HabitDayState) string {
	out := ""
	for _, d := range w {
		switch d.State {
		case "done":
			out += "#"
		case "miss":
			out += "x"
		default:
			out += "."
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The window
// ---------------------------------------------------------------------------

// The three states are the whole point. "Not done" means two different things:
// a daily habit not done yesterday is a broken run, and a habit with three days
// of slack not done yesterday is a day you had in hand.
func TestAWindowTellsSlackFromFailure(t *testing.T) {
	today := day("2026-09-29")
	// Done on the 25th and the 29th - four days apart.
	done := days("2026-09-29", "2026-09-25")

	// A daily habit: the 26th is the day it came due and is not yet late - you
	// still had that day to do it - so the misses start on the 27th.
	if got := states(habitWindow(done, 1, 5, today)); got != "#.xx#" {
		t.Errorf("slack 1: got %q want %q", got, "#.xx#")
	}
	// Three days of slack: the 26th, 27th and 28th are inside the grace, and
	// only the fourth day would have been a miss - which is today, and today it
	// was done.
	if got := states(habitWindow(done, 3, 5, today)); got != "#...#" {
		t.Errorf("slack 3: got %q want %q", got, "#...#")
	}
}

// A habit started last week must not open with eight weeks of red. There was
// nothing to miss before the first time it was ever done, and a tracker arguing
// with somebody who has just started is the opposite of the point.
func TestNothingIsMissedBeforeTheHabitBegan(t *testing.T) {
	today := day("2026-09-29")
	done := days("2026-09-29", "2026-09-28")
	if got := states(habitWindow(done, 1, 6, today)); got != "....##" {
		t.Errorf("got %q want %q", got, "....##")
	}
}

// A run that was broken before the window opened is still broken inside it: the
// last completion is looked for before the window starts, not assumed absent.
func TestAWindowSeesTheCompletionBeforeItStarts(t *testing.T) {
	today := day("2026-09-29")
	// Done long ago and then not since. Every day of the window is a miss.
	done := days("2026-09-01")
	if got := states(habitWindow(done, 1, 4, today)); got != "xxxx" {
		t.Errorf("got %q want %q", got, "xxxx")
	}
}

// A habit with no cadence to be late against is never late.
func TestNoRepeaterMeansNoMisses(t *testing.T) {
	today := day("2026-09-29")
	done := days("2026-09-20")
	if got := states(habitWindow(done, 0, 5, today)); got != "....." {
		t.Errorf("got %q want %q", got, ".....")
	}
}

// The window ends on today and runs back exactly as many days as asked.
func TestTheWindowEndsToday(t *testing.T) {
	today := day("2026-09-29")
	w := habitWindow(days("2026-09-29"), 1, 7, today)
	if len(w) != 7 {
		t.Fatalf("got %d days want 7", len(w))
	}
	if w[6].Date != "2026-09-29" {
		t.Errorf("last day is %s, want today", w[6].Date)
	}
	if w[0].Date != "2026-09-23" {
		t.Errorf("first day is %s, want 2026-09-23", w[0].Date)
	}
}

// Ticking a habit off twice in an afternoon is one day's worth, so a day is the
// unit here as it is everywhere else in this file.
func TestTwoCompletionsOnOneDayAreOneSquare(t *testing.T) {
	today := day("2026-09-29")
	done := days("2026-09-29", "2026-09-29", "2026-09-28")
	if got := states(habitWindow(done, 1, 3, today)); got != ".##" {
		t.Errorf("got %q want %q", got, ".##")
	}
}

// ---------------------------------------------------------------------------
// Streaks
// ---------------------------------------------------------------------------

// A tracker's whole job is to say "you have done better than this", so the best
// run is over every completion there has ever been - not over the window, which
// would quietly forget the winter you kept it for three months.
func TestTheBestRunLooksPastTheWindow(t *testing.T) {
	// Five in a row in August, broken, then two in a row now.
	done := days(
		"2026-09-29", "2026-09-28",
		"2026-08-05", "2026-08-04", "2026-08-03", "2026-08-02", "2026-08-01",
	)
	if got := bestHabitStreak(done, 1); got != 5 {
		t.Errorf("got %d want 5", got)
	}
	if got := streakFrom(done, 1, day("2026-09-29")); got != 2 {
		t.Errorf("current run: got %d want 2", got)
	}
}

func TestBestRunOfNothing(t *testing.T) {
	if got := bestHabitStreak(nil, 1); got != 0 {
		t.Errorf("got %d want 0", got)
	}
}

// A run you are no longer on is not a run. Reporting the length of one that
// ended in March reads as encouragement and is not.
func TestABrokenRunIsZero(t *testing.T) {
	done := days("2026-09-01", "2026-08-31")
	if got := streakFrom(done, 1, day("2026-09-29")); got != 0 {
		t.Errorf("got %d want 0", got)
	}
	// But the best is still remembered.
	if got := bestHabitStreak(done, 1); got != 2 {
		t.Errorf("best: got %d want 2", got)
	}
}

// Slack widens what counts as a run, and it must widen it by exactly what the
// habit asked for.
func TestSlackWidensARun(t *testing.T) {
	done := days("2026-09-29", "2026-09-26", "2026-09-23")
	if got := streakFrom(done, 1, day("2026-09-29")); got != 1 {
		t.Errorf("daily: got %d want 1", got)
	}
	if got := streakFrom(done, 3, day("2026-09-29")); got != 3 {
		t.Errorf("three days of slack: got %d want 3", got)
	}
}

// ---------------------------------------------------------------------------
// Rate
// ---------------------------------------------------------------------------

// A daily habit done twice on some days would otherwise report better than
// perfect, which reads as a bug rather than as enthusiasm.
func TestTheRateIsCappedAtPerfect(t *testing.T) {
	today := day("2026-09-29")
	w := habitWindow(days("2026-09-29", "2026-09-28", "2026-09-27"), 1, 3, today)
	if got := habitRate(w, 1, 3); got != 1 {
		t.Errorf("got %v want 1", got)
	}
}

// A habit is judged from the day it started, not from the edge of the window.
// Telling somebody who began on Tuesday and has kept it every day since that
// they are at 4% is the fastest way to make a tracker not get looked at again.
func TestANewHabitIsNotJudgedOnDaysItDidNotExist(t *testing.T) {
	today := day("2026-09-29")
	// Started the day before yesterday and kept every day since.
	w := habitWindow(days("2026-09-29", "2026-09-28"), 1, 56, today)
	if got := habitRate(w, 1, 56); got != 1 {
		t.Errorf("got %v want 1 - kept every day it has ever been asked for", got)
	}
}

// A habit that lapsed before the window even opened has nothing in it to score.
func TestTheRateOfAHabitWithNothingInTheWindow(t *testing.T) {
	today := day("2026-09-29")
	w := habitWindow(days("2026-01-01"), 1, 56, today)
	if got := habitRate(w, 1, 56); got != 0 {
		t.Errorf("got %v want 0", got)
	}
}

// A weekly habit is judged against how often it was meant to happen, not
// against the number of days on the screen - otherwise everything but a daily
// habit reads as a failure.
func TestTheRateIsAgainstTheCadence(t *testing.T) {
	today := day("2026-09-29")
	// Four weeks, done once a week.
	done := days("2026-09-29", "2026-09-22", "2026-09-15", "2026-09-08")
	w := habitWindow(done, 7, 28, today)
	if got := habitRate(w, 7, 28); got != 1 {
		t.Errorf("weekly habit kept weekly: got %v want 1", got)
	}
	// The same four completions judged as a daily habit is much worse: three
	// weeks live, four days kept.
	if got := habitRate(w, 1, 28); got > 0.25 {
		t.Errorf("as a daily habit: got %v, expected it to read as poor", got)
	}
}

func TestTheRateOfNothing(t *testing.T) {
	if got := habitRate(nil, 1, 56); got != 0 {
		t.Errorf("got %v want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Due and missed
// ---------------------------------------------------------------------------

func TestWhatIsDueAndWhatIsMissed(t *testing.T) {
	today := day("2026-09-29")
	// Never done, scheduled in the past: missed.
	if !missedFrom(nil, 1, "2026-09-20", today) {
		t.Error("a habit never done and long past due is not missed")
	}
	// Never done, scheduled today: not missed yet.
	if missedFrom(nil, 1, "2026-09-29", today) {
		t.Error("a habit due today is already missed")
	}
	// Done today: not missed.
	if missedFrom(days("2026-09-29"), 1, "2026-09-30", today) {
		t.Error("a habit done today is missed")
	}
	// Done four days ago with one day of slack: missed.
	if !missedFrom(days("2026-09-25"), 1, "2026-09-26", today) {
		t.Error("a habit four days stale with a day of slack is not missed")
	}
	// The same, with four days of slack: not missed.
	if missedFrom(days("2026-09-25"), 4, "2026-09-26", today) {
		t.Error("a habit inside its own slack is missed")
	}
	// Scheduled tomorrow is not due today.
	if dueToday("2026-09-30", today) {
		t.Error("tomorrow is due today")
	}
	if !dueToday("2026-09-29", today) {
		t.Error("today is not due today")
	}
}

// The day a habit comes round is not a day it was missed. A daily habit done
// yesterday and not yet done today is not failing at breakfast - it has all day.
// Getting this off by one paints a red square on every habit every morning,
// which is a tracker that cannot be trusted and so will not be looked at.
func TestTheDayItComesDueIsNotAMiss(t *testing.T) {
	today := day("2026-09-29")
	// Done yesterday, not yet today.
	if got := states(habitWindow(days("2026-09-28"), 1, 3, today)); got != ".#." {
		t.Errorf("got %q want %q - today should be open, not missed", got, ".#.")
	}
	// Not done yesterday either: yesterday is now a miss and today is still open.
	if got := states(habitWindow(days("2026-09-27"), 1, 3, today)); got != "#.x" {
		t.Errorf("got %q want %q", got, "#.x")
	}
}

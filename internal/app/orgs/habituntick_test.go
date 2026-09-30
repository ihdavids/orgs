package orgs

// Taking today's tick off, pinned.
//
// The orchestration needs a loaded database; what is tested here is every piece
// that decides *what to write*, because those are the ones that can quietly put a
// habit's schedule somewhere it never was.

import (
	"strings"
	"testing"
	"time"
)

func onDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// One interval back, for each spelling and each unit.
func TestRepeaterBack(t *testing.T) {
	for _, c := range []struct {
		cookie string
		from   time.Time
		want   time.Time
	}{
		{".+2d", onDay(2026, 10, 1), onDay(2026, 9, 29)},
		{"+2d", onDay(2026, 10, 1), onDay(2026, 9, 29)},
		{"++2d", onDay(2026, 10, 1), onDay(2026, 9, 29)},
		{".+1w", onDay(2026, 10, 8), onDay(2026, 10, 1)},
		{"+1m", onDay(2026, 10, 1), onDay(2026, 9, 1)},
		{"+1y", onDay(2026, 10, 1), onDay(2025, 10, 1)},
		// The slack half is part of the cookie and says nothing about the step.
		{".+2d/3d", onDay(2026, 10, 1), onDay(2026, 9, 29)},
	} {
		rep, ok := parseRepeater(c.cookie)
		if !ok {
			t.Fatalf("%s did not parse", c.cookie)
		}
		if got := rep.back(c.from); !got.Equal(c.want) {
			t.Errorf("%s from %s: got %s want %s", c.cookie,
				c.from.Format("2006-01-02"), got.Format("2006-01-02"), c.want.Format("2006-01-02"))
		}
	}
}

// Stepping back and forward have to be each other's inverse for `+`, which is the
// one spelling where that is exactly true.
func TestPlusIsExactlyReversible(t *testing.T) {
	rep, _ := parseRepeater("+3d")
	had := onDay(2026, 9, 20)
	if got := rep.back(rep.next(had, onDay(2026, 9, 29))); !got.Equal(had) {
		t.Fatalf("got %s want %s", got.Format("2006-01-02"), had.Format("2006-01-02"))
	}
}

// A `.+` habit ticked off **early** must get its own date back, not today's.
//
// This is the whole reason the previous completion is read out of the logbook. A
// habit last kept on the 28th with `.+2d` was due on the 30th; ticked off on the
// 29th the date became the 1st. Stepping back one interval gives the 29th - which
// quietly moves the schedule a day earlier. Counting from the previous completion
// gives the 30th, which is where it was.
func TestEarlyTickGetsItsOwnDateBack(t *testing.T) {
	lines := []string{
		"* TODO Stretch",
		"  SCHEDULED: <2026-10-01 Thu .+2d/3d>",
	}
	moved, when := retreatPlanning(lines, 0, 1, onDay(2026, 9, 28))
	if !moved {
		t.Fatal("the date should have moved")
	}
	if when != "2026-09-30" {
		t.Fatalf("got %s, want the date it actually had: 2026-09-30", when)
	}
	if !strings.Contains(lines[1], "<2026-09-30 Wed .+2d/3d>") {
		t.Fatalf("the line was rewritten wrongly: %q", lines[1])
	}
}

// With nothing to count from, one interval back is the best there is.
func TestWithNoPreviousCompletionItStepsBack(t *testing.T) {
	lines := []string{
		"* TODO Stretch",
		"  SCHEDULED: <2026-10-01 Thu .+2d>",
	}
	_, when := retreatPlanning(lines, 0, 1, time.Time{})
	if when != "2026-09-29" {
		t.Fatalf("got %s want 2026-09-29", when)
	}
}

// An overdue habit stays overdue. A habit last kept on the 20th and due on the
// 22nd, ticked off on the 29th, must come back to the 22nd - not to the 29th,
// which would erase the fact that it had been missed for a week.
func TestUntickKeepsAHabitOverdue(t *testing.T) {
	lines := []string{
		"* TODO Water the plants",
		"  SCHEDULED: <2026-10-01 Thu .+2d>",
	}
	_, when := retreatPlanning(lines, 0, 1, onDay(2026, 9, 20))
	if when != "2026-09-22" {
		t.Fatalf("got %s want 2026-09-22", when)
	}
}

// A repeating schedule and a fixed deadline: moving the deadline back because the
// schedule repeated would turn a real due date into a rolling one.
func TestOnlyRepeatingDatesAreMoved(t *testing.T) {
	lines := []string{
		"* TODO Thing",
		"  DEADLINE: <2026-12-01 Tue> SCHEDULED: <2026-10-01 Thu .+2d>",
	}
	if _, when := retreatPlanning(lines, 0, 1, time.Time{}); when != "2026-09-29" {
		t.Fatalf("got %s", when)
	}
	if !strings.Contains(lines[1], "DEADLINE: <2026-12-01 Tue>") {
		t.Fatalf("the deadline moved: %q", lines[1])
	}
}

// A day is the unit, so every completion recorded today comes off - a habit ticked
// twice in an afternoon is one filled square and has to be cleared as one.
func TestEveryCompletionForTodayComesOff(t *testing.T) {
	lines := []string{
		"* TODO Stretch",
		"  :LOGBOOK:",
		`  - State "DONE"       from "TODO"       [2026-09-29 Tue 21:00]`,
		`  - State "DONE"       from "TODO"       [2026-09-29 Tue 08:00]`,
		`  - State "DONE"       from "TODO"       [2026-09-27 Sun 08:00]`,
		"  :END:",
	}
	at := habitDoneLinesOn(lines, 0, 5, "LOGBOOK", "2026-09-29")
	if len(at) != 2 || at[0] != 2 || at[1] != 3 {
		t.Fatalf("got %v, want both of today's lines", at)
	}
	if got := previousCompletion(lines, 0, 5, "LOGBOOK", "2026-09-29"); !got.Equal(onDay(2026, 9, 27)) {
		t.Fatalf("the previous completion is the 27th, got %s", got.Format("2006-01-02"))
	}
}

// A child heading's logbook is its own. Counting a child's completions as the
// parent's would clear a square nobody pressed and delete a line from a heading
// that was not being talked about.
func TestAChildsLogbookIsNotThisHabitsLogbook(t *testing.T) {
	lines := []string{
		"* TODO Parent habit",
		"  :LOGBOOK:",
		`  - State "DONE"       from "TODO"       [2026-09-29 Tue 08:00]`,
		"  :END:",
		"** TODO A child of it",
		"   :LOGBOOK:",
		`   - State "DONE"       from "TODO"       [2026-09-29 Tue 09:00]`,
		"   :END:",
	}
	at := habitDoneLinesOn(lines, 0, 7, "LOGBOOK", "2026-09-29")
	if len(at) != 1 || at[0] != 2 {
		t.Fatalf("got %v, want only the parent's own line", at)
	}
}

// A heading whose log went into the body rather than into a drawer still has a
// history, and turning the drawer off must not turn this off with it.
func TestALogInTheBodyIsStillFound(t *testing.T) {
	lines := []string{
		"* TODO Stretch",
		"  SCHEDULED: <2026-10-01 Thu .+2d>",
		`  - State "DONE"       from "TODO"       [2026-09-29 Tue 08:00]`,
	}
	if at := habitDoneLinesOn(lines, 0, 2, "", "2026-09-29"); len(at) != 1 || at[0] != 2 {
		t.Fatalf("got %v", at)
	}
}

// Nothing today is the answer rather than a failure, and it is what makes pressing
// the key twice harmless.
func TestNothingRecordedTodayFindsNothing(t *testing.T) {
	lines := []string{
		"* TODO Stretch",
		"  :LOGBOOK:",
		`  - State "DONE"       from "TODO"       [2026-09-28 Mon 08:00]`,
		"  :END:",
	}
	if at := habitDoneLinesOn(lines, 0, 3, "LOGBOOK", "2026-09-29"); len(at) != 0 {
		t.Fatalf("got %v, want nothing", at)
	}
}

// :LAST_REPEAT: is the evidence that the date moved today, so reading the day off
// it has to work on the stamp org writes.
func TestStampDay(t *testing.T) {
	for in, want := range map[string]string{
		"[2026-09-29 Tue 21:24]": "2026-09-29",
		"[2026-09-29 Tue]":       "2026-09-29",
		"<2026-09-29 Tue>":       "2026-09-29",
		"":                       "",
		"not a stamp":            "",
	} {
		if got := stampDay(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestDropPropIn(t *testing.T) {
	lines := []string{
		"* TODO Stretch",
		"  :PROPERTIES:",
		"  :STYLE:       habit",
		"  :LAST_REPEAT: [2026-09-29 Tue 21:24]",
		"  :END:",
	}
	out, dropped := dropPropIn(lines, 1, 4, "LAST_REPEAT")
	if !dropped {
		t.Fatal("it was there")
	}
	if len(out) != 4 || strings.Contains(strings.Join(out, "\n"), "LAST_REPEAT") {
		t.Fatalf("got %v", out)
	}
	// And a key that is not there is not an error, just a no.
	if _, again := dropPropIn(out, 1, 3, "LAST_REPEAT"); again {
		t.Fatal("it was not there the second time")
	}
}

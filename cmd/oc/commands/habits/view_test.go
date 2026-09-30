package habits

// The tone, pinned.
//
// These are the same cases `worg/src/habits.test.ts` pins, for the same reason:
// the failures here are not crashes, they are a tracker that says something
// discouraging or untrue about somebody's own habits, and nothing about the
// output looks wrong when it happens. The browser and the terminal have to agree
// about what a run is worth, or the same habit reads as two different habits
// depending on where it is looked at.

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
	fzf "github.com/junegunn/fzf/src"
)

func days(spec string) []common.HabitDay {
	// A spec is one character per day, oldest first: `d` done, `m` missed,
	// `.` in hand. The dates are made up but consecutive, which is all any of
	// this reads off them.
	out := []common.HabitDay{}
	for i, r := range spec {
		state := "ok"
		switch r {
		case 'd':
			state = "done"
		case 'm':
			state = "miss"
		}
		out = append(out, common.HabitDay{
			Date:  isoDay(i),
			State: state,
		})
	}
	return out
}

// Dates from the 1st of August 2026 onwards, so a window can cross a month.
func isoDay(i int) string {
	day := 1 + i
	month := 8
	for day > 31 {
		day -= 31
		month++
	}
	return "2026-0" + string(rune('0'+month)) + "-" + pad2(day)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// A first unbroken run is technically its own best from day two, and saying so
// every day makes the words worthless by the time the run is worth something.
func TestBestEverNeedsAPreviousRunToBeat(t *testing.T) {
	first := common.Habit{Streak: 40, Best: 40, Total: 40}
	if text, tone := StreakNote(first); tone != "good" || text != "40" {
		t.Fatalf("a first unbroken run should just be its number, got %q/%s", text, tone)
	}
	// Completions behind the run mean there was a run before it, so matching it
	// is an achievement rather than an arithmetic inevitability.
	again := common.Habit{Streak: 40, Best: 40, Total: 55}
	if text, tone := StreakNote(again); tone != "best" || !strings.Contains(text, "best ever") {
		t.Fatalf("beating a previous run should say so, got %q/%s", text, tone)
	}
}

// A broken run never says what it was. The job at that moment is to make
// starting again look small, not to price what was lost.
func TestBrokenRunDoesNotSayWhatItWas(t *testing.T) {
	text, tone := StreakNote(common.Habit{Streak: 0, Best: 40, Total: 120})
	if tone != "broken" || text != "start again" {
		t.Fatalf("expected a plain start again, got %q/%s", text, tone)
	}
	if strings.ContainsAny(text, "0123456789") {
		t.Fatalf("a broken run must not name a number: %q", text)
	}
}

func TestNeverStartedIsNotAFailure(t *testing.T) {
	h := common.Habit{Streak: 0, Best: 0, Total: 0}
	if text, tone := StreakNote(h); tone != "new" || text != "not started" {
		t.Fatalf("expected not started, got %q/%s", text, tone)
	}
	// And nought per cent is a judgement about a habit that has not had the
	// chance to be judged.
	if got := RatePct(h); got != "—" {
		t.Fatalf("a habit never done should not be given a percentage, got %q", got)
	}
}

// The header is the difference between a scoreboard and a nag.
func TestSummary(t *testing.T) {
	none := common.HabitsResult{}
	if _, tone := Summary(none); tone != "none" {
		t.Fatalf("no habits should be its own case")
	}
	all := common.HabitsResult{Habits: make([]common.Habit, 3), DueToday: 0, DoneToday: 3}
	text, tone := Summary(all)
	if tone != "done" || !strings.HasPrefix(text, "all done") {
		t.Fatalf("everything done should feel like arriving there, got %q/%s", text, tone)
	}
	some := common.HabitsResult{Habits: make([]common.Habit, 5), DueToday: 2, DoneToday: 1}
	text, tone = Summary(some)
	if tone != "due" || strings.Contains(text, "!") {
		t.Fatalf("the number is the message, got %q/%s", text, tone)
	}
}

// A status bar segment with nothing to say says nothing rather than taking up
// room to say so.
func TestShortLineIsEmptyWithNoHabits(t *testing.T) {
	if got := ShortLine(common.HabitsResult{}); got != "" {
		t.Fatalf("expected nothing at all, got %q", got)
	}
}

// `.+1d/3d` is not something to put in front of somebody who has never read the
// org manual.
func TestCadenceInWords(t *testing.T) {
	for _, c := range []struct {
		every, slack int
		want         string
	}{
		{1, 1, "every day"},
		{7, 7, "every week"},
		{14, 14, "every 2 weeks"},
		{3, 3, "every 3 days"},
		{1, 3, "every day, late after 3 days"},
	} {
		h := common.Habit{Every: c.every, Slack: c.slack, Repeater: ".+1d"}
		if got := Cadence(h); got != c.want {
			t.Errorf("Every %d Slack %d: got %q want %q", c.every, c.slack, got, c.want)
		}
	}
	// A habit whose schedule does not repeat has no cadence, and must not be
	// given one: the server reports Every as 1 for these because a zero would
	// divide, not because they happen daily.
	if got := Cadence(common.Habit{Every: 1, Slack: 0}); got != "does not repeat" {
		t.Errorf("a habit with no repeater should say so, got %q", got)
	}
}

// The window is trimmed from the oldest end, never sampled: a strip that dropped
// every other day would fit any width and could not say what it was looking at.
func TestRecentDaysKeepsTheNewestAndEveryOneOfThem(t *testing.T) {
	all := days("dddmmm...ddd")
	got := RecentDays(all, 4)
	if len(got) != 4 {
		t.Fatalf("expected 4 days, got %d", len(got))
	}
	if got[3].Date != all[len(all)-1].Date {
		t.Fatalf("the window must end on the newest day")
	}
	if Plain(got) != Plain(all[len(all)-4:]) {
		t.Fatalf("the window must be the days as they were, got %q", Plain(got))
	}
	// Asking for more than there is gives what there is rather than padding.
	if len(RecentDays(all, 500)) != len(all) {
		t.Fatalf("a wider window than the history is the whole history")
	}
}

// Three states have to be three shapes, not three colours: a pipe, NO_COLOR and
// a status bar all strip the colour and the strip still has to be readable.
func TestEachStateIsItsOwnCharacter(t *testing.T) {
	got := Plain(days("dm."))
	if got != GlyphDone+GlyphMiss+GlyphOk {
		t.Fatalf("got %q", got)
	}
	if GlyphDone == GlyphMiss || GlyphMiss == GlyphOk || GlyphDone == GlyphOk {
		t.Fatalf("the three states must be three characters")
	}
}

// The colour changes once per run rather than once per day - and whatever it
// does, the characters have to come out in the order they were given.
func TestStripSaysEveryDayItWasGiven(t *testing.T) {
	in := days("ddd.mm.d")
	bare := stripEscapes(Strip(in))
	if bare != Plain(in) {
		t.Fatalf("the strip lost or moved a day: %q vs %q", bare, Plain(in))
	}
}

// Counted and cut in runes, not in bytes: a block glyph is three bytes and any
// of this measured in bytes is measuring the encoding.
func stripEscapes(s string) string {
	out := []rune{}
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc:
			if r == 'm' {
				esc = false
			}
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func runeIndex(hay, needle string) int {
	i := strings.Index(hay, needle)
	if i < 0 {
		return -1
	}
	return len([]rune(hay[:i]))
}

// A month name against the first character of the strip would label a day that
// is not the first of its month, and one at the right-hand end would run off it.
func TestMonthTicks(t *testing.T) {
	// 40 days from the 1st of August: the 1st of September is at index 31.
	ticks := MonthTicks(days(strings.Repeat(".", 40)), 4)
	if len(ticks) != 1 {
		t.Fatalf("expected one label, got %d: %v", len(ticks), ticks)
	}
	if ticks[0].At != 31 || ticks[0].Label != "Sep" {
		t.Fatalf("expected Sep at 31, got %v", ticks[0])
	}
	// The same month boundary too near the end is left off.
	if got := MonthTicks(days(strings.Repeat(".", 33)), 4); len(got) != 0 {
		t.Fatalf("a label with no room should be left off, got %v", got)
	}
	// And the very first day is never labelled, however the window opens.
	for _, tick := range MonthTicks(days(strings.Repeat(".", 40)), 4) {
		if tick.At == 0 {
			t.Fatalf("the first character of a strip is not the first of its month")
		}
	}
}

// The ruler puts each label in the column its month begins in, because it is
// read against the strip above it.
func TestRulerLinesUpWithTheStrip(t *testing.T) {
	d := days(strings.Repeat(".", 40))
	r := Ruler(d, 4)
	if runeIndex(r, "Sep") != 31 {
		t.Fatalf("Sep should start in column 31: %q", r)
	}
	if len([]rune(r)) > len(d) {
		t.Fatalf("the ruler must not be wider than the strip it is under")
	}
	if Ruler(days("...."), 4) != "" {
		t.Fatalf("a window inside one month has no ruler to draw")
	}
}

// A calendar column is a weekday, so the first row is padded to put the first day
// under the right one.
func TestCalendarKeepsTheWeekdayColumns(t *testing.T) {
	// The 1st of August 2026 is a Saturday, so the first row holds two days and
	// the Saturday column is the sixth.
	rows := Calendar(days("dd" + strings.Repeat(".", 12)))
	if len(rows) < 3 {
		t.Fatalf("expected a heading and two weeks, got %v", rows)
	}
	if !strings.HasPrefix(stripEscapes(rows[0]), strings.Repeat(" ", 7)+"Mon") {
		t.Fatalf("the heading should start on Monday past the label column: %q", rows[0])
	}
	first := stripEscapes(rows[1])
	if runeIndex(first, GlyphDone) != 7+5*4 {
		t.Fatalf("the 1st of August is a Saturday and should be in the sixth column: %q", first)
	}
}

// The picker's key bindings, handed to the parser that will have to read them.
//
// fzf reads its bindings when it starts, so a malformed one is not a key that
// does nothing - it is the picker refusing to open at all, with the listing it
// was standing in front of unreachable behind it. Nothing but running it by hand
// exercises this otherwise.
func TestBindingsParse(t *testing.T) {
	for _, ticks := range []bool{false, true} {
		args := []string{"--ansi", "--delimiter", "\t", "--with-nth", "2.."}
		// A selector with a quote in it, because it is about to be part of a
		// shell command: `orgs habits pick don't` must not take the picker with it.
		args = append(args, Binds("/usr/local/bin/orgs habits -days 56 -strip 12", "don't", ticks)...)
		if _, err := fzf.ParseOptions(true, args); err != nil {
			t.Fatalf("ticks=%v: fzf would refuse to start: %v", ticks, err)
		}
	}
}

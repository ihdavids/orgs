package habits

// The habit tracker's own decisions, in a terminal.
//
// The server works out what happened - which days a habit was kept, how long the
// current run is, how long the best one ever was, how much of what the cadence
// asked for actually happened. What is left here is what a *tracker* has to
// decide, which is not arithmetic but tone: how many days fit on this screen,
// what to say about a run, and when to say nothing.
//
// It is a module of its own, and tested, for the same reason `worg/src/habits.ts`
// is: getting the tone wrong is the way a habit tracker fails, and it is not the
// sort of bug a screenshot catches. A tracker that tells somebody who started on
// Tuesday that they are at 4%, or that congratulates them on a run they broke
// three weeks ago, is one they stop opening.
//
// The rules are worg's, said again here on purpose - the browser and the terminal
// have to agree about what a run is worth, or the same habit reads as two
// different habits depending on where it is looked at. Change one and change
// both.
//
// One thing is the terminal's own. In a browser three day states can be three
// colours; here colour is not a given - a pipe, NO_COLOR, -no-color, a tmux
// status line - so each state is a distinct **character** as well as a distinct
// colour. That is this end's version of worg's hardest-won rule about the strip:
// a missed day has to be as readable as a kept one, or a run of misses reads as
// a gap in the data rather than as the thing the tracker most needs to show.

import (
	"fmt"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// One character per day, and the three of them are three shapes as well as
// three colours.
const (
	GlyphDone = "█"
	GlyphMiss = "░"
	GlyphOk   = "·"
)

// RecentDays is the last n days of a habit's history.
//
// The window is one request for every habit, so it is asked for once at the
// widest any of them will be drawn and trimmed here rather than re-fetched when
// the terminal turns out to be narrower than that.
//
// Trimmed from the **oldest** end, never sampled. A strip that showed every
// other day would fit any width and would be a lie: two days apart is what
// tells a kept habit from a half-kept one, and a strip that drops half the days
// cannot say which it is looking at.
func RecentDays(days []common.HabitDay, n int) []common.HabitDay {
	if len(days) == 0 || n <= 0 {
		return nil
	}
	if n >= len(days) {
		return days
	}
	return days[len(days)-n:]
}

// GlyphOf is the character one day is drawn as, and Ink of its state the colour.
// Kept apart so that a strip can change colour once per run rather than once per
// day.
func GlyphOf(state string) string {
	switch state {
	case "done":
		return GlyphDone
	case "miss":
		return GlyphMiss
	default:
		return GlyphOk
	}
}

func stateInk(state string) string {
	switch state {
	case "done":
		return commands.C(commands.AnsiGreen)
	case "miss":
		return commands.C(commands.AnsiRed)
	default:
		return commands.C(commands.AnsiDim)
	}
}

// Glyph is one day, drawn and coloured. For anywhere the cells are not adjacent
// and there is nothing to run together.
func Glyph(state string) string { return ink(stateInk(state), GlyphOf(state)) }

// CalCell is one day of a calendar: two characters wide, because a week of them
// with air between reads as a row of specks otherwise.
//
// The two solid states are doubled, so a kept week is a bar. The third is a
// single dot with a space in front of it rather than two dots - `··` is an
// ellipsis to anybody reading it, which is a punctuation mark rather than a day
// that was inside its habit's grace.
func CalCell(state string) string {
	if state == "ok" || (state != "done" && state != "miss") {
		return ink(stateInk(state), " "+GlyphOf(state))
	}
	g := GlyphOf(state)
	return ink(stateInk(state), g+g)
}

// Strip is a habit's history as one row of characters, oldest first.
//
// It always ends on today, because the server's window does - which is why no
// cell is marked as today. Marking it would cost a character of contrast in
// every row to say something the footer says once, and a strip whose right-hand
// end is not today is not a thing this ever draws.
//
// The colour changes once per **run** rather than once per day. Eight weeks of
// squares is eight weeks of escape sequences done the naive way - a kept habit
// came out as a hundred and twelve of them to draw fifty-six characters - and
// these rows are redrawn every time a file changes with -w, go through fzf's
// preview as a shell string, and end up in people's status bars.
func Strip(days []common.HabitDay) string {
	var b strings.Builder
	run := ""
	for _, d := range days {
		if c := stateInk(d.State); c != run {
			if run != "" {
				b.WriteString(commands.C(commands.AnsiReset))
			}
			b.WriteString(c)
			run = c
		}
		b.WriteString(GlyphOf(d.State))
	}
	if run != "" {
		b.WriteString(commands.C(commands.AnsiReset))
	}
	return b.String()
}

// Plain is the strip with no escapes in it at all, for measuring and for tests.
func Plain(days []common.HabitDay) string {
	var b strings.Builder
	for _, d := range days {
		b.WriteString(GlyphOf(d.State))
	}
	return b.String()
}

// StreakNote is what to say about one habit's run, and how to colour it.
//
// The branching is the point. A number on its own says nothing: 7 is wonderful
// for a habit whose best is 7 and a disappointment for one whose best is 60, and
// a tracker that cannot tell those apart is a tracker nobody believes.
//
// Tones are "best", "good", "broken" and "new".
func StreakNote(h common.Habit) (string, string) {
	if h.Streak == 0 {
		if h.Total == 0 {
			return "not started", "new"
		}
		// Deliberately not "you lost a 40 day run". The tracker's job at this
		// moment is to make starting again look small, not to price what was
		// lost.
		return "start again", "broken"
	}
	// "best ever" only when there was a previous run to beat.
	//
	// A habit whose whole history is one unbroken run is *technically* on its
	// best ever from day two, and saying so every day makes the words worthless
	// by the time the run is worth something. Total > Streak is the test: there
	// are completions behind this run, so there was a run before it, and
	// matching it is an achievement rather than an arithmetic inevitability. For
	// a first unbroken run the number says it on its own - 40 needs no
	// adjective.
	if h.Streak >= h.Best && h.Total > h.Streak {
		return fmt.Sprintf("%d best ever", h.Streak), "best"
	}
	return fmt.Sprintf("%d", h.Streak), "good"
}

// Ink is the colour a tone is drawn in.
func Ink(tone string) string {
	switch tone {
	case "best":
		return commands.C(commands.AnsiGold)
	case "good":
		return commands.C(commands.AnsiGreen)
	case "due":
		return commands.C(commands.AnsiBlue)
	case "miss":
		return commands.C(commands.AnsiRed)
	default:
		return commands.C(commands.AnsiDim)
	}
}

// Summary is what the header says, and its tone.
//
// Three cases, and the difference between them is the whole difference between a
// tracker that reads as a nag and one that reads as a scoreboard:
//
//   - nothing wants doing and everything that could be done today has been: say
//     so, and stop. This is the state somebody is trying to reach and it should
//     feel like arriving at it.
//   - something is still due: count it plainly. Not "2 overdue!" - the number is
//     the message and the exclamation mark is somebody else's opinion about it.
//   - no habits at all: say how to make one, because an empty listing that
//     explains nothing is a blank strip people learn to ignore.
func Summary(r common.HabitsResult) (string, string) {
	total := len(r.Habits)
	if total == 0 {
		return "no habits yet — a heading with :STYLE: habit and a repeating SCHEDULED is one", "none"
	}
	if r.DueToday == 0 && r.DoneToday > 0 {
		return fmt.Sprintf("all done — %d of %d today", r.DoneToday, total), "done"
	}
	if r.DueToday == 0 {
		return "nothing due today", "done"
	}
	return fmt.Sprintf("%d to do · %d of %d done today", r.DueToday, r.DoneToday, total), "due"
}

// ShortLine is the whole tracker as one line, for a status bar.
//
// A status bar segment with nothing to say says nothing rather than taking up
// room to say so - the same rule `orgs clocks -short` follows. What is left when
// there is something to say is the smallest useful pair: how many want doing,
// and the run worth being reminded of.
func ShortLine(r common.HabitsResult) string {
	if len(r.Habits) == 0 {
		return ""
	}
	if r.DueToday == 0 {
		if r.BestStreak > 1 {
			return fmt.Sprintf("✓ habits · %d", r.BestStreak)
		}
		return "✓ habits"
	}
	if r.BestStreak > 1 {
		return fmt.Sprintf("%d habits · %d", r.DueToday, r.BestStreak)
	}
	return fmt.Sprintf("%d habits", r.DueToday)
}

// Cadence is a habit's schedule in words.
//
// `.+1d/3d` is not something to put in front of somebody who has never read the
// org manual, and it is what the file says.
//
// A heading marked `:STYLE: habit` whose schedule does not repeat has no cadence,
// and saying "every day" about one is inventing a schedule it has not got - the
// pane would then say "every day" and "no schedule" three lines apart. The server
// reports `Every` as 1 for these because a zero would divide, which is why the
// question has to be asked of the repeater rather than of the interval. (worg's
// `cadence()` does not make this distinction and should: the two are meant to say
// the same thing about the same habit.)
func Cadence(h common.Habit) string {
	if h.Repeater == "" {
		return "does not repeat"
	}
	var every string
	switch {
	case h.Every <= 1:
		every = "every day"
	case h.Every == 7:
		every = "every week"
	case h.Every%7 == 0:
		every = fmt.Sprintf("every %d weeks", h.Every/7)
	default:
		every = fmt.Sprintf("every %d days", h.Every)
	}
	if h.Slack > h.Every {
		return fmt.Sprintf("%s, late after %d days", every, h.Slack)
	}
	return every
}

// RatePct is how much of what the cadence asked for actually happened.
//
// An em dash rather than 0% for a habit that has never been done: nought per
// cent is a judgement about a habit that has not been given the chance to be
// judged, and the streak column already says "not started".
func RatePct(h common.Habit) string {
	if h.Total == 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", int(h.Rate*100+0.5))
}

// ---------------------------------------------------------------------------
// The month ruler
// ---------------------------------------------------------------------------

type Tick struct {
	At    int
	Label string
}

// MonthTicks says where to put a month label under a strip of days.
//
// A strip of characters is unreadable without a date on it somewhere, and one
// label per month is the most that fits. The label goes at the *first* day of
// each month that appears, except when that day is so near the right-hand end
// that the text would run off it.
func MonthTicks(days []common.HabitDay, minGapFromEnd int) []Tick {
	out := []Tick{}
	last := ""
	for i := 0; i < len(days); i++ {
		if len(days[i].Date) < 7 {
			continue
		}
		month := days[i].Date[:7]
		if month == last {
			continue
		}
		last = month
		// The first character of the strip is nearly always mid-month, so
		// labelling it would put a month name against a day that is not its
		// first - which is worse than no label at all.
		if i == 0 {
			continue
		}
		if i > len(days)-1-minGapFromEnd {
			continue
		}
		out = append(out, Tick{At: i, Label: MonthName(days[i].Date)})
	}
	return out
}

var months = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func MonthName(iso string) string {
	if len(iso) < 7 {
		return ""
	}
	var m int
	if _, err := fmt.Sscanf(iso[5:7], "%d", &m); err != nil || m < 1 || m > 12 {
		return ""
	}
	return months[m-1]
}

// Ruler is the line that goes under the strips: one label per month, in the
// columns the strip itself occupies.
//
// One ruler for the whole listing rather than one per habit. Every habit's
// window is the same days - the server answers about all of them at once - so a
// ruler per row would be the same row of month names repeated once per habit,
// which is how a listing of eight habits becomes sixteen lines saying eight
// things. worg draws it per row because there it is the only place it can go.
func Ruler(days []common.HabitDay, minGapFromEnd int) string {
	ticks := MonthTicks(days, minGapFromEnd)
	if len(ticks) == 0 {
		return ""
	}
	line := []rune(strings.Repeat(" ", len(days)))
	for _, t := range ticks {
		for i, r := range t.Label {
			if t.At+i < len(line) {
				line[t.At+i] = r
			}
		}
	}
	return strings.TrimRight(string(line), " ")
}

// ---------------------------------------------------------------------------
// The calendar
// ---------------------------------------------------------------------------

// Calendar draws a habit's window as weeks down the page, which is what one
// habit on its own is worth showing as.
//
// The row strip is right for a listing - eight habits beside each other, each
// one line - and wrong for the question you ask about a single habit, which is
// *when* rather than *how often*: which Tuesdays, whether the gaps are weekends.
// A week per row answers that and a strip cannot.
//
// Weeks start on Monday, and the first row is padded so that a column is always
// the same weekday. The label is the week's Monday, because a month name against
// a row that straddles two months has to pick one and would be wrong half the
// time.
func Calendar(days []common.HabitDay) []string {
	if len(days) == 0 {
		return nil
	}
	const cell = 4 // two characters of glyph and two of air
	head := strings.Repeat(" ", 7)
	for _, d := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		head += d + " "
	}
	out := []string{commands.C(commands.AnsiDim) + strings.TrimRight(head, " ") + commands.C(commands.AnsiReset)}

	// Where in its week the first day sits, so that a column is a weekday.
	first, err := time.ParseInLocation("2006-01-02", days[0].Date, time.Local)
	if err != nil {
		return out
	}
	lead := (int(first.Weekday()) + 6) % 7 // Monday is 0

	row, label, col := "", "", 0
	flush := func() {
		if row == "" {
			return
		}
		out = append(out, fmt.Sprintf("%s%s",
			ink(commands.C(commands.AnsiDim), fmt.Sprintf("%-7s", label)),
			strings.TrimRight(row, " ")))
		row, label = "", ""
	}
	weekLabel := func(d time.Time) string {
		monday := d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
		return fmt.Sprintf("%s %02d", MonthName(monday.Format("2006-01-02")), monday.Day())
	}
	for i := 0; i < lead; i++ {
		row += strings.Repeat(" ", cell)
		col++
	}
	for _, d := range days {
		when, perr := time.ParseInLocation("2006-01-02", d.Date, time.Local)
		if perr != nil {
			continue
		}
		if label == "" {
			label = weekLabel(when)
		}
		row += CalCell(d.State) + "  "
		col++
		if col == 7 {
			flush()
			col = 0
		}
	}
	flush()
	return out
}

// StreakCols is StreakNote split into a number and the words about it, so a
// listing can right-align the numbers.
//
// A column of numbers that do not line up is the difference between a table and
// a list of sentences, and the number is the thing being compared down the
// column. The words go beside it, left aligned, because "best ever" is about one
// row rather than about the column.
func StreakCols(h common.Habit) (string, string, string) {
	text, tone := StreakNote(h)
	switch tone {
	case "best":
		return fmt.Sprintf("%d", h.Streak), "best ever", tone
	case "good":
		return fmt.Sprintf("%d", h.Streak), "", tone
	default:
		return "", text, tone
	}
}

// Legend is the line that says what the characters mean.
//
// Once, at the foot of the listing, rather than never. Three shades of block are
// not self explanatory, and the difference between "missed" and "in hand" is the
// whole reason there are three of them rather than two.
func Legend(r common.HabitsResult, shown int) string {
	when := r.Today
	if t, err := time.ParseInLocation("2006-01-02", r.Today, time.Local); err == nil {
		when = t.Format("Mon 2 Jan")
	}
	return fmt.Sprintf("%d days to %s   %s kept   %s missed   %s in hand",
		shown, when, GlyphDone, GlyphMiss, GlyphOk)
}

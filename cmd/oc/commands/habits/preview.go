package habits

// One habit drawn on its own, and the picker that draws it beside the list.
//
// The machinery - the fzf run, finding this binary again, the boxes that are open
// on the right - is `cmd/oc/commands/picker.go`, shared with the code, links,
// tables and records pickers so that they behave identically and a fix lands
// once.
//
// What the pane draws is what you opened the list to ask, which for a habit is
// not "how often" - the row already said that - but **when**: which Tuesdays, and
// whether the gaps are weekends. So the pane is a calendar, weeks down the page,
// where the row was a strip along it.
//
// A habit is addressed by **its own hash**, the way a record is and unlike a
// link, which has no identity and is addressed by its position in the list. So
// the pane still finds the right habit after the list has been filtered under it.
// The hash goes into the child command as it stands rather than through
// `HashPath`, because it is going to `-hash` as an argument rather than into a
// url path - it is only a url segment that has to be encoded a second time.

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// The pane
// ---------------------------------------------------------------------------

// detail draws one habit: what it is, how it is going, and its calendar.
func detail(h common.Habit, width int) {
	if width < 30 {
		width = 30
	}

	commands.OpenBox(h.Headline, width)
	line := Cadence(h)
	if h.Repeater != "" {
		line += fmt.Sprintf("  %s%s%s", commands.C(commands.AnsiDim), h.Repeater,
			commands.C(commands.AnsiReset))
	}
	commands.BoxLine(line)
	where := fmt.Sprintf("%s%s:%d%s", commands.C(commands.AnsiDim),
		commands.BaseName(h.Filename), h.LineNum+1, commands.C(commands.AnsiReset))
	if h.Status != "" {
		where += "  " + h.Status
	}
	if len(h.Tags) > 0 {
		where += fmt.Sprintf("  %s:%s:%s", commands.C(commands.AnsiCyan),
			strings.Join(h.Tags, ":"), commands.C(commands.AnsiReset))
	}
	commands.BoxLine(where)
	commands.BoxLine(due(h))
	commands.CloseBox(width)

	commands.OpenBox("how it is going", width)
	text, tone := StreakNote(h)
	run := fmt.Sprintf("%s%s%s", Ink(tone), text, commands.C(commands.AnsiReset))
	if h.Streak > 0 {
		run = fmt.Sprintf("%son a %s day run%s", Ink(tone), text, commands.C(commands.AnsiReset))
		if tone == "best" {
			run = fmt.Sprintf("%son a %d day run — best ever%s", Ink(tone), h.Streak,
				commands.C(commands.AnsiReset))
		}
	}
	commands.BoxLine(run)
	if h.Total > 0 {
		commands.BoxLine(fmt.Sprintf("%s of what it asked for   %s%d day%s kept in all%s",
			RatePct(h), commands.C(commands.AnsiDim), h.Total, plural(h.Total),
			commands.C(commands.AnsiReset)))
		commands.BoxLine(fmt.Sprintf("%slast kept %s%s", commands.C(commands.AnsiDim),
			pretty(h.LastDone), commands.C(commands.AnsiReset)))
		if h.Best > h.Streak {
			commands.BoxLine(fmt.Sprintf("%sbest run %d days%s", commands.C(commands.AnsiDim),
				h.Best, commands.C(commands.AnsiReset)))
		}
	}
	commands.CloseBox(width)

	if len(h.Days) > 0 {
		commands.OpenBox(fmt.Sprintf("the last %d days", len(h.Days)), width)
		for _, l := range Calendar(h.Days) {
			commands.BoxLine(l)
		}
		commands.BoxLine("")
		commands.BoxLine(fmt.Sprintf("%s%s kept   %s missed   %s in hand%s",
			commands.C(commands.AnsiDim), GlyphDone, GlyphMiss, GlyphOk,
			commands.C(commands.AnsiReset)))
		commands.CloseBox(width)
	}
}

// Where the habit stands today, said rather than left to be read off the
// calendar - it is the one thing somebody looking at a habit wants first.
func due(h common.Habit) string {
	switch {
	case h.DoneToday:
		return fmt.Sprintf("%s✓ done today%s", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset))
	case h.Missed:
		return fmt.Sprintf("%s○ overdue — it was due %s%s", commands.C(commands.AnsiRed),
			pretty(h.Scheduled), commands.C(commands.AnsiReset))
	case h.Due:
		return fmt.Sprintf("%s○ due today%s", commands.C(commands.AnsiBlue),
			commands.C(commands.AnsiReset))
	case h.Scheduled != "":
		return fmt.Sprintf("%snext %s%s", commands.C(commands.AnsiDim),
			pretty(h.Scheduled), commands.C(commands.AnsiReset))
	default:
		return fmt.Sprintf("%sno schedule%s", commands.C(commands.AnsiDim),
			commands.C(commands.AnsiReset))
	}
}

func pretty(iso string) string {
	if iso == "" {
		return "never"
	}
	t, err := time.ParseInLocation("2006-01-02", iso, time.Local)
	if err != nil {
		return iso
	}
	// Counted in days rather than in hours. A date is a day, and the difference
	// between two of them has to be whole - `time.Since` on tomorrow is minus
	// something-and-a-bit, which truncates to nought and calls tomorrow today.
	// Rounded rather than truncated for the two days a year that are 23 or 25
	// hours long.
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	days := int(math.Round(from.Sub(t).Hours() / 24))
	when := t.Format("Mon 2 Jan")
	switch {
	case days == 0:
		return when + " (today)"
	case days == 1:
		return when + " (yesterday)"
	case days > 1:
		return fmt.Sprintf("%s (%d days ago)", when, days)
	case days == -1:
		return when + " (tomorrow)"
	default:
		return fmt.Sprintf("%s (in %d days)", when, -days)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// The pane, as fzf runs it: a second run of this binary against the same server.
func (self *Habits) preview(core *commands.Core) {
	// Drawn for fzf rather than for a terminal, so the colour has to be said
	// explicitly - stdout here is a pipe.
	commands.PickerOutput()
	h := self.resolve(core, "")
	detail(h, commands.PaneWidth())
}

// ---------------------------------------------------------------------------
// The picker
// ---------------------------------------------------------------------------

func (self *Habits) pick(core *commands.Core, sel string) {
	// Nothing is reading a chooser. A pipe, a -json run or a cron job gets the
	// tracker, which is the same question answered in a form that caller can
	// use rather than a full-screen application it can neither see nor answer.
	if !commands.Interactive() {
		self.track(core, sel)
		return
	}
	res := self.index(core, sel)
	if len(res.Habits) == 0 {
		fmt.Fprintln(os.Stderr, "no habits matched")
		return
	}
	for _, h := range self.choose(core, res.Habits, sel, true) {
		if self.Open {
			core.LaunchEditor(h.Filename, h.LineNum+1)
			continue
		}
		fmt.Println()
		detail(h, commands.PaneWidth())
		fmt.Println()
	}
}

// Put the list up and hand back what was chosen. Split out from `pick` so that
// `resolve` can use it when a name matched several habits.
//
// `ticks` is whether ticking one off from inside the list is offered: it is when
// somebody opened the picker to work through their habits, and it is not when the
// picker is standing in for a question some other verb asked. A tick offered
// there would be a second write in the middle of resolving the target of the
// first.
func (self *Habits) choose(core *commands.Core, rows []common.Habit, sel string, ticks bool) []common.Habit {
	self2, err := commands.SelfCommand(core)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orgs habits: no preview (%v)\n", err)
	}

	strip := self.stripWidth()

	lines := make([]string, 0, len(rows))
	for _, h := range rows {
		lines = append(lines, commands.PickLine([]string{h.Hash}, pickLine(h, strip)))
	}

	header := "enter: show · ctrl-o: edit · ctrl-/: hide pane"
	if ticks {
		header = "enter: show · ctrl-d: tick off · ctrl-u: untick today · ctrl-o: edit · ctrl-/: hide pane"
	}
	opts := commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "habit> ",
		Header:        header,
	}
	if self2 != "" {
		// Every flag that changes what is in the answer has to be passed on, or
		// the child is looking at a different list from its parent.
		args := self2 + " habits" + self.flagArgs() + fmt.Sprintf(" -strip %d", strip)
		opts.Preview = args + " preview -hash {1}"
		opts.Extra = Binds(args, sel, ticks)
	}

	out := []common.Habit{}
	for _, chosen := range commands.Pick(opts) {
		addr, ok := commands.Address(chosen, 1)
		if !ok {
			continue
		}
		for _, h := range rows {
			if h.Hash == addr[0] {
				out = append(out, h)
				break
			}
		}
	}
	return out
}

// Binds are the keys the picker answers to, as fzf's own option strings.
//
// Built here rather than inline so that a test can hand them to fzf's parser:
// a bad binding is a refusal at startup with no picker and no listing, and it is
// the one part of this that nothing but running it interactively exercises.
//
// Two of them write: `ctrl-d` ticks a habit off and `ctrl-u` takes today's tick
// off again. They are a pair on purpose - the moment somebody learns they pressed
// the wrong key is the moment they need the other one, and a picker with a key that
// writes and no key that unwrites is one you cannot afford to explore.
//
// Both reload the list afterwards, because the square filling in (or emptying) is
// the whole point: a row that has gone stale under a write is the tracker lying
// about the thing it was just asked to change.
func Binds(args, sel string, ticks bool) []string {
	out := []string{"--bind", "ctrl-o:execute-silent(" + args + " open -hash {1})"}
	if !ticks {
		return out
	}
	reload := "+reload(" + args + " lines " + commands.Shq(sel) + ")"
	return append(out,
		"--bind", "ctrl-d:execute("+quiet(args+" done -hash {1} -yes")+")"+reload,
		"--bind", "ctrl-u:execute("+quiet(args+" untick -hash {1}")+")"+reload,
	)
}

// quiet runs a child of the picker so that it says nothing when it worked and
// waits for a key when it did not.
//
// `execute` rather than `execute-silent`, and the output thrown away rather than
// shown. A write that worked prints a line nobody needs - the row it changed is
// about to be redrawn saying the same thing - and is gone before the screen has
// finished redrawing. One that could not has something to say, and said silently
// it would look exactly like a square that declined to change.
func quiet(cmd string) string {
	return cmd + ` >/dev/null || { printf '\n── press enter ──'; read -r _; }`
}

// The flags that decide which habits are in the list, spelled the way a child
// process has to be told them.
func (self *Habits) flagArgs() string {
	out := fmt.Sprintf(" -days %d", self.Days)
	if self.File != "" {
		out += " -file " + commands.Shq(self.File)
	}
	if self.Due {
		out += " -due"
	}
	return out
}

// lines prints the picker's own list, for fzf to reload after a tick.
//
// A hidden subcommand rather than fzf re-running the picker's own listing,
// because the two have to be the same list in the same order: reload is meant to
// put the row back where it was with its square filled in, and a listing built
// any other way would move it.
func (self *Habits) lines(core *commands.Core, sel string) {
	commands.PickerOutput()
	strip := self.stripWidth()
	res := self.index(core, sel)
	for _, h := range res.Habits {
		fmt.Println(commands.PickLine([]string{h.Hash}, pickLine(h, strip)))
	}
}

// One line of the picker: where it stands, what it is, a short strip, and the
// run it is on.
//
// The strip is here as well as in the pane because it is what the eye runs down
// looking for the habit it came about - a broken run is a shape rather than a
// number, and a list of names with numbers beside them makes somebody read every
// row. Shorter than the tracker's, because the list is sharing the terminal with
// the pane.
func pickLine(h common.Habit, strip int) string {
	line := markOf(h) + "  " +
		ink(inkName(h), padRight(commands.Ellipsis(h.Headline, 24), 24))
	if strip > 0 {
		line += " " + Strip(RecentDays(h.Days, strip))
	}
	num, words, tone := StreakCols(h)
	if num != "" {
		line += fmt.Sprintf(" %s%3s%s", Ink(tone), num, commands.C(commands.AnsiReset))
	} else {
		line += "    "
	}
	if words != "" {
		line += fmt.Sprintf(" %s%s%s", Ink(tone), words, commands.C(commands.AnsiReset))
	}
	// The file is in the display as well as being the habit's home, so that it
	// can be searched for: what is shown is also what is searched.
	line += fmt.Sprintf(" %s%s%s", commands.C(commands.AnsiDim),
		commands.BaseName(h.Filename), commands.C(commands.AnsiReset))
	return line
}

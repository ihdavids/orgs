package habits

// orgs habits - how your habits are going, and ticking one off.
//
//	orgs habits                     the tracker
//	orgs habits -due                only what wants doing today
//	orgs habits -short              one line, for a status bar
//	orgs habits -w                  redraw it as the files change
//	orgs habits show meditate       one habit, as a calendar
//	orgs habits done meditate       tick it off
//	orgs habits done -all           tick off everything due today
//	orgs habits untick meditate     take today's tick off again
//	orgs habits pick                fzf, with the habit drawn beside the list
//	orgs habits -json               for a program to read
//
// This is worg's Agenda-tab habit tracker at a prompt. The server already
// answers the whole question in one request - `GET /habits` gives every
// `:STYLE: habit` heading, its cadence, the current run, the best run it has
// ever been on, how much of what the cadence asked for actually happened, and
// one entry per day for the last eight weeks - so this command is a drawing and
// one write, and the tone decisions are in `view.go` beside their tests.
//
// Two things are different here from the browser, and both are the terminal
// being better at this rather than worse:
//
//   - **The default is the tracker, not a picker.** `orgs tables` and `orgs code`
//     open a picker because they are about one of hundreds of things; somebody
//     has eight habits, and all eight fit on the screen at once. A picker as the
//     front door would be a full-screen application standing between somebody
//     and a listing that was already the answer. `orgs habits pick` is there for
//     when choosing one is what you actually came for.
//   - **The month ruler is drawn once** at the foot of the listing rather than
//     under every row. Every habit's window is the same days, so a ruler per row
//     is the same row of month names repeated once per habit.
//
// Ticking one off is `POST /status/change`, which is the same write every other
// client makes: the server stamps the logbook line this endpoint reads back,
// moves the repeater on, and puts the keyword back to a live state. So the answer
// to "did that work" is the next read of `/habits` rather than anything guessed
// at here - which is why a tick is followed by a re-read rather than by patching
// the row on screen.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd"
	"golang.org/x/term"
)

type Habits struct {
	fset *flag.FlagSet
	tf   commands.TargetFlags

	Days  int
	Watch bool
	Short bool
	Due   bool
	File  string
	Open  bool
	Note  string
	Cal   bool
	Strip int
}

func (self *Habits) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Habits) StartPlugin(manager *common.PluginManager)         {}
func (self *Habits) HelpGroup() string                                 { return "Habits" }

func (self *Habits) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.IntVar(&self.Days, "days", 56, "how many days of history to ask for (7 to 371)")
	fset.BoolVar(&self.Watch, "w", false, "redraw it when a file changes")
	fset.BoolVar(&self.Short, "short", false, "one line, for a status bar")
	fset.BoolVar(&self.Due, "due", false, "only the habits that want doing today")
	fset.StringVar(&self.File, "file", "", "only habits in this file")
	fset.BoolVar(&self.Open, "open", false, "open what was picked in your editor instead")
	fset.BoolVar(&self.Cal, "cal", false, "draw each habit as a calendar rather than a strip")
	fset.StringVar(&self.Note, "note", "", "a note to keep with the state change")
	// How wide the strip is in a picker line. Worked out from the terminal when
	// nobody says, and said explicitly to the child processes fzf runs - the
	// parent and the reload have to agree or a row changes width when it is
	// written to, which is the one moment somebody is looking straight at it.
	fset.IntVar(&self.Strip, "strip", -1, "how many days of history a picker line shows")
}

func (self *Habits) Exec(core *commands.Core) {
	// Taken once: FreeArgs consumes the arguments as it parses, so asking the
	// flag set for them afterwards gets nothing.
	words := commands.FreeArgs(self.fset)
	sub := ""
	if len(words) > 0 {
		switch strings.ToLower(words[0]) {
		case "ls", "list", "show", "cat", "done", "tick", "do", "pick",
			"untick", "untoggle", "undone", "clear",
			"open", "edit", "preview", "lines":
			sub = strings.ToLower(words[0])
			words = words[1:]
		}
	}
	sel := strings.TrimSpace(strings.Join(words, " "))

	switch sub {
	case "show", "cat":
		self.show(core, sel)
	case "done", "tick", "do":
		self.done(core, sel)
	case "untick", "untoggle", "undone", "clear":
		self.untick(core, sel)
	case "open", "edit":
		self.edit(core, sel)
	case "pick":
		self.pick(core, sel)
	case "preview":
		self.preview(core)
	case "lines":
		self.lines(core, sel)
	default:
		// `orgs habits` and `orgs habits ls` are the same thing. The tracker is
		// the answer to the question the command's name asks, and a listing that
		// is already the answer should not be behind a subcommand.
		self.track(core, sel)
	}
}

// ---------------------------------------------------------------------------
// Reading
// ---------------------------------------------------------------------------

func (self *Habits) read(core *commands.Core) common.HabitsResult {
	days := self.Days
	if days < 7 {
		days = 7
	}
	if days > 371 {
		days = 371
	}
	res, err := commands.SendReceiveGetErr[common.HabitsResult](core, "habits",
		map[string]string{"days": strconv.Itoa(days)})
	if err != nil {
		commands.Fail("orgs habits: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs habits: %s", firstNonEmpty(res.Msg, "could not read your habits"))
	}
	return res
}

// The habits, filtered the way the flags and the words asked for.
//
// The filtering is here rather than asked for because `/habits` takes a window
// and nothing else, and everything worth matching on - the headline, the file,
// the tags - is already in the answer. The same reasoning as `orgs tables`.
func (self *Habits) index(core *commands.Core, sel string) common.HabitsResult {
	res := self.read(core)
	kept := res.Habits[:0:0]
	for _, h := range res.Habits {
		if self.File != "" && !sameFile(h.Filename, self.File) {
			continue
		}
		if self.Due && !h.Due {
			continue
		}
		kept = append(kept, h)
	}
	if sel != "" {
		kept = match(kept, sel)
	}
	res.Habits = kept
	// The counts came from the whole database, and a filtered listing whose
	// header still says "2 to do" of habits it is not showing is a header about
	// something else. Counted again over what survived.
	res.DueToday, res.DoneToday, res.BestStreak = 0, 0, 0
	for _, h := range kept {
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
	return res
}

func sameFile(have, want string) bool {
	return strings.EqualFold(have, want) ||
		strings.EqualFold(commands.BaseName(have), want) ||
		strings.HasSuffix(strings.ToLower(have), strings.ToLower(want))
}

// match narrows the list the way somebody typing at it means.
//
// An exact headline first, then a substring, then the fuzzy matcher the dnd
// chooser, the tui's filter and worg's palette all share - so the letters that
// find a heading in one of them find it here. The order matters: a habit called
// "Read" must not be beaten to it by "Reading glasses prescription" because the
// fuzzy score happened to like the longer one.
func match(hs []common.Habit, sel string) []common.Habit {
	for _, h := range hs {
		if strings.EqualFold(h.Headline, sel) {
			return []common.Habit{h}
		}
	}
	sub := hs[:0:0]
	for _, h := range hs {
		if strings.Contains(strings.ToLower(hay(h)), strings.ToLower(sel)) {
			sub = append(sub, h)
		}
	}
	if len(sub) > 0 {
		return sub
	}
	type scored struct {
		h common.Habit
		n int
	}
	var out []scored
	for _, h := range hs {
		if n, ok := dnd.FuzzyScore(sel, hay(h)); ok {
			out = append(out, scored{h, n})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].n > out[b].n })
	kept := hs[:0:0]
	for _, s := range out {
		kept = append(kept, s.h)
	}
	return kept
}

func hay(h common.Habit) string {
	return strings.Join(append([]string{h.Headline, commands.BaseName(h.Filename)}, h.Tags...), " ")
}

// resolve turns what somebody typed into one habit: a hash, a name, or - failing
// both, and only with somebody there to answer - a chooser.
func (self *Habits) resolve(core *commands.Core, sel string) common.Habit {
	all := self.index(core, "")
	if self.tf.Hash != "" {
		for _, h := range all.Habits {
			if h.Hash == self.tf.Hash {
				return h
			}
		}
		commands.Fail("orgs habits: no habit with hash %s", self.tf.Hash)
	}
	hs := all.Habits
	if sel != "" {
		hs = match(hs, sel)
		if len(hs) == 0 {
			commands.Fail("orgs habits: no habit matching %q", sel)
		}
	}
	if len(hs) == 0 {
		commands.Fail("orgs habits: no habits — a heading with :STYLE: habit and a repeating SCHEDULED is one")
	}
	if len(hs) == 1 {
		return hs[0]
	}
	if self.tf.At > 0 {
		if self.tf.At > len(hs) {
			commands.Fail("orgs habits: %q matched %d habits, there is no %dth",
				sel, len(hs), self.tf.At)
		}
		return hs[self.tf.At-1]
	}
	// More than one match is a question, not an assumption - the rule every
	// write verb here follows.
	if !commands.Interactive() {
		commands.Fail("orgs habits: %q matches %d habits; name one, or -at N", sel, len(hs))
	}
	chosen := self.choose(core, hs, sel, false)
	if len(chosen) == 0 {
		os.Exit(0)
	}
	return chosen[0]
}

// ---------------------------------------------------------------------------
// The tracker
// ---------------------------------------------------------------------------

func (self *Habits) track(core *commands.Core, sel string) {
	if !self.Watch {
		self.draw(self.index(core, sel))
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	self.draw(self.index(core, sel))

	// Two things move this listing, and only one of them is an event. A habit
	// ticked off anywhere - Emacs, worg, another terminal - is a file change;
	// the window sliding at midnight is nothing happening at all and has to be
	// redrawn on a timer, or a tracker left up overnight draws yesterday all
	// morning.
	changed := make(chan struct{}, 8)
	go func() {
		common.Events(ctx, &core.Rest, common.EventOpts{Kinds: []string{"reload"}},
			func(e common.OrgEvent) bool {
				if e.Kind == "hello" {
					return true
				}
				select {
				case changed <- struct{}{}:
				default:
				}
				return true
			})
	}()

	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	today := time.Now().Format("2006-01-02")
	for {
		select {
		case <-ctx.Done():
			return
		case <-changed:
		case <-tick.C:
			// Nothing has happened, so there is nothing to redraw unless the
			// day itself has turned over.
			now := time.Now().Format("2006-01-02")
			if now == today {
				continue
			}
			today = now
		}
		self.draw(self.index(core, sel))
	}
}

func (self *Habits) draw(res common.HabitsResult) {
	if self.Short {
		// A status bar wants one line whatever else was asked for, and `-json`
		// still beats it: something reading json asked for the answer rather
		// than for a line about it.
		if commands.RenderOne(res, nil) {
			return
		}
		fmt.Println(ShortLine(res))
		return
	}
	if commands.Render(res.Habits, nil) {
		return
	}
	if self.Watch && commands.Colour() {
		// Home, then clear. A tracker being watched is one screen redrawn, not a
		// log of screens.
		fmt.Print("\033[H\033[2J")
	}
	if self.Cal {
		self.drawCalendars(res)
		return
	}
	drawTracker(res)
}

// The listing.
//
// Five columns: whether it is done, what it is, its history, the run it is on,
// and how much of what it asked for happened. Laid out to the terminal's width,
// with the history taking whatever is left over - it is the column that can be
// any width and still say the same thing, and the only one that can.
//
// The cells are built as plain text and padded before they are coloured, because
// an escape sequence is characters that are not there: padding a string that
// already holds one measures the escape and the column comes out short by
// however many bytes the colour was.
func drawTracker(res common.HabitsResult) {
	head, tone := Summary(res)
	width := TermWidth()
	pad := strings.Repeat(" ", margin)

	if len(res.Habits) == 0 {
		fmt.Printf("\n%s%sHabits%s  %s%s%s\n\n", pad,
			commands.C(commands.AnsiBold), commands.C(commands.AnsiReset),
			Ink(tone), head, commands.C(commands.AnsiReset))
		return
	}

	// The name column is as wide as the longest name it has to hold, within
	// reason. Measured rather than fixed, because eight habits called "Read" and
	// "Stretch" should not be laid out around the possibility of a ninth with a
	// paragraph for a title.
	name := 10
	for _, h := range res.Habits {
		if n := commands.RuneLen(h.Headline); n > name {
			name = n
		}
	}
	if name > 26 {
		name = 26
	}

	// The history takes whatever is left, up to the window that was asked for.
	strip := width - rowWidth(name)
	if n := len(firstDays(res)); strip > n {
		strip = n
	}
	if strip < 7 {
		// Too narrow to say anything true about a history, so it says nothing
		// about one rather than showing a sliver dressed up as eight weeks.
		strip = 0
	}

	header(res, head, tone, width)

	var window []common.HabitDay
	kept := false
	for _, h := range res.Habits {
		// The server sorts what still wants doing to the top, so there is one
		// place in the list where the outstanding habits end and the kept ones
		// begin. A line of air there and the listing reads as two lists - to do,
		// and done - which is what somebody is looking at it to find out.
		if h.DoneToday && !kept {
			kept = true
			fmt.Println()
		}
		days := RecentDays(h.Days, strip)
		if len(days) > len(window) {
			window = days
		}
		num, words, ntone := StreakCols(h)

		row := pad + markOf(h) + gap +
			ink(inkName(h), padRight(commands.Ellipsis(h.Headline, name), name)) + gap
		if strip > 0 {
			row += Strip(days) + strings.Repeat(" ", strip-len(days)) + gap
		}
		row += ink(Ink(ntone), padLeft(num, wStreak)) + " " +
			ink(Ink(ntone), padRight(words, wNote)) + gap +
			ink(commands.C(commands.AnsiDim), padLeft(RatePct(h), wRate))
		fmt.Println(row)
	}

	// The ruler goes under the strips it is about; the rule closes the listing
	// whether or not there were strips, because a listing with a line over it and
	// nothing under it is a table that has not finished being drawn.
	if strip > 0 && len(window) > 0 {
		if r := Ruler(window, 4); r != "" {
			fmt.Printf("%s%s\n", strings.Repeat(" ", stripAt(name)),
				ink(commands.C(commands.AnsiDim), r))
		}
	}
	rule(width)
	if strip > 0 && len(window) > 0 {
		fmt.Printf("%s%s\n", pad, ink(commands.C(commands.AnsiDim), Legend(res, len(window))))
	}
	fmt.Println()
}

// The header: what this is on the left, how it is going beside it, and the
// longest run anybody is currently on pushed out to the right - which is the one
// number worth putting where it is seen without asking for anything.
func header(res common.HabitsResult, head, tone string, width int) {
	pad := strings.Repeat(" ", margin)
	left := "Habits  " + head
	run := ""
	if res.BestStreak > 1 {
		run = fmt.Sprintf("%d day run", res.BestStreak)
	}
	// Pushed to the right-hand end, and dropped altogether when there is not
	// room to push it: two things a space apart on a narrow terminal read as one
	// sentence, and "done today 41 day run" is a worse thing to have said than
	// nothing.
	gap := width - margin - commands.RuneLen(left) - commands.RuneLen(run) - margin
	if gap < 3 {
		run = ""
	}
	fmt.Println()
	fmt.Printf("%s%sHabits%s  %s%s\n", pad,
		commands.C(commands.AnsiBold), commands.C(commands.AnsiReset),
		ink(Ink(tone), head),
		rightOf(ink(commands.C(commands.AnsiGold), run), gap))
	rule(width)
}

// The layout. One row is a mark, a name, a history, the run and the rate, with
// two spaces between each - wide enough to read as columns without drawing any
// lines down the page.
const (
	margin  = 2
	wMark   = 1
	wStreak = 3
	wNote   = 9 // "best ever"
	wRate   = 4 // "100%"
	gap     = "  "
)

// Everything a row spends that is not the history.
func rowWidth(name int) int {
	return margin + wMark + 2 + name + 2 + 2 + wStreak + 1 + wNote + 2 + wRate + margin
}

// Where the history starts, so that the ruler lines up under it.
func stripAt(name int) int { return margin + wMark + 2 + name + 2 }

func rule(width int) {
	fmt.Printf("%s%s\n", strings.Repeat(" ", margin),
		ink(commands.C(commands.AnsiDim), commands.Rule(width-2*margin)))
}

// ink wraps text in a colour and turns it off again. An empty colour - which is
// what C() hands back when colour is off - wraps nothing in nothing.
func ink(colour, text string) string {
	if colour == "" || strings.TrimSpace(text) == "" {
		return text
	}
	return colour + text + commands.C(commands.AnsiReset)
}

// The habits one calendar each, for -cal.
func (self *Habits) drawCalendars(res common.HabitsResult) {
	head, tone := Summary(res)
	header(res, head, tone, TermWidth())
	for _, h := range res.Habits {
		fmt.Println()
		detail(h, TermWidth())
	}
}

func firstDays(res common.HabitsResult) []common.HabitDay {
	for _, h := range res.Habits {
		if len(h.Days) > 0 {
			return h.Days
		}
	}
	return nil
}

// Where a habit stands, as one character.
//
// Done today is a tick and it is green, because that is the one square of this
// listing meant to feel like a reward. Due is an open ring; not due today is
// neither - a habit that is simply not asking for anything today should not be
// drawn as an outstanding job.
func markOf(h common.Habit) string {
	switch {
	case h.DoneToday:
		return ink(commands.C(commands.AnsiGreen), "✓")
	case h.Missed:
		return ink(commands.C(commands.AnsiRed), "○")
	case h.Due:
		return ink(commands.C(commands.AnsiBlue), "○")
	default:
		return ink(commands.C(commands.AnsiDim), "·")
	}
}

func inkName(h common.Habit) string {
	switch {
	case h.Missed:
		return commands.C(commands.AnsiRed)
	case h.Due:
		return commands.C(commands.AnsiBold)
	case h.DoneToday:
		return commands.C(commands.AnsiDim)
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// One habit
// ---------------------------------------------------------------------------

func (self *Habits) show(core *commands.Core, sel string) {
	h := self.resolve(core, sel)
	if commands.RenderOne(h, nil) {
		return
	}
	if self.Open {
		core.LaunchEditor(h.Filename, h.LineNum+1)
		return
	}
	fmt.Println()
	detail(h, TermWidth())
	fmt.Println()
}

func (self *Habits) edit(core *commands.Core, sel string) {
	h := self.resolve(core, sel)
	core.LaunchEditor(h.Filename, h.LineNum+1)
}

// ---------------------------------------------------------------------------
// Ticking one off
// ---------------------------------------------------------------------------

// A tick is `POST /status/change` to the heading's own file's first finished
// keyword, which the endpoint hands over on each habit so that a tracker does
// not need a request per habit to find out what DONE is called here.
//
// What the server does with it is the whole loop: it stamps the `State "DONE"`
// line this tracker reads its history back out of, moves the repeater on, and
// puts the keyword back to a live state. That last part looks like a failed
// write unless it is said out loud, which is what the reply's Msg is for.
func (self *Habits) done(core *commands.Core, sel string) {
	var todo []common.Habit
	switch {
	case self.tf.All:
		all := self.index(core, sel)
		for _, h := range all.Habits {
			if h.Due && !h.DoneToday {
				todo = append(todo, h)
			}
		}
		if len(todo) == 0 {
			fmt.Fprintln(os.Stderr, "nothing due today")
			return
		}
		// Several files written on one word deserves the question, and -yes is
		// how a script says it does not need asking.
		if len(todo) > 1 && !commands.Confirm(&self.tf, "tick off %d habits?", len(todo)) {
			return
		}
	default:
		todo = []common.Habit{self.resolve(core, sel)}
	}

	lines := []string{}
	for _, h := range todo {
		if h.DoneKeyword == "" {
			commands.Fail("orgs habits done: %s has no finished keyword to use",
				commands.BaseName(h.Filename))
		}
		if h.DoneToday && !self.tf.Yes && !self.tf.All {
			// Doing it twice in a day is one day's worth as far as the counting
			// goes, so this is harmless - but it is also almost certainly not
			// what was meant, and saying so costs a keystroke.
			if !commands.Confirm(&self.tf, "%s is already done today; again?", h.Headline) {
				continue
			}
		}
		var reply common.Result
		commands.SendReceivePost(core, "status/change",
			&common.TodoItemChange{Hash: h.Hash, Value: h.DoneKeyword, Note: self.Note}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("orgs habits done: could not tick off %s", h.Headline)
		}
		line := fmt.Sprintf("%s%s%s %s", commands.C(commands.AnsiBold), h.Headline,
			commands.C(commands.AnsiReset), nextRun(h))
		if reply.Msg != "" {
			line += fmt.Sprintf(" %s(%s)%s", commands.C(commands.AnsiDim),
				reply.Msg, commands.C(commands.AnsiReset))
		}
		lines = append(lines, line)
	}
	commands.Ok(len(lines), "ticked off", lines)
	// Where somebody learns there is a way back is the moment they did not mean
	// to press it. Said once, after the write, and only when there is somebody at
	// a prompt to read it - `Interactive()` rather than `!Machine()`, because the
	// picker's own ctrl-d is a child process with its output thrown away, and a
	// line of advice about a key the header is already offering would flash up
	// over the list for no reason.
	if len(lines) > 0 && commands.Interactive() {
		fmt.Fprintf(os.Stderr, "%s  not today after all: orgs habits untick %s%s\n",
			commands.C(commands.AnsiDim), commands.Shq(todo[0].Headline),
			commands.C(commands.AnsiReset))
	}
}

// What to say about the run a tick has just extended.
//
// The number the habit was carrying plus one, because the answer the server
// gives back is about the write rather than about the history and re-reading
// `/habits` for one sentence is a request for a sentence. The same rules decide
// what it is worth saying: a run of one is a run of one and needs no adjective.
func nextRun(h common.Habit) string {
	n := h.Streak + 1
	if h.DoneToday {
		n = h.Streak
	}
	if n <= 1 {
		return ""
	}
	// The same test StreakNote makes, and for the same reason: "best ever" is
	// only worth saying when there was a previous run to beat. A first unbroken
	// run beats its own best every single day, and a tracker that says so every
	// single day has spent the words by the time the run is worth something.
	if h.Best > 0 && n > h.Best && h.Total > h.Streak {
		return fmt.Sprintf("%s— %d, best ever%s", commands.C(commands.AnsiGold), n,
			commands.C(commands.AnsiReset))
	}
	return fmt.Sprintf("%s— %d%s", commands.C(commands.AnsiDim), n, commands.C(commands.AnsiReset))
}

// ---------------------------------------------------------------------------
// Taking today's tick off again
// ---------------------------------------------------------------------------

// `orgs habits untick meditate` - I did not do that today after all.
//
// This was an undo to begin with, built on a copy of the file taken before the
// write, and it had to be thrown away: habits live many to a file, so ticking a
// second one off made the first one's copy stale and the undo refused. It refused
// exactly when somebody was most likely to want it, which is halfway down a list.
//
// `POST /habits/untick` asks the *file* instead - is there a completion recorded
// for today? - so there is nothing to go stale. It works on a habit ticked off in
// Emacs this morning, on one ticked off twice, after the server has been
// restarted, and under `-local`, none of which an undo buffer manages.
//
// "Nothing recorded for today" is the answer rather than an error - the file simply
// has nothing to take off, which is what makes pressing the key twice harmless. It
// is still said out loud and still exits non-zero, because the picker's binding
// shows a message only for a non-zero child: a key that silently does nothing reads
// as a key that is broken, and "there was nothing there" is the one sentence that
// explains an unchanged square.
func (self *Habits) untick(core *commands.Core, sel string) {
	h := self.resolve(core, sel)
	if commands.Wrote(fmt.Sprintf("take today's tick off %s", h.Headline), nil) {
		return
	}
	req := common.HabitUntick{Hash: h.Hash}
	res, err := commands.SendReceivePostErr[common.HabitUntick, common.HabitUntickResult](
		core, "habits/untick", &req)
	if err != nil {
		if err == commands.ErrDryRun {
			return
		}
		commands.Fail("orgs habits untick: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs habits untick: %s (%s)",
			firstNonEmpty(res.Msg, "nothing to take off"), h.Headline)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	// What it did, in the terms the tracker is read in: the square for today, and
	// when the habit is asking to be done again.
	line := fmt.Sprintf("%s↶%s %s%s%s — today's tick removed", commands.C(commands.AnsiGold),
		commands.C(commands.AnsiReset), commands.C(commands.AnsiBold),
		firstNonEmpty(res.Headline, h.Headline), commands.C(commands.AnsiReset))
	if res.Removed > 1 {
		// Said out loud: a day is the unit, so both of them came off, and somebody
		// who ticked twice by accident should see that both went.
		line += fmt.Sprintf(" %s(%d entries)%s", commands.C(commands.AnsiDim),
			res.Removed, commands.C(commands.AnsiReset))
	}
	if res.Moved && res.Scheduled != "" {
		line += fmt.Sprintf(", %sdue again %s%s", commands.C(commands.AnsiDim),
			pretty(res.Scheduled), commands.C(commands.AnsiReset))
	}
	fmt.Println(line)
}

// ---------------------------------------------------------------------------
// Odds and ends
// ---------------------------------------------------------------------------

// TermWidth is what the terminal says, or what the environment says, or 80.
//
// Asked of the terminal first rather than of COLUMNS, because COLUMNS is
// exported by some shells and not others and is stale in all of them after a
// window is resized.
func TermWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 30 {
		return w
	}
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 30 {
		return n
	}
	return 80
}

// How much of a habit's history fits in a picker line.
//
// The pane takes 62% of the terminal, so the list has the rest of it, less what
// the mark, the name, the run and the file cost.
func (self *Habits) stripWidth() int {
	if self.Strip >= 0 {
		return self.Strip
	}
	n := TermWidth()*38/100 - 34
	if n < 0 {
		n = 0
	}
	if n > 21 {
		n = 21
	}
	return n
}

func padRight(s string, n int) string {
	for commands.RuneLen(s) < n {
		s += " "
	}
	return s
}

func padLeft(s string, n int) string {
	for commands.RuneLen(s) < n {
		s = " " + s
	}
	return s
}

// rightOf pushes something to the right-hand end of the line it is on. An empty
// thing takes no room, rather than a line's worth of trailing spaces.
func rightOf(s string, gap int) string {
	if s == "" {
		return ""
	}
	if gap < 1 {
		gap = 1
	}
	return strings.Repeat(" ", gap) + s
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func init() {
	commands.AddCmd("habits", "how your habits are going, and ticking one off",
		func() commands.Cmd { return &Habits{Days: 56} })
}

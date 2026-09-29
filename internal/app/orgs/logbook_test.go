package orgs

import (
	"strings"
	"testing"
	"time"
)

var when = time.Date(2026, 9, 28, 14, 32, 0, 0, time.Local)

// ---------------------------------------------------------------------------
// Keyword cookies
// ---------------------------------------------------------------------------

// A `#+TODO:` line written the way the org manual writes them carries a cookie
// on each keyword. The keyword is the name; the cookie says how it is reached
// and what is logged about it. Handing back the whole word made every keyword in
// such a file one no reader would recognise.
func TestKeywordCookiesAreReadNotKept(t *testing.T) {
	active, done := ParseTodoStates("TODO(t) NEXT(n!) WAITING(w@/!) | DONE(d!) CANCELLED(c@)")
	if got := strings.Join(active, ","); got != "TODO,NEXT,WAITING" {
		t.Errorf("active: got %q", got)
	}
	if got := strings.Join(done, ","); got != "DONE,CANCELLED" {
		t.Errorf("done: got %q", got)
	}
}

func TestWhatACookieAsksFor(t *testing.T) {
	for _, c := range []struct {
		word  string
		name  string
		key   string
		enter LogMode
		leave LogMode
	}{
		{"TODO", "TODO", "", LogNone, LogNone},
		{"TODO(t)", "TODO", "t", LogNone, LogNone},
		{"NEXT(n!)", "NEXT", "n", LogTime, LogNone},
		{"DONE(d@)", "DONE", "d", LogNote, LogNone},
		{"WAITING(w@/!)", "WAITING", "w", LogNote, LogTime},
		{"HOLD(h!/!)", "HOLD", "h", LogTime, LogTime},
	} {
		kw := parseTodoKeyword(c.word)
		if kw.Name != c.name || kw.Key != c.key || kw.OnEnter != c.enter || kw.OnLeave != c.leave {
			t.Errorf("%s: got name=%q key=%q enter=%v leave=%v", c.word, kw.Name, kw.Key, kw.OnEnter, kw.OnLeave)
		}
	}
}

// Org's rule for a line with no bar in it: the last keyword is the done one.
func TestALineWithNoBar(t *testing.T) {
	active, done := ParseTodoStates("TODO NEXT DONE")
	if strings.Join(active, ",") != "TODO,NEXT" || strings.Join(done, ",") != "DONE" {
		t.Errorf("got active=%v done=%v", active, done)
	}
}

// ---------------------------------------------------------------------------
// Repeaters
// ---------------------------------------------------------------------------

// The three spellings are three different sums, and the difference between them
// is the whole reason org has three.
func TestTheThreeRepeaters(t *testing.T) {
	// A schedule a fortnight in the past, "completed" today.
	old := time.Date(2026, 9, 14, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)
	for _, c := range []struct {
		cookie string
		want   string
		why    string
	}{
		{"+2d", "2026-09-16", "counts from the old date, and may stay in the past"},
		{"++2d", "2026-09-30", "counts from the old date until it is in the future"},
		{".+2d", "2026-09-30", "counts from today"},
		{"+1w", "2026-09-21", "a week is seven days"},
		{"++1w", "2026-10-05", "shifted until it is strictly in the future"},
		{"+1m", "2026-10-14", "calendar months, not thirty days"},
		{"+1y", "2027-09-14", ""},
	} {
		rep, ok := parseRepeater(c.cookie)
		if !ok {
			t.Fatalf("%s did not parse", c.cookie)
		}
		got := rep.next(old, now).Format("2006-01-02")
		if got != c.want {
			t.Errorf("%s: got %s want %s (%s)", c.cookie, got, c.want, c.why)
		}
	}
}

// `++` must land strictly after now, not on it - otherwise a daily habit
// completed today comes round again today.
func TestDoublePlusLandsInTheFuture(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local)
	rep, _ := parseRepeater("++1d")
	got := rep.next(now, now)
	if !got.After(now) {
		t.Errorf("++1d from today gave %v, which is not in the future", got)
	}
}

// Advancing rewrites the date and nothing else about the line: the keyword, the
// brackets, the time of day, the repeater and the warning period all stay.
func TestAdvancingAPlanningLineKeepsEverythingElse(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"SCHEDULED: <2026-09-26 Sat .+2d>", "SCHEDULED: <2026-09-30 Wed .+2d>"},
		{"  SCHEDULED: <2026-09-26 Sat +2d>", "  SCHEDULED: <2026-09-28 Mon +2d>"},
		{"DEADLINE: <2026-09-26 Sat ++1w -3d>", "DEADLINE: <2026-10-03 Sat ++1w -3d>"},
		{"SCHEDULED: <2026-09-26 Sat 09:00 .+1d>", "SCHEDULED: <2026-09-29 Tue 09:00 .+1d>"},
		// No repeater: nothing moves.
		{"SCHEDULED: <2026-09-26 Sat>", "SCHEDULED: <2026-09-26 Sat>"},
		{"DEADLINE: <2026-10-01 Thu>", "DEADLINE: <2026-10-01 Thu>"},
	} {
		got, _ := advanceRepeaters(c.in, when)
		if got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// A heading can have a repeating schedule and a fixed deadline. Moving the
// deadline because the schedule repeated would turn a real due date into a
// rolling one, which is a change to what the file says rather than to when it
// says it.
func TestOnlyRepeatingDatesMove(t *testing.T) {
	line := "CLOSED: [2026-09-26 Sat 10:00] DEADLINE: <2026-10-01 Thu> SCHEDULED: <2026-09-26 Sat .+2d>"
	got, moved := advanceRepeaters(line, when)
	if !moved {
		t.Fatal("nothing moved")
	}
	if !strings.Contains(got, "DEADLINE: <2026-10-01 Thu>") {
		t.Errorf("the fixed deadline moved: %s", got)
	}
	if !strings.Contains(got, "SCHEDULED: <2026-09-30 Wed .+2d>") {
		t.Errorf("the repeating schedule did not move: %s", got)
	}
	if !strings.Contains(got, "CLOSED: [2026-09-26 Sat 10:00]") {
		t.Errorf("the closing stamp moved: %s", got)
	}
}

// The weekday is the one derived part of a timestamp. Carrying it over would
// leave a date that reads wrong and that Emacs silently corrects, making the
// file look changed when it was not.
func TestTheWeekdayIsRegenerated(t *testing.T) {
	got, _ := advanceRepeaters("SCHEDULED: <2026-09-28 Mon .+1d>", when)
	if !strings.Contains(got, "<2026-09-29 Tue") {
		t.Errorf("weekday not regenerated: %s", got)
	}
}

// ---------------------------------------------------------------------------
// The CLOSED stamp
// ---------------------------------------------------------------------------

// Org keeps all of a heading's planning on one line, with CLOSED at the front.
func TestClosedGoesOnTheFrontOfThePlanningLine(t *testing.T) {
	lines := []string{"* DONE Thing", "SCHEDULED: <2026-09-28 Mon>", "body"}
	out := stampClosed(lines, 0, 2, "", when)
	want := "CLOSED: [2026-09-28 Mon 14:32] SCHEDULED: <2026-09-28 Mon>"
	if out[1] != want {
		t.Errorf("got %q want %q", out[1], want)
	}
	if len(out) != 3 {
		t.Errorf("a line was added: %v", out)
	}
}

func TestClosedOnAHeadingWithNoPlanning(t *testing.T) {
	lines := []string{"* DONE Thing", "body"}
	out := stampClosed(lines, 0, 1, "", when)
	if out[1] != "CLOSED: [2026-09-28 Mon 14:32]" {
		t.Errorf("got %q", out[1])
	}
	if out[2] != "body" {
		t.Errorf("the body was overwritten: %v", out)
	}
}

// Marking a heading done twice must not leave two stamps.
func TestClosedIsReplacedNotRepeated(t *testing.T) {
	lines := []string{"* DONE Thing", "CLOSED: [2026-09-01 Tue 08:00] SCHEDULED: <2026-09-28 Mon>"}
	out := stampClosed(lines, 0, 1, "", when)
	if n := strings.Count(out[1], "CLOSED:"); n != 1 {
		t.Errorf("%d stamps: %q", n, out[1])
	}
	if !strings.Contains(out[1], "[2026-09-28 Mon 14:32]") {
		t.Errorf("not restamped: %q", out[1])
	}
	if !strings.Contains(out[1], "SCHEDULED: <2026-09-28 Mon>") {
		t.Errorf("the schedule was lost: %q", out[1])
	}
}

// Taking the stamp off leaves the rest of the planning, and takes the line away
// when the stamp was all it held.
func TestUnstamping(t *testing.T) {
	lines := []string{"* TODO Thing", "CLOSED: [2026-09-01 Tue 08:00] SCHEDULED: <2026-09-28 Mon>"}
	out := unstampClosed(lines, 0, 1)
	if out[1] != "SCHEDULED: <2026-09-28 Mon>" {
		t.Errorf("got %q", out[1])
	}

	lines = []string{"* TODO Thing", "CLOSED: [2026-09-01 Tue 08:00]", "body"}
	out = unstampClosed(lines, 0, 2)
	if len(out) != 2 || out[1] != "body" {
		t.Errorf("the empty planning line was left behind: %v", out)
	}
}

// ---------------------------------------------------------------------------
// The keyword on the heading line
// ---------------------------------------------------------------------------

// A pattern that decides for itself what a keyword looks like reads `* API
// redesign` as a heading whose keyword is API. Being told what the parser found
// is what makes this exact.
func TestReplacingTheKeyword(t *testing.T) {
	for _, c := range []struct{ line, from, to, want string }{
		{"* NEXT Water the plants", "NEXT", "DONE", "* NEXT Water the plants"},
		{"** TODO Thing  :work:", "TODO", "DONE", "** DONE Thing  :work:"},
		{"* API redesign", "", "TODO", "* TODO API redesign"},
		{"* DONE Thing", "DONE", "", "* Thing"},
		{"*** [#A] Thing", "", "NEXT", "*** NEXT [#A] Thing"},
		{"* TODOISH thing", "", "NEXT", "* NEXT TODOISH thing"},
	} {
		got, ok := replaceHeadlineStatus(c.line, c.from, c.to)
		if !ok {
			t.Errorf("%q was not read as a heading", c.line)
			continue
		}
		want := c.want
		if c.from == "NEXT" && c.to == "DONE" {
			want = "* DONE Water the plants"
		}
		if got != want {
			t.Errorf("%q (%s->%s)\n got %q\nwant %q", c.line, c.from, c.to, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Where the log lines go
// ---------------------------------------------------------------------------

// Org's own wording, so a file orgs has touched reads like one Emacs has and the
// regexes on both sides find the same thing.
func TestTheStateLineIsWrittenTheWayOrgWritesIt(t *testing.T) {
	got := stateLogLine("  ", "DONE", "NEXT", when)
	want := `  - State "DONE"       from "NEXT"       [2026-09-28 Mon 14:32]`
	if got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
	// And it has to be something habitDoneRe finds, since that is what the
	// agenda's habit graph is built out of.
	if !habitDoneRe.MatchString(got) {
		t.Errorf("the habit graph cannot read its own log line: %q", got)
	}
}

func TestLogIntoADrawerThatIsNotThereYet(t *testing.T) {
	lines := []string{"* DONE Thing", "CLOSED: [2026-09-28 Mon 14:32]", "body"}
	out := logInto(lines, 0, 2, "  ", "LOGBOOK", []string{"  - entry"})
	want := []string{"* DONE Thing", "CLOSED: [2026-09-28 Mon 14:32]", "  :LOGBOOK:", "  - entry", "  :END:", "body"}
	if strings.Join(out, "\n") != strings.Join(want, "\n") {
		t.Errorf("\n got %v\nwant %v", out, want)
	}
}

// Newest first, which is org's default and the order that matters: the line
// somebody wants is nearly always the last thing that happened.
func TestLogIntoAnExistingDrawerGoesOnTop(t *testing.T) {
	lines := []string{"* DONE Thing", ":LOGBOOK:", "- older", ":END:"}
	out := logInto(lines, 0, 3, "", "LOGBOOK", []string{"- newer"})
	if out[2] != "- newer" || out[3] != "- older" {
		t.Errorf("wrong order: %v", out)
	}
}

// With no drawer the lines go into the body as a plain list, which is what org
// does by default - after the planning and after the property drawer, never
// between a heading and its own properties.
func TestLogIntoTheBody(t *testing.T) {
	lines := []string{"* DONE Thing", "CLOSED: [x]", ":PROPERTIES:", ":ID: 1", ":END:", "body"}
	out := logInto(lines, 0, 5, "", "", []string{"- entry"})
	if out[5] != "- entry" {
		t.Errorf("entry landed at the wrong place: %v", out)
	}
	if out[2] != ":PROPERTIES:" || out[4] != ":END:" {
		t.Errorf("the property drawer was broken open: %v", out)
	}
}

// ---------------------------------------------------------------------------
// Settings, and the three places they can be said
// ---------------------------------------------------------------------------

func TestDrawerNameSpellings(t *testing.T) {
	for in, want := range map[string]string{
		"t": "LOGBOOK", "LOGBOOK": "LOGBOOK", "logbook": "LOGBOOK",
		"nil": "", "": "", "none": "",
		"MYLOG": "MYLOG", ":MYLOG:": "MYLOG", "mylog": "MYLOG",
	} {
		if got := drawerName(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestStartupWordsOverrideTheConfig(t *testing.T) {
	set := logSettings{done: LogTime, repeat: LogTime, drawer: "LOGBOOK"}
	for _, w := range []string{"nologdone", "lognoterepeat", "nologdrawer"} {
		startupLogWords[w](&set)
	}
	if set.done != LogNone {
		t.Error("nologdone did nothing")
	}
	if set.repeat != LogNote {
		t.Error("lognoterepeat did nothing")
	}
	if set.drawer != "" {
		t.Error("nologdrawer did nothing")
	}
}

// A heading saying `:LOGGING: nil` wants no history of itself, which is the
// shape somebody wants for the one task they do not want a record of.
func TestAHeadingCanTurnItAllOff(t *testing.T) {
	set := applyHeadingLogProps(
		logSettings{done: LogTime, repeat: LogTime, drawer: "LOGBOOK", states: true},
		map[string]string{"LOGGING": "nil"})
	if set.done != LogNone || set.repeat != LogNone || set.states {
		t.Errorf("still logging: %+v", set)
	}
}

func TestAHeadingCanChooseItsOwnDrawer(t *testing.T) {
	set := applyHeadingLogProps(logSettings{drawer: "LOGBOOK"},
		map[string]string{"LOG_INTO_DRAWER": "HISTORY"})
	if set.drawer != "HISTORY" {
		t.Errorf("got %q", set.drawer)
	}
	set = applyHeadingLogProps(logSettings{drawer: "LOGBOOK"},
		map[string]string{"LOGGING": "lognotedone nologdrawer"})
	if set.drawer != "" || set.done != LogNote {
		t.Errorf("got %+v", set)
	}
}

// A note says everything a timestamp says and more, so anything asking for one
// wins over anything asking for the other.
func TestTheStrongestModeWins(t *testing.T) {
	if strongest(LogNone, LogNote, LogTime) != LogNote {
		t.Error("a note did not win")
	}
	if strongest(LogNone, LogNone) != LogNone {
		t.Error("nothing became something")
	}
}

// A daily task due at nine in the morning is due at nine in the morning. Letting
// the clock follow the completion time walks a reminder round the day over a
// week of being a bit late, which is the opposite of what a repeater is for.
func TestARepeatKeepsItsTimeOfDay(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"SCHEDULED: <2026-09-26 Sat 09:00 .+1d>", "SCHEDULED: <2026-09-29 Tue 09:00 .+1d>"},
		{"SCHEDULED: <2026-09-26 Sat 09:00 +1d>", "SCHEDULED: <2026-09-27 Sun 09:00 +1d>"},
		// A range keeps its length.
		{"SCHEDULED: <2026-09-26 Sat 09:00-10:30 .+1d>", "SCHEDULED: <2026-09-29 Tue 09:00-10:30 .+1d>"},
	} {
		if got, _ := advanceRepeaters(c.in, when); got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// org-habit writes `.+1d/3d`: every day, and definitely overdue after three.
// Leaving the slack half out of the timestamp pattern did not make it ignored -
// it made the whole timestamp fail to match, so a habit written the way
// org-habit documents never repeated and its keyword stayed on DONE. The
// spelling has to survive the date moving, too.
func TestAHabitsSlackRepeaterAdvances(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"SCHEDULED: <2026-09-27 Sun .+1d/3d>", "SCHEDULED: <2026-09-29 Tue .+1d/3d>"},
		{"SCHEDULED: <2026-09-20 Sun ++1w/2w>", "SCHEDULED: <2026-10-04 Sun ++1w/2w>"},
		{"SCHEDULED: <2026-09-20 Sun +1w/2w>", "SCHEDULED: <2026-09-27 Sun +1w/2w>"},
	} {
		got, moved := advanceRepeaters(c.in, when)
		if !moved {
			t.Errorf("%q did not move at all", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

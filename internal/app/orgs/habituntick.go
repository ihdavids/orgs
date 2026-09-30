package orgs

// Taking today's tick off a habit.
//
// The obvious way to reverse a habit tick is an undo buffer - keep the file as it
// was and put it back - and it does not work here, for a reason that only shows up
// in use: a copy of a whole file is only good while the file is *untouched*, and
// habits live many to a file. Tick two of them off and the first one can no longer
// be put back, because the second one changed the file. Every write from anywhere -
// worg's kanban, a clock, Emacs - has the same effect. So the undo refuses exactly
// when somebody is most likely to want it: in the middle of working down a list.
//
// This asks the file instead. Is there a completion recorded for today? Then take
// it out, and put back the parts of the tick that can be worked out from what is
// still written down. Nothing is remembered between requests, so it works on a
// habit ticked off in Emacs this morning, on one ticked off twice, after a restart,
// and under `-local` where every command is its own short-lived server.
//
// Three decisions hold it together.
//
// **A day is the unit**, which is the rule the whole tracker counts by. So this
// takes off *every* completion recorded today rather than the last one - a habit
// ticked twice in an afternoon is one filled square, and clearing it has to empty
// that square rather than leave it filled and require a second go.
//
// **What comes off is exactly what fills the square.** The lines matched are the
// ones `parseHabitCompletions` counts, `habitDoneRe` and all. A tracker and its
// eraser disagreeing about which lines count would be a square that cannot be
// cleared, or a line that vanishes from a file with nothing changing on screen.
//
// **The date only goes back if it went forward today.** `:LAST_REPEAT:` is how org
// records that a repeat happened, so it is the evidence - and without it, moving
// the date would be guessing. That matters because one interval back is not always
// exactly where the date came from: for `.+2d` it lands on the day of the tick and
// for `+2d` it is exact, but `++2d` may have taken several steps and nothing in the
// file says how many. Taking one step back from a stamp written today is a sum with
// evidence behind it; doing it to a date nothing says moved is not.

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// The `from "NEXT"` half of a state line, which is how the keyword a heading had
// before the tick is recovered - the file says it, so nothing has to remember it.
var stateFromRe = regexp.MustCompile(`from\s+"([^"]*)"`)

// Any heading line, for confining the edits to this habit's *own* lines.
var anyHeadRe = regexp.MustCompile(`^\*+\s`)

// UntickHabit takes today's completion off a habit and puts back what the tick
// moved, as a line edit.
//
// Line edits rather than a document rewrite, the same as everything else that
// writes here: a habit shares its file with a hundred other headings, and writing
// the parsed document back would reformat all of them to clear one square.
//
// The order is forced, as in applyStatusChange: every edit that adds or removes a
// line moves the ones below it, so the heading's extent is measured again after
// each one. Measuring once at the top and using it throughout writes the second
// edit into the wrong heading and reports success.
func UntickHabit(hash string, now time.Time) (common.HabitUntickResult, error) {
	var res common.HabitUntickResult
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		res.Msg = "no heading with that hash"
		return res, nil
	}
	f := GetDb().ByHashToFile[hash]
	if f == nil {
		res.Msg = "no file for that heading"
		return res, nil
	}
	res.Headline = headlineText(sec)
	res.Status = sec.Headline.Status
	if !IsHabitSection(sec) {
		res.Msg = res.Headline + " is not a habit"
		return res, nil
	}

	lines, from, end, ok := recordLines(f.Filename, sec)
	if !ok {
		res.Msg = "could not read " + f.Filename
		return res, nil
	}
	lvl := sec.Headline.Lvl
	ind := indentOf(lvl)
	set := applyHeadingLogProps(fileLogSettings(f), propsFromLines(lines, from, end))
	today := dayOf(now).Format("2006-01-02")

	// 1. The completion lines. Removed back to front, so that each index is still
	//    the index it was found at.
	at := habitDoneLinesOn(lines, from, end, set.drawer, today)
	if len(at) == 0 {
		res.Msg = "nothing recorded for today"
		return res, nil
	}
	wasKeyword := ""
	for i := len(at) - 1; i >= 0; i-- {
		if m := stateFromRe.FindStringSubmatch(lines[at[i]]); m != nil && wasKeyword == "" {
			wasKeyword = m[1]
		}
		lines = append(lines[:at[i]], lines[at[i]+1:]...)
	}
	res.Removed = len(at)
	lines, from, end = reread(lines, from, lvl)

	// 2. An empty drawer goes with them. A bare :LOGBOOK:/:END: pair left behind
	//    is not wrong, exactly, but org does not write one and a habit that has
	//    never been kept should read like one that has never been kept.
	if set.drawer != "" {
		if s, e, found := drawerAt(lines, from, end, set.drawer); found && e == s+1 {
			lines = append(lines[:s], lines[e+1:]...)
			lines, from, end = reread(lines, from, lvl)
		}
	}

	// 3. The date, and only on the evidence that it moved today.
	props := propsFromLines(lines, from, end)
	if stampDay(props["LAST_REPEAT"]) == today {
		prev := previousCompletion(lines, from, end, set.drawer, today)
		if moved, when := retreatPlanning(lines, from, end, prev); moved {
			res.Moved = true
			res.Scheduled = when
		}
		// 4. And :LAST_REPEAT: itself, which now records a repeat that is no
		//    longer in the file.
		if ps, pe, found := drawerAt(lines, from, end, "PROPERTIES"); found {
			if out, dropped := dropPropIn(lines, ps, pe, "LAST_REPEAT"); dropped {
				lines = out
				lines, from, end = reread(lines, from, lvl)
			}
		}
	}

	// 5. The keyword, for a habit that did not repeat and so is sitting on a done
	//    state. The one it had is read off the line that was just removed - the
	//    file said it, so nothing here has to remember it. A repeating habit is
	//    already on a live keyword and is left alone.
	if _, isDone, known := keywordInfo(f, sec.Headline.Status); known && isDone {
		back := wasKeyword
		if back == "" {
			if active, _ := ValidStatusFromFile(f); len(active) > 0 {
				back = active[0]
			}
		}
		if head, done := replaceHeadlineStatus(lines[from], sec.Headline.Status, back); done {
			lines[from] = head
			res.Status = back
		}
		// It is not finished any more, so it is not closed either.
		lines = unstampClosed(lines, from, end)
		lines, from, end = reread(lines, from, lvl)
	}
	_ = ind

	if err := writeLines(f.Filename, lines); err != nil {
		return res, err
	}
	GetDb().ReloadFile(f.Filename)
	res.Ok = true
	return res, nil
}

func headlineText(sec *org.Section) string {
	out := ""
	for _, t := range sec.Headline.Title {
		out += t.String()
	}
	return strings.TrimSpace(out)
}

// The lines under this heading that record a completion on the given day.
//
// Confined to the habit's **own** lines: `recordLines` hands back the whole
// subtree, and a child heading with a logbook of its own would otherwise have its
// completions counted as this habit's and deleted as this habit's.
//
// Looked for in the log drawer where there is one and in the body where there is
// not, which is the same pair of places `parseHabitCompletions` reads - a heading
// whose log went into the body rather than into a drawer still has a history, and
// turning the drawer off must not turn this off with it.
func habitDoneLinesOn(lines []string, from, to int, drawer, day string) []int {
	own := ownLinesEnd(lines, from, to)
	lo, hi := from+1, own
	if drawer != "" {
		if s, e, found := drawerAt(lines, from, own, drawer); found {
			lo, hi = s+1, e-1
		}
	}
	out := []int{}
	for i := lo; i <= hi && i < len(lines); i++ {
		m := habitDoneRe.FindStringSubmatch(lines[i])
		if m != nil && m[1] == day {
			out = append(out, i)
		}
	}
	return out
}

// The last line that belongs to this heading rather than to a child of it.
func ownLinesEnd(lines []string, from, to int) int {
	for i := from + 1; i <= to && i < len(lines); i++ {
		if anyHeadRe.MatchString(lines[i]) {
			return i - 1
		}
	}
	return to
}

// The day a `[2026-09-29 Tue 21:24]` stamp names, or "".
func stampDay(s string) string {
	m := timestampRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[2] + "-" + m[3] + "-" + m[4]
}

// The newest completion left in the logbook that is not today's - the day the
// habit was kept before the tick being taken off.
//
// This is what makes a `.+` repeater reversible **exactly**. `.+2d` means "two days
// after I actually did it", so the date the heading was carrying before today's
// tick was the previous completion plus two days - which is in the file, once
// today's line has been taken out of it. Stepping back one interval instead lands
// on today, which is right for a habit ticked off on the day it was due and wrong
// for one ticked off early or late: it would quietly move the schedule and, for an
// overdue habit, erase the fact that it was overdue.
func previousCompletion(lines []string, from, to int, drawer, today string) time.Time {
	best := time.Time{}
	own := ownLinesEnd(lines, from, to)
	lo, hi := from+1, own
	if drawer != "" {
		if s, e, found := drawerAt(lines, from, own, drawer); found {
			lo, hi = s+1, e-1
		}
	}
	for i := lo; i <= hi && i < len(lines); i++ {
		m := habitDoneRe.FindStringSubmatch(lines[i])
		if m == nil || m[1] >= today {
			continue
		}
		if d, err := time.ParseInLocation("2006-01-02", m[1], time.Local); err == nil {
			if best.IsZero() || d.After(best) {
				best = d
			}
		}
	}
	return best
}

// Move every repeating date in this heading's planning back to where the tick
// found it, and say where the schedule ended up.
//
// The mirror of advancePlanning, and it touches only a date carrying a repeater
// for the same reason: a heading can have a repeating schedule and a fixed
// deadline, and moving the deadline back because the schedule repeated would turn a
// real due date into a rolling one.
//
// Two sums, matching the two `next` uses. A `.+` repeater counted from the day the
// habit was last done, so it is put back by counting from the completion before
// this one - exact, and `prev` is what that is. Everything else steps back one
// interval: exact for `+`, and for `++` the latest date it could have been
// carrying. The guard is for a hand-edited date, where the reconstruction can come
// out later than what is actually written and the step back is the safer of the
// two.
func retreatPlanning(lines []string, from, to int, prev time.Time) (bool, string) {
	moved, when := false, ""
	for i := from + 1; i <= to && i < len(lines); i++ {
		if !planningRe.MatchString(lines[i]) {
			break
		}
		out := timestampRe.ReplaceAllStringFunc(lines[i], func(s string) string {
			m := timestampRe.FindStringSubmatch(s)
			if m == nil {
				return s
			}
			rep, ok := parseRepeater(m[7])
			if !ok {
				return s
			}
			had, _, ok := stampTime(m)
			if !ok {
				return s
			}
			back := rep.back(had)
			if rep.pre == ".+" && rep.unit != "h" && !prev.IsZero() {
				base := time.Date(prev.Year(), prev.Month(), prev.Day(),
					had.Hour(), had.Minute(), 0, 0, had.Location())
				if when := rep.step(base); !when.After(had) {
					back = when
				}
			}
			moved = true
			if strings.Contains(lines[i], "SCHEDULED:") {
				when = back.Format("2006-01-02")
			}
			return rewriteStamp(m, back)
		})
		lines[i] = out
	}
	return moved, when
}

// Take one property out of a drawer, and say whether it was there.
//
// The line-level sibling of setPropIn. `SetProperty` in todo.go removes a property
// when it is given an empty value, but it does it by writing the whole parsed
// document back - which is the thing every writer in here avoids, and the reason
// it was worth having at this level.
func dropPropIn(lines []string, ps, pe int, key string) ([]string, bool) {
	want := ":" + strings.ToUpper(key) + ":"
	for i := ps + 1; i < pe && i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(strings.ToUpper(t), want) {
			continue
		}
		out := append(lines[:i], lines[i+1:]...)
		// The keys are re-aligned inside the drawer afterwards, because a property
		// drawer is text somebody opens in an editor - the same thing the record
		// editor does after a write.
		if pe-1 > ps+1 {
			alignDrawer(out, ps, pe-1)
		}
		return out, true
	}
	return lines, false
}

/* SDOC: API
* POST /habits/untick — Take Today's Tick Off A Habit

	Removes the completion recorded for today on one =:STYLE: habit= heading, and
	puts back what the tick moved: the repeating date goes back one interval,
	=:LAST_REPEAT:= comes off, and a habit left sitting on a done keyword goes back
	to the one the log line says it had.

	*Method:* =POST=

	*Request:* A =HabitUntick= — =Hash=, the heading. Required.

	*Response:* A =HabitUntickResult=. =Ok= is false with a =Msg= when there is
	nothing recorded for today, which is not an error: it is the answer, and it is
	what makes pressing the key twice harmless.

	This is deliberately **not** an undo. It is worked out from the file rather than
	from anything the server remembers, so it works on a habit ticked off in Emacs
	this morning, on one ticked off twice, after a restart, and under =-local=.

	Three things it decides:

	- *A day is the unit.* Every completion recorded today comes off, not just the
	  last one. A habit ticked twice in an afternoon is one filled square in the
	  tracker and is cleared as one.
	- *What comes off is exactly what fills the square* — the lines
	  =parseHabitCompletions= counts. A tracker and its eraser disagreeing about
	  which lines count would leave a square that cannot be cleared.
	- *The date only goes back if it went forward today*, which =:LAST_REPEAT:= is
	  the evidence for. One interval back is exact for =+2d=, lands on the day of
	  the tick for =.+2d=, and is the latest possible date for =++2d= — which may
	  have taken several steps with nothing in the file to say how many. Doing that
	  sum against a stamp written today has evidence behind it; doing it to a date
	  nothing says moved would be a guess.
	EDOC */
func PostHabitUntick(w http.ResponseWriter, r *http.Request) {
	var req common.HabitUntick
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		untickJson(w, common.HabitUntickResult{Msg: "could not read the request"})
		return
	}
	res, err := UntickHabit(req.Hash, time.Now())
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
	}
	untickJson(w, res)
}

func untickJson(w http.ResponseWriter, v common.HabitUntickResult) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

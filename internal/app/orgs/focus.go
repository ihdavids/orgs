package orgs

// Focus sessions: a pomodoro timer that is also the clock.
//
// Pick a heading, and the server clocks in on it and starts a work phase.
// When the phase runs out the server - not the browser tab that started it,
// which may be closed, asleep or on another machine - clocks out, which writes
// the CLOCK line, notes the pomodoro in the heading's logbook, and starts a
// break. When the break runs out the session goes idle (or, with AutoNext,
// starts the next pomodoro). Every step is published on /events, so worg's
// header chip, `orgs focus` and a menu-bar plugin all move together without
// polling.
//
// A few decisions worth knowing about:
//
//  1. **The timer lives on the server and survives a restart.** The session is
//     written to focus_data.json beside clock_data.json, and the ticker picks it
//     up where it was. A work phase that ran out while the server was down is
//     clocked out *at the moment it ran out* (Clock().ClockOutAt), so the CLOCK
//     line says 0:25 rather than however long the server was away.
//  2. **The clock is the truth about whether work is happening.** If the clock
//     is stopped, or moved to another heading, from anywhere else (worg's
//     clock button, `orgs clockout`, the auto clock-out plugin), the work phase
//     is over: the session stops and says why. Two timers disagreeing about
//     what you are doing is worse than either alone.
//  3. **A finished pomodoro leaves a line in the logbook**, beside its CLOCK
//     line: `- Pomodoro: 25 min focus, 5 min break [2026-10-10 Sat 10:25]`.
//     The CLOCK line already says how long; the note says it was a pomodoro and
//     what break followed, which is what makes "how many pomodoros did this
//     task take" answerable from the file itself, in Emacs, years later. A
//     pomodoro stopped early gets only its CLOCK line, so the notes count
//     finished ones. A short history (30 days) is also kept in the json, for
//     the "today" list - reading every logbook for that would be a walk over
//     every file on each request.
//  4. **Headings are held by file and outline path**, the clock's own form, not
//     by hash: writing the CLOCK line changes the hash of every heading after
//     it in the file (see Traps: hashes). The hash a client is given is worked
//     out afresh on every answer.
//
// The phase logic is a focusEngine with its clock and side effects (clocking,
// writing, publishing, saving) behind focusHooks, so focus_test.go can run a
// whole day in microseconds.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

// The kinds a focus session publishes on /events. Like the other kinds, a
// promise: add to them rather than changing one.
const (
	EventFocusStart  = "focusstart"  // a work phase began
	EventFocusEnd    = "focusend"    // a work phase ran its course (a pomodoro finished)
	EventBreakStart  = "breakstart"  // a break began
	EventBreakEnd    = "breakend"    // a break ran out, or was skipped
	EventFocusStop   = "focusstop"   // the session was stopped before its time
	EventFocusExtend = "focusextend" // the running phase was given more time
)

const (
	focusWork  = "work"
	focusBreak = "break"
	focusIdle  = "idle"
)

// How long finished work phases are remembered for the history list.
const focusHistoryDays = 30

var focusDefaults = common.FocusSettings{Work: 25, Short: 5, Long: 15, LongEvery: 4}

// The heading a session is about, as a person would name it.
type focusTask struct {
	Hash  string
	Title string
	File  string
}

// What the engine needs from the world. The live version is the clock and the
// database; the test version is a few slices.
type focusHooks interface {
	// Clock in on the heading, answering with the target in its lasting form
	// (file+olp) and the heading's names.
	clockIn(t common.Target) (common.Target, focusTask, error)
	// Is the clock running on this heading?
	clockedOn(t common.Target) bool
	// Stop the clock as of end, if it is still on this heading.
	clockOut(t common.Target, end time.Time)
	// Write the pomodoro's line into the heading's logbook.
	logPomodoro(t common.Target, work, brk float64, at time.Time) error
	// The heading's current names, or ok false when it cannot be found.
	describe(t common.Target) (focusTask, bool)
	publish(e Event)
	save(d *focusData)
}

// focusData is everything that has to survive a restart.
type focusData struct {
	Phase     string
	LongBreak bool
	Target    common.Target
	Task      focusTask
	Started   time.Time
	Ends      time.Time
	// When the last break ended (or the session stopped), so a pomodoro
	// started after a long gap begins a new set of four.
	LastEnd  time.Time
	Streak   int
	Settings common.FocusSettings
	History  []common.FocusRecord
}

type focusEngine struct {
	mu  sync.Mutex
	now func() time.Time
	h   focusHooks
	d   focusData
}

func newFocusEngine(h focusHooks, now func() time.Time) *focusEngine {
	e := &focusEngine{h: h, now: now}
	e.d.Phase = focusIdle
	e.d.Settings = focusDefaults
	return e
}

func minutes(m float64) time.Duration {
	return time.Duration(m * float64(time.Minute))
}

// A length as a person reads it: 25, or 0.5 for a test run.
func fmtMins(m float64) string {
	if m == math.Trunc(m) {
		return strconv.Itoa(int(m))
	}
	return strconv.FormatFloat(m, 'f', 1, 64)
}

// Settings asked for, with anything not given taken from what was used last.
func mergeFocusSettings(old common.FocusSettings, r common.FocusStart) common.FocusSettings {
	s := old
	if r.Work > 0 {
		s.Work = r.Work
	}
	if r.Short > 0 {
		s.Short = r.Short
	}
	if r.Long > 0 {
		s.Long = r.Long
	}
	if r.LongEvery > 0 {
		s.LongEvery = r.LongEvery
	}
	if r.AutoNext != nil {
		s.AutoNext = *r.AutoNext
	}
	// A settings file from before a field existed, or hand edited to nonsense.
	if s.Work <= 0 {
		s.Work = focusDefaults.Work
	}
	if s.Short <= 0 {
		s.Short = focusDefaults.Short
	}
	if s.Long <= 0 {
		s.Long = focusDefaults.Long
	}
	if s.LongEvery <= 0 {
		s.LongEvery = focusDefaults.LongEvery
	}
	return s
}

// A phase that ran its course (not one ended by a click) is worth a desktop
// notification: the person is usually looking at something else by then.
// Only the live hooks announce; a test's fakes need not.
func (e *focusEngine) announce(kind string) {
	if a, ok := e.h.(interface{ announce(kind, title, file string) }); ok {
		a.announce(kind, e.d.Task.Title, e.d.Task.File)
	}
}

func (e *focusEngine) event(kind, msg string) {
	e.h.publish(Event{Kind: kind, Hash: e.d.Task.Hash, Headline: e.d.Task.Title, File: e.d.Task.File, Msg: msg})
}

// Start a work phase. A session already under way is ended first: a running
// work phase is stopped (and clocked out), a break simply ends.
func (e *focusEngine) start(r common.FocusStart) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()

	var tgt common.Target
	switch {
	case r.Target != nil:
		tgt = *r.Target
	case r.Hash != "":
		tgt = common.Target{Type: "hash", Id: r.Hash}
	case e.d.Target.Id != "":
		// "Another one": the last task, which is what a client offers when idle.
		tgt = e.d.Target
	default:
		return fmt.Errorf("say which heading to focus on")
	}

	switch e.d.Phase {
	case focusWork:
		e.stopWork(now, "switched to another task")
	case focusBreak:
		e.event(EventBreakEnd, "break cut short")
	}
	if e.d.Phase == focusIdle && !e.d.LastEnd.IsZero() && now.Sub(e.d.LastEnd) > minutes(e.d.Settings.Long) {
		// Long enough away that this is a new set, not the next of the old one.
		e.d.Streak = 0
	}
	e.d.Settings = mergeFocusSettings(e.d.Settings, r)
	if err := e.beginWork(tgt, now); err != nil {
		e.d.Phase = focusIdle
		e.h.save(&e.d)
		return err
	}
	e.h.save(&e.d)
	return nil
}

func (e *focusEngine) beginWork(tgt common.Target, now time.Time) error {
	olp, task, err := e.h.clockIn(tgt)
	if err != nil {
		return err
	}
	e.d.Target = olp
	e.d.Task = task
	e.d.Phase = focusWork
	e.d.LongBreak = false
	e.d.Started = now
	e.d.Ends = now.Add(minutes(e.d.Settings.Work))
	e.event(EventFocusStart, fmt.Sprintf("%s min, until %s", fmtMins(e.d.Settings.Work), e.d.Ends.Format("15:04")))
	return nil
}

// stopWork ends a work phase before its time: clocked out now, remembered as
// not finished, no logbook note.
func (e *focusEngine) stopWork(at time.Time, why string) {
	e.h.clockOut(e.d.Target, at)
	e.record(at, false, 0, false)
	e.d.Phase = focusIdle
	e.d.LastEnd = at
	e.event(EventFocusStop, why)
}

func (e *focusEngine) record(at time.Time, done bool, brk float64, long bool) {
	if t, ok := e.h.describe(e.d.Target); ok {
		e.d.Task = t
	}
	rec := common.FocusRecord{
		Hash: e.d.Task.Hash, Title: e.d.Task.Title, File: e.d.Task.File,
		Start: e.d.Started.Format(time.RFC3339), End: at.Format(time.RFC3339),
		Minutes:   math.Round(at.Sub(e.d.Started).Minutes()*10) / 10,
		Completed: done, Break: brk, LongBreak: long,
	}
	// Newest first, which is the order every reader wants.
	e.d.History = append([]common.FocusRecord{rec}, e.d.History...)
	cut := at.AddDate(0, 0, -focusHistoryDays)
	for i, h := range e.d.History {
		if t, err := time.Parse(time.RFC3339, h.End); err == nil && t.Before(cut) {
			e.d.History = e.d.History[:i]
			break
		}
	}
}

// completeWork is a pomodoro finished: clock out at the moment it ended, note
// it, and start the break.
func (e *focusEngine) completeWork(at time.Time) {
	work := at.Sub(e.d.Started).Minutes()
	e.h.clockOut(e.d.Target, at)
	e.d.Streak++
	long := e.d.Settings.LongEvery > 0 && e.d.Streak >= e.d.Settings.LongEvery
	brk := e.d.Settings.Short
	if long {
		brk = e.d.Settings.Long
		e.d.Streak = 0
	}
	if err := e.h.logPomodoro(e.d.Target, math.Round(work*10)/10, brk, at); err != nil {
		fmt.Fprintf(os.Stderr, "focus: the pomodoro was not noted in the logbook: %v\n", err)
	}
	e.record(at, true, brk, long)
	e.event(EventFocusEnd, fmt.Sprintf("%s min done", fmtMins(math.Round(work*10)/10)))
	e.d.Phase = focusBreak
	e.d.LongBreak = long
	e.d.Started = at
	e.d.Ends = at.Add(minutes(brk))
	kind := "break"
	if long {
		kind = "long break"
	}
	e.event(EventBreakStart, fmt.Sprintf("%s min %s, until %s", fmtMins(brk), kind, e.d.Ends.Format("15:04")))
}

// endBreak is a break over. next starts the next pomodoro whatever AutoNext
// says - that is what skipping a break means.
func (e *focusEngine) endBreak(at, now time.Time, next bool) {
	e.event(EventBreakEnd, "break over")
	e.d.Phase = focusIdle
	e.d.LastEnd = at
	// AutoNext only when the break ended just now. A break that ran out while
	// the server was down hours ago is not a reason to start working at once.
	if next || (e.d.Settings.AutoNext && now.Sub(at) < time.Minute) {
		if err := e.beginWork(e.d.Target, now); err != nil {
			fmt.Fprintf(os.Stderr, "focus: could not start the next pomodoro: %v\n", err)
			e.d.Phase = focusIdle
		}
	}
}

// tick moves the session on to whatever phase the time says it is in. Run once
// a second by the server, and once on start so a session that ended while the
// server was down is caught up at once.
func (e *focusEngine) tick() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	changed := false
	// A loop because a long sleep can cover several transitions: work ran out,
	// and so did the break after it.
	for i := 0; i < 4 && e.step(now); i++ {
		changed = true
	}
	if changed {
		e.h.save(&e.d)
	}
}

// step makes one transition, if the time calls for one, and says whether it did.
func (e *focusEngine) step(now time.Time) bool {
	switch {
	case e.d.Phase == focusWork && !e.h.clockedOn(e.d.Target):
		e.d.Phase = focusIdle
		e.record(now, false, 0, false)
		e.d.LastEnd = now
		e.event(EventFocusStop, "the clock was stopped or moved elsewhere")
	case e.d.Phase == focusWork && !now.Before(e.d.Ends):
		e.announce(EventFocusEnd)
		e.completeWork(e.d.Ends)
	case e.d.Phase == focusBreak && !now.Before(e.d.Ends):
		e.announce(EventBreakEnd)
		e.endBreak(e.d.Ends, now, false)
	default:
		return false
	}
	return true
}

func (e *focusEngine) stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	switch e.d.Phase {
	case focusWork:
		e.stopWork(now, "stopped")
	case focusBreak:
		e.d.Phase = focusIdle
		e.d.LastEnd = now
		e.event(EventFocusStop, "break ended")
	default:
		return fmt.Errorf("no focus session is running")
	}
	e.h.save(&e.d)
	return nil
}

func (e *focusEngine) extend(mins float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.d.Phase == focusIdle {
		return fmt.Errorf("no focus session is running")
	}
	if mins <= 0 {
		mins = 5
	}
	e.d.Ends = e.d.Ends.Add(minutes(mins))
	e.event(EventFocusExtend, fmt.Sprintf("+%s min, until %s", fmtMins(mins), e.d.Ends.Format("15:04")))
	e.h.save(&e.d)
	return nil
}

// skip ends the phase under way as though its time were up: a work phase is
// finished early (and counts), a break is skipped and the next pomodoro starts.
func (e *focusEngine) skip() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	switch e.d.Phase {
	case focusWork:
		e.completeWork(now)
	case focusBreak:
		e.endBreak(now, now, true)
	default:
		return fmt.Errorf("no focus session is running")
	}
	e.h.save(&e.d)
	return nil
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// today counts the pomodoros finished today and the minutes of focus in all
// of today's work phases, stopped ones included.
func (e *focusEngine) today(now time.Time) (int, float64) {
	n, mins := 0, 0.0
	for _, r := range e.d.History {
		t, err := time.Parse(time.RFC3339, r.End)
		if err != nil {
			continue
		}
		if !sameDay(t.Local(), now.Local()) {
			// Newest first: once a record is before today, so are the rest.
			if t.Before(now) {
				break
			}
			continue
		}
		mins += r.Minutes
		if r.Completed {
			n++
		}
	}
	return n, math.Round(mins*10) / 10
}

func (e *focusEngine) state() common.FocusState {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	s := common.FocusState{Ok: true, Phase: e.d.Phase, Streak: e.d.Streak,
		Settings: e.d.Settings, Now: now.Format(time.RFC3339)}
	s.Today, s.TodayMinutes = e.today(now)
	if e.d.Target.Id != "" {
		task := e.d.Task
		if t, ok := e.h.describe(e.d.Target); ok {
			task = t
		}
		s.Hash, s.Title, s.File = task.Hash, task.Title, task.File
	}
	if e.d.Phase != focusIdle {
		s.LongBreak = e.d.Phase == focusBreak && e.d.LongBreak
		s.Started = e.d.Started.Format(time.RFC3339)
		s.Ends = e.d.Ends.Format(time.RFC3339)
		s.Total = e.d.Ends.Sub(e.d.Started).Seconds()
		s.Remaining = math.Max(0, e.d.Ends.Sub(now).Seconds())
	}
	return s
}

func (e *focusEngine) history(days int) common.FocusHistory {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	if days < 1 {
		days = 1
	}
	y, m, d := now.Date()
	from := time.Date(y, m, d-(days-1), 0, 0, 0, 0, now.Location())
	out := common.FocusHistory{Ok: true, Records: []common.FocusRecord{}}
	for _, r := range e.d.History {
		if t, err := time.Parse(time.RFC3339, r.End); err == nil && !t.Before(from) {
			out.Records = append(out.Records, r)
		}
	}
	out.Today, out.TodayMinutes = e.today(now)
	return out
}

// ---------------------------------------------------------------------------
// The live hooks: the real clock, the real files, the real event stream
// ---------------------------------------------------------------------------

type focusLive struct{}

func (focusLive) clockIn(t common.Target) (common.Target, focusTask, error) {
	tgt := t
	res, err := Clock().ClockIn(&tgt)
	if err != nil {
		return tgt, focusTask{}, err
	}
	if !res.Ok {
		return tgt, focusTask{}, fmt.Errorf("%s", res.Msg)
	}
	// ClockIn has turned the target into file+olp in place.
	task, _ := focusLive{}.describe(tgt)
	if task.Hash == "" && t.Type == "hash" {
		task.Hash = t.Id
	}
	return tgt, task, nil
}

func (focusLive) clockedOn(t common.Target) bool {
	c := Clock()
	cur := c.GetTarget()
	return c.IsClockActive() && cur != nil && cur.Filename == t.Filename && cur.Id == t.Id
}

func (focusLive) clockOut(t common.Target, end time.Time) {
	if (focusLive{}).clockedOn(t) {
		if res, _ := Clock().ClockOutAt(end); !res.Ok {
			fmt.Fprintf(os.Stderr, "focus: %s\n", res.Msg)
		}
	}
}

// The pomodoro's logbook line, newest first beside the CLOCK line it explains:
// a line edit like every other write (see Traps: line edits). The heading is
// found afresh because clocking out has just written to the file.
func (focusLive) logPomodoro(t common.Target, work, brk float64, at time.Time) error {
	f, sec := GetDb().GetFromTarget(&t, false)
	if f == nil || sec == nil || sec.Headline == nil {
		return fmt.Errorf("could not find %s in %s", t.Id, t.Filename)
	}
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", f.Filename)
	}
	ind := ownBodyIndent(lines, from, to, sec.Headline.Lvl)
	lines = logInto(lines, from, to, ind, Conf().ClockIntoDrawer, []string{pomodoroLine(ind, work, brk, at)})
	return writeLines(f.Filename, lines)
}

func pomodoroLine(ind string, work, brk float64, at time.Time) string {
	return fmt.Sprintf("%s- Pomodoro: %s min focus, %s min break %s", ind, fmtMins(work), fmtMins(brk), orgStamp(at))
}

func (focusLive) describe(t common.Target) (focusTask, bool) {
	if t.Id == "" {
		return focusTask{}, false
	}
	f, sec := GetDb().GetFromTarget(&t, false)
	if f == nil || sec == nil || sec.Headline == nil {
		return focusTask{}, false
	}
	return focusTask{Hash: sec.Hash, Title: common.GetSectionTitle(sec), File: f.Filename}, true
}

func (focusLive) publish(e Event) { Publish(e) }
func (focusLive) announce(kind, title, file string) {
	focusNote(kind, title, file, -1)
}

func (focusLive) save(d *focusData) {
	b, err := json.MarshalIndent(d, "", " ")
	if err != nil {
		return
	}
	if err := os.WriteFile(focusPath(), b, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "focus: could not save the session: %v\n", err)
	}
}

func focusPath() string {
	return path.Join(Conf().PlugManager.HomeDir, "focus_data.json")
}

var (
	focusOnce    sync.Once
	focusSession *focusEngine
)

// theFocus is the server's one session, read back from disk the first time it
// is asked for.
func theFocus() *focusEngine {
	focusOnce.Do(func() {
		e := newFocusEngine(focusLive{}, time.Now)
		if b, err := os.ReadFile(focusPath()); err == nil {
			if err := json.Unmarshal(b, &e.d); err != nil {
				fmt.Fprintf(os.Stderr, "focus: %s is not readable, starting idle: %v\n", focusPath(), err)
				e.d = focusData{}
			}
		}
		if e.d.Phase == "" {
			e.d.Phase = focusIdle
		}
		e.d.Settings = mergeFocusSettings(e.d.Settings, common.FocusStart{})
		focusSession = e
	})
	return focusSession
}

var focusTicking sync.Once

// The ticker. Once a second is plenty for a timer counted in minutes, costs
// nothing when idle, and needs no rescheduling when a phase is extended.
func startFocusTimer() {
	focusTicking.Do(func() {
		go func() {
			e := theFocus()
			e.tick()
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for range t.C {
				e.tick()
			}
		}()
	})
}

// ---------------------------------------------------------------------------
// The endpoints
// ---------------------------------------------------------------------------

// registerFocusRoutes adds the endpoints for focus / pomodoro sessions. Called
// from RestApi, which is also what starts the timer: a server with routes is a
// server whose session should be running.
func registerFocusRoutes(api *mux.Router) {
	api.HandleFunc("/focus", RequestFocusState).Methods("GET")
	api.HandleFunc("/focus/state", RequestFocusState).Methods("GET")
	api.HandleFunc("/focus/history", RequestFocusHistory).Methods("GET")
	api.HandleFunc("/focus/start", PostFocusStart).Methods("POST")
	api.HandleFunc("/focus/stop", PostFocusStop).Methods("POST")
	api.HandleFunc("/focus/extend", PostFocusExtend).Methods("POST")
	api.HandleFunc("/focus/skip", PostFocusSkip).Methods("POST")
	startFocusTimer()
}

// Every focus answer is the state after the change, so a client redraws from
// the reply rather than asking again. A refusal is the state too, with Ok
// false and the reason, answered 409: nothing was wrong with the request, the
// session was just not in a state to do it.
func answerFocus(w http.ResponseWriter, err error) {
	s := theFocus().state()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		s.Ok = false
		s.Msg = err.Error()
		w.WriteHeader(http.StatusConflict)
	}
	json.NewEncoder(w).Encode(s)
}

/* SDOC: API
* GET /focus — The Focus Session Now

	What the focus (pomodoro) session is doing: working on a heading, on a
	break, or idle. =/focus/state= is the same thing.

	*Method:* =GET=

	*Response:* A =FocusState=:
	| Field          | Meaning                                                             |
	|----------------+---------------------------------------------------------------------|
	| =Phase=        | =work=, =break= or =idle=.                                          |
	| =LongBreak=    | The break under way is a long one.                                  |
	| =Hash=, =Title=, =File= | The task. When idle, the last task, to offer another on it. |
	| =Started=, =Ends= | RFC3339; empty when idle.                                        |
	| =Remaining=, =Total= | Seconds left and seconds in the phase, counted by the server. |
	| =Now=          | The server's time, so a client with a wrong clock can allow for it. |
	| =Streak=       | Pomodoros finished since the last long break.                       |
	| =Today=, =TodayMinutes= | Pomodoros finished today, and minutes of focus today.      |
	| =Settings=     | ={Work, Short, Long, LongEvery, AutoNext}=, minutes.                |

	The hash is worked out afresh on each answer, since clocking out writes to
	the heading's file and can change it.
EDOC */
func RequestFocusState(w http.ResponseWriter, r *http.Request) {
	answerFocus(w, nil)
}

/* SDOC: API
* GET /focus/history — Focus Sessions Lately

	The work phases of the last few days, newest first: each with its heading,
	start and end, minutes, whether it was finished (=Completed=) or stopped,
	and the break that followed. Thirty days are kept.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type | Required | Description                             |
	|-----------+------+----------+-----------------------------------------|
	| =days=    | int  | no       | How many days, today included. Default 1. |

	*Response:* ={"Ok": true, "Today": 3, "TodayMinutes": 80, "Records": [...]}=
EDOC */
func RequestFocusHistory(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(theFocus().history(days))
}

/* SDOC: API
* POST /focus/start — Start a Pomodoro

	Clocks in on a heading and starts a work phase. When it runs out the server
	clocks out (writing the =CLOCK:= line), adds
	=- Pomodoro: 25 min focus, 5 min break [stamp]= to the heading's logbook,
	and starts a break; when the break runs out the session goes idle, or with
	=AutoNext= starts the next pomodoro. Every fourth break (=LongEvery=) is a
	long one. A session already running is ended first.

	The timer runs on the server and survives a restart. Stopping the clock or
	clocking into another heading from anywhere else stops the session.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field       | Type   | Required | Description                                                   |
	|-------------+--------+----------+---------------------------------------------------------------|
	| =Hash=      | string | no       | The heading. With neither this nor =Target=, the last task.   |
	| =Target=    | Target | no       | The heading as a target (=file+olp=, =id=, ...).              |
	| =Work=      | number | no       | Minutes of work. Fractions allowed.                           |
	| =Short=     | number | no       | Minutes of a short break.                                     |
	| =Long=      | number | no       | Minutes of a long break.                                      |
	| =LongEvery= | int    | no       | Every how many pomodoros the break is long.                   |
	| =AutoNext=  | bool   | no       | Start the next pomodoro when a break ends.                    |

	A field left out or zero keeps what was used last (25, 5, 15, 4, false at
	first), so every client starts with the same lengths.

	*Response:* The =FocusState= after the change. A refusal is the state with
	=Ok: false= and the reason in =Msg=, status 409.

	*Events:* =focusstart=, and later =focusend=, =breakstart=, =breakend=;
	=focusstop= when stopped early; =focusextend= when given more time. Each
	carries the heading's =hash=, =headline= and =file=.
EDOC */
func PostFocusStart(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		answerErr(w, err)
		return
	}
	var req common.FocusStart
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	answerFocus(w, theFocus().start(req))
}

/* SDOC: API
* POST /focus/stop — Stop the Focus Session

	Ends the session now. A work phase is clocked out and remembered as not
	finished (no logbook note); a break simply ends. Answers the =FocusState=,
	or =Ok: false= when nothing is running.

	*Method:* =POST=
EDOC */
func PostFocusStop(w http.ResponseWriter, r *http.Request) {
	answerFocus(w, theFocus().stop())
}

/* SDOC: API
* POST /focus/extend — More Time

	Gives the phase under way more minutes. Answers the =FocusState=.

	*Method:* =POST=

	*Request Body (JSON):* ={"Minutes": 5}=; 5 when left out.
EDOC */
func PostFocusExtend(w http.ResponseWriter, r *http.Request) {
	var req struct{ Minutes float64 }
	if body, err := io.ReadAll(r.Body); err == nil && len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			answerErr(w, err)
			return
		}
	}
	answerFocus(w, theFocus().extend(req.Minutes))
}

/* SDOC: API
* POST /focus/skip — Skip Ahead

	Ends the phase under way as though its time were up. A work phase is
	finished early and counts as a pomodoro (clocked out, noted, break begins);
	a break is skipped and the next pomodoro starts on the same heading.
	Answers the =FocusState=.

	*Method:* =POST=
EDOC */
func PostFocusSkip(w http.ResponseWriter, r *http.Request) {
	answerFocus(w, theFocus().skip())
}

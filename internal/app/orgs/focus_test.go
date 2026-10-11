package orgs

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// fakeFocus stands in for the clock, the files and the event stream, and
// writes down everything the engine asked of them.
type fakeFocus struct {
	clocked  *common.Target
	clockIns int
	outs     []time.Time
	notes    []string
	events   []string
	saves    int
}

func (f *fakeFocus) clockIn(t common.Target) (common.Target, focusTask, error) {
	if t.Id == "missing" {
		return t, focusTask{}, fmt.Errorf("failed to find target in database")
	}
	olp := common.Target{Type: "file+olp", Filename: "a.org", Id: "Write the report"}
	f.clocked = &olp
	f.clockIns++
	return olp, focusTask{Hash: "h1", Title: "Write the report", File: "a.org"}, nil
}
func (f *fakeFocus) clockedOn(t common.Target) bool {
	return f.clocked != nil && f.clocked.Id == t.Id && f.clocked.Filename == t.Filename
}
func (f *fakeFocus) clockOut(t common.Target, end time.Time) {
	if f.clockedOn(t) {
		f.outs = append(f.outs, end)
		f.clocked = nil
	}
}
func (f *fakeFocus) logPomodoro(t common.Target, work, brk float64, at time.Time) error {
	f.notes = append(f.notes, pomodoroLine("", work, brk, at))
	return nil
}
func (f *fakeFocus) describe(t common.Target) (focusTask, bool) {
	return focusTask{Hash: "h1", Title: "Write the report", File: "a.org"}, t.Id != ""
}
func (f *fakeFocus) publish(e Event)   { f.events = append(f.events, e.Kind) }
func (f *fakeFocus) save(d *focusData) { f.saves++ }

type fakeTime struct{ t time.Time }

func (c *fakeTime) now() time.Time      { return c.t }
func (c *fakeTime) add(d time.Duration) { c.t = c.t.Add(d) }
func (c *fakeTime) mins(m float64)      { c.add(minutes(m)) }
func newFocusTest() (*focusEngine, *fakeFocus, *fakeTime) {
	clk := &fakeTime{t: time.Date(2026, 10, 10, 9, 0, 0, 0, time.Local)}
	f := &fakeFocus{}
	return newFocusEngine(f, clk.now), f, clk
}

func joined(s []string) string { return strings.Join(s, " ") }

func TestFocusWorkBreakIdle(t *testing.T) {
	e, f, clk := newFocusTest()
	if err := e.start(common.FocusStart{Hash: "h1"}); err != nil {
		t.Fatal(err)
	}
	s := e.state()
	if s.Phase != "work" || s.Remaining != 25*60 || s.Title != "Write the report" {
		t.Fatalf("after start: %+v", s)
	}
	clk.mins(24)
	e.tick()
	if e.state().Phase != "work" || len(f.outs) != 0 {
		t.Fatal("work ended early")
	}
	// The tick comes a few seconds late; the clock still stops at 25 minutes.
	clk.mins(1)
	clk.add(3 * time.Second)
	e.tick()
	s = e.state()
	if s.Phase != "break" || s.LongBreak || s.Today != 1 || s.Streak != 1 {
		t.Fatalf("after work: %+v", s)
	}
	if len(f.outs) != 1 || !f.outs[0].Equal(time.Date(2026, 10, 10, 9, 25, 0, 0, time.Local)) {
		t.Fatalf("clocked out at %v", f.outs)
	}
	if len(f.notes) != 1 || f.notes[0] != "- Pomodoro: 25 min focus, 5 min break [2026-10-10 Sat 09:25]" {
		t.Fatalf("note %q", f.notes)
	}
	clk.mins(5)
	e.tick()
	if e.state().Phase != "idle" {
		t.Fatal("break did not end")
	}
	if got := joined(f.events); got != "focusstart focusend breakstart breakend" {
		t.Fatalf("events: %s", got)
	}
	// Idle still names the last task, so a client can offer another.
	if s := e.state(); s.Hash != "h1" || s.Remaining != 0 || s.Ends != "" {
		t.Fatalf("idle: %+v", s)
	}
}

func TestFocusLongBreakEveryFourth(t *testing.T) {
	e, _, clk := newFocusTest()
	for i := 1; i <= 4; i++ {
		// The first names the heading; the rest are "another one".
		req := common.FocusStart{}
		if i == 1 {
			req.Hash = "h1"
		}
		if err := e.start(req); err != nil {
			t.Fatal(err)
		}
		clk.mins(25)
		e.tick()
		s := e.state()
		if (i == 4) != s.LongBreak {
			t.Fatalf("pomodoro %d: long break %v", i, s.LongBreak)
		}
		if i == 4 && s.Remaining != 15*60 {
			t.Fatalf("long break is %v seconds", s.Remaining)
		}
		clk.mins(s.Remaining / 60)
		e.tick()
	}
	if s := e.state(); s.Today != 4 || s.Streak != 0 {
		t.Fatalf("after four: %+v", s)
	}
}

func TestFocusStreakResetsAfterAGap(t *testing.T) {
	e, _, clk := newFocusTest()
	e.start(common.FocusStart{Hash: "h1"})
	clk.mins(30)
	e.tick()
	if e.state().Streak != 1 {
		t.Fatal("no streak")
	}
	clk.mins(60)
	e.start(common.FocusStart{})
	if e.state().Streak != 0 {
		t.Fatal("an hour away should start a new set")
	}
}

func TestFocusResumeCatchesUpSeveralPhases(t *testing.T) {
	// A server down through the end of the work phase and the break: one tick
	// on the way back clocks out at 9:25 and ends the break at 9:30.
	e, f, clk := newFocusTest()
	e.d.Settings.AutoNext = true
	e.start(common.FocusStart{Hash: "h1"})
	clk.mins(120)
	e.tick()
	if s := e.state(); s.Phase != "idle" {
		t.Fatalf("auto next after a long sleep should not start work: %+v", s)
	}
	if !f.outs[0].Equal(time.Date(2026, 10, 10, 9, 25, 0, 0, time.Local)) {
		t.Fatalf("clocked out at %v", f.outs[0])
	}
}

func TestFocusAutoNext(t *testing.T) {
	e, f, clk := newFocusTest()
	yes := true
	e.start(common.FocusStart{Hash: "h1", Work: 0.5, Short: 0.5, AutoNext: &yes})
	clk.add(30 * time.Second)
	e.tick()
	clk.add(30 * time.Second)
	e.tick()
	if s := e.state(); s.Phase != "work" || f.clockIns != 2 {
		t.Fatalf("next pomodoro not started: %+v", s)
	}
	if f.notes[0] != "- Pomodoro: 0.5 min focus, 0.5 min break [2026-10-10 Sat 09:00]" {
		t.Fatalf("note %q", f.notes[0])
	}
}

func TestFocusStopExtendSkip(t *testing.T) {
	e, f, clk := newFocusTest()
	if err := e.stop(); err == nil {
		t.Fatal("stop with nothing running should refuse")
	}
	e.start(common.FocusStart{Hash: "h1"})
	e.extend(5)
	if s := e.state(); s.Remaining != 30*60 {
		t.Fatalf("extend: %v", s.Remaining)
	}
	clk.mins(10)
	e.stop()
	s := e.state()
	if s.Phase != "idle" || s.Today != 0 || len(f.notes) != 0 || len(f.outs) != 1 {
		t.Fatalf("stopped work counts as nothing finished: %+v notes %v", s, f.notes)
	}
	h := e.history(1)
	if len(h.Records) != 1 || h.Records[0].Completed || h.Records[0].Minutes != 10 || h.TodayMinutes != 10 {
		t.Fatalf("history %+v", h)
	}

	// Skip in work finishes early and counts; skip in a break starts the next.
	e.start(common.FocusStart{})
	clk.mins(20)
	e.skip()
	if s := e.state(); s.Phase != "break" || s.Today != 1 {
		t.Fatalf("skip in work: %+v", s)
	}
	e.skip()
	if s := e.state(); s.Phase != "work" {
		t.Fatalf("skip in a break: %+v", s)
	}
	if got := joined(f.events); got != "focusstart focusextend focusstop focusstart focusend breakstart breakend focusstart" {
		t.Fatalf("events: %s", got)
	}
}

func TestFocusClockStoppedElsewhereStopsTheSession(t *testing.T) {
	e, f, _ := newFocusTest()
	e.start(common.FocusStart{Hash: "h1"})
	f.clocked = nil // somebody pressed clock out in another window
	e.tick()
	if s := e.state(); s.Phase != "idle" {
		t.Fatalf("still %s", s.Phase)
	}
	if f.events[len(f.events)-1] != "focusstop" {
		t.Fatalf("events: %v", f.events)
	}
}

func TestFocusStartRefusals(t *testing.T) {
	e, _, _ := newFocusTest()
	if err := e.start(common.FocusStart{}); err == nil {
		t.Fatal("no heading and no last task should refuse")
	}
	if err := e.start(common.FocusStart{Hash: "missing"}); err == nil || e.state().Phase != "idle" {
		t.Fatal("a heading that cannot be clocked into should leave the session idle")
	}
}

func TestFocusSettingsRemembered(t *testing.T) {
	e, _, _ := newFocusTest()
	e.start(common.FocusStart{Hash: "h1", Work: 50, Short: 10})
	e.stop()
	e.start(common.FocusStart{})
	s := e.state()
	if s.Settings.Work != 50 || s.Settings.Short != 10 || s.Settings.Long != 15 || s.Remaining != 50*60 {
		t.Fatalf("settings %+v", s.Settings)
	}
}

func TestFocusTodayIgnoresYesterday(t *testing.T) {
	e, _, clk := newFocusTest()
	e.start(common.FocusStart{Hash: "h1"})
	clk.mins(30)
	e.tick()
	clk.add(24 * time.Hour)
	if s := e.state(); s.Today != 0 {
		t.Fatalf("yesterday counted: %+v", s)
	}
	if h := e.history(2); len(h.Records) != 1 {
		t.Fatalf("two days of history: %+v", h)
	}
}

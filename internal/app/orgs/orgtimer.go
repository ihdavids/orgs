package orgs

// The relative timer and idle time (org manual 8.6, 8.4.3).
//
// The timer is org-timer: a stopwatch for taking notes against (a meeting, a
// talk), each note a list item stamped with the time since it started -
// "- 0:03:12 :: they agreed". It starts, pauses, continues and stops, can be
// set to a count down that says when it is done (org-timer-set-timer, through
// deliverNote), and is the server's, so worg and the terminal see one timer.
// It is kept in timer_data.json beside clock_data.json so a restart keeps it.
//
// Idle time is org-clock's resolution: a clock left running while nobody was
// there. Clients notice the absence (worg by the page's input and visibility)
// and ask POST /clock/resolve to keep the time, subtract the idle minutes from
// the running clock (its start moves on by them), or clock out when the
// absence began. clockIdleMinutes in the yaml (org-clock-idle-time) is how
// long counts as away; 0, the default, is never.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/desktop"
)

// TimerState is the timer as a client draws it.
type TimerState struct {
	Ok      bool
	Msg     string `json:",omitempty"`
	Running bool
	Paused  bool
	// Counting down rather than up: Remaining is what is left.
	Countdown bool
	Elapsed   float64 // seconds
	Remaining float64 `json:",omitempty"`
	Display   string  // H:MM:SS, what a note is stamped with
	Started   string  `json:",omitempty"` // RFC 3339
}

type orgTimer struct {
	mu       sync.Mutex
	Start    time.Time     `json:"start"`
	PausedAt *time.Time    `json:"pausedAt,omitempty"`
	Paused   time.Duration `json:"paused"`
	Running  bool          `json:"running"`
	// A count down's length; zero counts up.
	Length time.Duration `json:"length,omitempty"`
	done   *time.Timer
}

var timer = &orgTimer{}
var timerLoaded sync.Once

func timerPath() string { return path.Join(Conf().PlugManager.HomeDir, "timer_data.json") }

func theTimer() *orgTimer {
	timerLoaded.Do(func() {
		if b, err := os.ReadFile(timerPath()); err == nil {
			json.Unmarshal(b, timer)
			timer.armCountdown()
		}
	})
	return timer
}

func (t *orgTimer) save() {
	b, _ := json.MarshalIndent(t, "", " ")
	_ = os.WriteFile(timerPath(), b, 0o644)
}

func (t *orgTimer) elapsed(now time.Time) time.Duration {
	if !t.Running {
		return 0
	}
	end := now
	if t.PausedAt != nil {
		end = *t.PausedAt
	}
	return end.Sub(t.Start) - t.Paused
}

// timerDisplay is org-timer's H:MM:SS.
func timerDisplay(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func (t *orgTimer) state(now time.Time) TimerState {
	e := t.elapsed(now)
	st := TimerState{Ok: true, Running: t.Running, Paused: t.PausedAt != nil, Elapsed: e.Seconds(), Display: timerDisplay(e)}
	if t.Running {
		st.Started = t.Start.Format(time.RFC3339)
	}
	if t.Length > 0 && t.Running {
		st.Countdown = true
		left := t.Length - e
		st.Remaining = left.Seconds()
		st.Display = timerDisplay(left)
	}
	return st
}

// armCountdown sets the alarm for a running count down.
func (t *orgTimer) armCountdown() {
	if t.done != nil {
		t.done.Stop()
		t.done = nil
	}
	if !t.Running || t.Length <= 0 || t.PausedAt != nil {
		return
	}
	left := t.Length - t.elapsed(time.Now())
	if left < 0 {
		left = 0
	}
	t.done = time.AfterFunc(left, func() {
		t.mu.Lock()
		t.Running, t.Length, t.PausedAt, t.Paused = false, 0, nil, 0
		t.save()
		t.mu.Unlock()
		deliverNote(desktop.Note{Title: "Timer done", Body: "The count down has finished.", Group: "orgs-timer"}, "", "", "")
		Publish(Event{Kind: "timerdone", Msg: "the count down has finished"})
	})
}

// TimerOp is POST /timer.
type TimerOp struct {
	// start (again from zero), pause, continue, stop, countdown (Minutes), set
	// (Offset "0:10:00": start as though that long ago).
	Op      string
	Minutes float64 `json:",omitempty"`
	Offset  string  `json:",omitempty"`
}

var hmsRe = regexp.MustCompile(`^(?:(\d+):)?(\d{1,2}):(\d{2})$`)

func parseHMS(s string) (time.Duration, error) {
	m := hmsRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("%q is not H:MM:SS", s)
	}
	n := func(v string) time.Duration { x, _ := strconv.Atoi(v); return time.Duration(x) }
	if m[1] == "" {
		// M:SS
		return n(m[2])*time.Minute + n(m[3])*time.Second, nil
	}
	return n(m[1])*time.Hour + n(m[2])*time.Minute + n(m[3])*time.Second, nil
}

func (t *orgTimer) do(op TimerOp, now time.Time) TimerState {
	t.mu.Lock()
	defer t.mu.Unlock()
	fail := func(msg string) TimerState { s := t.state(now); s.Ok, s.Msg = false, msg; return s }
	switch op.Op {
	case "start":
		t.Start, t.PausedAt, t.Paused, t.Running, t.Length = now, nil, 0, true, 0
	case "set":
		d, err := parseHMS(op.Offset)
		if err != nil {
			return fail(err.Error())
		}
		t.Start, t.PausedAt, t.Paused, t.Running, t.Length = now.Add(-d), nil, 0, true, 0
	case "countdown":
		if op.Minutes <= 0 {
			return fail("a count down needs minutes")
		}
		t.Start, t.PausedAt, t.Paused, t.Running = now, nil, 0, true
		t.Length = time.Duration(op.Minutes * float64(time.Minute))
	case "pause":
		if !t.Running || t.PausedAt != nil {
			return fail("the timer is not running")
		}
		p := now
		t.PausedAt = &p
	case "continue":
		if t.PausedAt == nil {
			return fail("the timer is not paused")
		}
		t.Paused += now.Sub(*t.PausedAt)
		t.PausedAt = nil
	case "stop":
		t.Running, t.PausedAt, t.Paused, t.Length = false, nil, 0, 0
	default:
		return fail(fmt.Sprintf("no timer operation %q: start, pause, continue, stop, countdown, set", op.Op))
	}
	t.armCountdown()
	t.save()
	Publish(Event{Kind: "timer", Msg: op.Op})
	return t.state(now)
}

var timerItemRe = regexp.MustCompile(`^\s*- \d+:\d{2}:\d{2} ::`)

// timerItemLines adds "- H:MM:SS :: text" at the end of a heading's own text:
// straight after the last timer item when the text ends with one, so the
// notes are one list, else after a blank line.
func timerItemLines(lines []string, from, to int, ind, item string) []string {
	own := ownLinesEnd(lines, from, to)
	at := own
	for at > from && strings.TrimSpace(lines[at]) == "" {
		at--
	}
	add := []string{ind + item}
	if !timerItemRe.MatchString(lines[at]) {
		add = append([]string{""}, add...)
	}
	return append(lines[:at+1], append(add, lines[at+1:]...)...)
}

// TimerItem is POST /timer/item: a note stamped with the timer.
type TimerItem struct {
	// The heading it goes under: by hash, or the clocked one when empty.
	Hash string
	Text string
}

func (t *orgTimer) item(req TimerItem, now time.Time) common.ResultMsg {
	t.mu.Lock()
	running := t.Running
	stamp := timerDisplay(t.elapsed(now))
	t.mu.Unlock()
	if !running {
		return common.ResultMsg{Msg: "the timer is not running: start it first"}
	}
	var f *common.OrgFile
	sec := GetDb().FindByHash(req.Hash)
	if req.Hash == "" && Clock().IsClockActive() {
		f, sec = GetDb().GetFromTarget(Clock().GetTarget(), false)
	} else if sec != nil {
		f = GetDb().FileFromSection(sec)
	}
	if f == nil || sec == nil || sec.Headline == nil {
		return common.ResultMsg{Msg: "say which heading (or clock in to one)"}
	}
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return common.ResultMsg{Msg: "could not read " + f.Filename}
	}
	ind := ownBodyIndent(lines, from, to, sec.Headline.Lvl)
	item := "- " + stamp + " :: " + strings.TrimSpace(strings.ReplaceAll(req.Text, "\n", " "))
	if err := writeLines(f.Filename, timerItemLines(lines, from, to, ind, item)); err != nil {
		return common.ResultMsg{Msg: err.Error()}
	}
	return common.ResultMsg{Ok: true, Msg: item}
}

// ClockResolve is POST /clock/resolve: what to do with idle minutes on the
// running clock.
type ClockResolve struct {
	// keep, subtract (the start moves on by Minutes) or stop (clock out
	// Minutes ago, when the absence began).
	How     string
	Minutes float64
}

func resolveIdle(req ClockResolve, now time.Time) common.ResultMsg {
	c := Clock()
	if !c.IsClockActive() || c.Time == nil {
		return common.ResultMsg{Msg: "no clock is running"}
	}
	idle := time.Duration(req.Minutes * float64(time.Minute))
	if idle < 0 {
		return common.ResultMsg{Msg: "idle minutes cannot be negative"}
	}
	if most := now.Sub(c.Time.Start); idle > most {
		idle = most
	}
	switch req.How {
	case "keep", "":
		return common.ResultMsg{Ok: true, Msg: "kept"}
	case "subtract":
		c.Time.Start = c.Time.Start.Add(idle)
		c.WriteOutClock()
		Publish(Event{Kind: EventClockIn, Msg: fmt.Sprintf("%.0f idle minutes taken off the clock", idle.Minutes())})
		return common.ResultMsg{Ok: true, Msg: fmt.Sprintf("%.0f minutes taken off", idle.Minutes())}
	case "stop":
		return mustResult(c.ClockOutAt(now.Add(-idle)))
	}
	return common.ResultMsg{Msg: fmt.Sprintf("no way %q to resolve idle time: keep, subtract or stop", req.How)}
}

func mustResult(r common.ResultMsg, err error) common.ResultMsg {
	if err != nil {
		return common.ResultMsg{Msg: err.Error()}
	}
	return r
}

/* SDOC: API
* GET /timer, POST /timer — The Relative Timer
* POST /timer/item — A Note Stamped With It
* POST /clock/resolve — Idle Time On The Running Clock

	=GET /timer= is ={Ok, Running, Paused, Countdown, Elapsed, Remaining, Display, Started}=.
	=POST /timer= takes ={"Op": "start"}= (or =pause=, =continue=, =stop=,
	={"Op": "countdown", "Minutes": 25}= - a notification when it ends -,
	={"Op": "set", "Offset": "0:10:00"}=) and answers the state.

	=POST /timer/item= ={"Hash": "...", "Text": "they agreed"}= writes =- 0:03:12 :: they agreed=
	at the end of the heading's own text (the clocked heading when =Hash= is
	empty), straight after the last such item.

	=POST /clock/resolve= ={"How": "subtract", "Minutes": 20}= takes idle
	minutes off the running clock; ="stop"= clocks out that long ago; ="keep"=
	does nothing. =GET /clock/idle= is ={Minutes}=, the yaml's
	=clockIdleMinutes= (0 is off).
EDOC */
func registerTimerRoutes(api *mux.Router) {
	api.HandleFunc("/timer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var op TimerOp
			if err := json.Unmarshal(body, &op); err != nil {
				json.NewEncoder(w).Encode(TimerState{Msg: err.Error()})
				return
			}
			json.NewEncoder(w).Encode(theTimer().do(op, time.Now()))
			return
		}
		t := theTimer()
		t.mu.Lock()
		st := t.state(time.Now())
		t.mu.Unlock()
		json.NewEncoder(w).Encode(st)
	}).Methods("GET", "POST")
	api.HandleFunc("/timer/item", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		var req TimerItem
		if err := json.Unmarshal(body, &req); err != nil {
			json.NewEncoder(w).Encode(common.ResultMsg{Msg: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(theTimer().item(req, time.Now()))
	}).Methods("POST")
	api.HandleFunc("/clock/resolve", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		var req ClockResolve
		if err := json.Unmarshal(body, &req); err != nil {
			json.NewEncoder(w).Encode(common.ResultMsg{Msg: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(resolveIdle(req, time.Now()))
	}).Methods("POST")
	api.HandleFunc("/clock/idle", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"Minutes": Conf().ClockIdleMinutes})
	}).Methods("GET")
}

package focus

// The live view: one line, redrawn every second, that follows the session.
//
//	🍅 12:31 ━━━━━━━━──────────── Write the report     q stop · + 5 min · n skip · d leave
//
// The countdown is ticked here, from the Remaining the server gave and this
// machine's own clock, because a second-by-second request would be absurd. What
// the session *is* comes only from the server: /events says something changed
// and the state is asked for again, so a pomodoro stopped from worg ends here
// too. A poll every thirty seconds covers a stream that has quietly died.
//
// A finished phase leaves a line above the live one (and rings the bell), so
// the scrollback is a small log of the session.

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

var focusKinds = []string{"focusstart", "focusend", "breakstart", "breakend", "focusstop", "focusextend"}

func (self *Focus) watch(core *commands.Core) {
	if !commands.Interactive() {
		printStatus(read(core), self.Idle)
		return
	}
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		commands.Fail("could not read keys from this terminal: %v", err)
	}
	c := commands.C
	fmt.Print("\033[?25l") // the cursor would blink on the countdown
	defer func() {
		fmt.Print("\r\033[K\033[?25h")
		term.Restore(fd, old)
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	keys := make(chan byte, 8)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				close(keys)
				return
			}
			keys <- buf[0]
		}
	}()

	changed := make(chan struct{}, 8)
	go common.Events(ctx, &core.Rest, common.EventOpts{Kinds: append(focusKinds, "clockout")},
		func(e common.OrgEvent) bool {
			select {
			case changed <- struct{}{}:
			default:
			}
			return true
		})

	v := &liveView{}
	v.load(core)
	say := func(s string) { fmt.Printf("\r\033[K%s\r\n", s) }
	if v.st.Phase == "work" {
		say(fmt.Sprintf("%s%s%s %s", c(commands.AnsiBold), v.st.Title, c(commands.AnsiReset),
			c(commands.AnsiDim)+commands.BaseName(v.st.File)+c(commands.AnsiReset)))
	}

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	poll := time.NewTicker(30 * time.Second)
	defer poll.Stop()

	act := func(path string, body map[string]any) {
		st, err := post(core, path, body)
		if err != nil {
			v.note = err.Error()
			return
		}
		if !st.Ok {
			v.note = st.Msg
		}
		v.update(st, say)
	}

	for {
		v.draw()
		select {
		case <-ctx.Done():
			return
		case k, ok := <-keys:
			if !ok {
				return
			}
			v.note = ""
			switch k {
			case 'q', 'Q':
				if v.st.Phase != "idle" {
					act("focus/stop", map[string]any{})
				}
				return
			case 'd', 'D', 27, 3, 4: // d, esc, ctrl-c, ctrl-d: leave it running
				if v.st.Phase != "idle" {
					say(c(commands.AnsiDim) + "still running on the server · orgs focus to come back" + c(commands.AnsiReset))
				}
				return
			case '+', '=':
				act("focus/extend", map[string]any{"Minutes": 5})
			case 'n', 'N', 's', 'S':
				act("focus/skip", map[string]any{})
			case '\r', '\n', ' ':
				if v.st.Phase == "idle" {
					act("focus/start", map[string]any{})
				}
			}
		case <-changed:
			v.load(core, say)
		case <-poll.C:
			v.load(core, say)
		case <-tick.C:
			// The server moves the phase on within a second of its end and
			// says so on the stream; asking once here covers a lost event.
			if v.st.Phase != "idle" && v.remaining() <= -2 {
				v.load(core, say)
			}
		}
	}
}

type liveView struct {
	st   common.FocusState
	end  time.Time // when the phase ends, on this machine's clock
	note string
	down bool
}

func (v *liveView) remaining() float64 { return time.Until(v.end).Seconds() }

func (v *liveView) load(core *commands.Core, say ...func(string)) {
	st, err := commands.SendReceiveGetErr[common.FocusState](core, "focus", map[string]string{})
	if err != nil {
		v.down = true
		return
	}
	v.down = false
	if len(say) > 0 {
		v.update(st, say[0])
	} else {
		v.set(st)
	}
}

func (v *liveView) set(st common.FocusState) {
	v.st = st
	v.end = time.Now().Add(time.Duration(st.Remaining * float64(time.Second)))
}

// update takes a new state and, when the phase changed, leaves a line saying
// what just finished.
func (v *liveView) update(st common.FocusState, say func(string)) {
	prev := v.st
	v.set(st)
	if prev.Phase == st.Phase && prev.Started == st.Started {
		return
	}
	c := commands.C
	stamp := c(commands.AnsiDim) + time.Now().Format("15:04") + c(commands.AnsiReset)
	switch {
	case st.Phase == "break":
		say(fmt.Sprintf("%s %s %s%s%s · %s, until %s\a", stamp, Glyph("work"), c(commands.AnsiGreen),
			"pomodoro done", c(commands.AnsiReset), phaseName(st), endClock(st)))
	case st.Phase == "work" && prev.Phase == "break":
		say(fmt.Sprintf("%s %s back to %s, until %s\a", stamp, Glyph("work"), st.Title, endClock(st)))
	case st.Phase == "work":
		say(fmt.Sprintf("%s %s %s, until %s", stamp, Glyph("work"), st.Title, endClock(st)))
	case st.Phase == "idle" && prev.Phase == "break":
		say(fmt.Sprintf("%s %s break over · %s today\a", stamp, Glyph("break"), pomodoros(st.Today)))
	case st.Phase == "idle" && prev.Phase == "work":
		say(fmt.Sprintf("%s stopped · %s today", stamp, pomodoros(st.Today)))
	}
}

func endClock(st common.FocusState) string {
	if t, ok := parseTime(st.Ends); ok {
		return t.Local().Format("15:04")
	}
	return "?"
}

func (v *liveView) draw() {
	c := commands.C
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 20 {
		width = w
	}
	var line, hint string
	switch v.st.Phase {
	case "work", "break":
		rem := v.remaining()
		colour := commands.AnsiRed
		if v.st.Phase == "break" {
			colour = commands.AnsiGreen
		}
		label := v.st.Title
		if v.st.Phase == "break" {
			label = phaseName(v.st)
		}
		line = fmt.Sprintf("%s %s%s%s %s%s%s %s", Glyph(v.st.Phase), c(commands.AnsiBold), Countdown(rem), c(commands.AnsiReset),
			c(colour), Progress(v.st, rem, 16), c(commands.AnsiReset), "%s")
		hint = "q stop · + 5 min · n skip · d leave"
		if v.st.Phase == "break" {
			hint = "q end · + 5 min · n next pomodoro · d leave"
		}
		used := commands.RuneLen(Countdown(rem)) + 16 + 6 + commands.RuneLen(hint) + 3
		line = fmt.Sprintf(line, ellipsis(label, max(8, width-used)))
	default:
		line = "done"
		if v.st.Today > 0 {
			line = fmt.Sprintf("done · %s today", pomodoros(v.st.Today))
		}
		hint = "q quit"
		if v.st.Title != "" {
			hint = "enter: another on " + ellipsis(v.st.Title, 30) + " · q quit"
		}
	}
	if v.down {
		hint = "server unreachable, retrying"
	}
	if v.note != "" {
		hint = v.note
	}
	fmt.Printf("\r\033[K%s   %s%s%s", line, c(commands.AnsiDim), hint, c(commands.AnsiReset))
}

// printStatus is `orgs focus status`: one line, or -idle's text, or nothing.
func printStatus(st common.FocusState, idle string) {
	if commands.RenderOne(st, nil) {
		return
	}
	line := StatusLine(st, false)
	if line == "" {
		line = strings.TrimSpace(idle)
	}
	if line != "" {
		fmt.Println(line)
	}
}

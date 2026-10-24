package dash

// orgs dash - the day on one screen.
//
//	orgs dash          today, what is running, what is late, habits, the inbox
//	orgs dash -w       the same, kept current
//	orgs dash -json    every section as data
//
// Nothing here is new information: the agenda, the clock, the focus timer,
// the habit tracker, the inbox and the watch notifications each already have
// an endpoint and a command. What was missing was the glance - the one
// screen that answers "what does today want from me" without six commands.
//
// The six requests go out together and each section stands alone: a server
// without the review routes, or a habit file that will not parse, costs that
// card a dim line, never the dashboard. It only reads; every card's action is
// the command named in the footer.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/clocks"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type DashCmd struct {
	Watch bool
}

func (self *DashCmd) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *DashCmd) StartPlugin(m *common.PluginManager)       {}
func (self *DashCmd) HelpGroup() string                         { return "Finding things" }
func (self *DashCmd) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Watch, "w", false, "keep it on screen, redrawn as things change")
}

func (self *DashCmd) Exec(core *commands.Core) {
	commands.FreeArgs(commands.Find("dash").Flags)
	if !self.Watch || commands.Machine() {
		d := build(fetch(core), time.Now())
		if commands.RenderOne(d, nil) {
			return
		}
		fmt.Print(render(d, termWidth(), time.Now()))
		return
	}
	self.watch(core)
}

// fetch asks the six endpoints at once. A dashboard waits for its slowest
// card, so asking one after another would make it the sum of them all.
func fetch(core *commands.Core) inputs {
	in := inputs{server: core.Rest.Url}
	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	run(func() {
		in.agenda, in.agendaErr = commands.SendReceiveGetErr[common.AgendaResult](core, "agenda", nil)
	})
	run(func() {
		in.clock, in.clockErr = commands.SendReceiveGetErr[clocks.ClockData](core, "clock", nil)
	})
	run(func() {
		in.focus, in.focusErr = commands.SendReceiveGetErr[common.FocusState](core, "focus", nil)
	})
	run(func() {
		// Seven days is all the strip shows; the tracker's eight weeks would
		// be read and thrown away.
		in.habits, in.habitsErr = commands.SendReceiveGetErr[common.HabitsResult](core, "habits",
			map[string]string{"days": strconv.Itoa(7)})
	})
	run(func() {
		in.inbox, in.inboxErr = commands.SendReceiveGetErr[common.ReviewList](core, "review/inbox", nil)
	})
	run(func() {
		in.notices, in.noticeErr = commands.SendReceiveGetErr[common.NoticeList](core, "notifications",
			map[string]string{"unread": "t", "limit": "5"})
	})
	wg.Wait()
	return in
}

// watch redraws on the events that change a card, once a minute because time
// itself moves the "now" line and the clock, and once a second while a focus
// session counts down - from the last answer, not a fresh request, so a
// running timer does not cost the server a request a second.
func (self *DashCmd) watch(core *commands.Core) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Print("\033[?25l") // the cursor blinking in the corner is noise
	defer fmt.Print("\033[?25h\n")

	changed := make(chan struct{}, 8)
	go common.Events(ctx, &core.Rest, common.EventOpts{
		Kinds: []string{"reload", "clockin", "clockout", "focusstart", "focusend", "breakstart",
			"breakend", "focusstop", "focusextend", "querymatch"},
	}, func(e common.OrgEvent) bool {
		if e.Kind != "hello" {
			select {
			case changed <- struct{}{}:
			default:
			}
		}
		return true
	})

	d := build(fetch(core), time.Now())
	read := time.Now()
	draw := func() {
		shown := d
		if d.Focus != nil {
			f := *d.Focus
			f.Remaining -= time.Since(read).Seconds()
			if f.Remaining < 0 {
				f.Remaining = 0
			}
			shown.Focus = &f
		}
		if d.Clock != nil {
			c := *d.Clock
			c.Mins = time.Since(c.Started).Minutes()
			shown.Clock = &c
		}
		fmt.Print("\033[H\033[2J" + render(shown, termWidth(), time.Now()))
	}
	draw()

	second := time.NewTicker(time.Second)
	defer second.Stop()
	lastMinute := time.Now().Minute()
	for {
		select {
		case <-ctx.Done():
			return
		case <-changed:
			// A save is often several events: let them land, then read once.
			time.Sleep(150 * time.Millisecond)
			for len(changed) > 0 {
				<-changed
			}
			d, read = build(fetch(core), time.Now()), time.Now()
			draw()
		case now := <-second.C:
			switch {
			case now.Minute() != lastMinute:
				lastMinute = now.Minute()
				d, read = build(fetch(core), time.Now()), time.Now()
				draw()
			case d.Focus != nil:
				draw()
			}
		}
	}
}

// Asked of the terminal first, COLUMNS second (stale after a resize in every
// shell that sets it), as `orgs habits` does.
func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 30 {
		return w
	}
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 30 {
		return n
	}
	return 100
}

func init() {
	commands.AddCmd("dash", "the day on one screen: now, today, overdue, habits, inbox and notices (-w keeps it live)",
		func() commands.Cmd { return &DashCmd{} })
}

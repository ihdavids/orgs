package clocks

// orgs clocks - what is being clocked, now.
//
//	orgs clocks              the running clock, or that there is none
//	orgs clocks -w           redraw it as it changes, for a status line
//	orgs clocks -short       one line: "2h14m Write the thing"
//	orgs clocks -json        for a program to read
//
// `-short -w` is the one that pays for the rest: a tmux status line, a waybar
// module or a starship segment is a command that prints one line and keeps
// printing it. Before /events that meant polling every second; now the line is
// redrawn when the clock changes and once a minute otherwise, because the
// elapsed time moves on its own even when nothing has happened.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type ClockData struct {
	Active bool
	Time   org.OrgDate
	Target common.Target
}

type Clocks struct {
	Watch bool
	Short bool
	Idle  string
}

func (self *Clocks) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Clocks) StartPlugin(manager *common.PluginManager) {
}

func (self *Clocks) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Watch, "w", false, "keep printing it as it changes")
	fset.BoolVar(&self.Short, "short", false, "one line, for a status bar")
	fset.StringVar(&self.Idle, "idle", "",
		"what to print when no clock is running; nothing at all by default in -short")
}

func (self *Clocks) Exec(core *commands.Core) {
	if !self.Watch {
		self.draw(self.read(core))
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	self.draw(self.read(core))

	// Two things move this line. The clock being started or stopped is an
	// event; the elapsed time going up is not an event at all and has to be
	// redrawn on a timer - a status bar that says "2h14m" for an hour is worse
	// than one that says nothing.
	changed := make(chan struct{}, 8)
	go func() {
		common.Events(ctx, &core.Rest, common.EventOpts{
			Kinds: []string{"clockin", "clockout", "reload"},
		}, func(e common.OrgEvent) bool {
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
	for {
		select {
		case <-ctx.Done():
			return
		case <-changed:
		case <-tick.C:
		}
		self.draw(self.read(core))
	}
}

func (self *Clocks) read(core *commands.Core) ClockData {
	var data ClockData
	commands.SendReceiveGet(core, "clock", map[string]string{}, &data)
	return data
}

func (self *Clocks) draw(data ClockData) {
	if commands.RenderOne(data, nil) {
		return
	}
	if !data.Active {
		switch {
		case self.Idle != "":
			fmt.Println(self.Idle)
		case self.Short:
			// A status bar segment with nothing to say says nothing, rather
			// than taking up room to say so.
			fmt.Println()
		default:
			fmt.Println("No active clock")
		}
		return
	}

	elapsed := time.Since(data.Time.Start)
	hours := int(elapsed.Hours())
	mins := int(elapsed.Minutes()) % 60

	if self.Short {
		what := data.Target.Id
		if what == "" {
			what = filepath.Base(data.Target.Filename)
		}
		fmt.Printf("%dh%02dm %s\n", hours, mins, commands.Ellipsis(what, 40))
		return
	}

	fmt.Printf("Active Clock:\n")
	fmt.Printf("  Target:  %s :: %s\n", data.Target.Filename, data.Target.Id)
	fmt.Printf("  Started: %s\n", data.Time.Start.Format("2006-01-02 15:04"))
	fmt.Printf("  Elapsed: %dh %02dm\n", hours, mins)
}

// init function is called at boot
func init() {
	commands.AddCmd("clocks", "show any active running clock",
		func() commands.Cmd {
			return &Clocks{}
		})
}

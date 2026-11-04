package timer

// `orgs timer`: org's relative timer, the server's (orgtimer.go), for taking
// notes stamped with how far into a meeting they were.
//
//	orgs timer                   where it is
//	orgs timer start             from zero
//	orgs timer pause | continue | stop
//	orgs timer countdown 25      a count down; a notification when it ends
//	orgs timer set 0:10:00       as though it started that long ago
//	orgs timer note they agreed  "- 0:03:12 :: they agreed" under the clocked heading
//	orgs timer note -hash H ...  under that heading instead

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Timer struct {
	fset *flag.FlagSet
	hash string
}

type state struct {
	Ok        bool
	Msg       string
	Running   bool
	Paused    bool
	Countdown bool
	Display   string
}

func (self *Timer) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Timer) StartPlugin(manager *common.PluginManager)       {}
func (self *Timer) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.hash, "hash", "", "with note: the heading it goes under (else the clocked one)")
}
func (self *Timer) HelpGroup() string { return "The clock" }

func (self *Timer) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	show := func(s state, err error) {
		if err != nil {
			commands.Fail("%v", err)
		}
		if !s.Ok {
			commands.Fail("%s", s.Msg)
		}
		commands.RenderOne(s, func() {
			switch {
			case !s.Running:
				fmt.Println("the timer is stopped")
			case s.Paused:
				fmt.Printf("%s (paused)\n", s.Display)
			case s.Countdown:
				fmt.Printf("%s left\n", s.Display)
			default:
				fmt.Println(s.Display)
			}
		})
	}
	type op struct {
		Op      string
		Minutes float64 `json:",omitempty"`
		Offset  string  `json:",omitempty"`
	}
	switch verb {
	case "", "status":
		show(commands.SendReceiveGetErr[state](core, "timer", nil))
	case "start", "pause", "continue", "stop":
		show(commands.SendReceivePostErr[op, state](core, "timer", &op{Op: verb}))
	case "countdown":
		if len(words) < 2 {
			commands.Fail("orgs timer countdown MINUTES")
		}
		m, err := strconv.ParseFloat(words[1], 64)
		if err != nil {
			commands.Fail("%q is not a number of minutes", words[1])
		}
		show(commands.SendReceivePostErr[op, state](core, "timer", &op{Op: "countdown", Minutes: m}))
	case "set":
		if len(words) < 2 {
			commands.Fail("orgs timer set H:MM:SS")
		}
		show(commands.SendReceivePostErr[op, state](core, "timer", &op{Op: "set", Offset: words[1]}))
	case "note":
		text := strings.Join(words[1:], " ")
		if text == "" {
			commands.Fail("orgs timer note TEXT")
		}
		res, err := commands.SendReceivePostErr[struct{ Hash, Text string }, common.ResultMsg](core, "timer/item",
			&struct{ Hash, Text string }{self.hash, text})
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		commands.RenderOne(res, func() { fmt.Println(res.Msg) })
	default:
		commands.Fail("orgs timer [start|pause|continue|stop|countdown MIN|set H:MM:SS|note TEXT]")
	}
}

func init() {
	commands.AddCmd("timer", "org's relative timer: start, pause, count down, and notes stamped with it",
		func() commands.Cmd { return &Timer{} })
}

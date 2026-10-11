package focus

// `orgs focus`: the pomodoro timer from the terminal.
//
//	orgs focus                  the session running now, live; a picker when idle
//	orgs focus 'IsStatus("NEXT")'   a query, a hash, or - for a hash on stdin
//	orgs focus -work 50 -break 10   the lengths (remembered by the server)
//	orgs focus status           one line for tmux or a prompt: 🍅 12:31 Write the report
//	orgs focus bar              xbar / SwiftBar plugin output (see barHelp)
//	orgs focus stop | skip | extend [N] | again | today
//
// The timer itself is on the server (internal/app/orgs/focus.go): it clocks in,
// clocks out when the time is up, notes the pomodoro in the logbook and runs
// the break whether or not anything is watching. Everything here is a view of
// that, or one request to it, so worg, this and the menu bar always agree.

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Focus struct {
	Work    float64
	Break   float64
	Long    float64
	Every   int
	Auto    bool
	NoAuto  bool
	Detach  bool
	Bar     bool
	Status  bool
	Days    int
	Idle    string
	Target  commands.TargetFlags
	fset    *flag.FlagSet
}

func (self *Focus) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }

func (self *Focus) StartPlugin(manager *common.PluginManager) {}

func (self *Focus) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.Float64Var(&self.Work, "work", 0, "minutes of work (25, or whatever was used last)")
	fset.Float64Var(&self.Break, "break", 0, "minutes of a short break (5)")
	fset.Float64Var(&self.Long, "long", 0, "minutes of a long break (15)")
	fset.IntVar(&self.Every, "every", 0, "a long break after every N pomodoros (4)")
	fset.BoolVar(&self.Auto, "auto", false, "start the next pomodoro by itself when a break ends")
	fset.BoolVar(&self.NoAuto, "no-auto", false, "stop when a break ends (turns -auto off again)")
	fset.BoolVar(&self.Detach, "d", false, "start and return, rather than staying to show the countdown")
	fset.BoolVar(&self.Bar, "bar", false, "print xbar / SwiftBar menu bar output (same as `orgs focus bar`)")
	fset.BoolVar(&self.Status, "status", false, "one line for a status bar (same as `orgs focus status`)")
	fset.IntVar(&self.Days, "days", 1, "today: how many days of history")
	fset.StringVar(&self.Idle, "idle", "", "status: what to print when nothing is running; nothing by default")
	commands.AddTargetFlags(fset, &self.Target)
}

// The words that are a subcommand rather than the start of a query.
var verbs = map[string]bool{
	"status": true, "bar": true, "stop": true, "skip": true, "extend": true,
	"again": true, "today": true, "watch": true, "start": true,
}

func (self *Focus) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 && verbs[words[0]] {
		verb, words = words[0], words[1:]
	}
	switch {
	case self.Bar:
		verb = "bar"
	case self.Status:
		verb = "status"
	}

	switch verb {
	case "status":
		printStatus(read(core), self.Idle)
	case "bar":
		if len(words) > 0 && words[0] == "install" {
			installBar(core, words[1:])
			return
		}
		printBar(core, read(core))
	case "stop":
		self.answer(post(core, "focus/stop", map[string]any{}))
	case "skip":
		self.answer(post(core, "focus/skip", map[string]any{}))
	case "extend":
		mins := 5.0
		if len(words) > 0 {
			n, err := strconv.ParseFloat(strings.TrimPrefix(words[0], "+"), 64)
			if err != nil || n <= 0 {
				commands.Fail("extend by how many minutes? %q is not a number", words[0])
			}
			mins = n
		}
		self.answer(post(core, "focus/extend", map[string]any{"Minutes": mins}))
	case "today":
		self.today(core)
	case "again":
		self.start(core, "")
	case "watch":
		self.watch(core)
	default:
		// `orgs focus` with nothing named: what is running, or a choice.
		if len(words) == 0 && self.Target.Hash == "" && !commands.StdinIsPipe() {
			st := read(core)
			if st.Phase != "idle" && st.Phase != "" {
				self.watch(core)
				return
			}
		}
		if len(words) == 0 && self.Target.Hash == "" && !commands.Interactive() && !commands.StdinIsPipe() {
			// A script that said nothing: report, never block on a picker.
			printStatus(read(core), self.Idle)
			return
		}
		found := commands.Resolve(core, &self.Target, words, commands.TargetOpts{
			Prompt: "Focus on> ",
			Query:  "IsTask()",
			Quiet:  true,
		})
		self.start(core, found[0].Hash)
	}
}

func (self *Focus) settings(hash string) common.FocusStart {
	req := common.FocusStart{Hash: hash, Work: self.Work, Short: self.Break, Long: self.Long, LongEvery: self.Every}
	if self.Auto {
		t := true
		req.AutoNext = &t
	} else if self.NoAuto {
		f := false
		req.AutoNext = &f
	}
	return req
}

// start begins a pomodoro (on the last task when hash is empty) and, with
// somebody watching, stays to show it.
func (self *Focus) start(core *commands.Core, hash string) {
	req := self.settings(hash)
	st, err := commands.SendReceivePostErr[common.FocusStart, common.FocusState](core, "focus/start", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("could not start: %v", err)
	}
	if !st.Ok {
		commands.Fail("could not start: %s", st.Msg)
	}
	if self.Detach || !commands.Interactive() {
		self.answer(st, nil)
		return
	}
	self.watch(core)
}

// answer prints what a write left behind: the state, as json or as the
// status line.
func (self *Focus) answer(st common.FocusState, err error) {
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !st.Ok {
		commands.Fail("%s", st.Msg)
	}
	commands.RenderOne(st, func() {
		line := StatusLine(st, commands.Colour())
		if line == "" {
			line = "idle"
			if st.Today > 0 {
				line = fmt.Sprintf("idle · %s today", pomodoros(st.Today))
			}
		}
		fmt.Println(line)
	})
}

func (self *Focus) today(core *commands.Core) {
	h, err := commands.SendReceiveGetErr[common.FocusHistory](core, "focus/history",
		map[string]string{"days": strconv.Itoa(self.Days)})
	if err != nil {
		commands.Fail("%v", err)
	}
	if commands.Render(h.Records, nil) {
		return
	}
	c := commands.C
	fmt.Printf("%s%s%s today, %s of focus\n", c(commands.AnsiBold), pomodoros(h.Today), c(commands.AnsiReset), Mins(h.TodayMinutes))
	for _, r := range h.Records {
		mark := c(commands.AnsiRed) + "●" + c(commands.AnsiReset)
		if !r.Completed {
			mark = c(commands.AnsiDim) + "○" + c(commands.AnsiReset)
		}
		when := r.Start
		if t, ok := parseTime(r.Start); ok {
			when = t.Local().Format("Mon 15:04")
			if self.Days <= 1 {
				when = t.Local().Format("15:04")
			}
		}
		fmt.Printf("  %s %s %s%6s%s  %s\n", mark, when, c(commands.AnsiDim), Mins(r.Minutes), c(commands.AnsiReset), r.Title)
	}
}

func read(core *commands.Core) common.FocusState {
	st, err := commands.SendReceiveGetErr[common.FocusState](core, "focus", map[string]string{})
	if err != nil {
		commands.Fail("could not ask the server: %v", err)
	}
	return st
}

func post(core *commands.Core, path string, body map[string]any) (common.FocusState, error) {
	return commands.SendReceivePostErr[map[string]any, common.FocusState](core, path, &body)
}

func init() {
	commands.AddCmd("focus", "pomodoro focus timer: pick a task, clock in, take a break (status, bar, stop, skip, extend, today)",
		func() commands.Cmd {
			return &Focus{}
		})
}

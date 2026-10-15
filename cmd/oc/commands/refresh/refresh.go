package refresh

// orgs refresh - what the server keeps current by itself.
//
//	orgs refresh                    everything on a timer: what, how often, last run, next, how it went
//	orgs refresh run 2              run the second of those now (or its key)
//	orgs refresh run due            whatever is due
//	orgs refresh run all            everything
//	orgs refresh dash.org           that file's dynamic blocks and table formulas, now
//	orgs refresh dash.org -blocks   and its :schedule source blocks
//
// The timer is the server's (internal/app/orgs/refresh.go); this asks it.

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Refresh struct {
	fset   *flag.FlagSet
	Blocks bool
}

func (self *Refresh) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Refresh) StartPlugin(manager *common.PluginManager)         {}
func (self *Refresh) HelpGroup() string                                 { return "Files and code" }

func (self *Refresh) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Blocks, "blocks", false, "with a file: run its :schedule source blocks too")
}

func (self *Refresh) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	switch {
	case len(words) == 0 || words[0] == "ls" || words[0] == "list":
		self.list(core)
	case words[0] == "run":
		if len(words) < 2 {
			commands.Fail("orgs refresh run <number|key|due|all>")
		}
		self.run(core, words[1])
	default:
		self.runReq(core, common.RefreshRun{File: strings.Join(words, " "), Blocks: self.Blocks})
	}
}

func (self *Refresh) items(core *commands.Core) common.RefreshList {
	res, err := commands.SendReceiveGetErr[common.RefreshList](core, "refresh", nil)
	if err != nil {
		commands.Fail("orgs refresh: %v (is the server new enough to have /refresh?)", err)
	}
	return res
}

// ago and in say a time the way a person reads a status line.
func ago(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "never"
	}
	return span(time.Since(t)) + " ago"
}

func in(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	d := time.Until(t)
	if d <= 0 {
		return "due now"
	}
	return "in " + span(d)
}

func span(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func label(it common.RefreshItem) string {
	name := it.Name
	if it.Kind == "file" {
		name = "dynamic blocks and tables"
	}
	if name == "" {
		name = "(unnamed)"
	}
	return name
}

func (self *Refresh) list(core *commands.Core) {
	res := self.items(core)
	commands.Render(res.Items, func() {
		g, rd, d, gold, r := commands.C(commands.AnsiGreen), commands.C(commands.AnsiRed), commands.C(commands.AnsiDim),
			commands.C(commands.AnsiGold), commands.C(commands.AnsiReset)
		if len(res.Items) == 0 {
			fmt.Println("Nothing is on a timer. Give a source block :schedule 1h, a dynamic block :refresh 1h,")
			fmt.Println("or a file #+REFRESH: 1h.")
		}
		for i, it := range res.Items {
			status := d + "not run yet" + r
			switch {
			case it.Blocked != "":
				status = gold + it.Blocked + r
			case it.Last != "" && it.Ok:
				status = g + "ok" + r + " " + d + it.Msg + r
			case it.Last != "":
				status = rd + "failed" + r + " " + it.Msg
			}
			when := ago(it.Last)
			if it.Next != "" {
				when += ", " + in(it.Next)
			}
			fmt.Printf("%2d  %-6s %s%s:%d%s  %s  %severy %s%s  %s\n     %s\n", i+1, it.Kind,
				d, commands.BaseName(it.File), it.Line+1, r, label(it), d, it.Every, r, when, status)
		}
		if res.OnExport {
			fmt.Printf("%sexports read dynamic blocks and formulas refreshed (refresh.onExport)%s\n", d, r)
		}
	})
}

func (self *Refresh) run(core *commands.Core, what string) {
	switch what {
	case "all":
		self.runReq(core, common.RefreshRun{All: true})
		return
	case "due":
		self.runReq(core, common.RefreshRun{Due: true})
		return
	}
	key := what
	if n, err := strconv.Atoi(what); err == nil {
		items := self.items(core).Items
		if n < 1 || n > len(items) {
			commands.Fail("there are %d things on a timer; %d is not one of them", len(items), n)
		}
		key = items[n-1].Key
	}
	self.runReq(core, common.RefreshRun{Key: key})
}

func (self *Refresh) runReq(core *commands.Core, req common.RefreshRun) {
	res, err := commands.SendReceivePostErr[common.RefreshRun, common.RefreshRan](core, "refresh/run", &req)
	if err != nil {
		if err == commands.ErrDryRun {
			return
		}
		commands.Fail("orgs refresh: %v", err)
	}
	if commands.RenderOne(res, nil) {
		return
	}
	g, rd, d, r := commands.C(commands.AnsiGreen), commands.C(commands.AnsiRed), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	for _, it := range res.Items {
		mark := g + "✓" + r
		if !it.Ok {
			mark = rd + "✗" + r
		}
		changed := ""
		if it.Changed {
			changed = g + " (file updated)" + r
		}
		fmt.Printf("%s %s %s%s:%d%s %s: %s%s %s%dms%s\n", mark, it.Kind, d, commands.BaseName(it.File), it.Line+1, r,
			label(it), it.Msg, changed, d, it.Millis, r)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	if len(res.Items) == 0 {
		fmt.Println(res.Msg)
	}
}

func init() {
	commands.AddCmd("refresh", "what the server keeps current: :schedule source blocks, dynamic blocks, table formulas",
		func() commands.Cmd { return &Refresh{} })
}

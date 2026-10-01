package help

// orgs help - what the commands are, and what one of them takes.
//
//	orgs help              every command, grouped
//	orgs help todo         what `orgs todo` takes
//	orgs help -all         every command with every flag
//
// There was no help command. `orgs` with nothing printed the registry, in *map
// order* - a different order every run, which is unreadable for a list of forty
// items and makes "did that command exist last week" impossible to answer by
// eye. And there was no way at all to ask what one command took without running
// it wrong on purpose and reading the error.
//
// The grouping is a table here rather than a field on the registration, so
// adding a command is still one line in one place. A command missing from the
// table is not lost - it lands under "Other", which is also the reminder to put
// it somewhere.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Help struct {
	All bool
}

func (self *Help) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Help) StartPlugin(m *common.PluginManager)       {}
func (self *Help) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.All, "all", false, "every command with all of its flags")
}

// NeedsNoServer: the registry is in this binary.
func (self *Help) NeedsNoServer() bool { return true }

// The groups, in the order they are worth reading. A command not named here
// lands in "Other" rather than vanishing.
var groups = []struct {
	Name string
	Cmds []string
}{
	{"Finding things", []string{"search", "q", "find", "grep", "agenda", "tui", "outline",
		"projects", "filter", "filters", "tags", "taggroups", "show", "log"}},
	{"Changing a heading", []string{"todo", "sched", "deadline", "tag", "prop", "rename",
		"note", "check", "archive", "rm", "ref", "cap", "listcap", "new", "daypage"}},
	{"The clock", []string{"clockin", "clockout", "clocks", "clocktable"}},
	{"Habits", []string{"habits"}},
	{"Files and code", []string{"files", "fmt", "tangle", "code", "tables", "export", "links"}},
	{"Records", []string{"rec", "record", "contact"}},
	{"Watching", []string{"watch"}},
	{"The server", []string{"serve", "login", "user", "adduser", "initconfig", "doctor", "completion", "help"}},
	{"Dungeons & Dragons", []string{"dnd"}},
	{"For other programs", []string{"mcp"}},
}

func (self *Help) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("help").Flags)
	if len(words) > 0 {
		self.one(words[0])
		return
	}
	if self.All {
		for _, name := range sortedNames() {
			self.one(name)
			fmt.Println()
		}
		return
	}
	self.list()
}

func sortedNames() []string {
	out := []string{}
	for name := range commands.CmdRegistry {
		if strings.HasPrefix(name, "__") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (self *Help) list() {
	// -json is a table of every command, which is what a wrapper or a
	// documentation build wants and what `orgs` printing prose never gave it.
	type row struct {
		Name  string
		Group string
		Usage string
	}
	placed := map[string]bool{}
	rows := []row{}
	for _, g := range groups {
		for _, name := range g.Cmds {
			if c := commands.Find(name); c != nil {
				placed[name] = true
				rows = append(rows, row{Name: name, Group: g.Name, Usage: c.Usage})
			}
		}
	}
	// Anything the table did not name goes under the group it asks for, or
	// under "Other". A command registered at runtime - every filter in the
	// yaml is one - can only be grouped by asking it.
	extra := map[string][]string{}
	extraOrder := []string{}
	for _, name := range sortedNames() {
		if placed[name] {
			continue
		}
		c := commands.Find(name)
		group := "Other"
		if g, ok := c.Cmd.(commands.Grouped); ok && g.HelpGroup() != "" {
			group = g.HelpGroup()
		}
		if _, seen := extra[group]; !seen {
			extraOrder = append(extraOrder, group)
		}
		extra[group] = append(extra[group], name)
		rows = append(rows, row{Name: name, Group: group, Usage: c.Usage})
	}
	if commands.Render(rows, nil) {
		return
	}

	fmt.Printf("%sorgs%s - an org mode server and the command line that talks to it\n\n",
		commands.C(commands.AnsiBold), commands.C(commands.AnsiReset))
	fmt.Printf("  orgs [-url URL] [-config FILE] [-local] <command> [flags]\n\n")

	width := 0
	for _, name := range sortedNames() {
		if len(name) > width {
			width = len(name)
		}
	}
	show := func(name string) {
		c := commands.Find(name)
		if c == nil {
			return
		}
		fmt.Printf("  %s%-*s%s  %s\n", commands.C(commands.AnsiBold), width, name,
			commands.C(commands.AnsiReset), c.Usage)
	}
	for _, g := range groups {
		any := false
		for _, name := range g.Cmds {
			if commands.Find(name) != nil {
				any = true
				break
			}
		}
		if !any {
			continue
		}
		fmt.Printf("%s%s%s\n", commands.C(commands.AnsiGold), g.Name, commands.C(commands.AnsiReset))
		for _, name := range g.Cmds {
			show(name)
		}
		fmt.Println()
	}
	sort.Strings(extraOrder)
	for _, group := range extraOrder {
		fmt.Printf("%s%s%s\n", commands.C(commands.AnsiGold), group,
			commands.C(commands.AnsiReset))
		for _, name := range extra[group] {
			show(name)
		}
		fmt.Println()
	}
	fmt.Printf("%sorgs help <command>%s for what one of them takes.\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	fmt.Printf("%sorgs completion zsh%s for tab completion.\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
}

func (self *Help) one(name string) {
	c := commands.Find(name)
	if c == nil {
		near := []string{}
		for _, n := range sortedNames() {
			if strings.Contains(n, name) || strings.Contains(name, n) {
				near = append(near, n)
			}
		}
		if len(near) > 0 {
			commands.Fail("no command called %q - did you mean %s?", name, strings.Join(near, ", "))
		}
		commands.Fail("no command called %q - orgs help for the list", name)
	}

	fmt.Printf("%sorgs %s%s  %s\n", commands.C(commands.AnsiBold), name,
		commands.C(commands.AnsiReset), c.Usage)
	if c.Flags == nil {
		return
	}
	// The flag package prints to its own output; point it at stdout so the
	// whole of the help is one stream somebody can pipe to a pager.
	c.Flags.SetOutput(os.Stdout)
	shown := false
	c.Flags.VisitAll(func(f *flag.Flag) {
		if !shown {
			fmt.Println()
			shown = true
		}
		def := ""
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			def = fmt.Sprintf(" %s(%s)%s", commands.C(commands.AnsiDim), f.DefValue,
				commands.C(commands.AnsiReset))
		}
		fmt.Printf("  %s-%-14s%s %s%s\n", commands.C(commands.AnsiCyan), f.Name,
			commands.C(commands.AnsiReset), f.Usage, def)
	})
}

func init() {
	commands.AddCmd("help", "what the commands are, and what one of them takes",
		func() commands.Cmd { return &Help{} })
}

package help

// orgs help - what the commands are, and what one of them takes.
//
//	orgs help              every command, grouped
//	orgs help todo         what `orgs todo` takes
//	orgs help -all         every command with every flag
//	orgs help -man search  search's man page (roff); with no command, orgs(1)
//	orgs help -man-dir ~/.local/share/man/man1   every page, for man to find
//
// Beside a command's flags it shows what the documentation says about it -
// the guides' =orgs NAME= headings, the SDOC Commands sections, the usage
// lines the command's package opens with - from docs.json, carried in the
// binary (docs.go). A command that takes a query also gets the query
// language, with every function the binary's query map has.
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
	All    bool
	Man    bool
	ManDir string
}

func (self *Help) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Help) StartPlugin(m *common.PluginManager)       {}
func (self *Help) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.All, "all", false, "every command with all of its flags")
	fset.BoolVar(&self.Man, "man", false, "print a man page (roff): orgs(1), or the command's")
	fset.StringVar(&self.ManDir, "man-dir", "", "write orgs.1 and an orgs-<command>.1 for every command into this folder")
}

// NeedsNoServer: the registry is in this binary.
func (self *Help) NeedsNoServer() bool { return true }

// The groups, in the order they are worth reading. A command not named here
// lands in "Other" rather than vanishing.
var groups = []struct {
	Name string
	Cmds []string
}{
	{"Finding things", []string{"dash", "p", "search", "q", "find", "grep", "agenda", "tui", "outline",
		"projects", "gantt", "kanban", "cols", "filter", "filters", "tags", "taggroups", "show", "log"}},
	{"Changing a heading", []string{"todo", "sched", "deadline", "tag", "prop", "rename",
		"note", "check", "archive", "rm", "ref", "cap", "listcap", "new", "daypage", "paste"}},
	{"The clock", []string{"clockin", "clockout", "clocks", "clocktable", "timesheet", "invoice"}},
	{"Reports", []string{"standup", "recap"}},
	{"Habits", []string{"habits"}},
	{"Writing", []string{"write", "lint", "ask", "models"}},
	{"Files and code", []string{"files", "fmt", "dblock", "tangle", "detangle", "code", "tables", "export", "site", "import", "csv", "refresh", "links", "go"}},
	{"Records", []string{"rec", "record", "contact"}},
	{"Watching", []string{"watch"}},
	{"The server", []string{"serve", "login", "worg", "cert", "user", "adduser", "initconfig", "doctor", "completion", "help"}},
	{"Dungeons & Dragons", []string{"dnd"}},
	{"For other programs", []string{"mcp"}},
}

func (self *Help) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("help").Flags)
	if self.ManDir != "" {
		writeManDir(self.ManDir)
		return
	}
	if self.Man {
		if len(words) > 0 {
			fmt.Print(manPage(mustFind(words[0])))
		} else {
			fmt.Print(manIndex())
		}
		return
	}
	if len(words) > 0 {
		self.one(words[0], true)
		return
	}
	if self.All {
		for _, name := range sortedNames() {
			self.one(name, false)
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

type group struct {
	Name  string
	Names []string
}

// helpGroups is every command under the heading it is listed under: the
// table's groups in their order, then the groups commands ask for themselves
// (or "Other"), alphabetically. rows is the same in -json's order.
func helpGroups() (out []group, rows []helpRow) {
	placed := map[string]bool{}
	for _, g := range groups {
		names := []string{}
		for _, name := range g.Cmds {
			if c := commands.Find(name); c != nil {
				placed[name] = true
				names = append(names, name)
				rows = append(rows, helpRow{Name: name, Group: g.Name, Usage: c.Usage})
			}
		}
		if len(names) > 0 {
			out = append(out, group{g.Name, names})
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
		g := "Other"
		if gr, ok := c.Cmd.(commands.Grouped); ok && gr.HelpGroup() != "" {
			g = gr.HelpGroup()
		}
		if _, seen := extra[g]; !seen {
			extraOrder = append(extraOrder, g)
		}
		extra[g] = append(extra[g], name)
		rows = append(rows, helpRow{Name: name, Group: g, Usage: c.Usage})
	}
	sort.Strings(extraOrder)
	for _, g := range extraOrder {
		out = append(out, group{g, extra[g]})
	}
	return out, rows
}

// -json is a table of every command, which is what a wrapper or a
// documentation build wants and what `orgs` printing prose never gave it.
type helpRow struct {
	Name  string
	Group string
	Usage string
}

func (self *Help) list() {
	grouped, rows := helpGroups()
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
	for _, g := range grouped {
		fmt.Printf("%s%s%s\n", commands.C(commands.AnsiGold), g.Name, commands.C(commands.AnsiReset))
		for _, name := range g.Names {
			fmt.Printf("  %s%-*s%s  %s\n", commands.C(commands.AnsiBold), width, name,
				commands.C(commands.AnsiReset), commands.Find(name).Usage)
		}
		fmt.Println()
	}
	fmt.Printf("%sorgs help <command>%s for what one of them takes.\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	fmt.Printf("%sorgs completion zsh%s for tab completion.\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
}

// mustFind is the command, or the reason there is none (with the names it
// may have meant) and an exit.
func mustFind(name string) *commands.CmdCreatorThunk {
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
	return c
}

// one is a command's usage and flags, and with docs what is written about it.
func (self *Help) one(name string, docs bool) {
	c := mustFind(name)
	fmt.Printf("%sorgs %s%s  %s\n", commands.C(commands.AnsiBold), name,
		commands.C(commands.AnsiReset), c.Usage)
	if docs {
		defer showDocs(c)
	}
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

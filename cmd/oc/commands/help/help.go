package help

// orgs help - what the commands are, and what one of them takes.
//
//	orgs help              every command, grouped
//	orgs help -full        with the org packages and your filters described too
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
	Full   bool
	Man    bool
	ManDir string
}

func (self *Help) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Help) StartPlugin(m *common.PluginManager)       {}
func (self *Help) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.All, "all", false, "every command with all of its flags")
	fset.BoolVar(&self.Full, "full", false, "describe every command, the org packages and your filters too")
	fset.BoolVar(&self.Man, "man", false, "print a man page (roff): orgs(1), or the command's")
	fset.StringVar(&self.ManDir, "man-dir", "", "write orgs.1 and an orgs-<command>.1 for every command into this folder")
}

// NeedsNoServer: the registry is in this binary.
func (self *Help) NeedsNoServer() bool { return true }

// The groups, in the order they are worth reading. A command not named here
// goes under the group it asks for (commands.Grouped) - into that group if the
// table has one, so a heading is never printed twice - or "Other".
var groups = []struct {
	Name string
	Cmds []string
	// Compact groups are names only in the plain listing: they are long, and
	// each of their commands is one word about an org package or a saved
	// filter, read by its name. -full prints them out.
	Compact bool
}{
	{"Finding things", []string{"dash", "p", "search", "q", "find", "grep", "agenda", "tui", "outline",
		"projects", "gantt", "kanban", "cols", "filter", "filters", "tags", "taggroups", "show", "log",
		"backlinks", "mentions", "stats"}, false},
	{"Changing a heading", []string{"todo", "sched", "deadline", "tag", "prop", "rename",
		"note", "check", "list", "tb", "archive", "rm", "ref", "cap", "listcap", "new", "daypage", "paste",
		"voice", "gocap"}, false},
	{"Refactoring the outline", []string{"retag", "split", "merge", "list2headings"}, false},
	{"Planning", []string{"plan", "review", "triage", "focus"}, false},
	{"The clock", []string{"clockin", "clockout", "clocks", "clocktable", "timesheet", "invoice", "timer"}, false},
	{"Reports", []string{"standup", "recap"}, false},
	{"Habits", []string{"habits"}, false},
	{"Writing", []string{"write", "lint", "ask", "models"}, false},
	{"Files and code", []string{"files", "fmt", "dblock", "tangle", "detangle", "code", "tables", "export",
		"site", "import", "csv", "refresh", "links", "go", "transclude", "diff", "blame"}, false},
	{"Records", []string{"rec", "record", "contact"}, false},
	{"Watching and notifications", []string{"watch", "notifications", "notify", "hooks"}, false},
	{"The server", []string{"serve", "login", "worg", "cert", "user", "adduser", "initconfig", "reloadconfig",
		"dav", "doctor", "version", "completion", "help"}, false},
	{"Dungeons & Dragons", []string{"dnd"}, false},
	{"Editors and other programs", []string{"lsp", "mcp"}, false},
	{"Org packages", nil, true},
	{"Your filters (each is a command)", nil, true},
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
	Name    string
	Names   []string
	Compact bool
}

// helpGroups is every command under the heading it is listed under: the
// table's groups in their order, each with the commands that ask for it, then
// groups only commands ask for (and "Other") alphabetically. rows is the same
// in -json's order.
func helpGroups() (out []group, rows []helpRow) {
	placed := map[string]string{}
	for _, g := range groups {
		for _, name := range g.Cmds {
			if commands.Find(name) != nil {
				placed[name] = g.Name
			}
		}
	}
	// A command registered at runtime - every filter in the yaml is one - can
	// only be grouped by asking it.
	asked := map[string][]string{}
	for _, name := range sortedNames() {
		if _, ok := placed[name]; ok {
			continue
		}
		g := "Other"
		if gr, ok := commands.Find(name).Cmd.(commands.Grouped); ok && gr.HelpGroup() != "" {
			g = gr.HelpGroup()
		}
		asked[g] = append(asked[g], name)
	}
	add := func(g group) {
		if len(g.Names) == 0 {
			return
		}
		out = append(out, g)
		for _, name := range g.Names {
			rows = append(rows, helpRow{Name: name, Group: g.Name, Usage: commands.Find(name).Usage})
		}
	}
	known := map[string]bool{}
	for _, g := range groups {
		known[g.Name] = true
		names := []string{}
		for _, name := range g.Cmds {
			if placed[name] == g.Name {
				names = append(names, name)
			}
		}
		add(group{g.Name, append(names, asked[g.Name]...), g.Compact})
	}
	rest := []string{}
	for g := range asked {
		if !known[g] {
			rest = append(rest, g)
		}
	}
	sort.Slice(rest, func(a, b int) bool {
		// Other last: it is the list of things nobody has placed yet.
		if (rest[a] == "Other") != (rest[b] == "Other") {
			return rest[b] == "Other"
		}
		return rest[a] < rest[b]
	})
	for _, g := range rest {
		add(group{g, asked[g], false})
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

	// Names that do the same thing (list and tb, rec and record) are one line.
	lines := func(names []string) (out [][]string) {
		at := map[string]int{}
		for _, name := range names {
			u := commands.Find(name).Usage
			if i, ok := at[u]; ok && u != "" {
				out[i] = append(out[i], name)
				continue
			}
			at[u] = len(out)
			out = append(out, []string{name})
		}
		return out
	}
	width := 0
	for _, g := range grouped {
		if g.Compact && !self.Full {
			continue
		}
		for _, l := range lines(g.Names) {
			if w := len(strings.Join(l, ", ")); w > width {
				width = w
			}
		}
	}
	compacted := false
	for _, g := range grouped {
		fmt.Printf("%s%s%s\n", commands.C(commands.AnsiGold), g.Name, commands.C(commands.AnsiReset))
		if g.Compact && !self.Full {
			compacted = true
			fmt.Println(wrapNames(g.Names, 2, 78))
			fmt.Println()
			continue
		}
		for _, l := range lines(g.Names) {
			fmt.Printf("  %s%-*s%s  %s\n", commands.C(commands.AnsiBold), width, strings.Join(l, ", "),
				commands.C(commands.AnsiReset), commands.Find(l[0]).Usage)
		}
		fmt.Println()
	}
	fmt.Printf("%sorgs help <command>%s for what one of them takes", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	if compacted {
		fmt.Printf("%s; orgs help -full to describe every command here%s", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
	}
	fmt.Println(".")
	fmt.Printf("%sorgs completion zsh%s for tab completion.\n",
		commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
}

// wrapNames lays names out as running text, indented, broken before width.
func wrapNames(names []string, indent, width int) string {
	var b strings.Builder
	col := 0
	for i, name := range names {
		word := name
		if i < len(names)-1 {
			word += ","
		}
		if col > 0 && col+1+len(word) > width {
			b.WriteString("\n")
			col = 0
		}
		if col == 0 {
			b.WriteString(strings.Repeat(" ", indent))
			col = indent
		} else {
			b.WriteString(" ")
			col++
		}
		b.WriteString(word)
		col += len(word)
	}
	return b.String()
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

package savedq

// orgs q - the queries you keep.
//
//	orgs q                      pick one and run it
//	orgs q stale                run the one called stale
//	orgs q ls                   what is saved
//	orgs q save stale 'IsStatus("NEXT") && OlderThan("30d")'
//	orgs q rm stale
//	orgs q edit stale           change it in $EDITOR
//
// These are the same stored queries worg's search tab keeps, over the same
// per-user endpoints - which is the point of the command rather than an
// implementation detail. A query worth keeping is worth keeping once: named in
// the terminal, it turns up in the browser's palette, and named in the browser
// it is `orgs q <name>` here.
//
// Running one is `orgs search` with the text looked up first, so everything
// `search` grew - -sort, -group, -limit, -open, -json, -format - works on a
// saved query without this command knowing any of it.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Q struct {
	Show  bool
	Limit int
	Sort  string
}

// StoredQuery is the wire shape of one. Restated rather than imported, for the
// usual reason: extensions.go is in internal/app/orgs.
type StoredQuery struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

func (self *Q) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Q) StartPlugin(m *common.PluginManager)       {}

func (self *Q) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Show, "show", false, "print the query rather than running it")
	fset.IntVar(&self.Limit, "limit", 0, "at most this many results")
	fset.StringVar(&self.Sort, "sort", "", "order the results: date, deadline, priority, file, status")
}

func (self *Q) Exec(core *commands.Core) {
	// FreeArgs consumes the arguments as it parses them, so the subcommand has
	// to be read off the slice it returns rather than asked of the flag set
	// afterwards - which silently turned `orgs links preview` into a search for
	// the word "preview" when this was got wrong there.
	words := commands.FreeArgs(commands.Find("q").Flags)
	sub := ""
	if len(words) > 0 {
		sub = words[0]
	}

	switch sub {
	case "ls", "list":
		self.list(core)
	case "save", "set", "add":
		self.save(core, words[1:])
	case "rm", "delete", "del":
		self.remove(core, words[1:])
	case "edit":
		self.edit(core, words[1:])
	case "":
		self.pick(core)
	default:
		self.run(core, sub)
	}
}

func (self *Q) all(core *commands.Core) []StoredQuery {
	qs := commands.SendReceiveGetOr[[]StoredQuery](core, "ext/queries", nil)
	sort.Slice(qs, func(i, j int) bool {
		return strings.ToLower(qs[i].Name) < strings.ToLower(qs[j].Name)
	})
	return qs
}

func (self *Q) list(core *commands.Core) {
	qs := self.all(core)
	if len(qs) == 0 {
		commands.Fail("nothing saved yet.\n\n%s", usage)
	}
	width := 0
	for _, q := range qs {
		if len(q.Name) > width {
			width = len(q.Name)
		}
	}
	commands.Render(qs, func() {
		for _, q := range qs {
			fmt.Printf("%s%-*s%s  %s%s%s\n", commands.C(commands.AnsiBold), width, q.Name,
				commands.C(commands.AnsiReset), commands.C(commands.AnsiDim), q.Query,
				commands.C(commands.AnsiReset))
		}
	})
}

func (self *Q) save(core *commands.Core, words []string) {
	if len(words) < 2 {
		commands.Fail("orgs q save <name> '<query>'")
	}
	name := words[0]
	// The query is the rest joined back up: somebody who did not quote it has
	// written the same thing, and refusing them on a space would be pedantry.
	query := strings.Join(words[1:], " ")

	var reply common.ResultMsg
	commands.SendReceivePost(core, "ext/query", &StoredQuery{Name: name, Query: query}, &reply)
	if commands.DryRun {
		return
	}
	if !reply.Ok {
		commands.Fail("could not save %s: %s", name, reply.Msg)
	}
	fmt.Printf("%s✓%s %s%s%s  %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
		commands.C(commands.AnsiBold), name, commands.C(commands.AnsiReset), query)
}

func (self *Q) remove(core *commands.Core, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs q rm <name>")
	}
	for _, name := range words {
		reply, err := commands.SendReceiveDelete[common.ResultMsg](core, "ext/query",
			map[string]string{"name": name})
		if err == commands.ErrDryRun {
			continue
		}
		if err != nil || !reply.Ok {
			commands.Fail("could not remove %s: %v %s", name, err, reply.Msg)
		}
		fmt.Printf("%s✓%s removed %s\n", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset), name)
	}
}

// edit opens the query in $EDITOR. A query is one line of a small language and
// editing it in place beats retyping it to change one clause - which is what
// `save` over the top of an existing name would otherwise mean.
func (self *Q) edit(core *commands.Core, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs q edit <name>")
	}
	name := words[0]
	current := ""
	for _, q := range self.all(core) {
		if q.Name == name {
			current = q.Query
		}
	}
	if current == "" {
		commands.Fail("no query called %s", name)
	}
	if !commands.Interactive() {
		commands.Fail("orgs q edit needs a terminal - use orgs q save %s '<query>'", name)
	}

	f, err := os.CreateTemp("", "orgs-query-*.txt")
	if err != nil {
		commands.Fail("%v", err)
	}
	defer os.Remove(f.Name())
	fmt.Fprintf(f, "%s\n", current)
	f.Close()

	// $EDITOR rather than core.LaunchEditor: that one is configured for opening
	// an org file at a line and starts the editor *without waiting*, which is
	// right for "go and look at this" and useless when the answer has to be
	// read back afterwards.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		commands.Fail("editor: %v", err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		commands.Fail("%v", err)
	}
	edited := strings.TrimSpace(string(b))
	if edited == "" {
		commands.Fail("nothing left in the file - %s is unchanged", name)
	}
	if edited == current {
		fmt.Printf("%sunchanged%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		return
	}
	self.save(core, append([]string{name}, edited))
}

func (self *Q) pick(core *commands.Core) {
	qs := self.all(core)
	if len(qs) == 0 {
		commands.Fail("nothing saved yet.\n\n%s", usage)
	}
	if !commands.Interactive() {
		self.list(core)
		return
	}
	width := 0
	for _, q := range qs {
		if len(q.Name) > width {
			width = len(q.Name)
		}
	}
	lines := []string{}
	for _, q := range qs {
		lines = append(lines, commands.PickLine([]string{q.Name},
			fmt.Sprintf("%-*s  %s%s%s", width, q.Name, commands.C(commands.AnsiDim), q.Query,
				commands.C(commands.AnsiReset))))
	}
	// The pane is the query's own results, which is the only thing that tells
	// two similarly named queries apart.
	preview := ""
	if self_, err := commands.SelfCommand(core); err == nil {
		preview = self_ + " q {1} -no-color"
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Query> ",
		Preview:       preview,
	})
	if len(sel) == 0 {
		return
	}
	addr, _ := commands.Address(sel[0], 1)
	self.run(core, addr[0])
}

// run looks the name up and asks /search, printing what `orgs search` prints.
func (self *Q) run(core *commands.Core, name string) {
	query := ""
	for _, q := range self.all(core) {
		if q.Name == name {
			query = q.Query
		}
	}
	if query == "" {
		known := []string{}
		for _, q := range self.all(core) {
			known = append(known, q.Name)
		}
		if len(known) == 0 {
			commands.Fail("no query called %s, and nothing is saved yet", name)
		}
		commands.Fail("no query called %s - there is %s", name, strings.Join(known, ", "))
	}
	if self.Show {
		commands.RenderOne(StoredQuery{Name: name, Query: query}, func() { fmt.Println(query) })
		return
	}

	todos := commands.SendReceiveGetOr[common.Todos](core, "search",
		map[string]string{"query": query})
	if self.Sort != "" {
		sortTodos(todos, self.Sort)
	}
	if self.Limit > 0 && len(todos) > self.Limit {
		todos = todos[:self.Limit]
	}
	if len(todos) == 0 {
		if !commands.Machine() {
			fmt.Fprintf(os.Stderr, "%snothing matched %s%s\n", commands.C(commands.AnsiDim),
				name, commands.C(commands.AnsiReset))
		}
		commands.Render(todos, nil)
		return
	}
	commands.Render(todos, func() {
		for _, t := range todos {
			kw := ""
			if t.Status != "" {
				kw = commands.C(commands.AnsiGold) + t.Status + commands.C(commands.AnsiReset) + " "
			}
			tags := ""
			if len(t.Tags) > 0 {
				tags = " " + commands.C(commands.AnsiCyan) + ":" + strings.Join(t.Tags, ":") + ":" +
					commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s%s%s %s%s\n", commands.C(commands.AnsiDim),
				fmt.Sprintf("%s:%d", commands.BaseName(t.Filename), t.LineNum),
				commands.C(commands.AnsiReset), kw+t.Headline, tags)
		}
	})
}

func sortTodos(todos common.Todos, by string) {
	switch strings.ToLower(by) {
	case "date", "scheduled":
		sort.SliceStable(todos, func(i, j int) bool {
			a, b := todos[i].Date, todos[j].Date
			if a == nil || b == nil {
				return b != nil
			}
			return a.Start.Before(b.Start)
		})
	case "deadline":
		sort.SliceStable(todos, func(i, j int) bool {
			a, b := todos[i].Deadline, todos[j].Deadline
			if a == nil || b == nil {
				return b != nil
			}
			return a.Start.Before(b.Start)
		})
	case "priority":
		sort.SliceStable(todos, func(i, j int) bool {
			// No priority sorts after every priority, because "[#C]" is still
			// more urgent than a heading nobody rated.
			a, b := todos[i].Priority, todos[j].Priority
			if a == "" || b == "" {
				return b == ""
			}
			return a < b
		})
	case "file":
		sort.SliceStable(todos, func(i, j int) bool {
			if todos[i].Filename == todos[j].Filename {
				return todos[i].LineNum < todos[j].LineNum
			}
			return todos[i].Filename < todos[j].Filename
		})
	case "status":
		sort.SliceStable(todos, func(i, j int) bool { return todos[i].Status < todos[j].Status })
	default:
		fmt.Fprintf(os.Stderr, "orgs q: no sort called %q, leaving the order alone\n", by)
	}
}

const usage = `  orgs q                           pick one and run it
  orgs q stale                     run the one called stale
  orgs q ls                        what is saved
  orgs q save stale 'IsStatus("NEXT")'
  orgs q edit stale                change it in $EDITOR
  orgs q rm stale

  These are the same saved queries worg's search tab keeps.`

func init() {
	commands.AddCmd("q", "the queries you keep - the same ones worg saves",
		func() commands.Cmd { return &Q{} })
}

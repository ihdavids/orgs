package search

// The query language, from the terminal.
//
//	orgs search 'IsTask() && IsStatus("NEXT")'
//	orgs search '{{ WorkTasks }}' -sort date
//	orgs search 'IsTask()' -json | jq '.[].Headline'
//	orgs search 'IsTask()' -format '{{.Filename}}:{{.LineNum}}\t{{.Headline}}'
//	orgs search 'IsProject()' -open
//
// This is /search, which queries the *parsed* database and understands
// headings, keywords, tags, properties and dates. `orgs find` is the other
// one - a regular expression over the text of every file, which understands
// nothing and is what finds a phrase written in a drawer or a table.
//
// The expression is the server's own, so every filter and tag group in the
// yaml is available through the same {{ Handlebars }} substitution the
// exporters use; the server expands them before parsing.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type Search struct {
	fset *flag.FlagSet

	Query string
	Sort  string
	Limit int
	Count bool
	Open  bool
	Group bool
}

func (self *Search) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Search) StartPlugin(manager *common.PluginManager)         {}

func (self *Search) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Query, "q", "", "the query, when you would rather not put it first")
	fset.StringVar(&self.Sort, "sort", "", "file, date, priority, status or headline")
	fset.IntVar(&self.Limit, "limit", 0, "keep only the first n results")
	fset.BoolVar(&self.Count, "count", false, "how many matched, and nothing else")
	fset.BoolVar(&self.Open, "open", false, "pick one and open it in your editor")
	fset.BoolVar(&self.Group, "group", false, "gather the results under a band per file")
}

func (self *Search) Exec(core *commands.Core) {
	// Everything that is not a flag is the query, wherever the flags were
	// written: `orgs search 'IsTask()' -json` and `orgs search -json
	// 'IsTask()'` have to mean the same thing, and Go's flag package stops at
	// the first word unless somebody makes it carry on.
	query := self.Query
	if free := commands.FreeText(self.fset); free != "" {
		query = free
	}
	if strings.TrimSpace(query) == "" {
		commands.Fail("orgs search: no query.\n\n%s", usage)
	}

	todos, err := commands.SendReceiveGetErr[common.Todos](core, "search",
		map[string]string{"query": query})
	if err != nil {
		commands.Fail("orgs search: %v", err)
	}
	rows := []common.Todo(todos)
	sortTodos(rows, self.Sort)
	if self.Limit > 0 && len(rows) > self.Limit {
		rows = rows[:self.Limit]
	}

	if self.Count {
		commands.RenderOne(struct{ Count int }{len(rows)}, func() {
			fmt.Printf("%d\n", len(rows))
		})
		return
	}
	if self.Open {
		self.open(core, rows)
		return
	}

	commands.Render(rows, func() { self.print(rows) })
}

func (self *Search) print(rows []common.Todo) {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "nothing matched")
		return
	}
	if !self.Group {
		for _, t := range rows {
			fmt.Println(commands.TodoLine(t, 0))
		}
		return
	}
	// Grouping is a sort plus a band, the way the search tab does it, so the
	// rows are the same rows read in a different order rather than a second
	// way of printing one.
	byFile := map[string][]common.Todo{}
	order := []string{}
	for _, t := range rows {
		if _, seen := byFile[t.Filename]; !seen {
			order = append(order, t.Filename)
		}
		byFile[t.Filename] = append(byFile[t.Filename], t)
	}
	for i, f := range order {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s%s%s %s(%d)%s\n",
			commands.C(commands.AnsiBold), f, commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), len(byFile[f]), commands.C(commands.AnsiReset))
		for _, t := range byFile[f] {
			fmt.Printf("  %s\n", commands.TodoLine(t, 0))
		}
	}
}

func (self *Search) open(core *commands.Core, rows []common.Todo) {
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "nothing matched")
		return
	}
	f, err := fzf.New(fzf.WithNoLimit(true))
	if err != nil {
		commands.Fail("%v", err)
	}
	idxs, err := f.Find(rows, func(i int) string { return commands.TodoLine(rows[i], 0) })
	if err != nil {
		return
	}
	for _, i := range idxs {
		core.LaunchEditor(rows[i].Filename, rows[i].LineNum)
	}
}

// sortTodos puts the rows in the order asked for. The default is the order the
// server walked the files in, which is stable and means nothing - fine for a
// pipe, and worth being able to change for a person.
func sortTodos(rows []common.Todo, by string) {
	key := func(t common.Todo) string { return "" }
	switch strings.ToLower(strings.TrimSpace(by)) {
	case "":
		return
	case "file":
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Filename != rows[j].Filename {
				return rows[i].Filename < rows[j].Filename
			}
			return rows[i].LineNum < rows[j].LineNum
		})
		return
	case "date", "deadline", "scheduled":
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := commands.DateOf(rows[i]), commands.DateOf(rows[j])
			// A heading with no date sorts last rather than first, because a
			// list sorted by date is being read for what is coming up.
			if (a == "") != (b == "") {
				return b == ""
			}
			return strings.TrimPrefix(a, "!") < strings.TrimPrefix(b, "!")
		})
		return
	case "priority", "pri":
		key = func(t common.Todo) string {
			if t.Priority == "" {
				return "~" // after every letter
			}
			return t.Priority
		}
	case "status", "keyword", "todo":
		key = func(t common.Todo) string { return t.Status }
	case "headline", "title", "text":
		key = func(t common.Todo) string { return strings.ToLower(t.Headline) }
	default:
		fmt.Fprintf(os.Stderr, "orgs search: no sort called %q, leaving the order alone\n", by)
		return
	}
	sort.SliceStable(rows, func(i, j int) bool { return key(rows[i]) < key(rows[j]) })
}

const usage = `  orgs search '<query>'            headings matching the expression
  orgs search '<query>' -sort date  in date order
  orgs search '<query>' -open       pick one and open it
  orgs search '<query>' -json       for a program to read

The expression is the server's own - IsTask(), IsStatus("NEXT"),
HasProperty("EFFORT"), MatchHeadline("re"), Today(), InCollection("contact"),
HasBacklinks(), LinksTo("re") and the rest - and a {{ Filter }} from the yaml
is expanded before it is parsed.`

func init() {
	commands.AddCmd("search", "query the parsed database with an expression",
		func() commands.Cmd { return &Search{} })
}

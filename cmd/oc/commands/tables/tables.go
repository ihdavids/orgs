package tables

// Every org table in the database, from the terminal.
//
//	orgs tables                       pick one, with it drawn beside the list
//	orgs tables ls                    the listing
//	orgs tables ls -file budget.org   one file's tables
//	orgs tables show expenses         one table, drawn
//	orgs tables eval expenses         run its formulas and write the result back
//	orgs tables ls -json              for a program to read
//	orgs tables import data.csv notes.org -header -name sales
//	                                  a csv (or tsv, or - for stdin) as a table
//
// This is worg's Tables tab as a picker. What makes a table worth a view of its
// own rather than a grep is the same thing that makes a source block worth one:
// there is more to it than the text. A table has a name other blocks call it
// by, a shape, and - the part nothing else will show you - **formulas**, which
// are written on a `#+TBLFM:` line at the bottom and say nothing about which
// cell they fill. The server works that out; this draws it.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Tables struct {
	fset *flag.FlagSet

	Query   string
	File    string
	Id      int
	Group   string
	Limit   int
	Formula bool
	Eval    bool
	Open    bool

	// import
	Header bool
	Name   string
	Sep    string
	At     int
	Under  string
}

func (self *Tables) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Tables) StartPlugin(manager *common.PluginManager)         {}

func (self *Tables) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Query, "q", "", "only tables matching this text")
	fset.StringVar(&self.File, "file", "", "only tables in this file")
	fset.IntVar(&self.Id, "id", -1, "the table's id within -file, for an exact address")
	fset.StringVar(&self.Group, "group", "file", "how to gather the listing: flat or file")
	fset.IntVar(&self.Limit, "limit", 0, "keep only the first n")
	fset.BoolVar(&self.Formula, "formulas", false, "show each table's formulas in the listing")
	fset.BoolVar(&self.Eval, "eval", false, "run the formulas and write the values back")
	fset.BoolVar(&self.Open, "open", false, "open what was picked in your editor instead")
	fset.BoolVar(&self.Header, "header", false, "import: the first row is the header, with a rule under it")
	fset.StringVar(&self.Name, "name", "", "import: a #+NAME: for the new table")
	fset.StringVar(&self.Sep, "sep", "", "import: the separator (, ; | tab); guessed when not given")
	fset.IntVar(&self.At, "at", 0, "import: put the table after this line (1 based) rather than at the end")
	fset.StringVar(&self.Under, "under", "", "import: put the table under the heading with this hash")
}

func (self *Tables) Exec(core *commands.Core) {
	// Taken once: FreeArgs consumes the arguments as it parses, so asking the
	// flag set for them afterwards gets nothing.
	words := commands.FreeArgs(self.fset)
	sub := ""
	if len(words) > 0 {
		switch strings.ToLower(words[0]) {
		case "ls", "list", "pick", "show", "cat", "eval", "exec", "preview", "import":
			sub = strings.ToLower(words[0])
			words = words[1:]
		}
	}
	free := strings.TrimSpace(strings.Join(words, " "))

	switch sub {
	case "ls", "list":
		self.list(core, firstNonEmpty(free, self.Query))
	case "show", "cat":
		self.show(core, firstNonEmpty(free, self.Query))
	case "eval", "exec":
		self.evaluate(core, firstNonEmpty(free, self.Query))
	case "preview":
		self.preview(core)
	case "import":
		self.importCsv(core, words)
	default:
		// Bare `orgs tables` is the picker. `orgs tables budget` narrows it,
		// the way `orgs code python` does.
		self.pick(core, firstNonEmpty(free, self.Query))
	}
}

// ---------------------------------------------------------------------------
// import
// ---------------------------------------------------------------------------

// A delimited file read here and written into an org file there as a table.
// The text travels rather than the path, because the server may be another
// machine and the csv is usually in Downloads.
func (self *Tables) importCsv(core *commands.Core, words []string) {
	if len(words) < 1 || (len(words) < 2 && self.Under == "") {
		commands.Fail("orgs tables import data.csv notes.org  (or - for stdin; -under HASH for a heading)")
	}
	var data []byte
	var err error
	if words[0] == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(words[0])
	}
	if err != nil {
		commands.Fail("could not read %s: %v", words[0], err)
	}
	req := common.TableImport{Text: string(data), Header: self.Header, Name: self.Name,
		Separator: self.Sep, AfterLine: -1, Hash: self.Under}
	if len(words) > 1 {
		req.Filename = commands.ResolveOrgFile(core, words[1])
	}
	if self.At > 0 {
		req.AfterLine = self.At - 1
	}
	var reply common.ResultMsg
	commands.SendReceivePost(core, "table/import", &req, &reply)
	if commands.DryRun {
		return
	}
	if !reply.Ok {
		commands.Fail("%s", reply.Msg)
	}
	commands.RenderOne(reply, func() { fmt.Println(reply.Msg) })
}

// ---------------------------------------------------------------------------
// Finding tables
// ---------------------------------------------------------------------------

func (self *Tables) index(core *commands.Core, q string) []common.TableInfo {
	ps := map[string]string{}
	if self.File != "" {
		ps["filename"] = self.File
	}
	res, err := commands.SendReceiveGetErr[common.TableListResult](core, "tables", ps)
	if err != nil {
		commands.Fail("orgs tables: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs tables: %s", res.Msg)
	}
	out := res.Tables
	// The text filter is applied here rather than asked for: /tables takes a
	// filename and nothing else, and everything worth matching on - the name,
	// the heading, the file - is already in the answer.
	if q != "" {
		n := strings.ToLower(q)
		kept := out[:0:0]
		for _, t := range out {
			if strings.Contains(strings.ToLower(tableHay(t)), n) {
				kept = append(kept, t)
			}
		}
		out = kept
	}
	if self.Limit > 0 && len(out) > self.Limit {
		out = out[:self.Limit]
	}
	return out
}

func tableHay(t common.TableInfo) string {
	return strings.Join(append([]string{t.Name, t.Heading, t.Filename}, t.Olp...), " ")
}

// A table's handle: its name when it has one, and "file:id" when it has not.
// The id is its position in the file in document order, which is the only
// identity an unnamed table has.
func handle(t common.TableInfo) string {
	if t.Name != "" {
		return t.Name
	}
	return fmt.Sprintf("%s:%d", commands.BaseName(t.Filename), t.Id)
}

func byAddress(tables []common.TableInfo, filename string, id int) (common.TableInfo, bool) {
	want := strings.ToLower(filename)
	for _, t := range tables {
		if t.Id == id && t.Filename == filename {
			return t, true
		}
	}
	// A person typing the address types `-file notes.org`, and refusing that
	// while printing the table as "notes.org:3" in the listing is telling
	// somebody to use a name and then not accepting it.
	for _, t := range tables {
		if t.Id != id {
			continue
		}
		if strings.EqualFold(commands.BaseName(t.Filename), filename) ||
			strings.HasSuffix(strings.ToLower(t.Filename), want) {
			return t, true
		}
	}
	return common.TableInfo{}, false
}

// resolve turns what somebody typed into one table: an exact address, a name,
// or - failing both - a chooser.
func (self *Tables) resolve(core *commands.Core, sel string) common.TableInfo {
	if self.File != "" && self.Id >= 0 {
		if t, ok := byAddress(self.index(core, ""), self.File, self.Id); ok {
			return t
		}
		commands.Fail("orgs tables: %s has no table %d", self.File, self.Id)
	}
	all := self.index(core, "")
	if sel != "" {
		for _, t := range all {
			if t.Name != "" && strings.EqualFold(t.Name, sel) {
				return t
			}
		}
		if i := strings.LastIndex(sel, ":"); i > 0 {
			if id, err := strconv.Atoi(sel[i+1:]); err == nil {
				if t, ok := byAddress(all, sel[:i], id); ok {
					return t
				}
			}
		}
		narrowed := self.index(core, sel)
		if len(narrowed) == 1 {
			return narrowed[0]
		}
		if len(narrowed) == 0 {
			commands.Fail("orgs tables: nothing called %q", sel)
		}
		all = narrowed
	}
	if len(all) == 0 {
		commands.Fail("orgs tables: no tables")
	}
	if !commands.Interactive() {
		commands.Fail("orgs tables: %q matches %d tables; name one as file:id", sel, len(all))
	}
	chosen := self.choose(core, all, "")
	if len(chosen) == 0 {
		os.Exit(0)
	}
	return chosen[0]
}

// ---------------------------------------------------------------------------
// The listing
// ---------------------------------------------------------------------------

func (self *Tables) list(core *commands.Core, q string) {
	rows := self.index(core, q)
	commands.Render(rows, func() {
		if len(rows) == 0 {
			fmt.Fprintln(os.Stderr, "no tables matched")
			return
		}
		if strings.ToLower(self.Group) == "flat" || strings.ToLower(self.Group) == "none" {
			for _, t := range rows {
				fmt.Println(listLine(t))
				self.printFormulas(t)
			}
			return
		}
		groups := map[string][]common.TableInfo{}
		order := []string{}
		for _, t := range rows {
			if _, seen := groups[t.Filename]; !seen {
				order = append(order, t.Filename)
			}
			groups[t.Filename] = append(groups[t.Filename], t)
		}
		sort.Strings(order)
		for i, f := range order {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("%s%s%s %s(%d)%s\n",
				commands.C(commands.AnsiBold), f, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), len(groups[f]), commands.C(commands.AnsiReset))
			for _, t := range groups[f] {
				fmt.Printf("  %s\n", listLine(t))
				self.printFormulas(t)
			}
		}
		fmt.Fprintf(os.Stderr, "\n%d table(s)\n", len(rows))
	})
}

func listLine(t common.TableInfo) string {
	name := t.Name
	if name == "" {
		name = commands.C(commands.AnsiDim) + handle(t) + commands.C(commands.AnsiReset)
	} else {
		name = commands.C(commands.AnsiBold) + name + commands.C(commands.AnsiReset)
	}
	where := t.Heading
	if where == "" {
		where = "(preamble)"
	}
	fx := ""
	if t.Formulas > 0 {
		fx = fmt.Sprintf(" %s·%d formula%s%s", commands.C(commands.AnsiGreen),
			t.Formulas, plural(t.Formulas), commands.C(commands.AnsiReset))
	}
	return fmt.Sprintf("%s %s%d×%d%s%s\n      %s%s — %s:%d%s",
		name,
		commands.C(commands.AnsiCyan), t.Rows, t.Cols, commands.C(commands.AnsiReset), fx,
		commands.C(commands.AnsiDim), where, commands.BaseName(t.Filename), t.Line+1,
		commands.C(commands.AnsiReset))
}

func (self *Tables) printFormulas(t common.TableInfo) {
	if !self.Formula || t.Formulas == 0 {
		return
	}
	// The formulas are only in the full table, so the listing would have to ask
	// per row to show them. Said rather than silently skipped.
	fmt.Printf("      %s(ask for it with `orgs tables show %s` to see them)%s\n",
		commands.C(commands.AnsiDim), handle(t), commands.C(commands.AnsiReset))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ---------------------------------------------------------------------------
// One table
// ---------------------------------------------------------------------------

func (self *Tables) fetch(core *commands.Core, t common.TableInfo) *common.TableData {
	res, err := commands.SendReceiveGetErr[common.TableResult](core, "table", map[string]string{
		"filename": t.Filename,
		"id":       strconv.Itoa(t.Id),
	})
	if err != nil {
		commands.Fail("orgs tables: %v", err)
	}
	if !res.Ok || res.Table == nil {
		commands.Fail("orgs tables: %s", firstNonEmpty(res.Msg, "that table could not be read"))
	}
	return res.Table
}

func (self *Tables) show(core *commands.Core, sel string) {
	t := self.resolve(core, sel)
	data := self.fetch(core, t)
	if commands.RenderOne(data, nil) {
		return
	}
	if self.Open {
		core.LaunchEditor(t.Filename, t.Line+1)
		return
	}
	renderPane(data, commands.PaneWidth())
}

// Run the table's formulas and write the values back into the org file.
//
// Through `/table/eval`, which takes the file and the table's ordinal - the
// same address everything else here uses - and writes the computed values to
// disk.
//
// Not `/exectable`, which is the obvious-looking one and does something else.
// Its documentation says "the file is re-saved to disk after the update" and
// `ExecTable` does no such thing: it runs the formulas into the *in-memory*
// table and hands back the rendered org text. Calling it appeared to work -
// reading the table afterwards showed the computed values, because the read
// comes from the same memory - and left the file on disk untouched, so the
// numbers vanished on the next reload.
func (self *Tables) evaluate(core *commands.Core, sel string) {
	t := self.resolve(core, sel)
	if t.Formulas == 0 {
		commands.Fail("orgs tables eval: %s has no formulas", handle(t))
	}
	if commands.Wrote(fmt.Sprintf("evaluate %s — %s:%d", handle(t), t.Filename, t.Line+1), nil) {
		return
	}
	req := common.TableEdit{Filename: t.Filename, Id: t.Id}
	res, err := commands.SendReceivePostErr[common.TableEdit, common.TableResult](core, "table/eval", &req)
	if err != nil {
		if err == commands.ErrDryRun {
			return
		}
		commands.Fail("orgs tables eval: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs tables eval: %s", res.Msg)
	}
	if commands.RenderOne(res.Table, nil) {
		return
	}
	// Drawn afterwards, because what the formulas did is the answer and "Ok"
	// is not it. The endpoint hands the table back already re-read, so this
	// costs no second request.
	if res.Table != nil {
		renderPane(res.Table, commands.PaneWidth())
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func init() {
	commands.AddCmd("tables", "every org table, drawn - the Tables tab as a picker",
		func() commands.Cmd { return &Tables{Id: -1, Group: "file"} })
}

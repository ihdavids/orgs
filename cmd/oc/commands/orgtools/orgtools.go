package orgtools

// Five commands that were endpoints with nobody to call them.
//
//	orgs tangle notes.org      write the source blocks out as files
//	orgs fmt                   normalise an org file - and -check it
//	orgs tags                  the tag vocabulary, and what uses each one
//	orgs outline notes.org     the tree, which nothing else in the CLI showed
//	orgs log                   a heading's clock entries
//
// They are together because each is one request and none of them has any logic
// of its own worth a package. What they have in common is that they are the
// *org* half of the tool rather than the task half: tangling and formatting are
// things you do to a file, and the outline is how a file is read.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// orgs tangle
// ---------------------------------------------------------------------------

type Tangle struct {
	Write bool
	Out   string
}

func (self *Tangle) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Tangle) StartPlugin(m *common.PluginManager)       {}

func (self *Tangle) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Write, "w", false, "write the files out (on the server's disk)")
	fset.StringVar(&self.Out, "o", "",
		"write the files here instead, on this machine - a directory, or - for stdout")
}

// What one tangled file is, on the wire. Restated rather than imported: the
// plugin lives under internal/app/orgs, which a command may not import.
type tangleFile struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Lang     string `json:"lang"`
	Lines    int    `json:"lines"`
}

type tangleResult struct {
	Files []tangleFile `json:"files"`
	Error string       `json:"error,omitempty"`
}

func (self *Tangle) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("tangle").Flags)
	if len(words) == 0 {
		commands.Fail("orgs tangle: say which file.\n\n%s", tangleUsage)
	}

	// -w and -o are two different machines, which is the distinction `orgs
	// export` had to learn the hard way: the server writing to its own disk is
	// right when the two are the same machine and surprising everywhere else.
	// So the default here is to say what *would* be written, -o writes it on
	// this machine, and -w is the explicit "on the server".
	params := map[string]string{"filename": words[0]}
	if self.Write {
		if commands.Wrote("tangle "+words[0]+" on the server", nil) {
			return
		}
		params["write"] = "t"
	}
	res, err := commands.SendReceiveGetErr[tangleResult](core, "tangle", params)
	if err != nil {
		commands.Fail("orgs tangle: %v", err)
	}
	if res.Error != "" {
		commands.Fail("orgs tangle: %s", res.Error)
	}
	if len(res.Files) == 0 {
		commands.Fail("nothing to tangle in %s - no block has a :tangle header", words[0])
	}

	if self.Out != "" {
		self.writeHere(core, words[0], res.Files)
		return
	}

	if commands.Render(res.Files, func() {
		for _, f := range res.Files {
			where := "would write"
			if self.Write {
				where = "wrote"
			}
			fmt.Printf("%s%s%s %s%s%s  %s%d lines, %s%s\n",
				commands.C(commands.AnsiGreen), where, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiBold), f.Filename, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), f.Lines, f.Lang, commands.C(commands.AnsiReset))
		}
		if !self.Write {
			fmt.Printf("%s-w writes them on the server, -o DIR writes them here%s\n",
				commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		}
	}) {
		return
	}
}

// writeHere puts the tangled files on this machine.
//
// The server answers with absolute paths, because it resolved each `:tangle
// hello.py` against the org file's own directory on its own disk. Writing those
// under -o would rebuild the server's whole path here, so the layout is taken
// *relative to the org file* - which is the shape the blocks asked for and the
// one somebody pointing -o at a source tree means.
//
// Anything that still escapes that directory, and any absolute path a block
// named outright, falls back to its base name: a path in somebody's org file
// must not be able to choose where on this disk it lands.
func (self *Tangle) writeHere(core *commands.Core, named string, files []tangleFile) {
	base := self.orgDir(core, named, files)
	if self.Out == "-" {
		for i, f := range files {
			if len(files) > 1 {
				if i > 0 {
					fmt.Println()
				}
				fmt.Printf("# %s\n", f.Filename)
			}
			fmt.Print(f.Content)
		}
		return
	}
	for _, f := range files {
		name := filepath.FromSlash(f.Filename)
		rel := filepath.Base(name)
		if base != "" {
			if r, err := filepath.Rel(base, name); err == nil && !strings.HasPrefix(r, "..") {
				rel = r
			}
		}
		dest := filepath.Join(self.Out, filepath.Clean("/"+rel))
		if commands.Wrote("write "+dest, nil) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			commands.Fail("could not make %s: %v", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, []byte(f.Content), 0o644); err != nil {
			commands.Fail("could not write %s: %v", dest, err)
		}
		fmt.Printf("%s✓%s %s %s(%d lines)%s\n", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset), dest, commands.C(commands.AnsiDim), f.Lines,
			commands.C(commands.AnsiReset))
	}
}

// orgDir is the directory the org file lives in on the server, which is what
// the tangle paths are relative to. Asked of the file list rather than guessed
// at, and when it cannot be worked out the answer is empty and writeHere falls
// back to base names.
func (self *Tangle) orgDir(core *commands.Core, named string, files []tangleFile) string {
	if filepath.IsAbs(named) {
		return filepath.Dir(named)
	}
	for _, k := range commands.SendReceiveGetOr[common.FileList](core, "files", nil) {
		if k == named || filepath.Base(k) == named || strings.HasSuffix(k, "/"+named) {
			return filepath.Dir(k)
		}
	}
	return ""
}

const tangleUsage = `  orgs tangle notes.org            say what the blocks would write
  orgs tangle notes.org -w         write them, on the server's disk
  orgs tangle notes.org -o ./src   write them here instead
  orgs tangle notes.org -o -       to stdout
  orgs tangle notes.org -json      for a program to read`

// ---------------------------------------------------------------------------
// orgs fmt
// ---------------------------------------------------------------------------

type Fmt struct {
	Check bool
	All   bool
}

func (self *Fmt) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Fmt) StartPlugin(m *common.PluginManager)       {}

func (self *Fmt) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Check, "check", false,
		"write nothing; exit non-zero if a file is not already formatted")
	fset.BoolVar(&self.All, "all", false, "every file the server is watching")
}

func (self *Fmt) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("fmt").Flags)

	var files []string
	switch {
	case self.All:
		files = commands.SendReceiveGetOr[common.FileList](core, "files", nil)
	case len(words) > 0:
		files = self.resolve(core, words)
	default:
		commands.Fail("orgs fmt: say which files.\n\n%s", fmtUsage)
	}
	if len(files) == 0 {
		commands.Fail("no org files matched")
	}

	// -check is what makes this usable as a pre-commit hook, and it must not
	// write. POST /reformat always writes, so the check goes to the GET of the
	// same path, which answers the question without doing it.
	if self.Check {
		self.check(core, files)
		return
	}

	for _, f := range files {
		var reply common.Result
		commands.SendReceivePost(core, "reformat", &common.FileList{f}, &reply)
		if commands.DryRun {
			continue
		}
		if !reply.Ok {
			commands.Fail("could not reformat %s", f)
		}
		fmt.Printf("%s✓%s %s\n", commands.C(commands.AnsiGreen),
			commands.C(commands.AnsiReset), f)
	}
}

// check reports which files the writer would rewrite, and exits non-zero when
// any would - which is the whole contract a hook or a CI step needs.
//
// The comparison is the server's, not this side's: the file may not be on this
// machine at all, and the answer has to be what the *writer* would produce
// rather than what some second implementation of it here thinks.
func (self *Fmt) check(core *commands.Core, files []string) {
	type row struct {
		Filename  string
		Formatted bool
	}
	rows := []row{}
	bad := 0
	for _, f := range files {
		// text=f: over a hundred files the formatted text is a hundred copies
		// of every org file on the wire, and nothing here reads it.
		res, err := commands.SendReceiveGetErr[common.ReformatCheck](core, "reformat",
			map[string]string{"filename": f, "text": "f"})
		if err != nil {
			commands.Fail("could not check %s: %v", f, err)
		}
		if !res.Ok {
			commands.Fail("could not check %s: %s", f, res.Msg)
		}
		if !res.Formatted {
			bad++
		}
		rows = append(rows, row{Filename: f, Formatted: res.Formatted})
	}
	commands.Render(rows, func() {
		for _, r := range rows {
			if r.Formatted {
				continue
			}
			fmt.Printf("%swould reformat%s %s\n", commands.C(commands.AnsiGold),
				commands.C(commands.AnsiReset), r.Filename)
		}
		if bad == 0 {
			fmt.Printf("%s%d files, all formatted%s\n", commands.C(commands.AnsiDim),
				len(rows), commands.C(commands.AnsiReset))
		}
	})
	if bad > 0 {
		os.Exit(1)
	}
}

// resolve turns the words into org files: a path as given, or a name matched
// against what the server is watching, so `orgs fmt todo.org` works from any
// directory.
func (self *Fmt) resolve(core *commands.Core, words []string) []string {
	known := commands.SendReceiveGetOr[common.FileList](core, "files", nil)
	out := []string{}
	for _, w := range words {
		if abs, err := filepath.Abs(w); err == nil {
			if _, err := os.Stat(abs); err == nil {
				out = append(out, abs)
				continue
			}
		}
		hit := false
		for _, k := range known {
			if k == w || filepath.Base(k) == w || strings.HasSuffix(k, "/"+w) {
				out = append(out, k)
				hit = true
			}
		}
		if !hit {
			commands.Fail("no org file called %s", w)
		}
	}
	return out
}

const fmtUsage = `  orgs fmt todo.org                normalise one file
  orgs fmt -all                    every file the server watches
  orgs fmt -check -all             write nothing, exit 1 if any would change
  orgs fmt -check todo.org         what a pre-commit hook wants`

// ---------------------------------------------------------------------------
// orgs tags
// ---------------------------------------------------------------------------

type Tags struct {
	Count bool
	Open  bool
}

func (self *Tags) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Tags) StartPlugin(m *common.PluginManager)       {}

func (self *Tags) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Count, "count", false, "how many headings carry each tag")
	fset.BoolVar(&self.Open, "open", false, "pick a tag and list what carries it")
}

type tagRow struct {
	Tag      string
	Headings int `json:",omitempty"`
}

func (self *Tags) Exec(core *commands.Core) {
	tags := commands.SendReceiveGetOr[common.ListResult](core, "alltags", nil).Vals
	sort.Slice(tags, func(i, j int) bool { return strings.ToLower(tags[i]) < strings.ToLower(tags[j]) })
	if len(tags) == 0 {
		commands.Fail("no tags anywhere in the database")
	}

	rows := make([]tagRow, 0, len(tags))
	for _, t := range tags {
		r := tagRow{Tag: t}
		if self.Count {
			// One query per tag is a lot of requests, which is why it is behind
			// a flag rather than being what the listing always shows.
			r.Headings = len(commands.SendReceiveGetOr[common.Todos](core, "search",
				map[string]string{"query": fmt.Sprintf("HasTags(%q)", t)}))
		}
		rows = append(rows, r)
	}
	if self.Count {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Headings > rows[j].Headings })
	}

	if self.Open && commands.Interactive() {
		self.pick(core, rows)
		return
	}

	commands.Render(rows, func() {
		for _, r := range rows {
			if self.Count {
				fmt.Printf("%s%4d%s  %s%s%s\n", commands.C(commands.AnsiDim), r.Headings,
					commands.C(commands.AnsiReset), commands.C(commands.AnsiCyan), r.Tag,
					commands.C(commands.AnsiReset))
				continue
			}
			fmt.Printf("%s%s%s\n", commands.C(commands.AnsiCyan), r.Tag,
				commands.C(commands.AnsiReset))
		}
	})
}

func (self *Tags) pick(core *commands.Core, rows []tagRow) {
	lines := []string{}
	for _, r := range rows {
		label := r.Tag
		if self.Count {
			label = fmt.Sprintf("%-24s %s%d%s", r.Tag, commands.C(commands.AnsiDim), r.Headings,
				commands.C(commands.AnsiReset))
		}
		lines = append(lines, commands.PickLine([]string{r.Tag}, label))
	}
	self_, err := commands.SelfCommand(core)
	preview := ""
	if err == nil {
		preview = self_ + ` search 'HasTags("'{1}'")' -no-color`
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Tag> ",
		Preview:       preview,
	})
	if len(sel) == 0 {
		return
	}
	addr, _ := commands.Address(sel[0], 1)
	todos := commands.SendReceiveGetOr[common.Todos](core, "search",
		map[string]string{"query": fmt.Sprintf("HasTags(%q)", addr[0])})
	for _, t := range todos {
		fmt.Printf("%s %s%s%s\n", commands.Describe(t), commands.C(commands.AnsiGold), t.Status,
			commands.C(commands.AnsiReset))
	}
}

// ---------------------------------------------------------------------------
// orgs outline
// ---------------------------------------------------------------------------

type Outline struct {
	Depth  int
	Todo   bool
	Narrow string
	Open   bool
}

func (self *Outline) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Outline) StartPlugin(m *common.PluginManager)       {}

func (self *Outline) SetupParameters(fset *flag.FlagSet) {
	fset.IntVar(&self.Depth, "depth", 0, "only headings this deep or shallower")
	fset.BoolVar(&self.Todo, "todo", false, "only headings with a keyword")
	fset.StringVar(&self.Narrow, "match", "",
		"a sparse tree: only headings matching this query, with their parents")
	fset.BoolVar(&self.Open, "open", false, "pick one and open it in the editor")
}

type outlineRow struct {
	Headline string
	Status   string
	Tags     []string
	Level    int
	Filename string
	LineNum  int
	Hash     string
	// Whether this row is only here to hold a matching child up. A sparse tree
	// is unreadable without them and misleading if they are not marked.
	Context bool `json:",omitempty"`
}

func (self *Outline) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("outline").Flags)

	var todos common.Todos
	if len(words) > 0 {
		todos = commands.SendReceiveGetOr[common.Todos](core, "filecontents/headings",
			map[string]string{"filename": words[0]})
		if len(todos) == 0 {
			commands.Fail("no headings in %s - is that a file the server watches?", words[0])
		}
	} else {
		todos = commands.SendReceiveGetOr[common.Todos](core, "filecontents/headings", nil)
	}

	rows := make([]outlineRow, 0, len(todos))
	for _, t := range todos {
		if self.Depth > 0 && t.Level > self.Depth {
			continue
		}
		if self.Todo && t.Status == "" {
			continue
		}
		rows = append(rows, outlineRow{Headline: t.Headline, Status: t.Status, Tags: t.Tags,
			Level: t.Level, Filename: t.Filename, LineNum: t.LineNum, Hash: t.Hash})
	}

	// A sparse tree is org's most useful way of reading a big file: the
	// headings that matched, and enough of their ancestors to say where they
	// are. Without the ancestors a list of matches is a flat list, which is
	// what `orgs search` already is.
	if self.Narrow != "" {
		rows = sparse(core, rows, self.Narrow)
		if len(rows) == 0 {
			commands.Fail("nothing in the outline matched %s", self.Narrow)
		}
	}

	if self.Open && commands.Interactive() {
		self.pick(core, rows)
		return
	}

	commands.Render(rows, func() {
		for _, r := range rows {
			indent := strings.Repeat("  ", max(r.Level-1, 0))
			stars := commands.C(commands.AnsiDim) + strings.Repeat("*", max(r.Level, 1)) +
				commands.C(commands.AnsiReset)
			kw := ""
			if r.Status != "" {
				kw = commands.C(statusInk(r.Status)) + r.Status + commands.C(commands.AnsiReset) + " "
			}
			text := r.Headline
			if r.Context {
				// Drawn faint, because it is here to hold something else up.
				text = commands.C(commands.AnsiDim) + text + commands.C(commands.AnsiReset)
			}
			tags := ""
			if len(r.Tags) > 0 {
				tags = " " + commands.C(commands.AnsiCyan) + ":" + strings.Join(r.Tags, ":") + ":" +
					commands.C(commands.AnsiReset)
			}
			fmt.Printf("%s%s %s%s%s\n", indent, stars, kw, text, tags)
		}
	})
}

// sparse keeps the rows a query matched, plus every ancestor of one. The match
// is asked of the server (it is the query language, not a substring) and the
// ancestors are worked out here from the levels, which is all an outline is.
func sparse(core *commands.Core, rows []outlineRow, query string) []outlineRow {
	hit := map[string]bool{}
	for _, t := range commands.SendReceiveGetOr[common.Todos](core, "search",
		map[string]string{"query": query}) {
		hit[t.Hash] = true
	}
	keep := make([]bool, len(rows))
	for i, r := range rows {
		if !hit[r.Hash] {
			continue
		}
		keep[i] = true
		// Walk back up: the nearest row above with a smaller level is the
		// parent, whatever level that turns out to be - which is the same rule
		// the mind map builder follows, and the only one that survives a
		// filtered list.
		level := r.Level
		for j := i - 1; j >= 0 && level > 1; j-- {
			if rows[j].Level < level {
				keep[j] = true
				level = rows[j].Level
			}
		}
	}
	out := []outlineRow{}
	for i, r := range rows {
		if !keep[i] {
			continue
		}
		r.Context = !hit[r.Hash]
		out = append(out, r)
	}
	return out
}

func (self *Outline) pick(core *commands.Core, rows []outlineRow) {
	lines := []string{}
	for _, r := range rows {
		kw := ""
		if r.Status != "" {
			kw = commands.C(statusInk(r.Status)) + r.Status + commands.C(commands.AnsiReset) + " "
		}
		lines = append(lines, commands.PickLine([]string{r.Hash},
			fmt.Sprintf("%s%s%s", strings.Repeat("  ", max(r.Level-1, 0)), kw, r.Headline)))
	}
	preview := ""
	if self_, err := commands.SelfCommand(core); err == nil {
		preview = self_ + " show -hash {1} -pane"
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Outline> ",
		Preview:       preview,
	})
	if len(sel) == 0 {
		return
	}
	addr, _ := commands.Address(sel[0], 1)
	for _, r := range rows {
		if r.Hash == addr[0] {
			core.LaunchEditor(r.Filename, r.LineNum)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// orgs log - a heading's clock entries
// ---------------------------------------------------------------------------

type Log struct {
	tf commands.TargetFlags
}

func (self *Log) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Log) StartPlugin(m *common.PluginManager)       {}

func (self *Log) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
}

type logRow struct {
	Headline string
	Start    string
	End      string
	Mins     float64
}

func (self *Log) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("log").Flags)
	todos := commands.Resolve(core, &self.tf, words, commands.TargetOpts{
		Prompt: "Logbook of> ",
		Multi:  true,
	})

	rows := []logRow{}
	total := 0.0
	for _, t := range todos {
		lb, err := commands.SendReceiveGetErr[common.Logbook](core,
			"logbook/"+commands.HashPath(t.Hash), nil)
		if err != nil {
			continue
		}
		for _, e := range lb.Entries {
			rows = append(rows, logRow{Headline: t.Headline, Start: e.Start, End: e.End, Mins: e.Mins})
			total += e.Mins
		}
	}
	if len(rows) == 0 {
		commands.Fail("nothing has been clocked against that")
	}

	commands.Render(rows, func() {
		for _, r := range rows {
			start, err := time.Parse(time.RFC3339, r.Start)
			when := r.Start
			at := ""
			if err == nil {
				when = start.Format("2006-01-02 Mon")
				at = start.Format("15:04")
			}
			until := commands.C(commands.AnsiGold) + "running" + commands.C(commands.AnsiReset)
			if r.End != "" {
				if e, err := time.Parse(time.RFC3339, r.End); err == nil {
					until = e.Format("15:04")
				}
			}
			fmt.Printf("%s  %s → %-10s %s%8s%s  %s\n", when, at, until,
				commands.C(commands.AnsiGreen), hours(r.Mins), commands.C(commands.AnsiReset),
				commands.Ellipsis(r.Headline, 40))
		}
		fmt.Printf("%s%42s%s\n", commands.C(commands.AnsiDim),
			"total "+hours(total), commands.C(commands.AnsiReset))
	})
}

// ---------------------------------------------------------------------------

func hours(mins float64) string {
	h, m := int(mins)/60, int(mins)%60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}

func statusInk(status string) string {
	switch strings.ToUpper(status) {
	case "DONE", "CANCELLED", "CANCELED", "CLOSED", "SHIPPED":
		return commands.AnsiGreen
	case "BLOCKED", "WAITING", "HOLD":
		return commands.AnsiRed
	default:
		return commands.AnsiGold
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	commands.AddCmd("tangle", "write a file's source blocks out as files",
		func() commands.Cmd { return &Tangle{} })
	commands.AddCmd("fmt", "normalise org files, or check that they already are",
		func() commands.Cmd { return &Fmt{} })
	commands.AddCmd("tags", "every tag in the database",
		func() commands.Cmd { return &Tags{} })
	commands.AddCmd("outline", "a file's tree, whole or as a sparse tree",
		func() commands.Cmd { return &Outline{} })
	commands.AddCmd("log", "what has been clocked against a heading",
		func() commands.Cmd { return &Log{} })
}

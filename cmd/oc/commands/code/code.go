package code

// Source blocks, from the terminal: find them, read them, run them.
//
//	orgs code ls                      every source block
//	orgs code ls -lang python         the python ones
//	orgs code ls chart                the ones matching "chart"
//	orgs code show monthly-report     the code, coloured by nothing
//	orgs code run monthly-report      run it and print what it produced
//	orgs code run notes.org:3         run the fourth block in that file
//	orgs code run -pick               choose one and run it
//	orgs code ls -json | jq '.[].Lang'
//
// A block is worth an endpoint rather than a grep because of its *variables*.
// `:var data=monthly` and `:var scale=2` are the same shape and mean entirely
// different things - one names a table further up the file, the other is the
// number two - and only the database can tell which. So each variable comes
// back resolved: what it points at, what kind of thing that is, and where it
// lives. A name that resolves to nothing is drawn as a warning, because that
// is a block that cannot run.
//
// Running is off unless `babel.enable` is set in the server settings, and the
// refusal says what to write. Reading somebody's org files and executing the
// programs inside them are different promises.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
)

type Code struct {
	fset *flag.FlagSet

	Lang    string
	Query   string
	File    string
	Group   string
	Id      int
	Pick    bool
	Run     bool
	NoRun   bool
	Open    bool
	Vars    bool
	Raw     bool
	Verbose bool
	Limit   int
}

func (self *Code) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Code) StartPlugin(manager *common.PluginManager)         {}

func (self *Code) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Lang, "lang", "", "only blocks in this language")
	fset.StringVar(&self.Query, "q", "", "only blocks matching this text")
	fset.StringVar(&self.File, "file", "", "only blocks in this file")
	fset.StringVar(&self.Group, "group", "lang", "how to gather the listing: flat, lang or file")
	fset.IntVar(&self.Id, "id", -1, "the block's id within -file, for an exact address")
	fset.BoolVar(&self.Pick, "pick", false, "choose the block rather than naming it")
	fset.BoolVar(&self.Run, "run", false, "run what was picked without asking")
	fset.BoolVar(&self.NoRun, "no-run", false, "never offer to run what was picked")
	fset.BoolVar(&self.Open, "open", false, "open what was picked in your editor instead")
	fset.BoolVar(&self.Vars, "vars", false, "show each block's variables in the listing")
	fset.BoolVar(&self.Raw, "raw", false, "print what the program printed rather than the shaped result")
	fset.BoolVar(&self.Verbose, "v", false, "show stderr and how long it took, even on success")
	fset.IntVar(&self.Limit, "limit", 0, "keep only the first n")
}

func (self *Code) Exec(core *commands.Core) {
	// Bare `orgs code` is the picker, the way bare `orgs grep` is. A listing is
	// still one word away, and is what -json and -format get whatever was
	// asked for, since nothing is reading a chooser.
	sub, words := "pick", []string{}
	if self.fset != nil {
		args := self.fset.Args()
		if len(args) > 0 {
			sub = args[0]
			args = args[1:]
		}
		// Flags may come before or after the words.
		self.fset.Parse(args)
		words = append(words, commands.FreeArgs(self.fset)...)
	}
	free := strings.TrimSpace(strings.Join(words, " "))

	switch strings.ToLower(sub) {
	case "pick", "":
		self.pick(core, free)
	case "ls", "list":
		self.list(core, firstNonEmpty(free, self.Query))
	case "show", "cat", "read":
		self.show(core, firstNonEmpty(free, self.Query))
	case "run", "exec", "eval":
		self.run(core, firstNonEmpty(free, self.Query))
	case "preview":
		// What fzf shells out to. Harmless to run by hand, and the easiest way
		// to see what the pane is doing when it is doing the wrong thing.
		self.preview(core)
	default:
		// `orgs code python` should narrow the picker rather than complain -
		// the first word is only a subcommand when it is one of the five.
		self.pick(core, strings.TrimSpace(sub+" "+free))
	}
}

// ---------------------------------------------------------------------------
// Finding blocks
// ---------------------------------------------------------------------------

func (self *Code) index(core *commands.Core, q string) common.CodeIndex {
	ps := map[string]string{}
	if self.Lang != "" {
		ps["lang"] = strings.ToLower(self.Lang)
	}
	if q != "" {
		ps["q"] = q
	}
	idx, err := commands.SendReceiveGetErr[common.CodeIndex](core, "code", ps)
	if err != nil {
		commands.Fail("orgs code: %v", err)
	}
	if self.File != "" {
		// The server's q already matches the filename, which is too loose when
		// somebody means one file. Done here rather than added there so the
		// two ways of asking stay distinct.
		kept := idx.Blocks[:0:0]
		for _, b := range idx.Blocks {
			if strings.Contains(strings.ToLower(b.Filename), strings.ToLower(self.File)) {
				kept = append(kept, b)
			}
		}
		idx.Blocks = kept
	}
	return idx
}

// resolve turns what somebody typed into one block. A name, a "file.org:3",
// or - failing both, or when -pick was asked for - a chooser.
func (self *Code) resolve(core *commands.Core, sel string) common.CodeBlock {
	// -file with -id is an exact address and is what the picker's own bindings
	// pass back to this binary. It is checked first so that a block whose name
	// happens to look like somebody else's cannot win over it.
	if self.File != "" && self.Id >= 0 {
		if b, ok := byAddress(self.index(core, baseName(self.File)).Blocks, self.File, self.Id); ok {
			return b
		}
		if b, ok := byAddress(self.index(core, "").Blocks, self.File, self.Id); ok {
			return b
		}
		commands.Fail("orgs code: %s has no block %d", self.File, self.Id)
	}
	idx := self.index(core, "")
	blocks := idx.Blocks
	if self.File != "" || self.Lang != "" {
		// Already narrowed by index().
	}
	if !self.Pick && sel != "" {
		if b, ok := match(blocks, sel); ok {
			return b
		}
		// Not an exact handle. Narrow by it and let the chooser do the rest,
		// which is what somebody typing half a name meant.
		narrowed := self.index(core, sel).Blocks
		if len(narrowed) == 1 {
			return narrowed[0]
		}
		if len(narrowed) == 0 {
			commands.Fail("orgs code: nothing called %q", sel)
		}
		blocks = narrowed
	}
	if len(blocks) == 0 {
		commands.Fail("orgs code: no source blocks")
	}
	if commands.Machine() {
		// Nothing is reading a chooser, so refuse rather than hang.
		commands.Fail("orgs code: %q matches %d blocks; name one as file:id", sel, len(blocks))
	}
	f, err := fzf.New()
	if err != nil {
		commands.Fail("%v", err)
	}
	idxs, err := f.Find(blocks, func(i int) string { return handle(blocks[i]) + "  " + summary(blocks[i]) })
	if err != nil || len(idxs) == 0 {
		os.Exit(0)
	}
	return blocks[idxs[0]]
}

// A block's handle: its name when it has one, and "file:id" when it does not.
// The id is its position in document order, which is the only identity an
// unnamed block has - the same way a table is addressed.
func handle(b common.CodeBlock) string {
	if b.Name != "" {
		return b.Name
	}
	return fmt.Sprintf("%s:%d", baseName(b.Filename), b.Id)
}

func match(blocks []common.CodeBlock, sel string) (common.CodeBlock, bool) {
	for _, b := range blocks {
		if b.Name != "" && strings.EqualFold(b.Name, sel) {
			return b, true
		}
	}
	// file:id — the file part may be a basename or any tail of the path.
	if i := strings.LastIndex(sel, ":"); i > 0 {
		name, num := sel[:i], sel[i+1:]
		if id, err := strconv.Atoi(num); err == nil {
			for _, b := range blocks {
				if b.Id == id && (strings.EqualFold(baseName(b.Filename), name) ||
					strings.HasSuffix(strings.ToLower(b.Filename), strings.ToLower(name))) {
					return b, true
				}
			}
		}
	}
	return common.CodeBlock{}, false
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func summary(b common.CodeBlock) string {
	where := b.Heading
	if where == "" {
		where = "(preamble)"
	}
	return fmt.Sprintf("[%s] %s — %s:%d", b.Lang, where, baseName(b.Filename), b.Line+1)
}

// ---------------------------------------------------------------------------
// ls
// ---------------------------------------------------------------------------

func (self *Code) list(core *commands.Core, q string) {
	idx := self.index(core, q)
	rows := idx.Blocks
	if self.Limit > 0 && len(rows) > self.Limit {
		rows = rows[:self.Limit]
	}
	commands.Render(rows, func() {
		if len(rows) == 0 {
			fmt.Fprintln(os.Stderr, "no source blocks matched")
			return
		}
		switch strings.ToLower(self.Group) {
		case "flat", "none", "":
			for _, b := range rows {
				fmt.Println(blockLine(b))
				self.printVars(b)
			}
		case "file":
			self.banded(rows, func(b common.CodeBlock) string { return b.Filename })
		default:
			self.banded(rows, func(b common.CodeBlock) string {
				if b.Lang == "" {
					return "none"
				}
				return b.Lang
			})
		}
		// The counts are of the whole database rather than of what survived
		// the filter: counting the filtered set would empty the strip the
		// moment a language was picked and leave no way back.
		if len(idx.Langs) > 1 {
			parts := []string{}
			langs := append([]common.CodeLang{}, idx.Langs...)
			sort.Slice(langs, func(i, j int) bool { return langs[i].Count > langs[j].Count })
			for _, l := range langs {
				parts = append(parts, fmt.Sprintf("%s %d", l.Lang, l.Count))
			}
			fmt.Fprintf(os.Stderr, "\n%d of %d blocks · %s\n", len(rows), idx.Total, strings.Join(parts, " · "))
		}
	})
}

func (self *Code) banded(rows []common.CodeBlock, by func(common.CodeBlock) string) {
	groups := map[string][]common.CodeBlock{}
	order := []string{}
	for _, b := range rows {
		k := by(b)
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], b)
	}
	sort.Slice(order, func(i, j int) bool {
		if len(groups[order[i]]) != len(groups[order[j]]) {
			return len(groups[order[i]]) > len(groups[order[j]])
		}
		return order[i] < order[j]
	})
	for i, k := range order {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s%s%s %s(%d)%s\n",
			commands.C(commands.AnsiBold), k, commands.C(commands.AnsiReset),
			commands.C(commands.AnsiDim), len(groups[k]), commands.C(commands.AnsiReset))
		for _, b := range groups[k] {
			fmt.Printf("  %s\n", blockLine(b))
			self.printVars(b)
		}
	}
}

func blockLine(b common.CodeBlock) string {
	var s strings.Builder
	name := b.Name
	if name == "" {
		name = commands.C(commands.AnsiDim) + fmt.Sprintf("%s:%d", baseName(b.Filename), b.Id) + commands.C(commands.AnsiReset)
	} else {
		name = commands.C(commands.AnsiBold) + name + commands.C(commands.AnsiReset)
	}
	fmt.Fprintf(&s, "%s %s%s%s", name, commands.C(commands.AnsiCyan), b.Lang, commands.C(commands.AnsiReset))
	fmt.Fprintf(&s, " %s%d line(s)%s", commands.C(commands.AnsiDim), b.Lines, commands.C(commands.AnsiReset))
	if b.Result != "" {
		fmt.Fprintf(&s, " %s· ran%s", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset))
	}
	fmt.Fprintf(&s, "\n      %s%s%s", commands.C(commands.AnsiDim), summary(b), commands.C(commands.AnsiReset))
	return s.String()
}

func (self *Code) printVars(b common.CodeBlock) {
	if !self.Vars || len(b.Vars) == 0 {
		return
	}
	for _, v := range b.Vars {
		switch {
		case v.Ref == "":
			fmt.Printf("      %s:var %s=%s%s\n",
				commands.C(commands.AnsiDim), v.Name, v.Value, commands.C(commands.AnsiReset))
		case v.RefKind == "":
			// A name that resolves to nothing is a block that cannot run, so
			// it is said in danger colours rather than left looking ordinary.
			fmt.Printf("      %s:var %s=%s → nothing%s\n",
				commands.C(commands.AnsiRed), v.Name, v.Value, commands.C(commands.AnsiReset))
		case v.Rows > 0:
			fmt.Printf("      %s:var %s=%s → %s %dx%d in %s%s\n",
				commands.C(commands.AnsiDim), v.Name, v.Value, v.RefKind, v.Rows, v.Cols,
				baseName(v.RefFile), commands.C(commands.AnsiReset))
		default:
			fmt.Printf("      %s:var %s=%s → %s in %s%s\n",
				commands.C(commands.AnsiDim), v.Name, v.Value, v.RefKind,
				baseName(v.RefFile), commands.C(commands.AnsiReset))
		}
	}
}

// ---------------------------------------------------------------------------
// show
// ---------------------------------------------------------------------------

func (self *Code) show(core *commands.Core, sel string) {
	self.detail(self.resolve(core, sel))
}

// One block, printed. Split out from `show` so that the picker can print what
// was picked without going back to the server to find it again.
func (self *Code) detail(b common.CodeBlock) {
	if commands.RenderOne(b, nil) {
		return
	}
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiBold), handle(b), commands.C(commands.AnsiReset))
	fmt.Printf("%s%s%s\n", commands.C(commands.AnsiDim), summary(b), commands.C(commands.AnsiReset))
	self.Vars = true
	self.printVars(b)
	fmt.Println()
	// The code arrives dedented - the indent belongs to the org file rather
	// than to the program - so it is printed as the program, not as the file.
	fmt.Println(b.Code)
	if b.Result != "" {
		fmt.Printf("%s#+RESULTS: (%s)%s\n%s\n",
			commands.C(commands.AnsiDim), b.ResultKind, commands.C(commands.AnsiReset), b.Result)
	}
}

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------

func (self *Code) run(core *commands.Core, sel string) {
	self.runBlock(core, self.resolve(core, sel))
}

// Run one block that has already been found. Split out from `run` for the same
// reason `detail` is: the picker has the block in its hand.
func (self *Code) runBlock(core *commands.Core, b common.CodeBlock) {
	req := common.CodeRun{Filename: b.Filename, Id: b.Id}

	// Running a block is a write as far as anybody watching is concerned: it
	// executes a program, and it may put a #+RESULTS: under the block. A dry
	// run says which block and in what language, and runs nothing.
	if commands.Wrote(fmt.Sprintf("run %s [%s] — %s:%d", handle(b), b.Lang, b.Filename, b.Line+1), nil) {
		return
	}

	res, err := commands.SendReceivePostErr[common.CodeRun, common.CodeResult](core, "code/run", &req)
	if err != nil {
		commands.Fail("orgs code run: %v", err)
	}
	if !res.Ok {
		commands.Fail("orgs code run: %s", res.Msg)
	}
	if commands.RenderOne(res, nil) {
		return
	}

	out := res.Result
	if self.Raw || out == "" {
		out = res.Raw
	}
	if strings.TrimSpace(out) != "" {
		fmt.Println(strings.TrimRight(out, "\n"))
	}

	// A result that names a file is only half an answer, so the server goes
	// and looks. Say what it found - including, and especially, that the block
	// named a file it never wrote.
	if res.File != "" {
		switch {
		case !res.Exists:
			fmt.Fprintf(os.Stderr, "%s%s was named but is not there%s\n",
				commands.C(commands.AnsiRed), res.File, commands.C(commands.AnsiReset))
		default:
			fmt.Fprintf(os.Stderr, "%s%s — %s, %s%s\n",
				commands.C(commands.AnsiDim), res.File, res.Media, size(res.Bytes), commands.C(commands.AnsiReset))
			if res.Media == "text" && res.Text != "" && !self.Raw {
				fmt.Println(strings.TrimRight(res.Text, "\n"))
				if res.Truncated {
					fmt.Fprintf(os.Stderr, "%s… and more%s\n",
						commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
				}
			}
		}
	}

	// Stderr is worth showing even on success: plenty of programs warn.
	if strings.TrimSpace(res.Stderr) != "" && (self.Verbose || res.Code != 0) {
		fmt.Fprintf(os.Stderr, "%s%s%s\n",
			commands.C(commands.AnsiGold), strings.TrimRight(res.Stderr, "\n"), commands.C(commands.AnsiReset))
	}
	if res.Code != 0 {
		fmt.Fprintf(os.Stderr, "%sexit %d%s\n", commands.C(commands.AnsiRed), res.Code, commands.C(commands.AnsiReset))
		os.Exit(res.Code)
	}
	if self.Verbose {
		fmt.Fprintf(os.Stderr, "%s%.2fs%s\n", commands.C(commands.AnsiDim), res.Seconds, commands.C(commands.AnsiReset))
	}
}

func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fkB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
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
	commands.AddCmd("code", "find, read and run the source blocks in your org files",
		func() commands.Cmd { return &Code{Group: "lang"} })
}

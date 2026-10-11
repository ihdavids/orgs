package refactor

// Structural refactors from the terminal.
//
//	orgs retag work job                  rename a tag everywhere (a merge when job exists)
//	orgs retag -delete stale             take a tag off everything
//	orgs retag work job -query 'IsProject()'   only the headlines a query matches
//	orgs retag work job -file a.org,b.org      only some files
//	orgs split Apollo                    a heading out to a file of its own
//	orgs split Apollo -to projects/      ... into a folder, or -to path/name.org
//	orgs merge Rocket Booster            Booster (and its children) into Rocket
//	orgs list2headings Shopping          a list in a heading's text into child headings
//	orgs list2headings Shopping -line 12 ... the list at that line (1 based)
//
// Each shows the change as a coloured unified diff, says what it could not do
// for you, and asks before writing (-yes does not ask; -dry-run is the diff and
// nothing else; -json is the server's plan as it stands).
//
// Nothing about what a refactor does is decided here: the preview and the
// apply are /refactor/*/preview and /refactor/*/apply, which the worg
// Refactor tab uses too. The apply carries the preview's token, so a file
// that changed while the question was on screen is refused, not overwritten.

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

func init() {
	commands.AddCmd("retag", "rename, merge or delete a tag everywhere: retag old new | retag -delete old",
		func() commands.Cmd { return &Retag{} })
	commands.AddCmd("split", "move a heading into a new file, leaving a link: split <heading> [-to path]",
		func() commands.Cmd { return &Split{} })
	commands.AddCmd("merge", "merge one heading into another: merge <into> <from>",
		func() commands.Cmd { return &Merge{} })
	commands.AddCmd("list2headings", "turn a list in a heading's text into child headings: list2headings <heading>",
		func() commands.Cmd { return &ListToHeadings{} })
}

const helpGroup = "Refactoring the outline"

// ---------------------------------------------------------------------------
// Shared: preview, show, ask, apply
// ---------------------------------------------------------------------------

func colourPatch(p string) string {
	if !commands.Colour() {
		return p
	}
	var sb strings.Builder
	for _, l := range strings.SplitAfter(p, "\n") {
		if l == "" {
			continue
		}
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			sb.WriteString(commands.C(commands.AnsiBold) + strings.TrimRight(l, "\n") + commands.C(commands.AnsiReset) + "\n")
		case strings.HasPrefix(l, "@@"):
			sb.WriteString(commands.C(commands.AnsiCyan) + strings.TrimRight(l, "\n") + commands.C(commands.AnsiReset) + "\n")
		case strings.HasPrefix(l, "+"):
			sb.WriteString(commands.C(commands.AnsiGreen) + strings.TrimRight(l, "\n") + commands.C(commands.AnsiReset) + "\n")
		case strings.HasPrefix(l, "-"):
			sb.WriteString(commands.C(commands.AnsiRed) + strings.TrimRight(l, "\n") + commands.C(commands.AnsiReset) + "\n")
		default:
			sb.WriteString(l)
		}
	}
	return sb.String()
}

// show prints a preview: the diff on stdout (it is the answer), what it means
// and what it could not do on stderr.
func show(p common.RefactorPlan) {
	for _, f := range p.Files {
		fmt.Print(colourPatch(f.Patch))
	}
	dim, gold, reset := commands.C(commands.AnsiDim), commands.C(commands.AnsiGold), commands.C(commands.AnsiReset)
	fmt.Fprintf(os.Stderr, "%s%s%s\n", commands.C(commands.AnsiBold), p.Summary, reset)
	for _, w := range p.Warnings {
		fmt.Fprintf(os.Stderr, "%s!%s %s\n", gold, reset, w)
	}
	if len(p.Mentions) > 0 {
		fmt.Fprintf(os.Stderr, "%sNamed here too, and left for you to change:%s\n", dim, reset)
		for _, m := range p.Mentions {
			fmt.Fprintf(os.Stderr, "  %s %s%s%s  %s\n", m.Kind, commands.C(commands.AnsiBold), m.Name, reset, m.Text)
		}
	}
}

// run is every refactor command: preview, show, ask, apply. withToken copies
// what the apply needs out of the preview into the request.
func run[T any](core *commands.Core, op string, req *T, yes bool, withToken func(*T, common.RefactorPlan)) {
	preview, err := common.RestPost[common.RefactorPlan](&core.Rest, "refactor/"+op+"/preview", req)
	if err != nil {
		commands.Fail("orgs %s: %v", op, err)
	}
	if !preview.Ok {
		commands.Fail("%s", preview.Msg)
	}
	if commands.JsonOut && (commands.DryRun || len(preview.Files) == 0) {
		commands.RenderOne(preview, nil)
		return
	}
	if !commands.Machine() {
		show(preview)
	}
	if len(preview.Files) == 0 {
		if !commands.Machine() {
			fmt.Fprintln(os.Stderr, "Nothing to change.")
		}
		return
	}
	if commands.DryRun {
		return
	}
	tf := &commands.TargetFlags{Yes: yes}
	if !commands.Confirm(tf, "Write this to %s?", files(len(preview.Files))) {
		fmt.Fprintln(os.Stderr, "Nothing written.")
		return
	}
	withToken(req, preview)
	done, err := commands.SendReceivePostErr[T, common.RefactorPlan](core, "refactor/"+op+"/apply", req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("orgs %s: %v", op, err)
	}
	if !done.Ok {
		commands.Fail("%s", done.Msg)
	}
	if commands.JsonOut {
		commands.RenderOne(done, nil)
		return
	}
	lines := []string{done.Summary}
	if done.NewFile != "" {
		lines = append(lines, "new file "+done.NewFile)
	}
	names := []string{}
	for _, f := range done.Files {
		names = append(names, commands.BaseName(f.Filename))
	}
	lines = append(lines, "wrote "+strings.Join(names, ", "))
	// One, not the file count: Ok's tally line counts headings.
	commands.Ok(1, "", lines)
}

func files(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

func apply(a *common.RefactorApply, p common.RefactorPlan) {
	a.Token = p.Token
	a.Expect = map[string]string{}
	for _, f := range p.Files {
		a.Expect[f.Filename] = f.Hash
	}
}

// A query has a call or an operator in it; plain words are a heading's text,
// and become a query for a heading called exactly that when there is one, or
// any heading with the words in it. The same reading `orgs mentions` gives.
func asQuery(core *commands.Core, words []string) []string {
	text := strings.TrimSpace(strings.Join(words, " "))
	if text == "" || text == "-" || commands.LooksLikeHash(text) || strings.ContainsAny(text, "()&|!=<>") {
		return words
	}
	pat := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			return r
		}
		return '.'
	}, text)
	exact := fmt.Sprintf(`MatchHeadline("(?i)^ *%s *$")`, pat)
	if found, err := commands.SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": exact}); err == nil && len(found) > 0 {
		return []string{exact}
	}
	return []string{fmt.Sprintf(`MatchHeadline("(?i)%s")`, pat)}
}

func one(core *commands.Core, tf *commands.TargetFlags, words []string, prompt string) common.Todo {
	found := commands.Resolve(core, tf, asQuery(core, words), commands.TargetOpts{Prompt: prompt, Quiet: true})
	if len(found) != 1 {
		commands.Fail("name one heading")
	}
	return found[0]
}

// ---------------------------------------------------------------------------
// orgs retag
// ---------------------------------------------------------------------------

type Retag struct {
	fset   *flag.FlagSet
	Delete bool
	Query  string
	Files  string
	Yes    bool
}

func (self *Retag) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Retag) StartPlugin(manager *common.PluginManager) {}
func (self *Retag) HelpGroup() string                         { return helpGroup }
func (self *Retag) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Delete, "delete", false, "take the tag off everything rather than renaming it")
	fset.StringVar(&self.Query, "query", "", "only the headlines of the headings this query matches")
	fset.StringVar(&self.Files, "file", "", "only these files, comma separated")
	fset.BoolVar(&self.Yes, "yes", false, "do not ask before writing")
}

func (self *Retag) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	req := common.TagRenameRequest{Delete: self.Delete, Query: self.Query}
	switch {
	case self.Delete && len(words) == 1:
		req.Old = words[0]
	case !self.Delete && len(words) == 2:
		req.Old, req.New = words[0], words[1]
	default:
		commands.Fail("orgs retag old new, or orgs retag -delete old")
	}
	for _, f := range strings.Split(self.Files, ",") {
		if f = strings.TrimSpace(f); f != "" {
			req.Files = append(req.Files, f)
		}
	}
	run(core, "tag", &req, self.Yes, func(r *common.TagRenameRequest, p common.RefactorPlan) { apply(&r.RefactorApply, p) })
}

// ---------------------------------------------------------------------------
// orgs split
// ---------------------------------------------------------------------------

type Split struct {
	fset *flag.FlagSet
	tf   commands.TargetFlags
	To   string
}

func (self *Split) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Split) StartPlugin(manager *common.PluginManager) {}
func (self *Split) HelpGroup() string                         { return helpGroup }
func (self *Split) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.To, "to", "", "the new file (relative to the first org directory), or a folder for it; default: beside the heading's file")
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *Split) Exec(core *commands.Core) {
	t := one(core, &self.tf, commands.FreeArgs(self.fset), "Split out> ")
	req := common.SplitRequest{Hash: t.Hash, To: self.To}
	run(core, "split", &req, self.tf.Yes, func(r *common.SplitRequest, p common.RefactorPlan) {
		apply(&r.RefactorApply, p)
		r.NewId = p.NewId
	})
}

// ---------------------------------------------------------------------------
// orgs merge
// ---------------------------------------------------------------------------

type Merge struct {
	fset *flag.FlagSet
	tf   commands.TargetFlags
}

func (self *Merge) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Merge) StartPlugin(manager *common.PluginManager) {}
func (self *Merge) HelpGroup() string                         { return helpGroup }
func (self *Merge) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
}

// Two headings, so each word is one of them: `orgs merge Rocket Booster`, or
// quoted queries. A word left out is asked for with a picker.
func (self *Merge) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	if len(words) > 2 {
		commands.Fail("orgs merge <into> <from> - quote a heading of more than one word")
	}
	pick := func(i int, prompt string) common.Todo {
		tf := commands.TargetFlags{}
		var w []string
		if i < len(words) {
			w = []string{words[i]}
		}
		return one(core, &tf, w, prompt)
	}
	into := pick(0, "Merge into> ")
	from := pick(1, "Merge "+into.Headline+" with> ")
	if into.Hash == from.Hash {
		commands.Fail("those are the same heading")
	}
	req := common.MergeRequest{Into: into.Hash, From: from.Hash}
	run(core, "merge", &req, self.tf.Yes, func(r *common.MergeRequest, p common.RefactorPlan) { apply(&r.RefactorApply, p) })
}

// ---------------------------------------------------------------------------
// orgs list2headings
// ---------------------------------------------------------------------------

type ListToHeadings struct {
	fset *flag.FlagSet
	tf   commands.TargetFlags
	Line int
}

func (self *ListToHeadings) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *ListToHeadings) StartPlugin(manager *common.PluginManager) {}
func (self *ListToHeadings) HelpGroup() string                         { return helpGroup }
func (self *ListToHeadings) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.IntVar(&self.Line, "line", 0, "the list at this line of the file (1 based); default the first list in the heading's text")
	commands.AddTargetFlags(fset, &self.tf)
}

func (self *ListToHeadings) Exec(core *commands.Core) {
	t := one(core, &self.tf, commands.FreeArgs(self.fset), "List to headings in> ")
	line := self.Line - 1
	if self.Line <= 0 {
		// More than one list is a question, when somebody is there to answer.
		lists := commands.SendReceiveGetOr[common.RefactorLists](core, "refactor/lists", map[string]string{"hash": t.Hash})
		if lists.Ok && len(lists.Lists) > 1 && commands.Interactive() && !commands.Machine() {
			for i, l := range lists.Lists {
				fmt.Fprintf(os.Stderr, "  %d) line %d: %d items, starting %q\n", i+1, l.Line+1, l.Items, l.First)
			}
			fmt.Fprintf(os.Stderr, "Which list? [1] ")
			var n int
			if _, err := fmt.Fscanln(os.Stdin, &n); err != nil || n < 1 || n > len(lists.Lists) {
				n = 1
			}
			line = lists.Lists[n-1].Line
		}
	}
	req := common.ListToHeadingsRequest{Hash: t.Hash, Line: line}
	run(core, "list", &req, self.tf.Yes, func(r *common.ListToHeadingsRequest, p common.RefactorPlan) { apply(&r.RefactorApply, p) })
}

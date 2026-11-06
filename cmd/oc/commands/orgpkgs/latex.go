package orgpkgs

// Latex Suite (features.latexSuite, cdlatex) from the command line.
//
//	orgs latex snippets                 every snippet, by group (words filter: orgs latex snippets greek)
//	orgs latex snippets -builtin        the built-in set alone
//	orgs latex expand 'x + @a'          what typing that becomes ($1… tab stops; -plain without them)
//	orgs latex expand -mode text 'see mk'   the mode is read from the text's delimiters unless named
//	orgs latex expand -tab 'x int'      a TAB snippet as well as the as-you-type ones
//	orgs latex check notes.org          unbalanced braces, \left without \right, unclosed maths
//	orgs latex check notes.org -format quickfix   file:line:col: lines for an editor (-check: exit 1)
//	orgs latex init                     write latex-snippets.org to make your own
//
// Needs `orgs features latexSuite on`; `expand` falls back to the built-in
// set when no server answers, so an editor can use it offline.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Latex struct {
	base
	Builtin bool
	Mode    string
	Tab     bool
	Plain   bool
	Check   bool
}

func (self *Latex) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Builtin, "builtin", false, "snippets: the built-in set alone")
	fset.StringVar(&self.Mode, "mode", "", "expand: text, inline or display (default: read from the text)")
	fset.BoolVar(&self.Tab, "tab", false, "expand: try TAB snippets too")
	fset.BoolVar(&self.Plain, "plain", false, "expand: print the text with its tab stops taken out")
	fset.BoolVar(&self.Check, "check", false, "check: exit 1 when there is a problem")
}

type latexSnippetsAnswer struct {
	Ok       bool
	Off      bool
	Msg      string
	File     string
	Exists   bool
	Snippets []common.LatexSnippet
	Problems []common.LatexTableProblem
}

type latexCheckAnswer struct {
	Ok       bool
	Off      bool
	Msg      string
	File     string
	Problems []common.LatexProblem
}

type latexInitAnswer struct {
	Ok   bool
	Off  bool
	Msg  string
	File string
}

func (self *Latex) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := "snippets"
	if len(words) > 0 {
		switch words[0] {
		case "snippets", "ls", "list", "expand", "check", "init":
			verb, words = words[0], words[1:]
		}
	}
	switch verb {
	case "expand":
		self.expand(core, words)
	case "check":
		self.check(core, words)
	case "init":
		self.init(core)
	default:
		self.list(core, words)
	}
}

func (self *Latex) fetch(core *commands.Core) latexSnippetsAnswer {
	q := map[string]string{}
	if self.Builtin {
		q["builtin"] = "1"
	}
	res, err := commands.SendReceiveGetErr[latexSnippetsAnswer](core, "latex/snippets", q)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
	return res
}

// latexOptionWords says a snippet's options in words.
func latexOptionWords(o string) string {
	w := []string{}
	switch {
	case strings.Contains(o, "t"):
		w = append(w, "text")
	case strings.Contains(o, "m"):
		w = append(w, "maths")
	case strings.Contains(o, "n"):
		w = append(w, "inline")
	case strings.Contains(o, "M"):
		w = append(w, "display")
	}
	if strings.Contains(o, "v") {
		w = append(w, "selection")
	} else if strings.Contains(o, "A") {
		w = append(w, "auto")
	} else {
		w = append(w, "tab")
	}
	if strings.Contains(o, "r") {
		w = append(w, "regex")
	}
	return strings.Join(w, " ")
}

func (self *Latex) list(core *commands.Core, words []string) {
	res := self.fetch(core)
	snips := res.Snippets
	if len(words) > 0 {
		kept := []common.LatexSnippet{}
		for _, s := range snips {
			hay := strings.ToLower(s.Trigger + " " + s.Replacement + " " + s.Description + " " + s.Group)
			all := true
			for _, w := range words {
				if !strings.Contains(hay, strings.ToLower(w)) {
					all = false
				}
			}
			if all {
				kept = append(kept, s)
			}
		}
		snips = kept
	}
	commands.Render(snips, func() {
		// The trigger column as wide as the widest trigger shown, so a regex
		// trigger does not push its replacement out of line.
		tw := 10
		for _, s := range snips {
			if n := utf8.RuneCountInString(s.Trigger); n > tw {
				tw = n
			}
		}
		group := "\x00"
		for _, s := range snips {
			g := s.Group
			if s.File != "" {
				g = filepath.Base(s.File) + ": " + g
			}
			if g != group {
				if group != "\x00" {
					fmt.Println()
				}
				group = g
				fmt.Println(commands.C(commands.AnsiBold) + g + commands.C(commands.AnsiReset))
			}
			rep := strings.ReplaceAll(s.Replacement, "\n", "⏎")
			fmt.Printf("  %-*s %-40s %s\n", tw, s.Trigger, rep, dim(latexOptionWords(s.Options)+"  "+s.Description))
		}
		for _, p := range res.Problems {
			fmt.Fprintf(os.Stderr, "%s:%d: %s\n", p.File, p.Line, p.Msg)
		}
		if !self.Builtin {
			if res.Exists {
				fmt.Fprintln(os.Stderr, dim("your snippets: "+res.File))
			} else if res.File != "" {
				fmt.Fprintln(os.Stderr, dim("no "+res.File+" yet: orgs latex init writes one to edit"))
			}
		}
	})
}

type latexExpandOut struct {
	Found   bool
	Mode    string
	Snippet *common.LatexSnippet `json:",omitempty"`
	Remove  int
	Body    string
	Text    string
	Stops   []common.LatexStop
}

func (self *Latex) expand(core *commands.Core, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs latex expand 'text before the cursor'")
	}
	text := strings.Join(words, " ")
	var snips []common.LatexSnippet
	res, err := commands.SendReceiveGetErr[latexSnippetsAnswer](core, "latex/snippets", nil)
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, dim("no server answered: the built-in snippets"))
		snips = common.LatexBuiltinSnippets()
	case !res.Ok:
		commands.Fail("%s", res.Msg)
	default:
		snips = res.Snippets
	}
	mode := self.Mode
	if mode == "" {
		mode = common.LatexModeAt(text, len(text)).Mode
	}
	out := latexExpandOut{Mode: mode, Stops: []common.LatexStop{}}
	e := common.LatexExpand(snips, text, mode, true)
	if e == nil && self.Tab {
		e = common.LatexExpand(snips, text, mode, false)
	}
	if e == nil && mode != "text" {
		e = common.LatexAutoFraction(text)
	}
	if e != nil {
		p := common.LatexPlace(e.Body)
		s := e.Snippet
		out = latexExpandOut{true, mode, &s, e.Remove, e.Body, p.Text, p.Stops}
	}
	commands.RenderOne(out, func() {
		if !out.Found {
			fmt.Fprintf(os.Stderr, "nothing expands there (%s mode)\n", mode)
			os.Exit(1)
		}
		if self.Plain {
			fmt.Println(text[:len(text)-out.Remove] + out.Text)
		} else {
			fmt.Println(out.Body)
		}
		fmt.Fprintln(os.Stderr, dim(fmt.Sprintf("%s → %s  (%s mode, takes %d before the cursor)", out.Snippet.Trigger, strings.ReplaceAll(out.Text, "\n", "⏎"), mode, out.Remove)))
	})
}

func (self *Latex) check(core *commands.Core, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs latex check FILE...")
	}
	diags := []commands.Diagnostic{}
	for _, w := range words {
		f := commands.ResolveOrgFile(core, w)
		req := struct{ File string }{f}
		res, err := commands.SendReceivePostErr[struct{ File string }, latexCheckAnswer](core, "latex/check", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
		for _, p := range res.Problems {
			diags = append(diags, commands.Diagnostic{File: res.File, Line: p.Line, Col: p.Col, Severity: p.Severity, Rule: p.Rule, Msg: p.Msg})
		}
	}
	commands.RenderDiagnostics(diags, func() {
		if len(diags) == 0 {
			fmt.Fprintln(os.Stderr, "the maths reads well")
			return
		}
		commands.PrintDiagnostics(diags)
	})
	if self.Check && len(diags) > 0 {
		os.Exit(1)
	}
}

func (self *Latex) init(core *commands.Core) {
	req := struct{}{}
	res, err := commands.SendReceivePostErr[struct{}, latexInitAnswer](core, "latex/snippets/init", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
	commands.RenderOne(res, func() { fmt.Println(res.Msg) })
}

func init() {
	commands.AddCmd("latex", "Quick maths typing (cdlatex): list the snippets, expand a trigger, check a file's maths",
		func() commands.Cmd { return &Latex{} })
}

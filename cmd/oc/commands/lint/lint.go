package lint

// `orgs lint`: worg writing mode's spelling and grammar checkers, as compiler
// output.
//
//	orgs lint chapter1.org              a file
//	orgs lint 'MatchHeadline("Draft")'  a heading and what is under it
//	orgs lint -                         text on stdin (or hashes, one a line)
//	orgs lint notes.org -format quickfix   file:line:col: lines for an editor
//	orgs lint *.org -check              for a hook: exit 1 on any problem
//
// Both checkers run on the server - Harper as harper-ls (`harper: enabled:
// true`), LanguageTool as whatever `languageTool:` points at - so the terminal
// and the browser give the same answers from the same rules. Only prose is
// sent (prose.go), and every problem comes back as a commands.Diagnostic, so
// `-format quickfix` is the same line `review`, `deps -check` and `fmt -check`
// print.
//
// Columns are 1 based **bytes**, which is what vim's quickfix (`%c`) and
// Emacs' compile mode count; a line with no accents is the same either way.

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Lint struct {
	File    string
	With    string
	Lang    string
	Dialect string
	Ignore  string
	Check   bool
	Target  commands.TargetFlags
	fset    *flag.FlagSet
}

func (self *Lint) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Lint) StartPlugin(manager *common.PluginManager)       {}
func (self *Lint) HelpGroup() string                                 { return "Writing" }

func (self *Lint) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.File, "file", "", "the org file to check")
	fset.StringVar(&self.With, "with", "", "harper, languagetool or all (default: the first the server offers)")
	fset.StringVar(&self.Lang, "lang", "en-US", "LanguageTool's language: en-US, en-GB, de-DE, auto ...")
	fset.StringVar(&self.Dialect, "dialect", "american", "Harper's English: american, british, australian, canadian, indian")
	fset.StringVar(&self.Ignore, "ignore", "", "rules to leave out, comma separated (either checker's names)")
	fset.BoolVar(&self.Check, "check", false, "exit 1 when anything is found, for a hook")
	commands.AddTargetFlags(fset, &self.Target)
}

// job is one stretch of one file to read: the whole file, or a heading's
// subtree.
type job struct {
	file     string
	lines    []string
	from, to int
}

func (self *Lint) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	jobs := self.jobs(core, words)

	ignore := map[string]bool{}
	off := []string{}
	for _, r := range strings.Split(self.Ignore, ",") {
		if r = strings.TrimSpace(r); r != "" {
			ignore[strings.ToLower(r)] = true
			off = append(off, r)
		}
	}
	checkers := self.checkers(core)

	diags := []commands.Diagnostic{}
	for _, j := range jobs {
		paras := paragraphs(j.lines, j.from, j.to)
		if len(paras) == 0 {
			continue
		}
		for _, name := range checkers {
			var res [][]found
			var err error
			switch name {
			case "harper":
				res, err = runHarper(core, paras, self.Dialect, off)
			case "languagetool":
				res, err = runLanguageTool(core, paras, self.Lang, off)
			}
			if err != nil {
				commands.Fail("%s: %v", j.file, err)
			}
			for _, fs := range res {
				for _, f := range fs {
					if ignore[strings.ToLower(f.rule)] {
						continue
					}
					diags = append(diags, commands.Diagnostic{File: j.file, Line: f.at.line + 1, Col: f.at.col + 1,
						Severity: f.severity, Rule: f.rule, Msg: f.msg, Fix: f.fix})
				}
			}
		}
	}
	diags = tidy(diags)

	commands.RenderDiagnostics(diags, func() {
		if len(diags) == 0 {
			what := "the text"
			if len(jobs) == 1 {
				what = jobs[0].file
			} else if len(jobs) > 1 {
				what = fmt.Sprintf("%d places", len(jobs))
			}
			fmt.Printf("%s✓ no problems in %s%s\n", commands.C(commands.AnsiGreen), what, commands.C(commands.AnsiReset))
			return
		}
		commands.PrintDiagnostics(diags)
		files := map[string]bool{}
		for _, d := range diags {
			files[d.File] = true
		}
		fmt.Fprintf(os.Stderr, "%d problem%s in %d file%s (%s)\n", len(diags), plural(len(diags)),
			len(files), plural(len(files)), strings.Join(checkers, ", "))
	})
	if self.Check && len(diags) > 0 {
		os.Exit(1)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// tidy sorts by place, drops a problem reported twice, and with both checkers
// on drops the second report of a problem both found: the same place with the same first suggestion is a
// misspelling that both of them know.
func tidy(diags []commands.Diagnostic) []commands.Diagnostic {
	sort.SliceStable(diags, func(a, b int) bool {
		x, y := diags[a], diags[b]
		if x.File != y.File {
			return x.File < y.File
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		return x.Col < y.Col
	})
	seen := map[string]bool{}
	out := []commands.Diagnostic{}
	for _, d := range diags {
		// A heading named twice - itself and its parent - reads the same
		// lines twice.
		same := fmt.Sprintf("%s:%d:%d:%s", d.File, d.Line, d.Col, d.Rule)
		if seen[same] {
			continue
		}
		seen[same] = true
		if len(d.Fix) > 0 {
			k := fmt.Sprintf("%s:%d:%d:%s", d.File, d.Line, d.Col, strings.ToLower(d.Fix[0]))
			if seen[k] {
				continue
			}
			seen[k] = true
		}
		out = append(out, d)
	}
	return out
}

// checkers asks the server which of the two it offers, and settles which to
// run. Naming one it does not offer is an error that says why; asking for
// neither runs Harper when it is there, as worg does, and LanguageTool when
// only that is.
func (self *Lint) checkers(core *commands.Core) []string {
	hs, herr := commands.SendReceiveGetErr[harperStatus](core, "harper/status", nil)
	ls, lerr := commands.SendReceiveGetErr[ltStatus](core, "languagetool/status", nil)
	harperOk := herr == nil && hs.Enabled
	ltOk := lerr == nil && ls.Ok && ls.Running
	harperWhy := "Harper is not set up on this server: add `harper: {enabled: true}` under server: in orgs.yaml"
	if herr != nil {
		harperWhy = "could not ask the server about Harper: " + herr.Error()
	}
	ltWhy := "LanguageTool is not set up on this server: add `languageTool: {enabled: true}` (or url:) under server: in orgs.yaml"
	if lerr != nil {
		ltWhy = "could not ask the server about LanguageTool: " + lerr.Error()
	} else if ls.Ok && !ls.Running {
		ltWhy = "LanguageTool is configured but not answering: " + ls.Msg
	}

	switch strings.ToLower(self.With) {
	case "harper":
		if !harperOk {
			commands.Fail("%s", harperWhy)
		}
		return []string{"harper"}
	case "languagetool", "lt":
		if !ltOk {
			commands.Fail("%s", ltWhy)
		}
		return []string{"languagetool"}
	case "all", "both":
		if !harperOk && !ltOk {
			commands.Fail("neither checker is available.\n  %s\n  %s", harperWhy, ltWhy)
		}
		out := []string{}
		if harperOk {
			out = append(out, "harper")
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", harperWhy)
		}
		if ltOk {
			out = append(out, "languagetool")
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", ltWhy)
		}
		return out
	case "":
		if harperOk {
			return []string{"harper"}
		}
		if ltOk {
			return []string{"languagetool"}
		}
		commands.Fail("no checker to run.\n  %s\n  %s", harperWhy, ltWhy)
	default:
		commands.Fail("-with %s: say harper, languagetool or all", self.With)
	}
	return nil
}

// jobs works out what to read from the words on the line.
//
// A word that is a file is a file. `-` - or nothing, with stdin a pipe - is
// stdin: hashes when every line of it is one (`orgs search … -json | jq -r
// .Hash | orgs lint`), text to check otherwise. Anything else goes to
// commands.Resolve as a heading, the same as every write verb.
func (self *Lint) jobs(core *commands.Core, words []string) []job {
	files := []string{}
	if self.File != "" {
		files = append(files, self.File)
	}
	rest := []string{}
	for _, w := range words {
		if strings.HasSuffix(strings.ToLower(w), ".org") || isFile(w) {
			files = append(files, w)
			continue
		}
		rest = append(rest, w)
	}
	jobs := []job{}
	for _, f := range files {
		lines := fileLines(core, f)
		jobs = append(jobs, job{file: f, lines: lines, from: 0, to: len(lines)})
	}
	if len(files) > 0 && len(rest) == 0 {
		return jobs
	}

	dash := len(rest) == 1 && rest[0] == "-"
	if dash || (len(rest) == 0 && self.Target.Hash == "" && commands.StdinIsPipe()) {
		text := readStdin()
		var hashes []string
		for _, l := range strings.Split(text, "\n") {
			if l = strings.TrimSpace(l); l == "" {
				continue
			}
			if !commands.LooksLikeHash(l) {
				hashes = nil
				break
			}
			hashes = append(hashes, l)
		}
		if len(hashes) == 0 {
			lines := strings.Split(text, "\n")
			return append(jobs, job{file: "<stdin>", lines: lines, from: 0, to: len(lines)})
		}
		for _, h := range hashes {
			jobs = append(jobs, self.headingJob(core, commands.TodoByHash(core, h)))
		}
		return jobs
	}

	todos := commands.Resolve(core, &self.Target, rest, commands.TargetOpts{
		Prompt: "Check> ", Multi: true, Quiet: true})
	for _, t := range todos {
		jobs = append(jobs, self.headingJob(core, t))
	}
	return jobs
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func readStdin() string {
	var b strings.Builder
	r := bufio.NewReader(os.Stdin)
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return b.String()
}

// headingJob is a heading's subtree: from its line (0 based, as the server
// counts) to the next heading at its level or above.
func (self *Lint) headingJob(core *commands.Core, t common.Todo) job {
	if t.Filename == "" {
		commands.Fail("no such heading")
	}
	lines := fileLines(core, t.Filename)
	from := t.LineNum
	if from < 0 || from >= len(lines) {
		commands.Fail("%s: line %d is past the end of the file; the server may be behind it", t.Filename, from+1)
	}
	level := stars(lines[from])
	to := len(lines)
	for i := from + 1; i < len(lines); i++ {
		if n := stars(lines[i]); n > 0 && (level == 0 || n <= level) {
			to = i
			break
		}
	}
	return job{file: t.Filename, lines: lines, from: from, to: to}
}

func stars(l string) int {
	n := 0
	for n < len(l) && l[n] == '*' {
		n++
	}
	if n > 0 && n < len(l) && (l[n] == ' ' || l[n] == '\t') {
		return n
	}
	return 0
}

// fileLines is a file's text: read here when it is on this disk, which is
// what lines an editor up with the problems, and asked of the server when it
// is not.
func fileLines(core *commands.Core, name string) []string {
	if b, err := os.ReadFile(name); err == nil {
		return strings.Split(string(b), "\n")
	}
	if !filepath.IsAbs(name) {
		if found := commands.SendReceiveGetOr[common.ResultMsg](core, "findfile", map[string]string{"filename": name}); found.Ok && found.Msg != "" {
			name = found.Msg
		}
	}
	res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "orgfile", map[string]string{"filename": name})
	if err != nil {
		commands.Fail("%s: %v", name, err)
	}
	if !res.Ok {
		commands.Fail("%s: %s", name, res.Msg)
	}
	return strings.Split(res.Msg, "\n")
}

func init() {
	commands.AddCmd("lint", "spelling and grammar over a file's prose (Harper, LanguageTool), as file:line:col problems",
		func() commands.Cmd { return &Lint{} })
}

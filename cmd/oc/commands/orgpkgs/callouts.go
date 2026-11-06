package orgpkgs

// Callouts (org special blocks drawn as callouts), from the command line.
//
//	orgs callouts                       every callout in every file
//	orgs callouts ls notes.org          one file's, by line and heading
//	orgs callouts types                 the types, their aliases and colours
//	orgs callouts check notes.org       warn about callouts of a type no row names (-check: exit 1)
//	orgs callouts ls notes.org -format quickfix   file:line: lines for an editor
//
// Needs `orgs features callouts on`. `orgs export -f txt` shows them as
// barred paragraphs, `-f md` writes them back as `> [!type] Title`.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Callouts struct {
	base
	Check bool
}

func (self *Callouts) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Check, "check", false, "with check: exit 1 when a callout has an unknown type")
}

type calloutAt struct {
	common.Callout
	Heading string
	Shown   string
}

type calloutFile struct {
	File     string
	Callouts []calloutAt
}

type calloutsAnswer struct {
	Ok       bool
	Off      bool
	Msg      string
	File     string
	Callouts []calloutAt
	Files    []calloutFile
}

type calloutTypes struct {
	Ok    bool
	Off   bool
	Msg   string
	Types []common.CalloutType
}

// calloutInk is the type's colour as a terminal colour, or "".
func calloutInk(hex string) string {
	if !commands.Colour() || len(hex) != 7 || hex[0] != '#' {
		return ""
	}
	r, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b, _ := strconv.ParseUint(hex[5:7], 16, 8)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

func (self *Callouts) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := "ls"
	if len(words) > 0 {
		switch words[0] {
		case "ls", "list", "types", "check":
			verb, words = words[0], words[1:]
		}
	}
	switch verb {
	case "types":
		self.types(core)
	case "check":
		self.check(core, words)
	default:
		self.list(core, words)
	}
}

func (self *Callouts) types(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[calloutTypes](core, "callouts/types", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
	commands.Render(res.Types, func() {
		for _, t := range res.Types {
			ink := calloutInk(t.Color)
			reset := ""
			if ink != "" {
				reset = commands.C(commands.AnsiReset)
			}
			al := ""
			if len(t.Aliases) > 0 {
				al = dim("  " + strings.Join(t.Aliases, ", "))
			}
			fmt.Printf("%s%s %-9s%s %s %s%s\n", ink, t.Glyph, t.Name, reset, dim(t.Color), dim(t.Beamer), al)
		}
		fmt.Fprintln(os.Stderr, dim("#+begin_<type> Title :fold open|closed :icon x :color #hex  …  #+end_<type>"))
	})
}

// fetch is the callouts of the files named, or of every file.
func (self *Callouts) fetch(core *commands.Core, words []string) []calloutFile {
	if len(words) == 0 {
		res, err := commands.SendReceiveGetErr[calloutsAnswer](core, "callouts", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
		return res.Files
	}
	out := []calloutFile{}
	for _, w := range words {
		f := commands.ResolveOrgFile(core, w)
		res, err := commands.SendReceiveGetErr[calloutsAnswer](core, "callouts", map[string]string{"file": f})
		if err != nil {
			commands.Fail("%v", err)
		}
		failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
		out = append(out, calloutFile{File: res.File, Callouts: res.Callouts})
	}
	return out
}

func (self *Callouts) list(core *commands.Core, words []string) {
	files := self.fetch(core, words)
	if commands.QuickfixFormat() {
		diags := []commands.Diagnostic{}
		for _, f := range files {
			for _, c := range f.Callouts {
				diags = append(diags, commands.Diagnostic{File: f.File, Line: c.Line, Severity: "info", Rule: c.Type, Msg: c.Shown})
			}
		}
		commands.RenderDiagnostics(diags, nil)
		return
	}
	commands.Render(files, func() {
		for i, f := range files {
			if len(files) > 1 || len(words) == 0 {
				if i > 0 {
					fmt.Println()
				}
				fmt.Println(commands.C(commands.AnsiBold) + filepath.Base(f.File) + commands.C(commands.AnsiReset))
			}
			if len(f.Callouts) == 0 {
				fmt.Println(dim("  no callouts"))
			}
			heading := "\x00"
			for _, c := range f.Callouts {
				if c.Heading != heading {
					heading = c.Heading
					h := c.Heading
					if h == "" {
						h = "(above the first heading)"
					}
					fmt.Println("  " + dim(h))
				}
				t := c.TypeInfo()
				ink := calloutInk(c.ShownColor())
				reset := ""
				if ink != "" {
					reset = commands.C(commands.AnsiReset)
				}
				fold := ""
				switch c.Fold {
				case "closed":
					fold = dim(" ▸")
				case "open":
					fold = dim(" ▾")
				}
				kind := t.Name
				if c.Unknown != "" {
					kind = c.Unknown + "?"
				}
				fmt.Printf("  %s%5d%s  %s%s %-9s%s %s%s\n", commands.C(commands.AnsiDim), c.Line, commands.C(commands.AnsiReset),
					strings.Repeat("  ", c.Depth), ink+c.ShownGlyph(), kind, reset, c.Shown, fold)
			}
		}
	})
}

func (self *Callouts) check(core *commands.Core, words []string) {
	files := self.fetch(core, words)
	diags := []commands.Diagnostic{}
	for _, f := range files {
		for _, c := range f.Callouts {
			if c.Unknown != "" {
				diags = append(diags, commands.Diagnostic{File: f.File, Line: c.Line, Severity: "warning", Rule: "callout-type",
					Msg: fmt.Sprintf("no callout type %q (drawn as a note): orgs callouts types lists them", c.Unknown)})
			}
		}
	}
	commands.RenderDiagnostics(diags, func() {
		if len(diags) == 0 {
			fmt.Fprintln(os.Stderr, "every callout is of a known type")
			return
		}
		commands.PrintDiagnostics(diags)
	})
	if self.Check && len(diags) > 0 {
		os.Exit(1)
	}
}

func init() {
	commands.AddCmd("callouts", "callouts as org special blocks (#+begin_warning Title :fold closed): list them, the types, check them",
		func() commands.Cmd { return &Callouts{} })
}

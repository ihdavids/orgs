package orgpkgs

// Tables of contents (toc-org, org-make-toc), from the command line.
//
//	orgs toc                         bring every file's tables of contents up to date
//	orgs toc notes.org other.org     just these
//	orgs toc -check                  exit 1 if any is stale (a git hook, CI; works with -local)
//	orgs toc -print notes.org        what the file's tables should say
//	orgs toc -insert notes.org       add a `* Table of Contents :TOC:` heading
//	orgs toc -insert notes.org -make-toc -line 12   org-make-toc's drawer at the heading on line 12
//
// Needs `orgs features toc on`.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type Toc struct {
	base
	Check   bool
	Print   bool
	Insert  bool
	MakeToc bool
	Line    int
}

func (self *Toc) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.BoolVar(&self.Check, "check", false, "write nothing; say which tables are stale and exit 1 if any is")
	fset.BoolVar(&self.Print, "print", false, "print what each table of contents in the file should say")
	fset.BoolVar(&self.Insert, "insert", false, "add a table of contents to the file")
	fset.BoolVar(&self.MakeToc, "make-toc", false, "with -insert: org-make-toc's :TOC: property and :CONTENTS: drawer instead of a :TOC: tag")
	fset.IntVar(&self.Line, "line", 0, "with -insert -make-toc: the heading on this line (from 1) gets it; 0 = a new Contents heading")
}

type tocSite struct {
	Kind    string
	Heading string
	Row     int
	From    int
	To      int
	Text    string
	Entries int
	Changed bool
}

type tocFile struct {
	File  string
	Ok    bool
	Msg   string
	Sites []tocSite
	Stale bool
	Wrote bool
	Row   int
}

type tocUpdated struct {
	Ok    bool
	Off   bool
	Msg   string
	Stale int
	Wrote int
	Files []tocFile
}

func tocWhere(s tocSite) string {
	h := s.Heading
	if h == "" {
		h = "(above the first heading)"
	}
	return fmt.Sprintf("line %d  %s  %s", s.Row+1, s.Kind, h)
}

func (self *Toc) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	files := []string{}
	for _, w := range words {
		files = append(files, commands.ResolveOrgFile(core, w))
	}
	switch {
	case self.Print:
		self.print(core, files)
	case self.Insert:
		self.insert(core, files)
	default:
		self.update(core, files)
	}
}

func (self *Toc) print(core *commands.Core, files []string) {
	if len(files) != 1 {
		commands.Fail("orgs toc -print FILE")
	}
	res, err := commands.SendReceiveGetErr[tocFile](core, "toc", map[string]string{"filename": files[0]})
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		if len(res.Sites) == 0 {
			fmt.Fprintf(os.Stderr, "%s has no table of contents: orgs toc -insert %s\n", filepath.Base(res.File), filepath.Base(res.File))
			return
		}
		for i, s := range res.Sites {
			if len(res.Sites) > 1 {
				if i > 0 {
					fmt.Println()
				}
				state := ""
				if s.Changed {
					state = commands.C(commands.AnsiGold) + "  stale" + commands.C(commands.AnsiReset)
				}
				fmt.Println(dim(tocWhere(s)) + state)
			}
			if s.Text != "" {
				fmt.Println(strings.TrimRight(s.Text, "\n"))
			}
		}
	})
}

func (self *Toc) insert(core *commands.Core, files []string) {
	if len(files) != 1 {
		commands.Fail("orgs toc -insert FILE [-make-toc [-line N]]")
	}
	req := struct {
		Filename string
		Kind     string
		Row      int
	}{files[0], "toc-org", -1}
	if self.MakeToc {
		req.Kind = "org-make-toc"
		req.Row = self.Line - 1
	}
	res, err := commands.SendReceivePostErr[struct {
		Filename string
		Kind     string
		Row      int
	}, tocFile](core, "toc/insert", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		if res.Wrote {
			fmt.Printf("%s: table of contents at line %d\n", filepath.Base(res.File), res.Row+1)
		} else {
			fmt.Printf("%s already has one, at line %d\n", filepath.Base(res.File), res.Row+1)
		}
	})
}

func (self *Toc) update(core *commands.Core, files []string) {
	req := struct {
		Files []string
		Check bool
	}{files, self.Check}
	res, err := commands.SendReceivePostErr[struct {
		Files []string
		Check bool
	}, tocUpdated](core, "toc/update", &req)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil {
		commands.Fail("%v", err)
	}
	if res.Off || (!res.Ok && len(res.Files) == 0) {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		for _, f := range res.Files {
			name := filepath.Base(f.File)
			switch {
			case !f.Ok:
				fmt.Fprintf(os.Stderr, "%s: %s\n", name, f.Msg)
			case f.Wrote:
				fmt.Printf("%s: brought up to date\n", name)
			case f.Stale:
				for _, s := range f.Sites {
					if s.Changed {
						fmt.Printf("%s: stale  %s\n", name, dim(tocWhere(s)))
					}
				}
			case len(files) > 0:
				if len(f.Sites) == 0 {
					fmt.Printf("%s: %s\n", name, dim("no table of contents"))
				} else {
					fmt.Printf("%s: %s\n", name, dim("up to date"))
				}
			}
		}
		if len(res.Files) == 0 {
			fmt.Println(dim("no file has a table of contents"))
		} else if res.Stale == 0 && len(files) == 0 {
			fmt.Println(dim(fmt.Sprintf("%d file%s, every table of contents up to date", len(res.Files), map[bool]string{true: "", false: "s"}[len(res.Files) == 1])))
		}
	})
	if self.Check && res.Stale > 0 {
		os.Exit(1)
	}
	if !res.Ok {
		os.Exit(1)
	}
}

func init() {
	commands.AddCmd("toc", "keep tables of contents current: :TOC: headings, :CONTENTS: drawers, #+BEGIN: toc (toc-org, org-make-toc)",
		func() commands.Cmd { return &Toc{} })
}

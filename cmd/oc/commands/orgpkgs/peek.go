package orgpkgs

// Hover previews (features.hoverEditor), from the command line: what a link
// points at, without going there.
//
//	orgs peek 'id:6a1e…'                    the heading with that id
//	orgs peek '[[*Budget]]' -from plan.org  a heading of plan.org
//	orgs peek file:notes.org::#setup        by CUSTOM_ID; ::*Title, ::42 and a whole file work too
//	orgs peek https://example.com           a web page's title and description
//	orgs peek id:6a1e… -edit                open it in $EDITOR at its line
//
// Needs `orgs features hoverEditor on`.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/orghl"
	"github.com/ihdavids/orgs/internal/common"
)

type Peek struct {
	base
	From  string
	Edit  bool
	Lines int
}

func (self *Peek) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.From, "from", "", "the org file the link is written in (relative links and [[*Title]] are read against it)")
	fset.BoolVar(&self.Edit, "edit", false, "open the target in the editor at its line instead of printing it")
	fset.IntVar(&self.Lines, "lines", 0, "print at most this many lines of the text (0 = all)")
}

// peekAnswer is GET /preview's answer (internal/app/orgs/preview.go).
type peekAnswer struct {
	Ok          bool
	Off         bool
	Msg         string
	Link        string
	Kind        string
	Title       string
	Todo        string
	Tags        []string
	Path        []string
	Text        string
	File        string
	From        int
	To          int
	Line        int
	Base        string
	Editable    bool
	Truncated   bool
	Hash        string
	Lang        string
	Url         string
	Description string
	Image       string
	Site        string
	CommandLine string
}

func (self *Peek) Exec(core *commands.Core) {
	link := strings.TrimSpace(strings.Join(commands.FreeArgs(self.fset), " "))
	if link == "" {
		commands.Fail("orgs peek LINK [-from FILE] [-edit]")
	}
	from := self.From
	if from != "" {
		if abs, err := filepath.Abs(from); err == nil {
			if _, serr := os.Stat(abs); serr == nil {
				from = abs
			}
		}
	}
	res, err := commands.SendReceiveGetErr[peekAnswer](core, "preview", map[string]string{"link": link, "from": from})
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	if self.Edit {
		if res.File == "" {
			commands.Fail("%s is not in a file (%s)", link, res.Kind)
		}
		core.LaunchEditor(res.File, res.Line)
		return
	}
	commands.RenderOne(res, func() { self.print(core, res) })
}

func (self *Peek) print(core *commands.Core, p peekAnswer) {
	c := commands.C
	head := c(commands.AnsiBold) + p.Title + c(commands.AnsiReset)
	if p.Todo != "" {
		head = c(commands.AnsiCyan) + p.Todo + c(commands.AnsiReset) + " " + head
	}
	if len(p.Tags) > 0 {
		head += "  " + dim(":"+strings.Join(p.Tags, ":")+":")
	}
	fmt.Println(head)
	where := p.Kind
	switch {
	case p.File != "":
		where = fmt.Sprintf("%s:%d", p.File, p.Line)
		if len(p.Path) > 0 {
			where += "  " + strings.Join(p.Path, " / ")
		}
	case p.Url != "":
		where = p.Url
		if p.Site != "" && !strings.Contains(p.Url, p.Site) {
			where = p.Site + "  " + p.Url
		}
	}
	fmt.Println(dim(where))
	switch p.Kind {
	case "web":
		if p.Description != "" {
			fmt.Println()
			fmt.Println(p.Description)
		}
		if p.Image != "" {
			fmt.Println(dim("picture: " + p.Image))
		}
	case "command":
		fmt.Println(p.CommandLine)
	case "image", "pdf", "media":
		// The url is all a terminal can show.
	default:
		fmt.Println()
		lines := strings.Split(p.Text, "\n")
		if self.Lines > 0 && len(lines) > self.Lines {
			lines = append(lines[:self.Lines], "…")
		}
		if p.Kind == "source" {
			fmt.Println(strings.Join(lines, "\n"))
			break
		}
		st := commands.SendReceiveGetOr[common.TodoStatesResult](core, "status", nil)
		hl := orghl.State{Active: st.Active, Done: st.Done}
		fmt.Println(orghl.ANSIText(strings.Join(lines, "\n"), &hl, commands.Colour()))
	}
	if p.Truncated {
		fmt.Println(dim("(cut short; orgs peek -edit opens all of it)"))
	}
	if p.Msg != "" {
		fmt.Fprintln(os.Stderr, p.Msg)
	}
}

func init() {
	commands.AddCmd("peek", "show what a link points at - a heading, a file, a web page - without going there (features.hoverEditor)",
		func() commands.Cmd { return &Peek{} })
}

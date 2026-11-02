package transclude

// `orgs transclude`: turning transclusion on and off, and what a file
// transcludes.
//
//	orgs transclude            whether it is on
//	orgs transclude on         on, and written into the server's yaml
//	orgs transclude off
//	orgs transclude ls notes.org   each #+transclude: line and where its text comes from
//
// The expansion itself is the server's (transclude.go): every export, worg's
// rendered view and the VS Code preview draw a #+transclude: line as what it
// names once this is on.

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Transclude struct {
	fset *flag.FlagSet
}

func (self *Transclude) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Transclude) StartPlugin(manager *common.PluginManager)       {}
func (self *Transclude) SetupParameters(fset *flag.FlagSet)              { self.fset = fset }
func (self *Transclude) HelpGroup() string                                { return "Files" }

type state struct {
	Ok     bool
	Msg    string
	Enable bool
	Depth  int
}

type transclusion struct {
	Line  int
	Link  string
	Opts  string
	File  string
	From  int
	To    int
	Title string
	Error string
}

func (self *Transclude) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "", "status":
		s, err := commands.SendReceiveGetErr[state](core, "transclusion", nil)
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.RenderOne(s, func() { say(s) })
	case "on", "off":
		s, err := commands.SendReceivePostErr[struct{ Enable bool }, state](core, "transclusion", &struct{ Enable bool }{verb == "on"})
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if !s.Ok {
			commands.Fail("%s", s.Msg)
		}
		commands.RenderOne(s, func() { say(s) })
	case "ls":
		if len(words) < 2 {
			commands.Fail("orgs transclude ls FILE")
		}
		file := commands.ResolveOrgFile(core, words[1])
		res, err := commands.SendReceiveGetErr[struct {
			Ok            bool
			Msg           string
			Enable        bool
			Transclusions []transclusion
		}](core, "transclusions", map[string]string{"file": file})
		if err != nil {
			commands.Fail("%v", err)
		}
		if !res.Ok {
			commands.Fail("%s", res.Msg)
		}
		commands.Render(res.Transclusions, func() {
			for _, t := range res.Transclusions {
				if t.Error != "" {
					fmt.Printf("%s:%d: %s%s%s\n", filepath.Base(file), t.Line+1, commands.C(commands.AnsiRed), t.Error, commands.C(commands.AnsiReset))
					continue
				}
				fmt.Printf("%s:%d: %s  →  %s:%d-%d %s%s%s\n", filepath.Base(file), t.Line+1, t.Link,
					t.File, t.From+1, t.To+1, commands.C(commands.AnsiDim), t.Title, commands.C(commands.AnsiReset))
			}
			if len(res.Transclusions) == 0 {
				fmt.Println("no #+transclude: lines")
			}
			if !res.Enable {
				fmt.Println("(transclusion is off: orgs transclude on)")
			}
		})
	default:
		commands.Fail("orgs transclude [on|off|status|ls FILE]")
	}
}

func say(s state) {
	if s.Enable {
		fmt.Printf("transclusion is %son%s (nested %d deep)\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), s.Depth)
	} else {
		fmt.Println("transclusion is off")
	}
	if s.Msg != "" {
		fmt.Println(s.Msg)
	}
}

func init() {
	commands.AddCmd("transclude", "turn #+transclude: on or off, and list what a file transcludes",
		func() commands.Cmd { return &Transclude{} })
}

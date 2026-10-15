package orgtools

// orgs detangle - a tangled file's edits back into the org blocks it came from.
//
//	orgs detangle install.sh            put the edits back
//	orgs detangle install.sh -dry-run   say which blocks would change
//	orgs detangle bin/*.sh              several at once
//
// The tangled file is read here and its text sent, as orgs import does: it is
// usually on this machine, and the server only reads files inside its org
// directories. Its path goes too, because the markers' links are relative to
// it. Needs the markers a tangle with :comments link writes.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Detangle struct{}

func (self *Detangle) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Detangle) StartPlugin(m *common.PluginManager)       {}
func (self *Detangle) SetupParameters(fset *flag.FlagSet)        {}
func (self *Detangle) HelpGroup() string                         { return "Files and code" }

func (self *Detangle) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("detangle").Flags)
	if len(words) == 0 {
		commands.Fail("orgs detangle <tangled file>... [-dry-run]")
	}
	all := []common.DetangleResult{}
	failed := false
	for _, name := range words {
		b, err := os.ReadFile(name)
		if err != nil {
			commands.Fail("could not read %s: %v", name, err)
		}
		abs, _ := filepath.Abs(name)
		req := common.DetangleRequest{File: abs, Text: string(b), DryRun: commands.DryRun}
		// Called directly, not through SendReceivePost: a dry run is the server
		// saying which blocks would change, which is the useful answer.
		res, err := common.RestPost[common.DetangleResult](&core.Rest, "detangle", &req)
		if err != nil {
			commands.Fail("orgs detangle: %v (is the server new enough to have /detangle?)", err)
		}
		all = append(all, res)
		if !res.Ok {
			failed = true
		}
		if commands.Machine() {
			continue
		}
		g, y, d, r := commands.C(commands.AnsiGreen), commands.C(commands.AnsiGold), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
		fmt.Printf("%s%s%s: %s\n", commands.C(commands.AnsiBold), name, r, res.Msg)
		for _, bl := range res.Blocks {
			mark := d + "=" + r
			switch bl.Status {
			case "updated":
				mark = g + "✓" + r
				if commands.DryRun {
					mark = g + "~" + r
				}
			case "skipped", "missing":
				mark = y + "!" + r
			}
			where := ""
			if bl.File != "" {
				where = fmt.Sprintf(" %s%s:%d%s", d, commands.BaseName(bl.File), bl.Line+1, r)
			}
			msg := bl.Status
			if bl.Msg != "" {
				msg += " - " + bl.Msg
			}
			fmt.Printf("  %s %s%s  %s\n", mark, bl.Label, where, msg)
		}
	}
	if commands.Machine() {
		commands.Render(all, nil)
	}
	if failed {
		os.Exit(1)
	}
}

func init() {
	commands.AddCmd("detangle", "put a tangled file's edits back into its org blocks (needs :comments link)",
		func() commands.Cmd { return &Detangle{} })
}

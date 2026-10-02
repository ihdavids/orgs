package reloadconfig

// Asks the server to read its yaml file again: capture templates, filters, tag
// groups, day page settings and the rest of what can change under a running
// server. Ports, orgDirs, auth and plugins still need a restart, and the
// answer names any of those the file changed.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type ReloadConfig struct {
}

func (self *ReloadConfig) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *ReloadConfig) StartPlugin(manager *common.PluginManager) {
}

func (self *ReloadConfig) SetupParameters(fset *flag.FlagSet) {
}

func (self *ReloadConfig) Exec(core *commands.Core) {
	reply, err := commands.SendReceivePostErr[common.Empty, common.ReloadResult](core, "reloadconfig", &common.Empty{})
	if errors.Is(err, commands.ErrDryRun) {
		return
	}
	if reply.Msg == "" {
		if err == nil {
			err = fmt.Errorf("no answer from the server")
		}
		commands.Fail("reloadconfig: %v", err)
		return
	}
	commands.RenderOne(reply, func() {
		if !reply.Ok {
			fmt.Fprintf(os.Stderr, "reloadconfig: %s\n", reply.Msg)
			return
		}
		fmt.Printf("%sreloaded%s %s\n", commands.C("32"), commands.C("0"), reply.File)
		if len(reply.NeedsRestart) > 0 {
			fmt.Printf("%schanged, needs a restart:%s %s\n", commands.C("33"), commands.C("0"), strings.Join(reply.NeedsRestart, ", "))
		}
	})
}

// init function is called at boot
func init() {
	commands.AddCmd("reloadconfig", "server re-reads its yaml: capture templates, filters, day page and other settings",
		func() commands.Cmd {
			return &ReloadConfig{}
		})
}

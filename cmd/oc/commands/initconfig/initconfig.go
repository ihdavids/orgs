package initconfig

import (
	"embed"
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

//go:embed orgs.yaml.tmpl
var configTemplate embed.FS

type InitConfig struct {
}

func (self *InitConfig) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *InitConfig) StartPlugin(manager *common.PluginManager) {
}

func (self *InitConfig) SetupParameters(fset *flag.FlagSet) {
}

func (self *InitConfig) Exec(core *commands.Core) {
	data, err := configTemplate.ReadFile("orgs.yaml.tmpl")
	if err != nil {
		fmt.Printf("ERROR: failed to read embedded config template: %v\n", err)
		return
	}
	fmt.Print(string(data))
}

func init() {
	commands.AddCmd("initconfig", "Output a default orgs.yaml configuration template",
		func() commands.Cmd {
			return &InitConfig{}
		})
}

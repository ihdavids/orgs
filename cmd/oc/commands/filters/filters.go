package filters

// Shows the Filters stored on the server

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type FiltersList struct {
}

func (self *FiltersList) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *FiltersList) SetupParameters(fset *flag.FlagSet) {
}

func (self *FiltersList) Exec(core *commands.Core) {
	fmt.Printf("TagGroups called\n")

	var qry map[string]string = map[string]string{}
	var reply map[string]string = map[string]string{}
	commands.SendReceiveGet(core, "filters", qry, &reply)
	common.FzfMapOfString(reply)
}

// init function is called at boot
func init() {
	commands.AddCmd("filters", "query information about stored filters",
		func() commands.Cmd {
			return &FiltersList{}
		})
}

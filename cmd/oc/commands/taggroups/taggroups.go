package taggroups

// Shows the TagGroups stored on the server

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type TagGroups struct {
}

func (self *TagGroups) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *TagGroups) StartPlugin(manager *common.PluginManager) {
}

func (self *TagGroups) SetupParameters(fset *flag.FlagSet) {
}

func (self *TagGroups) Exec(core *commands.Core) {
	fmt.Printf("TagGroups called\n")

	var qry map[string]string = map[string]string{}
	var reply map[string][]string = map[string][]string{}
	commands.SendReceiveGet(core, "taggroups", qry, &reply)
	common.FzfMapOfStringArray(reply)
}

// init function is called at boot
func init() {
	commands.AddCmd("taggroups", "query information about stored tag groups",
		func() commands.Cmd {
			return &TagGroups{}
		})
}

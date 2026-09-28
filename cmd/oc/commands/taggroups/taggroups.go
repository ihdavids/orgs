package taggroups

// Shows the TagGroups stored on the server

import (
	"flag"

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
	var qry map[string]string = map[string]string{}
	var reply map[string][]string = map[string][]string{}
	commands.SendReceiveGet(core, "taggroups", qry, &reply)
	// A chooser is no use to a program, so -json and -format answer with the
	// groups instead of opening one.
	if commands.RenderOne(reply, nil) {
		return
	}
	common.FzfMapOfStringArray(reply)
}

// init function is called at boot
func init() {
	commands.AddCmd("taggroups", "query information about stored tag groups",
		func() commands.Cmd {
			return &TagGroups{}
		})
}

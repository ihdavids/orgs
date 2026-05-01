package clockout

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type ClockOut struct {
}

func (self *ClockOut) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *ClockOut) StartPlugin(manager *common.PluginManager) {
}

func (self *ClockOut) SetupParameters(fset *flag.FlagSet) {
}

func (self *ClockOut) Exec(core *commands.Core) {
	var reply common.ResultMsg
	commands.SendReceivePost(core, "clockout", &common.Empty{}, &reply)
	if reply.Ok {
		fmt.Printf("OK: %s\n", reply.Msg)
	} else {
		fmt.Printf("Err: %s\n", reply.Msg)
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("clockout", "clock out of any active clock",
		func() commands.Cmd {
			return &ClockOut{}
		})
}

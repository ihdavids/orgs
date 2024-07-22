package export

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Export struct {
}

func (self *Export) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Export) SetupParameters(*flag.FlagSet) {
}

func (self *Export) Exec(core *commands.Core) {
	fmt.Printf("Export called\n")

	var qry map[string]string = map[string]string{}
	qry["filename"] = "./out.html"
	qry["query"] = "IsTask() && HasProperty(\"EFFORT\")"
	var reply common.Result = common.Result{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "file/mermaid", qry, &reply)
	//commands.SendReceiveRpc(core, "Db.ExportToFile", &query, &reply)
	if reply.Ok {
		fmt.Printf("OK")
	} else {
		fmt.Printf("Err")
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("export", "export a given module",
		func() commands.Cmd {
			return &Export{}
		})
}

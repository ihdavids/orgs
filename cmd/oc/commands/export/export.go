package export

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Export struct {
	Filename string
	Query    string
	Format   string
	Local    string
	Parent   string
}

func (self *Export) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Export) StartPlugin(manager *common.PluginManager) {
}

// 3484778973
func (self *Export) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&(self.Filename), "out", "./out.html", "output filename")
	fset.StringVar(&(self.Query), "query", "IsTask() && HasProperty(\"EFFORT\")", "query")
	fset.StringVar(&(self.Format), "f", "mermaid", "export format")
	fset.StringVar(&(self.Local), "l", "t", "local or not")
	fset.StringVar(&(self.Parent), "parent", "", "parent identifier")
}

func (self *Export) Exec(core *commands.Core) {
	fmt.Printf("Export called\n")

	var qry map[string]string = map[string]string{}
	qry["filename"] = self.Filename
	qry["query"] = self.Query
	qry["local"] = self.Local
	qry["parent"] = self.Parent
	var reply common.Result = common.Result{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, fmt.Sprintf("file/%s", self.Format), qry, &reply)
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
			return &Export{"./out.html", "IsTask() && HasProperty(\"EFFORT\")", "mermaid", "t", ""}
		})
}

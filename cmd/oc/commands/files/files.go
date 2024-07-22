package files

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type FilesQuery struct {
}

func (self *FilesQuery) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *FilesQuery) SetupParameters(*flag.FlagSet) {
}

func (self *FilesQuery) Exec(core *commands.Core) {
	fmt.Printf("FilesQuery called\n")

	var qry map[string]string = map[string]string{}
	//qry["filename"] = "./out.html"
	//qry["query"] = "IsTask() && HasProperty(\"EFFORT\")"
	var reply common.FileList = common.FileList{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "files", qry, &reply)
	//commands.SendReceiveRpc(core, "Db.ExportToFile", &query, &reply)
	if reply != nil {
		fmt.Printf("OK")
		for _, file := range reply {
			fmt.Printf("%s\n", file)
		}
	} else {
		fmt.Printf("Err")
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("files", "query information about files in DB",
		func() commands.Cmd {
			return &FilesQuery{}
		})
}

/*
func ShowFileList(c *rpc.Client) {
	var reply common.FileList
	err := c.Call("Db.GetFileList", nil, &reply)
	if err != nil {
		log.Printf("%v", err)
	} else {
		log.Printf("%v", reply)
	}
}
*/

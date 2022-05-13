package orgc

import (
	"log"
	"net/rpc"

	"github.com/ihdavids/orgs/internal/common"
)

func ShowFileList(c *rpc.Client) {
	var reply common.FileList
	err := c.Call("Db.GetFileList", nil, &reply)
	if err != nil {
		log.Printf("%v", err)
	} else {
		log.Printf("%v", reply)
	}
}

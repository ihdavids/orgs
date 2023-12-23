package orgc

import (
	b64 "encoding/base64"
	"fmt"

	"github.com/ihdavids/orgs/internal/common"
)

type CommandStatus struct {
	CommandEmpty
	Core *Core
}

func NewCommandStatus() {
	var todo *CommandStatus = new(CommandStatus)
	GetCmdRegistry().RegisterCommand("stat", todo)
}

func (self *CommandStatus) GetName() string {
	return "stat"
}

func (self *CommandStatus) GetDescription() string {
	return "Set status on current item\n  > stat <TAGNAME>"
}

func (self *CommandStatus) AutoComplete(core *Core, cmdTxt string) []string {
	var result []string
	if cmd, ok := core.statusBar.curCmd.(Selectable); ok {
		//var query common.TodoHash
		//query = common.TodoHash(cmd.GetSelectedHash())
		var reply common.TodoStatesResult
		//SendReceiveRpc(core, "Db.QueryValidStatus", &query, &reply)
		path := fmt.Sprintf("status/%s", b64.URLEncoding.EncodeToString([]byte(cmd.GetSelectedHash())))
		params := map[string]string{}
		SendReceiveGet(core, path, params, &reply)
		for _, v := range reply.Active {
			result = append(result, "stat "+v)
		}
		for _, v := range reply.Done {
			result = append(result, "stat "+v)
		}
	}
	return result
}

func (self *CommandStatus) IsTransient() bool { return true }

func (self *CommandStatus) Enter(core *Core, params []string) {
	self.Core = core
	if len(params) <= 0 {
		return
	}
	var query common.TodoItemChange
	if cmd, ok := core.statusBar.curCmd.(Selectable); ok {
		query.Hash = cmd.GetSelectedHash()
		query.Value = params[0]
		var reply common.Result = common.Result{}
		SendReceivePost(core, "status", &query, &reply)
		//SendReceiveRpc(core, "Db.ChangeStatus", &query, &reply)

	}
}

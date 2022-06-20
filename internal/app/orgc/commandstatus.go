package orgc

import "github.com/ihdavids/orgs/internal/common"

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
		SendReceiveRpc(core, "Db.ChangeStatus", &query, &reply)

	}
}

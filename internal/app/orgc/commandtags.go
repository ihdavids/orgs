package orgc

import "github.com/ihdavids/orgs/internal/common"

type CommandTags struct {
	CommandEmpty
	Core *Core
}

func NewCommandTags() {
	var todo *CommandTags = new(CommandTags)
	GetCmdRegistry().RegisterCommand("tag", todo)
}

func (self *CommandTags) GetName() string {
	return "tag"
}

func (self *CommandTags) GetDescription() string {
	return "Turn off or on a tag on a headline\n  > tag <TAGNAME>"
}

func (self *CommandTags) IsTransient() bool { return true }

func (self *CommandTags) Enter(core *Core, params []string) {
	self.Core = core
	if len(params) <= 0 {
		return
	}
	var query common.TodoItemChange
	if cmd, ok := core.statusBar.curCmd.(Selectable); ok {
		query.Hash = cmd.GetSelectedHash()
		query.Value = params[0]
		var reply common.Result = common.Result{}
		///SendReceiveRpc(core, "Db.ToggleTags", &query, &reply)
		SendReceivePost(core, "tags", &query, &reply)

	}
}

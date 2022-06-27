package orgc

import "github.com/ihdavids/orgs/internal/common"

type CommandDayPage struct {
	CommandEmpty
}

func NewCommandDayPage() {
	cmd := CommandDayPage{}
	GetCmdRegistry().RegisterCommand("daypage", &cmd)
}

func (self *CommandDayPage) GetName() string {
	return "daypage"
}

func (self *CommandDayPage) GetDescription() string {
	return "Generates a new day page"
}

func (self *CommandDayPage) EnterTasks(core *Core, params []string) {
	var query common.TodoHash = "empty"
	var reply common.Result

	SendReceiveRpc(core, "Db.CreateDayPage", &query, &reply)
}

package orgc

import (
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type CommandDayPage struct {
	CommandEmpty
	CurDayPage time.Time
}

var cmd *CommandDayPage

func NewCommandDayPage() {
	cmd = &CommandDayPage{}
	dt := time.Now()
	cmd.CurDayPage = dt
	GetCmdRegistry().RegisterCommand("daypage", cmd)
	GetCmdRegistry().RegisterCommand("nextdaypage", &CommandExec{"nextdaypage", "Jump to the next page in the list", func(core *Core, params []string) {

		var reply common.FileList
		cmd.CurDayPage = cmd.CurDayPage.AddDate(0, 0, 7)

		SendReceiveRpc(core, "Db.GetDayPageAt", &cmd.CurDayPage, &reply)

		if len(reply) > 0 {
			LaunchEditor(reply[0], 0)
		}
	}})
	GetCmdRegistry().RegisterCommand("prevdaypage", &CommandExec{"prevdaypage", "Jump to the prev page in the list", func(core *Core, params []string) {

		var reply common.FileList
		cmd.CurDayPage = cmd.CurDayPage.AddDate(0, 0, -7)

		SendReceiveRpc(core, "Db.GetDayPageAt", &cmd.CurDayPage, &reply)

		if len(reply) > 0 {
			core.statusBar.showForSeconds("[yellow::]GOT..."+reply[0], 5)
			LaunchEditor(reply[0], 0)
		} else {
			core.statusBar.showForSeconds("DID NOT GET ANYTHING FOR: "+cmd.CurDayPage.Format("Mon_2006_02_01"), 5)
		}
	}})
}

func (self *CommandDayPage) GetName() string {
	return "daypage"
}

func (self *CommandDayPage) GetDescription() string {
	return "Generates a new day page"
}

func (self *CommandDayPage) EnterTasks(core *Core, params []string) {
	var query common.TodoHash = "empty"
	var reply common.FileList
	cmd.CurDayPage = time.Now()
	SendReceiveRpc(core, "Db.CreateDayPage", &query, &reply)

	// Okay we created the page, now launch it!
	if len(reply) > 0 {
		LaunchEditor(reply[0], 0)
	}
}

func (self *CommandDayPage) IsTransient() bool { return true }

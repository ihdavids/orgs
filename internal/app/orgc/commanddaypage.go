package orgc

import (
	"fmt"
	"os"
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

		dt := cmd.CurDayPage
		// If we are missing 10 weeks worth of day pages then we abort!
		for iterations := 0; iterations < 10; iterations++ {
			var reply common.FileList
			cmd.CurDayPage = cmd.CurDayPage.AddDate(0, 0, 7)
			var send common.Date
			send.Set(cmd.CurDayPage)
			params := map[string]string{}
			getdp := fmt.Sprintf("daypage/%s", send)
			///SendReceiveRpc(core, "Db.GetDayPageAt", &send, &reply)
			SendReceiveGet(core, getdp, params, &reply)

			if len(reply) > 0 {
				if _, err := os.Stat(reply[0]); err == nil {
					core.statusBar.showForSeconds("[yellow::]GOT..."+reply[0], 1)
					LaunchEditor(reply[0], 0)
					return
				} else {
					// Did not find anything continue!
				}
			} else {
				core.statusBar.showForSeconds("DID NOT GET ANYTHING FOR: "+cmd.CurDayPage.Format("Mon_2006_02_01"), 5)
				return
			}
		}
		// Okay we didn't find a day page, restore our counter as that was just a bogus search.
		cmd.CurDayPage = dt
	}})
	GetCmdRegistry().RegisterCommand("prevdaypage", &CommandExec{"prevdaypage", "Jump to the prev page in the list", func(core *Core, params []string) {

		dt := cmd.CurDayPage
		// If we are missing 10 weeks worth of day pages then we abort!
		for iterations := 0; iterations < 10; iterations++ {
			var reply common.FileList
			cmd.CurDayPage = cmd.CurDayPage.AddDate(0, 0, -7)
			var send common.Date
			send.Set(cmd.CurDayPage)

			params := map[string]string{}
			getdp := fmt.Sprintf("daypage/%s", send)
			///SendReceiveRpc(core, "Db.GetDayPageAt", &send, &reply)
			SendReceiveGet(core, getdp, params, &reply)

			if len(reply) > 0 {
				if _, err := os.Stat(reply[0]); err == nil {
					core.statusBar.showForSeconds("[yellow::]GOT..."+reply[0], 1)
					LaunchEditor(reply[0], 0)
					return
				} else {
					// Did not find anything continue!
				}
			} else {
				core.statusBar.showForSeconds("DID NOT GET ANYTHING FOR: "+cmd.CurDayPage.Format("Mon_2006_02_01"), 5)
				return
			}
		}
		// Okay we didn't find a day page, restore our counter as that was just a bogus search.
		cmd.CurDayPage = dt
	}})
}

func (self *CommandDayPage) GetName() string {
	return "daypage"
}

func (self *CommandDayPage) GetDescription() string {
	return "Generates a new day page"
}

func (self *CommandDayPage) EnterTasks(core *Core, params []string) {
	//var query common.TodoHash = "empty"
	var reply common.FileList
	cmd.CurDayPage = time.Now()

	// CreateDayPage
	SendReceivePost(core, "daypage", &cmd, &reply)
	//SendReceiveRpc(core, "Db.CreateDayPage", &query, &reply)

	// Okay we created the page, now launch it!
	if len(reply) > 0 {
		LaunchEditor(reply[0], 0)
	}
}

func (self *CommandDayPage) IsTransient() bool { return true }

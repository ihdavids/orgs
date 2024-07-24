package daypage

import (
	"flag"
	"fmt"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type DayPage struct {
}

func (self *DayPage) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *DayPage) SetupParameters(*flag.FlagSet) {
}

func (self *DayPage) Exec(core *commands.Core) {
	fmt.Printf("DayPage called\n")
	var reply common.FileList
	cur := time.Now()

	// CreateDayPage
	commands.SendReceivePost(core, "daypage", &cur, &reply)
	//SendReceiveRpc(core, "Db.CreateDayPage", &query, &reply)

	// Okay we created the page, now launch it!
	if len(reply) > 0 {
		core.LaunchEditor(reply[0], 0)
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("daypage", "Open up daypage for me",
		func() commands.Cmd {
			return &DayPage{}
		})
}

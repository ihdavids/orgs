package clocks

import (
	"flag"
	"fmt"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type ClockData struct {
	Active bool
	Time   org.OrgDate
	Target common.Target
}

type Clocks struct {
}

func (self *Clocks) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Clocks) StartPlugin(manager *common.PluginManager) {
}

func (self *Clocks) SetupParameters(fset *flag.FlagSet) {
}

func (self *Clocks) Exec(core *commands.Core) {
	var qry map[string]string = map[string]string{}
	var data ClockData
	commands.SendReceiveGet(core, "clock", qry, &data)
	if !data.Active {
		fmt.Println("No active clock")
		return
	}
	elapsed := time.Since(data.Time.Start)
	hours := int(elapsed.Hours())
	mins := int(elapsed.Minutes()) % 60
	fmt.Printf("Active Clock:\n")
	fmt.Printf("  Target:  %s :: %s\n", data.Target.Filename, data.Target.Id)
	fmt.Printf("  Started: %s\n", data.Time.Start.Format("2006-01-02 15:04"))
	fmt.Printf("  Elapsed: %dh %02dm\n", hours, mins)
}

// init function is called at boot
func init() {
	commands.AddCmd("clocks", "show any active running clock",
		func() commands.Cmd {
			return &Clocks{}
		})
}

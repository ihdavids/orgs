package filters

// Shows the Filters stored on the server

import (
	"flag"
	"fmt"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

/*
		SDOC: Settings
      * Filters Orcs Cmd Module

   		This is the basis for a whole slew of visualizations
        These can be configured in your config file
        but orgs provides some built in (internal)
        ones that you can override if you don't like them.

   		The querries are found in the settings.go file
	    EDOC
*/

type FiltersList struct {
}

func (self *FiltersList) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *FiltersList) StartPlugin(manager *common.PluginManager) {
}

func (self *FiltersList) SetupParameters(fset *flag.FlagSet) {
}

func (self *FiltersList) Exec(core *commands.Core) {
	fmt.Printf("Filters called\n")

	var qry map[string]string = map[string]string{}
	var reply map[string]string = map[string]string{}
	commands.SendReceiveGet(core, "filters", qry, &reply)
	common.FzfMapOfString(reply)
}

// ------------------------------------
type Filter struct {
	CoreQuery    string
	DynamicQuery string

	core *commands.Core
}

func (self *Filter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Filter) StartPlugin(manager *common.PluginManager) {
	for k, v := range manager.Filters {
		commands.AddCmd(k, v,
			func() commands.Cmd {
				return &Filter{CoreQuery: k, DynamicQuery: self.DynamicQuery}
			})
	}
}

func (self *Filter) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.DynamicQuery, "f", "", "Additional filtering for filter list")
}

func (self *Filter) Exec(core *commands.Core) {
	fmt.Printf("Filter called\n")
	self.core = core
	var qry map[string]string = map[string]string{}
	qry["query"] = "{{" + self.CoreQuery + "}} "
	if self.DynamicQuery != "" {
		qry["query"] += " && " + self.DynamicQuery
	}
	var reply common.Todos = common.Todos{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "search", qry, &reply)

	common.RunTodoTable(&reply)
	//common.FzfMapOfString(reply)
}

// init function is called at boot
func init() {
	commands.AddCmd("filters", "Display a list of all registered filters",
		func() commands.Cmd {
			return &FiltersList{}
		})
	commands.AddCmd("filter", "Display nodes matching a given filter",
		func() commands.Cmd {
			return &Filter{}
		})
}

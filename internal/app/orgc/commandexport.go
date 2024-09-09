package orgc

import (
	"fmt"
	"log"

	"github.com/ihdavids/orgs/internal/common"
)

type CommandExport struct {
	CommandEmpty
	Core *Core
}

func NewCommandExport() {
	var todo *CommandExport = new(CommandExport)
	GetCmdRegistry().RegisterCommand("export", todo)
}

func (self *CommandExport) GetName() string {
	return "export"
}

func (self *CommandExport) GetDescription() string {
	return "Export to a file"
}

func (self *CommandExport) IsTransient() bool { return false }

func (self *CommandExport) Enter(core *Core, params []string) {
	log.Printf("COMMAND EXPORT")
	self.Core = core
	/*
		if len(params) <= 0 {
			return
		}
	*/
	/*
		var query common.ExportToFile
		query.Filename = "out2.html"
		query.Name = "gantt"
		query.Opts = ""
		query.Query = "IsTask()"
		var reply common.ResultMsg = common.ResultMsg{}
		SendReceiveRpc(core, "Db.ExportToFile", &query, &reply)
	*/
	output := "out.html"
	format := "html"
	query := "IsTask()"

	if len(params) > 0 {
		format = params[0]
	}
	if len(params) > 1 {
		output = params[1]
	}
	if len(params) > 2 {
		query = params[2]
	}

	var qry map[string]string = map[string]string{}
	qry["filename"] = output
	qry["query"] = query
	qry["opts"] = ""
	qry["local"] = "t"
	var reply common.ResultMsg = common.ResultMsg{}
	SendReceiveGet(core, fmt.Sprintf("file/%s", format), qry, &reply)

	if reply.Ok {
		core.taskPane.SetTitle("Export success")
		core.projectPane.SetTitle("Export success")
		core.statusBar.showForSeconds("EXPORT Success", 2)
		log.Printf("EXPORT Success")
	} else {
		core.taskPane.SetTitle("Export failed")
		core.projectPane.SetTitle("Export failed")
		core.statusBar.showForSeconds("EXPORT Failed", 2)
		log.Printf("EXPORT Failed: %v\n", reply.Msg)
	}
}

package projects

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
)

type ProjectsQuery struct {
	table  *tview.Table
	core   *commands.Core
	filter string
}

func (self *ProjectsQuery) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *ProjectsQuery) StartPlugin(manager *common.PluginManager) {
}

func (self *ProjectsQuery) SetupParameters(f *flag.FlagSet) {
	f.StringVar(&self.filter, "f", "", "Additional filtering for project lists")
}

func (self *ProjectsQuery) HandleShortcuts(in *tcell.EventKey) *tcell.EventKey {
	return in
}
func getCol(s string) tcell.Color {
	switch s {
	case "TODO":
		return tcell.ColorDarkRed
	case "INPROGRESS":
		return tcell.ColorLightCyan
	case "IN-PROGRESS":
		return tcell.ColorLightCyan
	case "BLOCKED":
		return tcell.ColorRed
	case "DONE":
		return tcell.ColorDarkGreen
	default:
		return tcell.ColorWhite
	}
}

func (self *ProjectsQuery) ShowTodos(core *commands.Core, reply common.Todos) {
	cell := tview.NewTableCell("Filename            ").SetTextColor(tcell.ColorYellow)
	self.table.SetCell(0, 0, cell)
	cell = tview.NewTableCell("Status    ").SetTextColor(tcell.ColorYellow)
	self.table.SetCell(0, 1, cell)
	cell = tview.NewTableCell("Heading").SetTextColor(tcell.ColorYellow)
	self.table.SetCell(0, 2, cell)
	for r, t := range reply {
		c := 0
		fn := filepath.Base(t.Filename)
		fn = strings.TrimSuffix(fn, filepath.Ext(fn))
		cell = tview.NewTableCell(fn)
		cell.SetTextColor(tcell.ColorDarkGray)
		self.table.SetCell(r+1, c, cell)
		c += 1

		cell = tview.NewTableCell(t.Status)

		cell.SetTextColor(getCol(t.Status))
		self.table.SetCell(r+1, c, cell)
		c += 1

		cell = tview.NewTableCell(t.Headline)
		self.table.SetCell(r+1, c, cell)
		c += 1
	}
}

func (self *ProjectsQuery) Exec(core *commands.Core) {
	fmt.Printf("ProjectsQuery called\n")

	var qry map[string]string = map[string]string{}
	//qry["filename"] = "./out.html"
	qry["query"] = "IsProject()"
	if self.filter != "" {
		qry["query"] += " && " + self.filter
	}
	var reply common.Todos = common.Todos{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "search", qry, &reply)

	self.table = tview.NewTable().SetFixed(1, 1)
	self.core = core
	app := tview.NewApplication()

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(self.table, 0, 1, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return self.HandleShortcuts(event)
	})
	self.ShowTodos(core, reply)
	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("projects", "Query a list of all projects",
		func() commands.Cmd {
			return &ProjectsQuery{}
		})
}

/*
func ShowFileList(c *rpc.Client) {
	var reply common.FileList
	err := c.Call("Db.GetFileList", nil, &reply)
	if err != nil {
		log.Printf("%v", err)
	} else {
		log.Printf("%v", reply)
	}
}
*/

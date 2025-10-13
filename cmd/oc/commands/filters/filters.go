package filters

// Shows the Filters stored on the server

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

	table *tview.Table
	core  *commands.Core
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

func (self *Filter) ShowTodos(core *commands.Core, reply common.Todos) {
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

func (self *Filter) HandleShortcuts(in *tcell.EventKey) *tcell.EventKey {
	return in
}

func (self *Filter) Exec(core *commands.Core) {
	fmt.Printf("Filter called\n")

	var qry map[string]string = map[string]string{}
	qry["query"] = "IsProject()"
	if self.DynamicQuery != "" {
		qry["query"] += " && " + self.DynamicQuery
	}
	var reply common.Todos = common.Todos{}

	//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
	commands.SendReceiveGet(core, "search", qry, &reply)

	self.table = tview.NewTable().SetFixed(1, 1)
	self.table.SetSelectable(true, false)
	self.table.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrBold | tcell.AttrItalic | tcell.AttrUnderline))
	self.core = core
	app := tview.NewApplication()

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(self.table, 0, 1, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return self.HandleShortcuts(event)
		return nil
	})
	self.ShowTodos(core, reply)
	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
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

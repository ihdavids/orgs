package orgc

import (
	//"strings"
	"fmt"
	"time"

	//"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/internal/common"
	//"github.com/rivo/tview"
)

type CommandAgenda struct {
	Reply     common.Todos
	TaskReply common.FullTodo
	Error     error
}

func NewCommandAgenda() {
	var todo *CommandAgenda = new(CommandAgenda)
	GetCmdRegistry().RegisterCommand("agenda", todo)
}

func (self *CommandAgenda) GetName() string {
	return "agenda"
}

func (self *CommandAgenda) GetDescription() string {
	return "return todays agenda"
}

func (self *CommandAgenda) Enter(core *Core) {
	query := "!IsProject && !IsArchived() && IsTodo() && Today()"
	//self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
	SendReceiveRpc(core, "Db.QueryTodosExp", &query, &self.Reply)
}
func (self *CommandAgenda) EnterProjects(core *Core) {}
func (self *CommandAgenda) EnterTasks(core *Core) {
	core.taskPane.text.Clear()
	core.projectPane.list.Clear()
	core.taskPane.text.SetDynamicColors(true)
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		//core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	core.projectPane.SetTitle(fmt.Sprintf("[::u]<P>[::-] %s [%d]", self.GetName(), len(self.Reply)))
	tm := time.Now()
	txt := "[blue]" + tm.Format("Monday 02 January 2006") + "\n\n"
	start := 8
	end := 20
	for i := start; i < end; i += 1 {
		for _, v := range self.Reply {
			if v.Date.Start.Hour() == i {
				txt += fmt.Sprintf("%s", v.Headline)
			}
		}
		txt += fmt.Sprintf("                [grey]%2d:00 ........ ---------------------------\n", i)
	}
	core.taskPane.text.SetText(txt)
	/*
		for _, v := range self.Reply {
			item := core.projectPane.list.AddItem(v.Headline, strings.Join(v.Tags, ","), 0, nil)
			item.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
				if index < len(self.Reply) {
					core.statusBar.showForSeconds("STAT: "+self.Reply[index].Headline, 5)
					//self.Error = core.ws.Call("Db.QuerySpecificTodo", self.Query, &self.TaskReply)
					SendReceiveRpc(core, "Db.QueryFullTodo", &self.Reply[index].Hash, &self.TaskReply)
					//self.Error = core.ws.Call("Db.QueryFullTodo", self.Reply[index].Hash, &self.TaskReply)
					//core.taskPane.list.Clear()
					core.taskPane.text.Clear()
					core.taskPane.text.SetTextColor(tcell.ColorWhite).SetTextAlign(tview.AlignLeft)
					core.taskPane.text.SetBorder(true)
					//core.taskPane.list.AddItem(self.TaskReply.Headline, "", 0, nil)
					core.taskPane.text.SetTitle(self.TaskReply.Headline)
					core.taskPane.text.SetText(self.TaskReply.Content)
				}
			})
		}
	*/
	/*
		if err != nil {
			log.Printf("%v", err)
		} else {
			for _, v := range reply {
				log.Printf("%v", v.Headline)
			}
		}
	*/
}

func (self *CommandAgenda) Execute(core *Core) {}

func (self *CommandAgenda) ExitTasks(core *Core)    {}
func (self *CommandAgenda) ExitProjects(core *Core) {}
func (self *CommandAgenda) Exit(core *Core)         {}

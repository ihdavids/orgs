package orgc

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
)

type CommandTodo struct {
	Query       *common.StringQuery
	Name        string
	Description string
	Reply       common.Todos
	TaskReply   common.FullTodo
	Error       error
}

func NewCommandTodo(name string, view *string, desc *string) {
	var todo *CommandTodo = new(CommandTodo)
	todo.Name = name
	todo.Query = new(common.StringQuery)
	todo.Query.Query = *view
	todo.Description = *desc
	GetCmdRegistry().RegisterCommand(name, todo)
}

func (self *CommandTodo) GetName() string {
	return self.Name
}

func (self *CommandTodo) GetDescription() string {
	return self.Description
}

func (self *CommandTodo) Enter(core *Core) {
	//self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
	SendReceiveRpc(core, "Db.QueryTodosExp", &self.Query, &self.Reply)
}
func (self *CommandTodo) EnterProjects(core *Core) {}
func (self *CommandTodo) EnterTasks(core *Core) {
	core.taskPane.list.Clear()
	core.projectPane.list.Clear()
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	core.projectPane.SetTitle("[::u]<P>[::-] " + self.GetName())

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

func (self *CommandTodo) Execute(core *Core) {}

func (self *CommandTodo) ExitTasks(core *Core)    {}
func (self *CommandTodo) ExitProjects(core *Core) {}
func (self *CommandTodo) Exit(core *Core)         {}

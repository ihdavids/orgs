package orgc

import (
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

type CommandTodo struct {
	Query *common.StringQuery
	Name  string
	Reply common.Todos
	Error error
}

func NewCommandTodo(name string, view *string) {
	var todo *CommandTodo = new(CommandTodo)
	todo.Name = name
	todo.Query = new(common.StringQuery)
	todo.Query.Query = *view
	GetCmdRegistry().RegisterCommand(name, todo)
}

func (self *CommandTodo) GetName() string {
	return self.Name
}

func (self *CommandTodo) Enter(core *Core) {
	self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
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
				core.statusBar.showForSeconds("STAT: "+self.Reply[index].Headline, 1)
				//self.Error = core.ws.Call("Db.QuerySpecificTodo", self.Query, &self.TaskReply)
				core.taskPane.list.Clear()
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

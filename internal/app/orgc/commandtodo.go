package orgc

import (
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

type CommandTodo struct {
	Query *common.Query
	Name  string
	Reply common.Todos
	Error error
}

func NewCommandTodo(name string, view *TodoFilterConfig) {
	var todo *CommandTodo = new(CommandTodo)
	todo.Name = name
	todo.Query = new(common.Query)
	*todo.Query = view.Query
	GetCmdRegistry().RegisterCommand(name, todo)
}

func (self *CommandTodo) GetName() string {
	return self.Name
}

func (self *CommandTodo) Enter(core *Core) {
	self.Error = core.ws.Call("Db.QueryTodos", self.Query, &self.Reply)
}
func (self *CommandTodo) EnterProjects(core *Core) {}
func (self *CommandTodo) EnterTasks(core *Core) {
	core.taskPane.list.Clear()
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	for _, v := range self.Reply {
		core.taskPane.list.AddItem(v.Headline, strings.Join(v.Tags, ","), 0, nil)
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

package orgc

import "github.com/ihdavids/orgs/internal/common"

type CommandTodo struct {
	Query *common.Query
	Name  string
}

func NewCommandTodo(name string, view *TodoFilterConfig) {
	var todo *CommandTodo = new(CommandTodo)
	todo.Name = name
	todo.Query = new(common.Query)
	todo.Query.IsProject = view.IsProject
	todo.Query.Priorities = view.Priorities
	todo.Query.Status = view.Status
	todo.Query.Tags = view.Tags
	todo.Query.HeadlineRe = view.HeadlineRe
	GetCmdRegistry().RegisterCommand(name, todo)
}

func (self *CommandTodo) Enter(core *Core)         {}
func (self *CommandTodo) EnterProjects(core *Core) {}
func (self *CommandTodo) EnterTasks(core *Core)    {}

func (self *CommandTodo) Execute(core *Core) {}

func (self *CommandTodo) ExitTasks(core *Core)    {}
func (self *CommandTodo) ExitProjects(core *Core) {}
func (self *CommandTodo) Exit(core *Core)         {}

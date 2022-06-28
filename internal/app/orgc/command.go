package orgc

import (
	"errors"
	"strings"

	"github.com/gdamore/tcell/v2"
)

type Command interface {
	GetName() string
	GetDescription() string
	Enter(core *Core, params []string)
	EnterProjects(core *Core, params []string)
	EnterTasks(core *Core, params []string)

	Execute(core *Core, params []string)

	ExitTasks(core *Core)
	ExitProjects(core *Core)
	Exit(core *Core)

	HandleShortcuts(event *tcell.EventKey) *tcell.EventKey
	IsTransient() bool // Does this command execute and then we go back into the previous command.
}

type Selectable interface {
	GetSelectedHash() string
}

type AutoCompleteable interface {
	AutoComplete(core *Core, cmdTxt string) []string
}

type Filterable interface {
	Filter(core *Core, filter string)
}

type CommandRegistry struct {
	Commands map[string]Command
}

var registry *CommandRegistry

func GetCmdRegistry() *CommandRegistry {
	if registry == nil {
		registry = new(CommandRegistry)
		registry.SetupRegistry()
	}
	return registry
}

func (self *CommandRegistry) FindCommand(name string, params *[]string) (Command, error) {
	if c, found := self.Commands[name]; found {
		return c, nil
	}
	if len(name) > 0 {
		fields := strings.FieldsFunc(name, func(c rune) bool { return c == ' ' })
		if len(fields) > 1 {
			if c, found := self.Commands[fields[0]]; found {
				*params = fields[1:]
				return c, nil
			}
		}
	}
	return nil, errors.New("Failed to find index " + name)
}

func (self *CommandRegistry) SetupRegistry() {
	self.Commands = make(map[string]Command)
	// This is where you add new commands
}

func (self *CommandRegistry) RegisterCommand(name string, cmd Command) {
	self.Commands[name] = cmd
}

type CommandEmpty struct {
}

func (self *CommandEmpty) GetDescription() string {
	return "empty command, please override"
}
func (self *CommandEmpty) HandleShortcuts(event *tcell.EventKey) *tcell.EventKey { return event }
func (self *CommandEmpty) Enter(core *Core, params []string)                     {}
func (self *CommandEmpty) EnterProjects(core *Core, params []string)             {}
func (self *CommandEmpty) EnterTasks(core *Core, params []string)                {}
func (self *CommandEmpty) Execute(core *Core, params []string)                   {}
func (self *CommandEmpty) ExitTasks(core *Core)                                  {}
func (self *CommandEmpty) ExitProjects(core *Core)                               {}
func (self *CommandEmpty) Exit(core *Core)                                       {}
func (self *CommandEmpty) IsTransient() bool                                     { return false }

type CommandExec struct {
	Name string
	Desc string
	DoIt func(core *Core, params []string)
}

func (self *CommandExec) GetDescription() string                                { return self.Desc }
func (self *CommandExec) GetName() string                                       { return self.Name }
func (self *CommandExec) HandleShortcuts(event *tcell.EventKey) *tcell.EventKey { return event }
func (self *CommandExec) Enter(core *Core, params []string)                     {}
func (self *CommandExec) EnterProjects(core *Core, params []string)             {}
func (self *CommandExec) EnterTasks(core *Core, params []string)                {}
func (self *CommandExec) Execute(core *Core, params []string)                   { self.DoIt(core, params) }
func (self *CommandExec) ExitTasks(core *Core)                                  {}
func (self *CommandExec) ExitProjects(core *Core)                               {}
func (self *CommandExec) Exit(core *Core)                                       {}
func (self *CommandExec) IsTransient() bool                                     { return true }

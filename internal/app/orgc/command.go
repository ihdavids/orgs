package orgc

import (
	"errors"

	"github.com/gdamore/tcell/v2"
)

type Command interface {
	GetName() string
	GetDescription() string
	Enter(core *Core)
	EnterProjects(core *Core)
	EnterTasks(core *Core)

	Execute(core *Core)

	ExitTasks(core *Core)
	ExitProjects(core *Core)
	Exit(core *Core)

	HandleShortcuts(event *tcell.EventKey) *tcell.EventKey
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

func (self *CommandRegistry) FindCommand(name string) (Command, error) {
	if c, found := self.Commands[name]; found {
		return c, nil
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

package orgc

import "errors"

type Command interface {
	GetName() string
	Enter(core *Core)
	EnterProjects(core *Core)
	EnterTasks(core *Core)

	Execute(core *Core)

	ExitTasks(core *Core)
	ExitProjects(core *Core)
	Exit(core *Core)
}

type CommandRegistry struct {
	commands map[string]Command
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
	if c, found := self.commands[name]; found {
		return c, nil
	}
	return nil, errors.New("Failed to find index " + name)
}

func (self *CommandRegistry) SetupRegistry() {
	self.commands = make(map[string]Command)
	// This is where you add new commands
}

func (self *CommandRegistry) RegisterCommand(name string, cmd Command) {
	self.commands[name] = cmd
}


package orgc

import (
)

type CommandHelp struct {

}

func NewCommandHelp() {
	help := CommandHelp {}
	GetCmdRegistry().RegisterCommand("help", &help)
}

func (self *CommandHelp) GetName() string {
	return "help"
}

func (self *CommandHelp) GetDescription() string {
	return "returns this help message"
}

func (self *CommandHelp) Enter(core *Core) {}
func (self *CommandHelp) EnterProjects(core *Core) {}
func (self *CommandHelp) EnterTasks(core *Core) {
	core.taskPane.list.Clear()
	core.projectPane.list.Clear()
	core.projectPane.SetTitle("[::u]<P>[::-] " + self.GetName())
	for k,_ := range GetCmdRegistry().Commands {
		item := core.projectPane.list.AddItem(k, "", 0, nil)
		item.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
			if index > 0 && index < len(GetCmdRegistry().Commands) {
				core.statusBar.showForSeconds("HELP: "+ mainText, 5)
				core.taskPane.list.Clear()
				core.taskPane.list.AddItem(mainText, GetCmdRegistry().Commands[mainText].GetDescription(), 0, nil)
			}
		})

	}
}

func (self *CommandHelp) Execute(core *Core) {}

func (self *CommandHelp) ExitTasks(core *Core)    {}
func (self *CommandHelp) ExitProjects(core *Core) {}
func (self *CommandHelp) Exit(core *Core)         {}

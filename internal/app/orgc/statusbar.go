package orgc

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// StatusBar displays hints and messages at the bottom of app
type StatusBar struct {
	*tview.Pages
	message   *tview.TextView
	quit      *tview.TextView
	navigate  *tview.TextView
	stepback  *tview.TextView
	newtask   *tview.TextView
	container *tview.Application
	command   *CommandPalette
	grid      *tview.Grid
	core      *Core
	curCmd    Command
	curParams []string
}

// Name of page keys
const (
	defaultPage = "default"
	messagePage = "message"
)

// Used to skip queued restore of statusBar
// in case of new showForSeconds within waiting period
var restorInQ = 0

func prepareStatusBar(core *Core) *StatusBar {
	var statusBar = &StatusBar{
		Pages:     tview.NewPages(),
		message:   tview.NewTextView().SetDynamicColors(true).SetText("Loading..."),
		container: core.app,
		command:   NewCommandPalette(core),
		core:      core,
		curCmd:    nil,
		curParams: nil,
	}
	core.statusBar = statusBar
	statusBar.navigate = tview.NewTextView().SetText("Navigate List: ↓,↑ / j,k")
	statusBar.quit = tview.NewTextView().SetText("Quit: Ctrl+C").SetTextAlign(tview.AlignRight)
	statusBar.newtask = tview.NewTextView().SetText("New Task/Project: n").SetTextAlign(tview.AlignCenter)
	statusBar.stepback = tview.NewTextView().SetText("Step back: Esc").SetTextAlign(tview.AlignCenter)
	statusBar.AddPage(messagePage, statusBar.message, true, true)
	statusBar.grid =
		tview.NewGrid(). // Content will not be modified, So, no need to declare explicitly
					SetColumns(0, 0, 0, 0).
					SetRows(0)
	statusBar.showBasicPanels()
	statusBar.AddPage(defaultPage,
		statusBar.grid,
		true,
		true,
	)

	return statusBar
}

func (bar *StatusBar) showBasicPanels() {
	bar.grid.AddItem(bar.navigate, 0, 0, 1, 1, 0, 0, false).
		AddItem(bar.newtask, 0, 1, 1, 1, 0, 0, false).
		AddItem(bar.stepback, 0, 2, 1, 1, 0, 0, false).
		AddItem(bar.quit, 0, 3, 1, 1, 0, 0, false)
}

func (bar *StatusBar) hideBasicPanels() {
	bar.grid.RemoveItem(bar.navigate).
		RemoveItem(bar.newtask).
		RemoveItem(bar.stepback).
		RemoveItem(bar.quit)
}

func (bar *StatusBar) restore() {
	bar.container.QueueUpdateDraw(func() {
		bar.SwitchToPage(defaultPage)
	})
}

func (self *StatusBar) cleanupCommandPalette() {
	self.grid.RemoveItem(self.command.view)
	self.showBasicPanels()
	self.command.core.app.SetFocus(self.command.core.projectPane)
}

func autoCompleteFunc(core *Core, curTxt string) []string {
	var cmds []string
	for k, c := range GetCmdRegistry().Commands {
		if curTxt == "" || strings.HasPrefix(k, curTxt) {
			cmds = append(cmds, fmt.Sprintf("%-15s # %-30s", k, strings.TrimSpace(c.GetDescription())))
		}
	}
	if len(cmds) <= 0 {
		cmds := strings.Split(curTxt, " ")
		curCmd := ""
		if len(cmds) > 0 {
			curCmd = strings.TrimSpace(cmds[0])
		}
		if curCmd != "" {
			var command Command = nil
			for k, c := range GetCmdRegistry().Commands {
				if k == curCmd {
					command = c
					break
				}
			}
			if command != nil {
				if cmd, ok := command.(AutoCompleteable); ok {
					curTxt = strings.TrimSpace(strings.Replace(curTxt, command.GetName(), "", 1))
					return cmd.AutoComplete(core, curTxt)
				}
			}

		}
	}
	return cmds
}

func (self *StatusBar) ExecuteCommand(cmdTxt string) {

	var params []string
	// trim off comment (description)
	cmdTxts := strings.Split(cmdTxt, "#")
	cmdTxt = strings.TrimSpace(cmdTxts[0])

	// We are trying to filter our active projects list!
	// We are locking in the filter
	cmds := strings.Fields(cmdTxt)
	if len(cmds) > 1 && cmds[0] == ":" {
		self.cleanupCommandPalette()
		return
	}
	if cmd, e := GetCmdRegistry().FindCommand(cmdTxt, &params); e == nil {
		if self.curCmd != nil {
			self.command.core.statusBar.showForSeconds("[yellow::]Cleaning up..."+self.curCmd.GetName(), 1)
			self.curCmd.ExitTasks(self.core)
			self.curCmd.ExitProjects(self.core)
			self.curCmd.Exit(self.core)
		}
		if cmd != nil {
			self.command.core.statusBar.showForSeconds("[yellow::]Executing..."+self.command.cmdText, 1)
			cmd.Enter(self.core, params)
			cmd.EnterProjects(self.core, params)
			cmd.EnterTasks(self.core, params)
			cmd.Execute(self.core, params)
			if cmd.IsTransient() {
				cmd.ExitTasks(self.core)
				cmd.ExitProjects(self.core)
				cmd.Exit(self.core)
				if self.curCmd != nil {
					self.curCmd.Enter(self.core, self.curParams)
					self.curCmd.EnterProjects(self.core, self.curParams)
					self.curCmd.EnterTasks(self.core, self.curParams)
					self.curCmd.Execute(self.core, self.curParams)
				}
			} else {
				self.curCmd = cmd
				self.curParams = params
			}
		}
	} else {
		self.command.core.statusBar.showForSeconds("[red::]Unknown Command: "+self.command.cmdText, 1)
	}
	self.cleanupCommandPalette()
}

func (self *StatusBar) commandPalette() {
	self.hideBasicPanels()

	self.grid.AddItem(self.command.view, 0, 0, 1, 2, 0, 0, true)
	self.command.view.SetAutocompleteFunc(func(cmdTxt string) []string { return autoCompleteFunc(self.command.core, cmdTxt) })

	self.command.core.app.SetFocus(self.command.view)
	self.command.view.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			self.ExecuteCommand(self.command.cmdText)
			//pane.addNewProject()
		case tcell.KeyEsc:
			self.cleanupCommandPalette()
		}
	})

	self.command.view.SetChangedFunc(func(text string) {

		// Strip off comments
		if text != "" {
			cmdTxts := strings.Split(text, "#")
			text = strings.TrimSpace(cmdTxts[0])
		}

		// We are trying to filter our active projects list!
		cmds := strings.Fields(text)
		if len(cmds) > 1 && cmds[0] == ":" {
			filter := strings.TrimSpace(strings.Join(cmds[1:], " "))
			if filter != "" {
				if self.core.statusBar.curCmd != nil {
					if op, ok := self.core.statusBar.curCmd.(Filterable); ok {
						op.Filter(self.core, filter)
					}
				}
			}
		}
		self.command.cmdText = text

	})

}

func (t *StatusBar) HandleEvent(evt tcell.Event) bool {
	switch evt.(type) {
	case *tcell.EventTime:
		Conf().Dispatch(t.core, nil)
	}
	return false
}

func (bar *StatusBar) showForSeconds(message string, timeout int) {
	if bar.container == nil {
		return
	}

	bar.message.SetText(message)
	bar.SwitchToPage(messagePage)
	restorInQ++

	go func() {
		time.Sleep(time.Second * time.Duration(timeout))

		// Apply restore only if this is the last pending restore
		if restorInQ == 1 {
			bar.restore()
		}
		restorInQ--
	}()
}

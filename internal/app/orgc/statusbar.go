package orgc

import (
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

func (self *StatusBar) commandPalette() {
	self.hideBasicPanels()

	self.grid.AddItem(self.command.view, 0, 0, 1, 2, 0, 0, true)
	self.command.core.app.SetFocus(self.command.view)
	var params []string
	self.command.view.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			if cmd, e := GetCmdRegistry().FindCommand(self.command.cmdText, &params); e == nil {
				if self.curCmd != nil {
					self.command.core.statusBar.showForSeconds("[yellow::]Cleaning up..."+self.curCmd.GetName(), 1)
					self.curCmd.ExitTasks(self.core)
					self.curCmd.ExitProjects(self.core)
					self.curCmd.Exit(self.core)
				}
				if cmd != nil {
					self.command.core.statusBar.showForSeconds("[yellow::]Executing..."+self.command.cmdText, 1)
					self.curCmd = cmd
					self.curCmd.Enter(self.core, params)
					self.curCmd.EnterProjects(self.core, params)
					self.curCmd.EnterTasks(self.core, params)
					self.curCmd.Execute(self.core, params)
				}
			} else {
				self.command.core.statusBar.showForSeconds("[red::]Unknown Command: "+self.command.cmdText, 1)
			}
			self.cleanupCommandPalette()
			//pane.addNewProject()
		case tcell.KeyEsc:
			self.cleanupCommandPalette()
		}
	})
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

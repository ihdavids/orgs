package orgc

import (
	"time"

	"github.com/rivo/tview"
)

// StatusBar displays hints and messages at the bottom of app
type StatusBar struct {
	*tview.Pages
	message   *tview.TextView
	container *tview.Application
	command   *CommandPalette
	grid      *tview.Grid
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
	}
	core.statusBar = statusBar
	statusBar.AddPage(messagePage, statusBar.message, true, true)
	statusBar.grid =
		tview.NewGrid(). // Content will not be modified, So, no need to declare explicitly
					SetColumns(0, 0, 0, 0).
					SetRows(0).
					AddItem(tview.NewTextView().SetText("Navigate List: ↓,↑ / j,k"), 0, 0, 1, 1, 0, 0, false).
					AddItem(tview.NewTextView().SetText("New Task/Project: n").SetTextAlign(tview.AlignCenter), 0, 1, 1, 1, 0, 0, false).
					AddItem(tview.NewTextView().SetText("Step back: Esc").SetTextAlign(tview.AlignCenter), 0, 2, 1, 1, 0, 0, false).
					AddItem(tview.NewTextView().SetText("Quit: Ctrl+C").SetTextAlign(tview.AlignRight), 0, 3, 1, 1, 0, 0, false)
	statusBar.AddPage(defaultPage,
		statusBar.grid,
		true,
		true,
	)

	return statusBar
}

func (bar *StatusBar) restore() {
	bar.container.QueueUpdateDraw(func() {
		bar.SwitchToPage(defaultPage)
	})
}

func (self *StatusBar) commandPalette() {
	self.grid.AddItem(self.command.view, 0, 3, 1, 1, 0, 0, true)
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

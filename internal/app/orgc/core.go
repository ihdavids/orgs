package orgc

import (
	"net/rpc"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Core struct {
	app              *tview.Application
	layout, contents *tview.Flex

	ws          *rpc.Client
	statusBar   *StatusBar
	projectPane *ProjectPane
	taskPane    *TaskPane
	//taskDetailPane    *TaskDetailPane
	//projectDetailPane *ProjectDetailPane
}

func NewCore(c *rpc.Client) *Core {
	core := new(Core)
	core.app = tview.NewApplication()
	core.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(makeTitleBar(), 2, 1, false).
		AddItem(prepareContentPages(core), 0, 2, true).
		AddItem(prepareStatusBar(core), 1, 1, false)
	core.ws = c
	setKeyboardShortcuts(core)

	return core
}

func (self *Core) Start() {
	if err := self.app.SetRoot(self.layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func makeTitleBar() *tview.Flex {
	titleText := tview.NewTextView().SetText("[lime::b]OrgC [::-]- Org Cli").SetDynamicColors(true)
	versionInfo := tview.NewTextView().SetText("[::d]Version: 0.0.1").SetTextAlign(tview.AlignRight).SetDynamicColors(true)

	return tview.NewFlex().
		AddItem(titleText, 0, 2, false).
		AddItem(versionInfo, 0, 1, false)
}

func prepareContentPages(core *Core) *tview.Flex {
	core.projectPane = NewProjectPane(core)
	core.taskPane = NewTaskPane(core)
	//core.projectDetailPane = NewProjectDetailPane()
	//core.taskDetailPane = NewTaskDetailPane(taskRepo)

	core.contents = tview.NewFlex().
		AddItem(core.projectPane, 25, 1, true).
		AddItem(core.taskPane, 0, 2, false)

	return core.contents
}

func (self *Core) AskYesNo(text string, f func()) {

	activePane := self.app.GetFocus()
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				f()
			}
			self.app.SetRoot(self.layout, true).EnableMouse(true)
			self.app.SetFocus(activePane)
		})

	pages := tview.NewPages().
		AddPage("background", self.layout, true, true).
		AddPage("modal", modal, true, true)
	_ = self.app.SetRoot(pages, true).EnableMouse(true)
}

func setKeyboardShortcuts(core *Core) *tview.Application {
	return core.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if ignoreKeyEvt(core) {
			return event
		}

		// Global shortcuts
		switch unicode.ToLower(event.Rune()) {
		case 'p':
			core.app.SetFocus(core.projectPane)
			//contents.RemoveItem(taskDetailPane)
			return nil
		case 'q':
		case 't':
			core.app.SetFocus(core.taskPane)
			//contents.RemoveItem(taskDetailPane)
			return nil
		case ':':
			core.statusBar.commandPalette()
			return nil
		}

		// Handle based on current focus. Handlers may modify event
		switch {
		case core.projectPane.HasFocus():
			event = core.projectPane.handleShortcuts(event)
		case core.taskPane.HasFocus():
			//event = core.taskPane.handleShortcuts(event)
			/*
				if event != nil && projectDetailPane.isShowing() {
					event = projectDetailPane.handleShortcuts(event)
				}
			*/
		}
		return event
	})
}

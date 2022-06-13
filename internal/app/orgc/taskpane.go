package orgc

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TaskPane displays tasks of current TaskList or Project
type TaskPane struct {
	*tview.Flex
	list *tview.List
	//tasks      []model.Task
	//activeTask *model.Task

	newTask *tview.InputField
	//projectRepo repository.ProjectRepository
	//taskRepo    repository.TaskRepository
	text *tview.TextView
	core *Core
}

// NewTaskPane initializes and configures a TaskPane
func NewTaskPane(core *Core) *TaskPane {
	pane := TaskPane{
		Flex: tview.NewFlex().SetDirection(tview.FlexRow),
		list: tview.NewList().ShowSecondaryText(false),
		//newTask: makeLightTextInput("+[New Task]"),
		//projectRepo: projectRepo,
		//taskRepo:    taskRepo,
		text: tview.NewTextView().SetTextColor(tcell.ColorYellow).SetTextAlign(tview.AlignCenter),
		core: core,
	}

	pane.list.SetSelectedBackgroundColor(tcell.ColorBlack)
	pane.list.SetSelectedTextColor(tcell.ColorYellow)
	pane.list.SetDoneFunc(func() {
		pane.core.app.SetFocus(pane.core.projectPane)
	})
	/*
		pane.newTask.SetDoneFunc(func(key tcell.Key) {
			switch key {
			case tcell.KeyEnter:
				name := pane.newTask.GetText()
				if len(name) < 3 {
					pane.core.statusBar.showForSeconds("[red::]Task title should be at least 3 character", 5)
					return
				}

					//task, err := taskRepo.Create(*projectPane.GetActiveProject(), name, "", "", 0)
					//if err != nil {
					//	statusBar.showForSeconds("[red::]Could not create Task:"+err.Error(), 5)
					//	return
					//}

					//pane.tasks = append(pane.tasks, task)
					//pane.addTaskToList(len(pane.tasks) - 1)
					//pane.newTask.SetText("")
					//statusBar.showForSeconds("[yellow::]Task created. Add another task or press Esc.", 5)
			case tcell.KeyEsc:
				pane.core.app.SetFocus(pane)
			}
		})
	*/
	pane.
		//AddItem(pane.list, 0, 1, true).
		AddItem(pane.text, 0, 1, true)

	//pane.SetBorder(true).SetTitle("[::u]T[::-]asks")
	//pane.setHintMessage()

	return &pane
}

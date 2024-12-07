//lint:file-ignore ST1006 allow the use of self
package capture

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/koki-develop/go-fzf"
	"github.com/rivo/tview"
)

type TaskPane struct {
	*tview.Flex
	list *tview.List
	//tasks      []model.Task
	//activeTask *model.Task

	newTask *tview.TextArea
	//projectRepo repository.ProjectRepository
	//taskRepo    repository.TaskRepository
	text *tview.TextView
	app  *tview.Application
}

func makeLightTextInput(placeholder string) *tview.TextArea {
	return tview.NewTextArea().
		SetPlaceholder(placeholder)
	//SetPlaceholderTextColor(tcell.ColorDarkSlateGrey).
	//SetFieldTextColor(tcell.ColorWhite).
	//SetFieldBackgroundColor(tcell.ColorBlack)
}
func MakeTaskPane(typeName string, app *tview.Application) *TaskPane {

	placeholder := ""
	switch typeName {
	default:
		placeholder = "+[Capture Text]"
	}
	pane := &TaskPane{
		Flex: tview.NewFlex().SetDirection(tview.FlexRow),
		//list: tview.NewList().ShowSecondaryText(false),
		newTask: makeLightTextInput(placeholder),
		//projectRepo: projectRepo,
		//taskRepo:    taskRepo,
		text: tview.NewTextView().SetTextColor(tcell.ColorYellow).SetTextAlign(tview.AlignCenter),
	}

	//pane.list.SetSelectedBackgroundColor(tcell.ColorBlack)
	//pane.list.SetSelectedTextColor(tcell.ColorYellow)
	//pane.list.SetDoneFunc(func() {
	//	pane.core.app.SetFocus(pane.core.projectPane)
	//})

	pane.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			pane.app.Stop()
			return nil
		}
		return event
	})

	/*
		pane.newTask.SetDoneFunc(func(key tcell.Key) {
			switch key {
			case tcell.KeyEnter:
				name := pane.newTask.GetText()
				if len(name) < 3 {
					//pane.core.statusBar.showForSeconds("[red::]Task title should be at least 3 character", 5)
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
				//pane.core.app.SetFocus(pane)
			}
		})
	*/

	pane.
		//AddItem(pane.list, 0, 1, true).
		AddItem(pane.newTask, 0, 1, true)
	return pane
}

type Capture struct {
	Template string
	Head     string
	Cont     string
}

func (self *Capture) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Capture) SetupParameters(fset *flag.FlagSet) {
	fmt.Printf("CAP CALLED\n")
	//fset := flag.NewFlagSet("capture", flag.ExitOnError)
	fset.StringVar(&(self.Template), "temp", "", "template name")
	fset.StringVar(&(self.Head), "head", "", "heading")
	fset.StringVar(&(self.Cont), "cont", "", "content")
	//fset.Parse(args)
}

func (self *Capture) Exec(core *commands.Core) {
	fmt.Printf("Capture called\n")
	/*
		fset := flag.NewFlagSet("capture", flag.ExitOnError)
		fset.StringVar(&self.Template, "temp", "", "template name")
		fset.StringVar(&self.Head, "head", "", "heading")
		fset.StringVar(&self.Cont, "cont", "", "content")
		fset.Parse(args)
	*/
	var qry map[string]string = map[string]string{}
	var rep []common.CaptureTemplate = []common.CaptureTemplate{}
	commands.SendReceiveGet(core, "capture/templates", qry, &rep)
	var reply common.ResultMsg = common.ResultMsg{}
	var capIndex int = 0
	if self.Template == "" {
		f, err := fzf.New(
			fzf.WithNoLimit(true),
			fzf.WithCountViewEnabled(true),
			fzf.WithCountView(func(meta fzf.CountViewMeta) string {
				return fmt.Sprintf("templates: %d, selected: %d", meta.ItemsCount, meta.SelectedCount)
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		var idx []int = []int{}
		idx, err = f.Find(rep, func(i int) string { return rep[i].Name })
		if err != nil {
			log.Fatal(err)
		}
		self.Template = rep[idx[0]].Name
	}
	for i, r := range rep {
		temp := r.Name
		if self.Template == temp {
			capIndex = i
		}
	}
	if self.Template == "" {
		log.Fatal("Cannot capture without a template")
	}

	if self.Head == "" {
		app := tview.NewApplication()
		p := MakeTaskPane(rep[capIndex].Type, app)

		if err := app.SetRoot(p, true).EnableMouse(true).Run(); err != nil {
			panic(err)
		}
		fmt.Printf("DONE")
		//if _, err := p.Run(); err != nil {
		//	log.Fatal(err)
		//}
	}

	if self.Head == "" {
		log.Fatal("Heading is required for some templates")
	}

	var query common.Capture
	query.Template = self.Template
	query.NewNode.Headline = self.Head
	query.NewNode.Content = self.Cont
	fmt.Printf("CAP: %s\n\t%s\n\t%s\n", query.Template, query.NewNode.Headline, query.NewNode.Content)
	commands.SendReceivePost(core, "capture", &query, &reply)
	//commands.SendReceiveRpc(core, "Db.Capture", &query, &reply)
	if reply.Ok {
		fmt.Printf("OK: %s\n", reply.Msg)
	} else {
		fmt.Printf("Err: %s\n", reply.Msg)
	}
}

type CaptureTemplate struct {
}

func (self *CaptureTemplate) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *CaptureTemplate) SetupParameters(*flag.FlagSet) {
}

func (self *CaptureTemplate) Exec(core *commands.Core) {
	fmt.Printf("Capture templates\n")

	var qry map[string]string = map[string]string{}
	//var reply common.Result = common.Result{}
	var reply *[]common.CaptureTemplate = &[]common.CaptureTemplate{}
	commands.SendReceiveGet(core, "capture/templates", qry, &reply)
	//commands.SendReceiveRpc(core, "Db.QueryCaptureTemplates", &query, &reply)
	for _, x := range *reply {
		b, err := json.MarshalIndent(x, "", "  ")
		if err != nil {
			fmt.Println(err)
		}
		fmt.Print(string(b))
		//fmt.Printf("%v", x)
	}
}

// init function is called at boot
func init() {
	commands.AddCmd("cap", "quick capture idea",
		func() commands.Cmd {
			return &Capture{}
		})
	commands.AddCmd("listcap", "list capture templates",
		func() commands.Cmd {
			return &CaptureTemplate{}
		})
}

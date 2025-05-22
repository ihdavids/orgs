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
	list    *tview.List
	newTask *tview.TextArea
	app     *tview.Application
}

func NeedsHeading(typeName string) bool {
	switch typeName {
	case "entry":
		return true
	case "item":
		return false
	case "checkitem":
		return false
	case "table-line":
		return false
	case "plain":
		return false
	}
	return false
}

func MakeTaskPane(title string, typeName string, app *tview.Application) *TaskPane {

	placeholder := ""
	switch typeName {
	default:
		placeholder = "+[Capture Text]"
	}
	pane := &TaskPane{
		Flex:    tview.NewFlex().SetDirection(tview.FlexRow),
		newTask: tview.NewTextArea().SetPlaceholder(placeholder),
		app:     app,
	}
	pane.newTask.SetTitle(title)
	pane.newTask.SetTitleColor(tcell.ColorDarkCyan)
	pane.newTask.SetTitleAlign(tview.AlignLeft)
	pane.newTask.SetBorder(true)

	pane.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			pane.app.Stop()
			return nil
		}
		return event
	})

	pane.
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
	//fmt.Printf("CAP CALLED\n")
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

	needsHeading := NeedsHeading(rep[capIndex].Type)
	if self.Head == "" && needsHeading {
		app := tview.NewApplication()
		p := MakeTaskPane("Enter Heading", rep[capIndex].Type, app)

		if err := app.SetRoot(p, true).EnableMouse(true).Run(); err != nil {
			panic(err)
		}
		self.Head = p.newTask.GetText()
	}

	if self.Head == "" && needsHeading {
		log.Fatal("Heading is required for some templates")
	}

	if self.Cont == "" {
		app := tview.NewApplication()
		p := MakeTaskPane("Enter Content", rep[capIndex].Type, app)

		if err := app.SetRoot(p, true).EnableMouse(true).Run(); err != nil {
			panic(err)
		}
		self.Cont = p.newTask.GetText()
	}
	//if _, err := p.Run(); err != nil {
	//	log.Fatal(err)
	//}
	//os.Exit(-1)

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

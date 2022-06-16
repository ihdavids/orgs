package orgc

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
	"github.com/zyedidia/highlight"
)

type CommandTodo struct {
	Query       *common.StringQuery
	Name        string
	Description string
	Reply       common.Todos
	TaskReply   common.FullTodo
	Error       error
}

func NewCommandTodo(name string, view *string, desc *string) {
	var todo *CommandTodo = new(CommandTodo)
	todo.Name = name
	todo.Query = new(common.StringQuery)
	todo.Query.Query = *view
	todo.Description = *desc
	GetCmdRegistry().RegisterCommand(name, todo)
}

var syntaxfile string = `
filetype: org

detect:
    filename: "\\.(org)$"

rules:
    # Tables (Github extension)
    - type: ".*[ :]\\|[ :].*"

      # quotes
    - statement:  "^>.*"

      # Emphasis
    - type: "(^|[[:space:]])(_[^ ][^_]*_|\\*[^ ][^*]*\\*)"

      # Strong emphasis
    - type: "(^|[[:space:]])(__[^ ][^_]*__|\\*\\*[^ ][^*]*\\*\\*)"

      # strike-through
    - type: "(^|[[:space:]])~~[^ ][^~]*~~"

      # horizontal rules
    - special: "^(---+|===+|___+|\\*\\*\\*+)\\s*$"

      # headlines
    - special:  "^\\*{1,6}.*"

      # tags
    - special:  "[:][#@a-zA-Z0-9]+[:]"

      # lists
    - identifier:   "^[[:space:]]*[\\*+-] |^[[:space:]]*[0-9]+\\. "

      # misc
    - preproc:   "(\\(([CcRr]|[Tt][Mm])\\)|\\.{3}|(^|[[:space:]])\\-\\-($|[[:space:]]))"

      # links
    - constant: "\\[\\[[^]]+\\]\\[[^]]+\\]\\]"

      # images
    - underlined: "!\\[[^][]*\\](\\([^)]+\\)|\\[[^]]+\\])"

      # urls
    - underlined: "https?://[^ )>]]+"

`

//    - constant: "\\[([^][]|\\[[^]]*\\])*\\]\\([^)]+\\)"

func FormatText(text string) string {
	if text == "" {
		return ""
	}

	d, err := highlight.ParseDef([]byte(syntaxfile))

	if err != nil {
		return "[red]" + err.Error()
	}

	h := highlight.NewHighlighter(d)
	var out string = ""

	matches := h.HighlightString(string(text))

	lines := strings.Split(string(text), "\n")
	var curGroup highlight.Group
	for lineN, l := range lines {
		colN := 0
		for _, ch := range l {
			var c string = string(ch)
			if group, ok := matches[lineN][colN]; ok {
				if group != curGroup {
					// There are more possible groups available than just these ones
					if group == highlight.Groups["statement"] {
						c = "[green]" + c
					} else if group == highlight.Groups["identifier"] {
						c = "[blue]" + c
					} else if group == highlight.Groups["preproc"] {
						c = "[red]" + c
					} else if group == highlight.Groups["special"] {
						c = "[red]" + c
					} else if group == highlight.Groups["constant.string"] {
						c = "[darkblue]" + c
					} else if group == highlight.Groups["constant"] {
						c = "[darkblue]" + c
					} else if group == highlight.Groups["constant.specialChar"] {
						c = "[magenta]" + c
					} else if group == highlight.Groups["type"] {
						c = "[yellow]" + c
					} else if group == highlight.Groups["constant.number"] {
						c = "[blue]" + c
					} else if group == highlight.Groups["comment"] {
						c = "[grey]" + c
					} else if group == highlight.Groups["underline"] {
						c = "[grey]" + c
					} else {
						c = "[none]" + c
					}

				}
			}
			out += string(c)
			colN++
		}
		if group, ok := matches[lineN][colN]; ok {
			if group != curGroup && (group == highlight.Groups["default"] || group == highlight.Groups[""]) {
				out += "[none]"
			}
		}
		out += "[none]\n"
	}
	return out
}

func (self *CommandTodo) GetName() string {
	return self.Name
}

func (self *CommandTodo) GetDescription() string {
	return self.Description
}

func (self *CommandTodo) HandleShortcuts(event *tcell.EventKey) *tcell.EventKey { return event }
func (self *CommandTodo) Enter(core *Core) {
	//self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
	SendReceiveRpc(core, "Db.QueryTodosExp", &self.Query, &self.Reply)
}
func (self *CommandTodo) EnterProjects(core *Core) {}
func (self *CommandTodo) EnterTasks(core *Core) {
	core.taskPane.list.Clear()
	core.projectPane.list.Clear()
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	core.projectPane.SetTitle("[::u]<P>[::-] " + self.GetName())

	for _, v := range self.Reply {
		item := core.projectPane.list.AddItem(v.Headline, strings.Join(v.Tags, ","), 0, nil)
		item.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
			if index < len(self.Reply) {
				//core.statusBar.showForSeconds("STAT: "+self.Reply[index].Headline, 5)
				//self.Error = core.ws.Call("Db.QuerySpecificTodo", self.Query, &self.TaskReply)
				SendReceiveRpc(core, "Db.QueryFullTodo", &self.Reply[index].Hash, &self.TaskReply)
				//self.Error = core.ws.Call("Db.QueryFullTodo", self.Reply[index].Hash, &self.TaskReply)
				//core.taskPane.list.Clear()
				core.taskPane.text.Clear()
				core.taskPane.text.SetTextColor(tcell.ColorWhite).SetTextAlign(tview.AlignLeft)
				core.taskPane.text.SetDynamicColors(true)
				core.taskPane.text.SetBorder(true)
				//core.taskPane.list.AddItem(self.TaskReply.Headline, "", 0, nil)
				core.taskPane.text.SetTitle(self.TaskReply.Headline)
				core.taskPane.text.SetText(FormatText(self.TaskReply.Content))
			}
		})

		item.SetSelectedFunc(func(index int, mainText string, secText string, shortcut rune) {

			// 0 offset or 1 offset needs to be handled
			LaunchEditor(self.Reply[index].Filename, self.Reply[index].LineNum+1)
			core.statusBar.showForSeconds("STAT: "+fmt.Sprintf("%d", self.Reply[index].LineNum)+" "+self.Reply[index].Headline, 5)
		})
		item.SetSelectedFunc(func(index int, mainText string, secText string, shortcut rune) {

			send := common.TodoStatusChange{Hash: self.Reply[index].Hash, Status: "DONE"}
			var reply common.Result = common.Result{}
			SendReceiveRpc(core, "Db.ChangeStatus", &send, &reply)
			res := "Ok"
			if !reply.Ok {
				res = "FAILED"
			}
			core.statusBar.showForSeconds("STATE: "+self.Reply[index].Headline+fmt.Sprint("%s", res), 5)
		})
	}
	/*
		if err != nil {
			log.Printf("%v", err)
		} else {
			for _, v := range reply {
				log.Printf("%v", v.Headline)
			}
		}
	*/
}

func (self *CommandTodo) Execute(core *Core) {}

func (self *CommandTodo) ExitTasks(core *Core)    {}
func (self *CommandTodo) ExitProjects(core *Core) {}
func (self *CommandTodo) Exit(core *Core)         {}

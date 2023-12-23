package orgc

import (
	b64 "encoding/base64"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
	"github.com/zyedidia/highlight"
)

type CommandTodo struct {
	CommandEmpty
	Query       *common.StringQuery
	Name        string
	Description string
	Reply       common.Todos
	TaskReply   common.FullTodo
	Error       error
	Core        *Core
	Selected    int
	Filtered    []common.Todo
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

func (self *CommandTodo) GetSelectedHash() string {
	if self.Selected >= 0 && self.Selected < len(self.Reply) {
		return self.Filtered[self.Selected].Hash
	}
	return ""
}

func (self *CommandTodo) HandleShortcuts(event *tcell.EventKey) *tcell.EventKey {
	if event.Rune() == 't' {
		self.Core.statusBar.showForSeconds("YOU PRESSED T", 1)
		return nil
	}
	return event
}

func (self *CommandTodo) GetName() string {
	return self.Name
}

func (self *CommandTodo) GetDescription() string {
	return self.Description
}

func (self *CommandTodo) Enter(core *Core, params []string) {
	self.Core = core
	//self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
	//SendReceiveRpc(core, "Db.QueryTodosExp", &self.Query, &self.Reply)
	pars := map[string]string{
		"query": string(self.Query.Query),
	}
	SendReceiveGet(core, "search", pars, &self.Reply)
}

var ActiveColorList []string = []string{"[red]", "[yellow]", "[orange]", "[magenta]"}
var InactiveColorList []string = []string{"[green]", "[blue]", "[grey]"}

var ActiveColors map[string]string = make(map[string]string)
var InactiveColors map[string]string = make(map[string]string)

func GetStatusColor(t *common.Todo) string {
	prefix := "[green]"
	if t.IsActive {
		if x, ok := ActiveColors[t.Status]; ok {
			return x
		} else {
			col := ActiveColorList[len(ActiveColors)%len(ActiveColorList)]
			ActiveColors[t.Status] = col
			return col
		}
	} else {
		if x, ok := InactiveColors[t.Status]; ok {
			return x
		} else {
			col := InactiveColorList[len(InactiveColors)%len(InactiveColorList)]
			InactiveColors[t.Status] = col
			return col
		}
	}
	return prefix
}

func (self *CommandTodo) ShowDetails(core *Core, filter string, index int, prefix *string) {

	self.Selected = index
	//SendReceiveRpc(core, "Db.QueryFullTodo", &self.Filtered[index].Hash, &self.TaskReply)
	params := map[string]string{}
	path := fmt.Sprintf("todofull/%s", b64.URLEncoding.EncodeToString([]byte(self.Filtered[index].Hash)))
	SendReceiveGet(core, path, params, &self.TaskReply)
	core.taskPane.text.Clear()
	core.taskPane.text.SetTextColor(tcell.ColorWhite).SetTextAlign(tview.AlignLeft)
	core.taskPane.text.SetDynamicColors(true)
	core.taskPane.text.SetBorder(true)
	*prefix = GetStatusColor(&self.Filtered[index])
	core.taskPane.text.SetTitle(*prefix + self.Filtered[index].Status + "[white] " + self.TaskReply.Headline)
	core.taskPane.text.SetText(FormatText(self.TaskReply.Content))
}

func (self *CommandTodo) Filter(core *Core, filter string) {
	core.projectPane.list.Clear()
	self.Filtered = []common.Todo{}
	for _, v := range self.Reply {
		if strings.Contains(v.Headline, filter) {
			self.Filtered = append(self.Filtered, v)
		}
	}
	for _, v := range self.Filtered {
		prefix := GetStatusColor(&v)
		item := core.projectPane.list.AddItem(prefix+v.Status+"[white] "+v.Headline, strings.Join(v.Tags, ","), 0, nil)
		self.ShowDetails(core, filter, 0, &prefix)
		item.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
			if index < len(self.Filtered) {
				self.ShowDetails(core, filter, index, &prefix)
			}
		})

		item.SetSelectedFunc(func(index int, mainText string, secText string, shortcut rune) {

			// 0 offset or 1 offset needs to be handled
			LaunchEditor(self.Filtered[index].Filename, self.Filtered[index].LineNum+1)
			core.statusBar.showForSeconds("STAT: "+fmt.Sprintf("%d", self.Filtered[index].LineNum)+" "+self.Filtered[index].Headline, 5)
		})
		/*
			item.SetSelectedFunc(func(index int, mainText string, secText string, shortcut rune) {
				self.Selected = index
				send := common.TodoItemChange{Hash: self.Reply[index].Hash, Value: "DONE"}
				var reply common.Result = common.Result{}
				SendReceiveRpc(core, "Db.ChangeStatus", &send, &reply)
				res := "Ok"
				if !reply.Ok {
					res = "FAILED"
				}
				core.statusBar.showForSeconds("STATE: "+self.Reply[index].Headline+fmt.Sprint("%s", res), 5)
			})
		*/
	}
}

func (self *CommandTodo) EnterTasks(core *Core, params []string) {
	core.taskPane.list.Clear()
	core.projectPane.list.Clear()
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	core.projectPane.SetTitle("[::u]<P>[::-] " + self.GetName())

	self.Filter(core, "")
	/*
		for _, v := range self.Reply {
			prefix := GetStatusColor(&v)
			item := core.projectPane.list.AddItem(prefix+v.Status+"[white] "+v.Headline, strings.Join(v.Tags, ","), 0, nil)
			item.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
				if index < len(self.Reply) {
					self.Selected = index
					//core.statusBar.showForSeconds("STAT: "+self.Reply[index].Headline, 5)
					//self.Error = core.ws.Call("Db.QuerySpecificTodo", self.Query, &self.TaskReply)
					SendReceiveRpc(core, "Db.QueryFullTodo", &self.Reply[index].Hash, &self.TaskReply)
					//self.Error = core.ws.Call("Db.QueryFullTodo", self.Reply[index].Hash, &self.TaskReply)
					//core.taskPane.list.Clear()
					core.taskPane.text.Clear()
					core.taskPane.text.SetTextColor(tcell.ColorWhite).SetTextAlign(tview.AlignLeft)
					core.taskPane.text.SetDynamicColors(true)
					core.taskPane.text.SetBorder(true)
					prefix = GetStatusColor(&self.Reply[index])
					//core.taskPane.list.AddItem(self.TaskReply.Headline, "", 0, nil)
					core.taskPane.text.SetTitle(prefix + self.Reply[index].Status + "[white] " + self.TaskReply.Headline)
					core.taskPane.text.SetText(FormatText(self.TaskReply.Content))
				}
			})

			item.SetSelectedFunc(func(index int, mainText string, secText string, shortcut rune) {

				// 0 offset or 1 offset needs to be handled
				LaunchEditor(self.Reply[index].Filename, self.Reply[index].LineNum+1)
				core.statusBar.showForSeconds("STAT: "+fmt.Sprintf("%d", self.Reply[index].LineNum)+" "+self.Reply[index].Headline, 5)
			})
			/*
				item.SetSelectedFunc(func(index int, mainText string, secText string, shortcut rune) {
					self.Selected = index
					send := common.TodoItemChange{Hash: self.Reply[index].Hash, Value: "DONE"}
					var reply common.Result = common.Result{}
					SendReceiveRpc(core, "Db.ChangeStatus", &send, &reply)
					res := "Ok"
					if !reply.Ok {
						res = "FAILED"
					}
					core.statusBar.showForSeconds("STATE: "+self.Reply[index].Headline+fmt.Sprint("%s", res), 5)
				})
	*/
	//}
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

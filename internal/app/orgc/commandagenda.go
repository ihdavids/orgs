package orgc

import (
	//"strings"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	//"github.com/gdamore/tcell/v2"
	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
)

type CommandAgenda struct {
	Reply     common.Todos
	TaskReply common.FullTodo
	Error     error
	CurDate   time.Time
	Core      *Core
	Selected  int
}

func NewCommandAgenda() {
	var todo *CommandAgenda = new(CommandAgenda)
	todo.CurDate = time.Now()
	todo.Selected = 0
	GetCmdRegistry().RegisterCommand("agenda", todo)
}

func (self *CommandAgenda) GetName() string {
	return "agenda"
}

func (self *CommandAgenda) GetDescription() string {
	return "return todays agenda"
}

func (self *CommandAgenda) BuildAgendaBlocks(v common.Todo) string {
	// TODO
	return ""
}

func (self *CommandAgenda) BuildDeadlineDisplay(v common.Todo) string {
	// D: Overdue
	// D: Due Today
	// D: @DATE
	return ""
}

func (self *CommandAgenda) BuildHabitDisplay(v common.Todo) string {
	//  habitbar = "[_____________________]"
	return ""
}

func FileNameWithoutExt(fileName string) string {
	return strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
}

func (self *CommandAgenda) RenderAgendaEntry(v common.Todo, index int) string {
	fname := FileNameWithoutExt(v.Filename)
	if len(fname) > 14 {
		fname = fname[:14]
	}
	fname += ":"
	h := v.Date.Start.Hour()
	m := v.Date.Start.Minute()
	todo := "    "
	if v.Status != "" {
		todo = v.Status
		color := "red"
		if c, ok := Conf().AgendaStatusColors[todo]; ok {
			color = c
		}
		if len(v.Status) > 4 {
			todo = v.Status[:4]
		}
		todo = "[" + color + "]" + todo
	}
	if self.Selected == index {
		return fmt.Sprintf("[%s]     %-15s [white:yellow]%02d:%02d[:none] %-8s %s [%s]%-45s %s%s\n", Conf().AgendaFilenameColor, fname, h, m, self.BuildAgendaBlocks(v), todo, Conf().AgendaTextColor, v.Headline, self.BuildDeadlineDisplay(v), self.BuildHabitDisplay(v))
	} else {
		return fmt.Sprintf("[%s]     %-15s [white:bu]%02d:%02d %-8s %s [%s]%-45s %s%s\n", Conf().AgendaFilenameColor, fname, h, m, self.BuildAgendaBlocks(v), todo, Conf().AgendaTextColor, v.Headline, self.BuildDeadlineDisplay(v), self.BuildHabitDisplay(v))
	}
}

func (self *CommandAgenda) HandleShortcuts(event *tcell.EventKey) *tcell.EventKey {
	switch unicode.ToLower(event.Rune()) {
	case '.':
		self.CurDate = self.CurDate.AddDate(0, 0, 1)
		self.ShowAgendaPane(self.Core)
		return nil
	case ',':
		self.CurDate = self.CurDate.AddDate(0, 0, -1)
		self.ShowAgendaPane(self.Core)
		return nil
	case 'j':
		self.Selected += 1
		if self.Selected >= len(self.Reply) {
			self.Selected = len(self.Reply)
		}
		self.ShowAgendaPane(self.Core)
		return nil
	case 'k':
		self.Selected -= 1
		if self.Selected <= 0 {
			self.Selected = 0
		}
		self.ShowAgendaPane(self.Core)
		return nil
	case 'n':
		self.CurDate = time.Now()
		self.ShowAgendaPane(self.Core)
		return nil
	}
	if event.Key() == tcell.KeyEnter {
		self.ShowAgendaPane(self.Core)

		if self.Selected > 0 {
			LaunchEditor(self.Reply[self.Selected-1].Filename, self.Reply[self.Selected-1].LineNum+1)
		}
		return nil
	}
	return event
}
func (self *CommandAgenda) ShowAgendaPane(core *Core) {
	self.Core = core
	query := new(common.StringQuery)
	query.Query = fmt.Sprintf(`!IsProject() && !IsArchived() && IsTodo() && OnDate("%s")`, self.CurDate.Format("2006 02 01"))
	//self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
	self.Reply = common.Todos{}
	SendReceiveRpc(core, "Db.QueryTodosExp", &query, &self.Reply)
	core.taskPane.text.Clear()
	core.projectPane.list.Clear()
	core.taskPane.text.SetDynamicColors(true)
	core.taskPane.text.SetTextAlign(tview.AlignLeft)
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		//core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	core.projectPane.SetTitle(fmt.Sprintf("[::u]<P>[::-] %s [%d]", self.GetName(), len(self.Reply)))
	tm := self.CurDate
	txt := "     [blue]" + tm.Format("Monday 02 January 2006") + "\n\n"
	start := 8
	end := 20
	for i := start; i < end; i += 1 {
		displayTime := true
		for _, v := range self.Reply {
			if v.Date.Start.Hour() == i && v.Date.Start.Minute() == 0 {
				displayTime = false
			}
		}
		if displayTime {
			txt += fmt.Sprintf("                     [grey]%02d:00 ........ ---------------------------\n", i)
		}
		index := 0
		for _, v := range self.Reply {
			if v.Date.Start.Hour() == i {
				index += 1
				txt += self.RenderAgendaEntry(v, index)
			}
		}
	}
	core.taskPane.text.SetText(txt)
}

func (self *CommandAgenda) Enter(core *Core)         {}
func (self *CommandAgenda) EnterProjects(core *Core) {}
func (self *CommandAgenda) EnterTasks(core *Core) {
	self.ShowAgendaPane(core)
}

func (self *CommandAgenda) Execute(core *Core) {}

func (self *CommandAgenda) ExitTasks(core *Core)    {}
func (self *CommandAgenda) ExitProjects(core *Core) {}
func (self *CommandAgenda) Exit(core *Core)         {}

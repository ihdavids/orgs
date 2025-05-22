package agenda

import (
	//"strings"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	//"github.com/gdamore/tcell/v2"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
)

type CommandAgenda struct {
	Reply               common.Todos
	TaskReply           common.FullTodo
	Error               error
	CurDate             time.Time
	Selected            int
	sym                 []string
	tophalfsym          []string
	bothalfsym          []string
	symUsed             []int
	blocks              []*common.Todo
	AgendaTextColor     string
	AgendaFilenameColor string
	AgendaStatusColors  map[string]string
	AgendaBlockColors   []string
	out                 *tview.TextView
	statusBar           *tview.Table
	core                *commands.Core
}

func NewCommandAgenda() *CommandAgenda {
	var todo *CommandAgenda = new(CommandAgenda)
	todo.CurDate = time.Now()
	todo.Selected = 0
	todo.ClearBlockView()
	todo.AgendaTextColor = "yellow"
	todo.AgendaFilenameColor = "darkcyan"
	todo.AgendaStatusColors = make(map[string]string)
	todo.AgendaStatusColors["TODO"] = "pink"
	todo.AgendaStatusColors["DONE"] = "green"
	todo.AgendaStatusColors["PHONE"] = "magenta"
	todo.AgendaStatusColors["NEXT"] = "blue"
	todo.AgendaStatusColors["WAITING"] = "orange"
	todo.AgendaStatusColors["BLOCKED"] = "red"
	todo.AgendaStatusColors["CANCELLED"] = "grey"
	todo.AgendaBlockColors = []string{"red", "green", "blue", "darkcyan", "orange", "grey", "magenta", "white", "yellow"}
	//todo.EditorTemplate = []string{"code", "-g", "{filename}:{linenum}"}
	return todo
}

func (self *CommandAgenda) ClearBlockView() {
	self.sym = []string{"[#28363d]█", "[#2f575d]█", "[#843b62]█", "[#6d9197]█", "[#99aead]█", "[#474044]█", "[#293132]█", "[#c4cdc1]█", "[#dee1dd]█"}
	// TODO: Improve algorithm to make blocks show 1/2 for half hour overlap
	self.tophalfsym = []string{"[#28363d]▀", "[#2f575d]▀", "[#843b62]▀", "[#6d9197]▀", "[#99aead]▀", "[#474044]▀", "[#293132]▀", "[#c4cdc1]▀", "[#dee1dd]▀"}
	self.bothalfsym = []string{"[#28363d]▄", "[#2f575d]▄", "[#843b62]▄", "[#6d9197]▄", "[#99aead]▄", "[#474044]▄", "[#293132]▄", "[#c4cdc1]▄", "[#dee1dd]▄"}
	self.symUsed = []int{-1, -1, -1, -1, -1, -1, -1}
	self.blocks = []*common.Todo{nil, nil, nil, nil, nil, nil, nil}
}

// We search our list of symbols for one that has yet to have been used
// for this time slot
func (self *CommandAgenda) GetUnusedSymbol(blk int) int {
	start := 0
	for i := 0; i < len(self.symUsed); i++ {
		if self.symUsed[i] >= 0 {
			start = i
			break
		}
	}
	for i := start; i < len(self.symUsed); i++ {
		if self.symUsed[i] < 0 {
			self.symUsed[i] = blk
			return i
		}
	}
	return -1
}

func (self *CommandAgenda) ReleaseSymbol(blk int) {
	for i := 0; i < len(self.symUsed); i++ {
		if self.symUsed[i] == blk {
			self.symUsed[i] = -1
			break
		}
	}
}

func (self *CommandAgenda) FindSymbol(blk int) int {
	for i := 0; i < len(self.symUsed); i++ {
		if self.symUsed[i] == blk {
			return i
		}
	}
	return 0
}

func Overlaps(s int, e int, rs int, re int) bool {
	//   | s e |
	// +---+
	if s <= rs && e >= rs && e <= re {
		return true
	}
	// | s e |
	//    +---+
	if s >= rs && s < re && e >= re {
		return true
	}
	// | s  e |
	//   +-+
	if s >= rs && e <= re {
		return true
	}
	// s |    | e
	// +-------+
	if s <= rs && e >= re {
		return true
	}
	return false
}

func IsInHourBracket(start time.Time, end time.Time, hour int) bool {
	if end.IsZero() {
		// TODO: Make this configurable
		end = start.Add(30 * time.Minute)
	}
	return Overlaps(start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute(), hour*60, hour*60+59)
}

func IsInHour(v *common.Todo, hour int, now time.Time) bool {
	if v == nil || v.Date.IsZero() || v.Date.TimestampType != org.Active {
		return false
	}
	// TODO: Handle repeating!
	if IsInHourBracket(v.Date.Start, v.Date.End, hour) {
		return true
	}
	// TODO: Handle scheduled
	// TODO: Handle deadline
	return false
}

func (self *CommandAgenda) ClearAgendaBlocks(hour int) {
	for i := 0; i < len(self.blocks); i++ {
		v := self.blocks[i]
		if !IsInHour(v, hour, self.CurDate) {
			self.ReleaseSymbol(i)
			self.blocks[i] = nil
		}
	}
}

func (self *CommandAgenda) UpdateWithThisBlock(v *common.Todo, hour int) int {
	idx := -1
	for i := 0; i < len(self.blocks); i++ {
		if idx == -1 && self.blocks[i] == nil {
			idx = i
		}
		if self.blocks[i] == v {
			idx = -1
			return i
		}
	}
	if idx != -1 {
		self.blocks[idx] = v
		return idx
	}
	return 0
}

func (self *CommandAgenda) BuildAgendaBlocks(v *common.Todo, hour int) string {
	out := ""
	if v != nil {
		symIdx := self.GetUnusedSymbol(0)
		self.ClearAgendaBlocks(hour)
		myIdx := self.UpdateWithThisBlock(v, hour)
		self.symUsed[symIdx] = myIdx
	} else {
		self.ClearAgendaBlocks(hour)
	}

	spaceSym := "."
	for i := 0; i < len(self.blocks); i++ {
		if self.blocks[i] != nil {
			spaceSym = " "
		}
	}
	if spaceSym == "." {
		out = ".."
	}
	for i := 0; i < len(self.blocks); i++ {
		if self.blocks[i] == nil {
			out = out + spaceSym
		} else {
			symIdx := self.FindSymbol(i)
			out = out + self.sym[symIdx]
		}
	}
	return out
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
		if c, ok := self.AgendaStatusColors[todo]; ok {
			color = c
		}
		if len(v.Status) > 4 {
			todo = v.Status[:4]
		}
		todo = "[" + color + "]" + todo
	}
	if self.Selected == index {
		return fmt.Sprintf("[%s]     %-15s [white:yellow]%02d:%02d[:none] %-8s %s [%s]%-45s %s%s\n", self.AgendaFilenameColor, fname, h, m, self.BuildAgendaBlocks(&v, h), todo, self.AgendaTextColor, v.Headline, self.BuildDeadlineDisplay(v), self.BuildHabitDisplay(v))
	} else {
		return fmt.Sprintf("[%s]     %-15s [green:bu]%02d:%02d %-8s %s [%s]%-45s %s%s\n", self.AgendaFilenameColor, fname, h, m, self.BuildAgendaBlocks(&v, h), todo, self.AgendaTextColor, v.Headline, self.BuildDeadlineDisplay(v), self.BuildHabitDisplay(v))
	}
}

func (self *CommandAgenda) HandleShortcuts(event *tcell.EventKey) *tcell.EventKey {
	switch unicode.ToLower(event.Rune()) {
	case '.':
		self.CurDate = self.CurDate.AddDate(0, 0, 1)
		self.ShowAgendaPane(self.core)
		return nil
	case ',':
		self.CurDate = self.CurDate.AddDate(0, 0, -1)
		self.ShowAgendaPane(self.core)
		return nil
	case 'j':
		self.Selected += 1
		if self.Selected >= len(self.Reply) {
			self.Selected = len(self.Reply)
		}
		self.ShowAgendaPane(self.core)
		return nil
	case 'k':
		self.Selected -= 1
		if self.Selected <= 0 {
			self.Selected = 0
		}
		self.ShowAgendaPane(self.core)
		return nil
	case 'n':
		self.CurDate = time.Now()
		self.ShowAgendaPane(self.core)
		return nil
	}
	if event.Key() == tcell.KeyEnter {
		self.ShowAgendaPane(self.core)

		if self.Selected > 0 {
			//LaunchEditor(self.Reply[self.Selected-1].Filename, self.Reply[self.Selected-1].LineNum+1)
		}
		return nil
	}
	return event
}

func (self *CommandAgenda) GetSelectedHash() string {
	if self.Selected >= 0 && self.Selected < len(self.Reply) {
		return self.Reply[self.Selected].Hash
	}
	return ""
}

func (self *CommandAgenda) ShowAgendaPane(core *commands.Core) {
	self.out.Clear()
	//self.Core = core
	params := map[string]string{
		"query": fmt.Sprintf(`!IsProject() && !IsArchived() && IsTodo() && OnDate("%s")`, self.CurDate.Format("2006 02 01")),
	}
	//self.Error = core.ws.Call("Db.QueryTodosExp", self.Query, &self.Reply)
	self.Reply = common.Todos{}
	commands.SendReceiveGet(core, "search", params, &self.Reply)
	///SendReceiveRpc(core, "Db.QueryTodosExp", &query, &self.Reply)
	//core.taskPane.text.Clear()
	//core.projectPane.list.Clear()
	self.out.SetDynamicColors(true)
	self.out.SetTextAlign(tview.AlignLeft)
	if self.Error != nil {
		//pane.list.AddItem("- Today", "", 0, func() { taskPane.LoadDynamicList("today") })
		//core.taskPane.list.AddItem("ERROR - could not query data", "", 0, nil)
	}
	self.out.SetTitle(fmt.Sprintf("[::u]<P>[::-] %s [%d]", "Agenda", len(self.Reply)))
	//fmt.Printf("[::u]<P>[::-] %s [%d]", "Agenda", len(self.Reply))
	tm := self.CurDate
	txt := "     [blue]" + tm.Format("Monday 02 January 2006") + "\n\n"
	start := 8
	end := 20
	index := 0
	now := time.Now()
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
		if now.Year() == self.CurDate.Year() && now.Month() == self.CurDate.Month() && now.Day() == self.CurDate.Day() && now.Hour() == i {
			txt += fmt.Sprintf("     [#ee00ee]%-15s %02d:%02d - - - - - - - - - - - - - - - - - - - - - \n", "now =>", now.Hour(), now.Minute())
		}
		for _, v := range self.Reply {
			if v.Date.Start.Hour() == i {
				index += 1
				txt += self.RenderAgendaEntry(v, index)
			}
		}
	}
	self.out.SetText(txt)
	//fmt.Printf(txt)
}

/*
	func (self *CommandAgenda) EnterTasks(core *Core, params []string) {
		self.ShowAgendaPane(core)
	}
*/
func (self *CommandAgenda) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *CommandAgenda) SetupParameters(*flag.FlagSet) {
}

func (self *CommandAgenda) Exec(core *commands.Core) {
	//fmt.Printf("CommandAgenda called\n")
	//box := tview.NewBox().SetBorder(true).SetTitle("Agenda")
	self.out = tview.NewTextView()
	self.statusBar = tview.NewTable()
	self.statusBar.SetCell(0, 0, tview.NewTableCell(",:Prev"))
	self.statusBar.SetCell(0, 1, tview.NewTableCell(",:Next"))
	self.statusBar.SetCell(0, 2, tview.NewTableCell("n:Today"))
	self.core = core
	app := tview.NewApplication()

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(self.out, 0, 1, true).
		AddItem(self.statusBar, 1, 1, false)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return self.HandleShortcuts(event)
	})
	self.ShowAgendaPane(core)
	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
	/*
		var qry map[string]string = map[string]string{}
		//qry["filename"] = "./out.html"
		//qry["query"] = "IsTask() && HasProperty(\"EFFORT\")"
		var reply common.FileList = common.FileList{}

		//func SendReceiveGet[RPC any, RESP any](core *Core, name string, args *RPC, resp *RESP) {
		commands.SendReceiveGet(core, "files", qry, &reply)
		//commands.SendReceiveRpc(core, "Db.ExportToFile", &query, &reply)
		if reply != nil {
			fmt.Printf("OK")
			for _, file := range reply {
				fmt.Printf("%s\n", file)
			}
		} else {
			fmt.Printf("Err")
		}*/
}

func init() {
	commands.AddCmd("agenda", "Show configured agenda",
		func() commands.Cmd {
			return NewCommandAgenda()
		})
}

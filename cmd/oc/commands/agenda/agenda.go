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
	weekView            *tview.Table
	statusBar           *tview.Table
	allTodos            common.Todos
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
	if v.Props == nil || v.Props["STYLE"] != "habit" {
		return ""
	}
	// Determine repeat interval in days from Deadline or Date
	intervalDays := 1
	src := v.Deadline
	if src == nil {
		src = v.Date
	}
	if src != nil && src.RepeatDWMY != "" {
		// Parse repeat interval: RepeatPre is like "+1", ".+2" etc, RepeatDWMY is "d","w","m","y"
		n := 1
		pre := src.RepeatPre
		// Strip leading . or + to get the number
		pre = strings.TrimLeft(pre, ".+")
		if len(pre) > 0 {
			if v, err := fmt.Sscanf(pre, "%d", &n); err == nil && v > 0 {
				intervalDays = n
			}
		}
		switch src.RepeatDWMY {
		case "w":
			intervalDays *= 7
		case "m":
			intervalDays *= 30
		case "y":
			intervalDays *= 365
		}
	}

	// Build set of completion dates
	doneSet := make(map[string]bool)
	for _, c := range v.Completions {
		doneSet[c] = true
	}

	today := time.Now().Truncate(24 * time.Hour)
	var bar strings.Builder
	bar.WriteString(" [white][[-:-]")
	for i := 20; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		dayStr := day.Format("2006-01-02")
		done := doneSet[dayStr]

		// Find days since last completion on or before this day
		daysSinceLast := -1
		for j := 0; j <= 21; j++ {
			check := day.AddDate(0, 0, -j)
			if doneSet[check.Format("2006-01-02")] {
				daysSinceLast = j
				break
			}
		}

		// Determine color based on how overdue the habit is
		color := "green" // due / on track
		if daysSinceLast >= 0 && daysSinceLast < intervalDays {
			color = "blue" // too early
		} else if daysSinceLast > intervalDays+intervalDays/2 {
			color = "red" // overdue
		} else if daysSinceLast > intervalDays {
			color = "yellow" // nearly overdue
		}

		sym := " "
		if i == 0 {
			sym = "!"
		} else if done {
			sym = "*"
		}
		bar.WriteString(fmt.Sprintf("[%s]%s", color, sym))
	}
	bar.WriteString("[white]][-]")
	return bar.String()
}

func FileNameWithoutExt(fileName string) string {
	return strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
}

func (self *CommandAgenda) RenderAllDayEntry(v common.Todo, index int) string {
	fname := FileNameWithoutExt(v.Filename)
	if len(fname) > 14 {
		fname = fname[:14]
	}
	fname += ":"
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
	habit := self.BuildHabitDisplay(v)
	if self.Selected == index {
		return fmt.Sprintf("[%s]     %-15s [white:yellow]      [:none] %-8s %s [%s]%-45s %s%s\n", self.AgendaFilenameColor, fname, "", todo, self.AgendaTextColor, v.Headline, self.BuildDeadlineDisplay(v), habit)
	}
	return fmt.Sprintf("[%s]     %-15s [green]      %-8s %s [%s]%-45s %s%s\n", self.AgendaFilenameColor, fname, "", todo, self.AgendaTextColor, v.Headline, self.BuildDeadlineDisplay(v), habit)
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

// fetchAllTodos queries for all active todos and caches the result.
// Returns a set of date keys ("2006-01-02") that have at least one agenda item.
func (self *CommandAgenda) fetchAllTodos(core *commands.Core) map[string]bool {
	params := map[string]string{
		"query": `!IsProject() && !IsArchived() && IsTodo()`,
	}
	self.allTodos = common.Todos{}
	commands.SendReceiveGet(core, "search", params, &self.allTodos)
	days := make(map[string]bool)
	for _, t := range self.allTodos {
		if t.Date != nil && !t.Date.Start.IsZero() {
			days[t.Date.Start.Format("2006-01-02")] = true
		}
	}
	return days
}

// renderWeekTable populates the week table with 7 day columns showing
// color-coded tasks for the week containing CurDate.
func (self *CommandAgenda) renderWeekTable() {
	self.weekView.Clear()
	today := time.Now()

	// Find start of week (Sunday)
	weekday := int(self.CurDate.Weekday())
	weekStart := self.CurDate.AddDate(0, 0, -weekday)

	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

	// Header row
	for col := 0; col < 7; col++ {
		day := weekStart.AddDate(0, 0, col)
		header := fmt.Sprintf(" %s %d ", dayNames[col], day.Day())
		cell := tview.NewTableCell(header).
			SetAlign(tview.AlignCenter).
			SetExpansion(1).
			SetSelectable(false)

		isToday := day.Year() == today.Year() && day.Month() == today.Month() && day.Day() == today.Day()
		isCurDay := day.Year() == self.CurDate.Year() && day.Month() == self.CurDate.Month() && day.Day() == self.CurDate.Day()

		switch {
		case isToday:
			cell.SetBackgroundColor(tcell.ColorBlue).SetTextColor(tcell.ColorWhite)
		case isCurDay:
			cell.SetBackgroundColor(tcell.ColorDarkCyan).SetTextColor(tcell.ColorWhite)
		default:
			cell.SetTextColor(tcell.ColorGrey)
		}
		self.weekView.SetCell(0, col, cell)
	}

	// Group cached todos by day key
	dayTodos := make(map[string][]common.Todo)
	for _, t := range self.allTodos {
		if t.Date != nil && !t.Date.Start.IsZero() {
			key := t.Date.Start.Format("2006-01-02")
			dayTodos[key] = append(dayTodos[key], t)
		}
	}

	// Find max rows across the week
	maxRows := 0
	for col := 0; col < 7; col++ {
		day := weekStart.AddDate(0, 0, col)
		key := day.Format("2006-01-02")
		if n := len(dayTodos[key]); n > maxRows {
			maxRows = n
		}
	}

	// Fill task cells
	for col := 0; col < 7; col++ {
		day := weekStart.AddDate(0, 0, col)
		key := day.Format("2006-01-02")
		todos := dayTodos[key]

		for row, t := range todos {
			timeStr := t.Date.Start.Format("15:04")
			headline := t.Headline
			label := timeStr + " " + headline

			color := tcell.ColorWhite
			if t.Status != "" {
				if c, ok := self.AgendaStatusColors[t.Status]; ok {
					color = tcell.GetColor(c)
				}
			}

			cell := tview.NewTableCell(label).
				SetTextColor(color).
				SetExpansion(1).
				SetAlign(tview.AlignLeft)
			self.weekView.SetCell(row+1, col, cell)
		}

		// Fill empty rows so columns line up
		for row := len(todos); row < maxRows; row++ {
			self.weekView.SetCell(row+1, col, tview.NewTableCell("").SetExpansion(1))
		}
	}
}

// renderMonthLines renders a single month calendar as lines of tview-tagged text.
// Each line has exactly 21 visible characters (7 columns x 3 chars).
func renderMonthLines(year int, month time.Month, today time.Time, activeDays map[string]bool) []string {
	lines := make([]string, 0, 9)

	// Centered month/year header
	header := fmt.Sprintf("%s %d", month.String(), year)
	pad := (21 - len(header)) / 2
	if pad < 0 {
		pad = 0
	}
	trail := 21 - pad - len(header)
	if trail < 0 {
		trail = 0
	}
	lines = append(lines, strings.Repeat(" ", pad)+"[::b]"+header+"[::-]"+strings.Repeat(" ", trail))

	// Day-of-week header
	lines = append(lines, "[grey] Su Mo Tu We Th Fr Sa[-]")

	// First day and number of days
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	startDow := int(first.Weekday())
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()

	line := ""
	col := 0

	// Leading blank cells
	for i := 0; i < startDow; i++ {
		line += "   "
		col++
	}

	for day := 1; day <= daysInMonth; day++ {
		dateKey := fmt.Sprintf("%04d-%02d-%02d", year, int(month), day)
		isToday := today.Year() == year && today.Month() == month && today.Day() == day
		hasItems := activeDays[dateKey]

		dayStr := fmt.Sprintf("%2d", day)
		switch {
		case isToday && hasItems:
			line += fmt.Sprintf(" [black:green]%s[-:-]", dayStr)
		case isToday:
			line += fmt.Sprintf(" [white:blue]%s[-:-]", dayStr)
		case hasItems:
			line += fmt.Sprintf(" [green]%s[-]", dayStr)
		default:
			line += " " + dayStr
		}
		col++

		if col == 7 {
			lines = append(lines, line)
			line = ""
			col = 0
		}
	}

	// Pad the last partial week with trailing blanks
	if col > 0 {
		for col < 7 {
			line += "   "
			col++
		}
		lines = append(lines, line)
	}

	// Pad to 9 lines so all months have the same height
	blank := strings.Repeat(" ", 21)
	for len(lines) < 9 {
		lines = append(lines, blank)
	}
	return lines
}

// renderThreeMonthCalendar renders prev/current/next month side by side with
// today highlighted and days with agenda items colored.
func renderThreeMonthCalendar(curDate time.Time, activeDays map[string]bool) string {
	today := time.Now()
	prev := curDate.AddDate(0, -1, 0)
	next := curDate.AddDate(0, 1, 0)

	left := renderMonthLines(prev.Year(), prev.Month(), today, activeDays)
	center := renderMonthLines(curDate.Year(), curDate.Month(), today, activeDays)
	right := renderMonthLines(next.Year(), next.Month(), today, activeDays)

	var sb strings.Builder
	sb.WriteString("\n")
	for i := 0; i < len(left); i++ {
		sb.WriteString(fmt.Sprintf("  %s    %s    %s\n", left[i], center[i], right[i]))
	}
	sb.WriteString("\n")
	return sb.String()
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
	activeDays := self.fetchAllTodos(core)
	tm := self.CurDate
	txt := renderThreeMonthCalendar(tm, activeDays)
	txt += "     [blue]" + tm.Format("Monday 02 January 2006") + "\n\n"
	start := 8
	end := 20
	index := 0
	now := time.Now()
	// Render all-day / untimed entries (habits, scheduled items without a time)
	for _, v := range self.Reply {
		if v.Date == nil || !v.Date.HaveTime {
			index += 1
			txt += self.RenderAllDayEntry(v, index)
		}
	}
	if index > 0 {
		txt += "                     [grey]------------------------------------------\n"
	}
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
			if v.Date != nil && v.Date.HaveTime && v.Date.Start.Hour() == i {
				index += 1
				txt += self.RenderAgendaEntry(v, index)
			}
		}
	}
	self.out.SetText(txt)
	self.renderWeekTable()
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

func (self *CommandAgenda) StartPlugin(manager *common.PluginManager) {
}

func (self *CommandAgenda) Exec(core *commands.Core) {
	//fmt.Printf("CommandAgenda called\n")
	//box := tview.NewBox().SetBorder(true).SetTitle("Agenda")
	self.out = tview.NewTextView()
	self.weekView = tview.NewTable()
	self.weekView.SetBorder(true).SetTitle(" Week ")
	self.weekView.SetBorders(false)
	self.weekView.SetFixed(1, 0)
	self.statusBar = tview.NewTable()
	self.statusBar.SetCell(0, 0, tview.NewTableCell(",:Prev"))
	self.statusBar.SetCell(0, 1, tview.NewTableCell(",:Next"))
	self.statusBar.SetCell(0, 2, tview.NewTableCell("n:Today"))
	self.core = core
	app := tview.NewApplication()

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(self.out, 0, 3, true).
		AddItem(self.weekView, 0, 2, false).
		AddItem(self.statusBar, 1, 0, false)
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

package tui

// `orgs tui` — one screen you can live in.
//
//	orgs tui                          the default query
//	orgs tui 'IsTask() && Today()'    start on something else
//	orgs tui -query '{{ WorkTasks }}' a filter from the yaml
//
// The other commands answer a question and stop, which is right for a pipe and
// wrong for the thing people actually do: read down a list, open one, come
// back, change a keyword, narrow it, read down it again. That loop is what
// this is.
//
// ---------------------------------------------------------------------------
// Two boxes, and why they are two
//
// The **query** (`:`) is the server's expression and costs a request. The
// **filter** (`/`) narrows what is already on screen and costs nothing. They
// look alike and are not remotely alike: a query can say IsStatus("NEXT") and
// cannot say "the one about the invoice"; a filter is the other way round.
// Keeping them apart is what lets the filter run on every keystroke - which is
// the whole reason it exists - without a round trip per letter.
//
// The filter is `internal/common/dnd/fuzzy.go`, the same matcher the dnd
// chooser and worg's palette use, so the letters that find a heading in one of
// them find it here.
//
// ---------------------------------------------------------------------------
// One rule about writing
//
// The keyword menu asks `/status/{hash}` rather than offering the keywords the
// last query happened to turn up: a heading's own file may declare its own
// `#+TODO:`, and offering a keyword that file does not have is offering to
// write something org will not read back. Same rule worg's kanban follows.

import (
	"flag"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd"
	"github.com/rivo/tview"
)

const defaultQuery = `!IsArchived() && IsTodo()`

type Tui struct {
	fset *flag.FlagSet

	Query string

	app     *tview.Application
	pages   *tview.Pages
	table   *tview.Table
	head    *tview.TextView
	status  *tview.TextView
	preview *tview.TextView
	layout  *tview.Flex
	input   *tview.InputField

	core    *commands.Core
	all     []common.Todo // what the query returned
	rows    []common.Todo // what is on screen after the filter
	filter  string
	showing bool // is the preview pane up
	err     string
}

func (self *Tui) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Tui) StartPlugin(manager *common.PluginManager)         {}

func (self *Tui) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Query, "query", defaultQuery, "the expression to start on")
}

func (self *Tui) Exec(core *commands.Core) {
	if free := commands.FreeText(self.fset); free != "" {
		self.Query = free
	}
	if commands.Machine() {
		commands.Fail("orgs tui is a screen, not an answer — use `orgs search` for -json and -format")
	}
	self.core = core
	self.build()
	self.reload()
	if err := self.app.SetRoot(self.pages, true).EnableMouse(true).Run(); err != nil {
		commands.Fail("orgs tui: %v", err)
	}
}

// ---------------------------------------------------------------------------
// The screen
// ---------------------------------------------------------------------------

func (self *Tui) build() {
	self.app = tview.NewApplication()

	self.head = tview.NewTextView().SetDynamicColors(true)
	self.status = tview.NewTextView().SetDynamicColors(true)
	self.preview = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	self.preview.SetBorder(true).SetTitle(" body ")

	self.table = tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	self.table.SetSelectionChangedFunc(func(row, col int) { self.refreshPreview() })

	// The input sits under the header and is only given a height when it is
	// being typed into. A box that is always there is a box that is always
	// taking a line off a list that could use it.
	self.input = tview.NewInputField()

	self.layout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(self.head, 1, 0, false).
		AddItem(self.input, 0, 0, false).
		AddItem(self.table, 0, 1, true).
		AddItem(self.status, 1, 0, false)

	self.pages = tview.NewPages().AddPage("main", self.layout, true, true)
	self.app.SetInputCapture(self.keys)
	self.drawStatus()
}

func (self *Tui) drawHead() {
	n := fmt.Sprintf("%d", len(self.rows))
	if len(self.rows) != len(self.all) {
		n = fmt.Sprintf("%d of %d", len(self.rows), len(self.all))
	}
	line := fmt.Sprintf(" [::b]%s[-:-:-]  [darkcyan]%s[-]", n, tview.Escape(self.Query))
	if self.filter != "" {
		line += fmt.Sprintf("  [yellow]/%s[-]", tview.Escape(self.filter))
	}
	if self.err != "" {
		line += fmt.Sprintf("  [red]%s[-]", tview.Escape(self.err))
	}
	self.head.SetText(line)
}

func (self *Tui) drawStatus() {
	self.status.SetText(
		" [darkcyan]enter[-] open  [darkcyan]t[-] keyword  [darkcyan]/[-] filter  " +
			"[darkcyan]:[-] query  [darkcyan]v[-] body  [darkcyan]r[-] reload  [darkcyan]q[-] quit")
}

// ---------------------------------------------------------------------------
// Data
// ---------------------------------------------------------------------------

func (self *Tui) reload() {
	todos, err := commands.SendReceiveGetErr[common.Todos](self.core, "search",
		map[string]string{"query": self.Query})
	if err != nil {
		// A bad query is an ordinary thing to have typed half of. Say why and
		// keep what was on screen, rather than emptying the list under
		// somebody who is still typing.
		self.err = err.Error()
		self.drawHead()
		return
	}
	self.err = ""
	self.all = []common.Todo(todos)
	self.apply()
}

// apply is the filter: the same fuzzy matcher the dnd chooser and worg's
// palette use, run over the headline, its tags and its file, because "the
// invoice one" and "the one in work.org" are both things people type.
func (self *Tui) apply() {
	if strings.TrimSpace(self.filter) == "" {
		self.rows = self.all
	} else {
		self.rows = self.rows[:0:0]
		for _, t := range self.all {
			if _, ok := dnd.FuzzyScore(self.filter, hay(t)); ok {
				self.rows = append(self.rows, t)
			}
		}
	}
	self.fill()
}

// The base name, never the path. A fuzzy match walks for its letters in order
// and does not care how far apart they are, so an absolute path - which is
// long, is the same for every row, and is full of letters nobody typed -
// matches very nearly anything: with the files under
// /Users/someone/dev/notes, "mig" finds every heading in the database. The
// base name is also what a person means when they type a file at a filter.
func hay(t common.Todo) string {
	return strings.Join([]string{t.Headline, t.Status, strings.Join(t.Tags, " "), base(t.Filename)}, " ")
}

func (self *Tui) fill() {
	self.table.Clear()
	for i, t := range self.rows {
		self.table.SetCell(i, 0, cell(t.Status, statusColour(t.Status)).SetMaxWidth(10))
		pri := ""
		if t.Priority != "" {
			pri = "#" + t.Priority
		}
		self.table.SetCell(i, 1, cell(pri, tcell.ColorOrangeRed).SetMaxWidth(3))
		self.table.SetCell(i, 2, cell(t.Headline, tcell.ColorWhite).SetExpansion(1))
		tags := ""
		if len(t.Tags) > 0 {
			tags = ":" + strings.Join(t.Tags, ":") + ":"
		}
		self.table.SetCell(i, 3, cell(tags, tcell.ColorMediumPurple).SetMaxWidth(24))
		self.table.SetCell(i, 4, cell(commands.DateOf(t), tcell.ColorSteelBlue).SetMaxWidth(12))
		self.table.SetCell(i, 5, cell(base(t.Filename), tcell.ColorGray).SetMaxWidth(22))
	}
	if len(self.rows) > 0 {
		self.table.Select(0, 0)
	}
	self.drawHead()
	self.refreshPreview()
}

func cell(text string, colour tcell.Color) *tview.TableCell {
	return tview.NewTableCell(" " + text).SetTextColor(colour).SetSelectable(true)
}

func statusColour(s string) tcell.Color {
	switch strings.ToUpper(s) {
	case "DONE", "CANCELLED", "CANCELED":
		return tcell.ColorGray
	case "NEXT":
		return tcell.ColorLightGreen
	case "WAITING", "BLOCKED", "HOLD":
		return tcell.ColorGoldenrod
	case "":
		return tcell.ColorDefault
	}
	return tcell.ColorSkyblue
}

func base(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (self *Tui) current() (common.Todo, bool) {
	row, _ := self.table.GetSelection()
	if row < 0 || row >= len(self.rows) {
		return common.Todo{}, false
	}
	return self.rows[row], true
}

// ---------------------------------------------------------------------------
// The body pane
// ---------------------------------------------------------------------------

func (self *Tui) togglePreview() {
	self.showing = !self.showing
	if self.showing {
		self.layout.AddItem(self.preview, 0, 1, false)
	} else {
		self.layout.RemoveItem(self.preview)
	}
	self.refreshPreview()
}

func (self *Tui) refreshPreview() {
	if !self.showing {
		return
	}
	t, ok := self.current()
	if !ok {
		self.preview.SetText("")
		return
	}
	res, err := commands.SendReceiveGetErr[struct {
		Ok   bool
		Msg  string
		Text string
	}](self.core, "body/"+commands.HashPath(t.Hash), nil)
	switch {
	case err != nil:
		self.preview.SetText("[red]" + tview.Escape(err.Error()) + "[-]")
	case !res.Ok && res.Msg != "":
		self.preview.SetText("[red]" + tview.Escape(res.Msg) + "[-]")
	case strings.TrimSpace(res.Text) == "":
		self.preview.SetText("[gray](nothing written under this heading)[-]")
	default:
		self.preview.SetText(tview.Escape(res.Text))
	}
	self.preview.ScrollToBeginning()
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

func (self *Tui) keys(ev *tcell.EventKey) *tcell.EventKey {
	// While a box is being typed into it owns every key, or "q" in the middle
	// of a query would quit.
	if self.app.GetFocus() == self.input {
		return ev
	}
	if self.pages.GetPageCount() > 1 {
		return ev
	}
	switch ev.Key() {
	case tcell.KeyEnter:
		self.open()
		return nil
	case tcell.KeyEscape:
		if self.filter != "" {
			self.filter = ""
			self.apply()
			return nil
		}
		self.app.Stop()
		return nil
	}
	switch ev.Rune() {
	case 'q':
		self.app.Stop()
	case '/':
		self.ask("filter> ", self.filter, func(v string) {
			self.filter = v
			self.apply()
		}, func(v string) {
			// Live: this is the whole reason the filter is not a query.
			self.filter = v
			self.apply()
		})
	case ':':
		self.ask("query> ", self.Query, func(v string) {
			if strings.TrimSpace(v) != "" {
				self.Query = v
				self.filter = ""
				self.reload()
			}
		}, nil)
	case 't':
		self.keyword()
	case 'v':
		self.togglePreview()
	case 'r':
		self.reload()
	case 'g':
		if len(self.rows) > 0 {
			self.table.Select(0, 0)
		}
	case 'G':
		if len(self.rows) > 0 {
			self.table.Select(len(self.rows)-1, 0)
		}
	default:
		return ev
	}
	return nil
}

func (self *Tui) open() {
	t, ok := self.current()
	if !ok {
		return
	}
	// Suspend rather than launch behind the screen: a terminal editor and a
	// tview application both want the terminal, and whichever loses draws over
	// the other. LaunchEditor does not wait, so a windowed editor comes back
	// at once and a terminal one holds the screen until it is done.
	self.app.Suspend(func() { self.core.LaunchEditor(t.Filename, t.LineNum) })
}

// ask puts the input box up with a prompt. done fires on Enter, live on every
// keystroke when it is given.
func (self *Tui) ask(prompt, initial string, done func(string), live func(string)) {
	self.input.SetLabel(prompt).SetText(initial)
	self.input.SetChangedFunc(func(v string) {
		if live != nil {
			live(v)
		}
	})
	finish := func(key tcell.Key) {
		self.layout.ResizeItem(self.input, 0, 0)
		self.input.SetChangedFunc(nil)
		self.app.SetFocus(self.table)
		if key == tcell.KeyEnter {
			done(self.input.GetText())
		}
	}
	self.input.SetDoneFunc(finish)
	self.layout.ResizeItem(self.input, 1, 0)
	self.app.SetFocus(self.input)
}

// keyword offers the keywords this heading's *own file* allows, asked for when
// the menu opens rather than guessed from the ones the query turned up.
func (self *Tui) keyword() {
	t, ok := self.current()
	if !ok {
		return
	}
	res, err := commands.SendReceiveGetErr[common.TodoStatesResult](self.core, "status/"+commands.HashPath(t.Hash), nil)
	if err != nil {
		self.err = err.Error()
		self.drawHead()
		return
	}
	options := append(append([]string{}, res.Active...), res.Done...)
	if len(options) == 0 {
		self.err = "that file does not say what keywords it allows"
		self.drawHead()
		return
	}

	list := tview.NewList().ShowSecondaryText(false)
	for _, k := range options {
		keyword := k
		list.AddItem(keyword, "", 0, func() {
			self.pages.RemovePage("keyword")
			self.app.SetFocus(self.table)
			self.setStatus(t, keyword)
		})
	}
	list.SetBorder(true).SetTitle(" keyword ")
	list.SetDoneFunc(func() {
		self.pages.RemovePage("keyword")
		self.app.SetFocus(self.table)
	})
	self.pages.AddPage("keyword", centred(list, 26, len(options)+2), true, true)
	self.app.SetFocus(list)
}

func (self *Tui) setStatus(t common.Todo, keyword string) {
	req := common.TodoItemChange{Hash: t.Hash, Value: keyword}
	res, err := commands.SendReceivePostErr[common.TodoItemChange, common.Result](self.core, "status/change", &req)
	switch {
	case err == commands.ErrDryRun:
		self.err = "dry run: " + t.Headline + " would become " + keyword
	case err != nil:
		self.err = err.Error()
	case !res.Ok:
		self.err = "the server would not change that one"
	default:
		self.err = ""
		// Show it at once and re-run the query behind it. The write has
		// happened; waiting for the round trip to redraw makes a keypress feel
		// like it did nothing.
		for i := range self.all {
			if self.all[i].Hash == t.Hash {
				self.all[i].Status = keyword
			}
		}
		self.apply()
		self.reload()
		return
	}
	self.drawHead()
}

func centred(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

func init() {
	commands.AddCmd("tui", "one screen: search, read, open and change",
		func() commands.Cmd { return &Tui{Query: defaultQuery} })
}

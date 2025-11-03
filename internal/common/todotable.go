package common

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TodoTable struct {
	preview *tview.TextView
	table   *tview.Table
	layout  *tview.Flex
	todos   *Todos
}

func getColor(s string) tcell.Color {
	switch s {
	case "TODO":
		return tcell.ColorDarkRed
	case "INPROGRESS":
		return tcell.ColorLightCyan
	case "IN-PROGRESS":
		return tcell.ColorLightCyan
	case "BLOCKED":
		return tcell.ColorRed
	case "DONE":
		return tcell.ColorDarkGreen
	default:
		return tcell.ColorWhite
	}
}

func RunTodoTable(todos *Todos) *TodoTable {
	t := TodoTable{}
	t.todos = todos
	t.table = tview.NewTable().SetFixed(1, 1)
	t.table.SetSelectable(true, false)
	t.table.SetSelectedStyle(tcell.StyleDefault.Attributes(tcell.AttrBold | tcell.AttrItalic | tcell.AttrUnderline))
	app := tview.NewApplication()

	t.layout = tview.NewFlex().SetDirection(tview.FlexRow)
	layout := t.layout.AddItem(t.table, 0, 1, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return t.HandleShortcuts(event)
	})
	t.ShowTodos(todos)
	if err := app.SetRoot(layout, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
	return &t
}

func (self *TodoTable) FormatFilename(filename string) string {
	fn := filepath.Base(filename)
	return strings.TrimSuffix(fn, filepath.Ext(fn))
}

func (self *TodoTable) FilenameTableCell(t Todo) *tview.TableCell {
	fn := self.FormatFilename(t.Filename)
	cell := tview.NewTableCell(fn)
	cell.SetTextColor(tcell.ColorDarkGray)
	return cell
}

func (self *TodoTable) StatusTableCell(t Todo) *tview.TableCell {
	cell := tview.NewTableCell(t.Status)
	cell.SetTextColor(getColor(t.Status))
	return cell
}

func (self *TodoTable) HeadlineTableCell(t Todo) *tview.TableCell {
	cell := tview.NewTableCell(t.Headline)
	return cell
}

func (self *TodoTable) ShowTodos(todos *Todos) {
	if todos == nil {
		return
	}
	cell := tview.NewTableCell("Filename            ").SetTextColor(tcell.ColorYellow)
	self.table.SetCell(0, 0, cell)
	cell = tview.NewTableCell("Status    ").SetTextColor(tcell.ColorYellow)
	self.table.SetCell(0, 1, cell)
	cell = tview.NewTableCell("Heading").SetTextColor(tcell.ColorYellow)
	self.table.SetCell(0, 2, cell)
	for r, t := range *todos {
		c := 0
		cell = self.FilenameTableCell(t)
		self.table.SetCell(r+1, c, cell)
		c += 1

		cell = self.StatusTableCell(t)
		self.table.SetCell(r+1, c, cell)
		c += 1

		cell = self.HeadlineTableCell(t)
		self.table.SetCell(r+1, c, cell)
		c += 1
	}
}

var td *regexp.Regexp = regexp.MustCompile("\\sTODO\\s")
var r1s *regexp.Regexp = regexp.MustCompile(`\n[*]\s`)
var r2s *regexp.Regexp = regexp.MustCompile(`\n[*][*]\s`)
var r3s *regexp.Regexp = regexp.MustCompile(`\n[*][*][*]\s`)
var r4s *regexp.Regexp = regexp.MustCompile(`\n[*][*][*][*]\s`)

func (self *TodoTable) Preview(todo *Todo) {
	self.preview = tview.NewTextView()
	self.preview.SetBorder(true)
	self.layout.AddItem(self.preview, 0, 1, true)
	if dat, err := os.ReadFile(todo.Filename); err == nil {
		start := 0
		if todo.LineNum > 5 {
			start = todo.LineNum - 5
		}
		str := string(dat)
		str = td.ReplaceAllString(str, " [red]TODO[-] ")
		str = r1s.ReplaceAllString(str, "\n[blue]*[-] ")
		str = r2s.ReplaceAllString(str, "\n[green]**[-] ")
		str = r3s.ReplaceAllString(str, "\n[yellow]***[-] ")
		str = r4s.ReplaceAllString(str, "\n[grew]****[-] ")
		self.preview.SetText(str)
		self.preview.SetScrollable(true)
		self.preview.ScrollTo(start, 0)
		self.preview.SetTitle(todo.Filename)
		self.preview.SetDynamicColors(true)
		self.preview.SetRegions(true)
		self.preview.SetTitleColor(tcell.ColorLightCyan)
	}
}

func (self *TodoTable) HidePreview() {
	self.layout.RemoveItem(self.preview)
	self.preview = nil
}

func (self *TodoTable) ShowPreview() {
	r, _ := self.table.GetSelection()
	if r >= 1 {
		self.Preview(&(*self.todos)[r-1])
	}
}

func (self *TodoTable) HandleShortcuts(in *tcell.EventKey) *tcell.EventKey {
	// Space should open a preview
	if in.Rune() == ' ' {
		self.ShowPreview()
	} else if self.preview != nil {
		if in.Key() == tcell.KeyEscape {
			self.HidePreview()
		} else if in.Key() == tcell.KeyDown || in.Key() == tcell.KeyUp {
			self.HidePreview()
			self.ShowPreview()
		}
	}
	return in
}

package checklist

// One list on a screen of its own, drawn the way `orgs list show` prints it,
// with a cursor. Every key is one write to the server, and the list is read
// back from the file after each - never patched here - so what is on screen is
// what is in the file.

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/mattn/go-runewidth"
)

type board struct {
	tuikit.UI
	core   *commands.Core
	todo   common.Todo
	list   List
	sel    int
	scroll int
	msg    string
	bad    bool
	quit   bool
	// The editor, and whether it wants this terminal (so the screen is put
	// down while it runs).
	edit     func(string, int)
	termEdit bool
}

const helpLine = "␣ check · v open in editor · a add · o/O add below/above · e edit · d delete · r reload · q back"

func newBoard(core *commands.Core, t common.Todo) *board {
	b := &board{core: core, todo: t}
	b.Th = tuikit.ThemeFor("")
	b.edit, b.termEdit = tuikit.Editor(core)
	b.reload()
	return b
}

// reload reads the list again, keeping the cursor on the same position.
func (b *board) reload() {
	l, err := load(b.core, b.todo)
	if err != nil {
		b.say(true, "could not read the list: %v", err)
	}
	b.list = l
	b.clamp()
}

func (b *board) clamp() {
	if b.sel >= len(b.list.Items) {
		b.sel = len(b.list.Items) - 1
	}
	if b.sel < 0 {
		b.sel = 0
	}
}

func (b *board) say(bad bool, format string, args ...interface{}) {
	b.msg = fmt.Sprintf(format, args...)
	b.bad = bad
}

func (b *board) current() (Item, bool) {
	if b.sel < 0 || b.sel >= len(b.list.Items) {
		return Item{}, false
	}
	return b.list.Items[b.sel], true
}

// wrote reloads after a write, or says why it failed.
func (b *board) wrote(err error, ok string) {
	if err != nil {
		b.say(true, "%v", err)
	} else {
		b.say(false, "%s", ok)
	}
	b.reload()
}

// adding opens the line to type a new item into, and opens it again after
// each one, so a list is written out in one go; esc stops. at is the position
// the item takes (from 1), 0 for the end.
func (b *board) adding(at int) {
	where := "end"
	if at > 0 {
		where = fmt.Sprintf("position %d", at)
	}
	b.Over = &tuikit.Prompt{
		Title: "Add to " + b.list.Title,
		Hint:  "enter adds (at the " + where + ") and asks for the next · esc stops",
		Done: func(text string) {
			err := add(b.core, b.list, text, at)
			b.wrote(err, "added "+text)
			if err != nil {
				return
			}
			if at > 0 {
				b.sel = at - 1
				at++
			} else {
				b.sel = len(b.list.Items) - 1
			}
			b.adding(at)
		},
	}
}

func (b *board) key(ev *tcell.EventKey) {
	b.msg = ""
	n := len(b.list.Items)
	switch ev.Key() {
	case tcell.KeyEscape:
		b.quit = true
		return
	case tcell.KeyUp:
		b.sel--
	case tcell.KeyDown:
		b.sel++
	case tcell.KeyHome:
		b.sel = 0
	case tcell.KeyEnd:
		b.sel = n - 1
	case tcell.KeyPgDn:
		b.sel += 10
	case tcell.KeyPgUp:
		b.sel -= 10
	case tcell.KeyEnter:
		b.toggle()
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q':
			b.quit = true
			return
		case 'j':
			b.sel++
		case 'k':
			b.sel--
		case 'g':
			b.sel = 0
		case 'G':
			b.sel = n - 1
		case ' ', 'x', 'c':
			b.toggle()
		case 'v':
			b.jump()
		case 'a':
			b.adding(0)
		case 'o':
			if n == 0 {
				b.adding(0)
			} else {
				b.adding(b.sel + 2)
			}
		case 'O':
			b.adding(b.sel + 1)
		case 'e', 'i':
			b.rewording()
		case 'd', 'D':
			b.deleting()
		case 'r':
			b.reload()
			b.say(false, "read again")
		}
	}
	b.clamp()
}

func (b *board) toggle() {
	it, ok := b.current()
	if !ok {
		return
	}
	done := !it.Done()
	verb := "checked "
	if !done {
		verb = "unchecked "
	}
	b.wrote(setDone(b.core, b.list, it, done), verb+it.Text)
}

// jump opens the file in the editor on the selected item, or on the heading
// when the list has no items yet. The list is read again afterwards, since
// whatever happened in the editor happened to the file.
func (b *board) jump() {
	if b.edit == nil {
		b.say(true, "no editor - set $EDITOR or editorTemplate")
		return
	}
	line := b.list.Line + 1
	if it, ok := b.current(); ok {
		line = it.Line
	}
	if b.termEdit {
		b.Scr.Suspend()
		b.edit(b.list.File, line)
		b.Scr.Resume()
		b.reload()
		return
	}
	b.edit(b.list.File, line)
	b.say(false, "opened %s:%d", commands.BaseName(b.list.File), line)
}

func (b *board) rewording() {
	it, ok := b.current()
	if !ok {
		return
	}
	b.Over = &tuikit.Prompt{
		Title: fmt.Sprintf("Edit item %d", it.Index+1),
		Text:  it.Text,
		Hint:  "enter saves · ctrl-u clears · esc leaves it",
		Done: func(text string) {
			if strings.TrimSpace(text) == strings.TrimSpace(it.Text) {
				return
			}
			b.wrote(reword(b.core, b.list, it, text), "changed")
		},
	}
}

func (b *board) deleting() {
	it, ok := b.current()
	if !ok {
		return
	}
	b.Over = &tuikit.Confirm{
		Text: fmt.Sprintf("Delete %d. %s?", it.Index+1, it.Text),
		Yes: func() {
			b.wrote(remove(b.core, b.list, it), "deleted "+it.Text)
		},
	}
}

// --- drawing ------------------------------------------------------------------------

var (
	magenta = tcell.GetColor("#c678dd")
)

func (b *board) markStyle(it Item, bg tcell.Color) (string, tcell.Style) {
	switch {
	case it.Doing():
		return "…", b.Style(b.Th.Warn, bg)
	case it.Done():
		return "✔", b.Style(b.Th.Ok, bg)
	}
	return "☐", b.Style(magenta, bg)
}

func (b *board) draw() {
	s := b.Scr
	s.Clear()
	th := b.Th
	w, h := s.Size()
	page := th.Page
	l := b.list

	// The title, as taskbook draws a board: underlined, count beside it.
	y := 1
	x := 2
	x += b.Put(x, y, l.Title, b.Style(th.Text, page).Bold(true).Underline(true), w-x-1)
	x += b.Put(x, y, fmt.Sprintf(" [%d/%d]", l.Done, l.Total), b.Style(th.Dim, page), w-x-1)
	where := fmt.Sprintf("  %s:%d", commands.BaseName(l.File), l.Line+1)
	b.Put(x, y, where, b.Style(th.Faint, page), w-x-1)
	y += 1

	// Room for the items between the title and the footer.
	rows := h - y - 6
	if rows < 1 {
		rows = 1
	}
	if b.sel < b.scroll {
		b.scroll = b.sel
	}
	if b.sel >= b.scroll+rows {
		b.scroll = b.sel - rows + 1
	}
	width := len(fmt.Sprint(len(l.Items)))
	if len(l.Items) == 0 {
		b.Put(4, y, "no items yet - a adds one", b.Style(th.Faint, page).Italic(true), w-6)
	}
	for i := 0; i < rows && b.scroll+i < len(l.Items); i++ {
		k := b.scroll + i
		it := l.Items[k]
		bg := page
		if k == b.sel {
			bg = th.CardSel
			b.Fill(2, y+i, w-4, b.Style(th.Text, bg))
		}
		xx := 4 + 2*it.Depth
		xx += b.Put(xx, y+i, fmt.Sprintf("%*d.", width, it.Index+1), b.Style(th.Faint, bg), w-xx)
		m, ms := b.markStyle(it, bg)
		xx += 1
		xx += b.Put(xx, y+i, m, ms, w-xx)
		xx += 2
		st := b.Style(th.Text, bg)
		if it.Done() {
			st = b.Style(th.Dim, bg)
		}
		if k == b.sel {
			st = st.Bold(true)
		}
		b.Put(xx, y+i, tuikit.Truncate(it.Text, w-xx-3), st, w-xx-3)
	}
	if len(l.Items) > rows {
		more := fmt.Sprintf("%d–%d of %d", b.scroll+1, min(b.scroll+rows, len(l.Items)), len(l.Items))
		b.Put(w-2-runewidth.StringWidth(more), 1, more, b.Style(th.Faint, page), runewidth.StringWidth(more))
	}

	// taskbook's footer, for this list.
	fy := y + min(rows, max(len(l.Items), 1)) + 1
	pct := 0
	if l.Total > 0 {
		pct = l.Done * 100 / l.Total
	}
	x = 2
	x += b.Put(x, fy, fmt.Sprintf("%d%%", pct), b.Style(th.Ok, page), w)
	b.Put(x, fy, " of all tasks complete.", b.Style(th.Dim, page), w)
	x = 2
	for _, part := range []struct {
		n    int
		col  tcell.Color
		what string
	}{{l.Done, th.Ok, " done · "}, {l.Doing, th.Accent, " in-progress · "}, {l.Total - l.Done - l.Doing, magenta, " pending"}} {
		x += b.Put(x, fy+1, fmt.Sprint(part.n), b.Style(part.col, page), w)
		x += b.Put(x, fy+1, part.what, b.Style(th.Dim, page), w)
	}

	// The message from the last key, and what the keys do.
	if b.msg != "" {
		col := th.Ok
		if b.bad {
			col = th.Danger
		}
		b.Put(2, h-2, tuikit.Truncate(b.msg, w-4), b.Style(col, page), w-4)
	}
	b.Put(2, h-1, tuikit.Truncate(helpLine, w-4), b.Style(th.Faint, page), w-4)
	b.DrawOverlay()
}

// run takes the terminal until q.
func (b *board) run() {
	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		commands.Fail("orgs list: %v", err)
	}
	b.Scr = scr
	defer scr.Fini()
	for !b.quit {
		b.draw()
		scr.Show()
		switch ev := scr.PollEvent().(type) {
		case *tcell.EventResize:
			scr.Sync()
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC {
				return
			}
			if b.KeyOverlay(ev) {
				continue
			}
			b.key(ev)
		}
	}
}

// print draws the screen once, for -print and for looking at it in a test.
func (b *board) print(w, h int) string {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		commands.Fail("%v", err)
	}
	sim.SetSize(w, h)
	b.Scr = sim
	b.draw()
	return tuikit.Dump(sim, w, h, commands.Colour())
}

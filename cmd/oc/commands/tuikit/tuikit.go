// Package tuikit is what the full-screen terminal views share: a theme, the
// drawing primitives, and the panels drawn over a view - menus that filter as
// you type, a line to type into, a question, and the hand-off between them.
//
// orgs kanban and orgs cols are built on it, so a menu works the same way in
// both and a fix to one lands in the other.
package tuikit

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/mattn/go-runewidth"
)

// --- the look -----------------------------------------------------------------

type Theme struct {
	Page, Card, CardSel, Chip, Text, Dim, Faint, Accent, Danger, Ok, Warn tcell.Color
	Dark                                                                  bool
}

func Hex(s string) tcell.Color { return tcell.GetColor(s) }

var Dark = Theme{
	Page: tcell.ColorDefault, Card: Hex("#1d232b"), CardSel: Hex("#2a3442"),
	Chip: Hex("#323b47"), Text: Hex("#e6e9ef"), Dim: Hex("#9aa3ae"), Faint: Hex("#5b6470"),
	Accent: Hex("#5b8def"), Danger: Hex("#e5484d"), Ok: Hex("#30a46c"), Warn: Hex("#f5a524"),
	Dark: true,
}

var Light = Theme{
	Page: tcell.ColorDefault, Card: Hex("#f1f3f6"), CardSel: Hex("#dbe5f6"),
	Chip: Hex("#e1e5ea"), Text: Hex("#1f2328"), Dim: Hex("#5f6773"), Faint: Hex("#a2a9b2"),
	Accent: Hex("#3e63dd"), Danger: Hex("#d93036"), Ok: Hex("#218358"), Warn: Hex("#b47a00"),
}

// ThemeFor is the theme asked for, or the terminal's own say through COLORFGBG
// ("15;0" is light text on dark), else dark, which most terminals are.
func ThemeFor(asked string) Theme {
	switch strings.ToLower(asked) {
	case "light":
		return Light
	case "dark":
		return Dark
	}
	if v := os.Getenv("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		if bg := parts[len(parts)-1]; bg == "7" || bg == "15" {
			return Light
		}
	}
	return Dark
}

// Ink is the text colour that reads on top of a filled colour.
func Ink(hex string) tcell.Color {
	r, g, b := Hex(hex).RGB()
	if (0.2126*float64(r)+0.7152*float64(g)+0.0722*float64(b))/255 > 0.62 {
		return Hex("#14161a")
	}
	return Hex("#ffffff")
}

// --- text --------------------------------------------------------------------------

// Truncate cuts to w cells with an ellipsis.
func Truncate(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	if w <= 1 {
		return runewidth.Truncate(s, w, "")
	}
	return runewidth.Truncate(s, w, "…")
}

// Wrap breaks text into lines of at most w cells, at spaces where it can.
func Wrap(text string, w int) []string {
	if w < 4 {
		w = 4
	}
	out := []string{}
	line := ""
	for _, word := range strings.Fields(text) {
		for runewidth.StringWidth(word) > w {
			if line != "" {
				out = append(out, line)
				line = ""
			}
			cut := runewidth.Truncate(word, w, "")
			out = append(out, cut)
			word = word[len(cut):]
		}
		switch {
		case line == "":
			line = word
		case runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

// --- the screen ------------------------------------------------------------------

// UI is a screen, its theme and whatever is drawn over it. A view embeds one.
type UI struct {
	Scr tcell.Screen
	Th  Theme
	// The panel on top, and the one to go back to when it closes - a menu
	// opened from the back of a card returns to the card.
	Over Overlay
	Back Overlay
}

func (u *UI) Style(fg, bg tcell.Color) tcell.Style {
	return tcell.StyleDefault.Foreground(fg).Background(bg)
}

// Put writes s at x,y in at most w cells and answers with the cells used.
func (u *UI) Put(x, y int, s string, st tcell.Style, w int) int {
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			continue
		}
		if used+rw > w {
			break
		}
		u.Scr.SetContent(x+used, y, r, nil, st)
		used += rw
	}
	return used
}

func (u *UI) Fill(x, y, w int, st tcell.Style) {
	for i := 0; i < w; i++ {
		u.Scr.SetContent(x+i, y, ' ', nil, st)
	}
}

// Box draws a panel in the middle of the screen and answers with its inner
// area: x, y, width, height.
func (u *UI) Box(w, h int, title string, accent tcell.Color) (int, int, int, int) {
	sw, sh := u.Scr.Size()
	if w > sw-4 {
		w = sw - 4
	}
	if h > sh-2 {
		h = sh - 2
	}
	x, y := (sw-w)/2, (sh-h)/2
	th := u.Th
	bg := th.Card
	edge := u.Style(th.Faint, bg)
	for yy := y; yy < y+h; yy++ {
		u.Fill(x, yy, w, u.Style(th.Text, bg))
		u.Scr.SetContent(x, yy, '│', nil, edge)
		u.Scr.SetContent(x+w-1, yy, '│', nil, edge)
	}
	for xx := x; xx < x+w; xx++ {
		u.Scr.SetContent(xx, y, '─', nil, edge)
		u.Scr.SetContent(xx, y+h-1, '─', nil, edge)
	}
	u.Scr.SetContent(x, y, '╭', nil, edge)
	u.Scr.SetContent(x+w-1, y, '╮', nil, edge)
	u.Scr.SetContent(x, y+h-1, '╰', nil, edge)
	u.Scr.SetContent(x+w-1, y+h-1, '╯', nil, edge)
	// A strip of the accent along the top edge.
	for xx := x + 1; xx < x+w-1; xx++ {
		u.Scr.SetContent(xx, y, '━', nil, u.Style(accent, bg))
	}
	if title != "" {
		t := " " + Truncate(title, w-6) + " "
		u.Put(x+2, y, t, u.Style(th.Text, bg).Bold(true), w-4)
	}
	return x + 2, y + 1, w - 4, h - 2
}

// --- panels over the view -----------------------------------------------------------

// Overlay.Key answers whether to stay open. One that opens another has
// replaced itself, and is left replaced.
type Overlay interface {
	Key(u *UI, ev *tcell.EventKey) bool
	Draw(u *UI)
}

// Alive is for a panel to come back to that may have gone in the meantime -
// the back of a card that has just been archived.
type Alive interface {
	Alive() bool
}

// KeyOverlay hands a key to the panel on top, if there is one, and answers
// whether there was.
func (u *UI) KeyOverlay(ev *tcell.EventKey) bool {
	cur := u.Over
	if cur == nil {
		return false
	}
	if !cur.Key(u, ev) && u.Over == cur {
		u.Over = nil
	}
	if u.Over == nil && u.Back != nil {
		if al, ok := u.Back.(Alive); !ok || al.Alive() {
			u.Over = u.Back
		}
		u.Back = nil
	}
	return true
}

func (u *UI) DrawOverlay() {
	if u.Over != nil {
		u.Over.Draw(u)
	}
}

// --- a menu ---------------------------------------------------------------------------

type MenuItem struct {
	Label string
	Note  string
	Color string // a dot of this colour, when set
	On    bool   // the current value, or ticked
	Check bool   // a box rather than a mark
}

type Menu struct {
	Title  string
	Items  []MenuItem
	Filter bool
	Query  string
	Sel    int
	scroll int
	// Pick answers whether to stay open (a box ticked, a list kept).
	Pick func(i int) bool
}

func (m *Menu) visible() []int {
	out := []int{}
	if strings.TrimSpace(m.Query) == "" {
		for i := range m.Items {
			out = append(out, i)
		}
		return out
	}
	// Every word somewhere in the label or the note, in the order the items
	// came - the list is usually already in the order worth reading.
	terms := strings.Fields(strings.ToLower(m.Query))
	for i, it := range m.Items {
		hay := strings.ToLower(it.Label + " " + it.Note)
		ok := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				ok = false
			}
		}
		if ok {
			out = append(out, i)
		}
	}
	return out
}

func (m *Menu) Key(u *UI, ev *tcell.EventKey) bool {
	vis := m.visible()
	switch ev.Key() {
	case tcell.KeyEscape:
		return false
	case tcell.KeyUp, tcell.KeyCtrlP:
		m.Sel--
	case tcell.KeyDown, tcell.KeyCtrlN:
		m.Sel++
	case tcell.KeyPgDn:
		m.Sel += 10
	case tcell.KeyPgUp:
		m.Sel -= 10
	case tcell.KeyEnter:
		if m.Sel >= 0 && m.Sel < len(vis) {
			return m.Pick(vis[m.Sel])
		}
		return false
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if r := []rune(m.Query); len(r) > 0 {
			m.Query = string(r[:len(r)-1])
			m.Sel = 0
		}
	case tcell.KeyRune:
		r := ev.Rune()
		switch {
		case r == ' ' && len(vis) > 0 && m.Items[vis[0]].Check && m.Query == "":
			// A box is ticked with space and the menu stays.
			return m.Pick(vis[m.Sel])
		case !m.Filter && r == 'j':
			m.Sel++
		case !m.Filter && r == 'k':
			m.Sel--
		case !m.Filter && r == 'q':
			return false
		case m.Filter:
			m.Query += string(r)
			m.Sel = 0
		default:
			// A menu without a filter picks by the first letter.
			for i, k := range vis {
				if strings.HasPrefix(strings.ToLower(m.Items[k].Label), strings.ToLower(string(r))) {
					m.Sel = i
					return m.Pick(vis[i])
				}
			}
		}
	}
	if m.Sel < 0 {
		m.Sel = 0
	}
	if m.Sel >= len(vis) {
		m.Sel = len(vis) - 1
	}
	return true
}

func (m *Menu) Draw(u *UI) {
	th := u.Th
	vis := m.visible()
	_, sh := u.Scr.Size()
	w := runewidth.StringWidth(m.Title) + 8
	for _, it := range m.Items {
		if n := runewidth.StringWidth(it.Label) + runewidth.StringWidth(it.Note) + 12; n > w {
			w = n
		}
	}
	if w < 36 {
		w = 36
	}
	if w > 90 {
		w = 90
	}
	rows := len(vis)
	if rows > sh-10 {
		rows = sh - 10
	}
	if rows < 1 {
		rows = 1
	}
	// Room for the edges, a line above the items, and the filter line.
	extra := 4
	if m.Filter {
		extra = 5
	}
	x, y, iw, _ := u.Box(w, rows+extra, m.Title, th.Accent)
	bg := th.Card
	if m.Filter {
		u.Put(x, y, "⌕ ", u.Style(th.Accent, bg), 2)
		if m.Query == "" {
			u.Put(x+2, y, "type to filter", u.Style(th.Faint, bg).Italic(true), iw-2)
		} else {
			n := u.Put(x+2, y, m.Query, u.Style(th.Text, bg).Bold(true), iw-2)
			u.Scr.SetContent(x+2+n, y, '▏', nil, u.Style(th.Accent, bg))
		}
		y += 2
	} else {
		y++
	}
	if m.Sel < m.scroll {
		m.scroll = m.Sel
	}
	if m.Sel >= m.scroll+rows {
		m.scroll = m.Sel - rows + 1
	}
	if len(vis) == 0 {
		u.Put(x, y, "nothing matches", u.Style(th.Faint, bg).Italic(true), iw)
	}
	for n := 0; n < rows && m.scroll+n < len(vis); n++ {
		i := m.scroll + n
		it := m.Items[vis[i]]
		rbg := bg
		if i == m.Sel {
			rbg = th.CardSel
			u.Fill(x-1, y+n, iw+2, u.Style(th.Text, rbg))
		}
		xx := x
		switch {
		case it.Check && it.On:
			xx += u.Put(xx, y+n, "☑ ", u.Style(th.Ok, rbg).Bold(true), 2)
		case it.Check:
			xx += u.Put(xx, y+n, "☐ ", u.Style(th.Dim, rbg), 2)
		case it.On:
			xx += u.Put(xx, y+n, "✓ ", u.Style(th.Ok, rbg).Bold(true), 2)
		default:
			xx += 2
		}
		if it.Color != "" {
			xx += u.Put(xx, y+n, "● ", u.Style(Hex(it.Color), rbg), 2)
		}
		st := u.Style(th.Text, rbg)
		if i == m.Sel {
			st = st.Bold(true)
		}
		room := iw - (xx - x)
		note := ""
		if it.Note != "" {
			note = Truncate(it.Note, room/2)
		}
		u.Put(xx, y+n, Truncate(it.Label, room-runewidth.StringWidth(note)-2), st, room)
		if note != "" {
			u.Put(x+iw-runewidth.StringWidth(note), y+n, note, u.Style(th.Faint, rbg), runewidth.StringWidth(note))
		}
	}
}

// --- a question ------------------------------------------------------------------------

type Confirm struct {
	Text string
	Yes  func()
}

func (c *Confirm) Key(u *UI, ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyEnter || ev.Rune() == 'y' || ev.Rune() == 'Y' {
		u.Over = nil
		c.Yes()
		return u.Over != nil
	}
	return false
}

func (c *Confirm) Draw(u *UI) {
	th := u.Th
	lines := Wrap(c.Text, 56)
	x, y, iw, ih := u.Box(62, len(lines)+4, "Sure?", th.Danger)
	for i, l := range lines {
		u.Put(x, y+1+i, l, u.Style(th.Text, th.Card).Bold(true), iw)
	}
	hint := "y yes   n no"
	u.Put(x+iw-runewidth.StringWidth(hint), y+ih-1, hint, u.Style(th.Dim, th.Card), iw)
}

// --- a line to type into -----------------------------------------------------------------

type Prompt struct {
	Title string
	Text  string
	Hint  string
	// Suggest offers completions for the text as typed; Tab takes the first.
	Suggest func(text string) []string
	Done    func(string)
	// AllowEmpty lets an empty line through (clearing a value).
	AllowEmpty bool
}

func (p *Prompt) Key(u *UI, ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape:
		return false
	case tcell.KeyEnter:
		if strings.TrimSpace(p.Text) == "" && !p.AllowEmpty {
			return true
		}
		u.Over = nil
		p.Done(p.Text)
		return u.Over != nil
	case tcell.KeyTab:
		if p.Suggest != nil {
			if s := p.Suggest(p.Text); len(s) > 0 {
				p.Text = s[0]
			}
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if r := []rune(p.Text); len(r) > 0 {
			p.Text = string(r[:len(r)-1])
		}
	case tcell.KeyCtrlU:
		p.Text = ""
	case tcell.KeyCtrlW:
		t := strings.TrimRight(p.Text, " ")
		if i := strings.LastIndex(t, " "); i >= 0 {
			p.Text = t[:i+1]
		} else {
			p.Text = ""
		}
	case tcell.KeyRune:
		p.Text += string(ev.Rune())
	}
	return true
}

func (p *Prompt) Draw(u *UI) {
	th := u.Th
	var sugg []string
	if p.Suggest != nil {
		sugg = p.Suggest(p.Text)
		if len(sugg) > 8 {
			sugg = sugg[:8]
		}
	}
	h := 5
	if len(sugg) > 0 {
		h += len(sugg) + 1
	}
	x, y, iw, _ := u.Box(76, h, p.Title, th.Accent)
	t := p.Text
	for runewidth.StringWidth(t) > iw-2 {
		t = string([]rune(t)[1:])
	}
	n := u.Put(x, y+1, t, u.Style(th.Text, th.Card).Bold(true), iw)
	u.Scr.SetContent(x+n, y+1, '▏', nil, u.Style(th.Accent, th.Card))
	hint := p.Hint
	if hint == "" {
		hint = "enter to go on, esc to stop"
	}
	if len(sugg) > 0 {
		hint += "   ⇥ takes the first"
	}
	u.Put(x, y+2, Truncate(hint, iw), u.Style(th.Faint, th.Card), iw)
	for i, s := range sugg {
		st := u.Style(th.Dim, th.Card)
		if i == 0 {
			st = u.Style(th.Accent, th.Card)
		}
		u.Put(x+2, y+4+i, Truncate(s, iw-2), st, iw-2)
	}
}

// --- outside the screen ------------------------------------------------------------------

// Editor is $VISUAL or $EDITOR when one is set, since that is a program that
// wants the terminal (the bool); the configured editorTemplate otherwise.
func Editor(core *commands.Core) (func(string, int), bool) {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed != "" {
		return func(file string, line int) {
			args := strings.Fields(ed)
			args = append(args, fmt.Sprintf("+%d", line), file)
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			_ = cmd.Run()
		}, true
	}
	if len(core.EditorTemplate) > 0 {
		return core.LaunchEditor, false
	}
	return nil, false
}

// EditText puts the screen down, opens text in $VISUAL/$EDITOR (vi when
// neither is set) and answers with what came back.
func (u *UI) EditText(text, pattern string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	fmt.Fprint(f, text)
	f.Close()
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	args := strings.Fields(ed)
	u.Scr.Suspend()
	cmd := exec.Command(args[0], append(args[1:], f.Name())...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()
	u.Scr.Resume()
	if runErr != nil {
		return "", fmt.Errorf("editor: %v", runErr)
	}
	b, err := os.ReadFile(f.Name())
	return string(b), err
}

// --- to paper --------------------------------------------------------------------------------

func sgr(st tcell.Style) string {
	fg, bg, attr := st.Decompose()
	codes := []string{"0"}
	if attr&tcell.AttrBold != 0 {
		codes = append(codes, "1")
	}
	if attr&tcell.AttrDim != 0 {
		codes = append(codes, "2")
	}
	if attr&tcell.AttrItalic != 0 {
		codes = append(codes, "3")
	}
	if r, g, b := fg.RGB(); r >= 0 && fg != tcell.ColorDefault {
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if r, g, b := bg.RGB(); r >= 0 && bg != tcell.ColorDefault {
		codes = append(codes, fmt.Sprintf("48;2;%d;%d;%d", r, g, b))
	}
	return "\033[" + strings.Join(codes, ";") + "m"
}

// Dump writes a screen that is not a terminal out as text, with the colours
// each cell was given when colour is wanted, trailing blank rows dropped.
func Dump(scr tcell.SimulationScreen, w, h int, colour bool) string {
	lines := []string{}
	for y := 0; y < h; y++ {
		var sb strings.Builder
		prev := ""
		blank := true
		for x := 0; x < w; {
			r, _, st, _ := scr.GetContent(x, y)
			if r == 0 {
				r = ' '
			}
			if r != ' ' {
				blank = false
			}
			if colour {
				if s := sgr(st); s != prev {
					sb.WriteString(s)
					prev = s
				}
			}
			sb.WriteRune(r)
			rw := runewidth.RuneWidth(r)
			if rw < 1 {
				rw = 1
			}
			x += rw
		}
		if colour {
			sb.WriteString("\033[0m")
		}
		if blank {
			lines = append(lines, "")
		} else {
			lines = append(lines, strings.TrimRight(sb.String(), " "))
		}
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n"
}

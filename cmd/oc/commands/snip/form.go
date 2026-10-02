package snip

// The form a command line's parameters are filled in on.
//
// The finished command is drawn at the top and redrawn on every key, with the
// values in colour, so what you are about to run is never a guess - pet shows
// its template, not the result. Below it, a field per parameter with its
// default already in; a parameter with choices cycles through them with the
// arrows and can still be typed over. Esc is a cancel that cancels: pet's ^C
// hands back an empty command and runs it.

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type field struct {
	p      Param
	value  []rune
	cursor int
	choice int
}

// The markers a value is wrapped in for drawing. They pass through Fill's
// quoting untouched (they are not anything the shell cares about) and come out
// where the value landed.
const valOn, valOff = "\x01", "\x02"

// form asks for the parameters' values. ok is false when it was cancelled.
// action is what Enter does, for the hint line: "run", "print", "copy".
func form(title, cmd string, params []Param, preset map[string]string, action string) (map[string]string, bool) {
	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		return nil, false
	}
	defer scr.Fini()
	scr.EnablePaste()

	fields := make([]*field, len(params))
	for i, p := range params {
		v := p.Default
		if pv, ok := preset[p.Name]; ok {
			v = pv
		}
		f := &field{p: p, value: []rune(v)}
		f.cursor = len(f.value)
		for ci, c := range p.Choices {
			if c == v {
				f.choice = ci
			}
		}
		fields[i] = f
	}
	at := 0
	pasting := false

	values := func(marked bool) map[string]string {
		out := map[string]string{}
		for _, f := range fields {
			v := string(f.value)
			if marked && (v != "" || !f.p.Optional) {
				v = valOn + v + valOff
			}
			out[f.p.Name] = v
		}
		return out
	}

	draw := func() {
		scr.Clear()
		w, h := scr.Size()
		base := tcell.StyleDefault
		dim := base.Foreground(tcell.ColorGray)
		accent := base.Foreground(tcell.ColorTeal).Bold(true)
		val := base.Foreground(tcell.ColorYellow).Bold(true)
		put := func(x, y int, s string, st tcell.Style) int {
			for _, r := range s {
				if x >= w {
					break
				}
				scr.SetContent(x, y, r, nil, st)
				x += runewidth.RuneWidth(r)
			}
			return x
		}

		y := 1
		put(2, y, title, accent)
		y += 2
		// The command as it will be, values marked.
		filled := Fill(cmd, params, values(true))
		for _, line := range strings.Split(filled, "\n") {
			if y >= h-4 {
				break
			}
			x := put(2, y, "$ ", dim)
			st := base
			for _, r := range line {
				switch string(r) {
				case valOn:
					st = val
					continue
				case valOff:
					st = base
					continue
				}
				if x >= w-1 {
					y++
					x = 4
				}
				scr.SetContent(x, y, r, nil, st)
				x += runewidth.RuneWidth(r)
			}
			y++
		}
		y++
		label := 0
		for _, f := range fields {
			label = max(label, runewidth.StringWidth(f.p.Name)+2)
		}
		for i, f := range fields {
			if y >= h-2 {
				break
			}
			mark, st := "  ", base
			if i == at {
				mark, st = "▸ ", accent
			}
			name := f.p.Name
			if f.p.Optional {
				name += "?"
			}
			put(2, y, mark, st)
			put(4, y, name, st)
			x := 4 + label + 1
			fx := x
			x = put(x, y, string(f.value), base.Underline(i == at))
			if len(f.value) == 0 {
				hint := "(empty)"
				if f.p.Optional {
					hint = "(optional - left out when empty)"
				}
				put(x, y, hint, dim)
			}
			if len(f.p.Choices) > 1 {
				put(max(x+2, fx+24), y, "↑↓ "+strings.Join(f.p.Choices, " · "), dim)
			}
			if i == at {
				cx := fx + runewidth.StringWidth(string(f.value[:f.cursor]))
				scr.ShowCursor(cx, y)
			}
			y++
		}
		put(2, h-1, "tab next · shift-tab back · ↑↓ choices · ctrl-u clear · enter "+action+" · esc cancel", dim)
		scr.Show()
	}

	set := func(f *field, s string) {
		f.value = []rune(s)
		f.cursor = len(f.value)
	}

	for {
		draw()
		switch ev := scr.PollEvent().(type) {
		case *tcell.EventResize:
			scr.Sync()
		case *tcell.EventPaste:
			pasting = ev.Start()
		case *tcell.EventKey:
			f := fields[at]
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				return nil, false
			case tcell.KeyEnter:
				if pasting {
					// A newline in something pasted is a space in a field
					// that is one line long.
					f.value = append(f.value[:f.cursor], append([]rune{' '}, f.value[f.cursor:]...)...)
					f.cursor++
					continue
				}
				out := values(false)
				for k, v := range out {
					out[k] = strings.TrimSpace(v)
				}
				return out, true
			case tcell.KeyTab:
				at = (at + 1) % len(fields)
			case tcell.KeyBacktab:
				at = (at + len(fields) - 1) % len(fields)
			case tcell.KeyUp, tcell.KeyDown:
				if len(f.p.Choices) > 1 {
					d := 1
					if ev.Key() == tcell.KeyUp {
						d = -1
					}
					f.choice = (f.choice + d + len(f.p.Choices)) % len(f.p.Choices)
					set(f, f.p.Choices[f.choice])
				} else if ev.Key() == tcell.KeyDown {
					at = (at + 1) % len(fields)
				} else {
					at = (at + len(fields) - 1) % len(fields)
				}
			case tcell.KeyLeft:
				f.cursor = max(0, f.cursor-1)
			case tcell.KeyRight:
				f.cursor = min(len(f.value), f.cursor+1)
			case tcell.KeyHome, tcell.KeyCtrlA:
				f.cursor = 0
			case tcell.KeyEnd, tcell.KeyCtrlE:
				f.cursor = len(f.value)
			case tcell.KeyCtrlU:
				set(f, "")
			case tcell.KeyCtrlW:
				i := f.cursor
				for i > 0 && f.value[i-1] == ' ' {
					i--
				}
				for i > 0 && f.value[i-1] != ' ' {
					i--
				}
				f.value = append(f.value[:i], f.value[f.cursor:]...)
				f.cursor = i
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if f.cursor > 0 {
					f.value = append(f.value[:f.cursor-1], f.value[f.cursor:]...)
					f.cursor--
				}
			case tcell.KeyDelete, tcell.KeyCtrlD:
				if f.cursor < len(f.value) {
					f.value = append(f.value[:f.cursor], f.value[f.cursor+1:]...)
				}
			case tcell.KeyRune:
				r := ev.Rune()
				if r == '\r' {
					continue
				}
				f.value = append(f.value[:f.cursor], append([]rune{r}, f.value[f.cursor:]...)...)
				f.cursor++
			}
		}
	}
}

package kanban

// What is drawn over the board: menus, the back of a card, a question, a line
// to type into, and the keys.

import (
	"fmt"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// --- the keys ------------------------------------------------------------------------

type help struct{ a *app }

var helpKeys = [][2]string{
	{"Moving about", ""},
	{"← → ↑ ↓  h l j k", "column, card"},
	{"g G  Home End", "top, bottom of the column"},
	{"⇥  ⇧⇥  b", "next board, previous, pick one"},
	{"/", "search the board (every word, anywhere on a card)"},
	{"A card", ""},
	{"␣  enter", "turn it over: body, properties, checklist"},
	{"H L  ⇧← ⇧→  < >", "move it to the next column"},
	{"m", "move it to any column (or an alias)"},
	{"J K  ⇧↑ ⇧↓", "move it down, up (when sorted by hand)"},
	{"t  p  a", "keyword, priority, labels"},
	{"c", "clock in, or out"},
	{"e", "open it in the editor"},
	{"R  y  A", "refile, copy, archive"},
	{"The board", ""},
	{"z  Z", "fold a column; fold or open all"},
	{"s  v", "order by; board or list"},
	{"x  X", "put a list section away; bring one back"},
	{"C", "keep the columns on show as the board's own"},
	{"S", "settings, in your editor"},
	{"N  D", "new board, delete this one"},
	{"r  q", "read again, quit"},
}

func (h *help) Key(_ *tuikit.UI, ev *tcell.EventKey) bool { return false }

func (h *help) Draw(_ *tuikit.UI) {
	a := h.a
	th := a.Th
	x, y, iw, _ := a.Box(72, len(helpKeys)+4, "Keys", th.Accent)
	y++
	for i, k := range helpKeys {
		if k[1] == "" {
			a.Put(x, y+i, k[0], a.Style(th.Accent, th.Card).Bold(true), iw)
			continue
		}
		a.Put(x+2, y+i, k[0], a.Style(th.Text, th.Card).Bold(true), 20)
		a.Put(x+24, y+i, k[1], a.Style(th.Dim, th.Card), iw-24)
	}
}

// --- the back of a card -------------------------------------------------------------

type detail struct {
	a      *app
	card   Card
	sel    int // the checklist item picked out, or -1
	scroll int
}

var linkRe = regexp.MustCompile(`\[\[([^\]]+)\](?:\[([^\]]*)\])?\]`)

func (d *detail) current(a *app) Card {
	for _, c := range a.cards {
		if c.Filename == d.card.Filename && c.Headline == d.card.Headline {
			d.card = c
			return c
		}
	}
	return d.card
}

func (d *detail) items(a *app) []checkItem {
	bd, _ := a.bodyOf(d.current(a).Hash)
	return checklistOf(bd.Text)
}

func (d *detail) Key(_ *tuikit.UI, ev *tcell.EventKey) bool {
	a := d.a
	items := d.items(a)
	c := d.current(a)
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyEnter:
		return false
	case tcell.KeyDown:
		d.step(1, len(items))
		return true
	case tcell.KeyUp:
		d.step(-1, len(items))
		return true
	case tcell.KeyPgDn:
		d.scroll += 10
		return true
	case tcell.KeyPgUp:
		d.scroll -= 10
		if d.scroll < 0 {
			d.scroll = 0
		}
		return true
	}
	switch ev.Rune() {
	case 'q':
		return false
	case 'j':
		d.step(1, len(items))
	case 'k':
		d.step(-1, len(items))
	case ' ', 'x':
		if d.sel >= 0 && d.sel < len(items) {
			it := items[d.sel]
			a.tick(&c, it.index, it.text, !it.done)
		}
	case 'c':
		a.toggleClock()
	case 'e':
		a.openEditor()
		a.rereadBody(d.current(a).Hash)
	case 't', 'p', 'a', 'A', 'R', 'y':
		// A menu opened from the back of a card comes back to it.
		open := map[rune]func(){'t': a.keywordMenu, 'p': a.priorityMenu, 'a': a.labelMenu,
			'A': a.archive, 'R': func() { a.moveTo("refile") }, 'y': func() { a.moveTo("copy") }}
		open[ev.Rune()]()
		if a.Over != tuikit.Overlay(d) {
			a.Back = d
			return true
		}
	}
	return true
}

// still says whether the card is on the board, for coming back to it.
func (d *detail) Alive() bool {
	a := d.a
	for _, c := range a.cards {
		if c.Filename == d.card.Filename && c.Headline == d.card.Headline {
			return true
		}
	}
	return false
}

func (d *detail) step(by, n int) {
	if n == 0 {
		d.scroll += by
		if d.scroll < 0 {
			d.scroll = 0
		}
		return
	}
	d.sel += by
	if d.sel < 0 {
		d.sel = 0
	}
	if d.sel >= n {
		d.sel = n - 1
	}
}

func (d *detail) Draw(_ *tuikit.UI) {
	a := d.a
	th := a.Th
	b := a.board()
	c := d.current(a)
	sw, sh := a.Scr.Size()
	w := sw * 3 / 4
	if w < 60 {
		w = sw - 4
	}
	if w > 100 {
		w = 100
	}
	hc := tuikit.Hex(HeaderColorOf(&c, b, th.Dark))
	x, y, iw, ih := a.Box(w, sh-4, "", hc)
	bg := th.Card
	lines := []row{}
	add := func(r row) { lines = append(lines, r) }
	base := a.Style(th.Text, bg)
	dim := a.Style(th.Dim, bg)
	label := func(k string) seg { return seg{fmt.Sprintf("%-10s", k), a.Style(th.Faint, bg)} }

	// The title, large as a terminal gets: bold, wrapped, after its priority.
	head := row{}
	if c.Priority != "" {
		p := strings.ToUpper(c.Priority)
		pc := PriorityColor[p]
		if pc == "" {
			pc = "#6b7280"
		}
		head = append(head, seg{" " + p + " ", a.Style(tuikit.Ink(pc), tuikit.Hex(pc)).Bold(true)}, seg{" ", base})
	}
	for i, l := range tuikit.Wrap(c.Headline, iw-4) {
		if i == 0 {
			add(append(head, seg{l, base.Bold(true)}))
		} else {
			add(row{{"    " + l, base.Bold(true)}})
		}
	}
	if ls := LabelsOf(&c, b); len(ls) > 0 {
		r := row{}
		for _, v := range ls {
			col := LabelColorOf(v, b)
			r = append(r, seg{" " + v + " ", a.Style(tuikit.Ink(col), tuikit.Hex(col))}, seg{" ", base})
		}
		add(r)
	}
	add(row{})
	now := time.Now()
	if c.Status != "" {
		add(row{label("keyword"), {c.Status, base.Bold(true)}})
	}
	if IsSet(c.Date) {
		add(row{label("scheduled"), {c.Date.Start.Format("Mon Jan 2 2006"), base}, {"  " + relative(c.Date.Start, now), dim}})
	}
	if IsSet(c.Deadline) {
		st := base
		if overdue(c.Deadline.Start, now) {
			st = a.Style(th.Danger, bg).Bold(true)
		}
		add(row{label("deadline"), {c.Deadline.Start.Format("Mon Jan 2 2006"), st}, {"  " + relative(c.Deadline.Start, now), dim}})
	}
	if len(c.Tags) > 0 {
		r := row{label("tags")}
		for _, t := range c.Tags {
			r = append(r, seg{"#" + t + " ", a.Style(tuikit.Hex(AutoSwatch(t).Hex), bg)})
		}
		add(r)
	}
	if a.clockKey == c.Filename+"\x00"+c.Headline {
		add(row{label("clock"), {"◉ running", a.Style(th.Ok, bg).Bold(true)}})
	}
	add(row{label("file"), {fmt.Sprintf("%s:%d", baseName(c.Filename), c.LineNum), base},
		{"  " + tuikit.Truncate(dirOf(c.Filename), iw-30), a.Style(th.Faint, bg)}})

	if (b.BackShows == "properties" || b.BackShows == "both") && len(c.Props) > 0 {
		add(row{})
		add(row{{"Properties", a.Style(th.Accent, bg).Bold(true)}})
		for _, k := range sortedKeys(c.Props) {
			add(row{{fmt.Sprintf("  %-14s", tuikit.Truncate(k, 14)), a.Style(th.Faint, bg)}, {c.Props[k], base}})
		}
	}

	selLine := -1
	if b.BackShows == "body" || b.BackShows == "both" {
		bd, ok := a.bodyOf(c.Hash)
		text := bodyLines(bd.Text)
		if !ok {
			add(row{})
			add(row{{"reading…", a.Style(th.Faint, bg).Italic(true)}})
		} else if strings.TrimSpace(strings.Join(text, "")) != "" {
			add(row{})
			items := checklistOf(bd.Text)
			done := 0
			for _, it := range items {
				if it.done {
					done++
				}
			}
			h := row{{"Notes", a.Style(th.Accent, bg).Bold(true)}}
			if len(items) > 0 {
				h = append(h, seg{fmt.Sprintf("   ☑ %d/%d", done, len(items)), dim})
			}
			add(h)
			ci := 0
			for _, l := range text {
				m := checkRe.FindStringSubmatch(l)
				if m != nil && !(m[2] == "*" && m[1] == "") {
					it := items[ci]
					box, st := "☐ ", base
					if it.done {
						box, st = "☑ ", a.Style(th.Dim, bg)
					}
					ind := strings.Repeat("  ", it.depth)
					r := row{{"  " + ind, base}}
					if ci == d.sel {
						r = row{{"▸ " + ind, a.Style(th.Accent, bg).Bold(true)}}
						selLine = len(lines)
						st = st.Bold(true)
					}
					bst := a.Style(th.Dim, bg)
					if it.done {
						bst = a.Style(th.Ok, bg)
					}
					r = append(r, seg{box, bst}, seg{linkRe.ReplaceAllString(it.text, "$2"), st})
					add(r)
					ci++
					continue
				}
				l = linkRe.ReplaceAllStringFunc(l, func(s string) string {
					mm := linkRe.FindStringSubmatch(s)
					if mm[2] != "" {
						return mm[2]
					}
					return mm[1]
				})
				st := base
				if strings.HasPrefix(strings.TrimSpace(l), "#+") {
					st = a.Style(th.Faint, bg)
				}
				if strings.TrimSpace(l) == "" {
					add(row{})
					continue
				}
				for _, wl := range tuikit.Wrap(l, iw-4) {
					add(row{{"  " + wl, st}})
				}
			}
		}
	}

	room := ih - 2
	if selLine >= 0 {
		if selLine < d.scroll {
			d.scroll = selLine
		}
		if selLine >= d.scroll+room {
			d.scroll = selLine - room + 1
		}
	}
	if d.scroll > len(lines)-1 {
		d.scroll = len(lines) - 1
	}
	if d.scroll < 0 {
		d.scroll = 0
	}
	for n := 0; n < room && d.scroll+n < len(lines); n++ {
		a.paint(x, y+n, iw, lines[d.scroll+n])
	}
	if d.scroll+room < len(lines) {
		a.Put(x+iw-8, y+room-1, "↓ more", a.Style(th.Faint, bg), 8)
	}

	keys := "j k item  ␣ tick  t keyword  p priority  a labels  c clock  e edit  R refile  A archive  esc back"
	a.Put(x, y+ih-1, tuikit.Truncate(keys, iw), a.Style(th.Dim, bg), iw)
}

func dirOf(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i > 0 {
		return p[:i]
	}
	return ""
}

// relative says how far off a date is, in the unit that reads best.
func relative(t, now time.Time) string {
	day := func(x time.Time) time.Time { y, m, d := x.Date(); return time.Date(y, m, d, 0, 0, 0, 0, time.Local) }
	n := int(math.Round(day(t).Sub(day(now)).Hours() / 24))
	ago := n < 0
	if ago {
		n = -n
	}
	var s string
	switch {
	case n == 0:
		return "today"
	case n == 1 && ago:
		return "yesterday"
	case n == 1:
		return "tomorrow"
	case n < 14:
		s = fmt.Sprintf("%d days", n)
	case n < 60:
		s = fmt.Sprintf("%d weeks", n/7)
	default:
		s = fmt.Sprintf("%d months", n/30)
	}
	if ago {
		return s + " ago"
	}
	return "in " + s
}

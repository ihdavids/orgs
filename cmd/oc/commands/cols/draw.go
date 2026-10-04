package cols

// Drawing the column view: the outline as a table.
//
// The titles are coloured by level the way org colours its headings, a parent
// carries a fold mark, and a cell the server rolled up (EFFORT{:}, CLOCKSUM)
// is drawn as a total rather than as something typed - with, beside a parent's
// own value in a number column the line does not sum, the small total of what
// is under it. The first column stays put while the rest scroll sideways.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands/kanban"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"github.com/mattn/go-runewidth"
)

// The colours org gives heading levels, in a palette that reads on dark and
// light alike.
var levelColors = []string{"#5b8def", "#f5a524", "#30a46c", "#8e4ec6", "#12a594", "#d6409f", "#e5484d", "#8db421"}

func priorityColor(p string) string {
	if c := kanban.PriorityColor[strings.ToUpper(p)]; c != "" {
		return c
	}
	if p == "" {
		return ""
	}
	return "#6b7280"
}

const (
	top    = 4 // the row the column titles are on
	colGap = 2
)

// itemText is what the ITEM column says for a row: indentation, the fold mark,
// the path when +path asks for it, and the title.
func (a *app) itemParts(r Row) (indent, mark, path, title string) {
	o := a.opts()
	if !o.Flat {
		indent = strings.Repeat("  ", max(0, r.Level-1))
		switch {
		case r.HasChildren && a.folded[r.Hash]:
			mark = "▸ "
		case r.HasChildren:
			mark = "▾ "
		default:
			mark = "· "
		}
	}
	if o.Path > 0 {
		anc := a.ancestors[r.Hash]
		if len(anc) > o.Path {
			anc = anc[len(anc)-o.Path:]
		}
		if len(anc) > 0 {
			path = strings.Join(anc, " › ") + " › "
		}
	}
	return indent, mark, path, r.Headline
}

func (a *app) cellText(r Row, i int) string {
	s := a.data.Spec[i]
	switch s.Kind {
	case "item":
		in, m, p, t := a.itemParts(r)
		return in + m + p + t
	case "todo":
		return r.Status
	case "priority":
		if r.Priority == "" {
			return ""
		}
		return " " + strings.ToUpper(r.Priority) + " "
	case "tags":
		if len(r.Tags) == 0 {
			return ""
		}
		return "#" + strings.Join(r.Tags, " #")
	}
	if i >= len(r.Cells) {
		return ""
	}
	v := r.Cells[i].Value
	if t, ok := a.totals[i][r.Hash]; ok && !a.opts().Flat {
		v += "  Σ" + a.number(s, t)
	}
	return v
}

func (a *app) number(s Spec, n float64) string {
	if s.Numbers == "duration" {
		return HoursMinutes(n)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".")
}

// widths are what each column takes: what its contents need, held to the
// width the line gives it (a maximum, as worg reads it, raised when it is too
// narrow for what the column shows), then the item squeezed to fit.
func (a *app) widths(screen int) []int {
	out := make([]int, len(a.data.Spec))
	for i, s := range a.data.Spec {
		w := runewidth.StringWidth(title(s)) + 2
		for _, r := range a.rows {
			if n := runewidth.StringWidth(a.cellText(r, i)); n > w {
				w = n
			}
		}
		limit := 24
		if s.Kind == "item" {
			limit = 64
		}
		if s.Width > 0 {
			limit = s.Width
			if min := runewidth.StringWidth(title(s)); limit < min {
				limit = min
			}
			if s.Kind == "priority" && limit < 3 {
				limit = 3
			}
		}
		if w > limit {
			w = limit
		}
		out[i] = w
	}
	// The item gives way first, down to something still readable.
	total := 0
	for _, w := range out {
		total += w + colGap
	}
	if ic := a.itemCol(); ic >= 0 && total > screen-2 {
		over := total - (screen - 2)
		if out[ic]-over >= 28 {
			out[ic] -= over
		} else if out[ic] > 28 {
			out[ic] = 28
		}
	}
	return out
}

func (a *app) shownWidth(i int) int {
	w, _ := a.Scr.Size()
	ws := a.widths(w)
	if i < 0 || i >= len(ws) {
		return 10
	}
	return ws[i]
}

func title(s Spec) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Property
}

// --- the frame ---------------------------------------------------------------------

func (a *app) draw() {
	a.Scr.Clear()
	w, h := a.Scr.Size()
	th := a.Th
	page := th.Page

	// The file, where the line came from, and what the view is doing.
	x := 1
	x += a.Put(x, 0, "▦ ", a.Style(th.Accent, page).Bold(true), w)
	x += a.Put(x, 0, " "+filepath.Base(a.file)+" ", a.Style(tuikit.Hex("#ffffff"), th.Accent).Bold(true), w-x)
	n := len(a.data.Rows)
	from := map[string]string{"file": "the file's #+COLUMNS:", "request": "this view - W writes it into the file",
		"config": "columns.default in the yaml", "default": "the built-in line"}[a.data.From]
	if from == "" {
		from = a.data.From
	}
	x += a.Put(x+2, 0, fmt.Sprintf("%d heading%s  ·  columns from %s", n, plural(n), from), a.Style(th.Dim, page), w-x-2) + 2
	if !a.printing {
		a.Put(w-7, 0, "? keys", a.Style(th.Faint, page), 7)
	}
	lst := a.Style(th.Faint, page)
	if a.data.From == "request" {
		lst = a.Style(th.Warn, page)
	}
	a.Put(3, 1, tuikit.Truncate(a.data.Columns, w-4), lst, w-4)

	chips := []string{}
	if a.filterBy != "" {
		chips = append(chips, fmt.Sprintf("only %s = %s (F shows all)", a.filterBy, a.filterVal))
	}
	if sc := a.sortCol(); sc >= 0 {
		dir := "rising"
		if a.sortDesc {
			dir = "falling"
		}
		chips = append(chips, fmt.Sprintf("sorted by %s, %s, within each parent", title(a.data.Spec[sc]), dir))
	}
	if len(chips) > 0 {
		a.Put(3, 2, tuikit.Truncate(strings.Join(chips, "  ·  "), w-4), a.Style(th.Accent, page), w-4)
	}

	if !a.data.Ok {
		msg := a.data.Msg
		if msg == "" {
			msg = "no answer from the server"
		}
		a.Put(3, top+1, tuikit.Truncate(msg, w-6), a.Style(th.Danger, page).Bold(true), w-6)
	} else {
		a.table(w, h)
	}
	a.footer(w, h)
	a.DrawOverlay()
	a.Scr.Show()
}

// columnsOnShow is the order the columns are drawn in: the first one fixed,
// the rest from the sideways scroll on, as many as fit.
func (a *app) columnsOnShow(ws []int, w int) []int {
	if len(ws) == 0 {
		return nil
	}
	if a.ci > 0 && a.ci < a.hscroll {
		a.hscroll = a.ci
	}
	if a.hscroll < 1 {
		a.hscroll = 1
	}
	fits := func(start int) []int {
		out := []int{0}
		used := 1 + ws[0] + colGap
		for i := start; i < len(ws); i++ {
			if used+ws[i] > w {
				break
			}
			out = append(out, i)
			used += ws[i] + colGap
		}
		return out
	}
	for {
		cols := fits(a.hscroll)
		if a.ci == 0 || contains(cols, a.ci) || a.hscroll >= a.ci {
			return cols
		}
		a.hscroll++
	}
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func (a *app) table(w, h int) {
	th := a.Th
	page := th.Page
	ws := a.widths(w)
	cols := a.columnsOnShow(ws, w)
	if len(cols) == 0 {
		a.Put(3, top, "no columns", a.Style(th.Faint, page).Italic(true), w)
		return
	}

	// The titles, the selected one in the accent and the sorted one marked.
	x := 1
	for _, i := range cols {
		s := a.data.Spec[i]
		t := title(s)
		if s.Summary != "" {
			t += " Σ"
		}
		if a.sortCol() == i {
			if a.sortDesc {
				t += " ▼"
			} else {
				t += " ▲"
			}
		}
		st := a.Style(th.Text, page).Bold(true)
		rule, rst := "─", a.Style(th.Faint, page)
		if i == a.ci && !a.printing {
			st = a.Style(th.Accent, page).Bold(true)
			rule, rst = "━", a.Style(th.Accent, page)
		}
		if s.Numbers != "" {
			a.Put(x+max(0, ws[i]-runewidth.StringWidth(t)), top, t, st, ws[i])
		} else {
			a.Put(x, top, tuikit.Truncate(t, ws[i]), st, ws[i])
		}
		a.Put(x, top+1, strings.Repeat(rule, ws[i]), rst, ws[i])
		x += ws[i] + colGap
		if i == 0 && len(cols) > 1 && cols[1] > 1 {
			a.Put(x-2, top, "◂", a.Style(th.Accent, page), 1)
		}
	}
	if last := cols[len(cols)-1]; last < len(ws)-1 {
		a.Put(w-2, top, "▸", a.Style(th.Accent, page).Bold(true), 1)
	}

	y0 := top + 2
	room := h - 1 - y0
	if a.ri < a.scroll {
		a.scroll = a.ri
	}
	if a.ri >= a.scroll+room {
		a.scroll = a.ri - room + 1
	}
	if a.printing {
		a.scroll = 0
	}
	if len(a.rows) == 0 {
		a.Put(3, y0, "no headings", a.Style(th.Faint, page).Italic(true), w)
	}
	for n := 0; n < room && a.scroll+n < len(a.rows); n++ {
		k := a.scroll + n
		r := a.rows[k]
		y := y0 + n
		sel := k == a.ri && !a.printing
		bg := page
		// A top-level heading carries a band, so the outline reads in blocks.
		if r.Level == 1 && !a.opts().Flat {
			bg = th.Card
		}
		if sel {
			bg = th.CardSel
		}
		if bg != page {
			a.Fill(0, y, w, a.Style(th.Text, bg))
		}
		x := 1
		for _, i := range cols {
			cbg := bg
			if sel && i == a.ci {
				cbg = th.Accent
				a.Fill(x-1, y, ws[i]+2, a.Style(th.Text, cbg))
			}
			a.cell(x, y, ws[i], r, i, cbg, sel && i == a.ci)
			x += ws[i] + colGap
		}
	}
	if a.scroll > 0 {
		a.Put(w-10, y0-1, fmt.Sprintf("↑ %d more", a.scroll), a.Style(th.Faint, page), 10)
	}
	if rest := len(a.rows) - (a.scroll + room); rest > 0 && !a.printing {
		a.Put(w-10, h-2, fmt.Sprintf("↓ %d more", rest), a.Style(th.Faint, page), 10)
	}
}

func (a *app) cell(x, y, w int, r Row, i int, bg tcell.Color, on bool) {
	th := a.Th
	s := a.data.Spec[i]
	fg := func(c tcell.Color) tcell.Style {
		if on {
			return a.Style(tuikit.Hex("#ffffff"), bg).Bold(true)
		}
		return a.Style(c, bg)
	}
	switch s.Kind {
	case "item":
		in, mark, path, t := a.itemParts(r)
		lc := tuikit.Hex(levelColors[(max(1, r.Level)-1)%len(levelColors)])
		at := a.Put(x, y, in, fg(th.Faint), w)
		at += a.Put(x+at, y, mark, fg(lc), w-at)
		if path != "" {
			// The path gives way before the title does, but keeps its
			// separator so the two never run together.
			p := tuikit.Truncate(strings.TrimSuffix(path, " › "), max(1, (w-at)/2-3))
			at += a.Put(x+at, y, p+" › ", fg(th.Faint), w-at)
		}
		st := fg(lc)
		if r.Level == 1 {
			st = st.Bold(true)
		}
		a.Put(x+at, y, tuikit.Truncate(t, w-at), st, w-at)
	case "todo":
		c := th.Warn
		if containsStr(a.states.Done, r.Status) {
			c = th.Ok
		}
		a.Put(x, y, tuikit.Truncate(r.Status, w), fg(c).Bold(true), w)
	case "priority":
		if r.Priority != "" {
			pc := priorityColor(r.Priority)
			a.Put(x, y, " "+strings.ToUpper(r.Priority)+" ", a.Style(tuikit.Ink(pc), tuikit.Hex(pc)).Bold(true), w)
		}
	case "tags":
		at := 0
		for _, t := range r.Tags {
			txt := "#" + t
			if at+runewidth.StringWidth(txt) > w {
				if at < w {
					a.Put(x+at, y, "…", fg(th.Faint), 1)
				}
				break
			}
			at += a.Put(x+at, y, txt, fg(tuikit.Hex(kanban.AutoSwatch(t).Hex)), w-at) + 1
		}
	default:
		if i >= len(r.Cells) {
			return
		}
		c := r.Cells[i]
		v := c.Value
		st := fg(th.Text)
		switch {
		case s.Kind == "derived":
			st = fg(th.Dim)
		case c.Summed:
			// A total the server worked out, not something typed.
			st = fg(tuikit.Hex("#12a594")).Bold(true)
		}
		extra := ""
		if t, ok := a.totals[i][r.Hash]; ok && !a.opts().Flat {
			extra = "  Σ" + a.number(s, t)
		}
		full := v + extra
		if s.Numbers != "" {
			pad := max(0, w-runewidth.StringWidth(full))
			at := a.Put(x+pad, y, v, st, w-pad)
			a.Put(x+pad+at, y, extra, fg(th.Faint), w-pad-at)
		} else {
			at := a.Put(x, y, tuikit.Truncate(v, w), st, w)
			a.Put(x+at, y, extra, fg(th.Faint), w-at)
		}
	}
}

func (a *app) footer(w, h int) {
	th := a.Th
	y := h - 1
	if a.printing {
		return
	}
	if a.msg != "" {
		st := a.Style(th.Ok, th.Page)
		mark := "✓ "
		if a.msgBad {
			st, mark = a.Style(th.Danger, th.Page), "✗ "
		}
		a.Put(1, y, tuikit.Truncate(mark+a.msg, w-2), st, w-2)
		return
	}
	keys := [][2]string{{"↑↓←→", "move"}, {"⏎", "edit"}, {"⇧←→", "step value"}, {"⇥", "fold"},
		{"1-9", "levels"}, {"s", "sort"}, {"f", "narrow"}, {"n", "new"}, {"c", "columns"}, {"< >", "move col"},
		{"W", "save"}, {"?", "all keys"}}
	x := 1
	for _, k := range keys {
		need := runewidth.StringWidth(k[0]) + runewidth.StringWidth(k[1]) + 3
		if x+need > w {
			break
		}
		x += a.Put(x, y, k[0], a.Style(th.Accent, th.Page).Bold(true), w-x)
		x += a.Put(x+1, y, k[1], a.Style(th.Dim, th.Page), w-x) + 3
	}
}

// --- the keys --------------------------------------------------------------------

type help struct{}

var helpKeys = [][2]string{
	{"Moving about", ""},
	{"↑ ↓ ← →  h j k l", "row, column"},
	{"g G  PgUp PgDn", "top, bottom, a page"},
	{"⇥  z  ␣", "fold or open a heading"},
	{"Z  1-9", "fold or open everything; show down to a level"},
	{"O", "another file"},
	{"A cell", ""},
	{"⏎  e", "edit it - its own value, never the total"},
	{"⇧← ⇧→  H L", "step through its allowed values, keywords, priorities"},
	{"d  ⌫", "empty it"},
	{"n", "a new heading after this one (NEXT Title 3d @who #tag ^fri !A K=v)"},
	{"v  E", "read the heading; open it in the editor"},
	{"The view", ""},
	{"s", "sort by this column, within each parent: rising, falling, off"},
	{"f  F  V", "only rows with this value; all rows; pick a value"},
	{"c", "write the columns line (⇥ completes)"},
	{"< >  + -", "move this column; widen or narrow it"},
	{"p  o", "path before the titles; flat"},
	{"W  R", "write the line into the file; back to the file's own"},
	{"r  q", "read again, quit"},
}

func (h *help) Key(_ *tuikit.UI, ev *tcell.EventKey) bool { return false }

func (h *help) Draw(u *tuikit.UI) {
	th := u.Th
	x, y, iw, _ := u.Box(84, len(helpKeys)+4, "Keys", th.Accent)
	y++
	for i, k := range helpKeys {
		if k[1] == "" {
			u.Put(x, y+i, k[0], u.Style(th.Accent, th.Card).Bold(true), iw)
			continue
		}
		u.Put(x+2, y+i, k[0], u.Style(th.Text, th.Card).Bold(true), 20)
		u.Put(x+24, y+i, tuikit.Truncate(k[1], iw-24), u.Style(th.Dim, th.Card), iw-24)
	}
}

// --- reading a heading -------------------------------------------------------------

type preview struct {
	row    Row
	text   string
	path   []string
	scroll int
}

func (p *preview) Key(_ *tuikit.UI, ev *tcell.EventKey) bool {
	switch {
	case ev.Key() == tcell.KeyDown || ev.Rune() == 'j':
		p.scroll++
		return true
	case ev.Key() == tcell.KeyUp || ev.Rune() == 'k':
		if p.scroll > 0 {
			p.scroll--
		}
		return true
	}
	return false
}

func (p *preview) Draw(u *tuikit.UI) {
	th := u.Th
	sw, sh := u.Scr.Size()
	w := min(100, sw-4)
	lc := tuikit.Hex(levelColors[(max(1, p.row.Level)-1)%len(levelColors)])
	x, y, iw, ih := u.Box(w, sh-4, "", lc)
	bg := th.Card
	if len(p.path) > 0 {
		u.Put(x, y, tuikit.Truncate(strings.Join(p.path, " › "), iw), u.Style(th.Faint, bg), iw)
	}
	head := strings.Repeat("*", max(1, p.row.Level)) + " "
	if p.row.Status != "" {
		head += p.row.Status + " "
	}
	u.Put(x, y+1, tuikit.Truncate(head+p.row.Headline, iw), u.Style(lc, bg).Bold(true), iw)
	lines := []string{}
	for _, l := range strings.Split(strings.TrimRight(p.text, "\n"), "\n") {
		// A line that fits is shown as written - a drawer's alignment and a
		// list's indentation are part of what it says; only a long one wraps.
		if runewidth.StringWidth(l) <= iw {
			lines = append(lines, l)
			continue
		}
		lines = append(lines, tuikit.Wrap(l, iw)...)
	}
	room := ih - 4
	if p.scroll > max(0, len(lines)-room) {
		p.scroll = max(0, len(lines)-room)
	}
	inDrawer := false
	for n := 0; n < room && p.scroll+n < len(lines); n++ {
		l := lines[p.scroll+n]
		t := strings.TrimSpace(l)
		st := u.Style(th.Text, bg)
		switch {
		case strings.HasPrefix(t, ":") && strings.HasSuffix(t, ":") && !strings.EqualFold(t, ":END:"):
			inDrawer = true
			st = u.Style(th.Faint, bg)
		case strings.EqualFold(t, ":END:"):
			inDrawer = false
			st = u.Style(th.Faint, bg)
		case inDrawer, strings.HasPrefix(t, "SCHEDULED:"), strings.HasPrefix(t, "DEADLINE:"), strings.HasPrefix(t, "CLOSED:"), strings.HasPrefix(t, "#+"):
			st = u.Style(th.Dim, bg)
		case strings.HasPrefix(t, "- [X]") || strings.HasPrefix(t, "- [x]"):
			st = u.Style(th.Ok, bg)
		}
		u.Put(x, y+3+n, l, st, iw)
	}
	if strings.TrimSpace(p.text) == "" {
		u.Put(x, y+3, "nothing written under it", u.Style(th.Faint, bg).Italic(true), iw)
	}
	u.Put(x, y+ih-1, "j k scroll   esc back", u.Style(th.Faint, bg), iw)
}

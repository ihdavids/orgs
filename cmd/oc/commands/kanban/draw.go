package kanban

// Drawing the board.
//
// A card is laid out as rows of styled runs first and painted second, so its
// height is known before anything is drawn - which is what lets a column
// scroll to keep the selected card whole. The card's face is worg's: the thin
// bar across the top from its COLOUR (or the board's header property), the
// colour-by stripe down the side, label chips, the priority, the title, then
// keyword, badges, tags and the checklist count, and the dates and file along
// the bottom.

import (
	"fmt"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// A run of text in one style, and a line of them. Kept here rather than
// taken from tuikit because the board writes hundreds of them positionally.
type seg struct {
	text string
	st   tcell.Style
}

type row []seg

func (a *app) paint(x, y, w int, r row) {
	at := 0
	for _, s := range r {
		if at >= w {
			return
		}
		at += a.Put(x+at, y, s.text, s.st, w-at)
	}
}

// --- a card ------------------------------------------------------------------------

func overdue(t time.Time, now time.Time) bool {
	y, m, d := now.Date()
	return t.Before(time.Date(y, m, d, 0, 0, 0, 0, t.Location()))
}

var checkRe = regexp.MustCompile(`^(\s*)([-+*]|\d+[.)])\s+\[([ xX-])\]\s?(.*)$`)

type checkItem struct {
	index int
	done  bool
	text  string
	depth int
}

// checklistOf is worg's checklistOf: the boxes outside drawers and planning
// lines, numbered the way the server counts them to find the line again.
func checklistOf(text string) []checkItem {
	out := []checkItem{}
	for _, line := range bodyLines(text) {
		m := checkRe.FindStringSubmatch(line)
		if m == nil || (m[2] == "*" && m[1] == "") {
			continue
		}
		out = append(out, checkItem{
			index: len(out),
			done:  m[3] == "x" || m[3] == "X",
			text:  strings.TrimSpace(m[4]),
			depth: len(strings.ReplaceAll(m[1], "\t", "  ")) / 2,
		})
	}
	return out
}

var (
	drawerOpen = regexp.MustCompile(`^\s*:[A-Za-z0-9_@#%-]+:\s*$`)
	drawerEnd  = regexp.MustCompile(`(?i)^\s*:END:\s*$`)
	planning   = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
)

func bodyLines(text string) []string {
	out := []string{}
	if text == "" {
		return out
	}
	in := false
	for _, line := range strings.Split(text, "\n") {
		if in {
			if drawerEnd.MatchString(line) {
				in = false
			}
			continue
		}
		if drawerOpen.MatchString(line) && !drawerEnd.MatchString(line) {
			in = true
			continue
		}
		if planning.MatchString(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

// cardRows lays one card out in w cells, the stripe column included.
func (a *app) cardRows(c *Card, w int, sel bool) []row {
	b := a.board()
	th := a.Th
	bg := th.Card
	if sel {
		bg = th.CardSel
	}
	base := a.Style(th.Text, bg)
	dim := a.Style(th.Dim, bg)
	inner := w - 3
	if inner < 6 {
		inner = 6
	}
	rows := []row{}

	// The bar across the top. Thin: it is a mark, not a heading.
	hc := HeaderColorOf(c, b, th.Dark)
	rows = append(rows, row{{strings.Repeat("▔", inner+1), a.Style(tuikit.Hex(hc), bg)}})

	// Label chips.
	if labels := LabelsOf(c, b); len(labels) > 0 {
		r := row{}
		used := 0
		for i, v := range labels {
			t := " " + v + " "
			if used+runewidth.StringWidth(t)+1 > inner {
				r = append(r, seg{fmt.Sprintf("+%d", len(labels)-i), dim})
				break
			}
			col := LabelColorOf(v, b)
			r = append(r, seg{t, a.Style(tuikit.Ink(col), tuikit.Hex(col))}, seg{" ", base})
			used += runewidth.StringWidth(t) + 1
		}
		rows = append(rows, r)
	}

	// The title, after its priority, up to three lines.
	lead := row{}
	leadW := 0
	if a.clockKey == c.Filename+"\x00"+c.Headline {
		lead = append(lead, seg{"◉ ", a.Style(th.Ok, bg).Bold(true)})
		leadW += 2
	}
	if c.Priority != "" {
		p := strings.ToUpper(c.Priority)
		pc := PriorityColor[p]
		if pc == "" {
			pc = "#6b7280"
		}
		lead = append(lead, seg{" " + p + " ", a.Style(tuikit.Ink(pc), tuikit.Hex(pc)).Bold(true)}, seg{" ", base})
		leadW += 4
	}
	title := c.Headline
	if title == "" {
		title = "(no headline)"
	}
	tst := base
	if sel {
		tst = tst.Bold(true)
	}
	lines := tuikit.Wrap(title, inner-leadW)
	if len(lines) > 3 {
		lines = lines[:3]
		lines[2] = tuikit.Truncate(lines[2]+" …", inner-leadW)
	}
	for i, l := range lines {
		r := row{}
		if i == 0 {
			r = append(r, lead...)
		} else if leadW > 0 {
			r = append(r, seg{strings.Repeat(" ", leadW), base})
		}
		rows = append(rows, append(r, seg{l, tst}))
	}

	// Keyword, checklist, badges, tags - wrapped onto two lines at most.
	chips := []seg{}
	if bd, ok := a.bodyOf(c.Hash); ok {
		if items := checklistOf(bd.Text); len(items) > 0 {
			done := 0
			for _, it := range items {
				if it.done {
					done++
				}
			}
			st := dim
			if done == len(items) {
				st = a.Style(th.Ok, bg).Bold(true)
			}
			chips = append(chips, seg{fmt.Sprintf("☑ %d/%d", done, len(items)), st})
		}
	}
	if c.Status != "" && b.GroupBy != "status" {
		chips = append(chips, seg{" " + c.Status + " ", a.Style(th.Dim, th.Chip)})
	}
	for _, name := range b.Badges {
		if v := prop(c, name); v != "" {
			chips = append(chips, seg{"‹" + v + "›", dim})
		}
	}
	for _, t := range c.Tags {
		if b.LabelSource == "tags" && contains(LabelsOf(c, b), t) {
			continue // already a chip of its own up top
		}
		chips = append(chips, seg{"#" + t, a.Style(tuikit.Hex(AutoSwatch(t).Hex), bg)})
	}
	if len(chips) > 0 {
		r, used, n := row{}, 0, 0
		for _, ch := range chips {
			cw := runewidth.StringWidth(ch.text)
			if used > 0 && used+1+cw > inner {
				rows = append(rows, r)
				n++
				r, used = row{}, 0
				if n == 2 {
					break
				}
			}
			if used > 0 {
				r = append(r, seg{" ", base})
				used++
			}
			r = append(r, ch)
			used += cw
		}
		if n < 2 && len(r) > 0 {
			rows = append(rows, r)
		}
	}

	// Dates and the file along the bottom.
	now := time.Now()
	foot := row{}
	fw := 0
	if IsSet(c.Deadline) {
		st := dim
		if overdue(c.Deadline.Start, now) {
			st = a.Style(th.Danger, bg).Bold(true)
		}
		t := "⚑ " + DateLabel(c.Deadline.Start, now)
		foot = append(foot, seg{t, st}, seg{"  ", base})
		fw += runewidth.StringWidth(t) + 2
	}
	if IsSet(c.Date) {
		t := "◷ " + DateLabel(c.Date.Start, now)
		foot = append(foot, seg{t, dim}, seg{"  ", base})
		fw += runewidth.StringWidth(t) + 2
	}
	file := baseName(c.Filename)
	room := inner - fw
	if room > 4 {
		file = tuikit.Truncate(file, room)
		pad := room - runewidth.StringWidth(file)
		foot = append(foot, seg{strings.Repeat(" ", pad), base}, seg{file, a.Style(th.Faint, bg)})
	}
	rows = append(rows, foot)
	return rows
}

// drawCard paints the rows at x,y, clipped to the rows between top and bottom.
func (a *app) drawCard(x, y, w int, c *Card, rows []row, sel bool, top, bottom int) {
	th := a.Th
	bg := th.Card
	if sel {
		bg = th.CardSel
	}
	stripe := a.Style(bg, bg)
	stripeCh := ' '
	if sw := CardSwatch(c, a.board()); sw != nil {
		stripe, stripeCh = a.Style(tuikit.Hex(sw.Hex), bg), '▌'
	} else if sel {
		stripe, stripeCh = a.Style(th.Accent, bg), '▌'
	}
	for i, r := range rows {
		yy := y + i
		if yy < top || yy >= bottom {
			continue
		}
		a.Fill(x, yy, w, a.Style(th.Text, bg))
		a.Scr.SetContent(x, yy, stripeCh, nil, stripe)
		a.paint(x+2, yy, w-3, r)
	}
}

// --- the frame ---------------------------------------------------------------------

func (a *app) draw() {
	a.Scr.Clear()
	w, h := a.Scr.Size()
	a.header(w)
	b := a.board()
	switch {
	case b == nil:
		a.empty(w, h, "No boards yet.", "N makes one - or make one in worg's Kanban tab; they are the same boards.")
	case a.problem != "":
		a.empty(w, h, a.problem, "S opens the board's settings.")
	case b.Layout == "list":
		a.drawList(w, h)
	default:
		a.drawBoard(w, h)
	}
	a.footer(w, h)
	a.DrawOverlay()
	a.Scr.Show()
}

func (a *app) empty(w, h int, line, hint string) {
	y := h / 2
	l := tuikit.Truncate(line, w-4)
	a.Put((w-runewidth.StringWidth(l))/2, y-1, l, a.Style(a.Th.Text, a.Th.Page).Bold(true), w)
	hh := tuikit.Truncate(hint, w-4)
	a.Put((w-runewidth.StringWidth(hh))/2, y+1, hh, a.Style(a.Th.Dim, a.Th.Page), w)
}

// header is the board tabs, and under them what this board is.
func (a *app) header(w int) {
	th := a.Th
	x := 1
	x += a.Put(x, 0, "▦ ", a.Style(th.Accent, th.Page).Bold(true), w-x)
	for i, b := range a.boards {
		t := " " + b.Name + " "
		st := a.Style(th.Dim, th.Page)
		if i == a.bi {
			st = a.Style(tuikit.Hex("#ffffff"), th.Accent).Bold(true)
		}
		if x+runewidth.StringWidth(t) > w-8 {
			a.Put(x, 0, "…", a.Style(th.Faint, th.Page), 1)
			break
		}
		x += a.Put(x, 0, t, st, w-x)
		x++
	}
	if !a.printing {
		hint := "? keys"
		a.Put(w-runewidth.StringWidth(hint)-1, 0, hint, a.Style(th.Faint, th.Page), w)
	}

	b := a.board()
	if b == nil {
		return
	}
	parts := []string{}
	if b.StoredQuery != "" {
		parts = append(parts, "saved query "+b.StoredQuery)
	} else if b.Query != "" {
		parts = append(parts, b.Query)
	}
	n := len(a.cards)
	shown := 0
	for _, cs := range a.buckets {
		shown += len(cs)
	}
	if a.search != "" {
		parts = append(parts, fmt.Sprintf("%d of %d cards", shown, n))
	} else {
		parts = append(parts, fmt.Sprintf("%d cards", n))
	}
	by := b.GroupBy
	if by == "property" {
		by = b.GroupKey
	}
	parts = append(parts, "by "+by, "sorted "+b.Sort)
	x = 3
	x += a.Put(x, 1, tuikit.Truncate(strings.Join(parts, "  ·  "), w-6), a.Style(th.Dim, th.Page), w-x)
	if a.loose > 0 {
		t := fmt.Sprintf("  ·  %d in no column", a.loose)
		a.Put(x, 1, t, a.Style(th.Warn, th.Page), w-x)
	}
}

func (a *app) footer(w, h int) {
	th := a.Th
	y := h - 1
	if a.printing {
		return
	}
	switch {
	case a.searching || a.search != "":
		x := 1
		x += a.Put(x, y, " / ", a.Style(tuikit.Hex("#ffffff"), th.Accent).Bold(true), w)
		x += a.Put(x+1, y, a.search, a.Style(th.Text, th.Page).Bold(true), w-x) + 1
		if a.searching {
			a.Scr.SetContent(x+1, y, '▏', nil, a.Style(th.Accent, th.Page))
		} else {
			a.Put(x+2, y, "esc clears", a.Style(th.Faint, th.Page), w-x)
		}
	case a.msg != "":
		st := a.Style(th.Ok, th.Page)
		mark := "✓ "
		if a.msgBad {
			st, mark = a.Style(th.Danger, th.Page), "✗ "
		}
		a.Put(1, y, tuikit.Truncate(mark+a.msg, w-2), st, w-2)
	default:
		keys := [][2]string{{"←→↑↓", "move"}, {"H L", "column"}, {"J K", "order"}, {"␣", "open"},
			{"t", "keyword"}, {"a", "labels"}, {"c", "clock"}, {"e", "edit"}, {"/", "search"},
			{"⇥", "board"}, {"?", "all keys"}}
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
}

// --- the columns -------------------------------------------------------------------

const (
	foldW = 3
	gap   = 2
	top   = 3
)

func (a *app) colColor(c Column) string {
	if c.Color == "" {
		return SwatchOf("slate").Hex
	}
	if strings.HasPrefix(c.Color, "#") {
		return c.Color
	}
	return SwatchOf(c.Color).Hex
}

func (a *app) drawBoard(w, h int) {
	th := a.Th
	if len(a.cols) == 0 {
		a.empty(w, h, "Nothing on this board.", "The query found no cards.")
		return
	}
	open := 0
	for _, c := range a.cols {
		if !a.folded(c.Value) {
			open++
		}
	}
	folds := len(a.cols) - open
	avail := w - 2 - folds*(foldW+gap)
	cw := 32
	if open > 0 {
		cw = avail/open - gap
	}
	if cw > 42 {
		cw = 42
	}
	if cw < 26 {
		cw = 26
	}
	widthOf := func(i int) int {
		if a.folded(a.cols[i].Value) {
			return foldW
		}
		return cw
	}

	// Scroll sideways so the selected column is whole.
	if a.ci < a.colScroll {
		a.colScroll = a.ci
	}
	for {
		x := 1
		for i := a.colScroll; i <= a.ci; i++ {
			x += widthOf(i) + gap
		}
		if x-gap <= w-1 || a.colScroll >= a.ci {
			break
		}
		a.colScroll++
	}

	x := 1
	bottom := h - 2
	for i := a.colScroll; i < len(a.cols); i++ {
		cwid := widthOf(i)
		if x+cwid > w {
			a.Put(w-2, top, "▸", a.Style(th.Accent, th.Page).Bold(true), 1)
			break
		}
		if a.folded(a.cols[i].Value) {
			a.drawFolded(x, i, bottom)
		} else {
			a.drawColumn(x, cwid, i, bottom)
		}
		x += cwid + gap
	}
	if a.colScroll > 0 {
		a.Put(0, top, "◂", a.Style(th.Accent, th.Page).Bold(true), 1)
	}
}

func (a *app) drawFolded(x, i, bottom int) {
	th := a.Th
	col := a.cols[i]
	cc := tuikit.Hex(a.colColor(col))
	sel := i == a.ci && !a.printing
	st := a.Style(cc, th.Page)
	if sel {
		st = a.Style(tuikit.Hex("#ffffff"), cc).Bold(true)
	}
	a.Put(x, top, " ▸ ", st, foldW)
	title := []rune(strings.ToUpper(a.titleOf(col.Value)))
	y := top + 2
	for _, r := range title {
		if y >= bottom-2 {
			break
		}
		a.Scr.SetContent(x+1, y, r, nil, a.Style(cc, th.Page))
		y++
	}
	n := fmt.Sprintf("%d", len(a.colCards(i)))
	a.Put(x+1, y+1, n, a.Style(th.Dim, th.Page), foldW)
}

func (a *app) drawColumn(x, cw, i, bottom int) {
	th := a.Th
	col := a.cols[i]
	cc := tuikit.Hex(a.colColor(col))
	sel := i == a.ci && !a.printing
	cards := a.colCards(i)

	// The heading: a dot of the column's colour, its name, how many, and
	// the work-in-progress limit when it has one - red once it is over.
	hx := x
	hx += a.Put(hx, top, "● ", a.Style(cc, th.Page), cw)
	title := a.titleOf(col.Value)
	tst := a.Style(th.Text, th.Page).Bold(true)
	if sel {
		tst = a.Style(cc, th.Page).Bold(true)
	}
	count := fmt.Sprintf("%d", len(cards))
	cst := a.Style(th.Dim, th.Page)
	if col.Limit > 0 {
		count = fmt.Sprintf("%d/%d", len(cards), col.Limit)
		if len(cards) > col.Limit {
			cst = a.Style(th.Danger, th.Page).Bold(true)
			count += " over"
		}
	}
	hx += a.Put(hx, top, tuikit.Truncate(title, cw-runewidth.StringWidth(count)-4), tst, cw-(hx-x))
	a.Put(hx+2, top, count, cst, cw-(hx-x)-2)
	rule := "─"
	rst := a.Style(th.Faint, th.Page)
	if sel {
		rule, rst = "━", a.Style(cc, th.Page)
	}
	a.Put(x, top+1, strings.Repeat(rule, cw), rst, cw)

	y0 := top + 3
	if len(cards) == 0 {
		a.Put(x+2, y0, "empty", a.Style(th.Faint, th.Page).Italic(true), cw)
		return
	}
	layouts := make([][]row, len(cards))
	for k := range cards {
		layouts[k] = a.cardRows(&cards[k], cw, sel && k == a.ri)
	}
	// Keep the selected card whole: scroll the column by cards.
	key := col.Value
	sc := a.rowScroll[key]
	if sel {
		if a.ri < sc {
			sc = a.ri
		}
		for {
			hgt := 0
			for k := sc; k <= a.ri && k < len(layouts); k++ {
				hgt += len(layouts[k]) + 1
			}
			if y0+hgt-1 <= bottom || sc >= a.ri {
				break
			}
			sc++
		}
		a.rowScroll[key] = sc
	}
	if sc >= len(cards) {
		sc = 0
	}
	if sc > 0 {
		a.Put(x+cw-9, top+2, fmt.Sprintf("↑ %d more", sc), a.Style(th.Faint, th.Page), 10)
	}
	y := y0
	k := sc
	for ; k < len(cards); k++ {
		if y >= bottom {
			break
		}
		a.drawCard(x, y, cw, &cards[k], layouts[k], sel && k == a.ri, y0, bottom)
		y += len(layouts[k]) + 1
	}
	if more := len(cards) - k; more > 0 || y > bottom+1 {
		if more == 0 {
			more = 1
		}
		a.Fill(x, bottom, cw, a.Style(th.Text, th.Page))
		a.Put(x+cw-9, bottom, fmt.Sprintf("↓ %d more", more), a.Style(th.Faint, th.Page), 10)
	}
}

// --- the list layout ---------------------------------------------------------------

// drawList is the same board as one grouped list: a section per column, a row
// per card, and the board's list fields as columns down the right.
func (a *app) drawList(w, h int) {
	th := a.Th
	b := a.board()
	fields := ListFieldsOf(b)
	type line struct {
		section int
		card    int // -1 for the section's heading
	}
	lines := []line{}
	selLine := 0
	for i := range a.cols {
		if i == a.ci && (a.folded(a.cols[i].Value) || len(a.colCards(i)) == 0) {
			selLine = len(lines)
		}
		lines = append(lines, line{i, -1})
		if a.folded(a.cols[i].Value) {
			continue
		}
		for k := range a.colCards(i) {
			if i == a.ci && k == a.ri {
				selLine = len(lines)
			}
			lines = append(lines, line{i, k})
		}
		lines = append(lines, line{i, -2}) // a gap
	}
	y0, bottom := top, h-2
	room := bottom - y0
	sc := a.rowScroll["\x00list"]
	if selLine < sc {
		sc = selLine
	}
	if selLine >= sc+room {
		sc = selLine - room + 1
	}
	a.rowScroll["\x00list"] = sc

	statusW := 0
	for _, c := range a.cards {
		if n := runewidth.StringWidth(c.Status); n > statusW {
			statusW = n
		}
	}
	fieldsW := 0
	for _, f := range fields {
		fieldsW += f.Width + 2
	}
	// The field headings, over the first rows.
	fx := w - 1 - fieldsW
	if fx > 30 {
		for _, f := range fields {
			a.Put(fx, top-1, tuikit.Truncate(strings.ToUpper(f.Label), f.Width), a.Style(th.Faint, th.Page), f.Width)
			fx += f.Width + 2
		}
	}

	for n := 0; n < room && sc+n < len(lines); n++ {
		ln := lines[sc+n]
		y := y0 + n
		col := a.cols[ln.section]
		cc := tuikit.Hex(a.colColor(col))
		switch ln.card {
		case -2:
			continue
		case -1:
			sel := !a.printing && ln.section == a.ci && (a.folded(col.Value) || len(a.colCards(ln.section)) == 0)
			arrow := "▾ "
			if a.folded(col.Value) {
				arrow = "▸ "
			}
			x := 1
			st := a.Style(cc, th.Page).Bold(true)
			if sel {
				a.Fill(0, y, w, a.Style(th.Text, th.CardSel))
				st = st.Background(th.CardSel)
			}
			x += a.Put(x, y, arrow+"● "+a.titleOf(col.Value), st, w-x)
			cnt := fmt.Sprintf("  %d", len(a.colCards(ln.section)))
			cst := a.Style(th.Dim, th.Page)
			if col.Limit > 0 {
				cnt = fmt.Sprintf("  %d/%d", len(a.colCards(ln.section)), col.Limit)
				if len(a.colCards(ln.section)) > col.Limit {
					cst = a.Style(th.Danger, th.Page).Bold(true)
				}
			}
			if sel {
				cst = cst.Background(th.CardSel)
			}
			a.Put(x, y, cnt, cst, w-x)
		default:
			c := a.colCards(ln.section)[ln.card]
			sel := ln.section == a.ci && ln.card == a.ri && !a.printing
			bg := th.Page
			if sel {
				bg = th.CardSel
				a.Fill(0, y, w, a.Style(th.Text, bg))
			}
			x := 3
			stripe := a.Style(cc, bg)
			if sw := CardSwatch(&c, b); sw != nil {
				stripe = a.Style(tuikit.Hex(sw.Hex), bg)
			}
			a.Put(x, y, "▌", stripe, 1)
			x += 2
			if statusW > 0 {
				a.Put(x, y, c.Status, a.Style(th.Dim, bg).Bold(true), statusW)
				x += statusW + 2
			}
			if a.clockKey == c.Filename+"\x00"+c.Headline {
				x += a.Put(x, y, "◉ ", a.Style(th.Ok, bg).Bold(true), 2)
			}
			titleW := w - 1 - fieldsW - x - 1
			tst := a.Style(th.Text, bg)
			if sel {
				tst = tst.Bold(true)
			}
			for _, l := range LabelsOf(&c, b) {
				lc := LabelColorOf(l, b)
				if titleW < 12 {
					break
				}
				a.Put(x, y, "▬", a.Style(tuikit.Hex(lc), bg), 1)
				x++
				titleW--
			}
			if len(LabelsOf(&c, b)) > 0 {
				x++
				titleW--
			}
			a.Put(x, y, tuikit.Truncate(c.Headline, titleW), tst, titleW)
			fx := w - 1 - fieldsW
			if fx > 30 {
				for _, f := range fields {
					a.listCell(fx, y, f, &c, bg)
					fx += f.Width + 2
				}
			}
		}
	}
	if sc > 0 {
		a.Put(w-10, top, fmt.Sprintf("↑ %d more", sc), a.Style(th.Faint, th.Page), 10)
	}
	if rest := len(lines) - (sc + room); rest > 0 {
		a.Put(w-10, bottom-1, fmt.Sprintf("↓ %d more", rest), a.Style(th.Faint, th.Page), 10)
	}
}

func (a *app) listCell(x, y int, f ListField, c *Card, bg tcell.Color) {
	th := a.Th
	now := time.Now()
	switch f.Key {
	case "priority":
		if c.Priority != "" {
			p := strings.ToUpper(c.Priority)
			pc := PriorityColor[p]
			if pc == "" {
				pc = "#6b7280"
			}
			a.Put(x, y, " "+p+" ", a.Style(tuikit.Ink(pc), tuikit.Hex(pc)).Bold(true), f.Width)
		}
	case "deadline", "scheduled":
		d := c.Deadline
		mark := "⚑ "
		if f.Key == "scheduled" {
			d, mark = c.Date, "◷ "
		}
		if IsSet(d) {
			st := a.Style(th.Dim, bg)
			if f.Key == "deadline" && overdue(d.Start, now) {
				st = a.Style(th.Danger, bg).Bold(true)
			}
			a.Put(x, y, mark+DateLabel(d.Start, now), st, f.Width)
		}
	case "tags":
		at := 0
		for _, t := range c.Tags {
			s := "#" + t
			if at+runewidth.StringWidth(s) > f.Width {
				break
			}
			at += a.Put(x+at, y, s, a.Style(tuikit.Hex(AutoSwatch(t).Hex), bg), f.Width-at) + 1
		}
	default:
		a.Put(x, y, tuikit.Truncate(ListFieldText(c, f.Key), f.Width), a.Style(th.Dim, bg), f.Width)
	}
}

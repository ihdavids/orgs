package gantt

// Drawing the chart in a terminal.
//
// A row is a strip of cells, one per column, each knowing its character and
// colours; the strip is turned into escapes once at the end, so nothing has to
// track what the terminal was last told. Bars are coloured cells rather than
// block characters wherever a whole cell is covered, which is what lets a bar
// carry text, and the cell a bar only partly covers gets one of the eighth
// blocks - so a half day at a coarse zoom is still half a cell.

import (
	"fmt"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

type cell struct {
	ch           rune
	fg, bg       rgb
	hasFg, hasBg bool
	dim, bold    bool
}

type view struct {
	m      *Model
	sch    scheme
	pal    palette
	now    time.Time
	crit   map[string]bool
	width  int
	from   time.Time // first day drawn
	to     time.Time // the midnight after the last day drawn
	title  string
	query  string
	colour bool

	labelW  int
	statusW int
	cols    int
	cpd     float64 // columns per day
}

const (
	indent = 2
	sep    = 2 // between the labels and the plot
)

func (v *view) layout() {
	days := v.to.Sub(v.from).Hours() / 24
	if days < 1 {
		days = 1
	}
	for _, t := range v.m.Tasks {
		if n := commands.RuneLen(t.Src.Status); n > v.statusW {
			v.statusW = n
		}
	}
	// Room for the gutter, the keyword, the arrow and a title worth reading,
	// but never so much the plot is squeezed out.
	want := 0
	for _, t := range v.m.Tasks {
		n := 1 + commands.RuneLen(t.Name) + 2
		if v.statusW > 0 {
			n += v.statusW + 1
		}
		if n > want {
			want = n
		}
	}
	for _, s := range v.m.Sections {
		if n := commands.RuneLen(s.Name) + 3; n > want {
			want = n
		}
	}
	maxLabel := v.width * 2 / 5
	if maxLabel > 48 {
		maxLabel = 48
	}
	if want > maxLabel {
		want = maxLabel
	}
	if want < 14 {
		want = 14
	}
	v.labelW = want

	room := v.width - indent - v.labelW - sep - 1
	if room < 10 {
		room = 10
	}
	v.cpd = float64(room) / days
	if v.cpd >= 1 {
		// Whole columns per day, so days line up with the cells and a
		// weekend is a weekend all the way down.
		c := int(v.cpd)
		if c > 8 {
			c = 8
		}
		v.cpd = float64(c)
		v.cols = int(days) * c
	} else {
		v.cols = room
	}
}

func (v *view) x(t time.Time) float64 {
	return t.Sub(v.from).Hours() / 24 * v.cpd
}

// col is the column a moment falls in, or -1 off the chart.
func (v *view) col(t time.Time) int {
	x := v.x(t)
	if x < 0 || int(x) >= v.cols {
		return -1
	}
	return int(x)
}

// --- the background every plot row shares -----------------------------------

func (v *view) grid() []cell {
	row := make([]cell, v.cols)
	for i := range row {
		row[i] = cell{ch: ' '}
	}
	if v.cpd >= 1 {
		// A weekend is a dot in the middle of its day: a band at a glance,
		// without painting a background the terminal may not suit.
		c := int(v.cpd)
		for d := 0; d*c < v.cols; d++ {
			wd := addDays(v.from, d).Weekday()
			if wd == time.Saturday || wd == time.Sunday {
				row[d*c+c/2] = cell{ch: '·', dim: true}
			}
		}
	}
	// The first of each month is a faint rule down the chart.
	for d := startOfDay(v.from); d.Before(v.to); d = addDays(d, 1) {
		if d.Day() == 1 && !d.Equal(startOfDay(v.from)) {
			if c := v.col(d); c >= 0 {
				row[c] = cell{ch: '┊', dim: true}
			}
		}
	}
	for _, t := range v.m.Tasks {
		if t.Mark {
			if c := v.col(t.Start); c >= 0 {
				row[c] = cell{ch: '┆', fg: markColor, hasFg: true}
			}
		}
	}
	if c := v.col(v.now); c >= 0 {
		row[c] = cell{ch: '│', fg: todayColor, hasFg: true}
	}
	return row
}

// --- one task ----------------------------------------------------------------

var eighths = []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

func (v *view) bar(row []cell, t *Task) {
	color := v.sch.color(t)
	if t.Src.Implied {
		color = mix(color, v.pal.bg, 0.55)
	}
	if t.Mark {
		if c := v.col(t.Start); c >= 0 {
			row[c] = cell{ch: '┃', fg: markColor, hasFg: true, bold: true}
		}
		return
	}
	if t.Milestone {
		c := v.col(t.Start)
		if c < 0 {
			v.offChart(row, t, color)
			return
		}
		row[c] = cell{ch: '◆', fg: color, hasFg: true, bold: true}
		v.note(row, t, float64(c), float64(c+1))
		return
	}

	xs, xe := v.x(t.Start), v.x(t.End)
	if xe <= 0 || xs >= float64(v.cols) {
		v.offChart(row, t, color)
		return
	}
	// The done part of a bar that records its progress is the bar's colour,
	// the rest a wash of it - worg's progress fill, one row high.
	wash := mix(color, v.pal.bg, 0.6)
	split := xe
	if tracked(t) && t.State != "done" && t.Src.Percent < 100 {
		split = xs + (xe-xs)*float64(t.Src.Percent)/100
	}
	paint := func(at float64) rgb {
		if at < split {
			return color
		}
		return wash
	}

	first, last := -1, -1
	if xe-xs < 1 {
		// Narrower than a cell: still something you can see.
		c := int(xs)
		if c < 0 {
			c = 0
		}
		row[c] = cell{ch: '▊', fg: color, hasFg: true}
		first, last = c, c
	} else {
		for c := int(xs); c < v.cols && float64(c) < xe; c++ {
			if c < 0 {
				continue
			}
			lo, hi := float64(c), float64(c+1)
			if xs > lo {
				lo = xs
			}
			if xe < hi {
				hi = xe
			}
			cover := hi - lo
			mid := (lo + hi) / 2
			switch {
			case cover > 0.94:
				row[c] = cell{ch: ' ', bg: paint(mid), hasBg: true}
				if !v.colour {
					row[c] = cell{ch: '█'}
					if paint(mid) == wash {
						row[c].ch = '░'
					}
				}
			case xs > float64(c):
				// The bar starts partway in: only the right half has a block.
				if cover >= 0.4 {
					row[c] = cell{ch: '▐', fg: paint(mid), hasFg: true}
				} else {
					continue
				}
			default:
				n := int(cover*8 + 0.5)
				if n < 1 {
					continue
				}
				row[c] = cell{ch: eighths[n], fg: paint(mid), hasFg: true}
			}
			if first < 0 {
				first = c
			}
			last = c
		}
	}
	if first < 0 {
		return
	}
	if xs < 0 {
		row[first] = cell{ch: '◂', fg: color, hasFg: true, bold: true}
	}
	if xe > float64(v.cols) {
		row[last] = cell{ch: '▸', fg: color, hasFg: true, bold: true}
	}

	// What the bar says about itself, written inside it when there is room:
	// how far along it is, or how long somebody thought it would take.
	if v.colour {
		label := ""
		switch {
		case tracked(t) && t.State != "done":
			label = fmt.Sprintf("%d%%", t.Src.Percent)
		case t.PlannedDays > 0:
			label = plannedText(t.PlannedDays)
		}
		inner := []rune(label)
		start := first + 1
		if label != "" && start+len(inner) <= last {
			for i, r := range inner {
				c := &row[start+i]
				if c.hasBg {
					c.ch = r
					c.fg, c.hasFg = c.bg.ink(), true
					c.bold = true
				}
			}
		}
	}
	v.note(row, t, xs, xe)
}

func plannedText(d float64) string {
	if d == float64(int(d)) {
		return fmt.Sprintf("%dd", int(d))
	}
	return fmt.Sprintf("%.1fd", d)
}

// noteOf is what a glance at a bar wants next: trouble if there is any, else
// its dates.
func (v *view) noteOf(t *Task) (string, cell) {
	kind, text := Health(t, v.now)
	switch kind {
	case "overdue":
		return text, cell{fg: overdueColor, hasFg: true}
	case "slipping":
		return text, cell{fg: slippingColor, hasFg: true}
	}
	return dateText(t), cell{dim: true}
}

// note writes it after the bar, or before it when the bar runs to the edge.
func (v *view) note(row []cell, t *Task, xs, xe float64) {
	text, st := v.noteOf(t)
	r := []rune(text)
	at := int(xe+0.999) + 1
	if at+len(r) > v.cols {
		at = int(xs) - 1 - len(r)
	}
	v.put(row, r, at, st)
}

func (v *view) put(row []cell, r []rune, at int, st cell) {
	if len(r) == 0 || at < 0 || at+len(r) > v.cols {
		return
	}
	for i, ch := range r {
		st.ch = ch
		row[at+i] = st
	}
}

func dateText(t *Task) string {
	if t.Milestone {
		return t.Start.Format("Jan 2")
	}
	last := t.End.Add(-time.Nanosecond)
	if startOfDay(last).Equal(startOfDay(t.Start)) {
		return t.Start.Format("Jan 2")
	}
	if t.Start.Month() == last.Month() {
		return fmt.Sprintf("%s–%d", t.Start.Format("Jan 2"), last.Day())
	}
	return fmt.Sprintf("%s – %s", t.Start.Format("Jan 2"), last.Format("Jan 2"))
}

// A task wholly before or after the window is an arrow at that edge, so a
// row is never blank without saying why.
func (v *view) offChart(row []cell, t *Task, color rgb) {
	text, st := v.noteOf(t)
	r := []rune(text)
	if !t.End.After(v.from) && !t.Start.After(v.from) {
		row[0] = cell{ch: '◂', fg: color, hasFg: true, bold: true}
		v.put(row, r, 2, st)
		return
	}
	row[v.cols-1] = cell{ch: '▸', fg: color, hasFg: true, bold: true}
	v.put(row, r, v.cols-2-len(r), st)
}

// --- turning cells into text -------------------------------------------------

func (v *view) style(c cell) string {
	if !v.colour {
		return ""
	}
	s := "\033[0m"
	if c.bold {
		s += "\033[1m"
	}
	if c.dim {
		s += "\033[2m"
	}
	if c.hasFg {
		s += fg(c.fg)
	}
	if c.hasBg {
		s += bg(c.bg)
	}
	return s
}

func (v *view) emit(b *strings.Builder, row []cell) {
	prev := "\033[0m"
	for _, c := range row {
		if st := v.style(c); st != prev && v.colour {
			b.WriteString(st)
			prev = st
		}
		b.WriteRune(c.ch)
	}
	if v.colour {
		b.WriteString("\033[0m")
	}
}

// text is a run of cells from a string in one style.
func text(s string, c cell) []cell {
	out := []cell{}
	for _, r := range s {
		c.ch = r
		out = append(out, c)
	}
	return out
}

func pad(cells []cell, n int) []cell {
	for len(cells) < n {
		cells = append(cells, cell{ch: ' '})
	}
	return cells
}

// --- the whole chart ---------------------------------------------------------

func (v *view) render() string {
	v.layout()
	b := &strings.Builder{}
	nl := func() { b.WriteString("\n") }
	spaces := strings.Repeat(" ", indent)
	left := strings.Repeat(" ", indent+v.labelW+sep)

	// The title: what was asked, how much came back, and when it runs.
	head := text("▍", cell{fg: v.pal.todo, hasFg: true, bold: true})
	head = append(head, text(v.title, cell{bold: true})...)
	n := 0
	for _, t := range v.m.Tasks {
		if !t.Mark && !t.Src.Implied {
			n++
		}
	}
	plural := "s"
	if n == 1 {
		plural = ""
	}
	last := v.to.Add(-time.Nanosecond)
	span := fmt.Sprintf("  ·  %d task%s  ·  %s – %s", n, plural, v.from.Format("Jan 2"), last.Format("Jan 2 2006"))
	if v.from.Year() != last.Year() {
		span = fmt.Sprintf("  ·  %d task%s  ·  %s – %s", n, plural, v.from.Format("Jan 2 2006"), last.Format("Jan 2 2006"))
	}
	head = append(head, text(span, cell{dim: true})...)
	b.WriteString(spaces)
	v.emit(b, head)
	nl()
	if v.query != "" && v.query != v.title {
		b.WriteString(spaces)
		v.emit(b, text("  "+commands.Ellipsis(v.query, v.width-indent-3), cell{dim: true}))
		nl()
	}
	nl()

	// Months, then days, then a rule with today on it.
	months := make([]cell, v.cols)
	for i := range months {
		months[i] = cell{ch: ' '}
	}
	firstLabel := true
	for d := startOfDay(v.from); d.Before(v.to); d = addDays(d, 1) {
		if d.Day() != 1 && !d.Equal(startOfDay(v.from)) {
			continue
		}
		c := v.col(d)
		next := v.cols
		if nc := v.col(time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, d.Location())); nc > 0 {
			next = nc
		}
		room := next - c - 1
		name := d.Format("January")
		if firstLabel || d.Month() == time.January {
			name = d.Format("January 2006")
		}
		if len(name) > room {
			name = d.Format("Jan 2006")
		}
		if len(name) > room {
			name = d.Format("Jan")
		}
		if len(name) > room && !(firstLabel && room >= 3) {
			firstLabel = false
			continue
		}
		if len(name) > room {
			name = name[:room]
		}
		for i, r := range name {
			months[c+i] = cell{ch: r, bold: true}
		}
		firstLabel = false
	}
	b.WriteString(left)
	v.emit(b, months)
	nl()

	days := make([]cell, v.cols)
	rule := make([]cell, v.cols)
	for i := range days {
		days[i] = cell{ch: ' '}
		rule[i] = cell{ch: '─', dim: true}
	}
	everyDay := v.cpd >= 3
	mondays := !everyDay && v.cpd*7 >= 4
	end := -1
	for d := startOfDay(v.from); d.Before(v.to); d = addDays(d, 1) {
		if !everyDay && !(mondays && d.Weekday() == time.Monday) {
			continue
		}
		c := v.col(d)
		label := fmt.Sprintf("%d", d.Day())
		if c < 0 || c <= end || c+len(label) > v.cols {
			continue
		}
		weekend := d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
		isToday := startOfDay(v.now).Equal(d)
		for i, r := range label {
			days[c+i] = cell{ch: r, dim: weekend && !isToday}
			if isToday {
				days[c+i] = cell{ch: r, fg: todayColor, hasFg: true, bold: true}
			}
		}
		rule[c] = cell{ch: '┬', dim: true}
		end = c + len(label)
	}
	for _, t := range v.m.Tasks {
		if t.Mark {
			if c := v.col(t.Start); c >= 0 {
				rule[c] = cell{ch: '┬', fg: markColor, hasFg: true}
			}
		}
	}
	if c := v.col(v.now); c >= 0 {
		rule[c] = cell{ch: '▼', fg: todayColor, hasFg: true, bold: true}
	}
	if everyDay || mondays {
		b.WriteString(left)
		v.emit(b, days)
		nl()
	}
	b.WriteString(left)
	v.emit(b, rule)
	nl()

	// The lanes. A chart that is one lane called main says nothing by naming it.
	showLanes := len(v.m.Sections) > 1 || (len(v.m.Sections) == 1 && v.m.Sections[0].Name != DefaultSection)
	for si, s := range v.m.Sections {
		if showLanes {
			if si > 0 {
				b.WriteString(left)
				v.emit(b, v.grid())
				nl()
			}
			laneColor := categorical[si%len(categorical)]
			if v.sch.title == "Lane" && len(s.Tasks) > 0 {
				laneColor = v.sch.color(s.Tasks[0])
			}
			label := text("▌", cell{fg: laneColor, hasFg: true})
			label = append(label, text(" "+commands.Ellipsis(s.Name, v.labelW-3)+" ", cell{bold: true, fg: laneColor, hasFg: true})...)
			for len(label) < v.labelW+sep-1 {
				label = append(label, cell{ch: '─', dim: true})
			}
			b.WriteString(spaces)
			v.emit(b, pad(label, v.labelW+sep))
			v.emit(b, v.grid())
			nl()
		}
		for _, t := range s.Tasks {
			b.WriteString(spaces)
			v.emit(b, v.label(t))
			row := v.grid()
			v.bar(row, t)
			v.emit(b, row)
			nl()
		}
	}

	nl()
	v.legend(b)
	for _, e := range v.m.Errors {
		b.WriteString(spaces)
		v.emit(b, text("! "+e, cell{dim: true}))
		nl()
	}
	return b.String()
}

// label is the heading's side of its row: a mark when it is on the critical
// path, its keyword, an arrow when it waits on something, and its title.
func (v *view) label(t *Task) []cell {
	out := []cell{}
	if v.crit[t.ID] {
		out = append(out, cell{ch: '┃', fg: v.pal.crit, hasFg: true, bold: true})
	} else {
		out = append(out, cell{ch: ' '})
	}
	if v.statusW > 0 {
		st := cell{fg: hex("#d4a12a"), hasFg: true, bold: true}
		switch t.State {
		case "done":
			st = cell{fg: v.pal.done, hasFg: true}
		case "active":
			st = cell{fg: v.pal.active, hasFg: true, bold: true}
		case "crit":
			st = cell{fg: v.pal.crit, hasFg: true, bold: true}
		}
		out = append(out, text(fmt.Sprintf("%-*s ", v.statusW, t.Src.Status), st)...)
	}
	name := cell{}
	switch {
	case t.Src.Implied || t.State == "done":
		name.dim = true
	case t.Mark:
		name = cell{fg: markColor, hasFg: true}
	}
	if len(t.After) > 0 {
		out = append(out, text("↳ ", cell{dim: true})...)
	}
	room := v.labelW - len(out)
	out = append(out, text(commands.Ellipsis(t.Name, room), name)...)
	return pad(out, v.labelW+sep)
}

func (v *view) legend(b *strings.Builder) {
	row := text(v.sch.title+"  ", cell{dim: true})
	for _, e := range v.sch.legend {
		if e.own {
			row = append(row, text("◇ "+e.label+"  ", cell{dim: true})...)
			continue
		}
		row = append(row, text("■", cell{fg: e.color, hasFg: true})...)
		label := " " + e.label
		if e.count > 0 {
			label += fmt.Sprintf(" %d", e.count)
		}
		row = append(row, text(label+"  ", cell{})...)
	}
	extras := [][]cell{
		append(text("│", cell{fg: todayColor, hasFg: true}), text(" today  ", cell{dim: true})...),
	}
	for _, t := range v.m.Tasks {
		if t.Milestone {
			extras = append(extras, text("◆ milestone  ", cell{dim: true}))
			break
		}
	}
	for _, t := range v.m.Tasks {
		if t.Mark {
			extras = append(extras, append(text("┆", cell{fg: markColor, hasFg: true}), text(" mark  ", cell{dim: true})...))
			break
		}
	}
	if len(v.crit) > 0 {
		extras = append(extras, append(text("┃", cell{fg: v.pal.crit, hasFg: true}), text(" critical path  ", cell{dim: true})...))
	}
	for _, t := range v.m.Tasks {
		if len(t.After) > 0 {
			extras = append(extras, text("↳ waits on another  ", cell{dim: true}))
			break
		}
	}
	// One line when it fits, two when it does not.
	more := []cell{}
	for _, e := range extras {
		more = append(more, e...)
	}
	b.WriteString(strings.Repeat(" ", indent))
	if len(row)+len(more) <= v.width-indent {
		v.emit(b, append(row, more...))
		b.WriteString("\n")
		return
	}
	v.emit(b, row)
	b.WriteString("\n")
	b.WriteString(strings.Repeat(" ", indent))
	v.emit(b, more)
	b.WriteString("\n")
}

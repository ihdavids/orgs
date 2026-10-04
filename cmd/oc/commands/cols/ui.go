package cols

// The column view in a terminal: worg's Columns tab, keys instead of a mouse.
//
// Every write goes through an endpoint that already existed - /property,
// /status/change, /headline/change, /heading/parts, /columns/spec,
// /gantt/add - and the view is read back afterwards rather than patched in
// place: a rollup is of the whole subtree, so one edited estimate changes the
// cell above it and the one above that, and working that out here would be
// saying the rollup rule a second time.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"github.com/ihdavids/orgs/internal/common"
)

type app struct {
	*tuikit.UI
	core *commands.Core

	file   string
	spec   string // the line asked for; empty lets the file and the config decide
	data   Result
	values common.ColumnValuesResult
	states common.TodoStatesResult

	rows      []Row // what is drawn, in order
	ancestors map[string][]string
	totals    map[int]map[string]float64
	folded    map[string]bool

	sortBy    string // the property sorted on, by name so moving columns keeps it
	sortDesc  bool
	filterBy  string // property name the rows are narrowed by
	filterVal string

	ri, ci  int
	scroll  int
	hscroll int

	msg     string
	msgBad  bool
	msgTill time.Time

	edit     func(string, int)
	termEdit bool
	printing bool
	quit     bool
}

// --- reading --------------------------------------------------------------------

func (a *app) load() {
	keep := ""
	if r := a.selected(); r != nil {
		keep = r.Headline
	}
	ps := map[string]string{"file": a.file}
	if a.spec != "" {
		ps["columns"] = a.spec
	}
	res, err := commands.SendReceiveGetErr[Result](a.core, "columns", ps)
	if err != nil {
		a.data = Result{Msg: err.Error()}
	} else {
		a.data = res
	}
	a.values = commands.SendReceiveGetOr[common.ColumnValuesResult](a.core, "columns/values", map[string]string{"file": a.file})
	a.layout()
	if keep != "" {
		for i, r := range a.rows {
			if r.Headline == keep {
				a.ri = i
			}
		}
	}
	a.clamp()
}

func (a *app) opts() Options { return OptionsOf(a.data.Columns) }

func (a *app) colIndex(prop string) int {
	for i, s := range a.data.Spec {
		if strings.EqualFold(s.Property, prop) {
			return i
		}
	}
	return -1
}

// sortCol is the column the rows are sorted on, or -1.
func (a *app) sortCol() int {
	if a.sortBy == "" {
		return -1
	}
	return a.colIndex(a.sortBy)
}

func (a *app) itemCol() int {
	for i, s := range a.data.Spec {
		if s.Kind == "item" {
			return i
		}
	}
	return -1
}

// layout works out the rows on show: narrowed to a value, sorted within
// their parents, and with folded branches taken out.
func (a *app) layout() {
	base := a.data.Rows
	a.ancestors = AncestorsOf(base)
	a.totals = map[int]map[string]float64{}
	for i, s := range a.data.Spec {
		if s.Numbers != "" && s.Summary == "" {
			a.totals[i] = SubtreeTotals(base, i)
		}
	}
	rows := base
	if a.filterBy != "" {
		if c := a.colIndex(a.filterBy); c >= 0 {
			keep := RowsWithValue(base, c, a.filterVal)
			kept := []Row{}
			for _, r := range base {
				if keep[r.Hash] {
					kept = append(kept, r)
				}
			}
			rows = kept
		}
	}
	if c := a.sortCol(); c >= 0 {
		rows = SortWithinParents(rows, c, a.sortDesc, a.data.Spec[c].Numbers != "")
	}
	if !a.opts().Flat {
		hidden := HiddenRows(rows, a.folded)
		shown := []Row{}
		for _, r := range rows {
			if !hidden[r.Hash] {
				shown = append(shown, r)
			}
		}
		rows = shown
	}
	a.rows = rows
	a.clamp()
}

func (a *app) clamp() {
	if a.ri >= len(a.rows) {
		a.ri = len(a.rows) - 1
	}
	if a.ri < 0 {
		a.ri = 0
	}
	if a.ci >= len(a.data.Spec) {
		a.ci = len(a.data.Spec) - 1
	}
	if a.ci < 0 {
		a.ci = 0
	}
}

func (a *app) selected() *Row {
	if a.ri < 0 || a.ri >= len(a.rows) {
		return nil
	}
	r := a.rows[a.ri]
	return &r
}

func (a *app) spec1() *Spec {
	if a.ci < 0 || a.ci >= len(a.data.Spec) {
		return nil
	}
	return &a.data.Spec[a.ci]
}

func (a *app) say(format string, args ...interface{}) {
	a.msg, a.msgBad, a.msgTill = fmt.Sprintf(format, args...), false, time.Now().Add(6*time.Second)
}

func (a *app) complain(format string, args ...interface{}) {
	a.msg, a.msgBad, a.msgTill = fmt.Sprintf(format, args...), true, time.Now().Add(10*time.Second)
}

// --- the loop -------------------------------------------------------------------

func (a *app) run() {
	a.load()
	a.draw()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	events := make(chan tcell.Event, 16)
	stop := make(chan struct{})
	go a.Scr.ChannelEvents(events, stop)
	for !a.quit {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch ev := ev.(type) {
			case *tcell.EventKey:
				a.key(ev)
			case *tcell.EventResize:
				a.Scr.Sync()
			}
			a.draw()
		case <-tick.C:
			if a.msg != "" && time.Now().After(a.msgTill) {
				a.msg = ""
				a.draw()
			}
		}
	}
	close(stop)
}

func (a *app) key(ev *tcell.EventKey) {
	if a.KeyOverlay(ev) {
		return
	}
	shift := ev.Modifiers()&tcell.ModShift != 0
	switch ev.Key() {
	case tcell.KeyCtrlC, tcell.KeyEscape:
		if ev.Key() == tcell.KeyEscape && a.filterBy != "" {
			a.filterBy = ""
			a.layout()
			return
		}
		a.quit = true
		return
	case tcell.KeyUp:
		a.ri--
	case tcell.KeyDown:
		a.ri++
	case tcell.KeyLeft:
		if shift {
			a.cycle(-1)
		} else {
			a.ci--
		}
	case tcell.KeyRight:
		if shift {
			a.cycle(1)
		} else {
			a.ci++
		}
	case tcell.KeyPgDn, tcell.KeyCtrlD:
		a.ri += 10
	case tcell.KeyPgUp, tcell.KeyCtrlU:
		a.ri -= 10
	case tcell.KeyHome:
		a.ri = 0
	case tcell.KeyEnd:
		a.ri = len(a.rows) - 1
	case tcell.KeyEnter:
		a.editCell()
	case tcell.KeyTab:
		a.fold()
	case tcell.KeyDelete, tcell.KeyBackspace, tcell.KeyBackspace2:
		a.clearCell()
	case tcell.KeyRune:
		a.rune(ev.Rune())
	}
	a.clamp()
}

func (a *app) rune(r rune) {
	switch r {
	case 'q':
		a.quit = true
	case 'j':
		a.ri++
	case 'k':
		a.ri--
	case 'h':
		a.ci--
	case 'l':
		a.ci++
	case 'g':
		a.ri = 0
	case 'G':
		a.ri = len(a.rows) - 1
	case 'H':
		a.cycle(-1)
	case 'L':
		a.cycle(1)
	case 'e', 'i':
		a.editCell()
	case 'd':
		a.clearCell()
	case 'z', ' ':
		a.fold()
	case 'Z':
		a.foldAll()
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		a.toLevel(int(r - '0'))
	case 's':
		a.cycleSort()
	case 'f':
		a.filterHere()
	case 'F':
		a.filterBy = ""
		a.layout()
	case 'V':
		a.valuesMenu()
	case 'c':
		a.editLine()
	case '<':
		a.moveCol(-1)
	case '>':
		a.moveCol(1)
	case '+', '=':
		a.widen(2)
	case '-':
		a.widen(-2)
	case 'p':
		a.cyclePath()
	case 'o':
		o := a.opts()
		a.setLine(SetOption(a.data.Columns, "flat", boolInt(!o.Flat)))
	case 'W':
		a.saveLine()
	case 'R':
		a.spec = ""
		a.load()
		a.say("back to the file's own columns")
	case 'n':
		a.newEntry()
	case 'E':
		a.openEditor()
	case 'v':
		a.preview()
	case 'O', 'b':
		a.filePicker()
	case 'r':
		a.load()
		a.say("read again")
	case '?':
		a.Over = &help{}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- folding, sorting and narrowing --------------------------------------------------

func (a *app) fold() {
	r := a.selected()
	if r == nil || a.opts().Flat {
		return
	}
	if !r.HasChildren {
		// A leaf folds its parent, which is where somebody standing on a
		// leaf and pressing fold meant.
		path := a.ancestors[r.Hash]
		if len(path) == 0 {
			return
		}
		for i := a.ri - 1; i >= 0; i-- {
			if a.rows[i].Level < r.Level {
				a.folded[a.rows[i].Hash] = true
				a.ri = i
				break
			}
		}
	} else {
		a.folded[r.Hash] = !a.folded[r.Hash]
	}
	a.layout()
}

// foldAll folds every heading with something under it, or opens them all when
// anything is folded.
func (a *app) foldAll() {
	if len(a.folded) > 0 {
		a.folded = map[string]bool{}
	} else {
		for _, r := range a.data.Rows {
			if r.HasChildren {
				a.folded[r.Hash] = true
			}
		}
	}
	a.layout()
}

// toLevel shows the outline down to a level: everything deeper is folded.
func (a *app) toLevel(n int) {
	a.folded = map[string]bool{}
	for _, r := range a.data.Rows {
		if r.HasChildren && r.Level >= n {
			a.folded[r.Hash] = true
		}
	}
	a.layout()
	a.say("showing %d level%s", n, plural(n))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// sortBy cycles the selected column: ascending, descending, the file's order.
func (a *app) cycleSort() {
	s := a.spec1()
	if s == nil {
		return
	}
	switch {
	case a.sortCol() != a.ci:
		a.sortBy, a.sortDesc = s.Property, false
	case !a.sortDesc:
		a.sortDesc = true
	default:
		a.sortBy = ""
	}
	a.layout()
}

func (a *app) filterHere() {
	r, s := a.selected(), a.spec1()
	if r == nil || s == nil || a.ci >= len(r.Cells) {
		return
	}
	v := strings.TrimSpace(r.Cells[a.ci].Own)
	if v == "" {
		a.complain("nothing in this cell to narrow by")
		return
	}
	a.filterBy, a.filterVal = s.Property, v
	a.layout()
}

// valuesMenu lists every value the selected column's property takes in the
// file, commonest first; picking one narrows the rows to it.
func (a *app) valuesMenu() {
	s := a.spec1()
	if s == nil {
		return
	}
	var pv *common.ColumnPropValues
	for i := range a.values.Props {
		if strings.EqualFold(a.values.Props[i].Name, s.Property) {
			pv = &a.values.Props[i]
		}
	}
	if pv == nil || len(pv.Values) == 0 {
		a.complain("%s has no values in this file", s.Property)
		return
	}
	items := []tuikit.MenuItem{}
	for _, v := range pv.Values {
		items = append(items, tuikit.MenuItem{Label: v.Value, Note: fmt.Sprintf("%d", v.Count),
			On: a.filterBy == s.Property && a.filterVal == v.Value})
	}
	prop := s.Property
	vals := pv.Values
	a.Over = &tuikit.Menu{
		Title:  fmt.Sprintf("%s in %s - narrow to", prop, filepath.Base(a.file)),
		Items:  items,
		Filter: true,
		Pick: func(i int) bool {
			a.filterBy, a.filterVal = prop, vals[i].Value
			a.layout()
			return false
		},
	}
}

// --- the columns line ------------------------------------------------------------------

// setLine applies a line to this view; W writes it into the file.
func (a *app) setLine(line string) {
	a.spec = line
	a.load()
}

func (a *app) moveCol(by int) {
	to := a.ci + by
	if to < 0 || to >= len(a.data.Spec) {
		return
	}
	a.setLine(MoveColumn(a.data.Columns, a.ci, to))
	a.ci = to
}

func (a *app) widen(by int) {
	s := a.spec1()
	if s == nil {
		return
	}
	w := s.Width
	if w <= 0 {
		w = a.shownWidth(a.ci)
	}
	w += by
	if w < 3 {
		w = 3
	}
	a.setLine(SetWidth(a.data.Columns, a.ci, w))
}

func (a *app) cyclePath() {
	n := (a.opts().Path + 1) % 4
	a.setLine(SetOption(a.data.Columns, "path", n))
	if n == 0 {
		a.say("no path before the titles")
	} else {
		a.say("%d ancestor%s before each title", n, plural(n))
	}
}

func (a *app) editLine() {
	props := a.values.Props
	a.Over = &tuikit.Prompt{
		Title: "Columns for " + filepath.Base(a.file),
		Text:  a.data.Columns,
		Hint:  "%25ITEM %TODO %EFFORT(Estimate){:} %OWNER  +path +flat",
		Suggest: func(text string) []string {
			from, s := Complete(text, props)
			out := []string{}
			for _, x := range s {
				out = append(out, text[:from]+x.Value)
			}
			return out
		},
		Done: func(line string) { a.setLine(strings.TrimSpace(line)) },
	}
}

// saveLine writes the line on show as the file's own #+COLUMNS:, so it is
// there next time, and there in Emacs too.
func (a *app) saveLine() {
	if a.post("columns/spec", map[string]string{"File": a.file, "Columns": a.data.Columns}) {
		a.spec = ""
		a.load()
		a.say("wrote #+COLUMNS: into %s", filepath.Base(a.file))
	}
}

// --- writing cells -------------------------------------------------------------------------

func (a *app) post(path string, req interface{}) bool {
	res, err := commands.SendReceivePostErr[interface{}, common.ResultMsg](a.core, path, &req)
	if err != nil {
		a.complain("%s: %v", path, err)
		return false
	}
	if !res.Ok {
		if res.Msg == "" {
			res.Msg = "the server did not take it"
		}
		a.complain("%s", res.Msg)
		return false
	}
	return true
}

// write puts a value into a cell through whichever endpoint the column's kind
// takes, then reads the view back for the rollups.
func (a *app) write(r Row, s Spec, value string) {
	ok := false
	switch s.Kind {
	case "property":
		ok = a.post("property", common.TodoPropertyChange{Hash: r.Hash, Name: s.Property, Value: value})
	case "todo":
		ok = a.post("status/change", common.TodoItemChange{Hash: r.Hash, Value: value})
	case "item":
		ok = a.post("headline/change", common.TodoItemChange{Hash: r.Hash, Value: value})
	case "priority":
		ok = a.post("heading/parts", map[string]interface{}{"Hash": r.Hash, "Priority": strings.TrimSpace(value)})
	case "tags":
		tags := strings.FieldsFunc(value, func(c rune) bool { return c == ' ' || c == ':' || c == ',' })
		ok = a.post("heading/parts", map[string]interface{}{"Hash": r.Hash, "Tags": tags})
	default:
		a.complain("%s is worked out, not written", s.Property)
		return
	}
	if ok {
		shown := value
		if shown == "" {
			shown = "nothing"
		}
		a.say("%s → %s", s.Property, shown)
	}
	a.load()
}

// editCell opens the editor a column's kind wants, on the heading's own value
// - never the rollup.
func (a *app) editCell() {
	r, s := a.selected(), a.spec1()
	if r == nil || s == nil {
		return
	}
	if !Editable(*s) {
		a.complain("%s is worked out, not written", s.Property)
		return
	}
	own := ""
	if a.ci < len(r.Cells) {
		own = r.Cells[a.ci].Own
	}
	row, spec := *r, *s
	switch s.Kind {
	case "todo":
		a.keywordMenu(row, spec)
	case "priority":
		ps := []string{"A", "B", "C", ""}
		items := []tuikit.MenuItem{}
		for _, p := range ps {
			label := "[#" + p + "]"
			if p == "" {
				label = "none"
			}
			items = append(items, tuikit.MenuItem{Label: label, Color: priorityColor(p), On: strings.EqualFold(row.Priority, p)})
		}
		a.Over = &tuikit.Menu{Title: "Priority for " + row.Headline, Items: items,
			Pick: func(i int) bool { a.write(row, spec, ps[i]); return false }}
	case "item":
		a.Over = &tuikit.Prompt{Title: "Rename", Text: row.Headline,
			Done: func(v string) { a.write(row, spec, strings.TrimSpace(v)) }}
	case "tags":
		used := a.valuesOf("TAGS")
		a.Over = &tuikit.Prompt{Title: "Tags on " + row.Headline, Text: strings.Join(row.Tags, " "),
			Hint: "space between tags; empty takes them all off", AllowEmpty: true,
			Suggest: func(text string) []string { return completeWord(text, used) },
			Done:    func(v string) { a.write(row, spec, v) }}
	default:
		vals := append(append([]string{}, s.Allowed...), a.valuesOf(s.Property)...)
		a.Over = &tuikit.Prompt{Title: s.Property + " for " + row.Headline, Text: own,
			Hint: "empty takes the property off", AllowEmpty: true,
			Suggest: func(text string) []string { return startsWith(vals, text) },
			Done:    func(v string) { a.write(row, spec, strings.TrimSpace(v)) }}
	}
}

func (a *app) clearCell() {
	r, s := a.selected(), a.spec1()
	if r == nil || s == nil {
		return
	}
	switch s.Kind {
	case "property", "priority", "tags":
		a.write(*r, *s, "")
	default:
		a.complain("%s cannot be emptied from here", s.Property)
	}
}

// cycle steps a cell through its choices, as org's S-left / S-right do: a
// property's allowed values, the heading's keywords, the priorities.
func (a *app) cycle(by int) {
	r, s := a.selected(), a.spec1()
	if r == nil || s == nil {
		return
	}
	var choices []string
	cur := ""
	if a.ci < len(r.Cells) {
		cur = strings.TrimSpace(r.Cells[a.ci].Own)
	}
	switch s.Kind {
	case "property":
		choices = append([]string{""}, s.Allowed...)
		if len(s.Allowed) == 0 {
			a.complain("%s has no allowed values to step through - Enter to type one (or give it :%s_ALL:)", s.Property, s.Property)
			return
		}
	case "priority":
		choices, cur = []string{"", "A", "B", "C"}, strings.ToUpper(r.Priority)
	case "todo":
		st := a.statesFor(r.Hash)
		choices, cur = append(append([]string{}, st.Active...), st.Done...), r.Status
	default:
		a.complain("%s does not step - Enter to edit it", s.Property)
		return
	}
	i := 0
	for k, c := range choices {
		if strings.EqualFold(c, cur) {
			i = k
		}
	}
	i = (i + by + len(choices)) % len(choices)
	a.write(*r, *s, choices[i])
}

func (a *app) statesFor(hash string) common.TodoStatesResult {
	st, err := commands.SendReceiveGetErr[common.TodoStatesResult](a.core, "status/"+commands.HashPath(hash), nil)
	if err != nil || len(st.Active)+len(st.Done) == 0 {
		return a.states
	}
	return st
}

// keywordMenu offers the heading's own file's keywords: a file may have its
// own #+TODO line, and offering one it lacks is offering to write something
// org will not read back.
func (a *app) keywordMenu(r Row, s Spec) {
	st := a.statesFor(r.Hash)
	all := append(append([]string{}, st.Active...), st.Done...)
	items := []tuikit.MenuItem{}
	for _, k := range all {
		col := "#f5a524"
		if containsStr(st.Done, k) {
			col = "#30a46c"
		}
		items = append(items, tuikit.MenuItem{Label: k, Color: col, On: k == r.Status})
	}
	a.Over = &tuikit.Menu{Title: "Keyword for " + r.Headline, Items: items,
		Pick: func(i int) bool { a.write(r, s, all[i]); return false }}
}

func (a *app) valuesOf(prop string) []string {
	out := []string{}
	for _, p := range a.values.Props {
		if strings.EqualFold(p.Name, prop) {
			for _, v := range p.Values {
				out = append(out, v.Value)
			}
		}
	}
	return out
}

// startsWith is the values that begin with what has been typed, most used
// first, the typed text itself left out.
func startsWith(vals []string, text string) []string {
	out := []string{}
	t := strings.ToLower(strings.TrimSpace(text))
	for _, v := range vals {
		if strings.HasPrefix(strings.ToLower(v), t) && v != text && !containsStr(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// completeWord completes the last word of a list typed with spaces.
func completeWord(text string, vals []string) []string {
	i := strings.LastIndex(text, " ") + 1
	head, word := text[:i], text[i:]
	have := strings.Fields(head)
	out := []string{}
	for _, v := range vals {
		if strings.HasPrefix(strings.ToLower(v), strings.ToLower(word)) && !containsStr(have, v) && v != word {
			out = append(out, head+v)
		}
	}
	return out
}

// --- a new heading --------------------------------------------------------------------------

// newEntry opens a line under the selected heading; what is typed there
// becomes a heading after it (after its whole subtree, at its level), read
// the way worg's quick add reads it. Enter adds it and opens the next line
// after the new one, so a list is typed straight down; Esc stops.
func (a *app) newEntry() {
	after := a.selected()
	where := "at the end of " + filepath.Base(a.file)
	afterHash := ""
	status := ""
	if after != nil {
		where = "after " + after.Headline
		afterHash = after.Hash
		if after.Status != "" {
			status = "TODO" // a sibling of a todo is a todo, unless one is typed
		}
	}
	a.Over = &tuikit.Prompt{
		Title: "New heading " + where,
		Hint:  "NEXT Title 3d @who #tag ^fri !A KEY=value",
		Done: func(line string) {
			all := append(append([]string{}, a.states.Active...), a.states.Done...)
			q := ParseQuick(line, all, time.Now())
			if q.Headline == "" {
				a.complain("a heading needs a title")
				return
			}
			props := map[string]string{}
			for k, v := range q.Props {
				props[k] = v
			}
			if q.Person != "" {
				props["ASSIGNED"] = q.Person
			}
			headline := q.Headline
			if q.Priority != "" {
				headline = "[#" + q.Priority + "] " + headline
			}
			st := q.Status
			if st == "" {
				st = status
			}
			req := common.GanttAdd{Headline: headline, Status: st, Start: q.Start, Effort: q.Effort, Tags: q.Tags, Props: props}
			if afterHash != "" {
				req.AfterHash = afterHash
			} else {
				req.Filename = a.file
			}
			if !a.post("gantt/add", req) {
				return
			}
			a.load()
			for i, r := range a.rows {
				if strings.TrimPrefix(r.Headline, "[#"+q.Priority+"] ") == q.Headline {
					a.ri = i
				}
			}
			a.say("added %s", q.Headline)
			a.newEntry()
		},
	}
}

// --- looking at a heading ------------------------------------------------------------------

func (a *app) openEditor() {
	r := a.selected()
	if r == nil || a.edit == nil {
		a.complain("no editor - set $EDITOR or editorTemplate")
		return
	}
	if a.termEdit {
		a.Scr.Suspend()
		a.edit(a.file, r.LineNum)
		a.Scr.Resume()
		a.load()
		return
	}
	a.edit(a.file, r.LineNum)
	a.say("opened %s:%d", filepath.Base(a.file), r.LineNum)
}

type body struct {
	Ok   bool
	Msg  string
	Text string
}

func (a *app) preview() {
	r := a.selected()
	if r == nil {
		return
	}
	bd, err := commands.SendReceiveGetErr[body](a.core, "body/"+commands.HashPath(r.Hash), nil)
	if err != nil {
		a.complain("%v", err)
		return
	}
	a.Over = &preview{row: *r, text: bd.Text, path: a.ancestors[r.Hash]}
}

// filePicker offers every org file the server watches.
func (a *app) filePicker() {
	files := commands.SendReceiveGetOr[[]string](a.core, "files", nil)
	if len(files) == 0 {
		a.complain("the server knows no files")
		return
	}
	items := []tuikit.MenuItem{}
	for _, f := range files {
		items = append(items, tuikit.MenuItem{Label: filepath.Base(f), Note: filepath.Dir(f), On: f == a.file})
	}
	a.Over = &tuikit.Menu{
		Title:  "Columns of which file?",
		Items:  items,
		Filter: true,
		Pick: func(i int) bool {
			a.open(files[i])
			return false
		},
	}
}

func (a *app) open(file string) {
	a.file, a.spec, a.filterBy = file, "", ""
	a.folded = map[string]bool{}
	a.sortBy, a.ri, a.ci, a.scroll, a.hscroll = "", 0, 0, 0, 0
	rememberFile(file)
	a.load()
}

// The file last looked at, kept beside the user's other orgs state: a column
// view is about one project, and one that forgets which is set up again
// every time.
func lastFilePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "orgs", "columns-file")
}

func rememberFile(f string) {
	p := lastFilePath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(f), 0o644)
}

func lastFile() string {
	p := lastFilePath()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

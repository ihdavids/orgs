package kanban

// The board in a terminal: worg's kanban, keys instead of a mouse.
//
// Everything a card shows is read afresh from the query on every refresh, as
// in the browser, so a heading edited anywhere else is right the next time
// this one looks. The writes are the browser's writes - the column's value
// (keyword, property or tag), one order number when the board sorts by hand,
// labels, keyword, priority, the clock, checkboxes, refile, copy and archive -
// through the same endpoints, and the board settings are the same per-user
// boards, so a board changed here is changed there.

import (
	"fmt"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/yaml.v3"
)

type body struct {
	Ok    bool
	Msg   string
	Text  string
	Image string
	Audio string
}

type clockState struct {
	Active bool
	Target common.Target
}

type app struct {
	*tuikit.UI
	core *commands.Core

	boards []Board
	bi     int
	stored []StoredQuery
	states common.TodoStatesResult

	cards    []Card
	cols     []Column
	buckets  map[string][]Card
	loose    int // cards no column claimed, on a board without the catch-all
	problem  string
	clockKey string // the clocked heading, as file+headline: its hash moves

	ci, ri    int // the selected column, and card within it
	colScroll int
	rowScroll map[string]int

	search    string
	searching bool

	msg     string
	msgBad  bool
	msgTill time.Time

	mu     sync.Mutex
	bodies map[string]body
	asked  map[string]bool

	edit     func(string, int)
	termEdit bool
	quit     bool
	printing bool // drawn once to paper: no keys, no selection
}

// --- loading ------------------------------------------------------------------

func (a *app) board() *Board {
	if a.bi < 0 || a.bi >= len(a.boards) {
		return nil
	}
	return &a.boards[a.bi]
}

func (a *app) loadBoards() error {
	bs, err := commands.SendReceiveGetErr[[]Board](a.core, "ext/kanban/boards", nil)
	if err != nil {
		return err
	}
	for i := range bs {
		Normalize(&bs[i])
	}
	a.boards = bs
	a.stored = commands.SendReceiveGetOr[[]StoredQuery](a.core, "ext/queries", nil)
	a.states = commands.SendReceiveGetOr[common.TodoStatesResult](a.core, "status", nil)
	return nil
}

// refresh reads the board's cards again and keeps the selection on the same
// heading when it is still there. A hash moves when a heading's keyword or
// title changes, so the heading is found again by file and title.
func (a *app) refresh() {
	b := a.board()
	a.cards, a.problem = nil, ""
	if b == nil {
		a.layout()
		return
	}
	keep := a.selected()
	if p := BoardQueryProblem(b, a.stored); p != "" {
		a.problem = p
	} else {
		cards, err := commands.SendReceiveGetErr[common.Todos](a.core, "search",
			map[string]string{"query": BoardQuery(b, a.stored)})
		if err != nil {
			a.problem = "Could not ask the server: " + err.Error()
		}
		a.cards = cards
	}
	cs := commands.SendReceiveGetOr[clockState](a.core, "clock", nil)
	a.clockKey = ""
	if cs.Active {
		// The server keeps the clock as a file and an outline path
		// ("Launch::Build the API"), not a hash, so the card is found by its
		// file and the last step of the path.
		olp := strings.Split(cs.Target.Id, "::")
		last := strings.TrimSpace(olp[len(olp)-1])
		for _, c := range a.cards {
			if c.Hash == cs.Target.Id || (c.Filename == cs.Target.Filename && c.Headline == last) {
				a.clockKey = c.Filename + "\x00" + c.Headline
			}
		}
	}
	a.layout()
	if keep != nil {
		a.selectCard(keep.Filename, keep.Headline)
	}
	a.fetchBodies()
}

// layout works out the columns on show and which cards are in each.
func (a *app) layout() {
	b := a.board()
	a.cols, a.buckets, a.loose = nil, map[string][]Card{}, 0
	if b == nil {
		return
	}
	cols := b.Columns
	if len(cols) == 0 {
		if b.GroupBy == "status" {
			cols = DefaultStatusColumns(a.states.Active, a.states.Done, StatusesInCards(a.cards))
		} else {
			// A board split by a property or a tag and told nothing about its
			// columns: the values the query turned up, as columns of their own.
			// Alphabetical rather than commonest first: a column that moves
			// every time a card is dropped on its neighbour cannot be learned.
			vals := DiscoverValues(a.cards, b)
			sort.Slice(vals, func(i, j int) bool { return strings.ToLower(vals[i]) < strings.ToLower(vals[j]) })
			for _, v := range vals {
				cols = append(cols, Column{Value: v, Color: AutoSwatch(v).Key})
			}
		}
	}
	if b.ShowUnset {
		cols = append(append([]Column{}, cols...), UnsetColumn(b))
	}
	terms := SearchTerms(a.search)
	for i := range a.cards {
		c := &a.cards[i]
		col := ColumnOf(c, b, cols)
		if col == Unset && !b.ShowUnset {
			a.loose++
			continue
		}
		if CardMatches(c, terms) {
			a.buckets[col] = append(a.buckets[col], *c)
		}
	}
	for k, v := range a.buckets {
		a.buckets[k] = SortCards(v, b)
	}
	// The list puts hidden sections away; the board keeps every column.
	if b.Layout == "list" {
		shown := []Column{}
		for _, c := range cols {
			if !contains(b.Hidden, c.Value) {
				shown = append(shown, c)
			}
		}
		cols = shown
	}
	a.cols = cols
	a.clamp()
}

func (a *app) clamp() {
	if a.ci >= len(a.cols) {
		a.ci = len(a.cols) - 1
	}
	if a.ci < 0 {
		a.ci = 0
	}
	n := len(a.colCards(a.ci))
	if a.ri >= n {
		a.ri = n - 1
	}
	if a.ri < 0 {
		a.ri = 0
	}
}

func (a *app) colCards(i int) []Card {
	if i < 0 || i >= len(a.cols) {
		return nil
	}
	return a.buckets[a.cols[i].Value]
}

func (a *app) selected() *Card {
	cs := a.colCards(a.ci)
	if a.ri < 0 || a.ri >= len(cs) {
		return nil
	}
	c := cs[a.ri]
	return &c
}

func (a *app) selectCard(file, headline string) {
	for ci := range a.cols {
		for ri, c := range a.colCards(ci) {
			if c.Filename == file && c.Headline == headline {
				a.ci, a.ri = ci, ri
				return
			}
		}
	}
}

func (a *app) folded(value string) bool {
	b := a.board()
	return b != nil && contains(b.Folded, value)
}

// fetchBodies reads what is written under each card, a few at a time, for the
// checklist count on its front. Kept for the life of the board: a hash changes
// when its heading does, so a stale body can only be one for a hash nobody
// asks about any more.
func (a *app) fetchBodies() {
	want := []string{}
	a.mu.Lock()
	for _, c := range a.cards {
		if !a.asked[c.Hash] {
			a.asked[c.Hash] = true
			want = append(want, c.Hash)
		}
	}
	a.mu.Unlock()
	if len(want) == 0 {
		return
	}
	work := func() {
		sem := make(chan struct{}, 6)
		var wg sync.WaitGroup
		for _, h := range want {
			wg.Add(1)
			sem <- struct{}{}
			go func(h string) {
				defer wg.Done()
				defer func() { <-sem }()
				bd, err := commands.SendReceiveGetErr[body](a.core, "body/"+commands.HashPath(h), nil)
				if err != nil {
					return
				}
				a.mu.Lock()
				a.bodies[h] = bd
				a.mu.Unlock()
			}(h)
		}
		wg.Wait()
		a.Scr.PostEvent(tcell.NewEventInterrupt(nil))
	}
	// On paper there is no second draw for them to arrive in time for.
	if a.printing {
		work()
		return
	}
	go work()
}

func (a *app) bodyOf(hash string) (body, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.bodies[hash]
	return b, ok
}

// rereadBody is for after a tick: the count on the card has to move with it.
func (a *app) rereadBody(hash string) {
	bd, err := commands.SendReceiveGetErr[body](a.core, "body/"+commands.HashPath(hash), nil)
	if err == nil {
		a.mu.Lock()
		a.bodies[hash] = bd
		a.mu.Unlock()
	}
}

func (a *app) say(format string, args ...interface{}) {
	a.msg, a.msgBad, a.msgTill = fmt.Sprintf(format, args...), false, time.Now().Add(6*time.Second)
}

func (a *app) complain(format string, args ...interface{}) {
	a.msg, a.msgBad, a.msgTill = fmt.Sprintf(format, args...), true, time.Now().Add(10*time.Second)
}

// --- the loop -------------------------------------------------------------------

func (a *app) run() {
	a.refresh()
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
	if a.searching {
		a.searchKey(ev)
		return
	}
	b := a.board()
	list := b != nil && b.Layout == "list"
	switch ev.Key() {
	case tcell.KeyCtrlC:
		a.quit = true
		return
	case tcell.KeyEscape:
		if a.search != "" {
			a.search = ""
			a.layout()
			return
		}
		a.quit = true
		return
	case tcell.KeyLeft:
		if ev.Modifiers()&tcell.ModShift != 0 {
			a.moveBy(-1)
		} else {
			a.column(-1)
		}
		return
	case tcell.KeyRight:
		if ev.Modifiers()&tcell.ModShift != 0 {
			a.moveBy(1)
		} else {
			a.column(1)
		}
		return
	case tcell.KeyUp:
		if ev.Modifiers()&tcell.ModShift != 0 {
			a.reorder(-1)
		} else {
			a.row(-1)
		}
		return
	case tcell.KeyDown:
		if ev.Modifiers()&tcell.ModShift != 0 {
			a.reorder(1)
		} else {
			a.row(1)
		}
		return
	case tcell.KeyHome:
		a.ri = 0
		return
	case tcell.KeyEnd:
		a.ri = len(a.colCards(a.ci)) - 1
		a.clamp()
		return
	case tcell.KeyPgDn, tcell.KeyCtrlD:
		a.row(5)
		return
	case tcell.KeyPgUp, tcell.KeyCtrlU:
		a.row(-5)
		return
	case tcell.KeyTab:
		a.switchBoard(1)
		return
	case tcell.KeyBacktab:
		a.switchBoard(-1)
		return
	case tcell.KeyEnter:
		a.openCard()
		return
	}
	switch ev.Rune() {
	case 'q':
		a.quit = true
	case 'h':
		a.column(-1)
	case 'l':
		a.column(1)
	case 'j':
		a.row(1)
	case 'k':
		a.row(-1)
	case 'g':
		a.ri = 0
	case 'G':
		a.ri = len(a.colCards(a.ci)) - 1
		a.clamp()
	case 'H', '<':
		a.moveBy(-1)
	case 'L', '>':
		a.moveBy(1)
	case 'J':
		a.reorder(1)
	case 'K':
		a.reorder(-1)
	case ' ':
		a.openCard()
	case 'm':
		a.moveMenu()
	case 't':
		a.keywordMenu()
	case 'p':
		a.priorityMenu()
	case 'a':
		a.labelMenu()
	case 'c':
		a.toggleClock()
	case 'e':
		a.openEditor()
	case 'A':
		a.archive()
	case 'R':
		a.moveTo("refile")
	case 'y':
		a.moveTo("copy")
	case '/':
		a.searching = true
	case 'z':
		a.fold()
	case 'Z':
		a.foldAll()
	case 's':
		a.sortMenu()
	case 'v':
		a.toggleLayout()
	case 'x':
		if list {
			a.hide()
		}
	case 'X':
		a.unhideMenu()
	case 'b':
		a.boardMenu()
	case 'N':
		a.newBoard()
	case 'S':
		a.settings()
	case 'D':
		a.deleteBoard()
	case 'C':
		a.adoptColumns()
	case 'r':
		a.refresh()
		a.say("read again")
	case '?':
		a.Over = &help{a: a}
	}
}

func (a *app) searchKey(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEscape:
		a.search, a.searching = "", false
	case tcell.KeyEnter:
		a.searching = false
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if r := []rune(a.search); len(r) > 0 {
			a.search = string(r[:len(r)-1])
		}
	case tcell.KeyCtrlU:
		a.search = ""
	case tcell.KeyRune:
		a.search += string(ev.Rune())
	}
	a.layout()
}

func (a *app) column(by int) {
	a.ci += by
	if a.ci < 0 {
		a.ci = 0
	}
	if a.ci >= len(a.cols) {
		a.ci = len(a.cols) - 1
	}
	a.clamp()
}

// row moves down the column - and in the list, where the sections are one
// list, on into the next section rather than stopping at its end.
func (a *app) row(by int) {
	b := a.board()
	if b != nil && b.Layout == "list" {
		for by != 0 {
			step := 1
			if by < 0 {
				step = -1
			}
			n := len(a.colCards(a.ci))
			if a.folded(a.cols[a.ci].Value) {
				n = 0
			}
			nr := a.ri + step
			switch {
			case nr >= 0 && nr < n:
				a.ri = nr
			case step > 0 && a.ci < len(a.cols)-1:
				a.ci++
				a.ri = 0
			case step < 0 && a.ci > 0:
				a.ci--
				a.ri = len(a.colCards(a.ci)) - 1
				if a.folded(a.cols[a.ci].Value) {
					a.ri = 0
				}
			}
			by -= step
		}
		a.clamp()
		return
	}
	a.ri += by
	a.clamp()
}

func (a *app) switchBoard(by int) {
	if len(a.boards) == 0 {
		return
	}
	a.bi = (a.bi + by + len(a.boards)) % len(a.boards)
	a.ci, a.ri, a.colScroll, a.search = 0, 0, 0, ""
	a.rowScroll = map[string]int{}
	a.refresh()
}

// --- writing ----------------------------------------------------------------------

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

// writeColumn puts a card's value for the column it is going to: the keyword,
// the property, or a swap of one tag for another.
func (a *app) writeColumn(c *Card, from, to string) bool {
	b := a.board()
	switch b.GroupBy {
	case "property":
		return a.post("property", common.TodoPropertyChange{Hash: c.Hash, Name: b.GroupKey, Value: to})
	case "tag":
		if from != Unset && !a.post("tags", common.TodoItemChange{Hash: c.Hash, Value: from}) {
			return false
		}
		if to != Unset {
			return a.post("tags", common.TodoItemChange{Hash: c.Hash, Value: to})
		}
		return true
	}
	if to == Unset {
		a.complain("A heading cannot have its todo keyword taken away from here.")
		return false
	}
	to = a.spellingFor(c, to)
	if !a.post("status/change", common.TodoItemChange{Hash: c.Hash, Value: to}) {
		a.complain("%s is not a todo keyword %s allows.", to, baseName(c.Filename))
		return false
	}
	return true
}

// spellingFor is the spelling of a keyword column the heading's own file
// takes. A column read off the server's states is called INPROGRESS while a
// file whose #+TODO says IN-PROGRESS refuses that word - but the column knows
// the other spelling as an alias, so the move writes the one that will stick.
func (a *app) spellingFor(c *Card, value string) string {
	var col *Column
	for i := range a.cols {
		if a.cols[i].Value == value {
			col = &a.cols[i]
		}
	}
	if col == nil || len(col.Aliases) == 0 {
		return value
	}
	st, err := commands.SendReceiveGetErr[common.TodoStatesResult](a.core, "status/"+commands.HashPath(c.Hash), nil)
	if err != nil {
		return value
	}
	allowed := append(append([]string{}, st.Active...), st.Done...)
	for _, v := range ColumnValues(*col) {
		if contains(allowed, v) {
			return v
		}
	}
	return value
}

func (a *app) writeOrder(ws []OrderWrite) {
	b := a.board()
	for _, w := range ws {
		if !a.post("property", common.TodoPropertyChange{Hash: w.Hash, Name: OrderKey(b), Value: FormatOrder(w.Value)}) {
			return
		}
	}
}

// moveCard is a drag: the column's value, then the order when the board sorts
// by hand, then a refresh - and a word when the card has left the board
// because the move made it stop matching the query.
func (a *app) moveCard(c Card, to string, at int) {
	b := a.board()
	from := ColumnOf(&c, b, a.cols)
	if from != to && !a.writeColumn(&c, from, to) {
		a.refresh()
		return
	}
	if b.Sort == "manual" {
		list := []Card{}
		for _, x := range a.buckets[to] {
			if x.Hash != c.Hash {
				list = append(list, x)
			}
		}
		a.writeOrder(PlanOrder(list, at, &c, b))
	}
	a.refresh()
	a.selectCard(c.Filename, c.Headline)
	if from != to {
		still := false
		for _, x := range a.cards {
			if x.Filename == c.Filename && x.Headline == c.Headline {
				still = true
			}
		}
		name := to
		if to == Unset {
			name = "unset"
		}
		if !still {
			a.say("%q is now %s and no longer matches this board's query, so it has left the board. It is still in %s.",
				c.Headline, name, baseName(c.Filename))
		} else {
			a.say("%s → %s", c.Headline, name)
		}
	}
}

func (a *app) moveBy(by int) {
	c := a.selected()
	if c == nil {
		return
	}
	to := a.ci + by
	for to >= 0 && to < len(a.cols) && a.board().Layout != "list" && a.folded(a.cols[to].Value) {
		to += by
	}
	if to < 0 || to >= len(a.cols) {
		return
	}
	a.moveCard(*c, a.cols[to].Value, len(a.colCards(to)))
}

func (a *app) reorder(by int) {
	c := a.selected()
	if c == nil {
		return
	}
	b := a.board()
	if b.Sort != "manual" {
		a.complain("This board sorts by %s - press s and pick manual to order cards by hand.", b.Sort)
		return
	}
	cs := a.colCards(a.ci)
	at := a.ri + by
	if at < 0 || at >= len(cs) {
		return
	}
	a.moveCard(*c, a.cols[a.ci].Value, at)
}

func (a *app) setStatus(c *Card, kw string) {
	b := a.board()
	if b.GroupBy == "status" {
		a.moveCard(*c, kw, len(a.buckets[kw]))
		return
	}
	if a.post("status/change", common.TodoItemChange{Hash: c.Hash, Value: kw}) {
		a.say("%s is %s", c.Headline, kw)
	}
	a.refresh()
}

func (a *app) toggleLabel(c *Card, v string) {
	b := a.board()
	on := contains(LabelsOf(c, b), v)
	if b.LabelSource == "tags" {
		a.post("tags", common.TodoItemChange{Hash: c.Hash, Value: v})
	} else {
		key := strings.ToUpper(b.LabelKey)
		next := []string{}
		for _, x := range SplitLabels(prop(c, key)) {
			if x != v {
				next = append(next, x)
			}
		}
		if !on {
			next = append(next, v)
		}
		a.post("property", common.TodoPropertyChange{Hash: c.Hash, Name: key, Value: strings.Join(next, " ")})
	}
	a.refresh()
}

func (a *app) toggleClock() {
	c := a.selected()
	if c == nil {
		return
	}
	if a.clockKey == c.Filename+"\x00"+c.Headline {
		if a.post("clockout", struct{}{}) {
			a.say("clocked out of %s", c.Headline)
		}
	} else if a.post("clockin", common.Target{Id: c.Hash, Type: "hash"}) {
		a.say("clocked in to %s", c.Headline)
	}
	a.refresh()
}

func (a *app) tick(c *Card, index int, text string, done bool) {
	if a.post("checklist", map[string]interface{}{"Hash": c.Hash, "Index": index, "Text": text, "Done": done}) {
		a.rereadBody(c.Hash)
	}
}

func (a *app) setPriority(c *Card, p string) {
	if a.post("heading/parts", map[string]interface{}{"Hash": c.Hash, "Priority": p}) {
		if p == "" {
			a.say("%s has no priority", c.Headline)
		} else {
			a.say("%s is [#%s]", c.Headline, p)
		}
	}
	a.refresh()
}

func (a *app) archive() {
	c := a.selected()
	if c == nil {
		return
	}
	cc := *c
	a.Over = &tuikit.Confirm{
		Text: fmt.Sprintf("Archive %q and everything under it?", c.Headline),
		Yes: func() {
			a.doMove("archive", cc, common.Target{})
		},
	}
}

func (a *app) doMove(op string, c Card, to common.Target) {
	req := common.MoveRequest{Op: op, From: []common.Target{{Type: "hash", Id: c.Hash}}, To: to}
	res, err := commands.SendReceivePostErr[common.MoveRequest, common.MoveResponse](a.core, "move", &req)
	switch {
	case err != nil:
		a.complain("%s: %v", op, err)
	case !res.Ok:
		msg := res.Msg
		for _, r := range res.Results {
			if !r.Ok && r.Msg != "" {
				msg = r.Msg
			}
		}
		a.complain("%s did not go through: %s", op, msg)
	default:
		verb := map[string]string{"archive": "archived", "refile": "refiled", "copy": "copied"}[op]
		a.say("%s %s", verb, c.Headline)
	}
	a.refresh()
}

// moveTo picks where a refile or a copy goes, out of every heading the server
// offers, filtered as you type.
func (a *app) moveTo(op string) {
	c := a.selected()
	if c == nil {
		return
	}
	list, err := commands.SendReceiveGetErr[common.RefileTargetList](a.core, "refile/targets", map[string]string{"depth": "4"})
	if err != nil || len(list.Targets) == 0 {
		a.complain("no refile targets: %v %s", err, list.Msg)
		return
	}
	items := []tuikit.MenuItem{}
	for _, t := range list.Targets {
		label := baseName(t.Filename)
		if len(t.Olp) > 0 {
			label += " › " + strings.Join(t.Olp, " › ")
		}
		if t.Todo != "" {
			label += "  " + t.Todo
		}
		items = append(items, tuikit.MenuItem{Label: label})
	}
	cc := *c
	targets := list.Targets
	verb := map[string]string{"refile": "Refile", "copy": "Copy"}[op]
	a.Over = &tuikit.Menu{
		Title:  fmt.Sprintf("%s %q to…", verb, c.Headline),
		Items:  items,
		Filter: true,
		Pick: func(i int) bool {
			t := targets[i]
			to := common.Target{Type: "hash", Filename: t.Filename, Id: t.Hash}
			if t.Hash == "" {
				to = common.Target{Type: "file", Filename: t.Filename}
			}
			a.doMove(op, cc, to)
			return false
		},
	}
}

// openEditor puts the terminal down for $EDITOR, or hands the file to the
// configured one and carries on.
func (a *app) openEditor() {
	c := a.selected()
	if c == nil || a.edit == nil {
		a.complain("no editor - set $EDITOR or editorTemplate")
		return
	}
	if a.termEdit {
		a.Scr.Suspend()
		a.edit(c.Filename, c.LineNum)
		a.Scr.Resume()
		a.refresh()
		return
	}
	a.edit(c.Filename, c.LineNum)
	a.say("opened %s:%d", baseName(c.Filename), c.LineNum)
}

// --- the board itself ---------------------------------------------------------------

// saveBoard writes the open board back, leaving every other one alone.
func (a *app) saveBoard() {
	b := a.board()
	if b == nil {
		return
	}
	a.post("ext/kanban/board", *b)
}

func (a *app) saveAll() bool {
	return a.post("ext/kanban/boards", a.boards)
}

func (a *app) fold() {
	b := a.board()
	if b == nil || len(a.cols) == 0 {
		return
	}
	v := a.cols[a.ci].Value
	if contains(b.Folded, v) {
		b.Folded = remove(b.Folded, v)
	} else {
		b.Folded = append(b.Folded, v)
	}
	a.saveBoard()
	a.clamp()
}

// foldAll folds every column, or opens every one when they all are.
func (a *app) foldAll() {
	b := a.board()
	if b == nil {
		return
	}
	all := true
	for _, c := range a.cols {
		if !contains(b.Folded, c.Value) {
			all = false
		}
	}
	b.Folded = []string{}
	if !all {
		for _, c := range a.cols {
			b.Folded = append(b.Folded, c.Value)
		}
	}
	a.saveBoard()
}

func (a *app) hide() {
	b := a.board()
	if b == nil || len(a.cols) == 0 {
		return
	}
	v := a.cols[a.ci].Value
	b.Hidden = append(b.Hidden, v)
	a.saveBoard()
	a.layout()
	a.say("put %s away - X brings it back", ColumnTitle(Column{Value: v, Title: a.titleOf(v)}))
}

func (a *app) titleOf(v string) string {
	for _, c := range a.cols {
		if c.Value == v {
			return ColumnTitle(c)
		}
	}
	if v == Unset {
		return UnsetColumn(a.board()).Title
	}
	return v
}

func (a *app) toggleLayout() {
	b := a.board()
	if b == nil {
		return
	}
	if b.Layout == "list" {
		b.Layout = "board"
	} else {
		b.Layout = "list"
	}
	a.saveBoard()
	a.layout()
}

// adoptColumns writes the columns on show into the board, so they can be
// renamed, recoloured and reordered; until then they are worked out afresh.
func (a *app) adoptColumns() {
	b := a.board()
	if b == nil {
		return
	}
	if len(b.Columns) > 0 {
		a.say("this board already has columns of its own - S to change them")
		return
	}
	for _, c := range a.cols {
		if c.Value != Unset {
			b.Columns = append(b.Columns, c)
		}
	}
	a.saveBoard()
	a.layout()
	a.say("the columns are the board's own now - S to rename, recolour or reorder them")
}

func (a *app) deleteBoard() {
	b := a.board()
	if b == nil {
		return
	}
	name := b.Name
	a.Over = &tuikit.Confirm{
		Text: fmt.Sprintf("Delete the board %q? The headings on it are not touched.", name),
		Yes: func() {
			_, err := commands.SendReceiveDelete[common.ResultMsg](a.core, "ext/kanban/board", map[string]string{"name": name})
			if err != nil {
				a.complain("%v", err)
				return
			}
			a.boards = append(a.boards[:a.bi], a.boards[a.bi+1:]...)
			if a.bi >= len(a.boards) {
				a.bi = len(a.boards) - 1
			}
			a.say("deleted %s", name)
			a.refresh()
		},
	}
}

// newBoard asks for a name, then what the cards are, then opens the settings.
func (a *app) newBoard() {
	names := []string{}
	for _, b := range a.boards {
		names = append(names, b.Name)
	}
	a.Over = &tuikit.Prompt{
		Title: "New board - its name",
		Text:  UniqueName("Board", names),
		Done: func(name string) {
			name = UniqueName(strings.TrimSpace(name), names)
			items := []tuikit.MenuItem{{Label: "✎ write a query"}}
			for _, q := range a.stored {
				items = append(items, tuikit.MenuItem{Label: q.Name, Note: q.Query})
			}
			a.Over = &tuikit.Menu{
				Title:  "Where do " + name + "'s cards come from?",
				Items:  items,
				Filter: true,
				Pick: func(i int) bool {
					nb := NewBoard(name)
					if i > 0 {
						nb.StoredQuery = a.stored[i-1].Name
						a.addBoard(nb)
						return false
					}
					a.Over = &tuikit.Prompt{
						Title: "The query (" + name + ")",
						Text:  "IsTodo()",
						Done: func(q string) {
							nb.Query = q
							a.addBoard(nb)
						},
					}
					return true
				},
			}
		},
	}
}

func (a *app) addBoard(nb Board) {
	a.boards = append(a.boards, nb)
	a.bi = len(a.boards) - 1
	a.saveBoard()
	a.ci, a.ri = 0, 0
	a.refresh()
	a.say("made %s - S for its settings", nb.Name)
}

// settings opens the board in the editor as yaml - every setting worg's dialog
// has, in the words the boards file uses - and saves it when it comes back.
func (a *app) settings() {
	b := a.board()
	if b == nil {
		return
	}
	text, err := yaml.Marshal(b)
	if err != nil {
		a.complain("%v", err)
		return
	}
	edited, err := a.EditText(settingsHelp+string(text), "orgs-board-*.yaml")
	if err != nil {
		a.complain("%v", err)
		return
	}
	raw := []byte(edited)
	var nb Board
	if err := yaml.Unmarshal(raw, &nb); err != nil {
		a.complain("that is not a board any more (%v) - nothing was saved", err)
		return
	}
	Normalize(&nb)
	if strings.TrimSpace(nb.Name) == "" {
		nb.Name = b.Name
	}
	old := b.Name
	a.boards[a.bi] = nb
	if nb.Name != old {
		// A rename changes the list rather than one entry.
		if a.saveAll() {
			a.say("saved %s (was %s)", nb.Name, old)
		}
	} else if a.post("ext/kanban/board", nb) {
		a.say("saved %s", nb.Name)
	}
	a.refresh()
}

const settingsHelp = `# The board, as worg keeps it. Save and quit to apply; nothing changes until then.
#
# storedQuery / query   the cards: a saved query's name (wins), or an expression
# groupBy / groupKey    status | property (groupKey = the property) | tag
# columns               value, title, color (slate red orange amber lime green teal
#                       cyan blue indigo violet pink), limit (WIP), aliases
#                       - none at all: read off the keywords, or the values in use
# showUnset/unsetTitle  a column for the cards no column claims
# colorBy / colorKey    none | status | priority | tag | property; colors maps value -> colour
# headerKey/headerColors  the bar across a card: a property (default COLOUR/COLOR)
# labelSource           none | tags | property (labelKey); labelColors, labelOrder
# badges                properties shown as chips on a card
# backShows             body | properties | both
# sort / orderKey       manual | priority | deadline | scheduled | headline | file
# layout / listFields   board | list; priority deadline scheduled tags file prop:NAME
#
`

func remove(list []string, v string) []string {
	out := []string{}
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// --- menus that ask the server first ------------------------------------------------

func (a *app) keywordMenu() {
	c := a.selected()
	if c == nil {
		return
	}
	// The heading's own file may have its own #+TODO line, so ask about it.
	st, err := commands.SendReceiveGetErr[common.TodoStatesResult](a.core, "status/"+commands.HashPath(c.Hash), nil)
	if err != nil || len(st.Active)+len(st.Done) == 0 {
		st = a.states
	}
	all := append(append([]string{}, st.Active...), st.Done...)
	items := []tuikit.MenuItem{}
	for _, s := range all {
		col := SwatchOf(activeHues[0]).Hex
		if contains(st.Done, s) {
			col = SwatchOf("green").Hex
		}
		items = append(items, tuikit.MenuItem{Label: s, Color: col, On: s == c.Status})
	}
	cc := *c
	a.Over = &tuikit.Menu{
		Title: "Keyword for " + c.Headline,
		Items: items,
		Pick: func(i int) bool {
			a.setStatus(&cc, all[i])
			return false
		},
	}
}

func (a *app) priorityMenu() {
	c := a.selected()
	if c == nil {
		return
	}
	ps := []string{"A", "B", "C", ""}
	items := []tuikit.MenuItem{}
	for _, p := range ps {
		label, col := "[#"+p+"]", PriorityColor[p]
		if p == "" {
			label, col = "none", ""
		}
		items = append(items, tuikit.MenuItem{Label: label, Color: col, On: strings.EqualFold(c.Priority, p)})
	}
	cc := *c
	a.Over = &tuikit.Menu{
		Title: "Priority for " + c.Headline,
		Items: items,
		Pick: func(i int) bool {
			a.setPriority(&cc, ps[i])
			return false
		},
	}
}

func (a *app) labelMenu() {
	c := a.selected()
	b := a.board()
	if c == nil || b == nil {
		return
	}
	if b.LabelSource == "none" {
		a.complain("This board has no labels - S, then labelSource: tags (or property)")
		return
	}
	choices := LabelsInCards(a.cards, b)
	cc := *c
	build := func() []tuikit.MenuItem {
		have := LabelsOf(&cc, b)
		items := []tuikit.MenuItem{}
		for _, v := range choices {
			items = append(items, tuikit.MenuItem{Label: v, Color: LabelColorOf(v, b), On: contains(have, v), Check: true})
		}
		return items
	}
	m := &tuikit.Menu{Title: "Labels on " + c.Headline, Items: build(), Filter: true}
	m.Pick = func(i int) bool {
		a.toggleLabel(&cc, choices[i])
		for _, x := range a.cards {
			if x.Filename == cc.Filename && x.Headline == cc.Headline {
				cc = x
			}
		}
		m.Items = build()
		return true
	}
	a.Over = m
}

func (a *app) moveMenu() {
	c := a.selected()
	if c == nil {
		return
	}
	type dest struct{ col, write string }
	dests := []dest{}
	items := []tuikit.MenuItem{}
	for _, col := range a.cols {
		for i, v := range ColumnValues(col) {
			label := ColumnTitle(col)
			if col.Value == Unset {
				label = col.Title
			}
			if i > 0 {
				label += "  (as " + v + ")"
			}
			dests = append(dests, dest{col.Value, v})
			items = append(items, tuikit.MenuItem{Label: label, Color: SwatchOf(col.Color).Hex, On: i == 0 && col.Value == a.cols[a.ci].Value})
		}
	}
	cc := *c
	a.Over = &tuikit.Menu{
		Title:  "Move " + c.Headline + " to…",
		Items:  items,
		Filter: true,
		Pick: func(i int) bool {
			d := dests[i]
			if d.write != d.col {
				// An alias is the other spelling, written deliberately.
				b := a.board()
				from := ColumnOf(&cc, b, a.cols)
				if a.writeColumn(&cc, from, d.write) {
					a.say("%s → %s", cc.Headline, d.write)
				}
				a.refresh()
				return false
			}
			a.moveCard(cc, d.col, len(a.buckets[d.col]))
			return false
		},
	}
}

func (a *app) sortMenu() {
	b := a.board()
	if b == nil {
		return
	}
	items := []tuikit.MenuItem{}
	for _, s := range Sorts {
		items = append(items, tuikit.MenuItem{Label: s, On: s == b.Sort})
	}
	a.Over = &tuikit.Menu{
		Title: "Order the cards by",
		Items: items,
		Pick: func(i int) bool {
			b.Sort = Sorts[i]
			a.saveBoard()
			a.layout()
			return false
		},
	}
}

func (a *app) unhideMenu() {
	b := a.board()
	if b == nil || len(b.Hidden) == 0 {
		a.say("nothing is put away")
		return
	}
	items := []tuikit.MenuItem{}
	hidden := append([]string{}, b.Hidden...)
	for _, v := range hidden {
		t := v
		if v == Unset {
			t = UnsetColumn(b).Title
		}
		items = append(items, tuikit.MenuItem{Label: t, Note: fmt.Sprintf("%d", len(a.buckets[v]))})
	}
	a.Over = &tuikit.Menu{
		Title: "Bring back",
		Items: items,
		Pick: func(i int) bool {
			b.Hidden = remove(b.Hidden, hidden[i])
			a.saveBoard()
			a.layout()
			return false
		},
	}
}

func (a *app) boardMenu() {
	items := []tuikit.MenuItem{}
	for i, b := range a.boards {
		q := b.Query
		if b.StoredQuery != "" {
			q = "saved: " + b.StoredQuery
		}
		items = append(items, tuikit.MenuItem{Label: b.Name, Note: q, On: i == a.bi})
	}
	a.Over = &tuikit.Menu{
		Title:  "Boards",
		Items:  items,
		Filter: true,
		Pick: func(i int) bool {
			a.switchBoard(i - a.bi)
			return false
		},
	}
}

// openCard turns the card over.
func (a *app) openCard() {
	c := a.selected()
	if c == nil {
		return
	}
	a.rereadBody(c.Hash)
	a.Over = &detail{a: a, card: *c, sel: -1}
}

// sortedKeys is a map's keys in order, for drawing properties the same way
// round every time.
func sortedKeys(m map[string]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

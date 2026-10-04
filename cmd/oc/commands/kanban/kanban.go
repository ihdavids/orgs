package kanban

// orgs kanban - worg's kanban boards, in the terminal.
//
//	orgs kanban                 open the first board (tab between them)
//	orgs kanban sprint          open the board called sprint (a prefix will do)
//	orgs kanban ls              the boards there are
//	orgs kanban sprint -print   draw it once and exit (also what a pipe gets)
//	orgs kanban sprint -json    the columns and their cards, for a program
//	orgs kanban new sprint -q 'IsTodo() && HasTags("sprint")'
//	orgs kanban new sprint -saved mine
//	orgs kanban rm sprint
//
// The boards are the per-user ones worg's Kanban tab keeps (/ext/kanban/boards),
// so a board made, folded, re-sorted or reconfigured here is that board in the
// browser too, and the other way round.

import (
	"flag"
	"fmt"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"os"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Kanban struct {
	Print  bool
	Width  int
	Theme  string
	Query  string
	Saved  string
	Group  string
	Layout string
}

func (self *Kanban) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Kanban) StartPlugin(m *common.PluginManager)       {}

func (self *Kanban) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Print, "print", false, "draw the board once and exit")
	fset.IntVar(&self.Width, "width", 0, "columns to draw -print in (default: the terminal's)")
	fset.StringVar(&self.Theme, "theme", "", "dark or light (default: guessed from COLORFGBG, else dark)")
	fset.StringVar(&self.Query, "q", "", "new: the board's query")
	fset.StringVar(&self.Saved, "saved", "", "new: a saved query's name for the board's cards")
	fset.StringVar(&self.Group, "group", "", "new: status, tag or property:NAME")
	fset.StringVar(&self.Layout, "layout", "", "board or list (for this look only, -print)")
}

func (self *Kanban) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("kanban").Flags)
	a := &app{UI: &tuikit.UI{Th: tuikit.ThemeFor(self.Theme)}, core: core,
		rowScroll: map[string]int{}, bodies: map[string]body{}, asked: map[string]bool{}}
	if err := a.loadBoards(); err != nil {
		commands.Fail("kanban: %v", err)
	}

	sub := ""
	if len(words) > 0 {
		sub = words[0]
	}
	switch sub {
	case "ls", "list":
		self.list(a)
		return
	case "new", "add":
		self.create(a, words[1:])
		return
	case "rm", "delete":
		self.remove(a, words[1:])
		return
	}

	if sub != "" {
		name := strings.Join(words, " ")
		a.bi = -1
		for i, b := range a.boards {
			if strings.EqualFold(b.Name, name) {
				a.bi = i
			}
		}
		if a.bi < 0 {
			for i, b := range a.boards {
				if strings.HasPrefix(strings.ToLower(b.Name), strings.ToLower(name)) {
					a.bi = i
					break
				}
			}
		}
		if a.bi < 0 {
			commands.Fail("no board called %s - there is %s", name, strings.Join(boardNames(a), ", "))
		}
	}
	if self.Layout != "" && a.board() != nil {
		a.board().Layout = self.Layout
	}

	if commands.Machine() {
		self.machine(a)
		return
	}
	if self.Print || !commands.Interactive() {
		self.print(a)
		return
	}

	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		commands.Fail("kanban: %v", err)
	}
	a.Scr = scr
	a.edit, a.termEdit = tuikit.Editor(core)
	defer scr.Fini()
	a.run()
}

func boardNames(a *app) []string {
	out := []string{}
	for _, b := range a.boards {
		out = append(out, b.Name)
	}
	return out
}

func (self *Kanban) list(a *app) {
	if len(a.boards) == 0 {
		commands.Fail("no boards yet - orgs kanban new <name> -q '<query>', or make one in worg")
	}
	commands.Render(a.boards, func() {
		w := 0
		for _, b := range a.boards {
			if len(b.Name) > w {
				w = len(b.Name)
			}
		}
		for _, b := range a.boards {
			q := b.Query
			if b.StoredQuery != "" {
				q = "saved: " + b.StoredQuery
			}
			by := b.GroupBy
			if by == "property" {
				by = b.GroupKey
			}
			fmt.Printf("%s%-*s%s  %sby %-10s%s %s%s%s\n", commands.C(commands.AnsiBold), w, b.Name,
				commands.C(commands.AnsiReset), commands.C(commands.AnsiCyan), by, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), q, commands.C(commands.AnsiReset))
		}
	})
}

func (self *Kanban) create(a *app, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs kanban new <name> -q '<query>' | -saved <query name>")
	}
	nb := NewBoard(UniqueName(strings.Join(words, " "), boardNames(a)))
	nb.Query, nb.StoredQuery = self.Query, self.Saved
	switch {
	case self.Group == "tag":
		nb.GroupBy = "tag"
	case strings.HasPrefix(self.Group, "property:"):
		nb.GroupBy, nb.GroupKey = "property", strings.ToUpper(strings.TrimPrefix(self.Group, "property:"))
	}
	if p := BoardQueryProblem(&nb, a.stored); p != "" {
		commands.Fail("%s", p)
	}
	res, err := commands.SendReceivePostErr[Board, common.ResultMsg](a.core, "ext/kanban/board", &nb)
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || !res.Ok {
		commands.Fail("could not save %s: %v %s", nb.Name, err, res.Msg)
	}
	fmt.Printf("%s✓%s made %s%s%s - orgs kanban %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset),
		commands.C(commands.AnsiBold), nb.Name, commands.C(commands.AnsiReset), commands.Shq(nb.Name))
}

func (self *Kanban) remove(a *app, words []string) {
	if len(words) == 0 {
		commands.Fail("orgs kanban rm <name>")
	}
	name := strings.Join(words, " ")
	if !contains(boardNames(a), name) {
		commands.Fail("no board called %s", name)
	}
	res, err := commands.SendReceiveDelete[common.ResultMsg](a.core, "ext/kanban/board", map[string]string{"name": name})
	if err == commands.ErrDryRun {
		return
	}
	if err != nil || !res.Ok {
		commands.Fail("could not remove %s: %v %s", name, err, res.Msg)
	}
	fmt.Printf("%s✓%s removed %s\n", commands.C(commands.AnsiGreen), commands.C(commands.AnsiReset), name)
}

// machine is the board for a program: each column, in order, with its cards.
func (self *Kanban) machine(a *app) {
	a.refresh()
	type col struct {
		Value, Title string
		Limit        int
		Folded       bool
		Cards        []Card
	}
	out := []col{}
	for _, c := range a.cols {
		cards := a.buckets[c.Value]
		if cards == nil {
			cards = []Card{}
		}
		out = append(out, col{Value: c.Value, Title: a.titleOf(c.Value), Limit: c.Limit, Folded: a.folded(c.Value), Cards: cards})
	}
	if a.problem != "" {
		commands.Fail("%s", a.problem)
	}
	commands.Render(out, nil)
}

// print draws the board once, the same drawing the live board makes, through
// a screen that is not a terminal - and writes the cells out as text.
func (self *Kanban) print(a *app) {
	w := self.Width
	if w <= 0 {
		if tw, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 40 {
			w = tw
		} else {
			w = 120
		}
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		commands.Fail("%v", err)
	}
	a.Scr = sim
	a.printing = true
	a.refresh()
	// Tall enough for the longest column; the blank rows are trimmed below.
	h := 8
	for i := range a.cols {
		n := 6
		for k, c := range a.colCards(i) {
			n += len(a.cardRows(&c, 40, k == 0)) + 1
		}
		n += len(a.colCards(i)) // the list layout's rows
		if n > h {
			h = n
		}
	}
	h += 4
	sim.SetSize(w, h)
	a.printing = true
	a.draw()
	fmt.Print(tuikit.Dump(sim, w, h, commands.Colour()))
}

func init() {
	commands.AddCmd("kanban", "worg's kanban boards in the terminal - the same boards, editable",
		func() commands.Cmd { return &Kanban{} })
}

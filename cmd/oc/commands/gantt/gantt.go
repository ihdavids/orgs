package gantt

// orgs gantt - a saved query as a gantt chart, in the terminal.
//
//	orgs gantt                      pick a saved query and draw it
//	orgs gantt launch               draw the saved query called launch
//	orgs gantt 'IsProject()'        draw a query written out
//	orgs gantt launch -color resource -lane OWNER -crit
//	orgs gantt launch -from today -to +6w
//
// The chart is worg's: the same /gantt/tasks answer (ORDERED and AFTER chains,
// EFFORT as a duration, GANTT_ORDER, GANTT_COLOR, PERCENTDONE), laid out by a
// port of worg's scheduler and coloured by worg's schemes and palette, so a
// plan looks the same here as in the browser. The queries are the ones
// `orgs q` and worg's search tab keep.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Gantt struct {
	Color    string
	Lane     string
	From     string
	To       string
	Width    int
	Weekends bool
	Crit     bool
	Theme    string
	Pane     bool
}

type storedQuery struct {
	Name  string `json:"name"`
	Query string `json:"query"`
}

func (self *Gantt) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Gantt) StartPlugin(m *common.PluginManager)       {}

func (self *Gantt) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Color, "color", "state",
		"what a bar's colour means: state, status, resource, section, progress, or a property name")
	fset.StringVar(&self.Lane, "lane", "", "split the lanes by this property (OWNER, RELEASE...) rather than SECTION")
	fset.StringVar(&self.From, "from", "", "first day drawn: a date, today, mon, -2w...")
	fset.StringVar(&self.To, "to", "", "last day drawn: a date, +6w, eom...")
	fset.IntVar(&self.Width, "width", 0, "columns to draw in (default: the terminal's)")
	fset.BoolVar(&self.Weekends, "weekends", false, "count weekends as working days")
	fset.BoolVar(&self.Crit, "crit", false, "mark the critical path")
	fset.StringVar(&self.Theme, "theme", "", "dark or light (default: guessed from COLORFGBG, else dark)")
	fset.BoolVar(&self.Pane, "pane", false, "draw for a picker's preview pane")
}

func (self *Gantt) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("gantt").Flags)
	if self.Pane {
		commands.PickerOutput()
	}

	title, query := "", ""
	switch {
	case len(words) == 0:
		q, ok := self.pick(core)
		if !ok {
			return
		}
		title, query = q.Name, q.Query
	default:
		// A saved query's name, or else the words are the query itself.
		name := strings.Join(words, " ")
		for _, q := range savedQueries(core) {
			if q.Name == name {
				title, query = q.Name, q.Query
			}
		}
		if query == "" {
			title, query = name, name
		}
	}

	data, err := commands.SendReceiveGetErr[common.GanttData](core, "gantt/tasks", map[string]string{"query": query})
	if err != nil {
		commands.Fail("gantt: %v", err)
	}
	if !data.Ok {
		commands.Fail("gantt: %s", data.Msg)
	}

	now := time.Now()
	m := Build(data.Tasks, now, !self.Weekends, self.Lane)

	crit := map[string]bool{}
	if self.Crit {
		crit = CriticalPath(m)
	}

	if commands.Machine() {
		self.machine(m, now, crit)
		return
	}
	if len(m.Tasks) == 0 {
		commands.Fail("nothing in %s to chart", title)
	}

	from, to := m.Start, m.End
	if self.From != "" {
		from = self.when(self.From, now)
	}
	if self.To != "" {
		// The last day drawn is part of the chart, so the window runs to the
		// midnight after it.
		to = addDays(self.when(self.To, now), 1)
	}
	if !to.After(from) {
		commands.Fail("gantt: -to has to be after -from")
	}

	pal := darkPalette
	if self.light() {
		pal = lightPalette
	}
	v := &view{
		m:      m,
		sch:    buildScheme(m.Tasks, self.colorMode(), pal),
		pal:    pal,
		now:    now,
		crit:   crit,
		width:  self.width(),
		from:   from,
		to:     to,
		title:  title,
		query:  query,
		colour: commands.Colour(),
	}
	fmt.Print(v.render())
}

// colorMode is one of the schemes by name, or a property to colour by.
func (self *Gantt) colorMode() string {
	c := strings.TrimSpace(self.Color)
	for _, m := range colorModes {
		if strings.EqualFold(m, c) {
			return m
		}
	}
	if c == "" {
		return "state"
	}
	return c
}

func (self *Gantt) when(s string, now time.Time) time.Time {
	d, _, err := commands.ParseDate(s, now)
	if err != nil {
		commands.Fail("gantt: cannot read %q as a date: %v", s, err)
	}
	return startOfDay(d.Day)
}

func (self *Gantt) width() int {
	if self.Width > 0 {
		return self.Width
	}
	if self.Pane {
		return commands.PaneWidth()
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 30 {
		return w
	}
	return 100
}

// light reads the theme: asked for, or the terminal's own say through
// COLORFGBG ("15;0" is light text on dark), else dark, which most terminals are.
func (self *Gantt) light() bool {
	switch strings.ToLower(self.Theme) {
	case "light":
		return true
	case "dark":
		return false
	}
	if v := os.Getenv("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		bgc := parts[len(parts)-1]
		return bgc == "7" || bgc == "15"
	}
	return false
}

// machine is the schedule for a program: every task with the dates the chart
// worked out, which is the part the server does not hand back.
func (self *Gantt) machine(m *Model, now time.Time, crit map[string]bool) {
	type row struct {
		Hash        string
		Headline    string
		Section     string
		Status      string
		State       string
		Start       string
		End         string
		Days        float64
		PlannedDays float64
		Percent     int
		Milestone   bool
		Mark        bool
		Implied     bool
		After       []string
		Health      string
		Critical    bool
		Filename    string
		LineNum     int
	}
	rows := []row{}
	for _, t := range m.Tasks {
		_, note := Health(t, now)
		rows = append(rows, row{
			Hash: t.ID, Headline: t.Name, Section: t.Section, Status: t.Src.Status,
			State: t.State, Start: t.Start.Format("2006-01-02 15:04"),
			End: t.End.Format("2006-01-02 15:04"), Days: t.Days, PlannedDays: t.PlannedDays,
			Percent: t.Src.Percent, Milestone: t.Milestone, Mark: t.Mark,
			Implied: t.Src.Implied, After: t.After, Health: note, Critical: crit[t.ID],
			Filename: t.Src.Filename, LineNum: t.Src.LineNum,
		})
	}
	commands.Render(rows, nil)
}

func savedQueries(core *commands.Core) []storedQuery {
	qs := commands.SendReceiveGetOr[[]storedQuery](core, "ext/queries", nil)
	sort.Slice(qs, func(i, j int) bool { return strings.ToLower(qs[i].Name) < strings.ToLower(qs[j].Name) })
	return qs
}

// pick offers the saved queries with each one's chart drawn beside it, which
// is the only way to tell "q4" from "q4-infra" before committing to one.
func (self *Gantt) pick(core *commands.Core) (storedQuery, bool) {
	qs := savedQueries(core)
	if len(qs) == 0 {
		commands.Fail("nothing saved yet - orgs q save <name> '<query>', or orgs gantt '<query>'")
	}
	width := 0
	for _, q := range qs {
		if len(q.Name) > width {
			width = len(q.Name)
		}
	}
	if !commands.Interactive() {
		for _, q := range qs {
			fmt.Printf("%s%-*s%s  %s%s%s\n", commands.C(commands.AnsiBold), width, q.Name,
				commands.C(commands.AnsiReset), commands.C(commands.AnsiDim), q.Query,
				commands.C(commands.AnsiReset))
		}
		return storedQuery{}, false
	}
	lines := []string{}
	for _, q := range qs {
		lines = append(lines, commands.PickLine([]string{q.Name},
			fmt.Sprintf("%-*s  %s%s%s", width, q.Name, commands.C(commands.AnsiDim), q.Query,
				commands.C(commands.AnsiReset))))
	}
	preview := ""
	if self_, err := commands.SelfCommand(core); err == nil {
		preview = self_ + " gantt -pane -color " + commands.Shq(self.Color) + " {1}"
		if self.Lane != "" {
			preview += " -lane " + commands.Shq(self.Lane)
		}
		if self.Theme != "" {
			preview += " -theme " + commands.Shq(self.Theme)
		}
		// The child's log lines go to stderr, which fzf shows in the pane.
		preview += " 2>/dev/null"
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Gantt> ",
		Preview:       preview,
	})
	if len(sel) == 0 {
		return storedQuery{}, false
	}
	addr, _ := commands.Address(sel[0], 1)
	for _, q := range qs {
		if q.Name == addr[0] {
			return q, true
		}
	}
	return storedQuery{}, false
}

func init() {
	commands.AddCmd("gantt", "a saved query as a gantt chart - the same plan worg draws",
		func() commands.Cmd { return &Gantt{} })
}

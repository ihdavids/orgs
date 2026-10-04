package cols

// orgs cols - org's column view of one file, editable, in the terminal.
//
//	orgs cols                      the file looked at last (or pick one)
//	orgs cols plan.org             that file (a name, or part of one, will do)
//	orgs cols plan.org -columns '%ITEM %TODO %EFFORT{:} %OWNER'
//	orgs cols plan.org -print      draw it once and exit (also what a pipe gets)
//	orgs cols plan.org -json       the rows and their cells, rollups worked out
//
// worg's Columns tab: the same GET /columns answer (the file's #+COLUMNS:
// line, effort rolled up the tree), the same edits through the same endpoints,
// and a line arranged here can be written into the file for both to use.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/cmd/oc/commands/tuikit"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Cols struct {
	Columns string
	Print   bool
	Width   int
	Theme   string
	Pane    bool
}

func (self *Cols) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Cols) StartPlugin(m *common.PluginManager)       {}

func (self *Cols) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Columns, "columns", "", "a columns line for this look, instead of the file's own")
	fset.BoolVar(&self.Print, "print", false, "draw the view once and exit")
	fset.IntVar(&self.Width, "width", 0, "columns to draw -print in (default: the terminal's)")
	fset.StringVar(&self.Theme, "theme", "", "dark or light (default: guessed from COLORFGBG, else dark)")
	fset.BoolVar(&self.Pane, "pane", false, "draw for a picker's preview pane")
}

func (self *Cols) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find(cmdName).Flags)
	if self.Pane {
		commands.PickerOutput()
		self.Print = true
		if self.Width <= 0 {
			self.Width = commands.PaneWidth()
		}
	}
	a := &app{UI: &tuikit.UI{Th: tuikit.ThemeFor(self.Theme)}, core: core,
		folded: map[string]bool{}, spec: self.Columns}
	a.states = commands.SendReceiveGetOr[common.TodoStatesResult](core, "status", nil)

	files := commands.SendReceiveGetOr[[]string](core, "files", nil)
	if len(words) > 0 {
		a.file = findFile(files, strings.Join(words, " "))
		if a.file == "" {
			commands.Fail("no org file the server knows matches %s", strings.Join(words, " "))
		}
	} else if commands.Interactive() && !commands.Machine() && !self.Print {
		// Nothing named: choose one, with each file's column view beside it.
		a.file = self.pick(core, files)
		if a.file == "" {
			return
		}
	} else if last := lastFile(); last != "" && containsStr(files, last) {
		a.file = last
	}

	if commands.Machine() {
		if a.file == "" {
			commands.Fail("orgs cols <file> -json")
		}
		a.loadQuiet()
		if !a.data.Ok {
			commands.Fail("%s", a.data.Msg)
		}
		commands.RenderOne(a.data, nil)
		return
	}
	if self.Print || !commands.Interactive() {
		if a.file == "" {
			commands.Fail("orgs cols <file> - there is %d org file%s to choose from", len(files), plural(len(files)))
		}
		self.print(a)
		return
	}

	scr, err := tcell.NewScreen()
	if err == nil {
		err = scr.Init()
	}
	if err != nil {
		commands.Fail("cols: %v", err)
	}
	a.Scr = scr
	a.edit, a.termEdit = tuikit.Editor(core)
	defer scr.Fini()
	rememberFile(a.file)
	a.run()
}

// pick is the fzf chooser over every org file the server watches, the one
// looked at last first, with the file's column view drawn in the pane.
// Cancelling it answers "".
func (self *Cols) pick(core *commands.Core, files []string) string {
	if len(files) == 0 {
		commands.Fail("the server knows no org files")
	}
	last := lastFile()
	ordered := []string{}
	if containsStr(files, last) {
		ordered = append(ordered, last)
	}
	for _, f := range files {
		if f != last {
			ordered = append(ordered, f)
		}
	}
	lines := []string{}
	for _, f := range ordered {
		lines = append(lines, commands.PickLine([]string{f},
			fmt.Sprintf("%s  %s%s%s", filepath.Base(f), commands.C(commands.AnsiDim), filepath.Dir(f),
				commands.C(commands.AnsiReset))))
	}
	preview := ""
	if me, err := commands.SelfCommand(core); err == nil {
		preview = me + " cols -pane"
		if self.Columns != "" {
			preview += " -columns " + commands.Shq(self.Columns)
		}
		if self.Theme != "" {
			preview += " -theme " + commands.Shq(self.Theme)
		}
		// The child's log lines go to stderr, which fzf shows in the pane.
		preview += " {1} 2>/dev/null"
	}
	sel := commands.Pick(commands.PickOpts{
		Lines:         lines,
		AddressFields: 1,
		Prompt:        "Columns> ",
		Preview:       preview,
	})
	if len(sel) == 0 {
		return ""
	}
	addr, ok := commands.Address(sel[0], 1)
	if !ok {
		return ""
	}
	return addr[0]
}

// loadQuiet reads the view for a program, with no screen to measure.
func (a *app) loadQuiet() {
	ps := map[string]string{"file": a.file}
	if a.spec != "" {
		ps["columns"] = a.spec
	}
	res, err := commands.SendReceiveGetErr[Result](a.core, "columns", ps)
	if err != nil {
		commands.Fail("cols: %v", err)
	}
	a.data = res
}

// findFile is the file a name means: the whole path, the base name, the base
// name without .org, then any file whose name contains it.
func findFile(files []string, name string) string {
	if abs, err := filepath.Abs(name); err == nil && containsStr(files, abs) {
		return abs
	}
	low := strings.ToLower(name)
	for _, f := range files {
		b := strings.ToLower(filepath.Base(f))
		if b == low || strings.TrimSuffix(b, ".org") == low {
			return f
		}
	}
	for _, f := range files {
		if strings.Contains(strings.ToLower(filepath.Base(f)), low) {
			return f
		}
	}
	return ""
}

func (self *Cols) print(a *app) {
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
	sim.SetSize(w, 10)
	a.load()
	h := len(a.rows) + top + 4
	sim.SetSize(w, h)
	a.draw()
	fmt.Print(tuikit.Dump(sim, w, h, commands.Colour()))
}

const cmdName = "cols"

func init() {
	commands.AddCmd(cmdName, "org's column view of a file, editable - worg's Columns tab",
		func() commands.Cmd { return &Cols{} })
}

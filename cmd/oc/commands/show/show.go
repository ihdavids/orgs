package show

// orgs show - one heading, whole.
//
//	orgs show                       pick one and read it
//	orgs show 'IsStatus("NEXT")'    the first match, or a picker over them
//	orgs show -hash hOpO…           by hash
//	orgs show -log                  with the clock entries
//	orgs show -raw                  the body text and nothing else
//	orgs show -edit                 open it where it lives
//
// This is the terminal's answer to "what is this heading", and it is also the
// pane every heading picker in the tool draws beside its list: `-pane` is the
// same drawing squeezed into whatever width fzf gave it. One command rather
// than two, because a pane that says less than the command does is a pane
// somebody has to leave to find out.
//
// It is made of three requests - /hash for what the database knows, /body for
// the text as written, /logbook for the clocks - and the body is asked for as
// *written* rather than as html: the point of reading a heading in a terminal
// is seeing the drawer, the table and the source block that a rendered page
// smooths away.

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

type Show struct {
	tf   commands.TargetFlags
	Log  bool
	Raw  bool
	Pane bool
	Edit bool
	All  bool
}

// What one `orgs show` answers with, and what -json prints. It is a type of
// its own rather than a map so the shape is documented and stable.
type Shown struct {
	Headline string
	Status   string
	Priority string
	Tags     []string
	Props    map[string]string
	Filename string
	LineNum  int
	Level    int
	Hash     string
	Body     string
	Audio    string   `json:",omitempty"`
	Images   []string `json:",omitempty"`
	Logbook  *common.Logbook
}

func (self *Show) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Show) StartPlugin(manager *common.PluginManager)         {}

func (self *Show) SetupParameters(fset *flag.FlagSet) {
	commands.AddTargetFlags(fset, &self.tf)
	fset.BoolVar(&self.Log, "log", false, "include the clock entries from the logbook")
	fset.BoolVar(&self.Raw, "raw", false, "the body text as written, and nothing else")
	fset.BoolVar(&self.Pane, "pane", false, "draw as a preview pane, to whatever width there is")
	fset.BoolVar(&self.Edit, "edit", false, "open the heading in the editor instead of printing it")
	fset.BoolVar(&self.All, "every", false, "show every match rather than narrowing to one")
}

func (self *Show) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("show").Flags)

	// A pane is a fresh process with no terminal, so the usual "nobody to ask"
	// rules would send it to the refusal rather than to the drawing. It is
	// always given a hash, so there is nothing to resolve.
	if self.Pane && self.tf.Hash != "" {
		self.draw(core, self.gather(core, commands.TodoByHash(core, self.tf.Hash)), true)
		return
	}

	if self.All {
		self.tf.All = true
	}
	todos := commands.Resolve(core, &self.tf, words, commands.TargetOpts{
		Prompt: "Show> ",
		Multi:  true,
	})

	if self.Edit {
		for _, t := range todos {
			core.LaunchEditor(t.Filename, t.LineNum)
		}
		return
	}

	shown := make([]Shown, 0, len(todos))
	for _, t := range todos {
		shown = append(shown, self.gather(core, t))
	}

	if self.Raw {
		// The body, as written, for a pipeline. No boxes, no colour, no
		// heading line - somebody asked for the text.
		if commands.Render(shown, nil) {
			return
		}
		for i, s := range shown {
			if i > 0 {
				fmt.Println()
			}
			fmt.Print(s.Body)
			if !strings.HasSuffix(s.Body, "\n") {
				fmt.Println()
			}
		}
		return
	}

	if len(shown) == 1 {
		if commands.RenderOne(shown[0], func() { self.draw(core, shown[0], false) }) {
			return
		}
		return
	}
	if commands.Render(shown, func() {
		for i, s := range shown {
			if i > 0 {
				fmt.Println()
			}
			self.draw(core, s, false)
		}
	}) {
		return
	}
}

// gather is the three requests. A heading with no body is the common case and
// not a failure, so a body that does not come back is left empty rather than
// being complained about.
func (self *Show) gather(core *commands.Core, t common.Todo) Shown {
	s := Shown{
		Headline: t.Headline,
		Status:   t.Status,
		Priority: t.Priority,
		Tags:     t.Tags,
		Props:    t.Props,
		Filename: t.Filename,
		LineNum:  t.LineNum,
		Level:    t.Level,
		Hash:     t.Hash,
	}
	if body, err := commands.SendReceiveGetErr[bodyResult](core,
		"body/"+commands.HashPath(t.Hash), nil); err == nil && body.Ok {
		s.Body = body.Text
		s.Audio = body.Audio
		s.Images = body.Images
	}
	if self.Log {
		if lb, err := commands.SendReceiveGetErr[common.Logbook](core,
			"logbook/"+commands.HashPath(t.Hash), nil); err == nil && len(lb.Entries) > 0 {
			s.Logbook = &lb
		}
	}
	// The dates live on the Todo rather than in the body, so they are put back
	// into the props map under their org names - which is where a reader
	// expects to find them and what `-format '{{prop .Props "SCHEDULED"}}'`
	// can then reach.
	if s.Props == nil {
		s.Props = map[string]string{}
	}
	if t.Date != nil && !t.Date.Start.IsZero() {
		s.Props["SCHEDULED"] = t.Date.Start.Format("2006-01-02 Mon")
	}
	if t.Deadline != nil && !t.Deadline.Start.IsZero() {
		s.Props["DEADLINE"] = t.Deadline.Start.Format("2006-01-02 Mon")
	}
	return s
}

// bodyResult is the shape /body/{hash} answers with. Restated here rather than
// imported because it lives in internal/app/orgs, which a command package may
// not import - that package already imports every command.
type bodyResult struct {
	Ok     bool
	Msg    string
	Text   string
	Audio  string
	Image  string
	Images []string
}

// draw is the human reading. In a pane it is boxes at the pane's width; on a
// terminal it is the same thing at the terminal's width, because the two are
// the same act.
func (self *Show) draw(core *commands.Core, s Shown, pane bool) {
	width := 80
	if pane {
		width = commands.PaneWidth()
	} else if w := termWidth(); w > 0 {
		width = w
	}
	if width > 100 && !pane {
		width = 100
	}

	stars := strings.Repeat("*", max(s.Level, 1))
	kw := ""
	if s.Status != "" {
		kw = commands.C(statusColour(s.Status)) + s.Status + commands.C(commands.AnsiReset) + " "
	}
	pri := ""
	if s.Priority != "" {
		pri = commands.C(commands.AnsiRed) + "[#" + s.Priority + "] " + commands.C(commands.AnsiReset)
	}
	fmt.Printf("%s%s%s %s%s%s%s\n", commands.C(commands.AnsiDim), stars,
		commands.C(commands.AnsiReset), kw, pri,
		commands.C(commands.AnsiBold)+s.Headline+commands.C(commands.AnsiReset), tagStr(s.Tags))
	fmt.Printf("%s%s:%d%s\n", commands.C(commands.AnsiDim), s.Filename, s.LineNum,
		commands.C(commands.AnsiReset))

	// The dates first and on their own line: they are what a reader of a task
	// is looking for, and burying them among the properties makes them
	// something to hunt for.
	dates := []string{}
	for _, k := range []string{"SCHEDULED", "DEADLINE", "CLOSED"} {
		if v, ok := s.Props[k]; ok && v != "" {
			dates = append(dates, fmt.Sprintf("%s%s:%s %s", commands.C(commands.AnsiGold), k,
				commands.C(commands.AnsiReset), v))
		}
	}
	if len(dates) > 0 {
		fmt.Printf("  %s\n", strings.Join(dates, "  "))
	}

	if props := otherProps(s.Props); len(props) > 0 {
		commands.OpenBox("properties", width)
		for _, k := range props {
			commands.BoxLine(fmt.Sprintf("%s%-14s%s %s", commands.C(commands.AnsiCyan), k,
				commands.C(commands.AnsiReset), s.Props[k]))
		}
		commands.CloseBox(width)
	}

	if strings.TrimSpace(s.Body) != "" {
		commands.OpenBox("body", width)
		commands.BoxText(strings.TrimRight(s.Body, "\n"))
		commands.CloseBox(width)
	}

	if s.Audio != "" {
		fmt.Printf("  %s♫%s %s\n", commands.C(commands.AnsiPurple),
			commands.C(commands.AnsiReset), s.Audio)
	}
	for _, img := range s.Images {
		fmt.Printf("  %s▣%s %s\n", commands.C(commands.AnsiPurple),
			commands.C(commands.AnsiReset), img)
	}

	if s.Logbook != nil && len(s.Logbook.Entries) > 0 {
		commands.OpenBox(fmt.Sprintf("clocked %s", hours(s.Logbook.TotalMin)), width)
		for _, e := range s.Logbook.Entries {
			commands.BoxLine(clockLine(e))
		}
		commands.CloseBox(width)
	}
}

func clockLine(e common.LogbookEntry) string {
	start, err := time.Parse(time.RFC3339, e.Start)
	if err != nil {
		return fmt.Sprintf("%s  %s", e.Start, hours(e.Mins))
	}
	end := "running"
	if e.End != "" {
		if t, err := time.Parse(time.RFC3339, e.End); err == nil {
			end = t.Format("15:04")
		}
	}
	return fmt.Sprintf("%s  %s → %-8s %s%s%s", start.Format("2006-01-02 Mon"),
		start.Format("15:04"), end, commands.C(commands.AnsiGreen), hours(e.Mins),
		commands.C(commands.AnsiReset))
}

func hours(mins float64) string {
	h := int(mins) / 60
	m := int(mins) % 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}

// otherProps is everything except the ones drawn elsewhere, in a stable order.
// ORIGINAL_* and the dates are already said; a listing that repeats itself
// reads as though the second copy means something.
func otherProps(props map[string]string) []string {
	skip := map[string]bool{"SCHEDULED": true, "DEADLINE": true, "CLOSED": true}
	out := []string{}
	for k := range props {
		if skip[k] || props[k] == "" {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func tagStr(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return " " + commands.C(commands.AnsiCyan) + ":" + strings.Join(tags, ":") + ":" +
		commands.C(commands.AnsiReset)
}

// statusColour is the one place the terminal clients agree on what a keyword
// looks like. Done is green and everything else is gold, which is as much as
// can be said without knowing the file's own #+TODO line.
func statusColour(status string) string {
	switch strings.ToUpper(status) {
	case "DONE", "CANCELLED", "CANCELED", "CLOSED":
		return commands.AnsiGreen
	case "BLOCKED", "WAITING", "HOLD":
		return commands.AnsiRed
	default:
		return commands.AnsiGold
	}
}

func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 20 {
		return w
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	commands.AddCmd("show", "read one heading whole - its dates, drawer, body and clocks",
		func() commands.Cmd { return &Show{} })
}

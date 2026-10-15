package printcmd

// orgs print: the agenda as a paper planner.
//
//	orgs print                         this week, a4 landscape, written to week-2026-10-12.html
//	orgs print week -from mon -weeks 4 -f pdf
//	orgs print day -from tomorrow -theme minimal -f pdf -open
//	orgs print month -from +1m -paper letter -out november.pdf
//	orgs print -list-themes
//
// The server lays the page out (GET /agenda/print, from the same agenda as
// GET /agenda) and this side writes it, or prints it to pdf with a headless
// Chrome on this machine, as orgs invoice does: the browser is where the
// person is, and the server may have none.

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"golang.org/x/term"
)

type Print struct {
	fset *flag.FlagSet

	From       string
	Weeks      int
	Days       int
	Months     int
	Format     string
	Out        string
	Theme      string
	Paper      string
	Query      string
	Tasks      string
	NoHabits   bool
	WeekStart  string
	Open       bool
	ListThemes bool
}

// What -json says was written.
type Printed struct {
	Ok     bool
	View   string
	From   string
	Sheets int
	Format string
	File   string `json:",omitempty"`
	Bytes  int
}

func (self *Print) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Print) StartPlugin(manager *common.PluginManager)         {}
func (self *Print) HelpGroup() string                                 { return "Finding things" }

func (self *Print) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.From, "from", "", "a day in the first sheet: 2026-10-12, today, tomorrow, mon, +1w (default today)")
	fset.IntVar(&self.Weeks, "weeks", 0, "week view: how many weeks (default 1)")
	fset.IntVar(&self.Days, "days", 0, "day view: how many days (default 1)")
	fset.IntVar(&self.Months, "months", 0, "month view: how many months (default 1)")
	fset.StringVar(&self.Format, "f", "html", "html or pdf (pdf needs Chrome, Chromium or Edge here)")
	fset.StringVar(&self.Out, "out", "", "the file to write (default: week-<date>.html or .pdf; - is stdout)")
	fset.StringVar(&self.Theme, "theme", "", "classic, minimal, bullet, modern, or one of yours (-list-themes)")
	fset.StringVar(&self.Paper, "paper", "a4", "a4 or letter")
	fset.StringVar(&self.Query, "query", "", "only headings this query finds, as orgs agenda")
	fset.StringVar(&self.Tasks, "tasks", "", "the query for the undated tasks panel (none for no panel)")
	fset.BoolVar(&self.NoHabits, "no-habits", false, "no habits strip, and habits listed on their days")
	fset.StringVar(&self.WeekStart, "weekstart", "mon", "mon or sun")
	fset.BoolVar(&self.Open, "open", false, "open what was written")
	fset.BoolVar(&self.ListThemes, "list-themes", false, "the themes the server has")
}

// The first day of the first sheet, for the file's name: the week's start,
// the day, or the month.
func firstDay(view string, from time.Time, weekStart time.Weekday) time.Time {
	switch view {
	case "day":
		return from
	case "month":
		return time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location())
	}
	back := (int(from.Weekday()) - int(weekStart) + 7) % 7
	return from.AddDate(0, 0, -back)
}

// defaultName is week-2026-10-12.pdf, day-2026-10-14.html, month-2026-10.pdf.
func defaultName(view string, first time.Time, format string) string {
	if view == "month" {
		return fmt.Sprintf("month-%s.%s", first.Format("2006-01"), format)
	}
	return fmt.Sprintf("%s-%s.%s", view, first.Format("2006-01-02"), format)
}

func (self *Print) Exec(core *commands.Core) {
	if self.ListThemes {
		body, _, status, err := core.Rest.GetRaw("agenda/print", map[string]string{"list": "t"})
		if err != nil {
			commands.Fail("orgs print: %v", err)
		}
		if status >= 400 {
			commands.Fail("orgs print: %s", strings.TrimSpace(string(body)))
		}
		names := strings.Fields(string(body))
		commands.Render(names, func() {
			for _, n := range names {
				fmt.Println(n)
			}
		})
		return
	}
	words := commands.FreeArgs(self.fset)
	view := "week"
	if len(words) > 1 {
		commands.Fail("orgs print [week|day|month] [-from DATE] [-f html|pdf] [-out FILE]")
	}
	if len(words) == 1 {
		view = strings.ToLower(words[0])
	}
	pages := 1
	switch view {
	case "week", "w":
		view = "week"
		pages = max(1, self.Weeks)
	case "day", "d", "today":
		view = "day"
		pages = max(1, self.Days)
	case "month", "m":
		view = "month"
		pages = max(1, self.Months)
	default:
		commands.Fail("orgs print: %q is not a view: week, day or month", words[0])
	}
	format := strings.ToLower(self.Format)
	if format != "html" && format != "pdf" {
		commands.Fail("orgs print -f %s: html or pdf", self.Format)
	}
	ws := time.Monday
	switch strings.ToLower(self.WeekStart) {
	case "mon", "monday", "":
	case "sun", "sunday":
		ws = time.Sunday
	default:
		commands.Fail("orgs print -weekstart %s: mon or sun", self.WeekStart)
	}
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if strings.TrimSpace(self.From) != "" {
		d, ok, err := commands.ParseDate(self.From, now)
		if err != nil || !ok || d.Day.IsZero() {
			commands.Fail("orgs print -from %q: not a date I can read (try 2026-10-12, tomorrow, mon, +1w)", self.From)
		}
		from = time.Date(d.Day.Year(), d.Day.Month(), d.Day.Day(), 0, 0, 0, 0, time.Local)
	}
	ps := map[string]string{
		"view": view, "from": from.Format("2006-01-02"), "pages": fmt.Sprint(pages),
		"theme": self.Theme, "paper": strings.ToLower(self.Paper), "query": self.Query,
		"tasks": self.Tasks, "weekstart": strings.ToLower(self.WeekStart),
	}
	if self.NoHabits {
		ps["habits"] = "f"
	}

	out := self.Out
	toStdout := out == "-"
	if out == "" {
		// A page on a terminal is noise; a pipe gets the bytes.
		if term.IsTerminal(int(os.Stdout.Fd())) || self.Open || commands.Machine() {
			out = defaultName(view, firstDay(view, from, ws), format)
		} else {
			toStdout = true
		}
	}
	if !toStdout && commands.Wrote(fmt.Sprintf("print the %s of %s to %s", view, from.Format("2 Jan 2006"), out), ps) {
		return
	}

	body, ctype, status, err := core.Rest.GetRaw("agenda/print", ps)
	if err != nil {
		commands.Fail("orgs print: %v", err)
	}
	if status >= 400 || !strings.Contains(ctype, "html") {
		msg := strings.TrimSpace(string(body))
		if status == 404 {
			msg = "the server has no /agenda/print (is it older than this command?)"
		}
		commands.Fail("orgs print: %s", msg)
	}
	if format == "pdf" {
		if body, err = commands.PrintPDF(body); err != nil {
			commands.Fail("orgs print: %v", err)
		}
	}
	if toStdout {
		os.Stdout.Write(body)
		return
	}
	if err := os.WriteFile(out, body, 0o644); err != nil {
		commands.Fail("orgs print: could not write %s: %v", out, err)
	}
	res := Printed{Ok: true, View: view, From: from.Format("2006-01-02"), Sheets: pages, Format: format, File: out, Bytes: len(body)}
	if self.Open && !commands.Machine() {
		commands.OpenInBrowser(out)
	}
	commands.RenderOne(res, func() {
		what := view
		if pages > 1 {
			what = fmt.Sprintf("%d %ss", pages, view)
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%s from %s, %d bytes)\n", out, what, from.Format("2 Jan 2006"), len(body))
	})
}

func init() {
	commands.AddCmd("print", "the agenda as a printable planner: a week, a day or a month, html or pdf",
		func() commands.Cmd { return &Print{Format: "html", Paper: "a4", WeekStart: "mon"} })
}

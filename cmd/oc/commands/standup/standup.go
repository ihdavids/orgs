package standup

// `orgs standup` and `orgs recap`: what you did, what is next and what is in
// the way, as Markdown ready to paste into Slack.
//
//	orgs standup                  since the last working day (Friday, on a Monday)
//	orgs standup -since mon       from a day of your choosing
//	orgs standup -md -copy        GitHub Markdown, and onto the clipboard
//	orgs recap                    the last seven days
//	orgs recap -month             this month so far
//	orgs recap -last -week        last week, Monday to Sunday
//	orgs recap -from 2026-10-01 -to 2026-10-15
//
// Both are one request to GET /recap, which reads every part with the rules
// of the view it comes from (the agenda, the clock table, the weekly review's
// waiting list, /insights/deps, the writing log), so a standup never says
// something the agenda would contradict. This side only lays it out. -json is
// the whole answer.
//
// Slack's own markup is not Markdown: *bold*, _italic_, ~strike~, no
// headings, and a list is a bullet character typed in the text. -md (or
// -format md) writes GitHub Markdown instead, for a pull request, a wiki or
// an issue.

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// How a recap is written: Slack's markup or GitHub's.
type style struct {
	md bool
}

func (s style) bold(t string) string {
	if s.md {
		return "**" + t + "**"
	}
	return "*" + t + "*"
}

func (s style) italic(t string) string { return "_" + t + "_" }

func (s style) strike(t string) string {
	if s.md {
		return "~~" + t + "~~"
	}
	return "~" + t + "~"
}

func (s style) bullet() string {
	if s.md {
		return "- "
	}
	return "• "
}

// A section's heading: a bold line in Slack (it has no headings), a level
// three heading in Markdown.
func (s style) heading(icon, t string) string {
	if s.md {
		return "### " + icon + " " + t
	}
	return icon + " " + s.bold(t)
}

// A title as text, not markup. The server has already taken org's emphasis
// off; what is left could still start Markdown emphasis by accident (a file
// name with underscores turning italic halfway), so Markdown gets it escaped.
// Slack has no escape, and only treats markers at word edges as markup, so a
// title goes to it as it is.
var mdEscape = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "`", "\\`")

func (s style) text(t string) string {
	if s.md {
		return mdEscape.Replace(t)
	}
	return t
}

// Minutes as a person says them: 45m, 2h, 1h 25m.
func hm(mins float64) string {
	m := int(math.Round(mins))
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%dh", m/60)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func thousands(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// The place a heading belongs to in a list: its project, else its file.
func groupOf(it common.RecapItem) string {
	if it.Project != "" {
		return it.Project
	}
	return strings.TrimSuffix(filepath.Base(it.Filename), ".org")
}

// One finished heading as a line.
func (s style) closedLine(it common.RecapItem, withProject bool) string {
	t := s.text(it.Headline)
	if it.Cancelled {
		t = s.strike(t) + " " + s.italic("cancelled")
	}
	extra := []string{}
	if it.Repeat && it.Count > 1 {
		extra = append(extra, fmt.Sprintf("×%d", it.Count))
	} else if it.Repeat {
		extra = append(extra, "repeating")
	}
	if withProject && it.Project != "" {
		extra = append(extra, it.Project)
	}
	if len(extra) > 0 {
		t += " " + s.italic("("+s.text(strings.Join(extra, ", "))+")")
	}
	return s.bullet() + t
}

// One of today's headings as a line.
func (s style) todayLine(it common.RecapItem, running *common.RecapItem) string {
	t := s.text(it.Headline)
	if it.When != "" {
		t = it.When + " " + t
	}
	switch it.Kind {
	case "overdue":
		t += " " + s.italic(fmt.Sprintf("overdue %dd", it.Late))
	case common.AgendaDeadline:
		switch {
		case it.Late > 0:
			t += " " + s.italic(fmt.Sprintf("deadline passed %dd ago", it.Late))
		case it.DueIn > 0:
			t += " " + s.italic(fmt.Sprintf("due in %dd", it.DueIn))
		default:
			t += " " + s.italic("due today")
		}
	case "next":
		if it.Status != "" {
			t += " " + s.italic("("+it.Status+")")
		}
	}
	if running != nil && running.Hash == it.Hash {
		t += " ⏳"
	}
	return s.bullet() + t
}

// What stands in the way, waiting first (somebody to chase), then held-up work.
func (s style) blockers(r common.Recap, b *strings.Builder) int {
	n := 0
	for _, w := range r.Waiting {
		days := ""
		switch {
		case w.Days < 0:
			days = " " + s.italic("(no date)")
		case w.Days == 0:
			days = " " + s.italic("(since today)")
		case w.SinceFrom == "logbook":
			days = " " + s.italic(fmt.Sprintf("(%s)", plural(w.Days, "day", "days")))
		default:
			days = " " + s.italic(fmt.Sprintf("(about %s)", plural(w.Days, "day", "days")))
		}
		fmt.Fprintf(b, "%sWaiting on %s%s\n", s.bullet(), s.text(w.Headline), days)
		n++
	}
	for _, p := range r.Blocked {
		fmt.Fprintf(b, "%s%s\n", s.bullet(), s.text(p.Title))
		n++
	}
	return n
}

func (s style) runningLine(r common.Recap) string {
	if r.Running == nil {
		return ""
	}
	return fmt.Sprintf("⏳ On the clock: %s %s\n", s.text(r.Running.Headline),
		s.italic(fmt.Sprintf("(since %s, %s)", r.Running.When, hm(r.Running.Mins))))
}

func day(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
	return t
}

// "since Friday", "since Oct 3", "yesterday".
func sinceWords(from, today time.Time) string {
	d := int(today.Sub(from).Hours()/24 + 0.5)
	switch {
	case d == 1:
		return "yesterday"
	case d <= 6:
		return "since " + from.Format("Monday")
	}
	return "since " + from.Format("Jan 2")
}

// ---------------------------------------------------------------------------
// Shared flags
// ---------------------------------------------------------------------------

type out struct {
	Md     bool
	Format string
	Copy   bool
}

func (o *out) flags(fset *flag.FlagSet) {
	fset.BoolVar(&o.Md, "md", false, "GitHub Markdown instead of Slack's markup")
	fset.StringVar(&o.Format, "format", "slack", "slack or md")
	fset.BoolVar(&o.Copy, "copy", false, "also put it on the clipboard")
}

func (o *out) style() style {
	f := strings.ToLower(o.Format)
	return style{md: o.Md || f == "md" || f == "markdown" || f == "github" || f == "gfm"}
}

func fetch(core *commands.Core, q map[string]string) common.Recap {
	res, err := commands.SendReceiveGetErr[common.Recap](core, "recap", q)
	if err != nil {
		commands.Fail("recap: %v", err)
	}
	if !res.Ok {
		commands.Fail("recap: %s", res.Msg)
	}
	return res
}

// Prints the text and, with -copy, hands it to the clipboard too: the point
// is pasting, and a pipe through pbcopy loses what was printed.
func (o *out) emit(text string) {
	fmt.Print(text)
	if !o.Copy {
		return
	}
	cmds := [][]string{}
	switch runtime.GOOS {
	case "darwin":
		cmds = [][]string{{"pbcopy"}}
	case "windows":
		cmds = [][]string{{"clip"}}
	default:
		cmds = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	for _, c := range cmds {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			fmt.Fprintf(os.Stderr, "%scopied to the clipboard%s\n", commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
			return
		}
	}
	fmt.Fprintln(os.Stderr, "no clipboard program found (pbcopy, wl-copy, xclip, xsel, clip)")
}

// ---------------------------------------------------------------------------
// orgs standup
// ---------------------------------------------------------------------------

type Standup struct {
	out
	Since string
	Max   int
}

func (self *Standup) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Standup) StartPlugin(manager *common.PluginManager)       {}
func (self *Standup) HelpGroup() string                                 { return "Reports" }

func (self *Standup) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.Since, "since", "", "first day to report (default: the last working day): mon, 2026-10-20, -2d")
	fset.IntVar(&self.Max, "n", 8, "at most this many lines a section (the rest are counted)")
	self.out.flags(fset)
}

// The last working day before today: Friday on a Saturday, Sunday or Monday.
func lastWorkingDay(today time.Time) time.Time {
	d := today.AddDate(0, 0, -1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

func (self *Standup) Exec(core *commands.Core) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	from := lastWorkingDay(today)
	if self.Since != "" {
		d, ok, err := commands.ParseDate(self.Since, now)
		if err != nil || !ok {
			commands.Fail("-since %q is not a day", self.Since)
		}
		from = d.Day
	}
	// Up to yesterday: today's work is the Today section, not news yet.
	to := today.AddDate(0, 0, -1)
	if !to.After(from) {
		to = from
	}
	r := fetch(core, map[string]string{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")})
	if commands.RenderOne(r, nil) {
		return
	}
	self.emit(self.render(r, from, today))
}

func (self *Standup) render(r common.Recap, from, today time.Time) string {
	s := self.style()
	max := self.Max
	if max <= 0 {
		max = 1 << 30
	}
	b := &strings.Builder{}
	title := "Standup · " + today.Format("Mon Jan 2")
	if s.md {
		fmt.Fprintf(b, "## %s\n\n", title)
	} else {
		fmt.Fprintf(b, "%s\n\n", s.bold(title))
	}

	// Done.
	fmt.Fprintf(b, "%s\n", s.heading("✅", "Done "+sinceWords(from, today)))
	if len(r.Closed) == 0 && len(r.Habits) == 0 {
		fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic("nothing closed"))
	}
	for i, it := range r.Closed {
		if i == max {
			fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic(fmt.Sprintf("and %d more", len(r.Closed)-max)))
			break
		}
		fmt.Fprintln(b, s.closedLine(it, true))
	}
	if len(r.Habits) > 0 {
		names := []string{}
		for _, h := range r.Habits {
			names = append(names, s.text(h.Headline))
		}
		fmt.Fprintf(b, "%sHabits kept: %s\n", s.bullet(), strings.Join(names, ", "))
	}

	// Clocked.
	if r.ClockedMins > 0 {
		fmt.Fprintf(b, "\n%s %s\n", s.heading("⏱", "Clocked"), hm(r.ClockedMins))
		for i, g := range r.Clocked {
			if i == max {
				fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic(fmt.Sprintf("and %d more", len(r.Clocked)-max)))
				break
			}
			parts := []string{}
			for j, it := range g.Items {
				if j == 3 {
					parts = append(parts, fmt.Sprintf("+%d more", len(g.Items)-3))
					break
				}
				parts = append(parts, fmt.Sprintf("%s %s", s.text(it.Headline), hm(it.Mins)))
			}
			name := strings.TrimSuffix(g.Name, ".org")
			if len(g.Items) == 1 && g.Items[0].Headline == g.Name {
				fmt.Fprintf(b, "%s%s %s\n", s.bullet(), s.text(name), hm(g.Mins))
				continue
			}
			fmt.Fprintf(b, "%s%s %s · %s\n", s.bullet(), s.bold(s.text(name)), hm(g.Mins), strings.Join(parts, ", "))
		}
	}

	// Today.
	fmt.Fprintf(b, "\n%s\n", s.heading("📅", "Today"))
	if line := s.runningLine(r); line != "" {
		fmt.Fprint(b, line)
	}
	if len(r.Agenda) == 0 {
		fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic("nothing scheduled"))
	}
	for i, it := range r.Agenda {
		if i == max {
			fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic(fmt.Sprintf("and %d more", len(r.Agenda)-max)))
			break
		}
		fmt.Fprintln(b, s.todayLine(it, r.Running))
	}

	// Blockers.
	fmt.Fprintf(b, "\n%s\n", s.heading("🚧", "Blockers"))
	if s.blockers(r, b) == 0 {
		fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic("none"))
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// orgs recap
// ---------------------------------------------------------------------------

type Recap struct {
	out
	Week  bool
	Month bool
	Last  bool
	From  string
	To    string
	Block string
	Max   int
}

func (self *Recap) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Recap) StartPlugin(manager *common.PluginManager)       {}
func (self *Recap) HelpGroup() string                                 { return "Reports" }

func (self *Recap) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Week, "week", false, "the last seven days (the default); with -last, last week Monday to Sunday")
	fset.BoolVar(&self.Month, "month", false, "this month so far; with -last, last month")
	fset.BoolVar(&self.Last, "last", false, "the whole previous week or month")
	fset.StringVar(&self.From, "from", "", "first day: 2026-10-01, mon, -14d")
	fset.StringVar(&self.To, "to", "", "last day, inclusive (default today)")
	fset.StringVar(&self.Block, "block", "", "a clock table range instead: thisweek, lastmonth, 2026-W40, 2026-Q3")
	fset.IntVar(&self.Max, "n", 10, "at most this many lines in a group (the rest are counted)")
	self.out.flags(fset)
}

func (self *Recap) Exec(core *commands.Core) {
	now := time.Now()
	q := map[string]string{}
	switch {
	case self.Block != "":
		q["block"] = self.Block
	case self.Month && self.Last:
		q["block"] = "lastmonth"
	case self.Month:
		q["block"] = "thismonth"
	case self.Last:
		q["block"] = "lastweek"
	}
	for flagName, v := range map[string]string{"from": self.From, "to": self.To} {
		if v == "" {
			continue
		}
		d, ok, err := commands.ParseDate(v, now)
		if err != nil || !ok {
			commands.Fail("-%s %q is not a day", flagName, v)
		}
		q[flagName] = d.Day.Format("2006-01-02")
		delete(q, "block")
	}
	if q["from"] != "" && q["to"] == "" {
		q["to"] = now.Format("2006-01-02")
	}
	r := fetch(core, q)
	if commands.RenderOne(r, nil) {
		return
	}
	self.emit(self.render(r))
}

func rangeTitle(r common.Recap) string {
	from, to := day(r.From), day(r.To)
	days := int(to.Sub(from).Hours()/24+0.5) + 1
	switch {
	case from.Day() == 1 && to.AddDate(0, 0, 1).Day() == 1:
		return from.Format("January 2006")
	case from.Day() == 1 && r.To == r.Today:
		return from.Format("January 2006") + " so far"
	case days == 7 && from.Weekday() == time.Monday:
		y, w := from.ISOWeek()
		return fmt.Sprintf("Week %d · %s – %s", w, from.Format("Jan 2"), to.Format("Jan 2")) + yearIf(y, to)
	case from.Year() != to.Year():
		return from.Format("Jan 2, 2006") + " – " + to.Format("Jan 2, 2006")
	}
	return from.Format("Jan 2") + " – " + to.Format("Jan 2")
}

func yearIf(y int, to time.Time) string {
	if y != time.Now().Year() {
		return fmt.Sprintf(", %d", to.Year())
	}
	return ""
}

func (self *Recap) render(r common.Recap) string {
	s := self.style()
	max := self.Max
	if max <= 0 {
		max = 1 << 30
	}
	b := &strings.Builder{}
	title := "Recap · " + rangeTitle(r)
	if s.md {
		fmt.Fprintf(b, "## %s\n\n", title)
	} else {
		fmt.Fprintf(b, "%s\n", s.bold(title))
	}

	// The headline numbers.
	done, cancelled := 0, 0
	for _, c := range r.Closed {
		if c.Cancelled {
			cancelled++
		} else {
			done++
		}
	}
	nums := []string{plural(done, "done", "done")}
	if r.ClockedMins > 0 {
		nums = append(nums, hm(r.ClockedMins)+" clocked")
	}
	if w := r.Writing; w != nil && w.Words != 0 {
		words := thousands(w.Words) + " words"
		if w.Goal > 0 {
			words += fmt.Sprintf(" (%d%% of goal)", int(math.Round(100*float64(w.Words)/float64(w.Goal))))
		}
		nums = append(nums, words)
	}
	if len(r.Habits) > 0 {
		kept, possible := 0, 0
		for _, h := range r.Habits {
			kept += h.Done
			possible += h.Days
		}
		nums = append(nums, fmt.Sprintf("habits %d/%d", kept, possible))
	}
	if n := len(r.Waiting) + len(r.Blocked); n > 0 {
		nums = append(nums, plural(n, "blocker", "blockers"))
	}
	fmt.Fprintf(b, "%s\n", s.italic(strings.Join(nums, " · ")))

	// Done, by project.
	if len(r.Closed) > 0 {
		head := fmt.Sprintf("Done (%d)", done)
		if cancelled > 0 {
			head = fmt.Sprintf("Done (%d, %d cancelled)", done, cancelled)
		}
		fmt.Fprintf(b, "\n%s\n", s.heading("✅", head))
		groups := map[string][]common.RecapItem{}
		order := []string{}
		for _, c := range r.Closed {
			g := groupOf(c)
			if _, ok := groups[g]; !ok {
				order = append(order, g)
			}
			groups[g] = append(groups[g], c)
		}
		// Biggest group first, then by name, so the week's main thread leads.
		sort.SliceStable(order, func(a, b int) bool {
			if len(groups[order[a]]) != len(groups[order[b]]) {
				return len(groups[order[a]]) > len(groups[order[b]])
			}
			return order[a] < order[b]
		})
		for _, g := range order {
			items := groups[g]
			if len(order) > 1 {
				fmt.Fprintf(b, "%s\n", s.bold(s.text(g)))
			}
			for i, it := range items {
				if i == max {
					fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic(fmt.Sprintf("and %d more", len(items)-max)))
					break
				}
				fmt.Fprintln(b, s.closedLine(it, false))
			}
		}
	}

	// Clocked, by project.
	if r.ClockedMins > 0 {
		fmt.Fprintf(b, "\n%s %s\n", s.heading("⏱", "Clocked"), hm(r.ClockedMins))
		for i, g := range r.Clocked {
			if i == max {
				fmt.Fprintf(b, "%s%s\n", s.bullet(), s.italic(fmt.Sprintf("and %d more", len(r.Clocked)-max)))
				break
			}
			share := int(math.Round(100 * g.Mins / r.ClockedMins))
			fmt.Fprintf(b, "%s%s %s %s\n", s.bullet(), s.text(strings.TrimSuffix(g.Name, ".org")), hm(g.Mins),
				s.italic(fmt.Sprintf("%d%%", share)))
		}
	}

	// Writing.
	if w := r.Writing; w != nil && (w.Words != 0 || w.Goal > 0) {
		line := thousands(w.Words) + " words"
		if w.Goal > 0 {
			line += fmt.Sprintf(" of a %s %s goal", thousands(w.Goal), w.GoalKind)
		}
		fmt.Fprintf(b, "\n%s %s\n", s.heading("✍️", "Writing"), line)
		detail := plural(w.Days, "day", "days") + " with words"
		if w.GoalDays > 0 {
			detail += fmt.Sprintf(", %d met the daily goal", w.GoalDays)
		}
		fmt.Fprintf(b, "%s%s\n", s.bullet(), detail)
		for i, f := range w.Files {
			if i == 5 {
				break
			}
			fmt.Fprintf(b, "%s%s %s\n", s.bullet(), s.text(strings.TrimSuffix(filepath.Base(f.File), ".org")), thousands(f.Words))
		}
	}

	// Habits.
	if len(r.Habits) > 0 {
		fmt.Fprintf(b, "\n%s\n", s.heading("🔁", "Habits"))
		for _, h := range r.Habits {
			fmt.Fprintf(b, "%s%s %d/%d\n", s.bullet(), s.text(h.Headline), h.Done, h.Days)
		}
	}

	// In the way now.
	if len(r.Waiting)+len(r.Blocked) > 0 {
		fmt.Fprintf(b, "\n%s\n", s.heading("🚧", "Blocked"))
		s.blockers(r, b)
	}
	return b.String()
}

func init() {
	commands.AddCmd("standup", "what you did since the last working day, what is on today, what is in the way - Markdown for Slack",
		func() commands.Cmd { return &Standup{} })
	commands.AddCmd("recap", "a week or a month in review: done, clocked, written, habits, blocked - Markdown for Slack",
		func() commands.Cmd { return &Recap{} })
}

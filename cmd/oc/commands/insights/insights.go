package insights

// `orgs estimates` and `orgs deps`: worg's Insights tab in the terminal.
//
//	orgs estimates                      every finished task, grouped by tag
//	orgs estimates -by project -from -90d
//	orgs estimates -by person -block lastmonth -day 8
//	orgs estimates 'HasTags("work")'    only what a query finds
//	orgs deps                           every dependency problem
//	orgs deps -kind cycle -check        for a hook: exit 1 on an error
//
// Both are one request (/insights/estimates, /insights/deps); the numbers and
// the problems are worked out by the server, so the two clients cannot
// disagree. -json is the whole answer; -format runs once per group (estimates)
// or per problem (deps).

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// ---------------------------------------------------------------------------
// orgs estimates
// ---------------------------------------------------------------------------

type Estimates struct {
	By    string
	From  string
	To    string
	Block string
	Day   float64
	Open  bool
	Worst int
	fset  *flag.FlagSet
}

func (self *Estimates) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Estimates) StartPlugin(manager *common.PluginManager)       {}
func (self *Estimates) HelpGroup() string                                 { return "Finding things" }

func (self *Estimates) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.By, "by", "tag", "group by tag, project, file or person (ASSIGNEE)")
	fset.StringVar(&self.From, "from", "", "first day: 2026-07-01, -90d, mon")
	fset.StringVar(&self.To, "to", "", "last day, inclusive")
	fset.StringVar(&self.Block, "block", "", "a clock table block instead: thismonth, lastmonth, 2026-Q3 ...")
	fset.Float64Var(&self.Day, "day", 0, "hours in a day of EFFORT (org's is 24; 8 for a working day)")
	fset.BoolVar(&self.Open, "open", false, "also measure open tasks clocked in the range (marked ~)")
	fset.IntVar(&self.Worst, "n", 5, "how many of the worst misses to list each way")
}

func (self *Estimates) Exec(core *commands.Core) {
	q := map[string]string{"by": self.By}
	if s := commands.FreeText(self.fset); s != "" {
		q["query"] = s
	}
	for k, v := range map[string]string{"from": self.From, "to": self.To, "block": self.Block} {
		if v != "" {
			q[k] = v
		}
	}
	if self.Day > 0 {
		q["day"] = strconv.FormatFloat(self.Day, 'f', -1, 64)
	}
	if self.Open {
		q["open"] = "t"
	}
	res, err := commands.SendReceiveGetErr[common.EstimateReport](core, "insights/estimates", q)
	if err != nil {
		commands.Fail("estimates: %v", err)
	}
	if !res.Ok {
		commands.Fail("estimates: %s", res.Msg)
	}
	if commands.JsonOut {
		commands.RenderOne(res, nil)
		return
	}
	if commands.Render(res.Groups, nil) {
		return
	}
	printEstimates(res, self.Worst)
}

// Minutes as the clock reads them: 45m, 3h, 3h20m. Never days, which would
// have to say how long a day is.
func hm(mins float64) string {
	m := int(math.Round(mins))
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h, r := m/60, m%60
	if r == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%02dm", h, r)
}

func times(x float64) string {
	if x <= 0 {
		return "–"
	}
	return strconv.FormatFloat(math.Round(x*10)/10, 'f', 1, 64) + "×"
}

// Pad or cut to a width counted in runes, since titles are not ascii.
func fit(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n > w {
		r := []rune(s)
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-n)
}

// The bar worg's weekly review draws: estimate and actual against the biggest
// in the table. Solid up to whichever is smaller, then light for estimate not
// spent, or red for time over the estimate.
func bar(est, actual, most float64, width int) string {
	if most <= 0 {
		return strings.Repeat(" ", width)
	}
	cells := func(x float64) int { return int(math.Round(x / most * float64(width))) }
	within, rest := cells(math.Min(est, actual)), 0
	over := actual > est
	if over {
		rest = cells(actual) - within
	} else {
		rest = cells(est) - within
	}
	within = min(max(within, 0), width)
	rest = min(max(rest, 0), width-within)
	pad := width - within - rest
	out := commands.C(commands.AnsiBlue) + strings.Repeat("█", within) + commands.C(commands.AnsiReset)
	if over {
		out += commands.C(commands.AnsiRed) + strings.Repeat("█", rest) + commands.C(commands.AnsiReset)
	} else {
		out += commands.C(commands.AnsiDim) + strings.Repeat("░", rest) + commands.C(commands.AnsiReset)
	}
	if !commands.Colour() && over {
		// Without colour the over-run must still look different.
		out = strings.Repeat("█", within) + strings.Repeat("▓", rest)
	}
	return out + strings.Repeat(" ", pad)
}

func ratioColour(r float64) string {
	switch {
	case r <= 0:
		return ""
	case r > 1.25:
		return commands.AnsiRed
	case r < 0.8:
		return commands.AnsiCyan
	}
	return commands.AnsiGreen
}

func where(filename string, line int) string {
	return fmt.Sprintf("%s:%d", filepath.Base(filename), line+1)
}

func printEstimates(res common.EstimateReport, worst int) {
	b, d, r := commands.C(commands.AnsiBold), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	day := "a day of effort is 24h"
	if res.DayHours != 24 {
		day = fmt.Sprintf("a day of effort is %sh", strconv.FormatFloat(res.DayHours, 'f', -1, 64))
	}
	fmt.Printf("%sEstimates%s %s· %s · by %s · %s%s\n", b, r, d, res.Label, res.By, day, r)
	if res.All.Tasks == 0 {
		fmt.Println("\nNothing measured: no finished task in this range has both an EFFORT and clocked time.")
		printUnmeasured(res)
		return
	}
	fmt.Println()
	most := 0.0
	for _, g := range res.Groups {
		most = math.Max(most, math.Max(g.EstMins, g.ActualMins))
	}
	nameW := 10
	for _, g := range res.Groups {
		nameW = int(math.Max(float64(nameW), float64(utf8.RuneCountInString(groupName(g)))))
	}
	if nameW > 28 {
		nameW = 28
	}
	fmt.Printf("  %s%s %5s %9s %9s %6s %8s  %s%s\n", d, fit("", nameW), "tasks", "estimate", "actual", "ratio", "median", "spread", r)
	row := func(g common.EstimateGroup, name string, most float64) {
		open := ""
		if g.InProgress > 0 {
			open = fmt.Sprintf(" %s~%d%s", d, g.InProgress, r)
		}
		spread := ""
		if g.Tasks >= 2 {
			spread = times(g.P25) + "–" + times(g.P75)
		}
		fmt.Printf("  %s %5d %9s %9s %s%6s%s %8s  %-12s %s%s\n", fit(name, nameW), g.Tasks, hm(g.EstMins), hm(g.ActualMins),
			commands.C(ratioColour(g.Ratio)), times(g.Ratio), r, times(g.Median), spread, bar(g.EstMins, g.ActualMins, most, 20), open)
	}
	for _, g := range res.Groups {
		row(g, groupName(g), most)
	}
	if len(res.Groups) > 1 {
		fmt.Printf("  %s\n", d+strings.Repeat("─", nameW+56)+r)
		// Against itself: the total is bigger than any one row.
		row(res.All, "all", math.Max(res.All.EstMins, res.All.ActualMins))
	}
	fmt.Println()
	// The sentences: everything, then each group with enough tasks to say.
	if res.All.Sentence != "" {
		fmt.Printf("  %s%s%s\n", b, res.All.Sentence, r)
	}
	for _, g := range res.Groups {
		// Only the groups worth acting on: "about right" five times is noise.
		if g.Multiplier > 0 && (g.Multiplier < 0.9 || g.Multiplier > 1.1) && len(res.Groups) > 1 {
			fmt.Printf("  %s\n", g.Sentence)
		}
	}
	list := func(title string, tasks []common.EstimateTask) {
		if len(tasks) == 0 || worst <= 0 {
			return
		}
		fmt.Printf("\n%s%s%s\n", b, title, r)
		for i, t := range tasks {
			if i >= worst {
				break
			}
			mark := " "
			if t.InProgress {
				mark = "~"
			}
			fmt.Printf("  %s%s%6s%s %s %8s → %-8s %s  %s%s%s\n", mark, commands.C(ratioColour(t.Ratio)), times(t.Ratio), r,
				d+"·"+r, hm(t.EstMins), hm(t.ActualMins), fit(t.Headline, 40), d, where(t.Filename, t.LineNum), r)
		}
	}
	list("Worst over-runs", res.Over)
	list("Biggest under-runs", res.Under)
	printUnmeasured(res)
}

func groupName(g common.EstimateGroup) string {
	if g.Kind == "tag" && !strings.HasPrefix(g.Name, "(") {
		return ":" + g.Name + ":"
	}
	return g.Name
}

func printUnmeasured(res common.EstimateReport) {
	d, r := commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	parts := []string{}
	if res.ClockNoEffort > 0 {
		parts = append(parts, fmt.Sprintf("%d clocked with no estimate (%s)", res.ClockNoEffort, hm(res.ClockNoEffortMins)))
	}
	if res.EffortNoClock > 0 {
		parts = append(parts, fmt.Sprintf("%d finished with an estimate and no clock", res.EffortNoClock))
	}
	if res.Rollups > 0 {
		parts = append(parts, fmt.Sprintf("%d rollup%s (an EFFORT over estimated subtasks)", res.Rollups, plural(res.Rollups)))
	}
	if len(parts) > 0 {
		fmt.Printf("\n%sNot measured: %s.%s\n", d, strings.Join(parts, "; "), r)
	}
	if res.DayEfforts > 0 && res.DayHours == 24 {
		fmt.Printf("%s%d estimate%s written in days count 24h a day; -day 8 reads them as working days.%s\n", d, res.DayEfforts, plural(res.DayEfforts), r)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ---------------------------------------------------------------------------
// orgs deps
// ---------------------------------------------------------------------------

type Deps struct {
	Min   int
	Kind  string
	Check bool
	fset  *flag.FlagSet
}

func (self *Deps) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Deps) StartPlugin(manager *common.PluginManager)       {}
func (self *Deps) HelpGroup() string                                 { return "Finding things" }

func (self *Deps) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.IntVar(&self.Min, "min", 3, "shortest blocked chain worth reporting")
	fset.StringVar(&self.Kind, "kind", "", "only these kinds: cycle, waiting-on-done, blocked-chain, dangling, ordered-stuck (comma separated)")
	fset.BoolVar(&self.Check, "check", false, "exit 1 when there is an error (a cycle), for a hook or CI")
}

var kindLabel = map[string]string{
	"cycle":           "cycle",
	"waiting-on-done": "waiting on done",
	"blocked-chain":   "blocked chain",
	"dangling":        "dangling AFTER",
	"ordered-stuck":   "ORDERED stuck",
}

func (self *Deps) Exec(core *commands.Core) {
	q := map[string]string{"min": strconv.Itoa(self.Min)}
	if s := commands.FreeText(self.fset); s != "" {
		q["query"] = s
	}
	res, err := commands.SendReceiveGetErr[common.DepReport](core, "insights/deps", q)
	if err != nil {
		commands.Fail("deps: %v", err)
	}
	if !res.Ok {
		commands.Fail("deps: %s", res.Msg)
	}
	if self.Kind != "" {
		want := map[string]bool{}
		for _, k := range strings.Split(self.Kind, ",") {
			want[strings.TrimSpace(k)] = true
		}
		kept := []common.DepProblem{}
		for _, p := range res.Problems {
			if want[p.Kind] {
				kept = append(kept, p)
			}
		}
		res.Problems = kept
	}
	errors := 0
	for _, p := range res.Problems {
		if p.Severity == "error" {
			errors++
		}
	}
	if commands.JsonOut {
		commands.RenderOne(res, nil)
	} else if !commands.Render(res.Problems, nil) {
		printDeps(res)
	}
	if self.Check && errors > 0 {
		os.Exit(1)
	}
}

func severityMark(s string) string {
	switch s {
	case "error":
		return commands.C(commands.AnsiRed) + "✖" + commands.C(commands.AnsiReset)
	case "warn":
		return commands.C(commands.AnsiGold) + "▲" + commands.C(commands.AnsiReset)
	}
	return commands.C(commands.AnsiCyan) + "●" + commands.C(commands.AnsiReset)
}

// A chain drawn A → B → C, each heading coloured by where it stands: done
// dimmed, waiting in gold, the rest plain. The link into a heading says how it
// waits when it is not an AFTER (ORDERED, the quieter of the two).
func drawChain(chain []common.DepHeading) string {
	d, r := commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	out := []string{}
	for i, h := range chain {
		name := h.Headline
		switch {
		case h.Done:
			name = d + name + " ✓" + r
		case isWaiting(h.Status):
			name = commands.C(commands.AnsiGold) + name + r
		}
		if i > 0 {
			arrow := " → "
			if h.Via == "ORDERED" {
				arrow = " ⇢ "
			}
			out = append(out, d+arrow+r)
		}
		out = append(out, name)
	}
	return strings.Join(out, "")
}

func isWaiting(status string) bool {
	switch strings.ToUpper(status) {
	case "WAITING", "WAIT", "BLOCKED", "HOLD", "ON-HOLD", "ONHOLD", "DEFERRED", "PENDING":
		return true
	}
	return false
}

func printDeps(res common.DepReport) {
	b, d, r := commands.C(commands.AnsiBold), commands.C(commands.AnsiDim), commands.C(commands.AnsiReset)
	if len(res.Problems) == 0 {
		fmt.Printf("%s✓%s No dependency problems among %d headings with keywords or links.\n", commands.C(commands.AnsiGreen), r, res.Checked)
		return
	}
	for i, p := range res.Problems {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%s %s%-15s%s %s%s%s\n", severityMark(p.Severity), d, kindLabel[p.Kind], r, b, p.Title, r)
		if len(p.Chain) > 1 || p.Kind == "dangling" {
			chain := drawChain(p.Chain)
			if p.Kind == "dangling" && p.Missing != "" && len(p.Chain) == 1 {
				chain = commands.C(commands.AnsiRed) + p.Missing + " ?" + r + d + " → " + r + chain
			}
			fmt.Printf("  %s\n", chain)
		}
		fmt.Printf("  %s%s%s\n", d, p.Suggestion, r)
		// Where to go: the headings, once each, as file:line an editor reads.
		seen := map[string]bool{}
		locs := []string{}
		for _, h := range p.Chain {
			k := h.Filename + ":" + strconv.Itoa(h.LineNum)
			if seen[k] {
				continue
			}
			seen[k] = true
			title := h.Headline
			if utf8.RuneCountInString(title) > 28 {
				title = fit(title, 28)
			}
			locs = append(locs, where(h.Filename, h.LineNum)+" "+title)
		}
		fmt.Printf("  %s%s%s\n", d, strings.Join(locs, " · "), r)
	}
	counts := []string{}
	for _, k := range []string{"cycle", "waiting-on-done", "blocked-chain", "dangling", "ordered-stuck"} {
		if n := countKind(res.Problems, k); n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, kindLabel[k]))
		}
	}
	fmt.Printf("\n%s%d problem%s: %s. %d headings checked.%s\n", d, len(res.Problems), plural(len(res.Problems)), strings.Join(counts, ", "), res.Checked, r)
}

func countKind(ps []common.DepProblem, kind string) int {
	n := 0
	for _, p := range ps {
		if p.Kind == kind {
			n++
		}
	}
	return n
}

func init() {
	commands.AddCmd("estimates", "effort vs actual: how far EFFORT estimates are off, per tag, project, file or person",
		func() commands.Cmd {
			return &Estimates{}
		})
	commands.AddCmd("deps", "dependency health: cycles, blocked chains, waiting on done, dangling AFTER, stuck ORDERED projects",
		func() commands.Cmd {
			return &Deps{}
		})
}

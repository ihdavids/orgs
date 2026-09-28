package review

// orgs review - the weekly review, as questions the database can answer.
// orgs stats  - what is in there, counted.
//
//	orgs review                 everything that wants a decision
//	orgs review stale           just that check
//	orgs review -fix            walk them, and change them as you go
//	orgs stats                  keywords, tags, files, clocked time
//	orgs stats -by tag
//
// Nothing here is a new capability: every check is an expression the query
// language could already evaluate. What was missing is that nobody writes those
// nine queries out on a Friday afternoon, and a system nobody reviews is a
// system nobody trusts - which is the actual failure mode of a personal org
// database, not any missing feature.
//
// So the checks are named, and each one says what it found *and what to do about
// it*. Two rules:
//
//  1. **A check is a query and a sentence, nothing else.** No check may need
//     code of its own, because a check that needed code would be a second
//     implementation of the query language and would drift from it.
//  2. **A check that finds nothing prints one green line, not silence.** "No
//     projects without a next action" is the most valuable output this command
//     has, and dropping it would leave a review that only ever nags.

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

// A check: a name to ask for it by, a line saying what it means, the query, and
// what to do about a hit.
type check struct {
	Name  string
	What  string
	Query string
	Fix   string
}

// The checks, in the order a review wants them: the things that block other
// things first, the tidying last.
var checks = []check{
	{
		"blocked-projects",
		"projects with nothing to do next",
		"IsBlockedProject() && !IsArchived()",
		"give it a NEXT, or drop the project",
	},
	{
		"stale",
		"NEXT actions nobody has touched in a month",
		`IsStatus("NEXT") && !IsArchived() && OlderThan(30)`,
		"do it, reschedule it, or admit it is not next",
	},
	{
		"no-date",
		"tasks with a deadline nobody has scheduled",
		"IsTask() && IsTodo() && !IsArchived() && HasDeadline() && !HasScheduled()",
		"orgs sched <when>",
	},
	{
		"overdue",
		"deadlines that have gone past",
		"IsTodo() && !IsArchived() && DeadlinePast()",
		"orgs deadline <when>, or finish it",
	},
	{
		"waiting",
		"things waiting on somebody else",
		`IsTodo() && !IsArchived() && (IsStatus("WAITING") || IsStatus("BLOCKED"))`,
		"chase it, or write down what it is waiting for",
	},
	{
		"finished-lists",
		"every box ticked and the keyword never moved",
		"IsTodo() && !IsArchived() && ChecklistDone()",
		"orgs todo DONE",
	},
	{
		"clocked-open",
		"time booked against something still open",
		"IsTodo() && !IsArchived() && HasClock()",
		"finish it, or clock the rest somewhere honest",
	},
	{
		"broken-links",
		"links that point at nothing",
		"HasBrokenLinks() && !IsArchived()",
		"fix the target, or take the link out",
	},
	{
		"untagged",
		"tasks with no tags at all",
		"IsTask() && IsTodo() && !IsArchived() && NoTags()",
		"orgs tag +something",
	},
	{
		"orphans",
		"notes nothing links to",
		"!IsTodo() && !IsArchived() && !IsRecord() && !HasBacklinks() && !HasLinks()",
		"link it from somewhere, or let it go",
	},
}

type Review struct {
	Fix   bool
	Only  string
	Limit int
}

func (self *Review) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Review) StartPlugin(m *common.PluginManager)       {}

func (self *Review) SetupParameters(fset *flag.FlagSet) {
	fset.BoolVar(&self.Fix, "fix", false, "walk the findings and change them as you go")
	fset.IntVar(&self.Limit, "limit", 8, "at most this many headings per check; 0 for all")
}

// One check's findings, which is also what -json prints.
type Finding struct {
	Check    string
	What     string
	Fix      string
	Query    string
	Count    int
	Headings []common.Todo `json:",omitempty"`
	// A check whose query this server cannot evaluate. Said out loud rather
	// than counted as zero: "nothing is stale" and "this server has no
	// OnDate()" must not look the same.
	Unsupported bool `json:",omitempty"`
}

func (self *Review) Exec(core *commands.Core) {
	words := commands.FreeArgs(commands.Find("review").Flags)
	want := map[string]bool{}
	for _, w := range words {
		want[w] = true
	}
	if len(want) > 0 {
		for _, w := range words {
			if !known(w) {
				commands.Fail("no check called %q - there is %s", w, strings.Join(names(), ", "))
			}
		}
	}

	findings := []Finding{}
	for _, c := range checks {
		if len(want) > 0 && !want[c.Name] {
			continue
		}
		f := Finding{Check: c.Name, What: c.What, Fix: c.Fix, Query: c.Query}
		todos, err := commands.SendReceiveGetErr[common.Todos](core, "search",
			map[string]string{"query": c.Query})
		if err != nil {
			f.Unsupported = true
			findings = append(findings, f)
			continue
		}
		f.Count = len(todos)
		f.Headings = todos
		findings = append(findings, f)
	}

	if commands.JsonOut || commands.FormatOut != "" {
		commands.Render(findings, nil)
		return
	}
	self.print(core, findings)
}

func (self *Review) print(core *commands.Core, findings []Finding) {
	total := 0
	for _, f := range findings {
		total += f.Count
	}

	for _, f := range findings {
		switch {
		case f.Unsupported:
			fmt.Printf("%s?%s %s %s(this server cannot evaluate %s)%s\n",
				commands.C(commands.AnsiGold), commands.C(commands.AnsiReset), f.What,
				commands.C(commands.AnsiDim), f.Query, commands.C(commands.AnsiReset))
			continue
		case f.Count == 0:
			// The most valuable line this command prints.
			fmt.Printf("%s✓%s %s\n", commands.C(commands.AnsiGreen),
				commands.C(commands.AnsiReset), f.What)
			continue
		}
		fmt.Printf("\n%s%s %d%s %s  %s%s%s\n", commands.C(commands.AnsiGold), "▸", f.Count,
			commands.C(commands.AnsiReset), f.What,
			commands.C(commands.AnsiDim), f.Fix, commands.C(commands.AnsiReset))
		shown := f.Headings
		if self.Limit > 0 && len(shown) > self.Limit {
			shown = shown[:self.Limit]
		}
		for _, t := range shown {
			fmt.Printf("    %s\n", line(t))
		}
		if len(shown) < len(f.Headings) {
			// Never a silent cap: a listing that stopped and did not say so
			// reads as though it covered everything.
			fmt.Printf("    %s… and %d more (-limit 0 for all, orgs review %s to see them)%s\n",
				commands.C(commands.AnsiDim), len(f.Headings)-len(shown), f.Check,
				commands.C(commands.AnsiReset))
		}
	}

	fmt.Println()
	if total == 0 {
		fmt.Printf("%sNothing wants a decision. %d checks.%s\n", commands.C(commands.AnsiGreen),
			len(findings), commands.C(commands.AnsiReset))
		return
	}
	fmt.Printf("%s%d headings across %d checks.%s\n", commands.C(commands.AnsiDim), total,
		len(findings), commands.C(commands.AnsiReset))
	if !self.Fix {
		fmt.Printf("%sorgs review -fix to walk them, or orgs review <check> for one of them.%s\n",
			commands.C(commands.AnsiDim), commands.C(commands.AnsiReset))
		return
	}
	self.walk(core, findings)
}

// walk is -fix: each finding's headings go to the picker, and what is chosen is
// opened in the editor. Deliberately not a menu of automatic repairs - "this
// project has no next action" is answered by *deciding something*, and a tool
// that offered to decide it for you would be making the review worthless.
func (self *Review) walk(core *commands.Core, findings []Finding) {
	if !commands.Interactive() {
		commands.Fail("-fix needs a terminal")
	}
	for _, f := range findings {
		if f.Count == 0 || f.Unsupported {
			continue
		}
		lines := []string{}
		for i, t := range f.Headings {
			lines = append(lines, commands.PickLine([]string{fmt.Sprintf("%d", i), t.Hash}, line(t)))
		}
		preview := ""
		if self_, err := commands.SelfCommand(core); err == nil {
			preview = self_ + " show -hash {2} -pane"
		}
		sel := commands.Pick(commands.PickOpts{
			Lines:         lines,
			AddressFields: 2,
			Prompt:        f.Check + "> ",
			Header:        f.What + "  ·  " + f.Fix + "  ·  enter opens it, esc skips this check",
			Preview:       preview,
			Extra:         []string{"--multi"},
		})
		for _, s := range sel {
			addr, ok := commands.Address(s, 2)
			if !ok {
				continue
			}
			var i int
			if _, err := fmt.Sscanf(addr[0], "%d", &i); err != nil || i >= len(f.Headings) {
				continue
			}
			t := f.Headings[i]
			core.LaunchEditor(t.Filename, t.LineNum)
		}
	}
}

func line(t common.Todo) string {
	kw := ""
	if t.Status != "" {
		kw = commands.C(commands.AnsiGold) + t.Status + commands.C(commands.AnsiReset) + " "
	}
	return fmt.Sprintf("%s%s  %s%s:%d%s", kw, commands.Ellipsis(t.Headline, 60),
		commands.C(commands.AnsiDim), commands.BaseName(t.Filename), t.LineNum,
		commands.C(commands.AnsiReset))
}

func known(name string) bool {
	for _, c := range checks {
		if c.Name == name {
			return true
		}
	}
	return false
}

func names() []string {
	out := []string{}
	for _, c := range checks {
		out = append(out, c.Name)
	}
	return out
}

// ---------------------------------------------------------------------------
// orgs stats
// ---------------------------------------------------------------------------

type Stats struct {
	By string
}

func (self *Stats) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *Stats) StartPlugin(m *common.PluginManager)       {}

func (self *Stats) SetupParameters(fset *flag.FlagSet) {
	fset.StringVar(&self.By, "by", "status", "count by: status, tag, file, priority, level")
}

type statRow struct {
	Key   string
	Count int
}

func (self *Stats) Exec(core *commands.Core) {
	// Everything at once, rather than a query per group: the grouping is
	// counting a list this side, and one request is one request.
	todos, err := commands.SendReceiveGetErr[common.Todos](core, "search",
		map[string]string{"query": "true"})
	if err != nil {
		commands.Fail("orgs stats: %v", err)
	}
	if len(todos) == 0 {
		commands.Fail("nothing in the database")
	}

	counts := map[string]int{}
	for _, t := range todos {
		switch strings.ToLower(self.By) {
		case "tag":
			if len(t.Tags) == 0 {
				counts["(untagged)"]++
			}
			for _, tag := range t.Tags {
				counts[tag]++
			}
		case "file":
			counts[commands.BaseName(t.Filename)]++
		case "priority":
			k := t.Priority
			if k == "" {
				k = "(none)"
			}
			counts[k]++
		case "level":
			counts[strings.Repeat("*", maxi(t.Level, 1))]++
		default:
			k := t.Status
			if k == "" {
				k = "(no keyword)"
			}
			counts[k]++
		}
	}

	rows := []statRow{}
	for k, n := range counts {
		rows = append(rows, statRow{Key: k, Count: n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].Count > rows[j].Count
	})

	commands.Render(rows, func() {
		width, most := 0, 0
		for _, r := range rows {
			if len(r.Key) > width {
				width = len(r.Key)
			}
			if r.Count > most {
				most = r.Count
			}
		}
		if width > 28 {
			width = 28
		}
		for _, r := range rows {
			// A bar, because the shape of the answer is the answer: "most of
			// what I have is BACKLOG" is a sentence a column of numbers makes
			// you work out.
			bar := ""
			if most > 0 {
				n := r.Count * 28 / most
				if n == 0 && r.Count > 0 {
					n = 1
				}
				bar = strings.Repeat("▄", n)
			}
			fmt.Printf("  %s%-*s%s %s%5d%s  %s%s%s\n", commands.C(commands.AnsiBold), width,
				commands.Ellipsis(r.Key, width), commands.C(commands.AnsiReset),
				commands.C(commands.AnsiDim), r.Count, commands.C(commands.AnsiReset),
				commands.C(commands.AnsiCyan), bar, commands.C(commands.AnsiReset))
		}
		fmt.Printf("\n  %s%d headings, by %s%s\n", commands.C(commands.AnsiDim), len(todos),
			strings.ToLower(self.By), commands.C(commands.AnsiReset))
		fmt.Fprint(os.Stderr, "")
	})
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	commands.AddCmd("review", "the weekly review - everything that wants a decision",
		func() commands.Cmd { return &Review{} })
	commands.AddCmd("stats", "what is in the database, counted",
		func() commands.Cmd { return &Stats{} })
}

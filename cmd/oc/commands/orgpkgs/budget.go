package orgpkgs

// Time budgets (org-clock-budget), from the command line.
//
//	orgs budget                          every budget, this day/week/month/quarter/year
//	orgs budget -period week             one period (-date 2026-10-15 for another week)
//	orgs budget clocked                  what the running clock is spending
//	orgs budget set 'Writing' week 5:00  give a heading a budget (off takes it away)
//
// Needs `orgs features clockBudget on`.

import (
	"flag"
	"fmt"
	"math"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Budget struct {
	base
	tf     commands.TargetFlags
	Period string
	Date   string
	Over   bool
	Width  int
}

func (self *Budget) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	commands.AddTargetFlags(fset, &self.tf)
	fset.StringVar(&self.Period, "period", "", "day, week, month, quarter, year, a list (week,month) or all")
	fset.StringVar(&self.Date, "date", "", "the periods around another day: 2026-10-15, -1w, last mon")
	fset.BoolVar(&self.Over, "over", false, "only budgets that are over")
	fset.IntVar(&self.Width, "width", 20, "how wide the bars are")
}

// budgetHM is H:MM, minus sign and all.
func budgetHM(mins float64) string {
	sign := ""
	if mins < 0 {
		sign, mins = "-", -mins
	}
	m := int(mins + 0.5)
	return fmt.Sprintf("%s%d:%02d", sign, m/60, m%60)
}

// budgetBar is the used part filled, a mark where the pace is, red when over
// and gold when spending ahead of the period.
func budgetBar(it common.BudgetItem, width int) string {
	if width < 4 {
		width = 4
	}
	fill := int(math.Round(math.Min(it.Used/it.Budget, 1) * float64(width)))
	pace := int(math.Round(it.Pace * float64(width)))
	if pace >= width {
		pace = width - 1
	}
	colour := commands.AnsiGreen
	switch {
	case it.Over:
		colour = commands.AnsiRed
	case it.Ahead:
		colour = commands.AnsiGold
	}
	var b strings.Builder
	b.WriteString(commands.C(colour))
	for i := 0; i < width; i++ {
		switch {
		case i == pace && it.Pace > 0 && it.Pace < 1:
			if i < fill {
				b.WriteString("┃")
			} else {
				b.WriteString(commands.C(commands.AnsiReset) + "│" + commands.C(colour))
			}
		case i < fill:
			b.WriteString("█")
		default:
			b.WriteString(commands.C(commands.AnsiDim) + "░" + commands.C(commands.AnsiReset) + commands.C(colour))
		}
	}
	b.WriteString(commands.C(commands.AnsiReset))
	return b.String()
}

func budgetLine(it common.BudgetItem, width int) string {
	state := ""
	switch {
	case it.Over:
		state = commands.C(commands.AnsiRed) + budgetHM(-it.Remaining) + " over" + commands.C(commands.AnsiReset)
	default:
		state = dim(budgetHM(it.Remaining) + " left")
	}
	run := ""
	if it.Running {
		run = commands.C(commands.AnsiCyan) + " ⏱" + commands.C(commands.AnsiReset)
	}
	return fmt.Sprintf("  %s %5s / %-5s %4.0f%%  %s%s %s", budgetBar(it, width), budgetHM(it.Used), budgetHM(it.Budget),
		it.Percent, commands.Ellipsis(it.Headline, 50), run, state)
}

func (self *Budget) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	verb := ""
	if len(words) > 0 {
		verb = words[0]
	}
	switch verb {
	case "set":
		self.set(core, words[1:])
		return
	case "clocked", "clock", "now":
		self.clocked(core)
		return
	case "", "ls", "list":
	default:
		commands.Fail("orgs budget [-period week] | clocked | set [HEADING] PERIOD H:MM")
	}
	ps := map[string]string{}
	if self.Period != "" {
		ps["period"] = self.Period
	}
	if self.Date != "" {
		ps["date"] = self.Date
	}
	res, err := commands.SendReceiveGetErr[common.BudgetReport](core, "budgets", ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	items := res.Items
	if self.Over {
		kept := []common.BudgetItem{}
		for _, it := range items {
			if it.Over {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	commands.Render(items, func() {
		if len(items) == 0 {
			if self.Over {
				fmt.Println("Nothing is over budget.")
			} else {
				fmt.Println("No headings have a budget. Give one with: orgs budget set HEADING week 5:00")
			}
			return
		}
		for _, p := range res.Periods {
			shown := 0
			for _, it := range items {
				if it.Period == p.Name {
					shown++
				}
			}
			if shown == 0 {
				continue
			}
			// The label already starts with the period's name ("week 2026-W45").
			label := p.Label
			if label == "" {
				label = p.Name
			}
			fmt.Printf("%s%s%s %s\n", commands.C(commands.AnsiBold), strings.ToUpper(label[:1])+label[1:], commands.C(commands.AnsiReset),
				dim(fmt.Sprintf("· %.0f%% through · %s budgeted of %s budgetable, %s clocked",
					p.Pace*100, budgetHM(p.Budgeted), budgetHM(p.Budgetable), budgetHM(p.Clocked))))
			for _, it := range items {
				if it.Period == p.Name {
					fmt.Println(budgetLine(it, self.Width))
				}
			}
		}
	})
}

func (self *Budget) clocked(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[common.BudgetClocked](core, "budgets/clocked", nil)
	if err != nil {
		commands.Fail("%v", err)
	}
	if !res.Ok {
		commands.Fail("%s", res.Msg)
	}
	commands.RenderOne(res, func() {
		if !res.Active {
			fmt.Println("No clock is running.")
			return
		}
		fmt.Printf("%s⏱%s %s\n", commands.C(commands.AnsiCyan), commands.C(commands.AnsiReset), res.Headline)
		if len(res.Items) == 0 {
			fmt.Println(dim("  no budget above it"))
		}
		for _, it := range res.Items {
			fmt.Printf("%s\n", budgetLine(it, self.Width)+" "+dim(budgetPeriodName(it.Period)))
		}
	})
}

func budgetPeriodName(p string) string {
	if p == "day" {
		return "today"
	}
	return "this " + p
}

// set: the last word is the budget, the one before it the period, and any
// words before those name the heading (a query or a hash; a picker without).
func (self *Budget) set(core *commands.Core, words []string) {
	if len(words) < 2 {
		commands.Fail("orgs budget set [HEADING] PERIOD H:MM   (PERIOD day|week|month|quarter|year; H:MM off to take it away)")
	}
	period, value := words[len(words)-2], words[len(words)-1]
	switch strings.ToLower(value) {
	case "off", "none", "-", "rm", "remove", "nil":
		value = ""
	}
	todos := commands.Resolve(core, &self.tf, words[:len(words)-2], commands.TargetOpts{Prompt: "Budget for> ", Quiet: true})
	for _, t := range todos {
		req := common.BudgetSet{Hash: t.Hash, Period: period, Value: value}
		reply, err := commands.SendReceivePostErr[common.BudgetSet, common.Result](core, "budgets/set", &req)
		if err == commands.ErrDryRun {
			return
		}
		if err != nil {
			commands.Fail("%v", err)
		}
		if !reply.Ok {
			commands.Fail("%s", reply.Msg)
		}
		commands.Ok(1, "budgeted", []string{commands.Describe(t) + " " + dim(reply.Msg)})
	}
}

func init() {
	commands.AddCmd("budget", "time budgets: clocked time against :BUDGET_WEEK: and kin (org-clock-budget)",
		func() commands.Cmd { return &Budget{} })
}

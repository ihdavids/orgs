package orgpkgs

// ledger-mode / ob-ledger from the command line (features.ledger).
//
//	orgs ledger                                  balance of every source, as ledger bal prints it
//	orgs ledger bal -source money.org -depth 2   one source, two levels deep
//	orgs ledger reg ^Assets:Bank -period 'last month'
//	orgs ledger monthly Expenses -depth 2 -period 'this year'
//	orgs ledger check                            problems, file:line; exits 1 on an error
//	orgs ledger sources                          the journals there are
//	orgs ledger -f money.ledger bal              read a file here, no server needed
//
// Words after the report are account regular expressions (any may match;
// `not X` leaves X out). -block reads one source block alone, as ob-ledger
// runs it.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common/ledger"
)

type Ledger struct {
	base
	Source string
	Block  string
	Period string
	Begin  string
	End    string
	Depth  int
	Real   bool
	Flat   bool
	File   string
}

func (self *Ledger) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Source, "source", "", "a source: money.org, books.ledger (orgs ledger sources lists them); every one when not given")
	fset.StringVar(&self.Block, "block", "", "one ledger block of an org source alone: its index from 0, its #+NAME: or line:N")
	fset.StringVar(&self.Period, "period", "", "the period: this month, last year, 2026, 2026-11, q3, from 2026-01-01 to 2026-04-01")
	fset.StringVar(&self.Period, "p", "", "the period (as -period)")
	fset.StringVar(&self.Begin, "begin", "", "from this day (or month, year)")
	fset.StringVar(&self.End, "end", "", "before this day (or month, year)")
	fset.IntVar(&self.Depth, "depth", 0, "fold accounts below this depth into their parent")
	fset.BoolVar(&self.Real, "real", false, "only real postings, no virtual ones")
	fset.BoolVar(&self.Flat, "flat", false, "bal: full account names, no tree")
	fset.StringVar(&self.File, "f", "", "read this journal or org file here instead of asking the server")
}

type ledgerAnswer struct {
	Ok           bool
	Off          bool
	Msg          string
	Source       string
	Period       string
	Text         string
	Problems     []ledger.Problem
	Rows         []map[string]interface{}
	Total        []ledger.Amt
	Months       []string
	Series       []ledger.Series
	Sources      []map[string]interface{}
	Transactions int
}

func (self *Ledger) Exec(core *commands.Core) {
	words := commands.FreeArgs(self.fset)
	report := "bal"
	if len(words) > 0 {
		switch strings.ToLower(words[0]) {
		case "bal", "balance", "b":
			report, words = "balance", words[1:]
		case "reg", "register", "r":
			report, words = "register", words[1:]
		case "monthly", "month", "m":
			report, words = "monthly", words[1:]
		case "check", "sources":
			report, words = strings.ToLower(words[0]), words[1:]
		default:
			report = "balance"
		}
	} else {
		report = "balance"
	}
	if self.File != "" {
		self.local(report, words)
		return
	}
	ps := map[string]string{"source": self.Source, "block": self.Block, "period": self.Period,
		"begin": self.Begin, "end": self.End, "account": strings.Join(words, " ")}
	if self.Depth > 0 {
		ps["depth"] = strconv.Itoa(self.Depth)
	}
	if self.Real {
		ps["real"] = "t"
	}
	if self.Flat {
		ps["flat"] = "t"
	}
	for k, v := range ps {
		if v == "" {
			delete(ps, k)
		}
	}
	res, err := commands.SendReceiveGetErr[ledgerAnswer](core, "ledger/"+report, ps)
	if err != nil {
		commands.Fail("%v", err)
	}
	failIfOff(offAnswer{res.Ok, res.Off, res.Msg})
	switch report {
	case "sources":
		commands.Render(res.Sources, func() {
			for _, s := range res.Sources {
				line := fmt.Sprintf("%-30v %5v transactions", s["Name"], s["Transactions"])
				if f, l := s["First"], s["Last"]; f != nil {
					line += dim(fmt.Sprintf("  %v .. %v", f, l))
				}
				if n, _ := s["Errors"].(float64); n > 0 {
					line += commands.C(commands.AnsiRed) + fmt.Sprintf("  %v errors", n) + commands.C(commands.AnsiReset)
				}
				if b, ok := s["Blocks"].([]interface{}); ok && len(b) > 0 {
					line += dim(fmt.Sprintf("  %d blocks", len(b)))
				}
				if inc, _ := s["IncludedBy"].(string); inc != "" {
					line += dim("  included by " + inc)
				}
				fmt.Println(line)
			}
		})
	case "check":
		showProblems(res.Problems, res.Transactions)
		exitOnErrors(res.Problems)
	case "monthly":
		commands.RenderOne(struct {
			Months []string
			Series []ledger.Series
		}{res.Months, res.Series}, func() {
			fmt.Print(res.Text)
			warnProblems(res.Problems)
		})
	default:
		commands.Render(res.Rows, func() {
			fmt.Print(res.Text)
			warnProblems(res.Problems)
		})
	}
}

// showProblems is check's answer: -format quickfix as file:line:col, -json
// the problems, else one coloured line each.
func showProblems(ps []ledger.Problem, txs int) {
	diags := []commands.Diagnostic{}
	for _, p := range ps {
		diags = append(diags, commands.Diagnostic{File: p.File, Line: p.Line, Col: 1, Severity: p.Severity, Rule: "ledger", Msg: p.Msg})
	}
	if commands.QuickfixFormat() {
		commands.RenderDiagnostics(diags, nil)
		return
	}
	commands.Render(ps, func() { printProblems(ps, txs) })
}

func printProblems(ps []ledger.Problem, txs int) {
	if len(ps) == 0 {
		fmt.Printf("%d transactions, no problems\n", txs)
		return
	}
	for _, p := range ps {
		col := commands.AnsiGold
		if p.Severity == "error" {
			col = commands.AnsiRed
		}
		fmt.Printf("%s:%d: %s%s%s: %s\n", p.File, p.Line, commands.C(col), p.Severity, commands.C(commands.AnsiReset), p.Msg)
	}
}

// Problems under a report go to stderr: the report is the answer.
func warnProblems(ps []ledger.Problem) {
	errs := 0
	for _, p := range ps {
		if p.Severity == "error" {
			errs++
		}
	}
	if errs > 0 {
		fmt.Fprintf(os.Stderr, "%d error%s in the journal (orgs ledger check)\n", errs, map[bool]string{true: "", false: "s"}[errs == 1])
	}
}

func exitOnErrors(ps []ledger.Problem) {
	for _, p := range ps {
		if p.Severity == "error" {
			os.Exit(1)
		}
	}
}

// local reads a file here, with no server: a journal, or an org file's
// ledger blocks. Includes are read beside it, never above its folder.
func (self *Ledger) local(report string, words []string) {
	abs, err := filepath.Abs(self.File)
	if err != nil {
		commands.Fail("%v", err)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		commands.Fail("%v", err)
	}
	root := filepath.Dir(abs)
	opt := ledger.Options{Now: time.Now(), Open: func(p string) (string, error) {
		if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("%s is outside %s", p, root)
		}
		t, err := os.ReadFile(p)
		return string(t), err
	}}
	var j *ledger.Journal
	if strings.EqualFold(filepath.Ext(abs), ".org") {
		blocks := ledger.OrgBlocks(string(b))
		if self.Block != "" {
			pick := []ledger.OrgBlock{}
			for _, bl := range blocks {
				if bl.Name == self.Block || strconv.Itoa(bl.Index) == self.Block {
					pick = append(pick, bl)
				}
			}
			if len(pick) == 0 {
				commands.Fail("%s has no ledger block %q", self.File, self.Block)
			}
			blocks = pick
		}
		j = ledger.Parse(ledger.Chunks(abs, blocks), opt)
	} else {
		j = ledger.ParseText(abs, string(b), opt)
	}
	p, err := ledger.Within(self.Period, self.Begin, self.End, time.Now())
	if err != nil {
		commands.Fail("%v", err)
	}
	f := ledger.Filter{Period: p, Accounts: words, Depth: self.Depth, Real: self.Real}
	switch report {
	case "sources":
		commands.Fail("sources asks the server; -f reads one file")
	case "check":
		showProblems(j.Problems, len(j.Transactions))
		exitOnErrors(j.Problems)
	case "monthly":
		months, series, err := j.Monthly(f)
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.RenderOne(struct {
			Months []string
			Series []ledger.Series
		}{months, series}, func() { fmt.Print(ledger.FormatMonthly(months, series)); warnProblems(j.Problems) })
	case "register":
		rows, err := j.Register(f)
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.Render(rows, func() { fmt.Print(j.FormatRegister(rows)); warnProblems(j.Problems) })
	default:
		rows, total, err := j.Balance(f)
		if err != nil {
			commands.Fail("%v", err)
		}
		commands.Render(rows, func() {
			if self.Flat {
				fmt.Print(j.FormatFlat(rows, total))
			} else {
				fmt.Print(j.FormatBalance(rows, total))
			}
			warnProblems(j.Problems)
		})
	}
}

func init() {
	commands.AddCmd("ledger", "ledger/hledger journals in org files: bal, reg, monthly, check (ledger-mode, ob-ledger)",
		func() commands.Cmd { return &Ledger{} })
}

package ledger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

const sample = `; a comment
account Assets:Bank
commodity $
  format $1,000.00

2026/10/01 * (101) Opening
    Assets:Bank                $1,000.00
    Equity:Opening

2026-10-05 ! Grocer  ; :food:
    Expenses:Food:Groceries      $45.50
    Expenses:Food:Snacks          $4.50
    Assets:Bank                 = $950.00

2026/11/02=2026/11/03 Landlord
    Expenses:Rent               $800
    Assets:Bank                 $-800 = $150
    (Budget:Rent)              $-800

2026/11/03 Broker
    Assets:Stocks       10 AAPL @ $10.00
    Assets:Bank
`

func parse(t *testing.T, text string) *Journal {
	t.Helper()
	return ParseText("t.ledger", text, Options{Now: now})
}

func noProblems(t *testing.T, j *Journal) {
	t.Helper()
	for _, p := range j.Problems {
		t.Errorf("unexpected problem: %s", p)
	}
}

func TestSampleReadsAndBalances(t *testing.T) {
	j := parse(t, sample)
	noProblems(t, j)
	if len(j.Transactions) != 4 {
		t.Fatalf("got %d transactions", len(j.Transactions))
	}
	tx := j.Transactions[0]
	if tx.Status != "*" || tx.Code != "101" || tx.Payee != "Opening" || tx.Line != 6 {
		t.Errorf("header: %+v", tx)
	}
	if got := j.Styles.Format("$", tx.Postings[1].Amount.Q); got != "$-1,000.00" || !tx.Postings[1].Elided {
		t.Errorf("elided posting: %s", got)
	}
	g := j.Transactions[1]
	if g.Status != "!" || g.Note != ":food:" {
		t.Errorf("grocer: %+v", g)
	}
	if _, ok := g.Tags["food"]; !ok {
		t.Errorf("tags: %v", g.Tags)
	}
	// The balance assignment: the bank goes from 1000 to 950.
	if got := j.Styles.Format("$", g.Postings[2].Amount.Q); got != "$-50.00" {
		t.Errorf("assignment: %s", got)
	}
	if l := j.Transactions[2]; l.Aux == nil || l.Aux.Day() != 3 {
		t.Errorf("aux date: %+v", l.Aux)
	}
	if b := j.Transactions[3].Postings[1]; j.Styles.Format("$", b.Amount.Q) != "$-100.00" {
		t.Errorf("cost balances the elided posting: %s", j.Styles.Format("$", b.Amount.Q))
	}
	if len(j.Prices) != 1 || j.Prices[0].Commodity != "AAPL" {
		t.Errorf("prices: %+v", j.Prices)
	}
	if len(j.Accounts) != 1 || j.Accounts[0] != "Assets:Bank" {
		t.Errorf("accounts: %v", j.Accounts)
	}
}

// What ledger 3 prints for the sample, checked by running it; the test
// below runs it again when it is installed.
const sampleBal = `              $50.00
             10 AAPL  Assets
              $50.00    Bank
             10 AAPL    Stocks
            $-800.00  Budget:Rent
          $-1,000.00  Equity:Opening
             $850.00  Expenses
              $50.00    Food
              $45.50      Groceries
               $4.50      Snacks
             $800.00    Rent
--------------------
            $-900.00
             10 AAPL
`

const sampleReg = `26-Oct-01 Opening               Assets:Bank               $1,000.00    $1,000.00
                                Equity:Opening           $-1,000.00            0
26-Oct-05 Grocer                Expense:Food:Groceries       $45.50       $45.50
                                Expenses:Food:Snacks          $4.50       $50.00
                                Assets:Bank                 $-50.00            0
26-Nov-02 Landlord              Expenses:Rent               $800.00      $800.00
                                Assets:Bank                $-800.00            0
                                (Budget:Rent)              $-800.00     $-800.00
26-Nov-03 Broker                Assets:Stocks               10 AAPL     $-800.00
                                                                         10 AAPL
                                Assets:Bank                $-100.00     $-900.00
                                                                         10 AAPL
`

func TestBalanceIsWrittenAsLedgerWritesIt(t *testing.T) {
	j := parse(t, sample)
	out, err := j.Run(Query{Report: "balance"})
	if err != nil {
		t.Fatal(err)
	}
	if out != sampleBal {
		t.Errorf("balance:\n%s\nwant:\n%s", out, sampleBal)
	}
}

func TestRegisterIsWrittenAsLedgerWritesIt(t *testing.T) {
	j := parse(t, sample)
	rows, err := j.Register(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if out := j.FormatRegister(rows); out != sampleReg {
		t.Errorf("register:\n%s\nwant:\n%s", out, sampleReg)
	}
}

func TestTheSameAsLedgerWhenItIsInstalled(t *testing.T) {
	if _, err := exec.LookPath("ledger"); err != nil {
		t.Skip("ledger is not installed")
	}
	f := filepath.Join(t.TempDir(), "t.ledger")
	os.WriteFile(f, []byte(sample), 0644)
	for _, c := range []struct {
		args []string
		want string
	}{{[]string{"bal"}, sampleBal}, {[]string{"reg"}, sampleReg}} {
		out, err := exec.Command("ledger", append([]string{"-f", f, "--columns", "80"}, c.args...)...).Output()
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != c.want {
			t.Errorf("ledger %v now prints:\n%s", c.args, out)
		}
	}
}

func TestBalanceFilters(t *testing.T) {
	j := parse(t, sample)
	q, err := ParseCmdline([]string{"bal", "Expenses", "--depth", "2"}, now)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := j.Run(q)
	want := "             $850.00  Expenses\n              $50.00    Food\n             $800.00    Rent\n"
	if out != want {
		t.Errorf("filtered:\n%s", out)
	}
	q, _ = ParseCmdline([]string{"bal", "-p", "this month", "--flat", "^Assets"}, now)
	out, _ = j.Run(q)
	want = "            $-900.00  Assets:Bank\n             10 AAPL  Assets:Stocks\n--------------------\n            $-900.00\n             10 AAPL\n"
	if out != want {
		t.Errorf("this month flat:\n%s", out)
	}
	q, _ = ParseCmdline([]string{"bal", "-R", "not", "Expenses"}, now)
	rows, _, _ := j.Balance(q.Filter)
	for _, r := range rows {
		if strings.HasPrefix(r.Account, "Expenses") || strings.HasPrefix(r.Account, "Budget") {
			t.Errorf("%s should be filtered out", r.Account)
		}
	}
}

func TestRowsCarryTheTree(t *testing.T) {
	j := parse(t, sample)
	rows, total, err := j.Balance(Filter{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]BalanceRow{}
	for _, r := range rows {
		byName[r.Account] = r
	}
	f := byName["Expenses:Food"]
	if f.Parent != "Expenses" || f.Depth != 1 || !f.Children || f.Total[0].Text != "$50.00" || len(f.Own) != 0 {
		t.Errorf("food row: %+v", f)
	}
	if g := byName["Expenses:Food:Groceries"]; g.Postings != 1 || g.Own[0].Number != 45.5 {
		t.Errorf("groceries row: %+v", g)
	}
	if len(total.Commodities()) != 2 {
		t.Errorf("total: %v", total)
	}
}

func TestMonthly(t *testing.T) {
	j := parse(t, sample)
	months, series, err := j.Monthly(Filter{Accounts: []string{"^Expenses"}, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(months, ",") != "2026-10,2026-11" || len(series) != 2 {
		t.Fatalf("months %v series %+v", months, series)
	}
	if series[0].Account != "Expenses:Food" || series[0].Values[0] != 50 || series[0].Values[1] != 0 {
		t.Errorf("food: %+v", series[0])
	}
	if series[1].Total != "$800.00" {
		t.Errorf("rent: %+v", series[1])
	}
	// No depth: one series of everything taken.
	_, series, _ = j.Monthly(Filter{Accounts: []string{"Expenses"}})
	if len(series) != 1 || series[0].Account != "Expenses" || series[0].Total != "$850.00" {
		t.Errorf("one series: %+v", series)
	}
	p, _ := ParsePeriod("2026", now)
	months, _, _ = j.Monthly(Filter{Accounts: []string{"Expenses"}, Period: p})
	if len(months) != 12 {
		t.Errorf("a whole year is twelve months: %v", months)
	}
}

func problemsOf(j *Journal) string {
	out := []string{}
	for _, p := range j.Problems {
		out = append(out, p.String())
	}
	return strings.Join(out, "\n")
}

func TestErrorsHaveLines(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"unbalanced", "2026/01/01 x\n  A  $10\n  B  $-9\n", "t.ledger:1: error: the transaction does not balance: off by $1"},
		{"two elided", "2026/01/01 x\n  A  $10\n  B\n  C\n", "t.ledger:1: error: only one posting may leave its amount out (lines 3 and 4 both do)"},
		{"bad date", "2026/02/30 x\n  A  $10\n  B\n", `t.ledger:1: error: "2026/02/30" is not a day there is`},
		{"assertion", "2026/01/01 x\n  A  $10 = $11\n  B\n", "t.ledger:2: error: balance assertion failed: A is $10, not $11"},
		{"bad amount", "2026/01/01 x\n  A  $10 junk\n  B\n", `t.ledger:2: error: could not read "junk" after the amount`},
		{"expression", "2026/01/01 x\n  A  ($10 * 2)\n  B\n", "t.ledger:2: error: value expressions like ($10 * 2) are not read; write the amount"},
		{"periodic", "~ monthly\n  A  $10\n  B\n", "t.ledger:1: warning: periodic transactions (~) are not read; it is skipped"},
		{"automated", "= expr true\n  A  1\n", "t.ledger:1: warning: automated transactions (=) are not read; it is skipped"},
		{"only ==", "2026/01/01 x\n  A  $10\n  A  5 EUR\n  B  $-10\n  B  -5 EUR\n2026/01/02 y\n  A  $0 == $10\n", "t.ledger:7: error: balance assertion failed: A also holds 5 EUR"},
		{"unknown", "frobnicate x\n", `t.ledger:1: error: "frobnicate" is not a transaction or a directive this reader knows`},
	}
	for _, c := range cases {
		j := parse(t, c.text)
		if got := problemsOf(j); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestAmounts(t *testing.T) {
	cases := []struct{ in, com, q, out string }{
		{"$1,000.50", "$", "1000.5", "$1,000.50"},
		{"-$5", "$", "-5", "$-5"},
		{"$-5.25", "$", "-5.25", "$-5.25"},
		{"10 AAPL", "AAPL", "10", "10 AAPL"},
		{"1.000,00 EUR", "EUR", "1000", "1.000,00 EUR"},
		{"-3.5EUR", "EUR", "-3.5", "-3.5EUR"},
		{`"M&M 2" 3`, "M&M 2", "3", `"M&M 2" 3`},
		{"€ 12", "€", "12", "€ 12"},
		{"5", "", "5", "5"},
	}
	for _, c := range cases {
		a, st, rest, err := parseAmount(c.in, "", false)
		if err != nil || rest != "" {
			t.Errorf("%s: %v %q", c.in, err, rest)
			continue
		}
		ss := Styles{}
		ss.learn(a.Commodity, st)
		want := new(Amount)
		_ = want
		if a.Commodity != c.com || a.Q.FloatString(4) != mustRat(c.q) {
			t.Errorf("%s: got %s %s", c.in, a.Commodity, a.Q.FloatString(4))
		}
		if got := ss.Format(a.Commodity, a.Q); got != c.out {
			t.Errorf("%s: written %s", c.in, got)
		}
	}
}

func mustRat(s string) string {
	a, _, _, _ := parseAmount(s, "", false)
	return a.Q.FloatString(4)
}

func TestDirectives(t *testing.T) {
	text := `Y 2025
alias food=Expenses:Food
D $1,000.00
P 2025/03/01 AAPL $150.00
apply account Personal
03/02 Shop
    food:Lunch   12
    Assets:Cash
end apply account
decimal-mark ,
2025-03-03 Cafe
    Expenses:Coffee   3,50 EUR
    Assets:Cash
comment
2025/13/45 nonsense
end comment
`
	j := parse(t, text)
	noProblems(t, j)
	if len(j.Transactions) != 2 {
		t.Fatalf("got %d", len(j.Transactions))
	}
	s := j.Transactions[0]
	if s.Date.Year() != 2025 || s.Date.Month() != 3 {
		t.Errorf("Y: %v", s.Date)
	}
	if s.Postings[0].Account != "Personal:Expenses:Food:Lunch" || s.Postings[0].Amount.Commodity != "$" {
		t.Errorf("alias and apply account and D: %+v", s.Postings[0])
	}
	if got := j.Styles.Format("$", s.Postings[0].Amount.Q); got != "$12.00" {
		t.Errorf("D sets the style: %s", got)
	}
	c := j.Transactions[1].Postings[0]
	if c.Amount.Q.FloatString(2) != "3.50" || c.Account != "Expenses:Coffee" {
		t.Errorf("decimal-mark: %+v", c)
	}
	if len(j.Prices) != 1 || j.Prices[0].Price.Q.FloatString(0) != "150" {
		t.Errorf("P: %+v", j.Prices)
	}
}

func TestIncludeGoesThroughOpen(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "more.ledger"), []byte("2026/01/02 b\n  A  $1\n  B\n"), 0644)
	main := filepath.Join(dir, "main.ledger")
	text := "2026/01/01 a\n  A  $1\n  B\ninclude more.ledger\ninclude ../outside.ledger\n"
	j := ParseText(main, text, Options{Now: now, Open: func(p string) (string, error) {
		if filepath.Dir(p) != dir {
			return "", os.ErrPermission
		}
		b, err := os.ReadFile(p)
		return string(b), err
	}})
	if len(j.Transactions) != 2 || j.Transactions[1].File != filepath.Join(dir, "more.ledger") || j.Transactions[1].Line != 1 {
		t.Errorf("include: %+v", j.Transactions)
	}
	if len(j.Problems) != 1 || !strings.Contains(j.Problems[0].Msg, "permission") || j.Problems[0].Line != 5 {
		t.Errorf("outside: %s", problemsOf(j))
	}
	j = ParseText(main, text, Options{Now: now})
	if j.Errors() != 2 {
		t.Errorf("no Open, no includes: %s", problemsOf(j))
	}
}

func TestImplicitExchangeAndTolerance(t *testing.T) {
	j := parse(t, "2026/01/01 fx\n  Assets:EUR  100 EUR\n  Assets:USD  $-110.00\n\n2026/01/02 thirds\n  A  3 X @ $0.333\n  B  $-1.00\n")
	noProblems(t, j)
	j = parse(t, "2026/01/01 fx\n  Assets:EUR  100 EUR\n  Assets:USD  $110.00\n")
	if j.Errors() != 1 {
		t.Errorf("same signs do not exchange: %s", problemsOf(j))
	}
}

func TestMultiCommodityElisionSplits(t *testing.T) {
	j := parse(t, "2026/01/01 x\n  A  $10\n  A  5 EUR\n  B\n")
	noProblems(t, j)
	if n := len(j.Transactions[0].Postings); n != 4 {
		t.Errorf("one posting per commodity: %d", n)
	}
}

func TestPeriods(t *testing.T) {
	cases := map[string]string{
		"this month":                    "2026-11-01 to 2026-11-30",
		"last month":                    "2026-10-01 to 2026-10-31",
		"this year":                     "2026-01-01 to 2026-12-31",
		"last year":                     "2025-01-01 to 2025-12-31",
		"2025":                          "2025-01-01 to 2025-12-31",
		"2026-02":                       "2026-02-01 to 2026-02-28",
		"2026/02":                       "2026-02-01 to 2026-02-28",
		"feb":                           "2026-02-01 to 2026-02-28",
		"march 2024":                    "2024-03-01 to 2024-03-31",
		"q1":                            "2026-01-01 to 2026-03-31",
		"2025q4":                        "2025-10-01 to 2025-12-31",
		"this week":                     "2026-11-02 to 2026-11-08",
		"last quarter":                  "2026-07-01 to 2026-09-30",
		"today":                         "2026-11-04 to 2026-11-04",
		"from 2026-01-01 to 2026-04-01": "2026-01-01 to 2026-03-31",
		"jan..mar":                      "2026-01-01 to 2026-02-28",
		"since last month":              "from 2026-10-01",
		"until this month":              "before 2026-11-01",
		"":                              "",
	}
	for in, want := range cases {
		p, err := ParsePeriod(in, now)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if p.String() != want {
			t.Errorf("%q: got %q want %q", in, p.String(), want)
		}
	}
	if _, err := ParsePeriod("whenever", now); err == nil {
		t.Errorf("nonsense is refused")
	}
}

func TestCmdlineRefusesWhatItDoesNotKnow(t *testing.T) {
	for _, w := range [][]string{{"print"}, {"bal", "--market"}, {"bal", "-p"}} {
		if _, err := ParseCmdline(w, now); err == nil {
			t.Errorf("%v should be refused", w)
		}
	}
	q, err := ParseCmdline([]string{"reg", "--period=last month", "-2", "Food"}, now)
	if err != nil || q.Report != "register" || q.Filter.Depth != 2 || q.Filter.Accounts[0] != "Food" || q.Filter.Period.String() != "2026-10-01 to 2026-10-31" {
		t.Errorf("reg: %+v %v", q, err)
	}
}

func TestOrgBlocksMapLines(t *testing.T) {
	org := `* Money
#+NAME: opening
#+BEGIN_SRC ledger :cmdline bal
2026/01/01 Open
    Assets:Bank   $100
    Equity
#+END_SRC

#+BEGIN_EXAMPLE
#+BEGIN_SRC ledger
not a block
#+END_SRC
#+END_EXAMPLE

- item
  #+begin_src hledger
  2026/01/02 Spend
      Expenses   $30
      Assets:Bank   $-30 = $71
  #+end_src
`
	bs := OrgBlocks(org)
	if len(bs) != 2 || bs[0].Name != "opening" || bs[0].Begin != 3 || bs[1].Lang != "hledger" || bs[1].Index != 1 {
		t.Fatalf("blocks: %+v", bs)
	}
	if !strings.HasPrefix(bs[1].Text, "2026/01/02 Spend\n    Expenses") {
		t.Errorf("dedented: %q", bs[1].Text)
	}
	j := Parse(Chunks("money.org", bs), Options{Now: now})
	// The second block's assertion is checked against the first's balance:
	// the file read as one journal. Its line is the org file's.
	if len(j.Problems) != 1 || j.Problems[0].Line != 19 || j.Problems[0].File != "money.org" {
		t.Errorf("problems: %s", problemsOf(j))
	}
	// Each block alone, as ob-ledger runs them: the assertion is about the
	// second block only.
	j = Parse(Chunks("money.org", bs[1:]), Options{Now: now})
	if !strings.Contains(problemsOf(j), "is $-30, not $71") {
		t.Errorf("alone: %s", problemsOf(j))
	}
}

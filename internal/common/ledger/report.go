package ledger

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Filter is what a report is over.
type Filter struct {
	Period Period
	// Account regular expressions, any of which may match (case
	// insensitive, anywhere in the name, as ledger's are). A word starting
	// with ^ or ending with $ anchors as written.
	Accounts []string
	// Postings to accounts deeper than this are counted in their ancestor
	// at this depth. 0 is no limit.
	Depth int
	// Only real postings, no virtual ones (ledger's --real).
	Real bool
	// Use the auxiliary date where there is one (ledger's --aux-date).
	Aux bool
}

type matcher struct {
	res []*regexp.Regexp
	not []*regexp.Regexp
}

func compileAccounts(pats []string) (*matcher, error) {
	m := &matcher{}
	neg := false
	for _, p := range pats {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.EqualFold(p, "not") {
			neg = true
			continue
		}
		into := &m.res
		if neg || strings.HasPrefix(p, "!") {
			into = &m.not
			p = strings.TrimPrefix(p, "!")
		}
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return nil, fmt.Errorf("account pattern %q: %v", p, err)
		}
		*into = append(*into, re)
	}
	return m, nil
}

func (m *matcher) match(acct string) bool {
	for _, re := range m.not {
		if re.MatchString(acct) {
			return false
		}
	}
	if len(m.res) == 0 {
		return true
	}
	for _, re := range m.res {
		if re.MatchString(acct) {
			return true
		}
	}
	return false
}

// Truncate is an account cut to depth parts.
func Truncate(acct string, depth int) string {
	if depth <= 0 {
		return acct
	}
	parts := strings.Split(acct, ":")
	if len(parts) > depth {
		parts = parts[:depth]
	}
	return strings.Join(parts, ":")
}

func parentOf(acct string) string {
	if at := strings.LastIndex(acct, ":"); at >= 0 {
		return acct[:at]
	}
	return ""
}

func dateOf(t *Transaction, aux bool) time.Time {
	if aux && t.Aux != nil {
		return *t.Aux
	}
	return t.Date
}

// each walks the postings a filter takes, in date order (the order written
// for one day).
func (j *Journal) each(f Filter, fn func(t *Transaction, p *Posting)) error {
	m, err := compileAccounts(f.Accounts)
	if err != nil {
		return err
	}
	ts := append([]*Transaction{}, j.Transactions...)
	sort.SliceStable(ts, func(a, b int) bool { return dateOf(ts[a], f.Aux).Before(dateOf(ts[b], f.Aux)) })
	for _, t := range ts {
		if !f.Period.Contains(dateOf(t, f.Aux)) {
			continue
		}
		for _, p := range t.Postings {
			if f.Real && p.Virtual != "" {
				continue
			}
			if !m.match(p.Account) {
				continue
			}
			fn(t, p)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Balance
// ---------------------------------------------------------------------------

// BalanceRow is one account of the tree.
type BalanceRow struct {
	Account  string // the full name
	Name     string // its last part
	Parent   string
	Depth    int // 0 for a top level account
	Total    []Amt
	Own      []Amt
	Postings int
	// Whether it has accounts under it in the report.
	Children bool

	total, own Mixed
}

// Balance is the account tree: each account's own postings and its total
// with its subaccounts', in name order, accounts that come to nothing left
// out. The total is everything the filter took.
func (j *Journal) Balance(f Filter) ([]BalanceRow, Mixed, error) {
	own := map[string]Mixed{}
	count := map[string]int{}
	err := j.each(f, func(t *Transaction, p *Posting) {
		a := Truncate(p.Account, f.Depth)
		if own[a] == nil {
			own[a] = Mixed{}
		}
		own[a].AddAmount(p.Amount)
		count[a]++
	})
	if err != nil {
		return nil, nil, err
	}
	total := map[string]Mixed{}
	all := Mixed{}
	for a, m := range own {
		all.AddMixed(m)
		for x := a; x != ""; x = parentOf(x) {
			if total[x] == nil {
				total[x] = Mixed{}
			}
			total[x].AddMixed(m)
		}
	}
	// An account that comes to something, and the accounts above it even
	// when they come to nothing (a bank and a card that cancel out).
	keep := map[string]bool{}
	for a, m := range total {
		if !m.IsZero() {
			for x := a; x != ""; x = parentOf(x) {
				keep[x] = true
			}
		}
	}
	names := []string{}
	for a := range keep {
		names = append(names, a)
	}
	sort.Strings(names)
	kids := map[string]bool{}
	for _, a := range names {
		kids[parentOf(a)] = true
	}
	rows := []BalanceRow{}
	for _, a := range names {
		o := own[a]
		if o == nil {
			o = Mixed{}
		}
		name := a
		if at := strings.LastIndex(a, ":"); at >= 0 {
			name = a[at+1:]
		}
		rows = append(rows, BalanceRow{
			Account: a, Name: name, Parent: parentOf(a), Depth: strings.Count(a, ":"),
			Total: j.Styles.Amts(total[a]), Own: j.Styles.Amts(o), Postings: count[a],
			Children: kids[a], total: total[a], own: o,
		})
	}
	return rows, all, nil
}

// FormatBalance is the tree as ledger's balance report writes it: amounts
// right aligned in twenty columns, two spaces of indent a level, an account
// with one subaccount and nothing of its own written as one name
// (Assets:Bank), and a total under a rule when there is more than one top
// level line.
func (j *Journal) FormatBalance(rows []BalanceRow, total Mixed) string {
	byParent := map[string][]int{}
	for i, r := range rows {
		byParent[r.Parent] = append(byParent[r.Parent], i)
	}
	var b strings.Builder
	tops := 0
	var walk func(i int, indent int, prefix string)
	walk = func(i int, indent int, prefix string) {
		r := rows[i]
		name := prefix + r.Name
		kids := byParent[r.Account]
		if len(kids) == 1 && r.own.IsZero() {
			walk(kids[0], indent, name+":")
			return
		}
		lines := j.Styles.FormatMixed(r.total)
		for k, l := range lines {
			if k < len(lines)-1 {
				fmt.Fprintf(&b, "%20s\n", l)
			} else {
				fmt.Fprintf(&b, "%20s  %s%s\n", l, strings.Repeat("  ", indent), name)
			}
		}
		for _, k := range kids {
			walk(k, indent+1, "")
		}
	}
	for _, i := range byParent[""] {
		tops++
		walk(i, 0, "")
	}
	// A report that starts below the top (depth cut, or a parent filtered
	// out) still has rows whose parent is not in it.
	have := map[string]bool{}
	for _, r := range rows {
		have[r.Account] = true
	}
	for i, r := range rows {
		if r.Parent != "" && !have[r.Parent] {
			tops++
			walk(i, 0, r.Parent+":")
		}
	}
	if tops > 1 {
		b.WriteString(strings.Repeat("-", 20) + "\n")
		for _, l := range j.Styles.FormatMixed(total) {
			fmt.Fprintf(&b, "%20s\n", l)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

// RegisterRow is one posting with the running total after it.
type RegisterRow struct {
	Date    string
	Aux     string `json:",omitempty"`
	Status  string
	Code    string `json:",omitempty"`
	Payee   string
	Account string
	Virtual string `json:",omitempty"` // "(" or "[" for a virtual posting
	Amount  []Amt
	Total   []Amt
	File    string
	Line    int    // the posting's line
	TxLine  int    // the transaction's
	Note    string `json:",omitempty"`
	// The first posting of its transaction: where a report writes the payee.
	First bool

	amount, total Mixed
}

// Register is the postings a filter takes, in date order, each with the
// running total.
func (j *Journal) Register(f Filter) ([]RegisterRow, error) {
	run := Mixed{}
	rows := []RegisterRow{}
	var last *Transaction
	err := j.each(f, func(t *Transaction, p *Posting) {
		run.AddAmount(p.Amount)
		am := Mixed{}
		am.AddAmount(p.Amount)
		r := RegisterRow{
			Date: dateOf(t, f.Aux).Format("2006-01-02"), Status: t.Status, Code: t.Code, Payee: t.Payee,
			Account: Truncate(p.Account, f.Depth), File: t.File, Line: p.Line, TxLine: t.Line,
			Note: p.Note, First: t != last, Virtual: p.Virtual, amount: am, total: run.Clone(),
		}
		if p.Status != "" {
			r.Status = p.Status
		}
		if t.Aux != nil {
			r.Aux = t.Aux.Format("2006-01-02")
		}
		r.Amount = j.Styles.Amts(am)
		if len(r.Amount) == 0 {
			r.Amount = j.Styles.Amts(Mixed{p.Amount.Commodity: big.NewRat(0, 1)})
		}
		r.Total = j.Styles.Amts(run)
		rows = append(rows, r)
		last = t
	})
	return rows, err
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	rs := []rune(s)
	if n <= 2 {
		return string(rs[:n])
	}
	return string(rs[:n-2]) + ".."
}

// shortAccount abbreviates an account to fit, as ledger does: a letter at a
// time off the parts before the last, in turn, down to two letters each
// (Expenses:Food:Groceries -> Expense:Food:Groceries), then clipped.
func shortAccount(a string, n int) string {
	if utf8.RuneCountInString(a) <= n {
		return a
	}
	parts := [][]rune{}
	for _, p := range strings.Split(a, ":") {
		parts = append(parts, []rune(p))
	}
	length := utf8.RuneCountInString(a)
	for length > n {
		cut := false
		for i := 0; i < len(parts)-1 && length > n; i++ {
			if len(parts[i]) > 2 {
				parts[i] = parts[i][:len(parts[i])-1]
				length--
				cut = true
			}
		}
		if !cut {
			break
		}
	}
	out := []string{}
	for _, p := range parts {
		out = append(out, string(p))
	}
	return clip(strings.Join(out, ":"), n)
}

// FormatRegister is the register as ledger writes it in eighty columns:
// date (ledger's 26-Nov-04), payee, account, amount, running total, the
// date and payee only on a transaction's first posting, and a line more for
// each further commodity.
func (j *Journal) FormatRegister(rows []RegisterRow) string {
	var b strings.Builder
	for _, r := range rows {
		payee := ""
		date := ""
		if r.First {
			payee, date = r.Payee, r.Date
			if t, err := time.Parse("2006-01-02", r.Date); err == nil {
				date = t.Format("06-Jan-02")
			}
		}
		acct := r.Account
		switch r.Virtual {
		case "(":
			acct = "(" + acct + ")"
		case "[":
			acct = "[" + acct + "]"
		}
		am := j.Styles.FormatMixed(r.amount)
		tot := j.Styles.FormatMixed(r.total)
		n := len(am)
		if len(tot) > n {
			n = len(tot)
		}
		for k := 0; k < n; k++ {
			a, t := "", ""
			if k < len(am) {
				a = am[k]
			}
			if k < len(tot) {
				t = tot[k]
			}
			if k == 0 {
				fmt.Fprintf(&b, "%-9s %-21s %-22s %12s %12s\n", date, clip(payee, 21), shortAccount(acct, 22), a, t)
			} else {
				fmt.Fprintf(&b, "%-9s %-21s %-22s %12s %12s\n", "", "", "", a, t)
			}
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Monthly totals
// ---------------------------------------------------------------------------

// Series is one account's total each month, in one commodity.
type Series struct {
	Account   string
	Commodity string
	Values    []float64
	Texts     []string
	Total     string
}

// Monthly is the postings a filter takes, summed by month and by account
// (cut to the filter's depth; with no depth, all of them as one series per
// commodity). Months run from the first to the last with a posting, or
// across the whole period when it has both ends.
func (j *Journal) Monthly(f Filter) ([]string, []Series, error) {
	type key struct{ acct, com string }
	sums := map[key]map[string]*big.Rat{}
	var first, last time.Time
	label := "Total"
	if len(f.Accounts) > 0 {
		label = strings.Join(f.Accounts, " ")
	}
	err := j.each(f, func(t *Transaction, p *Posting) {
		d := dateOf(t, f.Aux)
		if first.IsZero() || d.Before(first) {
			first = d
		}
		if d.After(last) {
			last = d
		}
		a := label
		if f.Depth > 0 {
			a = Truncate(p.Account, f.Depth)
		}
		k := key{a, p.Amount.Commodity}
		if sums[k] == nil {
			sums[k] = map[string]*big.Rat{}
		}
		m := d.Format("2006-01")
		if sums[k][m] == nil {
			sums[k][m] = new(big.Rat)
		}
		sums[k][m].Add(sums[k][m], p.Amount.Q)
	})
	if err != nil {
		return nil, nil, err
	}
	if !f.Period.Begin.IsZero() && !f.Period.End.IsZero() {
		first, last = f.Period.Begin, f.Period.End.AddDate(0, 0, -1)
	}
	months := []string{}
	if !first.IsZero() {
		for m := day(first.Year(), first.Month(), 1); !m.After(last); m = m.AddDate(0, 1, 0) {
			months = append(months, m.Format("2006-01"))
		}
	}
	keys := []key{}
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].acct != keys[b].acct {
			return keys[a].acct < keys[b].acct
		}
		return keys[a].com < keys[b].com
	})
	out := []Series{}
	for _, k := range keys {
		s := Series{Account: k.acct, Commodity: k.com}
		tot := new(big.Rat)
		for _, m := range months {
			q := sums[k][m]
			if q == nil {
				q = new(big.Rat)
			}
			tot.Add(tot, q)
			f, _ := q.Float64()
			s.Values = append(s.Values, f)
			s.Texts = append(s.Texts, j.Styles.Format(k.com, q))
		}
		if tot.Sign() == 0 && allZeroRats(sums[k]) {
			continue
		}
		s.Total = j.Styles.Format(k.com, tot)
		out = append(out, s)
	}
	return months, out, nil
}

func allZeroRats(m map[string]*big.Rat) bool {
	for _, q := range m {
		if q.Sign() != 0 {
			return false
		}
	}
	return true
}

// FormatMonthly is a month a line, a column a series.
func FormatMonthly(months []string, series []Series) string {
	if len(series) == 0 {
		return ""
	}
	heads := []string{"Month"}
	widths := []int{7}
	for _, s := range series {
		h := SeriesLabel(s, series)
		heads = append(heads, h)
		w := utf8.RuneCountInString(h)
		for _, t := range append(append([]string{}, s.Texts...), s.Total) {
			if n := utf8.RuneCountInString(t); n > w {
				w = n
			}
		}
		widths = append(widths, w)
	}
	var b strings.Builder
	row := func(cells []string) {
		for i, c := range cells {
			if i == 0 {
				fmt.Fprintf(&b, "%-*s", widths[0], c)
			} else {
				fmt.Fprintf(&b, "  %*s", widths[i], c)
			}
		}
		b.WriteString("\n")
	}
	row(heads)
	for i, m := range months {
		cells := []string{m}
		for _, s := range series {
			cells = append(cells, s.Texts[i])
		}
		row(cells)
	}
	cells := []string{"Total"}
	for _, s := range series {
		cells = append(cells, s.Total)
	}
	row(cells)
	return b.String()
}

// SeriesLabel is a series' account, with its commodity when the series are
// in more than one.
func SeriesLabel(s Series, all []Series) string {
	for _, o := range all {
		if o.Commodity != s.Commodity {
			return s.Account + " (" + s.Commodity + ")"
		}
	}
	return s.Account
}

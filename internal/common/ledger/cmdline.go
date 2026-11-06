package ledger

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Query is a report asked for in ledger's command line words: `bal`, `reg
// Expenses -p "this month"`, `balance --depth 2 ^Assets`.
type Query struct {
	Report string // "balance", "register", "accounts"
	Filter Filter
	Flat   bool
}

// ParseCmdline reads ledger's command line words, the ones a babel block's
// :cmdline uses. It refuses what it does not know, rather than answering a
// different question.
func ParseCmdline(words []string, now time.Time) (Query, error) {
	q := Query{}
	period, begin, end := "", "", ""
	pats := []string{}
	value := func(i *int, w string) (string, error) {
		if at := strings.Index(w, "="); at >= 0 && strings.HasPrefix(w, "--") {
			return w[at+1:], nil
		}
		if *i+1 >= len(words) {
			return "", fmt.Errorf("%s needs a value", w)
		}
		*i++
		return words[*i], nil
	}
	for i := 0; i < len(words); i++ {
		w := words[i]
		name := w
		if at := strings.Index(w, "="); at >= 0 && strings.HasPrefix(w, "--") {
			name = w[:at]
		}
		switch {
		case q.Report == "" && !strings.HasPrefix(w, "-"):
			switch strings.ToLower(w) {
			case "bal", "balance", "b":
				q.Report = "balance"
			case "reg", "register", "r":
				q.Report = "register"
			case "accounts", "acc":
				q.Report = "accounts"
			default:
				return q, fmt.Errorf("the built in reader answers bal, reg and accounts, not %q - install ledger for the rest", w)
			}
		case name == "-p" || name == "--period":
			v, err := value(&i, w)
			if err != nil {
				return q, err
			}
			period = v
		case name == "-b" || name == "--begin":
			v, err := value(&i, w)
			if err != nil {
				return q, err
			}
			begin = v
		case name == "-e" || name == "--end":
			v, err := value(&i, w)
			if err != nil {
				return q, err
			}
			end = v
		case name == "--depth":
			v, err := value(&i, w)
			if err != nil {
				return q, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return q, fmt.Errorf("--depth %s is not a number", v)
			}
			q.Filter.Depth = n
		case len(w) == 2 && w[0] == '-' && w[1] >= '1' && w[1] <= '9':
			// hledger's -2 for --depth 2.
			q.Filter.Depth = int(w[1] - '0')
		case name == "-R" || name == "--real":
			q.Filter.Real = true
		case name == "--flat":
			q.Flat = true
		case name == "--aux-date" || name == "--effective":
			q.Filter.Aux = true
		case name == "--tree" || name == "--no-total" || name == "-E" || name == "--empty" || name == "-f":
			// Accepted and ignored: the tree is the default, and -f is the
			// block itself.
			if name == "-f" {
				i++
			}
		case strings.HasPrefix(w, "-"):
			return q, fmt.Errorf("the built in reader does not know %s - install ledger for it", w)
		default:
			pats = append(pats, w)
		}
	}
	if q.Report == "" {
		q.Report = "balance"
	}
	p, err := Within(period, begin, end, now)
	if err != nil {
		return q, err
	}
	q.Filter.Period = p
	q.Filter.Accounts = pats
	return q, nil
}

// Run answers a query as ledger would print it.
func (j *Journal) Run(q Query) (string, error) {
	switch q.Report {
	case "register":
		rows, err := j.Register(q.Filter)
		if err != nil {
			return "", err
		}
		return j.FormatRegister(rows), nil
	case "accounts":
		seen := map[string]bool{}
		err := j.each(q.Filter, func(t *Transaction, p *Posting) { seen[Truncate(p.Account, q.Filter.Depth)] = true })
		if err != nil {
			return "", err
		}
		names := []string{}
		for a := range seen {
			names = append(names, a)
		}
		sort.Strings(names)
		if len(names) == 0 {
			return "", nil
		}
		return strings.Join(names, "\n") + "\n", nil
	}
	rows, total, err := j.Balance(q.Filter)
	if err != nil {
		return "", err
	}
	if q.Flat {
		return j.FormatFlat(rows, total), nil
	}
	return j.FormatBalance(rows, total), nil
}

// FormatFlat is the balance with full names and no tree: each account with
// postings of its own, as `bal --flat` writes it.
func (j *Journal) FormatFlat(rows []BalanceRow, total Mixed) string {
	var b strings.Builder
	n := 0
	for _, r := range rows {
		if r.own.IsZero() {
			continue
		}
		n++
		lines := j.Styles.FormatMixed(r.own)
		for k, l := range lines {
			if k < len(lines)-1 {
				fmt.Fprintf(&b, "%20s\n", l)
			} else {
				fmt.Fprintf(&b, "%20s  %s\n", l, r.Account)
			}
		}
	}
	if n > 1 {
		b.WriteString(strings.Repeat("-", 20) + "\n")
		for _, l := range j.Styles.FormatMixed(total) {
			fmt.Fprintf(&b, "%20s\n", l)
		}
	}
	return b.String()
}

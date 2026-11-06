// Package ledger reads ledger and hledger journals: the subset of the format
// people write by hand and keep in org files, enough for balances, registers,
// monthly totals and checks without ledger or hledger installed.
//
// It is pure: text in, transactions and reports out. Reading an include goes
// through Options.Open, so the caller decides which files may be read.
package ledger

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
	"unicode"
)

// Style is how a commodity is written, learned from its first use (or a
// commodity directive), so a report writes $1,234.50 the way the journal
// does rather than 1234.5 USD.
type Style struct {
	Prefix    bool // $10, not 10 $
	Space     bool // a space between the commodity and the number
	Precision int  // decimals: the most seen
	Thousands bool // 1,000
	Comma     bool // the decimal mark is a comma: 1.000,00
	Declared  bool // from a commodity directive, which later uses do not change
}

// Amount is a quantity of one commodity. The quantity is exact.
type Amount struct {
	Commodity string
	Q         *big.Rat
}

func (a Amount) Neg() Amount { return Amount{a.Commodity, new(big.Rat).Neg(a.Q)} }

// Mixed is a sum of amounts in several commodities.
type Mixed map[string]*big.Rat

func (m Mixed) Add(c string, q *big.Rat) {
	if v, ok := m[c]; ok {
		v.Add(v, q)
		return
	}
	m[c] = new(big.Rat).Set(q)
}

func (m Mixed) AddAmount(a Amount) { m.Add(a.Commodity, a.Q) }

func (m Mixed) AddMixed(o Mixed) {
	for c, q := range o {
		m.Add(c, q)
	}
}

func (m Mixed) Clone() Mixed {
	out := Mixed{}
	out.AddMixed(m)
	return out
}

// Get is the quantity of a commodity, zero when there is none.
func (m Mixed) Get(c string) *big.Rat {
	if v, ok := m[c]; ok {
		return new(big.Rat).Set(v)
	}
	return new(big.Rat)
}

// Commodities in a stable order, those that are zero left out.
func (m Mixed) Commodities() []string {
	out := []string{}
	for c, q := range m {
		if q.Sign() != 0 {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

func (m Mixed) IsZero() bool { return len(m.Commodities()) == 0 }

// ---------------------------------------------------------------------------
// Reading an amount
// ---------------------------------------------------------------------------

// A character that may be part of an unquoted commodity: ledger's rule, no
// digits, no white space and none of the characters that mean something in
// an amount or expression.
func commodityRune(r rune) bool {
	if unicode.IsSpace(r) || unicode.IsDigit(r) {
		return false
	}
	return !strings.ContainsRune(`-+*/^&|=<>{}[]()@;.,"!?:%#`+"`", r)
}

// parseAmount reads an amount from the front of s: `$1,000.50`, `-$5`,
// `$-5`, `10 AAPL`, `1.000,00 EUR`, `"M&M" 3`, `5` (with def as its
// commodity). It returns the amount, how it was written and the rest of s.
func parseAmount(s string, def string, comma bool) (Amount, Style, string, error) {
	st := Style{}
	rs := []rune(s)
	i := 0
	skip := func() bool {
		start := i
		for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
			i++
		}
		return i > start
	}
	commodity := func() (string, error) {
		if i < len(rs) && rs[i] == '"' {
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			if j >= len(rs) {
				return "", fmt.Errorf("a quoted commodity with no closing quote")
			}
			c := string(rs[i+1 : j])
			i = j + 1
			return c, nil
		}
		start := i
		for i < len(rs) && commodityRune(rs[i]) {
			i++
		}
		return string(rs[start:i]), nil
	}
	skip()
	neg := false
	if i < len(rs) && (rs[i] == '-' || rs[i] == '+') {
		neg = rs[i] == '-'
		i++
		skip()
	}
	com := ""
	if i < len(rs) && !unicode.IsDigit(rs[i]) && rs[i] != '.' && rs[i] != ',' {
		c, err := commodity()
		if err != nil {
			return Amount{}, st, s, err
		}
		if c == "" {
			return Amount{}, st, s, fmt.Errorf("%q is not an amount", strings.TrimSpace(s))
		}
		com, st.Prefix = c, true
		st.Space = skip()
		if i < len(rs) && (rs[i] == '-' || rs[i] == '+') {
			neg = neg != (rs[i] == '-')
			i++
		}
	}
	start := i
	for i < len(rs) && (unicode.IsDigit(rs[i]) || rs[i] == '.' || rs[i] == ',') {
		i++
	}
	num := string(rs[start:i])
	if !strings.ContainsAny(num, "0123456789") {
		return Amount{}, st, s, fmt.Errorf("%q is not an amount", strings.TrimSpace(s))
	}
	if !st.Prefix {
		save := i
		sp := skip()
		c, err := commodity()
		if err != nil {
			return Amount{}, st, s, err
		}
		if c != "" {
			com, st.Space = c, sp
		} else {
			i = save
		}
	}
	q, prec, thousands, decComma, err := parseNumber(num, comma)
	if err != nil {
		return Amount{}, st, s, err
	}
	if neg {
		q.Neg(q)
	}
	st.Precision, st.Thousands, st.Comma = prec, thousands, decComma
	if com == "" {
		com = def
	}
	return Amount{com, q}, st, string(rs[i:]), nil
}

// parseNumber reads 1,000.50 (or 1.000,50 when the decimal mark is a comma).
// With both marks present the last one is the decimal mark, whatever the
// journal says; with only a comma, it is a thousands separator unless comma
// is set (ledger's reading; `decimal-mark ,` sets it).
func parseNumber(num string, comma bool) (*big.Rat, int, bool, bool, error) {
	dec := "."
	lastDot, lastComma := strings.LastIndex(num, "."), strings.LastIndex(num, ",")
	switch {
	case lastDot >= 0 && lastComma >= 0:
		if lastComma > lastDot {
			dec = ","
		}
	case comma:
		dec = ","
	}
	thou := map[string]string{".": ",", ",": "."}[dec]
	thousands := strings.Contains(num, thou)
	if strings.Count(num, dec) > 1 {
		return nil, 0, false, false, fmt.Errorf("%q has more than one decimal mark", num)
	}
	clean := strings.ReplaceAll(num, thou, "")
	prec := 0
	if at := strings.Index(clean, dec); at >= 0 {
		prec = len(clean) - at - 1
		clean = clean[:at] + "." + clean[at+1:]
	}
	if strings.HasPrefix(clean, ".") {
		clean = "0" + clean
	}
	q, ok := new(big.Rat).SetString(clean)
	if !ok {
		return nil, 0, false, false, fmt.Errorf("%q is not a number", num)
	}
	return q, prec, thousands, dec == ",", nil
}

// ---------------------------------------------------------------------------
// Writing one
// ---------------------------------------------------------------------------

// Styles is what each commodity looks like.
type Styles map[string]*Style

// learn notes how a commodity was written: the first use settles the side
// and spacing, the most decimals seen the precision.
func (ss Styles) learn(c string, st Style) {
	have, ok := ss[c]
	if !ok {
		cp := st
		cp.Declared = false
		ss[c] = &cp
		return
	}
	if have.Declared {
		return
	}
	if st.Precision > have.Precision {
		have.Precision = st.Precision
	}
	have.Thousands = have.Thousands || st.Thousands
}

// learnSide is learn for a price or a cost: ledger does not take a
// commodity's precision from what something cost in it, so 3 X @ $0.333
// leaves dollars at two places (and balancing to them).
func (ss Styles) learnSide(c string, st Style) {
	if _, ok := ss[c]; !ok {
		ss.learn(c, st)
	}
}

func (ss Styles) style(c string) Style {
	if st, ok := ss[c]; ok {
		return *st
	}
	if c == "" {
		return Style{}
	}
	return Style{Space: true}
}

func quoteCommodity(c string) string {
	for _, r := range c {
		if !commodityRune(r) {
			return `"` + c + `"`
		}
	}
	return c
}

// Format writes an amount the way the journal writes that commodity:
// `$-1,234.50`, `-10 AAPL`, `1.000,00 EUR`.
func (ss Styles) Format(c string, q *big.Rat) string {
	st := ss.style(c)
	n := numberText(q, st.Precision, st)
	if c == "" {
		return n
	}
	cc := quoteCommodity(c)
	sp := ""
	if st.Space {
		sp = " "
	}
	if st.Prefix {
		return cc + sp + n
	}
	return n + sp + cc
}

func numberText(q *big.Rat, prec int, st Style) string {
	s := q.FloatString(prec)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if allZero(s) {
		neg = false
	}
	whole, frac := s, ""
	if at := strings.Index(s, "."); at >= 0 {
		whole, frac = s[:at], s[at+1:]
	}
	thou, dec := ",", "."
	if st.Comma {
		thou, dec = ".", ","
	}
	if st.Thousands && len(whole) > 3 {
		var b strings.Builder
		lead := len(whole) % 3
		if lead > 0 {
			b.WriteString(whole[:lead])
		}
		for k := lead; k < len(whole); k += 3 {
			if b.Len() > 0 {
				b.WriteString(thou)
			}
			b.WriteString(whole[k : k+3])
		}
		whole = b.String()
	}
	out := whole
	if frac != "" {
		out += dec + frac
	}
	if neg {
		out = "-" + out
	}
	return out
}

func allZero(s string) bool {
	for _, r := range s {
		if r != '0' && r != '.' {
			return false
		}
	}
	return true
}

// FormatMixed is a sum as one line per commodity, "0" when it is nothing.
func (ss Styles) FormatMixed(m Mixed) []string {
	out := []string{}
	for _, c := range m.Commodities() {
		out = append(out, ss.Format(c, m[c]))
	}
	if len(out) == 0 {
		out = append(out, "0")
	}
	return out
}

// Amt is an amount on its way to a client: exact quantity as a decimal
// string, written text, and a float for drawing a chart.
type Amt struct {
	Commodity string
	Quantity  string
	Text      string
	Number    float64
}

func (ss Styles) Amts(m Mixed) []Amt {
	out := []Amt{}
	for _, c := range m.Commodities() {
		q := m[c]
		f, _ := q.Float64()
		prec := ss.style(c).Precision
		if prec < 2 {
			prec = 2
		}
		out = append(out, Amt{Commodity: c, Quantity: trimZeros(q.FloatString(prec + 4)), Text: ss.Format(c, q), Number: f})
	}
	return out
}

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// nearZero is whether q rounds to zero at the commodity's precision: the
// tolerance ledger balances a transaction to, so 3 @ $0.333 balances
// against $1.00 the way it does in ledger.
func (ss Styles) nearZero(c string, q *big.Rat) bool {
	if q.Sign() == 0 {
		return true
	}
	prec := ss.style(c).Precision
	half := new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Mul(big.NewInt(2), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(prec)), nil)))
	abs := new(big.Rat).Abs(q)
	return abs.Cmp(half) < 0
}

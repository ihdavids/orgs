package ledger

import (
	"fmt"
	"math/big"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Problem is something wrong with a journal, at a line of the file it was
// written in (an org file's own line for a source block).
type Problem struct {
	File     string
	Line     int
	Msg      string
	Severity string // "error" or "warning"
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", filepath.Base(p.File), p.Line, p.Severity, p.Msg)
}

// Posting is one line of a transaction.
type Posting struct {
	Line    int
	Status  string // "", "*", "!"
	Account string
	// "" for a real posting, "(" for a virtual one that need not balance,
	// "[" for a virtual one that must.
	Virtual string
	Amount  Amount // set once the transaction is read, elided or not
	// The amount was left out and worked out from the others.
	Elided bool
	// What the amount cost, in total, when it was bought with @ or @@ (or a
	// {lot price}): the commodity the transaction balances in.
	Cost *Amount
	// A balance assertion: the account's balance after this posting.
	Assert *Amount
	// == rather than =: no other commodity may be in the account either.
	AssertOnly bool
	// =* / ==*: the balance with the subaccounts', hledger's inclusive form.
	AssertTree bool
	Note       string
	Tags       map[string]string

	assign bool // a balance assignment: no amount, only = $100
}

// Transaction is a dated entry.
type Transaction struct {
	File     string
	Line     int
	Date     time.Time
	Aux      *time.Time // =date, the effective or auxiliary date
	Status   string
	Code     string
	Payee    string
	Note     string
	Tags     map[string]string
	Postings []*Posting
	// The lines it was written on, for a report that links back to them.
	EndLine int
}

// Price is a P directive (or what an @ said a commodity cost that day).
type Price struct {
	File      string
	Line      int
	Date      time.Time
	Commodity string
	Price     Amount
}

// Journal is everything read.
type Journal struct {
	Transactions []*Transaction
	Prices       []Price
	Accounts     []string // declared with `account`
	Styles       Styles
	Problems     []Problem
	// Files read through include, for knowing when to read again.
	Included []string
}

// Errors is how many of the problems are errors rather than warnings.
func (j *Journal) Errors() int {
	n := 0
	for _, p := range j.Problems {
		if p.Severity == "error" {
			n++
		}
	}
	return n
}

// Chunk is a piece of journal text and where it was written: the file and
// the line (from 1) of its first line. A ledger source block is a chunk
// starting on the line after its #+BEGIN_SRC.
type Chunk struct {
	File string
	Line int
	Text string
}

// Options say how to read.
type Options struct {
	// Reads an included file, given its absolute path. Nil refuses every
	// include. The caller decides which files may be read.
	Open func(path string) (string, error)
	// For a date with no year and no Y directive.
	Now time.Time
}

// Parse reads chunks in order as one journal: the year, aliases and
// commodities carry from one into the next, and so do the balances that
// assertions check.
func Parse(chunks []Chunk, opt Options) *Journal {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	p := &parser{
		j:   &Journal{Styles: Styles{}, Problems: []Problem{}, Transactions: []*Transaction{}, Prices: []Price{}, Accounts: []string{}},
		opt: opt, bal: map[string]Mixed{}, seenAcct: map[string]bool{},
	}
	for _, c := range chunks {
		p.chunk(c, 0)
	}
	return p.j
}

// ParseText reads one text as a journal written in file from line 1.
func ParseText(file, text string, opt Options) *Journal {
	return Parse([]Chunk{{File: file, Line: 1, Text: text}}, opt)
}

type alias struct {
	from string
	re   *regexp.Regexp
	to   string
}

type parser struct {
	j        *Journal
	opt      Options
	year     int
	aliases  []alias
	apply    []string
	defCom   string
	comma    bool
	bal      map[string]Mixed
	seenAcct map[string]bool

	file   string
	cur    *Transaction
	last   *Posting
	skip   string // what indented lines belong to when there is no transaction
	comCtx string // the commodity a `commodity` directive's sub-lines are about
	block  string // "comment" or "test": everything until its end line
}

func (p *parser) problem(line int, sev, format string, args ...interface{}) {
	p.j.Problems = append(p.j.Problems, Problem{File: p.file, Line: line, Msg: fmt.Sprintf(format, args...), Severity: sev})
}

func (p *parser) chunk(c Chunk, depth int) {
	saveFile := p.file
	p.file = c.File
	defer func() { p.file = saveFile }()
	lines := strings.Split(strings.ReplaceAll(c.Text, "\r\n", "\n"), "\n")
	for i, raw := range lines {
		p.line(raw, c.Line+i, depth)
	}
	p.finish()
	p.skip, p.block = "", ""
}

func indented(s string) bool { return s != "" && (s[0] == ' ' || s[0] == '\t') }

func (p *parser) line(raw string, ln int, depth int) {
	line := strings.TrimRight(raw, " \t\r")
	trim := strings.TrimSpace(line)
	if p.block != "" {
		if strings.EqualFold(trim, "end "+p.block) {
			p.block = ""
		}
		return
	}
	if trim == "" {
		p.finish()
		p.skip = ""
		return
	}
	if indented(line) {
		switch {
		case p.cur != nil:
			p.indentedInTxn(trim, ln)
		case p.skip == "commodity":
			if w, rest := firstWord(trim); w == "format" {
				p.commodityFormat(rest, ln)
			}
		case p.skip != "":
		default:
			if !strings.HasPrefix(trim, ";") {
				p.problem(ln, "warning", "an indented line that is not part of a transaction")
			}
		}
		return
	}
	p.finish()
	p.skip = ""
	switch line[0] {
	case ';', '#', '%', '|', '*':
		return
	case '=':
		p.problem(ln, "warning", "automated transactions (=) are not read; it is skipped")
		p.skip = "auto"
		return
	case '~':
		p.problem(ln, "warning", "periodic transactions (~) are not read; it is skipped")
		p.skip = "periodic"
		return
	}
	if line[0] >= '0' && line[0] <= '9' {
		p.header(line, ln)
		return
	}
	p.directive(trim, ln, depth)
}

func firstWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	if at := strings.IndexAny(s, " \t"); at >= 0 {
		return s[:at], strings.TrimSpace(s[at+1:])
	}
	return s, ""
}

// ---------------------------------------------------------------------------
// Directives
// ---------------------------------------------------------------------------

func (p *parser) directive(line string, ln int, depth int) {
	w, rest := firstWord(line)
	switch strings.ToLower(w) {
	case "account":
		acct := p.account(stripComment(rest))
		if acct != "" && !p.seenAcct[acct] {
			p.seenAcct[acct] = true
			p.j.Accounts = append(p.j.Accounts, acct)
		}
		p.skip = "account"
	case "commodity":
		p.skip = "commodity"
		body := stripComment(rest)
		p.comCtx = body
		if strings.ContainsAny(body, "0123456789") {
			p.commodityFormat(body, ln)
		} else if _, ok := p.j.Styles[body]; !ok && body != "" {
			p.j.Styles[body] = &Style{Prefix: len([]rune(body)) == 1, Space: len([]rune(body)) > 1}
		}
	case "include", "!include":
		p.include(stripComment(rest), ln, depth)
	case "alias":
		p.addAlias(rest, ln)
	case "end":
		w2, _ := firstWord(rest)
		switch strings.ToLower(w2) {
		case "aliases":
			p.aliases = nil
		case "apply":
			if len(p.apply) > 0 {
				p.apply = p.apply[:len(p.apply)-1]
			}
		}
	case "apply":
		w2, r2 := firstWord(rest)
		switch strings.ToLower(w2) {
		case "account":
			p.apply = append(p.apply, strings.Trim(stripComment(r2), ":"))
		case "year":
			p.setYear(r2, ln)
		default:
			p.problem(ln, "warning", "apply %s is not read", w2)
			p.apply = append(p.apply, "")
		}
	case "y", "year":
		p.setYear(rest, ln)
	case "d":
		if a, st, _, err := parseAmount(rest, "", p.comma); err == nil {
			p.defCom = a.Commodity
			st.Declared = true
			p.j.Styles[a.Commodity] = &st
		} else {
			p.problem(ln, "error", "D: %v", err)
		}
	case "p":
		p.price(rest, ln)
	case "decimal-mark":
		p.comma = strings.TrimSpace(rest) == ","
	case "comment", "test":
		p.block = strings.ToLower(w)
	case "tag", "payee", "check", "assert", "define", "bucket", "a", "n", "c", "python", "eval", "value", "capture", "expr", "fixed", "i", "o", "b", "h":
		p.skip = "directive"
		switch strings.ToLower(w) {
		case "tag", "payee":
		default:
			p.problem(ln, "warning", "the %s directive is not read", w)
		}
	default:
		p.problem(ln, "error", "%q is not a transaction or a directive this reader knows", w)
		p.skip = "directive"
	}
}

func stripComment(s string) string {
	if at := commentAt(s); at >= 0 {
		s = s[:at]
	}
	return strings.TrimSpace(s)
}

// The first ; that starts a comment: not inside quotes.
func commentAt(s string) int {
	q := false
	for i, r := range s {
		switch r {
		case '"':
			q = !q
		case ';':
			if !q {
				return i
			}
		}
	}
	return -1
}

func (p *parser) setYear(s string, ln int) {
	y, err := strconv.Atoi(strings.TrimSpace(stripComment(s)))
	if err != nil || y < 1 {
		p.problem(ln, "error", "%q is not a year", s)
		return
	}
	p.year = y
}

func (p *parser) commodityFormat(s string, ln int) {
	a, st, rest, err := parseAmount(stripComment(s), "", p.comma)
	if err != nil || strings.TrimSpace(rest) != "" {
		p.problem(ln, "warning", "could not read the commodity format %q", s)
		return
	}
	st.Declared = true
	p.j.Styles[a.Commodity] = &st
}

func (p *parser) addAlias(s string, ln int) {
	eq := strings.Index(s, "=")
	if eq < 0 {
		p.problem(ln, "error", "alias needs NAME=ACCOUNT")
		return
	}
	from, to := strings.TrimSpace(s[:eq]), strings.TrimSpace(s[eq+1:])
	a := alias{from: from, to: to}
	if strings.HasPrefix(from, "/") && strings.HasSuffix(from, "/") && len(from) > 1 {
		re, err := regexp.Compile("(?i)" + from[1:len(from)-1])
		if err != nil {
			p.problem(ln, "error", "alias: %v", err)
			return
		}
		a.re = re
	}
	p.aliases = append(p.aliases, a)
}

func (p *parser) include(path string, ln int, depth int) {
	if path == "" {
		p.problem(ln, "error", "include names no file")
		return
	}
	if p.opt.Open == nil {
		p.problem(ln, "error", "include %s: includes are not read here", path)
		return
	}
	if depth >= 10 {
		p.problem(ln, "error", "include %s: included ten deep, which is a loop", path)
		return
	}
	if strings.HasPrefix(path, "~/") {
		p.problem(ln, "error", "include %s: write it relative to this file", path)
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(p.file), path)
	}
	files := []string{path}
	if strings.ContainsAny(path, "*?[") {
		m, _ := filepath.Glob(path)
		sort.Strings(m)
		files = m
		if len(files) == 0 {
			p.problem(ln, "warning", "include %s matches no file", path)
		}
	}
	for _, f := range files {
		text, err := p.opt.Open(f)
		if err != nil {
			p.problem(ln, "error", "include %s: %v", filepath.Base(f), err)
			continue
		}
		p.j.Included = append(p.j.Included, f)
		p.finish()
		saveSkip := p.skip
		p.chunk(Chunk{File: f, Line: 1, Text: text}, depth+1)
		p.skip = saveSkip
	}
}

func (p *parser) price(s string, ln int) {
	s = stripComment(s)
	ds, rest := firstWord(s)
	d, err := p.date(ds)
	if err != nil {
		p.problem(ln, "error", "P: %v", err)
		return
	}
	// An optional time of day.
	if w, r := firstWord(rest); strings.Count(w, ":") >= 1 && w[0] >= '0' && w[0] <= '9' {
		rest = r
	}
	com, rest := firstWord(rest)
	com = strings.Trim(com, `"`)
	a, st, left, err := parseAmount(rest, p.defCom, p.comma)
	if err != nil || strings.TrimSpace(left) != "" {
		p.problem(ln, "error", "P: could not read the price %q", rest)
		return
	}
	p.j.Styles.learnSide(a.Commodity, st)
	p.j.Prices = append(p.j.Prices, Price{File: p.file, Line: ln, Date: d, Commodity: com, Price: a})
}

// ---------------------------------------------------------------------------
// Transactions
// ---------------------------------------------------------------------------

func (p *parser) date(s string) (time.Time, error) {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '-' || r == '.' })
	nums := []int{}
	for _, x := range parts {
		n, err := strconv.Atoi(x)
		if err != nil {
			return time.Time{}, fmt.Errorf("%q is not a date", s)
		}
		nums = append(nums, n)
	}
	y, m, d := 0, 0, 0
	switch len(nums) {
	case 3:
		y, m, d = nums[0], nums[1], nums[2]
	case 2:
		y = p.year
		if y == 0 {
			y = p.opt.Now.Year()
		}
		m, d = nums[0], nums[1]
	default:
		return time.Time{}, fmt.Errorf("%q is not a date", s)
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d {
		return time.Time{}, fmt.Errorf("%q is not a day there is", s)
	}
	return t, nil
}

var tagListRe = regexp.MustCompile(`(?:^|\s):((?:[^\s:]+:)+)`)
var metaRe = regexp.MustCompile(`^\s*([^\s:]+):\s+(.*)$`)

// readTags adds a comment's :tags: and key: value to tags.
func readTags(comment string, tags map[string]string) {
	if m := metaRe.FindStringSubmatch(comment); m != nil {
		tags[m[1]] = strings.TrimSpace(m[2])
		return
	}
	for _, m := range tagListRe.FindAllStringSubmatch(comment, -1) {
		for _, t := range strings.Split(strings.Trim(m[1], ":"), ":") {
			if t != "" {
				tags[t] = ""
			}
		}
	}
}

func (p *parser) header(line string, ln int) {
	head, note := line, ""
	if at := headerCommentAt(line); at >= 0 {
		head, note = line[:at], strings.TrimSpace(line[at+1:])
	}
	ds, rest := firstWord(head)
	aux := ""
	if eq := strings.Index(ds, "="); eq >= 0 {
		ds, aux = ds[:eq], ds[eq+1:]
	}
	d, err := p.date(ds)
	if err != nil {
		p.problem(ln, "error", "%v", err)
		p.skip = "bad"
		return
	}
	t := &Transaction{File: p.file, Line: ln, EndLine: ln, Date: d, Tags: map[string]string{}}
	if aux != "" {
		a, err := p.date(aux)
		if err != nil {
			p.problem(ln, "error", "the auxiliary date: %v", err)
		} else {
			t.Aux = &a
		}
	}
	rest = strings.TrimSpace(rest)
	if rest != "" && (rest[0] == '*' || rest[0] == '!') {
		t.Status, rest = rest[:1], strings.TrimSpace(rest[1:])
	}
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end > 0 {
			t.Code, rest = rest[1:end], strings.TrimSpace(rest[end+1:])
		}
	}
	t.Payee = strings.TrimSpace(rest)
	// hledger's payee | note.
	if bar := strings.Index(t.Payee, "|"); bar >= 0 {
		t.Payee = strings.TrimSpace(t.Payee[:bar])
	}
	if note != "" {
		t.Note = note
		readTags(note, t.Tags)
	}
	p.cur = t
	p.last = nil
}

// A header's comment starts at a ; with white space before it (a payee may
// have a ; in it otherwise - "Joe's;Bar" is rare, but ledger reads it so).
func headerCommentAt(s string) int {
	for i := 1; i < len(s); i++ {
		if s[i] == ';' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

func (p *parser) indentedInTxn(trim string, ln int) {
	t := p.cur
	t.EndLine = ln
	if trim[0] == ';' || trim[0] == '#' {
		c := strings.TrimSpace(trim[1:])
		if p.last != nil {
			if p.last.Note != "" {
				p.last.Note += "\n"
			}
			p.last.Note += c
			readTags(c, p.last.Tags)
		} else {
			if t.Note != "" {
				t.Note += "\n"
			}
			t.Note += c
			readTags(c, t.Tags)
		}
		return
	}
	po, err := p.posting(trim, ln)
	if err != nil {
		p.problem(ln, "error", "%v", err)
		return
	}
	t.Postings = append(t.Postings, po)
	p.last = po
}

func (p *parser) account(name string) string {
	name = strings.TrimSpace(name)
	for _, a := range p.aliases {
		if a.re != nil {
			if a.re.MatchString(name) {
				name = a.re.ReplaceAllString(name, a.to)
			}
			continue
		}
		if strings.EqualFold(name, a.from) {
			name = a.to
		} else if strings.HasPrefix(strings.ToLower(name), strings.ToLower(a.from)+":") {
			name = a.to + name[len(a.from):]
		}
	}
	for i := len(p.apply) - 1; i >= 0; i-- {
		if p.apply[i] != "" {
			name = p.apply[i] + ":" + name
		}
	}
	return name
}

func (p *parser) posting(s string, ln int) (*Posting, error) {
	po := &Posting{Line: ln, Tags: map[string]string{}}
	if len(s) > 1 && (s[0] == '*' || s[0] == '!') && (s[1] == ' ' || s[1] == '\t') {
		po.Status, s = s[:1], strings.TrimSpace(s[1:])
	}
	if at := commentAt(s); at >= 0 {
		po.Note = strings.TrimSpace(s[at+1:])
		readTags(po.Note, po.Tags)
		s = strings.TrimRight(s[:at], " \t")
	}
	var acct, rest string
	switch s[0] {
	case '(', '[':
		closer := map[byte]string{'(': ")", '[': "]"}[s[0]]
		end := strings.Index(s, closer)
		if end < 0 {
			return nil, fmt.Errorf("a virtual account with no closing %s", closer)
		}
		po.Virtual, acct, rest = s[:1], s[1:end], s[end+1:]
	default:
		end := len(s)
		if at := strings.Index(s, "  "); at >= 0 {
			end = at
		}
		if at := strings.Index(s, "\t"); at >= 0 && at < end {
			end = at
		}
		acct, rest = s[:end], s[end:]
	}
	po.Account = p.account(acct)
	if po.Account == "" {
		return nil, fmt.Errorf("a posting with no account")
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		po.Elided = true
		return po, nil
	}
	if rest[0] == '(' {
		return nil, fmt.Errorf("value expressions like %s are not read; write the amount", rest)
	}
	if rest[0] != '=' {
		a, st, left, err := parseAmount(rest, p.defCom, p.comma)
		if err != nil {
			return nil, err
		}
		p.j.Styles.learn(a.Commodity, st)
		po.Amount = a
		rest = strings.TrimSpace(left)
		// A lot annotation: {price}, {{total}}, [date], (note).
		for rest != "" && (rest[0] == '{' || rest[0] == '[' || rest[0] == '(') {
			closer := map[byte]string{'{': "}", '[': "]", '(': ")"}[rest[0]]
			total := strings.HasPrefix(rest, "{{")
			if total {
				closer = "}}"
			}
			end := strings.Index(rest, closer)
			if end < 0 {
				return nil, fmt.Errorf("an annotation with no closing %s", closer)
			}
			inner := rest[1:end]
			if total {
				inner = rest[2:end]
			}
			if rest[0] == '{' {
				inner = strings.TrimPrefix(strings.TrimSpace(inner), "=")
				c, cst, l, err := parseAmount(inner, p.defCom, p.comma)
				if err != nil || strings.TrimSpace(l) != "" {
					return nil, fmt.Errorf("could not read the lot price {%s}", inner)
				}
				p.j.Styles.learnSide(c.Commodity, cst)
				po.Cost = costOf(a, c, total)
			}
			rest = strings.TrimSpace(rest[end+len(closer):])
		}
		if strings.HasPrefix(rest, "@") {
			total := strings.HasPrefix(rest, "@@")
			rest = strings.TrimLeft(rest, "@")
			c, cst, left, err := parseAmount(rest, p.defCom, p.comma)
			if err != nil {
				return nil, fmt.Errorf("the cost: %v", err)
			}
			p.j.Styles.learnSide(c.Commodity, cst)
			po.Cost = costOf(a, c, total)
			if a.Q.Sign() != 0 {
				unit := new(big.Rat).Quo(new(big.Rat).Abs(po.Cost.Q), new(big.Rat).Abs(a.Q))
				p.j.Prices = append(p.j.Prices, Price{File: p.file, Line: ln, Date: p.cur.Date, Commodity: a.Commodity, Price: Amount{c.Commodity, unit}})
			}
			rest = strings.TrimSpace(left)
		}
	} else {
		po.assign = true
	}
	if strings.HasPrefix(rest, "=") {
		rest = rest[1:]
		if strings.HasPrefix(rest, "=") {
			po.AssertOnly, rest = true, rest[1:]
		}
		if strings.HasPrefix(rest, "*") {
			po.AssertTree, rest = true, rest[1:]
		}
		a, st, left, err := parseAmount(rest, p.defCom, p.comma)
		if err != nil {
			return nil, fmt.Errorf("the balance assertion: %v", err)
		}
		p.j.Styles.learn(a.Commodity, st)
		po.Assert = &a
		rest = strings.TrimSpace(left)
	}
	if rest != "" {
		return nil, fmt.Errorf("could not read %q after the amount", rest)
	}
	return po, nil
}

// The total an amount cost: unit price times quantity, or the total as
// given, signed as the quantity is.
func costOf(a, price Amount, total bool) *Amount {
	q := new(big.Rat)
	if total {
		q.Abs(price.Q)
		if a.Q.Sign() < 0 {
			q.Neg(q)
		}
	} else {
		q.Mul(price.Q, a.Q)
	}
	return &Amount{price.Commodity, q}
}

// ---------------------------------------------------------------------------
// Balancing and asserting
// ---------------------------------------------------------------------------

// The balance an assertion checks: the account's own, or with its
// subaccounts' for =*.
func (p *parser) balanceOf(acct string, tree bool, extra Mixed) Mixed {
	out := Mixed{}
	if !tree {
		if b, ok := p.bal[acct]; ok {
			out.AddMixed(b)
		}
	} else {
		for a, b := range p.bal {
			if a == acct || strings.HasPrefix(a, acct+":") {
				out.AddMixed(b)
			}
		}
	}
	out.AddMixed(extra)
	return out
}

func (p *parser) finish() {
	t := p.cur
	if t == nil {
		return
	}
	p.cur, p.last = nil, nil
	if len(t.Postings) == 0 {
		p.problem(t.Line, "error", "a transaction with no postings")
		return
	}
	// Balance assignments: the amount is what brings the account to the
	// asserted balance, counting this transaction's earlier postings to it.
	pending := map[string]Mixed{}
	for _, po := range t.Postings {
		if po.assign {
			have := p.balanceOf(po.Account, po.AssertTree, pending[po.Account])
			po.Amount = Amount{po.Assert.Commodity, new(big.Rat).Sub(po.Assert.Q, have.Get(po.Assert.Commodity))}
		}
		if !po.Elided {
			if pending[po.Account] == nil {
				pending[po.Account] = Mixed{}
			}
			pending[po.Account].AddAmount(po.Amount)
		}
	}
	// What the balancing postings come to, in the commodity each one
	// balances in: its cost when it has one.
	sum := Mixed{}
	var elided []*Posting
	costs := false
	for _, po := range t.Postings {
		if po.Virtual == "(" {
			if po.Elided {
				p.problem(po.Line, "error", "a virtual (unbalanced) posting needs an amount")
				po.Elided = false
				po.Amount = Amount{"", new(big.Rat)}
			}
			continue
		}
		if po.Elided {
			elided = append(elided, po)
			continue
		}
		if po.Cost != nil {
			costs = true
			sum.AddAmount(*po.Cost)
		} else {
			sum.AddAmount(po.Amount)
		}
	}
	residual := Mixed{}
	for _, c := range sum.Commodities() {
		if !p.j.Styles.nearZero(c, sum[c]) {
			residual[c] = sum[c]
		}
	}
	switch {
	case len(elided) > 1:
		p.problem(t.Line, "error", "only one posting may leave its amount out (lines %d and %d both do)", elided[0].Line, elided[1].Line)
		for _, po := range elided {
			po.Amount = Amount{"", new(big.Rat)}
		}
	case len(elided) == 1:
		po := elided[0]
		cs := residual.Commodities()
		if len(cs) == 0 {
			po.Amount = Amount{p.defCom, new(big.Rat)}
		} else {
			po.Amount = Amount{cs[0], new(big.Rat).Neg(residual[cs[0]])}
			// Ledger splits the elided posting into one per commodity.
			at := 0
			for k, x := range t.Postings {
				if x == po {
					at = k
				}
			}
			for _, c := range cs[1:] {
				cp := *po
				cp.Amount = Amount{c, new(big.Rat).Neg(residual[c])}
				cp.Tags = po.Tags
				t.Postings = append(t.Postings[:at+1], append([]*Posting{&cp}, t.Postings[at+1:]...)...)
				at++
			}
		}
	case len(residual) == 2 && !costs:
		// Two commodities and nothing saying what one cost in the other:
		// ledger takes the transaction as the exchange rate.
		cs := residual.Commodities()
		if residual[cs[0]].Sign() == residual[cs[1]].Sign() {
			p.problem(t.Line, "error", "the transaction does not balance: off by %s", strings.Join(p.j.Styles.FormatMixed(residual), ", "))
		}
	case len(residual) > 0:
		p.problem(t.Line, "error", "the transaction does not balance: off by %s", strings.Join(p.j.Styles.FormatMixed(residual), ", "))
	}
	// Apply, then check each assertion as its posting is reached.
	for _, po := range t.Postings {
		if p.bal[po.Account] == nil {
			p.bal[po.Account] = Mixed{}
		}
		p.bal[po.Account].AddAmount(po.Amount)
		if po.Assert == nil || po.assign {
			continue
		}
		have := p.balanceOf(po.Account, po.AssertTree, nil)
		want := po.Assert
		c := want.Commodity
		if c == "" && want.Q.Sign() == 0 {
			// `= 0`: nothing in any commodity.
			if !have.IsZero() {
				p.problem(po.Line, "error", "balance assertion failed: %s is %s, not 0", po.Account, strings.Join(p.j.Styles.FormatMixed(have), ", "))
			}
			continue
		}
		got := have.Get(c)
		if !p.j.Styles.nearZero(c, new(big.Rat).Sub(got, want.Q)) {
			p.problem(po.Line, "error", "balance assertion failed: %s is %s, not %s", po.Account, p.j.Styles.Format(c, got), p.j.Styles.Format(c, want.Q))
			continue
		}
		if po.AssertOnly {
			for _, o := range have.Commodities() {
				if o != c {
					p.problem(po.Line, "error", "balance assertion failed: %s also holds %s", po.Account, p.j.Styles.Format(o, have[o]))
				}
			}
		}
	}
	p.j.Transactions = append(p.j.Transactions, t)
}

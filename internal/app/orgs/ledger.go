package orgs

// ledger-mode and ob-ledger: ledger and hledger journals read by orgs itself
// (internal/common/ledger), so balances, registers, monthly totals and
// checks work without either program installed. Only with features.ledger
// on (features.go).
//
// A source is an org file's ledger/hledger source blocks, read in order as
// one journal, or a .ledger/.journal/.hledger/.dat file under the org
// directories. One block alone (the way ob-ledger runs a block) is
// source=FILE&block=N (or the block's #+NAME:). Every line a problem names
// is the line of the file it was written in: an org file's own line for a
// block.

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common/ledger"
)


func registerLedgerRoutes(api *mux.Router) {
	api.HandleFunc("/ledger/sources", RequestLedgerSources).Methods("GET")
	api.HandleFunc("/ledger/balance", RequestLedgerBalance).Methods("GET")
	api.HandleFunc("/ledger/register", RequestLedgerRegister).Methods("GET")
	api.HandleFunc("/ledger/monthly", RequestLedgerMonthly).Methods("GET")
	api.HandleFunc("/ledger/check", RequestLedgerCheck).Methods("GET")
	api.HandleFunc("/ledger/check", PostLedgerCheck).Methods("POST")
}

func init() {
	// hledger is its own program, not another name for ledger: a block
	// that says hledger runs hledger when it is installed.
	runners["hledger"] = runner{cmd: []string{"hledger", "-f"}, ext: ".journal", defaultCollect: "output", defaultCmdline: "bal"}
	delete(runnerAliases, "hledger")
	dynGenerators["ledger"] = dynLedger
}

// ---------------------------------------------------------------------------
// Sources
// ---------------------------------------------------------------------------

// LedgerBlock is one ledger block of an org file, for a picker.
type LedgerBlock struct {
	Index int
	Name  string `json:",omitempty"`
	Lang  string
	Line  int
}

// LedgerSource is a journal a report can be over.
type LedgerSource struct {
	Name         string // relative to its org directory
	File         string
	Kind         string // "org" or "journal"
	Blocks       []LedgerBlock `json:",omitempty"`
	Transactions int
	Errors       int
	Warnings     int
	First        string `json:",omitempty"`
	Last         string `json:",omitempty"`
	// A journal another source includes: not counted again in "all".
	IncludedBy string `json:",omitempty"`
}

var journalExts = map[string]bool{".ledger": true, ".journal": true, ".hledger": true, ".dat": true}

type ledgerEntry struct {
	mod    time.Time
	size   int64
	deps   map[string]time.Time
	blocks []ledger.OrgBlock
	j      *ledger.Journal
}

var ledgerCache = struct {
	sync.Mutex
	m map[string]*ledgerEntry
}{m: map[string]*ledgerEntry{}}

func ledgerNow() time.Time { return time.Now() }

// ledgerOpen reads an included file, only from inside the org directories.
func ledgerOpen(path string) (string, error) {
	p, ok := insideOrgDirs(path)
	if !ok {
		return "", fmt.Errorf("%s is not inside the org directories", path)
	}
	b, err := os.ReadFile(p)
	return string(b), err
}

func ledgerOptions() ledger.Options { return ledger.Options{Open: ledgerOpen, Now: ledgerNow()} }

func modOf(path string) (time.Time, int64, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}, 0, false
	}
	return st.ModTime(), st.Size(), true
}

// readLedgerFile is a file's journal, read again only when it or a file it
// includes has changed. An org file with no ledger blocks has a nil journal.
func readLedgerFile(path string) (*ledgerEntry, error) {
	mod, size, ok := modOf(path)
	if !ok {
		return nil, fmt.Errorf("could not read %s", path)
	}
	ledgerCache.Lock()
	e := ledgerCache.m[path]
	ledgerCache.Unlock()
	if e != nil && e.mod.Equal(mod) && e.size == size {
		fresh := true
		for d, m := range e.deps {
			if dm, _, ok := modOf(d); !ok || !dm.Equal(m) {
				fresh = false
			}
		}
		if fresh {
			return e, nil
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	e = &ledgerEntry{mod: mod, size: size, deps: map[string]time.Time{}}
	if strings.EqualFold(filepath.Ext(path), ".org") {
		e.blocks = ledger.OrgBlocks(string(b))
		if len(e.blocks) > 0 {
			e.j = ledger.Parse(ledger.Chunks(path, e.blocks), ledgerOptions())
		}
	} else {
		e.j = ledger.ParseText(path, string(b), ledgerOptions())
	}
	if e.j != nil {
		for _, d := range e.j.Included {
			if m, _, ok := modOf(d); ok {
				e.deps[d] = m
			}
		}
	}
	ledgerCache.Lock()
	ledgerCache.m[path] = e
	ledgerCache.Unlock()
	return e, nil
}

func relToOrgDir(path string) string {
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.Base(path)
}

// ledgerFiles is every file that may be a source: the org files the
// database holds, and the journal files under the org directories.
func ledgerFiles() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, f := range GetDb().GetFiles() {
		if a, err := filepath.Abs(f); err == nil && !seen[a] && strings.EqualFold(filepath.Ext(a), ".org") {
			seen[a] = true
			out = append(out, a)
		}
	}
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if de.IsDir() {
				if p != root && (strings.HasPrefix(de.Name(), ".") || de.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if journalExts[strings.ToLower(filepath.Ext(p))] && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func ledgerSources() []LedgerSource {
	out := []LedgerSource{}
	includedBy := map[string]string{}
	for _, f := range ledgerFiles() {
		e, err := readLedgerFile(f)
		if err != nil || e.j == nil {
			continue
		}
		kind := "journal"
		if strings.EqualFold(filepath.Ext(f), ".org") {
			kind = "org"
		}
		// A .dat file is anybody's data; it is a journal only when it reads
		// as one.
		if strings.EqualFold(filepath.Ext(f), ".dat") && (len(e.j.Transactions) == 0 || e.j.Errors() > 0) {
			continue
		}
		s := LedgerSource{Name: relToOrgDir(f), File: f, Kind: kind, Transactions: len(e.j.Transactions)}
		for _, b := range e.blocks {
			s.Blocks = append(s.Blocks, LedgerBlock{Index: b.Index, Name: b.Name, Lang: b.Lang, Line: b.Begin})
		}
		for _, p := range e.j.Problems {
			if p.Severity == "error" {
				s.Errors++
			} else {
				s.Warnings++
			}
		}
		for _, t := range e.j.Transactions {
			d := t.Date.Format("2006-01-02")
			if s.First == "" || d < s.First {
				s.First = d
			}
			if d > s.Last {
				s.Last = d
			}
		}
		for _, inc := range e.j.Included {
			includedBy[inc] = s.Name
		}
		out = append(out, s)
	}
	for i := range out {
		out[i].IncludedBy = includedBy[out[i].File]
	}
	return out
}

// loadLedger is the journal a request asks for: one source (by name, base
// name or path), one block of it, or every source together.
func loadLedger(source, block string) (*ledger.Journal, string, error) {
	source = strings.TrimSpace(source)
	if source == "" || source == "all" {
		if block != "" {
			return nil, "", fmt.Errorf("block= needs a source=")
		}
		all := &ledger.Journal{Styles: ledger.Styles{}, Problems: []ledger.Problem{}}
		srcs := ledgerSources()
		if len(srcs) == 0 {
			return nil, "", fmt.Errorf("no ledger sources: no org file has a ledger block and no .ledger or .journal file is under the org directories")
		}
		for _, s := range srcs {
			if s.IncludedBy != "" {
				continue
			}
			e, err := readLedgerFile(s.File)
			if err != nil {
				return nil, "", err
			}
			mergeJournal(all, e.j)
		}
		return all, "all", nil
	}
	var match *LedgerSource
	srcs := ledgerSources()
	for i, s := range srcs {
		if s.Name == source || s.File == source || filepath.Base(s.File) == source {
			match = &srcs[i]
			break
		}
	}
	if match == nil {
		// A file that is not (yet) a source, named by path.
		if p, ok := insideOrgDirs(source); ok {
			match = &LedgerSource{Name: relToOrgDir(p), File: p}
		} else {
			return nil, "", fmt.Errorf("no ledger source called %q (orgs ledger sources lists them)", source)
		}
	}
	e, err := readLedgerFile(match.File)
	if err != nil {
		return nil, "", err
	}
	if e.j == nil {
		return nil, "", fmt.Errorf("%s has no ledger or hledger source blocks", match.Name)
	}
	if block == "" {
		return e.j, match.Name, nil
	}
	b, err := pickBlock(e.blocks, block)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %v", match.Name, err)
	}
	j := ledger.Parse(ledger.Chunks(match.File, []ledger.OrgBlock{b}), ledgerOptions())
	return j, fmt.Sprintf("%s#%d", match.Name, b.Index), nil
}

// pickBlock finds a block by its index, its #+NAME: or a line inside it.
func pickBlock(blocks []ledger.OrgBlock, which string) (ledger.OrgBlock, error) {
	for _, b := range blocks {
		if b.Name != "" && b.Name == which {
			return b, nil
		}
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(which, "line:")); err == nil {
		if strings.HasPrefix(which, "line:") {
			for _, b := range blocks {
				if n >= b.Begin && n <= b.End {
					return b, nil
				}
			}
			return ledger.OrgBlock{}, fmt.Errorf("no ledger block at line %d", n)
		}
		if n >= 0 && n < len(blocks) {
			return blocks[n], nil
		}
	}
	return ledger.OrgBlock{}, fmt.Errorf("no ledger block %q (there are %d)", which, len(blocks))
}

func mergeJournal(into, from *ledger.Journal) {
	if from == nil {
		return
	}
	into.Transactions = append(into.Transactions, from.Transactions...)
	into.Prices = append(into.Prices, from.Prices...)
	into.Accounts = append(into.Accounts, from.Accounts...)
	into.Problems = append(into.Problems, from.Problems...)
	into.Included = append(into.Included, from.Included...)
	// One journal's $20 and another's $1,000.00 are one commodity: written
	// the declared way when either declares it, else with the most places.
	for c, st := range from.Styles {
		have, ok := into.Styles[c]
		switch {
		case !ok || (st.Declared && !have.Declared):
			cp := *st
			into.Styles[c] = &cp
		case !have.Declared:
			if st.Precision > have.Precision {
				have.Precision = st.Precision
			}
			have.Thousands = have.Thousands || st.Thousands
		}
	}
}

// ---------------------------------------------------------------------------
// Requests
// ---------------------------------------------------------------------------

// The filter a request's parameters ask for: period, begin, end, account
// (repeated, or several words), depth, real, aux.
func ledgerFilter(get func(string) string, all func(string) []string) (ledger.Filter, error) {
	f := ledger.Filter{}
	p, err := ledger.Within(get("period"), get("begin"), get("end"), ledgerNow())
	if err != nil {
		return f, err
	}
	f.Period = p
	for _, a := range all("account") {
		f.Accounts = append(f.Accounts, strings.Fields(a)...)
	}
	if d := get("depth"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < 0 {
			return f, fmt.Errorf("depth=%s is not a number", d)
		}
		f.Depth = n
	}
	f.Real = dynTrue(get("real"))
	f.Aux = dynTrue(get("aux"))
	return f, nil
}

type ledgerReq struct {
	j      *ledger.Journal
	source string
	f      ledger.Filter
}

func ledgerRequest(w http.ResponseWriter, r *http.Request) (*ledgerReq, bool) {
	if featureOff(w, "ledger") {
		return nil, false
	}
	q := r.URL.Query()
	j, name, err := loadLedger(q.Get("source"), q.Get("block"))
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return nil, false
	}
	f, err := ledgerFilter(q.Get, func(k string) []string { return q[k] })
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return nil, false
	}
	return &ledgerReq{j, name, f}, true
}

// LedgerAnswer is what every report carries besides its rows.
type LedgerAnswer struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	Source   string
	Period   string `json:",omitempty"`
	Problems []ledger.Problem
	// The report as ledger would print it.
	Text string
}

func answerOf(lr *ledgerReq) LedgerAnswer {
	return LedgerAnswer{Ok: true, Source: lr.source, Period: lr.f.Period.String(), Problems: lr.j.Problems}
}

/* SDOC: API
* GET /ledger/sources — The Journals There Are

	#+BEGIN_EXAMPLE
	GET /ledger/sources
	-> {"Ok": true, "Sources": [{"Name": "money.org", "File": "/home/me/org/money.org",
	    "Kind": "org", "Blocks": [{"Index": 0, "Name": "opening", "Lang": "ledger", "Line": 12}],
	    "Transactions": 41, "Errors": 0, "Warnings": 1, "First": "2026-01-01", "Last": "2026-11-03"}]}
	#+END_EXAMPLE

	Every org file with =ledger= or =hledger= source blocks, and every
	=.ledger=, =.journal=, =.hledger= and =.dat= file under the org
	directories (a =.dat= file only when it reads as a journal). A journal
	another one includes says =IncludedBy=, and is not counted twice when a
	report is over every source. Needs =features: ledger:=.
EDOC */
func RequestLedgerSources(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "ledger") {
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Sources": ledgerSources()})
}

/* SDOC: API
* GET /ledger/balance — Balances, As An Account Tree

	#+BEGIN_EXAMPLE
	GET /ledger/balance?source=money.org&period=this+year&account=Expenses&depth=2
	-> {"Ok": true, "Source": "money.org", "Period": "2026-01-01 to 2026-12-31",
	    "Rows": [{"Account": "Expenses:Food", "Name": "Food", "Parent": "Expenses", "Depth": 1,
	              "Total": [{"Commodity": "$", "Quantity": "50", "Text": "$50.00", "Number": 50}],
	              "Own": [], "Postings": 0, "Children": true}, ...],
	    "Total": [...], "Problems": [], "Text": "   $850.00  Expenses\n..."}
	#+END_EXAMPLE

	The parameters every =/ledger/= report takes:

	| =source= | a source's name, file name or path; none (or =all=) for every source |
	| =block=  | one block of an org file alone, as ob-ledger runs it: its index from 0, its =#+NAME:=, or =line:N= |
	| =period= | ledger's =-p= in words: =this month=, =last year=, =2026=, =2026-11=, =q3=, =from 2026-01-01 to 2026-04-01= |
	| =begin=, =end= | either end alone (the end is not included) |
	| =account= | account regular expressions, any of which may match; =not= before one (or =!=) leaves it out |
	| =depth=  | fold accounts below this depth into their parent |
	| =real=   | only real postings |
	| =aux=    | use the auxiliary =date=aux= dates |

	=Rows= is every account in the tree, parents first, with its total
	(subaccounts included) and its own postings. =Text= is the report as
	=ledger bal= prints it. =Problems= are the journal's errors and warnings,
	each at its file and line.
EDOC */
func RequestLedgerBalance(w http.ResponseWriter, r *http.Request) {
	lr, ok := ledgerRequest(w, r)
	if !ok {
		return
	}
	rows, total, err := lr.j.Balance(lr.f)
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	a := answerOf(lr)
	if dynTrue(r.URL.Query().Get("flat")) {
		a.Text = lr.j.FormatFlat(rows, total)
	} else {
		a.Text = lr.j.FormatBalance(rows, total)
	}
	dynJson(w, struct {
		LedgerAnswer
		Rows  []ledger.BalanceRow
		Total []ledger.Amt
	}{a, rows, lr.j.Styles.Amts(total)})
}

/* SDOC: API
* GET /ledger/register — Postings With A Running Total

	#+BEGIN_EXAMPLE
	GET /ledger/register?source=money.org&account=^Assets:Bank$&period=last+month
	-> {"Ok": true, "Rows": [{"Date": "2026-10-05", "Status": "*", "Payee": "Grocer",
	    "Account": "Assets:Bank", "Amount": [...], "Total": [...],
	    "File": "/home/me/org/money.org", "Line": 23, "TxLine": 20, "First": true}, ...],
	    "Text": "26-Oct-05 Grocer ..."}
	#+END_EXAMPLE

	The postings the filter takes in date order, each with the running total
	after it and the file and line it was written on. Takes the parameters
	=/ledger/balance= does.
EDOC */
func RequestLedgerRegister(w http.ResponseWriter, r *http.Request) {
	lr, ok := ledgerRequest(w, r)
	if !ok {
		return
	}
	rows, err := lr.j.Register(lr.f)
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	a := answerOf(lr)
	a.Text = lr.j.FormatRegister(rows)
	dynJson(w, struct {
		LedgerAnswer
		Rows []ledger.RegisterRow
	}{a, rows})
}

/* SDOC: API
* GET /ledger/monthly — Totals A Month, For A Chart

	#+BEGIN_EXAMPLE
	GET /ledger/monthly?source=money.org&account=^Expenses&depth=2&period=this+year
	-> {"Ok": true, "Months": ["2026-01", ..., "2026-12"],
	    "Series": [{"Account": "Expenses:Food", "Commodity": "$", "Values": [120, 98.5, ...],
	                "Texts": ["$120.00", ...], "Total": "$1,204.50"}, ...]}
	#+END_EXAMPLE

	The postings summed by month, a series an account (cut to =depth=) and
	commodity; with no =depth=, everything the filter takes as one series.
	The months run across the period when it has both ends, else from the
	first posting to the last.
EDOC */
func RequestLedgerMonthly(w http.ResponseWriter, r *http.Request) {
	lr, ok := ledgerRequest(w, r)
	if !ok {
		return
	}
	months, series, err := lr.j.Monthly(lr.f)
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	a := answerOf(lr)
	a.Text = ledger.FormatMonthly(months, series)
	dynJson(w, struct {
		LedgerAnswer
		Months []string
		Series []ledger.Series
	}{a, months, series})
}

/* SDOC: API
* GET /ledger/check — Problems In A Journal
* POST /ledger/check — Problems In Text Not Yet Saved

	#+BEGIN_EXAMPLE
	GET  /ledger/check?source=money.org
	POST /ledger/check   {"File": "/home/me/org/money.org", "Text": "* Money\n#+BEGIN_SRC ledger\n..."}
	-> {"Ok": true, "Transactions": 41, "Problems": [{"File": "/home/me/org/money.org", "Line": 31,
	    "Msg": "the transaction does not balance: off by $1.00", "Severity": "error"}]}
	#+END_EXAMPLE

	Unbalanced transactions, failed balance assertions, dates that are not
	days, amounts that do not read, and (as warnings) what the reader skips:
	periodic and automated transactions, directives it does not know. The
	POST reads the text it is given - an org file's ledger blocks as one
	journal, or a journal when =File= is not an org file - which is what an
	editor checks a buffer with; its includes are read relative to =File=,
	and only inside the org directories. =Block= (an index) reads one block
	alone.
EDOC */
func RequestLedgerCheck(w http.ResponseWriter, r *http.Request) {
	lr, ok := ledgerRequest(w, r)
	if !ok {
		return
	}
	a := answerOf(lr)
	a.Text = checkText(lr.j)
	dynJson(w, struct {
		LedgerAnswer
		Transactions int
	}{a, len(lr.j.Transactions)})
}

func checkText(j *ledger.Journal) string {
	if len(j.Problems) == 0 {
		return fmt.Sprintf("%d transactions, no problems\n", len(j.Transactions))
	}
	lines := []string{}
	for _, p := range j.Problems {
		lines = append(lines, fmt.Sprintf("%s:%d: %s: %s", relToOrgDir(p.File), p.Line, p.Severity, p.Msg))
	}
	return strings.Join(lines, "\n") + "\n"
}

func PostLedgerCheck(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "ledger") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		File  string
		Text  string
		Block *int
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	file := req.File
	if file == "" {
		file = "buffer.org"
	}
	var j *ledger.Journal
	var blocks []ledger.OrgBlock
	if strings.EqualFold(filepath.Ext(file), ".org") || filepath.Ext(file) == "" {
		blocks = ledger.OrgBlocks(req.Text)
		use := blocks
		if req.Block != nil {
			if *req.Block < 0 || *req.Block >= len(blocks) {
				dynJson(w, map[string]interface{}{"Ok": false, "Msg": fmt.Sprintf("no ledger block %d", *req.Block)})
				return
			}
			use = blocks[*req.Block : *req.Block+1]
		}
		j = ledger.Parse(ledger.Chunks(file, use), ledgerOptions())
	} else {
		j = ledger.ParseText(file, req.Text, ledgerOptions())
	}
	info := []LedgerBlock{}
	for _, b := range blocks {
		info = append(info, LedgerBlock{Index: b.Index, Name: b.Name, Lang: b.Lang, Line: b.Begin})
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Source": file, "Transactions": len(j.Transactions),
		"Problems": j.Problems, "Blocks": info, "Text": checkText(j)})
}

// ---------------------------------------------------------------------------
// The dynamic block
// ---------------------------------------------------------------------------

// dynLedger is #+BEGIN: ledger :report balance :source "money.org"
// :account Expenses :period "this year" :depth 2, as an org table.
func dynLedger(c *dynCtx) ([]string, error) {
	if !featureOn("ledger") {
		return nil, fmt.Errorf("ledger is turned off on this server: turn it on with `orgs features ledger on`")
	}
	source := c.param("source", "")
	if source == "" && c.File != nil {
		// The file the block is in, when it keeps a journal; else every one.
		if abs, err := filepath.Abs(c.File.Filename); err == nil {
			if e, err := readLedgerFile(abs); err == nil && e.j != nil {
				source = abs
			}
		}
	} else if source != "" && !filepath.IsAbs(source) && c.File != nil {
		// Relative to the file the block is in, when it is there.
		near := filepath.Join(filepath.Dir(c.File.Filename), source)
		if _, err := os.Stat(near); err == nil {
			source, _ = filepath.Abs(near)
		}
	}
	j, _, err := loadLedger(source, c.param("block", ""))
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	for k, v := range c.Block.Params {
		params[k] = v
	}
	f, err := ledgerFilter(func(k string) string { return params[k] },
		func(k string) []string { return dynList(params[k]) })
	if err != nil {
		return nil, err
	}
	var rows [][]string
	switch strings.ToLower(c.param("report", "balance")) {
	case "balance", "bal":
		rows, err = balanceTable(j, f)
	case "register", "reg":
		rows, err = registerTable(j, f)
	case "monthly":
		rows, err = monthlyTable(j, f)
	case "check":
		rows = [][]string{{"File", "Line", "Problem"}, nil}
		for _, p := range j.Problems {
			rows = append(rows, []string{relToOrgDir(p.File), strconv.Itoa(p.Line), p.Severity + ": " + p.Msg})
		}
		if len(j.Problems) == 0 {
			return []string{fmt.Sprintf("%d transactions, no problems.", len(j.Transactions))}, nil
		}
	default:
		return nil, fmt.Errorf(":report %s - it is balance, register, monthly or check", c.param("report", ""))
	}
	if err != nil {
		return nil, err
	}
	return orgTableLines(rows), nil
}

func cellsOf(a []ledger.Amt) []string {
	out := []string{}
	for _, x := range a {
		out = append(out, x.Text)
	}
	if len(out) == 0 {
		out = []string{"0"}
	}
	return out
}

var ledgerPipeRe = regexp.MustCompile(`\|`)

func ledgerCell(s string) string { return ledgerPipeRe.ReplaceAllString(s, "¦") }

func balanceTable(j *ledger.Journal, f ledger.Filter) ([][]string, error) {
	rows, total, err := j.Balance(f)
	if err != nil {
		return nil, err
	}
	out := [][]string{{"Account", "Balance"}, nil}
	for _, r := range rows {
		name := r.Account
		if r.Depth > 0 {
			name = `\_` + strings.Repeat("  ", r.Depth) + r.Name
		}
		for k, t := range cellsOf(r.Total) {
			n := ""
			if k == 0 {
				n = name
			}
			out = append(out, []string{ledgerCell(n), ledgerCell(t)})
		}
	}
	out = append(out, nil)
	for k, t := range j.Styles.FormatMixed(total) {
		n := ""
		if k == 0 {
			n = "*Total*"
		}
		out = append(out, []string{n, ledgerCell(t)})
	}
	return out, nil
}

func registerTable(j *ledger.Journal, f ledger.Filter) ([][]string, error) {
	rows, err := j.Register(f)
	if err != nil {
		return nil, err
	}
	out := [][]string{{"Date", "Payee", "Account", "Amount", "Total"}, nil}
	for _, r := range rows {
		am, tot := cellsOf(r.Amount), cellsOf(r.Total)
		n := len(am)
		if len(tot) > n {
			n = len(tot)
		}
		for k := 0; k < n; k++ {
			row := []string{"", "", "", "", ""}
			if k == 0 {
				row[0], row[1], row[2] = r.Date, ledgerCell(r.Payee), ledgerCell(r.Account)
				if !r.First {
					row[0], row[1] = "", ""
				}
			}
			if k < len(am) {
				row[3] = ledgerCell(am[k])
			}
			if k < len(tot) {
				row[4] = ledgerCell(tot[k])
			}
			out = append(out, row)
		}
	}
	return out, nil
}

func monthlyTable(j *ledger.Journal, f ledger.Filter) ([][]string, error) {
	months, series, err := j.Monthly(f)
	if err != nil {
		return nil, err
	}
	head := []string{"Month"}
	for _, s := range series {
		head = append(head, ledgerCell(ledger.SeriesLabel(s, series)))
	}
	out := [][]string{head, nil}
	for i, m := range months {
		row := []string{m}
		for _, s := range series {
			row = append(row, ledgerCell(s.Texts[i]))
		}
		out = append(out, row)
	}
	out = append(out, nil)
	tot := []string{"*Total*"}
	for _, s := range series {
		tot = append(tot, ledgerCell(s.Total))
	}
	return append(out, tot), nil
}

// ---------------------------------------------------------------------------
// Babel, when ledger is not installed
// ---------------------------------------------------------------------------

// nativeLedgerRun answers a ledger or hledger block itself when the feature
// is on and the program is not installed: the block alone (as ob-ledger
// runs it) with :cmdline read as ledger's words. ok is false when the block
// should run the program as usual.
func nativeLedgerRun(key string, r runner, program string, j *babelJob, extra []string) (babelOutcome, bool) {
	if (key != "ledger" && key != "hledger") || !featureOn("ledger") || len(r.cmd) == 0 {
		return babelOutcome{}, false
	}
	if _, err := exec.LookPath(r.cmd[0]); err == nil {
		return babelOutcome{}, false
	}
	start := time.Now()
	res := babelOutcome{}
	file, line := j.OrgPath, 1
	if at := strings.LastIndex(j.Where, ":"); at >= 0 {
		if n, err := strconv.Atoi(j.Where[at+1:]); err == nil {
			line = n + 1
		}
	}
	if file == "" {
		file = "block.ledger"
	}
	jr := ledger.Parse([]ledger.Chunk{{File: file, Line: line, Text: program}}, ledgerOptions())
	notes := []string{}
	for _, p := range jr.Problems {
		notes = append(notes, fmt.Sprintf("%s:%d: %s: %s", filepath.Base(p.File), p.Line, p.Severity, p.Msg))
	}
	if jr.Errors() > 0 {
		res.code, res.stderr = 1, strings.Join(notes, "\n")+"\n"
		res.took = time.Since(start)
		return res, true
	}
	q, err := ledger.ParseCmdline(extra, ledgerNow())
	if err == nil {
		res.stdout, err = jr.Run(q)
	}
	if err != nil {
		res.code = 1
		notes = append(notes, err.Error())
	}
	if len(notes) > 0 {
		res.stderr = strings.Join(notes, "\n") + "\n"
	}
	res.took = time.Since(start)
	return res, true
}

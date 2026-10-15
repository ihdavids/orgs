package orgs

/* SDOC: Editing
* Babel: Results, Calls And Inline Code

  What org's C-c C-c does, from any client: run what is at a line and write
  what it produced into the file.

  ** Results

  A block's result is written under it as =#+RESULTS:=, replacing the one
  that was there. Its shape follows =:results=:

  | Written              | Comes out as                                    |
  |----------------------+-------------------------------------------------|
  | (text, short)        | =: line= for each line                          |
  | (text, ten lines on) | an =#+begin_example= block                      |
  | =table=              | an org table                                    |
  | =list=               | an org list                                     |
  | =file=               | =[[file:...]]=                                  |
  | =raw=                | the text as it is, to be read as org           |
  | =drawer=             | inside a =:RESULTS:= drawer                     |
  | =html=, =latex=      | an export block                                 |
  | =code=, =org=        | a source block                                  |
  | =silent=, =none=     | nothing written                                 |
  | =append=, =prepend=  | added to what is there instead of replacing it  |
  | =pp=                 | python's value pretty printed (=:results value pp=) |
  | =link=, =graphics=   | with =:file=: the code writes the file itself, and the result is the link |

  =:results= words come in groups - collection (=output=, =value=), type
  (=table=, =list=, =scalar=, =verbatim=, =file=...), format (=raw=, =drawer=,
  =pp=, =link=...) and handling (=replace=, =append=, =silent=...) - and, as in
  org, a word nearer the block replaces an inherited one of its own group:
  =:results replace= on a block undoes a file's =header-args :results append=.

  A block named with =#+NAME:= writes =#+RESULTS: name=, so another block can
  take it as a variable. =:eval no= (or =never=) is never run.

  A block that fails (a non-zero exit) has what it printed and what it wrote to
  standard error written as its result, so the error is in the file where the
  answer would be. =:stderr yes= adds standard error after the output of a
  block that worked (for programs that warn, or answer there).

  ** Around the program and the result

  | Header             | Does                                                      |
  |--------------------+-----------------------------------------------------------|
  | =:prologue "text"= | put before the program (after its variables)              |
  | =:epilogue "text"= | put after it                                              |
  | =:wrap=            | the result in =#+begin_results= ... =#+end_results=        |
  | =:wrap example=    | ... in any block: =:wrap src python=, =:wrap quote=         |
  | =:post fmt(x=*this*)= | run block =fmt= with =*this*= standing for the result, and write what it returns instead |
  | =:cache yes=       | run only when the code or its inputs changed: the result is written as =#+RESULTS[hash]:= and a run with the same hash leaves it be |
  | =:stdin name=      | a table (or result) by name as standard input - for awk, which reads its program from a file |

  ** Noweb

  With =:noweb yes= (or =eval=, =no-export=, =strip-export=, =strip-tangle=), a
  block's =<<name>>= is replaced by the code of block =name= (or of every block
  whose =:noweb-ref= is =name=) before it runs, and =<<name(args)>>= by the
  *result* of running =name= with those arguments, as a =#+CALL:= would. Text
  before a reference on its line is repeated on every line it brings in
  (=:noweb-prefix no= turns that off). Tangling reads references the same way.

  ** Calling a block

  #+BEGIN_EXAMPLE
  #+NAME: double
  #+BEGIN_SRC python :var n=1
  return n * 2
  #+END_SRC

  #+CALL: double(n=21)
  #+END_EXAMPLE

  runs =double= with =n= set to 21 and writes the result under the call. A
  call names its block by =#+NAME:=, looked for in its own file first and then
  in every file the server watches, so a file of useful blocks works as
  org's library of babel. Header arguments may go in square brackets after
  the name or after the arguments: =#+CALL: double[:results raw](n=2) :results table=.

  ** Inline code

  =src_python{return 6*7}= and =call_double(n=4)= in a paragraph run in place,
  and their result follows them as ={{{results(=42=)}}}=. An inline result
  has to fit on a line.

  ** Variables

  =:var x=name= takes a table by its name, the result of a named block (which
  is run first), a =#+RESULTS: name= already in a file, a named list (its
  items) or a named example block (its text). =other.org:name= names one in
  another file, relative to this one.

  An index picks part of a table or list, counting from 0 as org does, rules
  included: =data[2,1]= a cell, =data[,0]= a column, =data[1:3]= rows 1 to 3,
  =data[-1]= the last row, =*= everything at that level.

  A table's first row is its column names when a rule is under it: they are
  taken off before the block sees the table and put back on its result when
  that is a table the same width (=:colnames no= passes the header as data,
  =:colnames yes= always takes the first row, =:colnames '(a b)= names the
  result's columns). =:rownames yes= does the same with the first column. Rules
  are left out unless =:hlines yes=, which passes them as the language's null
  (=None=, =nil=...).

  Every language gets its variables in its own way: an assignment before the
  code for python, ruby, perl, lua, R, javascript, powershell and shell (a
  table as tab separated text); package-level =var=s for go (a body with no
  =package main= and =func main= is wrapped in them, as ob-go does, and
  =:imports "os" "strings"= adds imports); a =let= round the program for
  emacs-lisp; =-v name=value= for awk; and =$name= substituted in the SQL for
  sqlite (a table as the path of a csv file with its header, for =.import=;
  =:db= names the database, =:colnames yes= or =:header= prints a header, and
  the answer comes back as a table). A table handed to gnuplot is written to
  a file and bound as its name, so =plot data using 1:2= works as it does in
  org.

  ** Pictures

  =dot= (graphviz), =plantuml=, =ditaa=, =mermaid= (mmdc) and =gnuplot=
  blocks draw a picture where =:file= says, and the result is a link to it.
  The picture's format follows the file's extension.

  ** Ledgers

  A =beancount= block is a journal, asked a question by =bean-query=: the
  question is =:cmdline= (=BALANCES= when it says none), and the answer comes
  back as an org table. A =ledger= block runs =ledger -f= over itself with
  =:cmdline= as the report (=bal= by default).

  #+BEGIN_EXAMPLE
  #+BEGIN_SRC beancount :cmdline "SELECT account, sum(position) GROUP BY account"
  2026-01-01 open Assets:Bank
  ...
  #+END_SRC
  #+END_EXAMPLE

  =:cmdline= works for every language: its words go after the code's file on
  the command line, split on spaces with quotes holding words together, and
  never through a shell.
EDOC */

import (
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/noweb"
)

// ----------------------------------------------------------------------------
// A run, wherever the code came from
// ----------------------------------------------------------------------------

// What to run: a block, the block a #+CALL names with its arguments in, or an
// inline snippet. Everything below runs a babelJob, so all three read
// variables and :results the same way.
type babelJob struct {
	Lang string
	Code string
	Vars []common.CodeVar
	// The header arguments, later ones winning.
	Args []common.CodeArg
	// Where table names are looked up when a variable does not say.
	File *common.OrgFile
	// The org file the code is in, which relative paths are relative to.
	OrgPath string
	// Where the program runs.
	Dir string
	// How many blocks deep this run is, so blocks that name each other as
	// variables cannot run forever.
	depth int
	// Where the code is, "notes.org:12", for a session's tracebacks.
	Where string
	// Values given outright rather than looked up: :post's *this*.
	bound map[string]babelVal
	// The hash on the #+RESULTS[...] line already under the block, for
	// :cache yes; empty when there is none.
	cachedHash string
}

// The last value given for a header argument.
func (j *babelJob) arg(key string) (string, bool) {
	val, found := "", false
	for _, a := range j.Args {
		if strings.EqualFold(a.Key, key) {
			val, found = strings.Trim(strings.TrimSpace(a.Value), `"`), true
		}
	}
	return val, found
}

// The groups :results words come in. A later word replaces an earlier one of
// its own group - so a block's own :results replace undoes an inherited
// append - and words of different groups add up (org's
// org-babel-merge-params over org-babel-common-header-args-w-values).
var resultGroups = [][]string{
	{"output", "value"},
	{"file", "list", "vector", "table", "scalar", "verbatim"},
	{"raw", "html", "latex", "org", "code", "pp", "drawer", "link", "graphics"},
	{"replace", "silent", "none", "discard", "append", "prepend"},
}

func resultGroup(w string) int {
	for i, g := range resultGroups {
		for _, x := range g {
			if x == w {
				return i
			}
		}
	}
	return -1
}

// The :results words in force, lower-cased: "output table replace". The
// header arguments come inherited first and the block's own last, so the
// nearest wins within each group.
func (j *babelJob) results() []string {
	chosen := map[int]string{}
	extra := []string{}
	for _, a := range j.Args {
		if !strings.EqualFold(a.Key, "results") {
			continue
		}
		for _, w := range strings.Fields(strings.ToLower(a.Value)) {
			if g := resultGroup(w); g >= 0 {
				chosen[g] = w
			} else if !hasWord(extra, w) {
				extra = append(extra, w)
			}
		}
	}
	out := []string{}
	for g := range resultGroups {
		if w, ok := chosen[g]; ok {
			out = append(out, w)
		}
	}
	return append(out, extra...)
}

func hasWord(words []string, w ...string) bool {
	for _, x := range words {
		for _, y := range w {
			if x == y {
				return true
			}
		}
	}
	return false
}

// A block, ready to run.
func jobForBlock(b *common.CodeBlock, f *common.OrgFile) babelJob {
	args := append(inheritedArgs(b.Filename, b.Lang, b.Hash), b.Args...)
	j := babelJob{Lang: b.Lang, Code: b.Code, Vars: b.Vars, Args: args, File: f, OrgPath: b.Filename,
		Where: fmt.Sprintf("%s:%d", filepath.Base(b.Filename), b.Line+1)}
	j.Dir = j.runDir()
	return j
}

var propertyHeaderRe = regexp.MustCompile(`(?i)^\s*#\+PROPERTY:\s+header-args(:[^\s+]+)?\+?\s+(.*)$`)

// The header arguments a block inherits, as org gives them: the file's
// #+PROPERTY: header-args lines, then the header-args properties of the
// headings above it, outermost first - so that, with the block's own after
// them, the nearest one is the one that counts. Both the plain form and the
// language's own (header-args:python) apply, the language's after.
//
// This is how a notebook's blocks share a session without each saying so:
// one line at the top of the file.
func inheritedArgs(path, lang, hash string) []common.CodeArg {
	out := []common.CodeArg{}
	if path == "" {
		return out
	}
	_, key, _ := runnerFor(lang)
	if key == "" {
		key = strings.ToLower(lang)
	}
	sameLang := func(l string) bool {
		_, k, _ := runnerFor(l)
		return strings.EqualFold(l, lang) || (k != "" && k == key)
	}
	add := func(text string) {
		_, _, args, _ := splitArgs(splitParametersLike(" " + text))
		out = append(out, args...)
	}
	if data, err := os.ReadFile(path); err == nil {
		general, specific := []string{}, []string{}
		for _, l := range strings.Split(string(data), "\n") {
			m := propertyHeaderRe.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			if m[1] == "" {
				general = append(general, m[2])
			} else if sameLang(m[1][1:]) {
				specific = append(specific, m[2])
			}
		}
		for _, t := range append(general, specific...) {
			add(t)
		}
	}
	if hash == "" || GetDb() == nil {
		return out
	}
	chain := []*org.Section{}
	for s := GetDb().FindByHash(hash); s != nil; s = s.Parent {
		chain = append([]*org.Section{s}, chain...)
	}
	for _, s := range chain {
		if s.Headline == nil || s.Headline.Properties == nil {
			continue
		}
		for _, kv := range s.Headline.Properties.Properties {
			if len(kv) < 2 {
				continue
			}
			k := strings.ToLower(strings.TrimSuffix(kv[0], "+"))
			if k == "header-args" {
				add(kv[1])
			}
		}
		for _, kv := range s.Headline.Properties.Properties {
			if len(kv) < 2 {
				continue
			}
			k := strings.ToLower(strings.TrimSuffix(kv[0], "+"))
			if strings.HasPrefix(k, "header-args:") && sameLang(k[len("header-args:"):]) {
				add(kv[1])
			}
		}
	}
	return out
}

// :dir, relative to the org file, or the org file's own directory.
func (j *babelJob) runDir() string {
	base := filepath.Dir(j.OrgPath)
	if d, ok := j.arg("dir"); ok && d != "" {
		if strings.HasPrefix(d, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				return filepath.Join(home, d[2:])
			}
		}
		if filepath.IsAbs(d) {
			return d
		}
		return filepath.Join(base, d)
	}
	return base
}

// A path from :file, made absolute against where the program runs.
func (j *babelJob) fileArg() (written string, abs string, ok bool) {
	f, ok := j.arg("file")
	if !ok || f == "" {
		return "", "", false
	}
	if filepath.IsAbs(f) {
		return f, f, true
	}
	// Absolute, because the program runs in Dir and a relative path would be
	// taken from there a second time.
	abs, err := filepath.Abs(filepath.Join(j.Dir, f))
	if err != nil {
		abs = filepath.Join(j.Dir, f)
	}
	return f, abs, true
}

const maxBabelDepth = 8

// runJob runs one job and reads what came back.
//
// In order, as org does it: noweb references expanded, the variables looked
// up and shaped (index, :colnames, :rownames, :hlines) and bound, :prologue
// and :epilogue around the program, :cache consulted, the program run, its
// output shaped by :results, the column and row names put back, :post run
// over it, and :wrap put round it.
func runJob(j babelJob) (common.CodeResult, error) {
	res := common.CodeResult{}
	if j.depth > maxBabelDepth {
		return res, fmt.Errorf("blocks call each other more than %d deep - is one calling itself?", maxBabelDepth)
	}
	if ev, ok := j.arg("eval"); ok && (ev == "no" || ev == "never") {
		return res, fmt.Errorf("this block says :eval %s, so it is not run", ev)
	}
	r, err := babelAllows(j.Lang)
	if err != nil {
		return res, err
	}
	want := j.results()
	collect := r.defaultCollect
	for _, w := range want {
		if w == "output" || w == "value" {
			collect = w
		}
	}
	// Only python has sessions. Any other language with a :session runs on
	// its own, as it always has.
	_, key, _ := runnerFor(j.Lang)
	session := ""
	if key == "python" {
		session = j.sessionName()
		if p, ok := j.arg("python"); ok && strings.TrimSpace(p) != "" && session == "" {
			r.cmd = shellWords(p)
		}
	}

	code := j.Code
	if j.nowebOnEval() {
		expanded, warns, err := noweb.Expand(code, j.nowebResolver())
		if err != nil {
			return res, fmt.Errorf("noweb: %v", err)
		}
		code = expanded
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "BABEL %s: noweb: %s\n", j.Where, w)
		}
	}

	written, out, hasFile := j.fileArg()
	if r.picture != nil {
		if !hasFile {
			return res, fmt.Errorf("a %s block draws a picture: say where it goes with :file, e.g. :file %s.png", j.Lang, j.Lang)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return res, err
		}
		if r.picture.prepare != nil {
			code = r.picture.prepare(code, out)
		}
	}
	if collect == "value" && r.wrapValue != nil && session == "" {
		code = r.wrapValue(code)
		if hasWord(want, "pp") && key == "python" {
			code = strings.Replace(code, "    print(__orgs_result)", "    import pprint; pprint.pprint(__orgs_result)", 1)
		}
	}

	// The variables. A table handed to a language that reads tables from
	// files (gnuplot, sqlite) goes in one for the length of the run.
	tmp := ""
	defer func() {
		if tmp != "" {
			os.RemoveAll(tmp)
		}
	}()
	shape := j.shape()
	binds, argv := []string{}, []string{}
	if r.bind != nil || r.bindArgs != nil {
		for _, v := range j.Vars {
			if v.Name == "" {
				continue
			}
			val, err := j.resolveValue(v)
			if err != nil {
				return res, fmt.Errorf("variable %s: %v", v.Name, err)
			}
			val, header := shape.prepareWithHeader(val)
			if (val.isTbl || val.isList) && r.tableFile != "" {
				if tmp == "" {
					if tmp, err = os.MkdirTemp("", "orgsbabel"); err != nil {
						return res, err
					}
				}
				data := filepath.Join(tmp, v.Name+r.tableFile)
				text := tsv(val)
				if r.tableFile == ".csv" {
					// With its header: .import names a new table's columns
					// from the first row, and would take the first data row.
					withHead := val
					if header != nil && val.isTbl {
						withHead.rows = append([][]string{header}, val.rows...)
					}
					text = csvText(withHead)
				}
				if err := os.WriteFile(data, []byte(text+"\n"), 0600); err != nil {
					return res, err
				}
				val = textVal(v.Name, data)
			}
			if r.bindArgs != nil {
				argv = append(argv, r.bindArgs(v.Name, val)...)
			} else {
				binds = append(binds, r.bind(v.Name, val))
			}
		}
	}
	program := code
	switch {
	case r.assemble != nil:
		program = r.assemble(&j, code, binds)
	case len(binds) > 0:
		program = strings.Join(binds, "\n") + "\n" + code
	}
	if p, ok := j.arg("prologue"); ok && p != "" {
		program = p + "\n" + program
	}
	if e, ok := j.arg("epilogue"); ok && e != "" {
		program = program + "\n" + e
	}

	cmdline := r.defaultCmdline
	if c, ok := j.arg("cmdline"); ok {
		cmdline = c
	}
	extra := append(argv, shellWords(cmdline)...)
	if key == "sqlite" {
		if db, ok := j.arg("db"); ok && db != "" {
			extra = append(extra, db)
		}
	}
	stdin := ""
	if s, ok := j.arg("stdin"); ok && s != "" {
		val, err := j.resolveValue(readVar("stdin="+s, namedKinds(j.File), j.OrgPath))
		if err != nil {
			return res, fmt.Errorf(":stdin %s: %v", s, err)
		}
		stdin = tsv(val) + "\n"
	}

	// :cache yes: the same program with the same inputs gives the same
	// answer, so a result already written for it is the answer.
	if j.cacheOn() {
		res.Hash = babelHash(j.Lang, program, extra, stdin, want, written)
		if j.cachedHash != "" && j.cachedHash == res.Hash {
			res.Ok, res.Cached = true, true
			res.Msg = "cached: the result already there is for this code and these inputs"
			return res, nil
		}
	}

	timeout := time.Duration(Conf().Server.Babel.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if session != "" {
		res, err := runInSession(j, r, session, program, collect, timeout)
		if err == nil {
			hash := babelHash(j.Lang, program, extra, stdin, want, written)
			j.finish(&res, shape)
			if j.cacheOn() {
				res.Hash = hash
			}
		}
		return res, err
	}
	ran, rerr := runProgram(r, program, j.Dir, timeout, out, stdin, extra...)
	res.Seconds = ran.took.Seconds()
	res.Stderr = strings.TrimRight(ran.stderr, "\n")
	res.Code = ran.code
	if rerr != nil {
		res.Msg = rerr.Error()
		return res, nil
	}
	text := ran.stdout
	if ran.code != 0 {
		// It failed: what it printed and what it said went wrong are the
		// result, written in place of the last good one as a notebook shows a
		// traceback - an error nobody can see is worse.
		res.Kind = "error"
		res.Msg = fmt.Sprintf("exit %d", ran.code)
		parts := []string{}
		if t := strings.TrimRight(text, "\n"); t != "" {
			parts = append(parts, t)
		}
		if e := strings.TrimRight(ran.stderr, "\n"); e != "" {
			parts = append(parts, e)
		}
		if len(parts) == 0 {
			parts = append(parts, res.Msg)
		}
		res.Raw = strings.Join(parts, "\n")
		res.Result = res.Raw
		return res, nil
	}
	res.Ok = true
	// :stderr yes puts what went to standard error after what went to
	// standard out, for programs that write their answer there (or warn).
	if s, ok := j.arg("stderr"); ok && (s == "yes" || s == "t" || s == "merge") && strings.TrimSpace(ran.stderr) != "" {
		text = strings.TrimRight(text, "\n")
		if text != "" {
			text += "\n"
		}
		text += strings.TrimRight(ran.stderr, "\n") + "\n"
	}
	res.Raw = strings.TrimRight(text, "\n")

	switch {
	case r.picture != nil:
		res.Kind, res.Result = "file", "[["+"file:"+written+"]]"
	case hasFile && hasWord(want, "link", "graphics"):
		// The code wrote the file itself; the result is the link to it, and
		// what it printed is not the file (org's :results link/graphics).
		res.Kind, res.Result = "file", "[["+"file:"+written+"]]"
	case hasFile && !hasWord(want, "silent", "none"):
		// :file on a block that does not draw: what it printed is the file, as
		// in org. A block that printed nothing wrote the file itself.
		if strings.TrimSpace(text) != "" {
			if hasWord(want, "mkdirp") || j.argYes("mkdirp") {
				os.MkdirAll(filepath.Dir(out), 0755)
			}
			if err := os.WriteFile(out, []byte(text), 0644); err != nil {
				return res, err
			}
		}
		res.Kind, res.Result = "file", "[["+"file:"+written+"]]"
	case key == "sqlite" && !hasWord(want, "raw", "verbatim", "scalar") && sqliteTabular(&j):
		res.Kind, res.Result = csvTable(text, sqliteHeader(&j))
	case r.csvOut && !hasWord(want, "raw", "verbatim", "scalar"):
		res.Kind, res.Result = csvTable(text, true)
	default:
		res.Kind = babelShape(want, text)
		res.Result = babelFormat(res.Kind, text)
	}
	if j.cacheOn() {
		res.Hash = babelHash(j.Lang, program, extra, stdin, want, written)
	}
	j.finish(&res, shape)
	// A result that names a file is only half an answer until somebody can see
	// what is in it.
	describeResultFile(&res, j.OrgPath)
	return res, nil
}

// csvTable reads csv output as a table, the first row a header over a rule
// when header says so.
func csvTable(text string, header bool) (string, string) {
	rows, err := csvRows(text, ",")
	if err != nil || len(rows) == 0 {
		return "text", strings.TrimRight(text, "\n")
	}
	table := rows
	if header && len(rows) > 1 {
		table = append([][]string{rows[0], nil}, rows[1:]...)
	}
	return "table", strings.Join(orgTableLines(table), "\n")
}

func sqliteTabular(j *babelJob) bool {
	for _, m := range []string{"list", "column", "line", "html", "box", "markdown", "table", "json"} {
		if _, ok := j.arg(m); ok {
			return false
		}
	}
	return true
}

// csvText is a table as csv, for sqlite's .import.
func csvText(v babelVal) string {
	buf := &strings.Builder{}
	w := csv.NewWriter(buf)
	if v.isList {
		for _, c := range v.list {
			w.Write([]string{c})
		}
	}
	for _, r := range v.rows {
		if r != nil {
			w.Write(r)
		}
	}
	w.Flush()
	return strings.TrimRight(buf.String(), "\n")
}

// finish is what happens to a result after the program: column and row names
// put back, :post run over it, :wrap put round it.
func (j *babelJob) finish(res *common.CodeResult, shape *tableShape) {
	if !res.Ok {
		return
	}
	shape.reassemble(res)
	if post, ok := j.arg("post"); ok && strings.TrimSpace(post) != "" {
		if err := j.runPost(res, post); err != nil {
			res.Ok, res.Msg = false, ":post "+post+": "+err.Error()
			return
		}
	}
	if w, ok := j.arg("wrap"); ok {
		wrapResult(res, w, j.results(), j.Lang)
	}
}

var postCallRe = regexp.MustCompile(`^\s*([^\s(]+)\s*(?:\((.*)\))?\s*$`)

// runPost runs :post name(args) with *this* standing for the result: the
// block's result becomes whatever the post block makes of it.
func (j *babelJob) runPost(res *common.CodeResult, post string) error {
	m := postCallRe.FindStringSubmatch(post)
	if m == nil {
		return fmt.Errorf("not a block call (name or name(args))")
	}
	sub, _, err := callJob(m[1], "", m[2], "", j.File, j.OrgPath)
	if err != nil {
		return err
	}
	sub.depth = j.depth + 1
	this := resultValue("*this*", res.Kind, res.Result)
	sub.bound = map[string]babelVal{}
	for _, v := range sub.Vars {
		if strings.TrimSpace(v.Value) == "*this*" {
			b := this
			b.v.Name = v.Name
			sub.bound[v.Name] = b
		}
	}
	if len(sub.bound) == 0 && len(sub.Vars) > 0 && strings.TrimSpace(m[2]) == "" {
		// name with no arguments: its first variable is the result, the way
		// org's *this* reads in a block that declares one.
		b := this
		b.v.Name = sub.Vars[0].Name
		sub.bound[sub.Vars[0].Name] = b
	}
	out, err := runJob(sub)
	if err != nil {
		return err
	}
	if !out.Ok {
		return fmt.Errorf("%s", strings.TrimSpace(out.Msg))
	}
	hash := res.Hash
	*res = out
	res.Hash = hash
	return nil
}

// wrapResult puts :wrap's block round a result: :wrap alone is
// #+begin_results, :wrap src python is a python block.
func wrapResult(res *common.CodeResult, w string, want []string, lang string) {
	words := strings.Fields(w)
	name := "results"
	args := ""
	if len(words) > 0 {
		name, args = words[0], strings.Join(words[1:], " ")
	}
	body := strings.TrimRight(res.Result, "\n")
	if res.Kind == "text" || res.Kind == "error" {
		body = strings.TrimRight(res.Raw, "\n")
	}
	lines := []string{"#+begin_" + name}
	if args != "" {
		lines[0] += " " + args
	}
	lines = append(lines, escapeBlockLines(body)...)
	lines = append(lines, "#+end_"+name)
	res.Kind, res.Result = "wrap", strings.Join(lines, "\n")
}

// resultValue is a result as a variable's value: a table, or the text.
func resultValue(name, kind, text string) babelVal {
	if kind == "table" {
		if rows := babelRows(text); len(rows) > 0 {
			return tableVal(name, rows)
		}
	}
	// The fixed width colons org writes in front of a text result are not
	// part of the value.
	lines := []string{}
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		t := strings.TrimSpace(l)
		if t == ":" {
			t = ""
		}
		lines = append(lines, strings.TrimPrefix(t, ": "))
	}
	return textVal(name, strings.Join(lines, "\n"))
}

// ----------------------------------------------------------------------------
// :cache
// ----------------------------------------------------------------------------

func (j *babelJob) argYes(key string) bool {
	v, ok := j.arg(key)
	return ok && (v == "yes" || v == "t")
}

func (j *babelJob) cacheOn() bool { return j.argYes("cache") }

// babelHash is what a cached result is for: the language, the program as it
// will run (variables, noweb and prologue included), its command line and
// input, and how the result is asked for.
func babelHash(lang, program string, extra []string, stdin string, want []string, file string) string {
	h := sha1.New()
	for _, s := range []string{lang, program, strings.Join(extra, "\x00"), stdin, strings.Join(want, " "), file} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

var resultsHashRe = regexp.MustCompile(`(?i)^\s*#\+RESULTS\[([0-9a-f]+)\]:`)

// cachedHashAt is the hash on the #+RESULTS[...] line after row, or "".
func cachedHashAt(lines []string, after int) string {
	i := after + 1
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) {
		if m := resultsHashRe.FindStringSubmatch(lines[i]); m != nil {
			return strings.ToLower(m[1])
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// noweb
// ----------------------------------------------------------------------------

// nowebOnEval says :noweb expands references when the block is run
// (org-babel-noweb-p for :eval).
func (j *babelJob) nowebOnEval() bool {
	v, _ := j.arg("noweb")
	switch strings.ToLower(v) {
	case "yes", "eval", "no-export", "strip-export", "strip-tangle":
		return noweb.Has(j.Code)
	}
	return false
}

// nowebResolver finds what a reference means in the job's file: the blocks
// whose :noweb-ref is the name (inherited header arguments counted), joined
// with the first one's :noweb-sep, else the block of that name; a call is a
// #+CALL: of the named block.
func (j *babelJob) nowebResolver() noweb.Resolver {
	prefix := true
	if p, ok := j.arg("noweb-prefix"); ok && (p == "no" || p == "nil") {
		prefix = false
	}
	return noweb.Resolver{
		Prefix: prefix,
		Body: func(name string) (string, bool) {
			if j.File == nil {
				return "", false
			}
			names := namedKinds(j.File)
			parts := []string{}
			sep := "\n"
			for _, ref := range collectFileBlocks(j.File) {
				b := readBlock(ref, names)
				sub := jobForBlock(&b, j.File)
				if r, ok := sub.arg("noweb-ref"); ok && r == name {
					if len(parts) == 0 {
						if s, ok := sub.arg("noweb-sep"); ok {
							sep = strings.ReplaceAll(s, `\n`, "\n")
						}
					}
					parts = append(parts, strings.TrimSuffix(b.Code, "\n"))
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, sep), true
			}
			if b, _ := findNamedSrc(name, j.File); b != nil {
				return strings.TrimSuffix(b.Code, "\n"), true
			}
			return "", false
		},
		Call: func(name, args string) (string, error) {
			sub, _, err := callJob(name, "", args, "", j.File, j.OrgPath)
			if err != nil {
				return "", err
			}
			sub.depth = j.depth + 1
			out, err := runJob(sub)
			if err != nil {
				return "", err
			}
			if !out.Ok {
				return "", fmt.Errorf("%s: %s", name, out.Msg)
			}
			if out.Kind == "table" {
				return out.Result, nil
			}
			return strings.TrimRight(out.Raw, "\n"), nil
		},
	}
}

// :cmdline split the way a shell would: on spaces, with quotes keeping words
// together and taken off. Nothing else a shell does - the words go to the
// program, never through a shell.
func shellWords(s string) []string {
	out := []string{}
	var cur strings.Builder
	quote := rune(0)
	have := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, have = r, true
		case r == ' ' || r == '\t':
			if have || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if have || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// resolveValue is a variable's value: what its reference names (a table, a
// block's result, a named result, list or example) or the literal it is, with
// its index (data[1:3,2]) applied. Shaping for :colnames and the rest comes
// after, in runJob.
func (j *babelJob) resolveValue(v common.CodeVar) (babelVal, error) {
	if b, ok := j.bound[v.Name]; ok {
		return b, nil
	}
	f := j.File
	if v.RefFile != "" {
		if other := GetDb().FindByFile(v.RefFile); other != nil {
			f = other
		} else if v.Ref != "" {
			return babelVal{}, fmt.Errorf("no file %s", v.RefFile)
		}
	}
	var val babelVal
	switch v.RefKind {
	case "table":
		found := false
		if f != nil {
			for _, ref := range collectFileTables(f) {
				if ref.Name != v.Ref {
					continue
				}
				rows := [][]string{}
				for _, row := range tableRows(ref.Table) {
					if row.Kind == "sep" {
						rows = append(rows, nil)
					} else {
						rows = append(rows, row.Cells)
					}
				}
				val, found = tableVal(v.Name, rows), true
				break
			}
		}
		if !found {
			return val, fmt.Errorf("no table called %q", v.Ref)
		}
	case "src":
		block, bf := findNamedSrc(v.Ref, f)
		if block == nil {
			return val, fmt.Errorf("no block called %q", v.Ref)
		}
		sub := jobForBlock(block, bf)
		sub.depth = j.depth + 1
		r, err := runJob(sub)
		if err != nil {
			return val, fmt.Errorf("running %s: %v", v.Ref, err)
		}
		if !r.Ok {
			return val, fmt.Errorf("running %s: %s", v.Ref, r.Msg)
		}
		val = resultValue(v.Name, r.Kind, r.Result)
	case "results", "list", "example":
		node, ok := namedNode(f, v.Ref)
		if !ok {
			return val, fmt.Errorf("no %s called %q", v.RefKind, v.Ref)
		}
		switch v.RefKind {
		case "list":
			val = listVal(v.Name, listItems(node))
		case "example":
			val = textVal(v.Name, exampleText(node))
		default:
			text := strings.TrimRight(node.String(), "\n")
			kind := "text"
			if strings.HasPrefix(strings.TrimSpace(text), "|") {
				kind = "table"
			}
			val = resultValue(v.Name, kind, text)
		}
	default:
		val = scalarVal(v)
	}
	if v.Index != "" {
		return applyIndex(val, v.Index)
	}
	return val, nil
}

// The block a name means: one in the file first, then the first one in any
// file the server watches, which is how a file of useful blocks becomes a
// library every other file can call.
func findNamedSrc(name string, f *common.OrgFile) (*common.CodeBlock, *common.OrgFile) {
	look := func(f *common.OrgFile) *common.CodeBlock {
		if f == nil || f.Doc == nil {
			return nil
		}
		names := namedKinds(f)
		for _, ref := range collectFileBlocks(f) {
			if ref.Name == name {
				b := readBlock(ref, names)
				return &b
			}
		}
		return nil
	}
	if b := look(f); b != nil {
		return b, f
	}
	files := GetDb().GetFiles()
	sort.Strings(files)
	for _, n := range files {
		other := GetDb().GetFile(n)
		if other == f {
			continue
		}
		if b := look(other); b != nil {
			return b, other
		}
	}
	return nil, nil
}

// ----------------------------------------------------------------------------
// Writing #+RESULTS:
// ----------------------------------------------------------------------------

var resultsLineRe = regexp.MustCompile(`(?i)^\s*#\+RESULTS(\[[^\]]*\])?:`)
var greaterBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_(\S+)`)
var headingLineRe = regexp.MustCompile(`^\*+\s`)

// The last row of the result whose #+RESULTS: line is at row r: the element
// right after it - a block, a drawer, or a run of lines up to a blank one.
func resultExtent(lines []string, r int) int {
	i := r + 1
	if i >= len(lines) {
		return r
	}
	t := strings.TrimSpace(lines[i])
	switch {
	case t == "" || headingLineRe.MatchString(lines[i]) || resultsLineRe.MatchString(lines[i]):
		return r
	case greaterBeginRe.MatchString(t):
		name := greaterBeginRe.FindStringSubmatch(t)[1]
		end := regexp.MustCompile(`(?i)^\s*#\+end_` + regexp.QuoteMeta(name) + `\b`)
		for j := i + 1; j < len(lines); j++ {
			if end.MatchString(lines[j]) {
				return j
			}
		}
		return r
	case strings.EqualFold(t, ":RESULTS:"):
		for j := i + 1; j < len(lines); j++ {
			if strings.EqualFold(strings.TrimSpace(lines[j]), ":END:") {
				return j
			}
		}
		return r
	}
	// Otherwise the result is one element, and ends where that element does -
	// not at the next blank line, or a paragraph written straight under a
	// result is taken for part of it and deleted on the next run.
	fixed := func(t string) bool { return t == ":" || strings.HasPrefix(t, ": ") }
	table := func(t string) bool {
		return strings.HasPrefix(t, "|") || strings.HasPrefix(strings.ToUpper(t), "#+TBLFM:")
	}
	var same func(t, raw string) bool
	switch {
	case fixed(t):
		same = func(t, _ string) bool { return fixed(t) }
	case strings.HasPrefix(t, "|"):
		same = func(t, _ string) bool { return table(t) }
	case listItemRe.MatchString(t):
		// Items, and the lines indented under them.
		base := len(lines[i]) - len(strings.TrimLeft(lines[i], " \t"))
		same = func(t, raw string) bool {
			return listItemRe.MatchString(t) || len(raw)-len(strings.TrimLeft(raw, " \t")) > base
		}
	case strings.HasPrefix(t, "[["):
		return i
	default:
		// Raw text: everything up to a blank line, a heading or a keyword.
		same = func(t, _ string) bool { return !strings.HasPrefix(t, "#+") }
	}
	end := i
	for j := i + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "" || headingLineRe.MatchString(lines[j]) || !same(t, lines[j]) {
			break
		}
		end = j
	}
	return end
}

var listItemRe = regexp.MustCompile(`^([-+]|\d+[.)])\s`)

// placeResult writes a result under the line at row after (a block's END
// line or a #+CALL:), replacing the result already there. Returns the file's
// lines afterwards.
func placeResult(lines []string, after int, name, indent string, body []string, want []string, hash string) []string {
	ind := func(ls []string) []string {
		out := []string{}
		for _, l := range ls {
			if l == "" {
				out = append(out, "")
			} else {
				out = append(out, indent+l)
			}
		}
		return out
	}
	i := after + 1
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) && resultsLineRe.MatchString(lines[i]) {
		end := resultExtent(lines, i)
		var add []string
		switch {
		case hasWord(want, "append"):
			return splice(lines, end+1, ind(body))
		case hasWord(want, "prepend"):
			return splice(lines, i+1, ind(body))
		default:
			// The header line stays as written; only what is under it goes -
			// but for its hash, which is what :cache checks next time.
			head := lines[i]
			if hash != "" || resultsHashRe.MatchString(head) {
				bracket := ""
				if hash != "" {
					bracket = "[" + hash + "]"
				}
				head = regexp.MustCompile(`(?i)#\+RESULTS(\[[^\]]*\])?:`).ReplaceAllString(head, "#+RESULTS"+bracket+":")
			}
			add = append([]string{head}, ind(body)...)
		}
		out := append([]string{}, lines[:i]...)
		out = append(out, add...)
		return append(out, lines[end+1:]...)
	}
	header := indent + "#+RESULTS:"
	if hash != "" {
		header = indent + "#+RESULTS[" + hash + "]:"
	}
	if name != "" {
		header += " " + name
	}
	return splice(lines, after+1, append([]string{"", header}, ind(body)...))
}

// The lines a result is written as, by its shape and what :results asked for.
func resultBody(res *common.CodeResult, want []string, lang string) []string {
	var body []string
	text := strings.TrimRight(res.Result, "\n")
	switch {
	case res.Kind == "table" || res.Kind == "list" || res.Kind == "file":
		if text != "" {
			body = strings.Split(text, "\n")
		}
	case res.Kind == "wrap":
		// :wrap's block, already round it.
		body = strings.Split(text, "\n")
		if hasWord(want, "drawer") {
			body = append(append([]string{":RESULTS:"}, body...), ":END:")
		}
		return body
	case res.Kind == "drawer" || strings.HasPrefix(text, ":results:\n"):
		// Several outputs already gathered in their drawer (a session's text
		// and figures, a notebook cell's outputs): written as they stand.
		return strings.Split(text, "\n")
	case hasWord(want, "raw") || hasWord(want, "drawer"):
		if text != "" {
			body = strings.Split(text, "\n")
		}
	case hasWord(want, "html", "latex"):
		kind := "html"
		if hasWord(want, "latex") {
			kind = "latex"
		}
		body = append([]string{"#+begin_export " + kind}, escapeBlockLines(text)...)
		body = append(body, "#+end_export")
	case hasWord(want, "code", "org"):
		l := lang
		if hasWord(want, "org") {
			l = "org"
		}
		body = append([]string{"#+begin_src " + l}, escapeBlockLines(text)...)
		body = append(body, "#+end_src")
	default:
		if text == "" {
			break
		}
		lines := strings.Split(text, "\n")
		// org's org-babel-min-lines-for-block-output.
		if len(lines) >= 10 {
			body = append([]string{"#+begin_example"}, escapeBlockLines(text)...)
			body = append(body, "#+end_example")
			break
		}
		for _, l := range lines {
			if l == "" {
				body = append(body, ":")
			} else {
				body = append(body, ": "+l)
			}
		}
	}
	if hasWord(want, "drawer") {
		body = append(append([]string{":RESULTS:"}, body...), ":END:")
	}
	return body
}

// Lines that would end or start something inside a block get org's comma.
func escapeBlockLines(text string) []string {
	if text == "" {
		return nil
	}
	out := []string{}
	for _, l := range strings.Split(text, "\n") {
		t := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(t, "*") || strings.HasPrefix(t, "#+") || strings.HasPrefix(t, ",*") || strings.HasPrefix(t, ",#+") {
			l = l[:len(l)-len(t)] + "," + t
		}
		out = append(out, l)
	}
	return out
}

// Whether a #+RESULTS: follows the line at row after, past blank lines.
func hasResultAfter(lines []string, after int) bool {
	i := after + 1
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	return i < len(lines) && resultsLineRe.MatchString(lines[i])
}

// Writes a block's result under it in its file.
func writeBlockResult(block *common.CodeBlock, job babelJob, res *common.CodeResult) error {
	want := job.results()
	if hasWord(want, "silent", "none", "discard") || res.Cached {
		return nil
	}
	lines, err := ganttFileLines(block.Filename)
	if err != nil {
		return err
	}
	end := block.EndLine
	if end < 0 || end >= len(lines) || !regexp.MustCompile(`(?i)^\s*#\+END_SRC`).MatchString(lines[end]) {
		return fmt.Errorf("the block is not where the database thinks it is - reload and try again")
	}
	indent := blockIndent(lines[block.Line])
	body := resultBody(res, want, block.Lang)
	if len(body) == 0 && !hasResultAfter(lines, end) {
		// Nothing to say and nothing said before: a bare #+RESULTS: under a
		// block that only defines things is noise, and a notebook imported
		// without one would change on its first run.
		return nil
	}
	before := strings.Join(lines, "\n")
	lines = placeResult(lines, end, block.Name, indent, body, want, res.Hash)
	// The same result as last time is left alone: a block re-run on a timer
	// (refresh.go) would otherwise rewrite its file every few minutes for
	// nothing, waking every watcher of it.
	if strings.Join(lines, "\n") == before {
		res.Unchanged = true
		return nil
	}
	if err := writeLines(block.Filename, lines); err != nil {
		return err
	}
	res.Written = true
	return nil
}

// ----------------------------------------------------------------------------
// #+CALL:
// ----------------------------------------------------------------------------

var callLineRe = regexp.MustCompile(`(?i)^(\s*)#\+CALL:\s*([^\s(\[]+)(\[[^\]]*\])?(?:\(([^)]*)\))?\s*(.*)$`)

// The job a call stands for: the named block, with the call's arguments in
// place of its own variables and the call's header arguments over its own.
func callJob(name, inside, argList, end string, f *common.OrgFile, path string) (babelJob, *common.CodeBlock, error) {
	block, bf := findNamedSrc(name, f)
	if block == nil {
		return babelJob{}, nil, fmt.Errorf("no block called %q, here or in any other file", name)
	}
	j := jobForBlock(block, bf)
	names := namedKinds(f)
	for _, raw := range splitVarList(argList) {
		v := readVar(raw, names, path)
		if v.Name == "" {
			continue
		}
		replaced := false
		for i := range j.Vars {
			if j.Vars[i].Name == v.Name {
				j.Vars[i], replaced = v, true
			}
		}
		if !replaced {
			j.Vars = append(append([]common.CodeVar{}, j.Vars...), v)
		}
	}
	args := append([]common.CodeArg{}, j.Args...)
	for _, h := range []string{strings.Trim(inside, "[]"), end} {
		if strings.TrimSpace(h) == "" {
			continue
		}
		_, _, more, vars := splitArgs(splitParametersLike(" " + h))
		args = append(args, more...)
		for _, raw := range vars {
			if v := readVar(raw, names, path); v.Name != "" {
				j.Vars = append(j.Vars, v)
			}
		}
	}
	j.Args = args
	// A call runs where it is written, and its relative paths are its file's.
	j.OrgPath = path
	j.Dir = j.runDir()
	return j, block, nil
}

// ----------------------------------------------------------------------------
// Inline src_ and call_
// ----------------------------------------------------------------------------

var inlineSrcRe = regexp.MustCompile(`src_([^\s\[\]{}]+)(\[[^\]\n]*\])?\{`)
var inlineCallRe = regexp.MustCompile(`call_([^\s\[\]()]+)(\[[^\]\n]*\])?\(([^)\n]*)\)(\[[^\]\n]*\])?`)
var inlineResultRe = regexp.MustCompile(`^ ?\{\{\{results\((.*?)\)\}\}\}`)

// One src_ or call_ in a line, and where its result goes.
type inlineCode struct {
	start, end int // the code itself, end exclusive
	resEnd     int // past the {{{results(...)}}} after it, or end when none
	call       bool
	lang, head string
	body       string // the code, or a call's arguments
	tail       string // a call's end header arguments
}

func wordBefore(line string, at int) bool {
	if at == 0 {
		return false
	}
	c := line[at-1]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// Every inline snippet in a line, in order.
func inlineCodeIn(line string) []inlineCode {
	out := []inlineCode{}
	for _, m := range inlineSrcRe.FindAllStringSubmatchIndex(line, -1) {
		if wordBefore(line, m[0]) {
			continue
		}
		// The body runs to the brace that closes the one the match ended on.
		depth, end := 1, -1
		for i := m[1]; i < len(line); i++ {
			if line[i] == '{' {
				depth++
			} else if line[i] == '}' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		if end < 0 {
			continue
		}
		c := inlineCode{start: m[0], end: end + 1, lang: line[m[2]:m[3]], body: line[m[1]:end]}
		if m[4] >= 0 {
			c.head = line[m[4]+1 : m[5]-1]
		}
		out = append(out, c)
	}
	for _, m := range inlineCallRe.FindAllStringSubmatchIndex(line, -1) {
		if wordBefore(line, m[0]) {
			continue
		}
		c := inlineCode{start: m[0], end: m[1], call: true, lang: line[m[2]:m[3]], body: line[m[6]:m[7]]}
		if m[4] >= 0 {
			c.head = line[m[4]+1 : m[5]-1]
		}
		if m[8] >= 0 {
			c.tail = line[m[8]+1 : m[9]-1]
		}
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].start < out[b].start })
	for i := range out {
		out[i].resEnd = out[i].end
		if m := inlineResultRe.FindStringIndex(line[out[i].end:]); m != nil {
			out[i].resEnd = out[i].end + m[1]
		}
	}
	return out
}

// An inline result as org writes one: verbatim unless :results raw, and on
// one line, because there is nowhere else for it to go.
func inlineResult(res *common.CodeResult, want []string) (string, error) {
	if res.Kind == "table" || res.Kind == "list" {
		return "", fmt.Errorf("an inline result has to fit on a line, and this one is a %s", res.Kind)
	}
	t := strings.TrimSpace(res.Result)
	if strings.Contains(t, "\n") {
		return "", fmt.Errorf("an inline result has to fit on a line, and this one has %d", strings.Count(t, "\n")+1)
	}
	// A comma would end the macro's argument.
	t = strings.ReplaceAll(t, ",", `\,`)
	if res.Kind != "file" && !hasWord(want, "raw") && t != "" {
		t = "=" + t + "="
	}
	return " {{{results(" + t + ")}}}", nil
}

// ----------------------------------------------------------------------------
// What is at a line, and running it
// ----------------------------------------------------------------------------

// Something runnable, found by scanning a file's lines.
type babelItem struct {
	kind string // "block", "call", "inline"
	row  int
	end  int
}

var srcBeginRe = regexp.MustCompile(`(?i)^\s*#\+BEGIN_SRC\b`)
var srcEndRe = regexp.MustCompile(`(?i)^\s*#\+END_SRC\b`)

// Every block, call and line of inline code in a file, top to bottom. Inline
// code is not looked for inside blocks, where src_ is just text, nor in fixed
// width lines, where a result is.
func babelItems(lines []string) []babelItem {
	out := []babelItem{}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if srcBeginRe.MatchString(l) {
			end := i
			for j := i + 1; j < len(lines); j++ {
				if srcEndRe.MatchString(lines[j]) {
					end = j
					break
				}
			}
			out = append(out, babelItem{kind: "block", row: i, end: end})
			i = end
			continue
		}
		if m := rawBeginRe.FindStringSubmatch(l); m != nil {
			endRe := regexp.MustCompile(`(?i)^\s*#\+END_` + m[1] + `\b`)
			for i+1 < len(lines) && !endRe.MatchString(lines[i+1]) {
				i++
			}
			i++
			continue
		}
		if callLineRe.MatchString(l) {
			out = append(out, babelItem{kind: "call", row: i, end: i})
			continue
		}
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, ": ") || t == ":" || strings.HasPrefix(t, "#+") {
			continue
		}
		if (strings.Contains(l, "src_") || strings.Contains(l, "call_")) && len(inlineCodeIn(l)) > 0 {
			out = append(out, babelItem{kind: "inline", row: i, end: i})
		}
	}
	return out
}

// Runs one item of a file, writing what it produced unless told not to.
func runItem(path string, it babelItem, write bool) ([]common.BabelRan, error) {
	GetDb().ReloadFile(path)
	f := GetDb().FindByFile(path)
	if f == nil || f.Doc == nil {
		return nil, fmt.Errorf("no file called %q", path)
	}
	lines, err := ganttFileLines(path)
	if err != nil {
		return nil, err
	}
	switch it.kind {
	case "block":
		names := namedKinds(f)
		for _, ref := range collectFileBlocks(f) {
			if ref.Block.Pos.Row != it.row {
				continue
			}
			block := readBlock(ref, names)
			job := jobForBlock(&block, f)
			job.cachedHash = cachedHashAt(lines, block.EndLine)
			ran := common.BabelRan{Kind: "block", Name: block.Name, Line: it.row}
			res, err := runJob(job)
			if err != nil {
				res = common.CodeResult{Msg: err.Error()}
			}
			// A session's failed run is written too: its traceback is what a
			// notebook shows in place of the output.
			if (res.Ok || res.Kind == "error") && write {
				if err := writeBlockResult(&block, job, &res); err != nil {
					res.Ok, res.Msg = false, "ran, but the result could not be written: "+err.Error()
				}
			}
			ran.Result, ran.Written = res, res.Written
			return []common.BabelRan{ran}, nil
		}
		return nil, fmt.Errorf("the block at line %d is not one the database knows - reload and try again", it.row+1)

	case "call":
		m := callLineRe.FindStringSubmatch(lines[it.row])
		ran := common.BabelRan{Kind: "call", Name: m[2], Line: it.row}
		job, _, err := callJob(m[2], m[3], m[4], m[5], f, path)
		var res common.CodeResult
		if err == nil {
			job.cachedHash = cachedHashAt(lines, it.row)
			res, err = runJob(job)
		}
		if err != nil {
			res = common.CodeResult{Msg: err.Error()}
		}
		want := job.results()
		if (res.Ok || res.Kind == "error") && write && !res.Cached && !hasWord(want, "silent", "none", "discard") {
			before := strings.Join(lines, "\n")
			lines = placeResult(lines, it.row, blockName(lines, it.row), m[1], resultBody(&res, want, job.Lang), want, res.Hash)
			if strings.Join(lines, "\n") == before {
				res.Unchanged = true
			} else if err := writeLines(path, lines); err != nil {
				res.Ok, res.Msg = false, "ran, but the result could not be written: "+err.Error()
			} else {
				res.Written = true
			}
		}
		ran.Result, ran.Written = res, res.Written
		return []common.BabelRan{ran}, nil

	case "inline":
		line := lines[it.row]
		codes := inlineCodeIn(line)
		out := []common.BabelRan{}
		replace := make([]string, len(codes))
		for i, c := range codes {
			var job babelJob
			var err error
			ran := common.BabelRan{Kind: "inline", Line: it.row}
			if c.call {
				ran.Name = c.lang
				job, _, err = callJob(c.lang, "["+c.head+"]", c.body, c.tail, f, path)
			} else {
				_, _, args, vars := splitArgs(splitParametersLike(" " + c.head))
				names := namedKinds(f)
				args = append(inheritedArgs(path, c.lang, ""), args...)
				job = babelJob{Lang: strings.ToLower(c.lang), Code: c.body, Args: args, File: f, OrgPath: path,
					Where: fmt.Sprintf("%s:%d", filepath.Base(path), it.row+1)}
				for _, raw := range vars {
					job.Vars = append(job.Vars, readVar(raw, names, path))
				}
				job.Dir = job.runDir()
			}
			var res common.CodeResult
			if err == nil {
				// Inline code is an expression: its value is what it is for.
				if !hasWord(job.results(), "output") {
					job.Args = append(job.Args, common.CodeArg{Key: "results", Value: "value"})
				}
				res, err = runJob(job)
			}
			if err != nil {
				res = common.CodeResult{Msg: err.Error()}
			}
			if res.Ok {
				if s, err := inlineResult(&res, job.results()); err != nil {
					res.Ok, res.Msg = false, err.Error()
				} else if !hasWord(job.results(), "silent", "none", "discard") {
					replace[i] = s
				}
			}
			ran.Result = res
			out = append(out, ran)
		}
		if write {
			changed := false
			for i := len(codes) - 1; i >= 0; i-- {
				if replace[i] == "" {
					continue
				}
				line = line[:codes[i].end] + replace[i] + line[codes[i].resEnd:]
				changed = true
			}
			if changed {
				lines[it.row] = line
				if err := writeLines(path, lines); err != nil {
					return out, err
				}
				for i := range out {
					if replace[i] != "" {
						out[i].Written, out[i].Result.Written = true, true
					}
				}
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("nothing to run")
}

// ExecBabel is the whole of POST /babel/exec.
func ExecBabel(req *common.BabelExec) common.BabelExecResult {
	res := common.BabelExecResult{Ok: true}
	f := GetDb().FindByFile(req.Filename)
	if f == nil || f.Doc == nil {
		return common.BabelExecResult{Msg: fmt.Sprintf("no file called %q", req.Filename)}
	}
	path := f.Doc.Path
	lines, err := ganttFileLines(path)
	if err != nil {
		return common.BabelExecResult{Msg: err.Error()}
	}
	items := babelItems(lines)
	if !req.All {
		var pick *babelItem
		for i := range items {
			if req.Line >= items[i].row && req.Line <= items[i].end {
				pick = &items[i]
			}
		}
		if pick == nil {
			return common.BabelExecResult{Msg: fmt.Sprintf("nothing to run at line %d: no source block, #+CALL: or inline code there", req.Line+1)}
		}
		items = []babelItem{*pick}
	}
	// Each write moves the lines below it, so after one the file is read again
	// and the next item found by its place in the order rather than its row.
	original := map[int]int{}
	for k, it := range items {
		original[k] = it.row
	}
	for k := range items {
		it := items[k]
		if req.All && k > 0 {
			lines, err := ganttFileLines(path)
			if err != nil {
				res.Ok, res.Msg = false, err.Error()
				break
			}
			now := babelItems(lines)
			if k >= len(now) {
				break
			}
			it = now[k]
		}
		ran, err := runItem(path, it, !req.NoWrite)
		for i := range ran {
			ran[i].Line = original[k]
		}
		res.Ran = append(res.Ran, ran...)
		if err != nil {
			res.Ok, res.Msg = false, err.Error()
			continue
		}
	}
	ok := 0
	for _, r := range res.Ran {
		if r.Result.Ok {
			ok++
		} else if res.Ok {
			res.Ok = false
			res.Msg = fmt.Sprintf("line %d: %s", r.Line+1, r.Result.Msg)
		}
	}
	if res.Ok {
		res.Msg = strconv.Itoa(ok) + " ran"
	}
	return res
}

/* SDOC: API
* POST /babel/exec — Run What Is At A Line

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Line": 41 }      the block, call or inline code at row 41
	{ "Filename": "/path/notes.org", "All": true }     every one of them, top to bottom
	{ "Filename": "/path/notes.org", "Line": 41, "NoWrite": true }
	#+END_EXAMPLE

	Runs a source block (any line of it), a =#+CALL:= line, or every
	=src_lang{...}= / =call_name(...)= on the line, and writes the results into
	the file as org would: =#+RESULTS:= under a block or call, ={{{results(...)}}}=
	after inline code. Lines are zero based. Each run comes back in =Ran= with
	its result; =Ok= is true only if all of them worked.

	Off unless =babel.enable= is set, as for =/code/run=.
EDOC */
func PostBabelExec(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.BabelExec
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.BabelExecResult{Msg: err.Error()})
		return
	}
	dynJson(w, ExecBabel(&req))
}

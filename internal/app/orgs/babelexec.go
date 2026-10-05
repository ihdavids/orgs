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

  A block named with =#+NAME:= writes =#+RESULTS: name=, so another block can
  take it as a variable. =:eval no= (or =never=) is never run.

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
  is run first), or a =#+RESULTS: name= already in a file. A table handed to
  gnuplot is written to a file and bound as its name, so =plot data using
  1:2= works as it does in org.

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

	"github.com/ihdavids/orgs/internal/common"
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

// Every word of every :results, lower-cased: "output table replace".
func (j *babelJob) results() []string {
	out := []string{}
	for _, a := range j.Args {
		if strings.EqualFold(a.Key, "results") {
			out = append(out, strings.Fields(strings.ToLower(a.Value))...)
		}
	}
	return out
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
	j := babelJob{Lang: b.Lang, Code: b.Code, Vars: b.Vars, Args: b.Args, File: f, OrgPath: b.Filename}
	j.Dir = j.runDir()
	return j
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

	program := j.Code
	written, out, hasFile := j.fileArg()
	if r.picture != nil {
		if !hasFile {
			return res, fmt.Errorf("a %s block draws a picture: say where it goes with :file, e.g. :file %s.png", j.Lang, j.Lang)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
			return res, err
		}
		if r.picture.prepare != nil {
			program = r.picture.prepare(program, out)
		}
	}
	if collect == "value" && r.wrapValue != nil {
		program = r.wrapValue(program)
	}

	// A table handed to gnuplot goes in a file for the length of the run.
	tmp := ""
	defer func() {
		if tmp != "" {
			os.RemoveAll(tmp)
		}
	}()
	if r.bind != nil && len(j.Vars) > 0 {
		pre := []string{}
		for _, v := range j.Vars {
			if v.Name == "" {
				continue
			}
			v, rows, err := j.resolveVar(v)
			if err != nil {
				return res, fmt.Errorf("variable %s: %v", v.Name, err)
			}
			if rows != nil && r.picture != nil && r.picture.prepare != nil {
				if tmp == "" {
					if tmp, err = os.MkdirTemp("", "orgsbabel"); err != nil {
						return res, err
					}
				}
				data := filepath.Join(tmp, v.Name+".dat")
				lines := []string{}
				for _, row := range rows {
					lines = append(lines, strings.Join(row, "\t"))
				}
				if err := os.WriteFile(data, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
					return res, err
				}
				v = common.CodeVar{Name: v.Name, Value: data, Ref: data}
				rows = nil
			}
			pre = append(pre, r.bind(v.Name, v, rows))
		}
		if len(pre) > 0 {
			program = strings.Join(pre, "\n") + "\n" + program
		}
	}

	timeout := time.Duration(Conf().Server.Babel.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cmdline := r.defaultCmdline
	if c, ok := j.arg("cmdline"); ok {
		cmdline = c
	}
	ran, rerr := runProgram(r, program, j.Dir, timeout, out, shellWords(cmdline)...)
	res.Seconds = ran.took.Seconds()
	res.Stderr = strings.TrimRight(ran.stderr, "\n")
	res.Code = ran.code
	if rerr != nil {
		res.Msg = rerr.Error()
		return res, nil
	}
	text := ran.stdout
	if strings.TrimSpace(text) == "" && ran.code != 0 {
		// Nothing on stdout and a non-zero exit: what there is to show is what
		// went wrong.
		res.Kind = "text"
		res.Msg = fmt.Sprintf("exit %d", ran.code)
		return res, nil
	}
	res.Ok = ran.code == 0
	if !res.Ok {
		res.Msg = fmt.Sprintf("exit %d", ran.code)
	}
	res.Raw = strings.TrimRight(text, "\n")

	switch {
	case r.picture != nil:
		res.Kind, res.Result = "file", "[["+"file:"+written+"]]"
	case hasFile && !hasWord(want, "silent", "none"):
		// :file on a block that does not draw: what it printed is the file, as
		// in org. A block that printed nothing wrote the file itself.
		if strings.TrimSpace(text) != "" {
			if err := os.WriteFile(out, []byte(text), 0644); err != nil {
				return res, err
			}
		}
		res.Kind, res.Result = "file", "[["+"file:"+written+"]]"
	case r.csvOut && !hasWord(want, "raw", "verbatim", "scalar"):
		rows, err := csvRows(text, ",")
		if err != nil || len(rows) == 0 {
			res.Kind, res.Result = "text", strings.TrimRight(text, "\n")
			break
		}
		// The first row is the header.
		table := [][]string{rows[0]}
		if len(rows) > 1 {
			table = append(table, nil)
			table = append(table, rows[1:]...)
		}
		res.Kind, res.Result = "table", strings.Join(orgTableLines(table), "\n")
	default:
		res.Kind = babelShape(want, text)
		res.Result = babelFormat(res.Kind, text)
	}
	// A result that names a file is only half an answer until somebody can see
	// what is in it.
	describeResultFile(&res, j.OrgPath)
	return res, nil
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

// What a variable is when the program gets it: its rows when it names a
// table (or something that produced one), or itself with Value set to the
// text it stands for.
func (j *babelJob) resolveVar(v common.CodeVar) (common.CodeVar, [][]string, error) {
	f := j.File
	if v.RefFile != "" {
		if other := GetDb().FindByFile(v.RefFile); other != nil {
			f = other
		}
	}
	switch v.RefKind {
	case "table":
		if f != nil {
			for _, ref := range collectFileTables(f) {
				if ref.Name != v.Ref {
					continue
				}
				rows := [][]string{}
				for _, row := range tableRows(ref.Table) {
					if row.Kind != "sep" {
						rows = append(rows, row.Cells)
					}
				}
				return v, rows, nil
			}
		}
		return v, nil, fmt.Errorf("no table called %q", v.Ref)
	case "src":
		block, bf := findNamedSrc(v.Ref, f)
		if block == nil {
			return v, nil, fmt.Errorf("no block called %q", v.Ref)
		}
		sub := jobForBlock(block, bf)
		sub.depth = j.depth + 1
		r, err := runJob(sub)
		if err != nil {
			return v, nil, fmt.Errorf("running %s: %v", v.Ref, err)
		}
		if !r.Ok {
			return v, nil, fmt.Errorf("running %s: %s", v.Ref, r.Msg)
		}
		return valueOf(v, r.Kind, r.Result)
	case "results":
		if f != nil && f.Doc != nil {
			if node, ok := f.Doc.NamedNodes[v.Ref]; ok {
				text := strings.TrimRight(node.String(), "\n")
				kind := "text"
				if strings.HasPrefix(strings.TrimSpace(text), "|") {
					kind = "table"
				}
				return valueOf(v, kind, text)
			}
		}
		return v, nil, fmt.Errorf("no result called %q", v.Ref)
	}
	return v, nil, nil
}

// A variable standing for something a block produced.
func valueOf(v common.CodeVar, kind, text string) (common.CodeVar, [][]string, error) {
	if kind == "table" {
		rows := [][]string{}
		for _, row := range babelRows(text) {
			if row != nil {
				rows = append(rows, row)
			}
		}
		return v, rows, nil
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
	// Ref set and RefKind empty is a string to the binders; a number stays one.
	return common.CodeVar{Name: v.Name, Value: strings.Join(lines, "\n"), Ref: v.Ref}, nil, nil
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
func placeResult(lines []string, after int, name, indent string, body []string, want []string) []string {
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
			// The header line stays as written; only what is under it goes.
			add = append([]string{lines[i]}, ind(body)...)
		}
		out := append([]string{}, lines[:i]...)
		out = append(out, add...)
		return append(out, lines[end+1:]...)
	}
	header := indent + "#+RESULTS:"
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

// Writes a block's result under it in its file.
func writeBlockResult(block *common.CodeBlock, job babelJob, res *common.CodeResult) error {
	want := job.results()
	if hasWord(want, "silent", "none", "discard") {
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
	lines = placeResult(lines, end, block.Name, indent, resultBody(res, want, block.Lang), want)
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
			ran := common.BabelRan{Kind: "block", Name: block.Name, Line: it.row}
			res, err := runJob(job)
			if err != nil {
				res = common.CodeResult{Msg: err.Error()}
			}
			if res.Ok && write {
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
			res, err = runJob(job)
		}
		if err != nil {
			res = common.CodeResult{Msg: err.Error()}
		}
		want := job.results()
		if res.Ok && write && !hasWord(want, "silent", "none", "discard") {
			lines = placeResult(lines, it.row, blockName(lines, it.row), m[1], resultBody(&res, want, job.Lang), want)
			if err := writeLines(path, lines); err != nil {
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
				job = babelJob{Lang: strings.ToLower(c.lang), Code: c.body, Args: args, File: f, OrgPath: path}
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

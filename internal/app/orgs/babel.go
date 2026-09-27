package orgs

/* SDOC: Editing
* Running A Source Block

  =POST /code/run= runs one =#+BEGIN_SRC= block and hands back what it
  produced, read the way org reads a babel result rather than as a lump of
  text: a table comes back as an org table, a list as an org list, a =:results
  file= as a link.

  It is **off unless it is turned on**, under =babel:= in the server settings.
  Reading somebody's org files and executing the programs inside them are
  different promises, and the second is made deliberately.

** What gets run

   The block's code, with its variables written in front of it in the
   language's own syntax. A =:var scale=2= becomes an assignment; a =:var
   data=monthly= naming a table becomes that table as a list of lists, so the
   code can walk it. This is the whole point of babel and the reason the
   variables were resolved in the first place.

   Two languages take their code and not their variables: go and emacs-lisp
   are compiled or scoped in ways that do not survive a prepended assignment,
   and a variable silently missing is worse than one that was never offered.

** What comes back

   =:results= is read the way org reads it, and what is not said is guessed at
   from the output rather than assumed:

   | Written            | Means                                             |
   |--------------------+---------------------------------------------------|
   | =:results output=  | What it printed                                   |
   | =:results value=   | What it returned - python is wrapped in a function |
   | =:results table=   | Read the output as a table                        |
   | =:results list=    | Each line an item                                 |
   | =:results file=    | The output is a filename; comes back as a link    |
   | =:results raw=     | As it stands                                      |
   | nothing            | Output, and a table if it looks like one          |

   "Looks like one" is three shapes: lines already written as an org table,
   lines separated by tabs, and the list of lists a python or lisp block
   prints when it returns one. Anything else is text, because a paragraph
   split into columns on a guess is worse than a paragraph.
EDOC */

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// How orgs runs each language it knows. The code goes in on standard input
// unless the runner says otherwise - a compiled language needs a file with the
// right extension on the end of it.
type runner struct {
	cmd []string
	// When set, the code is written to a temp file with this extension and the
	// path is appended to the command instead of being piped in.
	ext string
	// How to write a variable binding. Nil means this language is run without
	// its variables; see the doc block.
	bind func(name string, v common.CodeVar, names map[string]*common.CodeBlock) string
	// Whether `:results value` needs the body wrapped so the return can be
	// printed.
	wrapValue func(code string) string
	// What this language does when nothing says otherwise.
	defaultCollect string
}

var runners = map[string]runner{
	"python": {
		cmd: []string{"python3"}, bind: bindPython,
		wrapValue: wrapPythonValue, defaultCollect: "value",
	},
	"shell": {cmd: []string{"bash"}, bind: bindShell, defaultCollect: "output"},
	"javascript": {
		cmd: []string{"node"}, bind: bindJs, defaultCollect: "output",
	},
	"ruby":   {cmd: []string{"ruby"}, bind: bindRuby, defaultCollect: "output"},
	"perl":   {cmd: []string{"perl"}, bind: bindPerl, defaultCollect: "output"},
	"lua":    {cmd: []string{"lua"}, bind: bindLua, defaultCollect: "output"},
	"r":      {cmd: []string{"Rscript", "-"}, bind: bindR, defaultCollect: "output"},
	"awk":    {cmd: []string{"awk", "-f", "-"}, defaultCollect: "output"},
	"sqlite": {cmd: []string{"sqlite3"}, defaultCollect: "output"},
	// Compiled or scoped in a way a prepended assignment does not survive, so
	// they run without their variables rather than pretending.
	"go":   {cmd: []string{"go", "run"}, ext: ".go", defaultCollect: "output"},
	"lisp": {cmd: []string{"emacs", "-Q", "--batch", "--script"}, ext: ".el", defaultCollect: "output"},
}

// The names people write, and the runner each one means. Shares the highlighter's
// view of what is an alias for what, so allowing "python" allows "py".
var runnerAliases = map[string]string{
	"py": "python", "python3": "python", "ipython": "python",
	"sh": "shell", "bash": "shell", "zsh": "shell", "shell-script": "shell",
	"js": "javascript", "node": "javascript",
	"rb": "ruby", "elisp": "lisp", "emacs-lisp": "lisp", "golang": "go",
	"rscript": "r",
}

func runnerFor(lang string) (runner, string, bool) {
	key := strings.ToLower(strings.TrimSpace(lang))
	if key == "" {
		return runner{}, "", false
	}
	if a, ok := runnerAliases[key]; ok {
		key = a
	}
	r, ok := runners[key]
	return r, key, ok
}

// Whether the settings allow this language to be run, and why not when they do
// not. The refusal is the message somebody reads, so it says what to change.
func babelAllows(lang string) (runner, error) {
	set := Conf().Server.Babel
	if !set.Enable {
		return runner{}, fmt.Errorf(
			"running source blocks is off. Turn it on in orgs.yaml:\n\n" +
				"  babel:\n    enable: true\n    languages: [\"python\", \"sh\"]\n\n" +
				"It is off by default because a source block is a program, and " +
				"this server may be reachable from more than this machine.")
	}
	r, key, ok := runnerFor(lang)
	if !ok {
		if cmd, has := set.Commands[strings.ToLower(lang)]; has && len(cmd) > 0 {
			return runner{cmd: cmd, defaultCollect: "output"}, nil
		}
		return runner{}, fmt.Errorf(
			"orgs does not know how to run %q. Say how in orgs.yaml:\n\n"+
				"  babel:\n    commands:\n      %s: [\"the-interpreter\"]", lang, lang)
	}
	if len(set.Languages) > 0 {
		allowed := false
		for _, l := range set.Languages {
			if lk, _, _ := runnerFor(l); true {
				_ = lk
			}
			if a, ok := runnerAliases[strings.ToLower(strings.TrimSpace(l))]; ok {
				if a == key {
					allowed = true
					break
				}
			}
			if strings.EqualFold(strings.TrimSpace(l), key) {
				allowed = true
				break
			}
		}
		if !allowed {
			return runner{}, fmt.Errorf(
				"%q is not in the babel languages list. Add it in orgs.yaml:\n\n"+
					"  babel:\n    languages: [..., %q]", lang, key)
		}
	}
	// A command override wins over the built in one.
	if cmd, has := set.Commands[key]; has && len(cmd) > 0 {
		r.cmd = cmd
	} else if cmd, has := set.Commands[strings.ToLower(lang)]; has && len(cmd) > 0 {
		r.cmd = cmd
	}
	return r, nil
}

// ----------------------------------------------------------------------------
// Writing the variables in
// ----------------------------------------------------------------------------

// A value as the language would write it. A table becomes a list of lists, a
// list becomes a list, and anything else is passed through as it was written -
// which is what makes `:var n=2` an integer and `:var s="x"` a string without
// anybody having to say which.
func literalOf(v common.CodeVar, rows [][]string, quote func(string) string, open, close, sep string) string {
	if rows == nil {
		val := strings.TrimSpace(v.Value)
		if val == "" {
			return quote("")
		}
		if literalRe.MatchString(val) {
			return val
		}
		// A bare word that resolved to nothing is a string as far as the
		// program is concerned; org does the same.
		if v.Ref != "" && v.RefKind == "" {
			return quote(val)
		}
		return val
	}
	outer := []string{}
	for _, row := range rows {
		cells := []string{}
		for _, c := range row {
			cells = append(cells, cellLiteral(c, quote))
		}
		outer = append(outer, open+strings.Join(cells, sep)+close)
	}
	return open + strings.Join(outer, sep) + close
}

// A table cell, as a number when it reads as one and a string otherwise. That
// is what makes `sum(r[1] for r in data)` work without the block having to
// convert anything.
func cellLiteral(c string, quote func(string) string) string {
	t := strings.TrimSpace(c)
	if t == "" {
		return quote("")
	}
	if _, err := strconv.ParseFloat(t, 64); err == nil {
		return t
	}
	return quote(t)
}

func quoteDouble(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func quoteSingle(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'`
}

func bindPython(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	return name + " = " + literalOf(v, varRows(v), quoteDouble, "[", "]", ", ")
}

func bindJs(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	return "const " + name + " = " + literalOf(v, varRows(v), quoteDouble, "[", "]", ", ") + ";"
}

func bindRuby(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	return name + " = " + literalOf(v, varRows(v), quoteDouble, "[", "]", ", ")
}

func bindLua(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	return "local " + name + " = " + literalOf(v, varRows(v), quoteDouble, "{", "}", ", ")
}

func bindPerl(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	return "my $" + name + " = " + literalOf(v, varRows(v), quoteDouble, "[", "]", ", ") + ";"
}

func bindR(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	rows := varRows(v)
	if rows == nil {
		return name + " <- " + literalOf(v, nil, quoteDouble, "c(", ")", ", ")
	}
	cols := []string{}
	for _, row := range rows {
		cells := []string{}
		for _, c := range row {
			cells = append(cells, cellLiteral(c, quoteDouble))
		}
		cols = append(cols, "c("+strings.Join(cells, ", ")+")")
	}
	return name + " <- list(" + strings.Join(cols, ", ") + ")"
}

// A shell gets scalars as variables and a table as tab separated text, which
// is the shape every tool in a pipeline already reads.
func bindShell(name string, v common.CodeVar, _ map[string]*common.CodeBlock) string {
	rows := varRows(v)
	if rows == nil {
		return name + "=" + quoteSingle(strings.Trim(strings.TrimSpace(v.Value), `"'`))
	}
	lines := []string{}
	for _, row := range rows {
		lines = append(lines, strings.Join(row, "\t"))
	}
	return name + "=" + quoteSingle(strings.Join(lines, "\n"))
}

// The rows of the table a variable points at, or nil when it does not point at
// one. Held on the variable by the run request so that the same walk that
// resolved the reference is the one that reads it.
var varTables = map[string][][]string{}

func varRows(v common.CodeVar) [][]string {
	if v.RefKind != "table" {
		return nil
	}
	return varTables[v.RefFile+"#"+v.Ref]
}

// ----------------------------------------------------------------------------
// Running it
// ----------------------------------------------------------------------------

// `:results value` in python means the block's return, and a python file does
// not return. Org solves this by wrapping the body in a function; so does this.
func wrapPythonValue(code string) string {
	if !regexp.MustCompile(`(?m)^\s*return\b`).MatchString(code) {
		return code
	}
	out := []string{"def __orgs_main():"}
	for _, l := range strings.Split(code, "\n") {
		out = append(out, "    "+l)
	}
	out = append(out, "__orgs_result = __orgs_main()")
	out = append(out, "if __orgs_result is not None:")
	out = append(out, "    print(__orgs_result)")
	return strings.Join(out, "\n")
}

type babelOutcome struct {
	stdout string
	stderr string
	code   int
	took   time.Duration
}

func runProgram(r runner, program string, dir string, timeout time.Duration) (babelOutcome, error) {
	out := babelOutcome{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := append([]string{}, r.cmd[1:]...)
	var tmp string
	if r.ext != "" {
		d, err := os.MkdirTemp("", "orgsbabel")
		if err != nil {
			return out, err
		}
		defer os.RemoveAll(d)
		tmp = filepath.Join(d, "block"+r.ext)
		if err := os.WriteFile(tmp, []byte(program), 0600); err != nil {
			return out, err
		}
		args = append(args, tmp)
	}

	cmd := exec.CommandContext(ctx, r.cmd[0], args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if r.ext == "" {
		cmd.Stdin = strings.NewReader(program)
	}
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se

	start := time.Now()
	err := cmd.Run()
	out.took = time.Since(start)
	out.stdout = so.String()
	out.stderr = se.String()

	if ctx.Err() == context.DeadlineExceeded {
		out.code = -1
		return out, fmt.Errorf("it was still running after %s and was stopped", timeout)
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			out.code = ee.ExitCode()
			return out, nil
		}
		return out, err
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Reading what came back
// ----------------------------------------------------------------------------

var tabbedRe = regexp.MustCompile(`\t`)
var listOfListsRe = regexp.MustCompile(`^[\[(]\s*[\[(]`)

// resultKind decides what shape the output is, from what :results asked for
// and - when it asked for nothing - from the output itself.
//
// Guessing is deliberately timid. Three shapes are recognised because each is
// unambiguous; everything else is text, because a paragraph cut into columns
// on a hunch reads worse than a paragraph.
func babelShape(want []string, text string) string {
	for _, w := range want {
		switch w {
		case "table", "vector":
			return "table"
		case "list":
			return "list"
		case "file":
			return "file"
		case "raw", "verbatim", "scalar", "code", "html", "latex":
			return "text"
		}
	}
	t := strings.TrimSpace(text)
	if t == "" {
		return "text"
	}
	if strings.HasPrefix(t, "|") {
		return "table"
	}
	if listOfListsRe.MatchString(t) {
		return "table"
	}
	lines := strings.Split(t, "\n")
	if len(lines) > 1 && tabbedRe.MatchString(lines[0]) {
		tabs := strings.Count(lines[0], "\t")
		all := true
		for _, l := range lines {
			if strings.Count(l, "\t") != tabs {
				all = false
				break
			}
		}
		if all {
			return "table"
		}
	}
	return "text"
}

// The output as org text of the shape decided. A table is written as an org
// table so the client can hand it to the table viewer it already has, rather
// than either side inventing a second format for rows.
func babelFormat(shape, text string) string {
	t := strings.TrimRight(text, "\n")
	switch shape {
	case "table":
		rows := babelRows(t)
		if len(rows) == 0 {
			return t
		}
		return orgTableText(rows)
	case "list":
		out := []string{}
		for _, l := range strings.Split(strings.TrimSpace(t), "\n") {
			out = append(out, "- "+strings.TrimSpace(l))
		}
		return strings.Join(out, "\n")
	case "file":
		name := strings.TrimSpace(t)
		if name == "" {
			return ""
		}
		// A block that wrote the link itself has already answered the question.
		// Wrapping it again gives [[file:[[file:plot.png]]]], which names no
		// file at all - and a block that prints its own link is the common case,
		// because that is what emacs babel puts in the buffer.
		if strings.HasPrefix(name, "[[") && strings.HasSuffix(name, "]]") {
			return name
		}
		if strings.HasPrefix(name, "file:") {
			return "[[" + name + "]]"
		}
		return "[[file:" + name + "]]"
	}
	return t
}

// The rows behind whatever shape the output came in.
func babelRows(t string) [][]string {
	t = strings.TrimSpace(t)
	if t == "" {
		return nil
	}
	if strings.HasPrefix(t, "|") {
		rows := [][]string{}
		for _, line := range strings.Split(t, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "|-") {
				rows = append(rows, nil) // a rule
				continue
			}
			body := strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
			cells := []string{}
			for _, c := range strings.Split(body, "|") {
				cells = append(cells, strings.TrimSpace(c))
			}
			rows = append(rows, cells)
		}
		return rows
	}
	if listOfListsRe.MatchString(t) {
		return parseListOfLists(t)
	}
	rows := [][]string{}
	for _, line := range strings.Split(t, "\n") {
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

// A list of lists as python, lisp or javascript prints one:
// [['a', 1], ['b', 2]] or (("a" 1) ("b" 2)).
//
// Scanned rather than parsed as json, because none of the three is json:
// python quotes with apostrophes, lisp has no commas, and both print numbers
// bare.
func parseListOfLists(t string) [][]string {
	rows := [][]string{}
	depth := 0
	cur := []string{}
	field := strings.Builder{}
	quote := rune(0)
	flush := func() {
		s := strings.TrimSpace(field.String())
		field.Reset()
		if s != "" {
			cur = append(cur, strings.Trim(s, `"'`))
		}
	}
	for _, r := range t {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			field.WriteRune(r)
			continue
		}
		switch r {
		case '"', '\'':
			quote = r
			field.WriteRune(r)
		case '[', '(':
			depth++
			if depth == 2 {
				cur = []string{}
			}
		case ']', ')':
			if depth == 2 {
				flush()
				rows = append(rows, cur)
				cur = nil
			}
			depth--
		case ',':
			if depth == 2 {
				flush()
			}
		default:
			if depth == 2 {
				field.WriteRune(r)
			}
		}
	}
	return rows
}

// Rows written as an org table, padded so the columns line up - which is what
// makes it worth writing back into a file as well as showing.
func orgTableText(rows [][]string) string {
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	if width == 0 {
		return ""
	}
	colw := make([]int, width)
	for _, r := range rows {
		for i, c := range r {
			if len(c) > colw[i] {
				colw[i] = len(c)
			}
		}
	}
	out := []string{}
	for _, r := range rows {
		if r == nil {
			parts := []string{}
			for i := 0; i < width; i++ {
				parts = append(parts, strings.Repeat("-", colw[i]+2))
			}
			out = append(out, "|"+strings.Join(parts, "+")+"|")
			continue
		}
		parts := []string{}
		for i := 0; i < width; i++ {
			c := ""
			if i < len(r) {
				c = r[i]
			}
			parts = append(parts, " "+c+strings.Repeat(" ", colw[i]-len(c))+" ")
		}
		out = append(out, "|"+strings.Join(parts, "|")+"|")
	}
	return strings.Join(out, "\n")
}

// ----------------------------------------------------------------------------
// The endpoint
// ----------------------------------------------------------------------------

// RunBlock runs one block and reads what came back.
func RunBlock(req *common.CodeRun) (common.CodeResult, error) {
	res := common.CodeResult{}
	block, f, err := findCodeBlock(req.Filename, req.Id)
	if err != nil {
		return res, err
	}
	code := block.Code
	if req.SetCode {
		// Running what is in the editor, before it has been saved. A run is
		// the fastest way to find out whether an edit works, and making
		// somebody save first to find out is the wrong order.
		code = req.Code
	}

	r, err := babelAllows(block.Lang)
	if err != nil {
		return res, err
	}

	// The tables the variables point at, read now so the bindings can be
	// written from them.
	loadVarTables(block, f)

	want := []string{}
	for _, a := range block.Args {
		if a.Key == "results" {
			want = append(want, strings.Fields(strings.ToLower(a.Value))...)
		}
	}
	collect := r.defaultCollect
	for _, w := range want {
		if w == "output" || w == "value" {
			collect = w
		}
	}

	program := code
	if collect == "value" && r.wrapValue != nil {
		program = r.wrapValue(program)
	}
	if r.bind != nil && len(block.Vars) > 0 {
		pre := []string{}
		for _, v := range block.Vars {
			if v.Name == "" {
				continue
			}
			pre = append(pre, r.bind(v.Name, v, nil))
		}
		if len(pre) > 0 {
			program = strings.Join(pre, "\n") + "\n" + program
		}
	}

	timeout := time.Duration(Conf().Server.Babel.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	// Run it beside the file it came out of, so a relative path in the code
	// means what it means when the file is read.
	dir := filepath.Dir(block.Filename)

	out, rerr := runProgram(r, program, dir, timeout)
	res.Seconds = out.took.Seconds()
	res.Stderr = strings.TrimRight(out.stderr, "\n")
	res.Code = out.code
	if rerr != nil {
		res.Msg = rerr.Error()
		return res, nil
	}

	text := out.stdout
	if strings.TrimSpace(text) == "" && out.code != 0 {
		// Nothing on stdout and a non-zero exit: what there is to show is what
		// went wrong.
		res.Ok = false
		res.Kind = "text"
		res.Result = ""
		res.Msg = fmt.Sprintf("exit %d", out.code)
		return res, nil
	}
	res.Ok = out.code == 0
	if !res.Ok {
		res.Msg = fmt.Sprintf("exit %d", out.code)
	}
	res.Kind = babelShape(want, text)
	res.Result = babelFormat(res.Kind, text)
	res.Raw = strings.TrimRight(text, "\n")
	// A result that names a file is only half an answer until somebody can see
	// what is in it.
	describeResultFile(&res, block.Filename)
	return res, nil
}

// The tables this block's variables name, read out of the file they live in.
func loadVarTables(block *common.CodeBlock, f *common.OrgFile) {
	varTables = map[string][][]string{}
	if f == nil || f.Doc == nil {
		return
	}
	for _, v := range block.Vars {
		if v.RefKind != "table" || v.Ref == "" {
			continue
		}
		for _, ref := range collectFileTables(f) {
			if ref.Name != v.Ref {
				continue
			}
			rows := [][]string{}
			for _, row := range tableRows(ref.Table) {
				if row.Kind == "sep" {
					continue
				}
				rows = append(rows, row.Cells)
			}
			varTables[v.RefFile+"#"+v.Ref] = rows
			break
		}
	}
}

// findCodeBlock is the block a request names: a file and the ordinal /code
// reported for it.
func findCodeBlock(filename string, id int) (*common.CodeBlock, *common.OrgFile, error) {
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil {
		return nil, nil, fmt.Errorf("no file called %q", filename)
	}
	names := namedKinds(f)
	refs := collectFileBlocks(f)
	if id < 0 || id >= len(refs) {
		return nil, nil, fmt.Errorf("that file has no block %d", id)
	}
	b := readBlock(refs[id], names)
	return &b, f, nil
}

/* SDOC: API
* POST /code/run — Run A Source Block

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Id": 0 }
	#+END_EXAMPLE

	Runs that block with its variables written in front of it and hands back
	what it produced. =Code= with =SetCode= runs text that is not in the file
	yet, which is how an editor tries an edit before saving it.

	The answer carries =Kind= - =table=, =list=, =file= or =text= - and
	=Result= as org text of that shape, so a table can be handed straight to a
	table view. =Raw= is what the program actually printed, for showing when
	the shape was guessed wrong.

	Refuses unless =babel.enable= is set in the server settings, and the
	refusal says what to write.
EDOC */
func PostCodeRun(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.CodeRun
	if err := json.Unmarshal(body, &req); err != nil {
		codeErr(w, err)
		return
	}
	res, err := RunBlock(&req)
	if err != nil {
		codeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func codeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.CodeResult{Ok: false, Msg: err.Error()})
}

// ----------------------------------------------------------------------------
// A result that is a file
// ----------------------------------------------------------------------------

// The most of a produced file worth sending back. A generated csv or a mermaid
// diagram is a few kilobytes; anything past this is not something to read in a
// results panel, and the link is still there for opening it properly.
const maxResultFileBytes = 512 << 10

var resultLinkRe = regexp.MustCompile(`\[\[file:([^\[\]]+)\]\]`)

// Video, which nothing else here needed a list of. Audio and images already
// have theirs in checklist.go, where the kanban cards needed them first.
var videoExt = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".webm": true, ".ogv": true, ".mkv": true,
}

// What a produced file is, and what to colour it as when it is text.
//
// Decided here rather than left to the client so that one list says what an
// image is. The extension is the whole of it: a file a block just wrote is
// named by the program that wrote it, and that name is the only thing either
// side has to go on.
func mediaKindOf(name string) (kind string, lang string) {
	ext := strings.ToLower(filepath.Ext(strings.SplitN(name, "?", 2)[0]))
	switch {
	case imageExt[ext]:
		// An svg is a picture and also text. Shown as a picture, because that
		// is what somebody who produced one wants to see.
		return "image", ""
	case audioExt[ext] && !videoExt[ext]:
		return "audio", ""
	case videoExt[ext]:
		return "video", ""
	case ext == ".pdf":
		return "pdf", ""
	}
	return "text", textLangFor(ext)
}

// The language to colour a text file as, from its extension. Empty where there
// is nothing sensible to say, which leaves it plain rather than half coloured.
var textLangs = map[string]string{
	".py": "python", ".js": "javascript", ".mjs": "javascript", ".ts": "typescript",
	".tsx": "typescript", ".jsx": "javascript", ".go": "go", ".rs": "rust",
	".c": "c", ".h": "c", ".cc": "c", ".cpp": "c", ".hpp": "c", ".java": "c",
	".cs": "c", ".swift": "c", ".kt": "c", ".scala": "c",
	".sh": "shell", ".bash": "shell", ".zsh": "shell",
	".el": "emacs-lisp", ".lisp": "lisp", ".clj": "clojure", ".scm": "scheme",
	".rb": "ruby", ".pl": "perl", ".lua": "lua", ".r": "r", ".hs": "haskell",
	".sql": "sql", ".yaml": "yaml", ".yml": "yaml", ".json": "json",
	".css": "css", ".scss": "css", ".mermaid": "mermaid", ".mmd": "mermaid",
	".org": "org", ".csv": "csv", ".tsv": "csv",
}

func textLangFor(ext string) string {
	return textLangs[ext]
}

// A file is text when its first few kilobytes hold no NUL. Crude, and the same
// rule diff and grep use, which is the point: it is wrong only about files
// nobody wanted printed anyway.
func looksLikeText(b []byte) bool {
	n := len(b)
	if n > 8192 {
		n = 8192
	}
	return !bytes.Contains(b[:n], []byte{0})
}

// describeResultFile fills in what a file result points at: where it is, and
// what is in it when that is worth showing.
//
// Resolved against the directory the block ran in first, because that is where
// a relative path a program wrote means something, and against the org root
// after, because that is where a link written by hand usually points.
func describeResultFile(res *common.CodeResult, blockFile string) {
	m := resultLinkRe.FindStringSubmatch(res.Result)
	if m == nil {
		return
	}
	name := strings.TrimSpace(m[1])
	if name == "" {
		return
	}
	res.File = name

	candidates := []string{}
	if filepath.IsAbs(name) {
		candidates = append(candidates, name)
	} else {
		candidates = append(candidates, filepath.Join(filepath.Dir(blockFile), name))
		if Conf().Server != nil {
			for _, d := range Conf().Server.OrgDirs {
				candidates = append(candidates, filepath.Join(d, name))
			}
		}
	}

	res.Media, res.TextLang = mediaKindOf(name)

	for _, path := range candidates {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			continue
		}
		res.Exists = true
		res.Bytes = st.Size()
		res.Url = mediaURL(path, blockFile)

		// A picture, a recording or a film is shown rather than printed, so
		// its bytes never need to come back - the url is the whole answer.
		if res.Media != "text" {
			return
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return
		}
		if !looksLikeText(data) {
			// Named like text and is not: say so rather than printing it.
			res.Media = "binary"
			res.TextLang = ""
			return
		}
		if len(data) > maxResultFileBytes {
			data = data[:maxResultFileBytes]
			res.Truncated = true
		}
		res.Text = strings.TrimRight(string(data), "\n")
		return
	}
}

package orgs

/* SDOC: Editing
* Finding Source Blocks

  =GET /code= answers with every =#+BEGIN_SRC= block the server holds, read
  rather than merely located: its language, the name another block would call
  it by, its switches, its babel header arguments, and its variables.

  The variables are the reason this endpoint exists rather than a grep. A
  variable may be a literal, or it may be the *name of something else in the
  file* - a table, a list, the results of another block - and there is no way
  to tell which by looking at the text. The database knows, so each variable
  comes back with what it resolves to and where that thing lives:

  #+BEGIN_EXAMPLE
  #+NAME: monthly
  | month | sales |
  |-------+-------|
  | Jan   |   120 |
  | Feb   |   180 |

  #+NAME: chart
  #+BEGIN_SRC python :var data=monthly :var scale=2 :results file
    ...
  #+END_SRC
  #+END_EXAMPLE

  =data= comes back as a reference to a table of two columns and three rows;
  =scale= comes back as a literal. A name that resolves to nothing comes back
  with an empty kind, which is worth saying out loud: it is a block that cannot
  run.

  | Parameter | Meaning                                                |
  |-----------+--------------------------------------------------------|
  | lang      | Only blocks in this language                           |
  | q         | Only blocks matching this text                         |

  =q= is matched against the code, the name, the language, the heading, the
  file and the variable names - somebody looking for a block remembers any one
  of those.
EDOC */

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ----------------------------------------------------------------------------
// Finding the blocks
// ----------------------------------------------------------------------------

// A named node may be wrapped in a name or metadata node; dig out the block.
// The same shape as unwrapTable in tables.go and for the same reason.
func unwrapBlock(n org.Node) *org.Block {
	switch t := n.(type) {
	case *org.Block:
		return t
	case org.Block:
		return &t
	case org.NodeWithName:
		return unwrapBlock(t.Node)
	case *org.NodeWithName:
		return unwrapBlock(t.Node)
	case org.NodeWithMeta:
		return unwrapBlock(t.Node)
	case *org.NodeWithMeta:
		return unwrapBlock(t.Node)
	}
	return nil
}

// What each named node in a file is, by name: "table", "src", "list",
// "example", "results". This is what turns `:var data=monthly` from a word
// into a reference.
type namedKind struct {
	Kind string
	Line int
	Rows int
	Cols int
}

func namedKinds(f *common.OrgFile) map[string]namedKind {
	out := map[string]namedKind{}
	if f == nil || f.Doc == nil {
		return out
	}
	for name, node := range f.Doc.NamedNodes {
		out[name] = kindOfNamed(node)
	}
	// A block's own name is put back over whatever the document's name map
	// ended up holding for it. `#+RESULTS: chart` under a block named chart
	// registers the *result* under that name and wins by being written later,
	// which would have `:var x=chart` reported as pointing at a paragraph.
	lines := codeFileLines(f)
	blocks := []*org.Block{}
	walkBlocks(f.Doc.Nodes, &blocks)
	for _, b := range blocks {
		if n := blockName(lines, b.Pos.Row); n != "" {
			out[n] = namedKind{Kind: blockKind(b.Name), Line: b.Pos.Row}
		}
	}
	return out
}

func kindOfNamed(node org.Node) namedKind {
	switch t := node.(type) {
	case org.NodeWithName:
		return kindOfNamed(t.Node)
	case *org.NodeWithName:
		return kindOfNamed(t.Node)
	case org.NodeWithMeta:
		return kindOfNamed(t.Node)
	case *org.NodeWithMeta:
		return kindOfNamed(t.Node)
	case *org.Table:
		return namedKind{Kind: "table", Line: t.Pos.Row, Rows: len(t.Rows), Cols: t.GetWidth()}
	case org.Table:
		return namedKind{Kind: "table", Line: t.Pos.Row, Rows: len(t.Rows), Cols: t.GetWidth()}
	case *org.Block:
		return namedKind{Kind: blockKind(t.Name), Line: t.Pos.Row}
	case org.Block:
		return namedKind{Kind: blockKind(t.Name), Line: t.Pos.Row}
	case *org.List:
		return namedKind{Kind: "list", Line: t.Pos.Row}
	case org.List:
		return namedKind{Kind: "list", Line: t.Pos.Row}
	case *org.Result:
		return namedKind{Kind: "results", Line: t.Pos.Row}
	}
	return namedKind{Kind: "text"}
}

func blockKind(name string) string {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "SRC":
		return "src"
	case "EXAMPLE":
		return "example"
	}
	return strings.ToLower(strings.TrimSpace(name))
}

// One block plus where it was found.
type blockRef struct {
	Block *org.Block
	Sec   *org.Section
	File  *common.OrgFile
	Name  string
	Id    int
}

// Every source block under a node, however deep.
//
// Walked rather than read off `Headline.Blocks`, which only ever holds a block
// that happens to be a heading's *first* child - the loop that fills it breaks
// after the first node it looks at. A heading with a paragraph and then a
// block has an empty Blocks list, which is most headings with a block in them.
func walkBlocks(nodes []org.Node, out *[]*org.Block) {
	for _, n := range nodes {
		if b := unwrapBlock(n); b != nil {
			if strings.EqualFold(b.Name, "SRC") {
				*out = append(*out, b)
			}
			walkBlocks(b.Children, out)
			continue
		}
		switch t := n.(type) {
		case org.Headline:
			walkBlocks(t.Children, out)
		case *org.Headline:
			walkBlocks(t.Children, out)
		case org.List:
			walkBlocks(t.Items, out)
		case *org.List:
			walkBlocks(t.Items, out)
		case org.ListItem:
			walkBlocks(t.Children, out)
		case *org.ListItem:
			walkBlocks(t.Children, out)
		case org.Drawer:
			walkBlocks(t.Children, out)
		case *org.Drawer:
			walkBlocks(t.Children, out)
		}
	}
}

var nameLineRe = regexp.MustCompile(`^\s*#\+(?i:NAME):\s*(.*)$`)
var affiliatedRe = regexp.MustCompile(`^\s*#\+\w`)

// The #+NAME: written above a block, which is what another block calls it by.
//
// Read off the file's own lines, which is the only place it reliably is.
// go-org takes the name away from the block twice over: the parser consumes
// `#+NAME:` as a *named node* keyword, so it never reaches the block's own
// keyword list, and the name map it goes into is overwritten by a
// `#+RESULTS: chart` under the block - the result is registered under the same
// name and is written later, so it wins.
//
// Scanned upwards over the affiliated keywords, because `#+HEADER:` lines are
// allowed to sit between the name and the block.
func blockName(lines []string, row int) string {
	for i := row - 1; i >= 0 && i < len(lines); i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		if m := nameLineRe.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1])
		}
		if !affiliatedRe.MatchString(line) {
			break
		}
	}
	return ""
}

// The lines of a file, for reading the things the parser did not keep.
func codeFileLines(f *common.OrgFile) []string {
	if f == nil || f.Doc == nil || f.Doc.Path == "" {
		return nil
	}
	lines, err := ganttFileLines(f.Doc.Path)
	if err != nil {
		return nil
	}
	return lines
}

// Every source block in a file, in document order, each attributed to the
// heading it sits under.
//
// The heading is found by position - the last one starting above the block -
// rather than by walking the outline. That is the same rule the link index
// follows and for the same reason: go-org ends a headline's body at a drawer
// written in column zero and hoists the rest of that heading to the top of the
// document, where the outline no longer says where anything is.
func collectFileBlocks(f *common.OrgFile) []*blockRef {
	out := []*blockRef{}
	if f == nil || f.Doc == nil {
		return out
	}
	lines := codeFileLines(f)
	blocks := []*org.Block{}
	walkBlocks(f.Doc.Nodes, &blocks)
	sort.SliceStable(blocks, func(a, b int) bool { return blocks[a].Pos.Row < blocks[b].Pos.Row })

	secs := flattenSections(f) // already in row order
	for _, b := range blocks {
		var owner *org.Section
		for _, sec := range secs {
			if sec.Headline == nil || sec.Headline.Pos.Row > b.Pos.Row {
				break
			}
			owner = sec
		}
		out = append(out, &blockRef{Block: b, Sec: owner, File: f, Name: blockName(lines, b.Pos.Row), Id: len(out)})
	}
	return out
}

// ----------------------------------------------------------------------------
// Reading what a block says about itself
// ----------------------------------------------------------------------------

// The indexing org allows after a name: monthly[1,2], monthly[,0], data[2:4].
var varIndexRe = regexp.MustCompile(`\[[^\]]*\]$`)

// A value that is plainly a literal rather than a name: a number, a quoted
// string, an elisp form. Checked so that `:var n=2` is not reported as a
// reference to a table called "2" that happens not to exist.
var literalRe = regexp.MustCompile(`^(-?\d+(\.\d+)?|".*"|'.*'|\(.*\)|yes|no|t|nil)$`)

// splitArgs walks the parameter list go-org built and sorts it into the
// language, the switches, the variables and everything else.
//
// go-org hands over [lang, ":key", "value", ":key", "value", …], splitting on
// " :" - so the switches written before the first header argument are still
// stuck to the language. Taking the first word off it is what separates
// "python" from "python -n -r".
func splitArgs(params []string) (lang string, switches []string, args []common.CodeArg, rawVars []string) {
	switches = []string{}
	args = []common.CodeArg{}
	rawVars = []string{}
	if len(params) == 0 {
		return "", switches, args, rawVars
	}
	words := strings.Fields(params[0])
	if len(words) > 0 {
		lang = strings.ToLower(words[0])
		switches = append(switches, words[1:]...)
	}
	for i := 1; i+1 < len(params); i += 2 {
		key := strings.TrimPrefix(strings.TrimSpace(params[i]), ":")
		val := strings.TrimSpace(params[i+1])
		if key == "" {
			continue
		}
		if strings.EqualFold(key, "var") {
			// One :var may bind several variables, comma separated.
			for _, one := range splitVarList(val) {
				if one != "" {
					rawVars = append(rawVars, one)
				}
			}
			continue
		}
		args = append(args, common.CodeArg{Key: strings.ToLower(key), Value: val})
	}
	return lang, switches, args, rawVars
}

// `:var a=1, b=2` binds two variables; `:var m=tbl[1,2]` binds one. Split on
// commas that are not inside brackets, quotes or parentheses.
func splitVarList(v string) []string {
	out := []string{}
	depth := 0
	quote := rune(0)
	cur := strings.Builder{}
	for _, r := range v {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '[' || r == '(':
			depth++
		case r == ']' || r == ')':
			if depth > 0 {
				depth--
			}
		case r == ',' && depth == 0:
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// readVar turns "data=monthly[1,2]" into a variable, resolved against the
// names its own file knows.
func readVar(raw string, names map[string]namedKind, filename string) common.CodeVar {
	v := common.CodeVar{}
	eq := strings.Index(raw, "=")
	if eq < 0 {
		// `:var data` with nothing after it: a binding somebody has not
		// finished writing. Reported as it stands rather than guessed at.
		v.Name = strings.TrimSpace(raw)
		return v
	}
	v.Name = strings.TrimSpace(raw[:eq])
	v.Value = strings.TrimSpace(raw[eq+1:])

	target := v.Value
	if m := varIndexRe.FindString(target); m != "" {
		v.Index = m
		target = strings.TrimSuffix(target, m)
	}
	target = strings.TrimSpace(target)
	if target == "" || literalRe.MatchString(target) {
		return v
	}
	// Anything left that looks like a name is treated as one. Where the name
	// is not known, RefKind stays empty and Ref is still reported - a broken
	// reference is the thing most worth seeing.
	if !strings.ContainsAny(target, " \t\"'()") {
		v.Ref = target
		if k, ok := names[target]; ok {
			v.RefKind = k.Kind
			v.RefFile = filename
			v.RefLine = k.Line
			v.Rows = k.Rows
			v.Cols = k.Cols
		}
	}
	return v
}

// headerArgs are the `#+HEADER:` lines written above a block. Org treats them
// as part of the same argument list, so they are read the same way and put in
// front - the line on the block itself wins where the two disagree, which is
// what org does.
func headerArgs(b *org.Block) []string {
	out := []string{}
	for _, k := range b.Keywords {
		if strings.EqualFold(k.Key, "HEADER") || strings.EqualFold(k.Key, "HEADERS") {
			// Given to the same splitter the block line goes through, with an
			// empty language in front so the shape matches.
			out = append(out, " "+strings.TrimSpace(k.Value))
		}
	}
	return out
}

func blockCode(b *org.Block) string {
	w := org.OrgWriter{}
	return dedent(strings.TrimRight(w.WriteNodesAsString(b.Children...), "\n"))
}

// The code with the indent the *block* sits at taken off, leaving the indent
// the code has of its own.
//
// A block written under a heading is indented, and that indentation belongs to
// the org file rather than to the program: handing it to python as it stands
// is an IndentationError on the first line. Org strips it before running a
// block and this does the same, which also makes it the right text to put in
// an editor - what goes back in is re-indented by UpdateBlock.
//
// The *smallest* indent across the non-blank lines is what comes off, so a
// function body inside the block keeps its shape.
func dedent(code string) string {
	lines := strings.Split(code, "\n")
	least := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if least < 0 || n < least {
			least = n
		}
	}
	if least <= 0 {
		return code
	}
	for i, l := range lines {
		if len(l) >= least {
			lines[i] = l[least:]
		} else {
			lines[i] = strings.TrimLeft(l, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

// What a block last produced, and what shape it came back in.
func blockResult(b *org.Block) (string, string) {
	if b.Result == nil {
		return "", ""
	}
	// go-org's parser returns a Result *value*, not a pointer - both forms are
	// taken because one of them being wrong here was silent: no result, no
	// error, and nothing on screen to say a block had ever been run.
	var node org.Node
	switch r := b.Result.(type) {
	case *org.Result:
		node = r.Node
	case org.Result:
		node = r.Node
	}
	if node == nil {
		return "", ""
	}
	w := org.OrgWriter{}
	text := strings.TrimRight(w.WriteNodesAsString(node), "\n")
	kind := "text"
	switch node.(type) {
	case *org.Table, org.Table:
		kind = "table"
	case *org.List, org.List:
		kind = "list"
	}
	return text, kind
}

func readBlock(ref *blockRef, names map[string]namedKind) common.CodeBlock {
	b := ref.Block
	filename := ""
	if ref.File != nil && ref.File.Doc != nil {
		filename = ref.File.Doc.Path
	}

	lang, switches, args, rawVars := splitArgs(b.Parameters)
	// Header lines first, so that an argument repeated on the block itself is
	// the one that ends up last - which is the one a reader takes as final.
	for _, extra := range headerArgs(b) {
		_, _, hargs, hvars := splitArgs(splitParametersLike(extra))
		args = append(hargs, args...)
		rawVars = append(hvars, rawVars...)
	}

	vars := []common.CodeVar{}
	for _, raw := range rawVars {
		vars = append(vars, readVar(raw, names, filename))
	}

	code := blockCode(b)
	result, resultKind := blockResult(b)

	out := common.CodeBlock{
		Filename: filename,
		Id:       ref.Id,
		Name:     ref.Name,
		Lang:     lang,
		Line:     b.Pos.Row,
		EndLine:  b.EndPos.Row,
		Code:     code,
		Lines:    len(strings.Split(code, "\n")),
		Switches: switches,
		Args:     args,
		Vars:     vars,
		Result:   result,
		ResultKind: resultKind,
	}
	if code == "" {
		out.Lines = 0
	}
	if ref.Sec != nil {
		out.Hash = ref.Sec.Hash
		out.Heading = common.BuildOutlinePath(ref.Sec, "/")
		if out.Heading != "" {
			out.Olp = strings.Split(out.Heading, "/")
		}
	}
	return out
}

// splitParametersLike does to a #+HEADER: line what go-org's own splitter does
// to the text after #+BEGIN_SRC. Written here rather than exported from go-org
// because it is three lines and the alternative is a fork.
func splitParametersLike(s string) []string {
	params := []string{}
	parts := strings.Split(s, " :")
	params = append(params, strings.TrimSpace(parts[0]))
	for _, p := range parts[1:] {
		kv := strings.SplitN(p+" ", " ", 2)
		params = append(params, ":"+kv[0], strings.TrimSpace(kv[1]))
	}
	return params
}

// ----------------------------------------------------------------------------
// The index
// ----------------------------------------------------------------------------

// One file's source blocks, cached per file.
//
// Was one cache of the whole database gated on the reload counter, which meant
// a single saved file threw away every file's blocks and the next request
// walked all five hundred of them - about fifty milliseconds, on every save, for
// a tab that searches as you type.
var codeParts = NewFileParts[[]common.CodeBlock]()

// Every source block in the database.
func allCodeBlocks() []common.CodeBlock {
	per := codeParts.All(func(f *common.OrgFile) []common.CodeBlock {
		// Sections are registered lazily as queries walk them, and a block's
		// hash is its section's - so register on the way past, the way the
		// link index and the records do.
		for _, sec := range flattenSections(f) {
			GetDb().RegisterSection(sec.Hash, sec, f)
		}
		names := namedKinds(f)
		blocks := []common.CodeBlock{}
		for _, ref := range collectFileBlocks(f) {
			blocks = append(blocks, readBlock(ref, names))
		}
		return blocks
	})
	out := []common.CodeBlock{}
	for _, part := range per {
		out = append(out, part...)
	}
	// The parts arrive in the database's file order, so this only orders within
	// a file - but it is left as it was rather than dropped, because "in file
	// then line order" is what the tab's grouping relies on and the database's
	// own order is not guaranteed to be sorted.
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Filename != out[b].Filename {
			return out[a].Filename < out[b].Filename
		}
		return out[a].Line < out[b].Line
	})
	return out
}

func codeMatches(b *common.CodeBlock, q string) bool {
	if q == "" {
		return true
	}
	n := strings.ToLower(q)
	if strings.Contains(strings.ToLower(b.Name), n) ||
		strings.Contains(strings.ToLower(b.Lang), n) ||
		strings.Contains(strings.ToLower(b.Heading), n) ||
		strings.Contains(strings.ToLower(b.Filename), n) ||
		strings.Contains(strings.ToLower(b.Code), n) {
		return true
	}
	for _, v := range b.Vars {
		if strings.Contains(strings.ToLower(v.Name), n) || strings.Contains(strings.ToLower(v.Ref), n) {
			return true
		}
	}
	return false
}

/* SDOC: API
* GET /code — Every Source Block

	Answers with every =#+BEGIN_SRC= block, read: its language, its name, its
	switches, its header arguments and its variables. A variable that names
	something else in the file - a table, another block, a list - comes back
	resolved, with what it points at and where that lives.

	| Parameter | Meaning                                |
	|-----------+----------------------------------------|
	| lang      | Only blocks in this language           |
	| q         | Only blocks matching this text         |

	The languages in use come back beside the blocks, with a count each, so a
	client can offer the filter without asking twice.
EDOC */
func RequestCode(w http.ResponseWriter, r *http.Request) {
	lang := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	all := allCodeBlocks()
	counts := map[string]int{}
	out := []common.CodeBlock{}
	for i := range all {
		b := all[i]
		// The language list counts every block, not the ones that got through
		// the filter - otherwise picking a language would empty the list of
		// every other language and there would be no way back.
		name := b.Lang
		if name == "" {
			name = "none"
		}
		counts[name]++
		if lang != "" && !strings.EqualFold(b.Lang, lang) {
			continue
		}
		if !codeMatches(&b, q) {
			continue
		}
		out = append(out, b)
	}

	langs := []common.CodeLang{}
	for k, v := range counts {
		langs = append(langs, common.CodeLang{Lang: k, Count: v})
	}
	sort.SliceStable(langs, func(a, b int) bool {
		if langs[a].Count != langs[b].Count {
			return langs[a].Count > langs[b].Count
		}
		return langs[a].Lang < langs[b].Lang
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.CodeIndex{Blocks: out, Langs: langs, Total: len(all)})
}

// ----------------------------------------------------------------------------
// Changing a block
// ----------------------------------------------------------------------------

// blockIndent is the indent the #+BEGIN_SRC line sits at, which is what the
// code inside it and the delimiters around it are written to.
func blockIndent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// headerLine builds the #+BEGIN_SRC line from its parts, in org's order:
// language, then switches, then the header arguments with the variables first.
//
// The variables come first because that is the order org writes them and the
// order anybody reads them in - what a block takes before how it behaves.
func headerLine(indent, lang string, switches []string, vars []common.CodeVar, args []common.CodeArg) string {
	parts := []string{indent + "#+BEGIN_SRC"}
	if l := strings.TrimSpace(lang); l != "" {
		parts = append(parts, l)
	}
	for _, sw := range switches {
		if sw = strings.TrimSpace(sw); sw != "" {
			parts = append(parts, sw)
		}
	}
	for _, v := range vars {
		if strings.TrimSpace(v.Name) == "" {
			continue
		}
		parts = append(parts, ":var "+strings.TrimSpace(v.Name)+"="+strings.TrimSpace(v.Value))
	}
	for _, a := range args {
		k := strings.TrimSpace(a.Key)
		if k == "" || strings.EqualFold(k, "var") {
			continue
		}
		if v := strings.TrimSpace(a.Value); v != "" {
			parts = append(parts, ":"+k+" "+v)
		} else {
			parts = append(parts, ":"+k)
		}
	}
	return strings.Join(parts, " ")
}

// UpdateBlock writes a block's name, header line and body back into its file.
//
// A line splice rather than a document rewrite, for the reason every other
// write in this codebase is one: a block usually shares its file with a
// heading structure, tables and prose, and writing the parsed document back
// would reformat all of it to change three lines of python.
func UpdateBlock(req *common.CodeUpdate) (common.ResultMsg, error) {
	res := common.ResultMsg{Ok: false}
	f := GetDb().FindByFile(req.Filename)
	if f == nil || f.Doc == nil {
		return res, fmt.Errorf("no file called %q", req.Filename)
	}
	refs := collectFileBlocks(f)
	if req.Id < 0 || req.Id >= len(refs) {
		return res, fmt.Errorf("that file has no block %d", req.Id)
	}
	ref := refs[req.Id]
	filename := f.Doc.Path

	lines, err := ganttFileLines(filename)
	if err != nil {
		return res, err
	}
	begin := ref.Block.Pos.Row
	end := ref.Block.EndPos.Row
	if begin < 0 || begin >= len(lines) || end < begin || end >= len(lines) {
		return res, fmt.Errorf("that block is not where the database thinks it is - reload and try again")
	}
	indent := blockIndent(lines[begin])

	// Built from the bottom up, so every row used is still the row it was
	// worked out from. The body first, then the header, then the name - each
	// of which sits above the last.
	if req.SetCode {
		body := []string{}
		for _, l := range strings.Split(strings.TrimRight(req.Code, "\n"), "\n") {
			if strings.TrimSpace(l) == "" {
				body = append(body, "")
			} else {
				body = append(body, indent+"  "+strings.TrimRight(l, " \t"))
			}
		}
		if len(body) == 1 && body[0] == "" {
			body = []string{}
		}
		out := append([]string{}, lines[:begin+1]...)
		out = append(out, body...)
		out = append(out, lines[end:]...)
		lines = out
		end = begin + 1 + len(body)
	}

	if req.SetHeader {
		lines[begin] = headerLine(indent, req.Lang, req.Switches, req.Vars, req.Args)
	}

	if req.SetName {
		// Where the name is, or would go: above the block, past the
		// #+HEADER: lines that are allowed to sit between the two.
		at := -1
		insertAt := begin
		for i := begin - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "" {
				break
			}
			if nameLineRe.MatchString(lines[i]) {
				at = i
				break
			}
			if !affiliatedRe.MatchString(lines[i]) {
				break
			}
			insertAt = i
		}
		name := strings.TrimSpace(req.Name)
		switch {
		case at >= 0 && name == "":
			lines = append(lines[:at], lines[at+1:]...)
		case at >= 0:
			lines[at] = indent + "#+NAME: " + name
		case name != "":
			lines = splice(lines, insertAt, []string{indent + "#+NAME: " + name})
		}
	}

	if err := writeLines(filename, lines); err != nil {
		return res, err
	}
	res.Ok = true
	res.Msg = "written"
	return res, nil
}

/* SDOC: API
* POST /code/update — Change A Source Block

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Id": 0,
	  "SetCode": true,   "Code": "print(1)\n",
	  "SetName": true,   "Name": "chart",
	  "SetHeader": true, "Lang": "python",
	  "Vars": [{"Name": "data", "Value": "monthly"}],
	  "Args": [{"Key": "results", "Value": "table"}] }
	#+END_EXAMPLE

	Each part is written only when its flag is set, because "" is a thing
	somebody means: clearing the name and leaving it alone are different. An
	empty =Name= with =SetName= takes the =#+NAME:= line off.

	Only the lines the block occupies are touched. The header line is rebuilt
	from its parts in org's own order - language, switches, variables, then the
	rest of the arguments - so a block edited here reads the way a block
	written by hand does.
EDOC */
func PostCodeUpdate(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.CodeUpdate
	if err := json.Unmarshal(body, &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	res, err := UpdateBlock(&req)
	if err != nil {
		res = common.ResultMsg{Ok: false, Msg: err.Error()}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

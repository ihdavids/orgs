package orgs

/* SDOC: Editing
* Dynamic Blocks

  A dynamic block is a part of a file that orgs writes for you and rewrites on
  request: a clock report, a column view, a list of headings a query finds.
  Everything between the =#+BEGIN:= and =#+END:= lines is replaced; nothing
  else in the file is touched.

  #+BEGIN_EXAMPLE
  #+BEGIN: clocktable :scope file :maxlevel 2 :block thisweek
  #+END:
  #+END_EXAMPLE

  Refresh one with =orgs dblock FILE LINE=, every block in a file with
  =orgs dblock FILE=, and every block anywhere with =orgs dblock -all=
  (a cron line that keeps the week's clock tables current).

  ** clocktable

  The time clocked under headings, the way org writes it: one time column
  per outline level, a bold total, and a file column when more than one file
  is reported.

  | Parameter   | Meaning                                                                         |
  |-------------+---------------------------------------------------------------------------------|
  | =:scope=    | =file= (default), =subtree=, =tree= / =treeN=, =agenda= (every file), or a list |
  |             | of files: =("work.org" "home.org")=                                             |
  | =:maxlevel= | deepest level listed (default 3)                                                |
  | =:block=    | =today=, =yesterday=, =thisweek=, =lastweek=, =thismonth=, =lastmonth=,         |
  |             | =thisyear=, =lastyear=, =today-3=, =thisweek-2=, =untilnow=, =2026-10-04=,      |
  |             | =2026-10=, =2026-W40=, =2026-Q3=, =2026=                                        |
  | =:tstart=   | start of the range, as a timestamp or a typed date (=<2026-10-01>=, =-2w=)       |
  | =:tend=     | end of the range, exclusive                                                     |
  | =:match=    | an org tags match (=+work-meeting=); only matching headings count their time    |
  | =:query=    | an orgs query (=IsStatus("DONE")=); only matching headings count their time     |
  | =:indent=   | =nil= to write titles without the =\_= indentation                              |
  | =:fileskip0= | with several files, leave out the ones with no time                            |

  Weeks start on Monday, as they do in org. A clock that crosses the edge of the
  range counts only the part inside it, and a clock still running counts up to
  now.

  ** columnview

  org's column view written as a table.

  | Parameter            | Meaning                                                                  |
  |----------------------+--------------------------------------------------------------------------|
  | =:id=                | =local= (the heading the block is under, default), =global= (the whole  |
  |                      | file), =file:notes.org=, or a heading's =ID= / =CUSTOM_ID=               |
  | =:format=            | a =#+COLUMNS:= line; otherwise the file's own, then the server default   |
  | =:maxlevel=          | deepest level listed                                                     |
  | =:hlines=            | =t= for a rule between every row, =N= for one above each level <= N      |
  | =:indent=            | =t= to indent the ITEM column by level                                   |
  | =:skip-empty-rows=   | =t= to leave out rows with nothing but a title                           |
  | =:exclude-tags=      | =(tag1 tag2)= - headings carrying any of these are left out              |

  Summed columns (=%EFFORT{:}=) show the rollup. A =#+TBLFM:= line under the
  table is kept and run again after every refresh.

  ** query

  The headings an orgs query finds, as a table.

  #+BEGIN_EXAMPLE
  #+BEGIN: query :q "IsStatus(\"NEXT\")" :columns "TODO ITEM FILE DEADLINE" :link t
  #+END:
  #+END_EXAMPLE

  =:q= is required. =:columns= defaults to =TODO ITEM FILE= and takes the names
  a =#+COLUMNS:= line takes, with or without their =%=. =:scope file= keeps to
  this file, =:limit N= stops after N rows, =:link t= makes each title a link
  to its heading.

  ** insertdatetime

  The moment the block was refreshed, as an inactive timestamp. =:active t=
  writes an active one, =:format "%Y-%m-%d %H:%M"= writes it your way.

  Any other name is looked up among the dynamic blocks plugins register.
EDOC */

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/mattn/go-runewidth"
)

// ----------------------------------------------------------------------------
// Finding the blocks
// ----------------------------------------------------------------------------

var dynBeginRe = regexp.MustCompile(`(?i)^(\s*)#\+BEGIN:\s*(\S+)(.*)$`)
var dynEndRe = regexp.MustCompile(`(?i)^\s*#\+END:\s*$`)

// A greater block whose contents are text rather than org. A `#+BEGIN:` written
// inside one is an example of a dynamic block, not a dynamic block, and every
// guide that shows how to write one has such a line in it.
var rawBeginRe = regexp.MustCompile(`(?i)^\s*#\+BEGIN_(SRC|EXAMPLE|EXPORT|COMMENT)\b`)

type dynBlock struct {
	Name   string
	Header string
	Params map[string]string
	Begin  int
	End    int
	Indent string
}

// Every dynamic block in a file, in order. A BEGIN with no END before the next
// BEGIN (or the end of the file) is not a block: writing to it would eat
// everything up to wherever an END turned up.
func findDynBlocks(lines []string) []dynBlock {
	out := []dynBlock{}
	for i := 0; i < len(lines); i++ {
		if m := rawBeginRe.FindStringSubmatch(lines[i]); m != nil {
			endRe := regexp.MustCompile(`(?i)^\s*#\+END_` + m[1] + `\b`)
			for i+1 < len(lines) && !endRe.MatchString(lines[i+1]) {
				i++
			}
			i++
			continue
		}
		m := dynBeginRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if dynEndRe.MatchString(lines[j]) {
				end = j
				break
			}
			if dynBeginRe.MatchString(lines[j]) {
				break
			}
		}
		if end < 0 {
			continue
		}
		out = append(out, dynBlock{
			Name:   strings.ToLower(m[2]),
			Header: strings.TrimSpace(m[3]),
			Params: parseDynParams(m[3]),
			Begin:  i,
			End:    end,
			Indent: m[1],
		})
		i = end
	}
	return out
}

// The `:key value` pairs of a block's header, as org reads them: a value is
// one word, a "quoted string" (with \" inside it), or a (parenthesised list),
// kept with its parentheses so a list stays a list. A key followed by another
// key, or by nothing, is a flag and reads as "t".
func parseDynParams(s string) map[string]string {
	out := map[string]string{}
	rs := []rune(s)
	i := 0
	skip := func() {
		for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
			i++
		}
	}
	word := func() string {
		start := i
		for i < len(rs) && rs[i] != ' ' && rs[i] != '\t' {
			i++
		}
		return string(rs[start:i])
	}
	for {
		skip()
		if i >= len(rs) {
			break
		}
		if rs[i] != ':' {
			word()
			continue
		}
		key := strings.ToLower(strings.TrimPrefix(word(), ":"))
		skip()
		if i >= len(rs) || (rs[i] == ':' && i+1 < len(rs) && rs[i+1] != ' ') {
			out[key] = "t"
			continue
		}
		switch rs[i] {
		case '"':
			i++
			var b strings.Builder
			for i < len(rs) && rs[i] != '"' {
				if rs[i] == '\\' && i+1 < len(rs) {
					i++
				}
				b.WriteRune(rs[i])
				i++
			}
			i++
			out[key] = b.String()
		case '(':
			start, depth := i, 0
			for i < len(rs) {
				if rs[i] == '(' {
					depth++
				} else if rs[i] == ')' {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				i++
			}
			out[key] = string(rs[start:i])
		default:
			out[key] = word()
		}
	}
	return out
}

// The words of a `(a b "c d")` list, or the one word of a value that is not a
// list, so a parameter can be written either way.
func dynList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" || v == "nil" {
		return nil
	}
	if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") {
		v = v[1 : len(v)-1]
	}
	out := []string{}
	for _, f := range regexp.MustCompile(`"[^"]*"|\S+`).FindAllString(v, -1) {
		out = append(out, strings.Trim(f, `"`))
	}
	return out
}

func dynTrue(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v != "" && v != "nil" && v != "no" && v != "false"
}

// ----------------------------------------------------------------------------
// Writing what goes in them
// ----------------------------------------------------------------------------

// What a block's generator gets to look at.
type dynCtx struct {
	File  *common.OrgFile
	Lines []string
	Block dynBlock
	// The heading the block sits under, nil above the first heading.
	Owner *org.Section
	Now   time.Time
}

func (c *dynCtx) param(key, def string) string {
	if v, ok := c.Block.Params[key]; ok {
		return v
	}
	return def
}

// The lines that go between BEGIN and END, without the block's indent.
type dynGenerator func(c *dynCtx) ([]string, error)

var dynGenerators = map[string]dynGenerator{
	"clocktable":     dynClocktable,
	"columnview":     dynColumnview,
	"query":          dynQuery,
	"insertdatetime": dynInsertDateTime,
}

// The heading a row belongs to: the last one starting at or above it. Found by
// position rather than by walking the outline, as the link and code indexes do.
func sectionAtRow(f *common.OrgFile, row int) *org.Section {
	var owner *org.Section
	for _, s := range flattenSections(f) {
		if headlineRow(s) < 0 || headlineRow(s) > row {
			break
		}
		owner = s
	}
	return owner
}

// An org table as aligned text. A nil row is a rule. A column that is mostly
// numbers (durations included) is right-aligned, as org aligns it.
func orgTableLines(rows [][]string) []string {
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	nums := make([]int, cols)
	filled := make([]int, cols)
	for _, r := range rows {
		for i, c := range r {
			if w := runewidth.StringWidth(c); w > widths[i] {
				widths[i] = w
			}
			if strings.TrimSpace(c) != "" {
				filled[i]++
				if tableNumberRe.MatchString(strings.TrimSpace(c)) {
					nums[i]++
				}
			}
		}
	}
	out := []string{}
	for _, r := range rows {
		if r == nil {
			parts := make([]string, cols)
			for i := range parts {
				parts[i] = strings.Repeat("-", widths[i]+2)
			}
			out = append(out, "|"+strings.Join(parts, "+")+"|")
			continue
		}
		var b strings.Builder
		b.WriteString("|")
		for i := 0; i < cols; i++ {
			c := ""
			if i < len(r) {
				c = r[i]
			}
			pad := strings.Repeat(" ", widths[i]-runewidth.StringWidth(c))
			if filled[i] > 0 && nums[i]*2 > filled[i] {
				b.WriteString(" " + pad + c + " |")
			} else {
				b.WriteString(" " + c + pad + " |")
			}
		}
		out = append(out, b.String())
	}
	return out
}

// What org counts as a number when it decides how to align a column.
var tableNumberRe = regexp.MustCompile(`^[<>]?[-+^.0-9]*[0-9][-+^.0-9eEdDx()%:]*$`)

// A value made safe to sit in a table cell: a `|` would end the cell.
func cellText(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "|", `\vert{}`)
}

// The indentation org puts in front of a title to show its depth in a table
// (`org-clocktable-indent-string`): `\_` and two spaces a level.
func levelIndent(level int) string {
	if level <= 1 {
		return ""
	}
	return `\_` + strings.Repeat("  ", level-1)
}

// ----------------------------------------------------------------------------
// Refreshing
// ----------------------------------------------------------------------------

// refreshDynBlocks rewrites the blocks pick chooses in one file, as one write.
//
// Every block's text is worked out against the file as it stands before any
// of them is written, and they are spliced in from the bottom up, so each
// block's rows are still the rows it was found at (see Traps: re-measure).
func refreshDynBlocks(filename string, pick func(b dynBlock) bool) (common.DynBlockFile, error) {
	res := common.DynBlockFile{File: filename}
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil {
		return res, fmt.Errorf("no file called %q", filename)
	}
	path := f.Doc.Path
	res.File = path
	// The database's reading of the file must be the one on disk, or the rows
	// a block is attributed by belong to a different version of it.
	GetDb().ReloadFile(path)
	if f = GetDb().FindByFile(path); f == nil || f.Doc == nil {
		return res, fmt.Errorf("could not read %q", filename)
	}
	lines, err := ganttFileLines(path)
	if err != nil {
		return res, err
	}
	blocks := findDynBlocks(lines)
	bodies := make([][]string, len(blocks))
	picked := make([]bool, len(blocks))
	now := time.Now()
	for i, b := range blocks {
		if !pick(b) {
			continue
		}
		picked[i] = true
		owner := sectionAtRow(f, b.Begin)
		out := blockInfo(b, owner)
		body, err := generateDynBlock(&dynCtx{File: f, Lines: lines, Block: b, Owner: owner, Now: now})
		if err != nil {
			out.Msg = err.Error()
		} else {
			out.Ok = true
			bodies[i] = body
		}
		res.Blocks = append(res.Blocks, out)
	}
	if len(res.Blocks) == 0 {
		return res, nil
	}

	changed := false
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if !picked[i] || bodies[i] == nil {
			continue
		}
		body := make([]string, 0, len(bodies[i]))
		for _, l := range bodies[i] {
			if strings.TrimSpace(l) == "" {
				body = append(body, "")
			} else {
				body = append(body, b.Indent+l)
			}
		}
		if strings.Join(lines[b.Begin+1:b.End], "\n") == strings.Join(body, "\n") {
			continue
		}
		out := append([]string{}, lines[:b.Begin+1]...)
		out = append(out, body...)
		out = append(out, lines[b.End:]...)
		lines = out
		changed = true
	}
	if changed {
		if err := writeLines(path, lines); err != nil {
			return res, err
		}
		res.Written = true
	}
	return res, nil
}

func blockInfo(b dynBlock, owner *org.Section) common.DynBlock {
	out := common.DynBlock{Name: b.Name, Header: b.Header, Params: b.Params, Line: b.Begin, EndLine: b.End}
	if owner != nil && owner.Headline != nil {
		out.Heading = common.GetSectionTitle(owner)
	}
	return out
}

func generateDynBlock(c *dynCtx) ([]string, error) {
	if gen, ok := dynGenerators[c.Block.Name]; ok {
		return gen(c)
	}
	// A block a plugin registered, which is handed the block the way go-org
	// would have built it: the name, then the parameters as key, value pairs.
	if exec, ok := Conf().PlugManager.BlockExec[c.Block.Name]; ok {
		params := []string{c.Block.Name}
		keys := make([]string, 0, len(c.Block.Params))
		for k := range c.Block.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			params = append(params, ":"+k, c.Block.Params[k])
		}
		parent := c.Owner
		if parent == nil {
			parent = c.File.Doc.Outline.Section
		}
		r := exec(c.File, parent, &org.Block{Name: "DYN", Parameters: params})
		if r == nil || !r.Ok {
			msg := "the block's plugin failed"
			if r != nil && r.Msg != "" {
				msg = r.Msg
			}
			return nil, fmt.Errorf("%s", msg)
		}
		return strings.Split(strings.TrimRight(r.Msg, "\n"), "\n"), nil
	}
	known := []string{}
	for k := range dynGenerators {
		known = append(known, k)
	}
	for k := range Conf().PlugManager.BlockExec {
		known = append(known, k)
	}
	sort.Strings(known)
	return nil, fmt.Errorf("no dynamic block called %q (there are: %s)", c.Block.Name, strings.Join(known, ", "))
}

// RefreshDynBlockAt rewrites the block whose BEGIN..END span holds row.
func RefreshDynBlockAt(filename string, row int) (common.DynBlockFile, error) {
	res, err := refreshDynBlocks(filename, func(b dynBlock) bool { return row >= b.Begin && row <= b.End })
	if err == nil && len(res.Blocks) == 0 {
		err = fmt.Errorf("line %d of %s is not in a dynamic block", row+1, filepath.Base(filename))
	}
	return res, err
}

// Every file the server watches that has a dynamic block in it, so refreshing
// all of them does not read and rewrite files that have none.
func filesWithDynBlocks() []string {
	out := []string{}
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		lines, err := ganttFileLines(f.Doc.Path)
		if err != nil {
			continue
		}
		if len(findDynBlocks(lines)) > 0 {
			out = append(out, f.Doc.Path)
		}
	}
	sort.Strings(out)
	return out
}

/* SDOC: API
* GET /dblocks — The Dynamic Blocks In A File

	=GET /dblocks?file=/path/notes.org= lists every =#+BEGIN:= block in the
	file: its name, its header and parameters, the rows of its BEGIN and END
	lines (zero based) and the heading it sits under. Without =file=, every
	file that has one is listed.
EDOC */
func RequestDynBlocks(w http.ResponseWriter, r *http.Request) {
	res := common.DynBlocksResult{Ok: true}
	files := []string{}
	if name := r.URL.Query().Get("file"); name != "" {
		f := GetDb().FindByFile(name)
		if f == nil || f.Doc == nil {
			res = common.DynBlocksResult{Msg: fmt.Sprintf("no file called %q", name)}
		} else {
			files = append(files, f.Doc.Path)
		}
	} else {
		files = filesWithDynBlocks()
	}
	for _, path := range files {
		f := GetDb().FindByFile(path)
		lines, err := ganttFileLines(path)
		if f == nil || err != nil {
			continue
		}
		out := common.DynBlockFile{File: path}
		for _, b := range findDynBlocks(lines) {
			out.Blocks = append(out.Blocks, blockInfo(b, sectionAtRow(f, b.Begin)))
		}
		res.Files = append(res.Files, out)
	}
	dynJson(w, res)
}

/* SDOC: API
* POST /dblocks/update — Refresh Dynamic Blocks

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Line": 12 }       the block holding row 12
	{ "Filename": "/path/notes.org", "All": true }      every block in the file
	{ "All": true, "Name": "clocktable" }               every clock table anywhere
	#+END_EXAMPLE

	Only the lines between =#+BEGIN:= and =#+END:= change. A block that comes
	out the same as it was leaves its file unwritten. =Ok= is true only when
	every block asked for was refreshed; each block carries its own =Ok= and
	=Msg=.
EDOC */
func PostDynBlocksUpdate(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.DynBlockRequest
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.DynBlocksResult{Msg: err.Error()})
		return
	}
	dynJson(w, UpdateDynBlocks(&req))
}

// UpdateDynBlocks is the whole of POST /dblocks/update.
func UpdateDynBlocks(req *common.DynBlockRequest) common.DynBlocksResult {
	res := common.DynBlocksResult{Ok: true}
	named := func(b dynBlock) bool { return req.Name == "" || strings.EqualFold(req.Name, b.Name) }
	files := []string{}
	switch {
	case req.Filename != "":
		files = append(files, req.Filename)
	case req.All:
		files = filesWithDynBlocks()
	default:
		return common.DynBlocksResult{Msg: "name a file, or ask for all of them"}
	}
	for _, name := range files {
		var out common.DynBlockFile
		var err error
		if req.Filename != "" && !req.All {
			out, err = RefreshDynBlockAt(name, req.Line)
		} else {
			out, err = refreshDynBlocks(name, named)
		}
		if err != nil {
			res.Ok = false
			res.Msg = err.Error()
			continue
		}
		for _, b := range out.Blocks {
			if !b.Ok {
				res.Ok = false
				if res.Msg == "" {
					res.Msg = fmt.Sprintf("%s line %d: %s", filepath.Base(out.File), b.Line+1, b.Msg)
				}
			}
		}
		if len(out.Blocks) > 0 {
			res.Files = append(res.Files, out)
		}
	}
	if res.Ok {
		n, w := 0, 0
		for _, f := range res.Files {
			n += len(f.Blocks)
			if f.Written {
				w++
			}
		}
		res.Msg = fmt.Sprintf("%d block(s) refreshed, %d file(s) written", n, w)
	}
	return res
}

func dynJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ----------------------------------------------------------------------------
// insertdatetime
// ----------------------------------------------------------------------------

func dynInsertDateTime(c *dynCtx) ([]string, error) {
	if f := c.param("format", ""); f != "" && f != "t" {
		return []string{strftime(f, c.Now)}, nil
	}
	s := c.Now.Format("2006-01-02 Mon 15:04")
	if dynTrue(c.param("active", "")) {
		return []string{"<" + s + ">"}, nil
	}
	return []string{"[" + s + "]"}, nil
}

// The strftime directives people write in an org file's format strings.
func strftime(format string, t time.Time) string {
	repl := map[byte]string{
		'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'e': "_2", 'H': "15", 'I': "03",
		'M': "04", 'S': "05", 'p': "PM", 'a': "Mon", 'A': "Monday", 'b': "Jan",
		'B': "January", 'Z': "MST", 'z': "-0700",
	}
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case '%':
			b.WriteByte('%')
		case 'j':
			b.WriteString(fmt.Sprintf("%03d", t.YearDay()))
		case 'V':
			_, w := t.ISOWeek()
			b.WriteString(fmt.Sprintf("%02d", w))
		default:
			if layout, ok := repl[format[i]]; ok {
				b.WriteString(t.Format(layout))
			} else {
				b.WriteByte('%')
				b.WriteByte(format[i])
			}
		}
	}
	return b.String()
}

// ----------------------------------------------------------------------------
// columnview
// ----------------------------------------------------------------------------

var tblfmRe = regexp.MustCompile(`(?i)^\s*#\+TBLFM:`)

func dynColumnview(c *dynCtx) ([]string, error) {
	id := c.param("id", "local")
	filename := c.File.Doc.Path
	var root *org.Section
	switch {
	case id == "local" || id == "t":
		root = c.Owner
	case id == "global" || id == "nil":
	case strings.HasPrefix(id, "file:"):
		name := strings.TrimPrefix(id, "file:")
		if !filepath.IsAbs(name) {
			name = filepath.Join(filepath.Dir(filename), name)
		}
		f := GetDb().FindByFile(name)
		if f == nil || f.Doc == nil {
			return nil, fmt.Errorf("no file called %q", strings.TrimPrefix(id, "file:"))
		}
		filename = f.Doc.Path
	default:
		sec := GetDb().FindByAnyId(id)
		if sec == nil {
			return nil, fmt.Errorf("no heading with the id %q", id)
		}
		f := fileOfSection(sec)
		if f == nil {
			return nil, fmt.Errorf("the heading with the id %q is in no file the server watches", id)
		}
		filename, root = f.Doc.Path, sec
	}

	view, err := BuildColumnView(filename, c.param("format", ""))
	if err != nil {
		return nil, err
	}
	rows := view.Rows
	baseLevel := 0
	if root != nil && root.Headline != nil {
		from := -1
		for i, r := range rows {
			if r.LineNum == root.Headline.Pos.Row {
				from = i
				break
			}
		}
		if from < 0 {
			return nil, fmt.Errorf("the heading %q is not in its file's column view", common.GetSectionTitle(root))
		}
		to := from + 1
		for to < len(rows) && rows[to].Level > rows[from].Level {
			to++
		}
		rows = rows[from:to]
		baseLevel = root.Headline.Lvl - 1
	}

	maxLevel, _ := strconv.Atoi(c.param("maxlevel", "0"))
	exclude := map[string]bool{}
	for _, t := range dynList(c.param("exclude-tags", "")) {
		exclude[t] = true
	}
	skipEmpty := dynTrue(c.param("skip-empty-rows", ""))
	indent := dynTrue(c.param("indent", ""))
	hlines := c.param("hlines", "")
	hlineLevel, _ := strconv.Atoi(hlines)

	header := []string{}
	for _, s := range view.Spec {
		header = append(header, s.Title)
	}
	table := [][]string{header, nil}
	first := true
	for _, r := range rows {
		if maxLevel > 0 && r.Level > maxLevel {
			continue
		}
		skip := false
		for _, t := range r.Tags {
			if exclude[t] {
				skip = true
			}
		}
		if skip {
			continue
		}
		cells := []string{}
		empty := true
		for i, cell := range r.Cells {
			v := cellText(cell.Value)
			if view.Spec[i].Property == "ITEM" {
				if indent {
					v = levelIndent(r.Level-baseLevel) + v
				}
			} else if v != "" {
				empty = false
			}
			cells = append(cells, v)
		}
		if skipEmpty && empty {
			continue
		}
		if !first && (hlines == "t" || (hlineLevel > 0 && r.Level-baseLevel <= hlineLevel)) {
			table = append(table, nil)
		}
		first = false
		table = append(table, cells)
	}
	out := orgTableLines(table)

	// Formulas written under the table are the user's, and are run again over
	// the new rows, as org does.
	formulas := []string{}
	for _, l := range c.Lines[c.Block.Begin+1 : c.Block.End] {
		if tblfmRe.MatchString(l) {
			formulas = append(formulas, strings.TrimSpace(l))
		}
	}
	if len(formulas) == 0 {
		return out, nil
	}
	return applyTableFormulas(out, formulas)
}

// A table's lines with its formulas run over them, formula lines included, so a
// column view can carry sums and ratios the way a table written by hand does.
func applyTableFormulas(table []string, formulas []string) ([]string, error) {
	text := "* t\n" + strings.Join(table, "\n") + "\n" + strings.Join(formulas, "\n") + "\n"
	doc := org.New().Parse(strings.NewReader(text), "columnview.org")
	if doc.Error != nil || len(doc.Outline.Children) == 0 || len(doc.Outline.Children[0].Headline.Tables) == 0 {
		return append(table, formulas...), nil
	}
	sec := doc.Outline.Children[0]
	tbl := sec.Headline.Tables[0]
	if err := ExecuteFormula(nil, sec, &common.OrgFile{Filename: "columnview.org", Doc: doc}, tbl); err != nil {
		return nil, fmt.Errorf("the table's formulas failed: %v", err)
	}
	return strings.Split(strings.TrimRight(renderTable(tbl, ""), "\n"), "\n"), nil
}

// The file holding a heading, looked for through every file the server has.
func fileOfSection(sec *org.Section) *common.OrgFile {
	for _, name := range GetDb().GetFiles() {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		for _, s := range flattenSections(f) {
			if s == sec {
				return f
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// query
// ----------------------------------------------------------------------------

func dynQuery(c *dynCtx) ([]string, error) {
	q := strings.TrimSpace(c.param("q", c.param("query", "")))
	if q == "" || q == "t" {
		return nil, fmt.Errorf(`a query block needs :q "the query"`)
	}
	expanded := expandQueryFilters(q)
	exp, err := ParseString(&common.StringQuery{Query: expanded})
	if err != nil {
		return nil, fmt.Errorf("the query does not parse: %v", err)
	}

	colsLine := c.param("columns", "TODO ITEM FILE")
	words := strings.Fields(colsLine)
	for i, w := range words {
		if !strings.HasPrefix(w, "%") {
			words[i] = "%" + w
		}
	}
	spec := ParseColumnSpec(strings.Join(words, " "))
	if len(spec) == 0 {
		return nil, fmt.Errorf("%q names no columns", colsLine)
	}
	limit, _ := strconv.Atoi(c.param("limit", "0"))
	link := dynTrue(c.param("link", ""))

	files := []string{}
	if c.param("scope", "") == "file" {
		files = append(files, c.File.Doc.Path)
	} else {
		registerAllSections()
		files, _ = filesForQuery(expanded)
		sort.Strings(files)
	}

	header := []string{}
	for _, s := range spec {
		header = append(header, s.Title)
	}
	table := [][]string{header, nil}
	count := 0
	for _, name := range files {
		f := GetDb().GetFile(name)
		if f == nil || f.Doc == nil {
			continue
		}
		var hits []*org.Section
		for _, v := range f.Doc.Outline.Children {
			hits, _ = EvalForNodes(exp, v, f, hits)
		}
		sort.SliceStable(hits, func(a, b int) bool { return headlineRow(hits[a]) < headlineRow(hits[b]) })
		lines := fileLines(f.Doc.Path)
		for _, s := range hits {
			if s.Headline == nil {
				continue
			}
			if limit > 0 && count >= limit {
				break
			}
			count++
			props := propsOfSection(lines, s)
			row := ownRow(f, s, props, spec, len(s.Children) > 0)
			cells := []string{}
			for i, cell := range row.Cells {
				v := cell.Value
				switch spec[i].Property {
				case "FILE":
					v = relativeName(c.File.Doc.Path, f.Doc.Path)
				case "ITEM":
					if link {
						v = headingLink(c.File.Doc.Path, f.Doc.Path, row.Headline)
					}
				}
				cells = append(cells, cellText(v))
			}
			table = append(table, cells)
		}
	}
	return orgTableLines(table), nil
}

// A query with the yaml's {{ Filters }} written into it, as /search does.
func expandQueryFilters(q string) string {
	tempo := Conf().PlugManager.Tempo
	if tempo == nil {
		return q
	}
	return tempo.ExecuteTemplateString(q, tempo.GetAugmentedStandardContextFromStringMap(Conf().Filters, true))
}

// A file named the way a link from `from` would name it: relative when it is
// beside or below it, absolute otherwise.
func relativeName(from, to string) string {
	if rel, err := filepath.Rel(filepath.Dir(from), to); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return to
}

// A link to a heading by its title, written from the file at `from`.
func headingLink(from, to, title string) string {
	desc := regexp.MustCompile(`\[\[(?:[^\]]*)\]\[([^\]]*)\]\]|\[\[([^\]]*)\]\]`).ReplaceAllString(title, "$1$2")
	desc = strings.NewReplacer("[", "{", "]", "}").Replace(desc)
	target := "*" + desc
	if from != to {
		target = "file:" + relativeName(from, to) + "::" + target
	}
	return "[[" + target + "][" + desc + "]]"
}

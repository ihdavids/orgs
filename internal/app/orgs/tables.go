package orgs

/* SDOC: Editing
* Table Browsing And Editing

  The table endpoints let a client list every org table the server knows about
  and edit one as a spreadsheet. A table is addressed by the file it lives in
  plus its ordinal position within that file (=Id=), counted in document order
  starting at zero.

  Writing a table only rewrites the lines the table occupies, so the rest of the
  file is left byte for byte as it was. The =#+TBLFM:= lines of a table are
  rewritten as a single line holding every formula, separated by =::=.
EDOC */

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// A table plus the context needed to find it again and write it back out.
type tableRef struct {
	Table *org.Table
	Sec   *org.Section
	File  *common.OrgFile
	Name  string
	Id    int
}

var nameKeywordRe = regexp.MustCompile(`^\s*#\+(?i:NAME):`)
var tblfmKeywordRe = regexp.MustCompile(`^\s*#\+(?i:TBLFM):\s*(.*)$`)

// A named node may be wrapped in a name or metadata node, dig out the table.
func unwrapTable(n org.Node) *org.Table {
	switch t := n.(type) {
	case *org.Table:
		return t
	case org.NodeWithName:
		return unwrapTable(t.Node)
	case *org.NodeWithName:
		return unwrapTable(t.Node)
	case org.NodeWithMeta:
		return unwrapTable(t.Node)
	case *org.NodeWithMeta:
		return unwrapTable(t.Node)
	}
	return nil
}

// Reverse the documents named node map so a table can look up its own name.
func namedTables(f *common.OrgFile) map[*org.Table]string {
	res := map[*org.Table]string{}
	for name, node := range f.Doc.NamedNodes {
		if t := unwrapTable(node); t != nil {
			res[t] = name
		}
	}
	return res
}

func collectSectionTables(sec *org.Section, f *common.OrgFile, names map[*org.Table]string, out *[]*tableRef) {
	if sec.Headline != nil {
		for _, t := range sec.Headline.Tables {
			*out = append(*out, &tableRef{Table: t, Sec: sec, File: f, Name: names[t], Id: len(*out)})
		}
	}
	for _, c := range sec.Children {
		collectSectionTables(c, f, names, out)
	}
}

// Every table in a file, in document order. Tables that sit above the first
// heading are not attached to a section so they are picked up from the
// documents top level node list instead.
func collectFileTables(f *common.OrgFile) []*tableRef {
	out := []*tableRef{}
	if f == nil || f.Doc == nil {
		return out
	}
	names := namedTables(f)
	for _, n := range f.Doc.Nodes {
		if t := unwrapTable(n); t != nil {
			out = append(out, &tableRef{Table: t, Sec: nil, File: f, Name: names[t], Id: len(out)})
		}
	}
	for _, sec := range f.Doc.Outline.Children {
		collectSectionTables(sec, f, names, &out)
	}
	return out
}

func (s *tableRef) Heading() string {
	if s.Sec == nil {
		return ""
	}
	return common.BuildOutlinePath(s.Sec, "/")
}

func (s *tableRef) Olp() []string {
	h := s.Heading()
	if h == "" {
		return []string{}
	}
	return strings.Split(h, "/")
}

// The last line the table occupies, its #+TBLFM: lines included.
func tableLastLine(tbl *org.Table) int {
	last := tbl.Pos.Row + len(tbl.Rows) - 1
	if tbl.Formulas != nil {
		for _, k := range tbl.Formulas.Keywords {
			if k != nil && k.Pos.Row > last {
				last = k.Pos.Row
			}
		}
	}
	return last
}

func cellValue(col *org.Column) string {
	w := org.OrgWriter{}
	v := strings.TrimSpace(w.WriteNodesAsString(col.Children...))
	// A cell cannot hold a bare pipe, we write it as the org entity on the way
	// in so hand it back as the pipe the user typed.
	return strings.ReplaceAll(v, `\vert{}`, "|")
}

func tableRows(tbl *org.Table) []common.TableRow {
	rows := []common.TableRow{}
	width := tbl.GetWidth()
	for i, row := range tbl.Rows {
		if tbl.IsSeparatorRow(i) || len(row.Columns) == 0 {
			rows = append(rows, common.TableRow{Kind: "sep", Cells: make([]string, width)})
			continue
		}
		cells := make([]string, 0, len(row.Columns))
		for _, col := range row.Columns {
			cells = append(cells, cellValue(col))
		}
		rows = append(rows, common.TableRow{Kind: "data", Advanced: row.IsAdvanced, Cells: cells})
	}
	return rows
}

// Work out which cell each formula writes to so a client can show the formula
// of the selected cell. Keys are "rawrow,col", both zero based, matching the
// row list handed back by tableRows.
func tableCellFormulas(tbl *org.Table) map[string]string {
	res := map[string]string{}
	if tbl.Formulas == nil || len(tbl.Rows) == 0 {
		return res
	}
	tbl.Formulas.Process(tbl)
	cur := tbl.Cur
	defer func() { tbl.Cur = cur }()
	for _, frml := range tbl.Formulas.Formulas {
		if frml == nil || frml.Target == nil || frml.Expr == "" {
			continue
		}
		out := frml.Target.CreateIterator(tbl)
		for {
			tgt := out()
			if tgt == nil {
				break
			}
			// Relative ranges resolve against the cell being written, so the
			// current position has to move with the iterator.
			tbl.Cur.Row = tgt.Row
			tbl.Cur.Col = tgt.Col
			row, col := tbl.GetRealRowCol(tgt.Row, tgt.Col)
			if row < 0 || row >= len(tbl.Rows) || org.ShouldSkipAdvancedRow(tbl.Rows[row].IsAdvanced) {
				continue
			}
			if col < 0 || col >= len(tbl.Rows[row].Columns) {
				continue
			}
			res[fmt.Sprintf("%d,%d", row, col)] = frml.FormulaStr
		}
	}
	return res
}

func tableFormulaList(tbl *org.Table) []string {
	res := []string{}
	if tbl.Formulas == nil {
		return res
	}
	tbl.Formulas.Process(tbl)
	for _, frml := range tbl.Formulas.Formulas {
		if frml == nil || frml.FormulaStr == "" {
			continue
		}
		res = append(res, frml.FormulaStr)
	}
	return res
}

func tableInfo(ref *tableRef) common.TableInfo {
	return common.TableInfo{
		Id:       ref.Id,
		Filename: ref.File.Filename,
		Name:     ref.Name,
		Heading:  ref.Heading(),
		Olp:      ref.Olp(),
		Line:     ref.Table.Pos.Row,
		Rows:     ref.Table.GetHeight(),
		Cols:     ref.Table.GetWidth(),
		Formulas: len(tableFormulaList(ref.Table)),
	}
}

func tableData(ref *tableRef, lines []string) *common.TableData {
	indent := ""
	if ref.Table.Pos.Row < len(lines) {
		indent = leadingWhitespace(lines[ref.Table.Pos.Row])
	}
	return &common.TableData{
		TableInfo:    tableInfo(ref),
		EndLine:      tableLastLine(ref.Table),
		Data:         tableRows(ref.Table),
		FormulaList:  tableFormulaList(ref.Table),
		CellFormulas: tableCellFormulas(ref.Table),
		ColNames:     ref.Table.ColNames,
		Params:       ref.Table.Params,
		Indent:       indent,
	}
}

func leadingWhitespace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func readFileLines(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

// A cell has to survive a round trip through the org table syntax.
func sanitizeCell(v string) string {
	v = strings.ReplaceAll(v, "\r\n", " ")
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	v = strings.ReplaceAll(v, "|", `\vert{}`)
	return strings.TrimSpace(v)
}

// Build a real org table out of the rows a client posted by writing them as
// org text and parsing that back. Going through the parser is what gives us
// the column names, parameters and formulas of the new table without having
// to duplicate any of the parsers work.
func buildTable(rows []common.TableRow, formulas []string) (*org.Table, error) {
	width := 0
	dataRows := 0
	for _, r := range rows {
		if len(r.Cells) > width {
			width = len(r.Cells)
		}
		if r.Kind != "sep" {
			dataRows += 1
		}
	}
	if width <= 0 || dataRows <= 0 {
		return nil, fmt.Errorf("a table needs at least one row and one column")
	}
	var sb strings.Builder
	// The parser only hangs tables off a headline, so give it one to hang on.
	sb.WriteString("* t\n")
	for _, r := range rows {
		if r.Kind == "sep" {
			sb.WriteString("|")
			for c := 0; c < width; c++ {
				sb.WriteString("---")
				if c < width-1 {
					sb.WriteString("+")
				}
			}
			sb.WriteString("|\n")
			continue
		}
		sb.WriteString("|")
		for c := 0; c < width; c++ {
			v := ""
			if c < len(r.Cells) {
				v = sanitizeCell(r.Cells[c])
			}
			sb.WriteString(" " + v + " |")
		}
		sb.WriteString("\n")
	}
	frmls := []string{}
	for _, f := range formulas {
		f = strings.TrimSpace(strings.ReplaceAll(f, "\n", " "))
		if f != "" {
			frmls = append(frmls, f)
		}
	}
	if len(frmls) > 0 {
		sb.WriteString("#+TBLFM: " + strings.Join(frmls, "::") + "\n")
	}
	doc := org.New().Silent().Parse(strings.NewReader(sb.String()), "")
	if doc == nil || doc.Error != nil {
		return nil, fmt.Errorf("could not parse the rewritten table")
	}
	if len(doc.Outline.Children) <= 0 || doc.Outline.Children[0].Headline == nil ||
		len(doc.Outline.Children[0].Headline.Tables) <= 0 {
		return nil, fmt.Errorf("the rewritten table did not parse as a table")
	}
	return doc.Outline.Children[0].Headline.Tables[0], nil
}

// Serialize a table the way org would write it, formulas included.
func renderTable(tbl *org.Table, indent string) string {
	tbl.RecomputeColumnInfos()
	w := org.NewOrgWriter()
	w.Indent = indent
	w.SetLineBreak()
	org.WriteNodes(w, tbl)
	out := w.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if tbl.Formulas != nil {
		for _, k := range tbl.Formulas.Keywords {
			if k == nil || strings.TrimSpace(k.Value) == "" {
				continue
			}
			out += indent + "#+TBLFM: " + strings.TrimSpace(k.Value) + "\n"
		}
	}
	return out
}

// Replace the lines of an existing table with new text, leaving the rest of
// the file untouched. Returns the rewritten file content.
func spliceTable(lines []string, start, end int, text string) (string, error) {
	if start < 0 || start >= len(lines) || end < start {
		return "", fmt.Errorf("table is not where the database says it is, reload and try again")
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	newLines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	out := []string{}
	out = append(out, lines[:start]...)
	out = append(out, newLines...)
	out = append(out, lines[end+1:]...)
	return strings.Join(out, "\n"), nil
}

// Write one table back into its file and reload the file in the database.
func saveTableText(ref *tableRef, tbl *org.Table, name string, setName bool) error {
	filename := ref.File.Filename
	lines, err := readFileLines(filename)
	if err != nil {
		return err
	}
	start := ref.Table.Pos.Row
	end := tableLastLine(ref.Table)
	if start >= len(lines) {
		return fmt.Errorf("table is not where the database says it is, reload and try again")
	}
	indent := leadingWhitespace(lines[start])
	if setName {
		name = strings.TrimSpace(strings.ReplaceAll(name, "\n", " "))
		hasName := start > 0 && nameKeywordRe.MatchString(lines[start-1])
		if hasName && name == "" {
			lines = append(lines[:start-1], lines[start:]...)
			start -= 1
			end -= 1
		} else if hasName {
			lines[start-1] = indent + "#+NAME: " + name
		} else if name != "" {
			lines = append(lines[:start], append([]string{indent + "#+NAME: " + name}, lines[start:]...)...)
			start += 1
			end += 1
		}
	}
	content, err := spliceTable(lines, start, end, renderTable(tbl, indent))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return err
	}
	GetDb().ReloadFile(filename)
	return nil
}

// Look a table up by file and ordinal.
func findTable(filename string, id int) (*tableRef, error) {
	ofile := GetDb().GetFile(filename)
	if ofile == nil {
		return nil, fmt.Errorf("unknown org file [%s]", filename)
	}
	refs := collectFileTables(ofile)
	if id < 0 || id >= len(refs) {
		return nil, fmt.Errorf("file [%s] does not have a table %d", filename, id)
	}
	return refs[id], nil
}

// The parser only hangs #+TBLFM: lines off a table that lives under a heading,
// so a table above the first heading in a file would come back without its
// formulas. Pick those up off the file itself, otherwise we would neither run
// them nor rewrite them when the table is saved.
func attachOrphanFormulas(tbl *org.Table, lines []string) {
	if tbl == nil || tbl.Formulas != nil {
		return
	}
	keywords := []*org.Keyword{}
	for i := tbl.Pos.Row + len(tbl.Rows); i >= 0 && i < len(lines); i++ {
		m := tblfmKeywordRe.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}
		keywords = append(keywords, &org.Keyword{
			Pos:    org.Pos{Row: i, Col: len(leadingWhitespace(lines[i]))},
			EndPos: org.Pos{Row: i, Col: len(lines[i])},
			Key:    "TBLFM",
			Value:  strings.TrimSpace(m[1]),
		})
	}
	if len(keywords) > 0 {
		tbl.Formulas = &org.Formulas{Keywords: keywords}
	}
}

// Find a table and the lines of the file it lives in, ready to read or write.
func loadTable(filename string, id int) (*tableRef, []string, error) {
	ref, err := findTable(filename, id)
	if err != nil {
		return nil, nil, err
	}
	lines, err := readFileLines(ref.File.Filename)
	if err != nil {
		return nil, nil, err
	}
	if ref.Sec == nil {
		attachOrphanFormulas(ref.Table, lines)
	}
	return ref, lines, nil
}

/* SDOC: API
* GET /tables — List All Tables
	Returns a summary of every table in every org file the server has loaded, in
	document order. A table is identified by its =Filename= and its =Id=, the
	ordinal of the table within that file. Both are what the other table
	endpoints take.

	*Method:* =GET=

	*Parameters:*
	| Parameter   | Type   | Required | Description                                  |
	|-------------+--------+----------+----------------------------------------------|
	| =filename=  | string | no       | Only list the tables of this org file.       |

	*Response:* A =TableListResult= JSON object:
	#+BEGIN_SRC json
	{"Ok": true, "Msg": "", "Tables": [
	  {"Id": 0, "Filename": "/org/budget.org", "Name": "expenses", "Heading": "Money/Budget",
	   "Olp": ["Money", "Budget"], "Line": 12, "Rows": 6, "Cols": 3, "Formulas": 2}]}
	#+END_SRC
	EDOC */
func RequestTables(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	res := common.TableListResult{Ok: true, Tables: []common.TableInfo{}}
	files := []string{}
	if filename != "" {
		if f := GetDb().GetFile(filename); f != nil {
			files = append(files, f.Filename)
		} else {
			res = common.TableListResult{Ok: false, Msg: fmt.Sprintf("unknown org file [%s]", filename)}
			json.NewEncoder(w).Encode(res)
			return
		}
	} else {
		files = append(files, GetDb().GetFiles()...)
		sort.Strings(files)
	}
	for _, name := range files {
		ofile := GetDb().GetFile(name)
		if ofile == nil {
			continue
		}
		refs := collectFileTables(ofile)
		var lines []string
		for _, ref := range refs {
			// A table above the first heading carries its formulas in the file
			// rather than in the parse tree, so the file has to be consulted.
			if ref.Sec == nil && ref.Table.Formulas == nil {
				if lines == nil {
					lines, _ = readFileLines(ofile.Filename)
				}
				attachOrphanFormulas(ref.Table, lines)
			}
			res.Tables = append(res.Tables, tableInfo(ref))
		}
	}
	json.NewEncoder(w).Encode(res)
}

/* SDOC: API
* GET /table — Get One Table
	Returns the full contents of a single table: its rows, its formulas, and a
	map telling you which formula writes which cell so an editor can show the
	formula of the selected cell.

	*Method:* =GET=

	*Parameters:*
	| Parameter  | Type   | Required | Description                                       |
	|------------+--------+----------+---------------------------------------------------|
	| =filename= | string | yes      | The org file holding the table.                   |
	| =id=       | int    | yes      | Ordinal of the table within the file, from zero.  |

	*Response:* A =TableResult= JSON object. =Table.Data= holds one entry per
	line of the table, each either a ="data"= row with its cells or a ="sep"=
	row standing for a =|---+---|= rule. =Table.CellFormulas= is keyed by
	="row,col"=, both zero based, indexing into =Table.Data=.
	EDOC */
func RequestTable(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	id := 0
	fmt.Sscanf(r.URL.Query().Get("id"), "%d", &id)
	ref, lines, err := loadTable(filename, id)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(common.TableResult{Ok: true, Table: tableData(ref, lines)})
}

/* SDOC: API
* POST /table — Write A Table Back To Its File
	Replaces the rows and formulas of one table. Only the lines the table
	occupies are rewritten, the rest of the file is left alone. Every formula in
	=Formulas= is written to a single =#+TBLFM:= line, separated by =::=.

	With =Eval= set the tables formulas are run after the save and the computed
	values are written to the file as well, exactly as =/exectable= would.

	*Method:* =POST=

	*Request Body (JSON):* A =TableEdit= object.
	| Field      | Type       | Required | Description                                          |
	|------------+------------+----------+------------------------------------------------------|
	| =Filename= | string     | yes      | The org file holding the table.                      |
	| =Id=       | int        | yes      | Ordinal of the table within the file, from zero.     |
	| =Data=     | []TableRow | yes      | Replacement rows, ="data"= or ="sep"=.               |
	| =Formulas= | []string   | no       | Replacement formulas, e.g. =["$3=$1*$2;%.2f"]=.      |
	| =Name=     | string     | no       | New =#+NAME:= for the table, empty removes it.       |
	| =SetName=  | bool       | no       | Only touch the =#+NAME:= line when this is set.      |
	| =Eval=     | bool       | no       | Run the tables formulas after saving.                |

	*Response:* A =TableResult= JSON object holding the table as it now reads
	back from disk.
	EDOC */
func PostTable(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	var args common.TableEdit
	if err = json.Unmarshal(body, &args); err != nil {
		fmt.Println("Table edit failed to deserialize", err, string(body))
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	ref, _, err := loadTable(args.Filename, args.Id)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	tbl, err := buildTable(args.Data, args.Formulas)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	if err = saveTableText(ref, tbl, args.Name, args.SetName); err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	msg := ""
	if args.Eval {
		if err = evalTable(args.Filename, args.Id); err != nil {
			// The rows are saved either way, a bad formula should not lose them.
			msg = err.Error()
		}
	}
	ref, lines, err := loadTable(args.Filename, args.Id)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(common.TableResult{Ok: msg == "", Msg: msg, Table: tableData(ref, lines)})
}

// Run the #+TBLFM: formulas of a table and write the results back to the file.
func evalTable(filename string, id int) error {
	ref, _, err := loadTable(filename, id)
	if err != nil {
		return err
	}
	if ref.Table.Formulas == nil {
		return nil
	}
	if err = ExecuteFormula(db, ref.Sec, ref.File, ref.Table); err != nil {
		return err
	}
	return saveTableText(ref, ref.Table, "", false)
}

/* SDOC: API
* POST /table/eval — Recalculate A Table
	Runs the =#+TBLFM:= formulas of one table and writes the computed values
	back to the org file. This is =/exectable= addressed by file and table
	ordinal rather than by heading and row.

	*Method:* =POST=

	*Request Body (JSON):*
	#+BEGIN_SRC json
	{"Filename": "/org/budget.org", "Id": 0}
	#+END_SRC

	*Response:* A =TableResult= JSON object holding the recalculated table.
	EDOC */
func PostTableEval(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	var args common.TableEdit
	if err = json.Unmarshal(body, &args); err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	msg := ""
	if err = evalTable(args.Filename, args.Id); err != nil {
		msg = err.Error()
	}
	ref, lines, err := loadTable(args.Filename, args.Id)
	if err != nil {
		json.NewEncoder(w).Encode(common.TableResult{Ok: false, Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(common.TableResult{Ok: msg == "", Msg: msg, Table: tableData(ref, lines)})
}

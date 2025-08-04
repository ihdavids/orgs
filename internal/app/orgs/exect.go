package orgs

/* SDOC: Editing
* Table Execution

  TODO: Fill in information on table execution
EDOC */

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/govaluate"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

func ExecTable(db plugs.ODb, ofile *common.OrgFile, sec *org.Section, tbl *org.Table) (common.ResultMsg, error) {
	res := common.ResultMsg{Ok: false, Msg: "Unknown table exec error"}
	strt := tbl.GetPos()
	end := tbl.GetEnd()
	res.Pos = org.Pos{Row: strt.Row, Col: strt.Col}
	res.End = org.Pos{Row: end.Row, Col: end.Col}
	//fmt.Printf("GOING TO EXECUTE\n")
	// Okay we have a table and we have a formula, lets go execute it!
	if err := ExecuteFormula(db, sec, ofile, tbl); err != nil {
		res.Msg = err.Error()
		return res, err
	}
	w := org.NewOrgWriter()
	// Recompute our sizes so the table is the correct size.
	tbl.RecomputeColumnInfos()
	//fmt.Printf("SERIALIZE\n")
	// Setup our indent for the current node so the table is indented properly
	w.Indent = strings.Repeat(" ", sec.Headline.Lvl+1)
	// Respect the fact that a table represents an implicit line break
	w.SetLineBreak()
	// Write out our table nodes.
	org.WriteNodes(w, tbl)
	res.Msg = w.String()
	res.Ok = true
	/*
		writer := bytes.Buffer{}
		out := tablewriter.NewWriter(&writer)
		// Org mode style table borders
		out.SetBorders(tablewriter.Border{Top: false, Left: true, Right: true, Bottom: false, CenterMarkers: false})
		out.SetAutoFormatHeaders(false)
		tbl.
		if len(ifo.Headers) > 0 || len(ifo.Cells) > 0 {
			out.SetHeader(ifo.Headers)
			out.AppendBulk(ifo.Cells)
			out.Render()
		}
	*/
	return res, nil
}

// REMOVE: UNUSED CURRENTLY????
func RequestTableRandomGetFromDb(db plugs.ODb, t *common.PreciseTarget) (common.ResultMsg, error) {
	ofile, sec, table := db.GetFromPreciseTarget(t, org.TableNode)
	res := common.ResultMsg{Ok: false, Msg: "Unknown table get error"}
	if table != nil {
		var tbl *org.Table
		var tok bool
		if tbl, tok = table.(*org.Table); tok {
			if tbl.Formulas == nil {
				res.Msg = fmt.Sprintf("Cannot execute table does not have a formula [%s]:[%s]", ofile.Filename, common.GetSectionTitle(sec))
				return res, nil
			}
		} else {
			res.Msg = fmt.Sprintf("did not find table, cannot execute")
			return res, nil
		}
		return ExecTable(db, ofile, sec, tbl)
	}
	return res, nil
}

func ExecTableAt(db plugs.ODb, t *common.PreciseTarget) (common.ResultMsg, error) {
	ofile, sec, table := db.GetFromPreciseTarget(t, org.TableNode)
	res := common.ResultMsg{Ok: false, Msg: "Unknown table exec error"}
	if table != nil {
		var tbl *org.Table
		var tok bool
		if tbl, tok = table.(*org.Table); tok {
			if tbl.Formulas == nil {
				res.Msg = fmt.Sprintf("Cannot execute table does not have a formula [%s]:[%s]", ofile.Filename, common.GetSectionTitle(sec))
				return res, nil
			}
		} else {
			res.Msg = fmt.Sprintf("did not find table, cannot execute")
			return res, nil
		}
		return ExecTable(db, ofile, sec, tbl)
	}
	return res, nil
}

func ExecTablesInSubheadings(db plugs.ODb, ofile *common.OrgFile, sec *org.Section) ([]common.ResultMsg, []error) {
	res := []common.ResultMsg{}
	err := []error{}
	for _, c := range sec.Children {
		if c.Headline.Tables != nil {
			for _, tbl := range c.Headline.Tables {
				r, e := ExecTable(db, ofile, c, tbl)
				if e != nil {
					err = append(err, e)
				}
				res = append(res, r)
				rs, es := ExecTablesInSubheadings(db, ofile, c)
				if len(es) > 0 {
					err = append(err, es...)
				}
				if len(rs) > 0 {
					res = append(res, rs...)
				}
			}
		}
	}
	return res, err
}

func ExecAllTables(db plugs.ODb, filename string) ([]common.ResultMsg, []error) {
	res := []common.ResultMsg{}
	err := []error{}
	ofile := db.GetFile(filename)
	for _, sec := range ofile.Doc.Outline.Children {
		if sec.Headline.Tables != nil {
			for _, tbl := range sec.Headline.Tables {
				r, e := ExecTable(db, ofile, sec, tbl)
				if e != nil {
					fmt.Printf("HAVE TABLE ERROR: %s\n", e)
					err = append(err, e)
				}
				res = append(res, r)
				rs, es := ExecTablesInSubheadings(db, ofile, sec)
				if len(es) > 0 {
					err = append(err, es...)
				}
				if len(rs) > 0 {
					res = append(res, rs...)
				}
			}
		}
	}
	return res, err
}

func makelistfromrange(tbl *org.Table, mode *CalcState, it org.ColRefIterator) (interface{}, error) {
	res := []interface{}{}
	for {
		ref := it()
		for ; ref != nil && mode != nil && mode.ShouldSkip(tbl, ref); ref = it() {
			// We skip over empty cells if told to by the CalcState
		}
		if ref == nil {
			break
		}
		v := mode.GetCell(tbl, ref)
		res = append(res, v)
	}
	return res, nil
}

func TableHasHeader(tbl *org.Table) bool {
	return len(tbl.SeparatorIndices) > 0 && tbl.SeparatorIndices[0] == 1
}

/*
func makerange(frml *org.Formula, expr *Expr, tbl *org.Table, args ...interface{}) (interface{}, error) {
	if len(args) > 0 {
		form := org.FormulaTarget{Raw: args[0].(string)}
		form.Process(tbl)
		it := form.CreateIterator(expr.Tbl)
		return makelistfromrange(tbl, it)
	}
	return 0, fmt.Errorf("range expression failed to parse [%s]", frml.Expr)
}
*/

func ParseTableFormula(tbl *org.Table, frml *org.Formula, sec *org.Section, ofile *common.OrgFile) (*Expr, error) {
	expr := &Expr{}
	expr.Sec = sec
	expr.File = ofile
	expr.Doc = ofile.Doc
	expr.Tbl = tbl

	// This has to be recreated every time to reference the state.
	// the rest do not currently need state, wish govaluate had the ability to pass state to the functions
	//functions["makerange"] = func(args ...interface{}) (interface{}, error) { return makerange(frml, expr, tbl, args...) }
	functions["remote"] = func(args ...interface{}) (interface{}, error) { return tblRemote(ofile.Filename, args...) }

	var err error
	expr.Expression, err = govaluate.NewEvaluableExpressionWithFunctions(frml.Expr, functions)
	return expr, err
}

/*
parameters := make(map[string]interface{}, 8)
parameters["section"] = v
// This is the implicit this pointer of our expressions
exp.Sec = v
exp.Doc = f.Doc
exp.File = f
result, _ := exp.Expression.Evaluate(parameters)

	if result != nil {
		return result.(bool)
	}
*/
//var tableTargetRe = regexp.MustCompile(`\s*(([@](?P<rowonly>[-]?[0-9><]+))|([$](?P<colonly>[-]?[0-9><]+))|([@](?P<row>[-]?[0-9><]+)[$](?P<col>[-]?[0-9><]+)))\s*`)

// var RE_TARGET_A = regexp.MustCompile(`\s*(([@](?P<row>(?P<rsign>[+-])?([0-9]+)|([>]+)|([<]+)|[#])[$](?P<col>((?P<csign>[+-])?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))|([@](?P<rowonly>((?P<rosign>[+-])?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))|([$](?P<colonly>((?P<cosign>[+-])?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\)))))(?P<end>[^@$]|$)`)
var rowColRe = `\s*(([@](?P<row>[+-]?([0-9]+)|([>]+)|([<]+)|[#])[$](?P<col>([+-]?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))|([@](?P<rowonly>([+-]?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))|([$](?P<colonly>([+-]?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\)))))(\.\.(([@](?P<row2>[+-]?([0-9]+)|([>]+)|([<]+)|[#])[$](?P<col2>([+-]?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))|([@](?P<rowonly2>([+-]?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))|([$](?P<colonly2>([+-]?([0-9]+)|([>]+)|([<]+)|[#])|(ridx\(\))|(cidx\(\))))))?`
var RE_TARGET_A = regexp.MustCompile(rowColRe)
var RE_REMOTE = regexp.MustCompile(`remote\(\s*(?P<tblnm>[a-zA-Z][a-zA-Z0-9]*)\s*,(?P<range>` + rowColRe + `\s*)\)`)
var RE_ROW_TOKEN = regexp.MustCompile(`[@][#]`)
var RE_COL_TOKEN = regexp.MustCompile(`[$][#]`)
var RE_SYMBOL_OR_CELL_NAME = regexp.MustCompile(`[$](?P<name>[a-zA-Z][a-zA-Z0-9_-]*)`)

type RangeIter struct {
	It   org.ColRefIterator
	Val  string
	Tbl  *org.Table
	Mode *CalcState
	Form org.FormulaTarget
	Idx  int
}
type RangeMap map[string]*RangeIter

func (s *RangeIter) IsRange() bool {
	return (s.Form.Start.Row != s.Form.End.Row) || (s.Form.Start.Col != s.Form.End.Col)
}

func (s *RangeIter) Reset() {
	s.Idx = -1
	s.It = s.Form.CreateIterator(s.Tbl)
}

func (s *RangeIter) Next() string {
	if s.IsRange() { // This is an iterator so return the current iterator value
		// Skip over any empty cells
		ref := s.It()
		for ; ref != nil && s.Mode != nil && s.Mode.ShouldSkip(s.Tbl, ref); ref = s.It() {
			// We skip over empty cells if told to by the CalcState
		}
		s.Val = strings.TrimSpace(s.Tbl.GetValRef(ref))
		s.Idx = 1
	} else { // This is a cell so return the current cell value from the table!
		ref := s.Form.Start
		if ref.WildCol {
			ref.Col = s.Tbl.CurrentCol()
		}
		if ref.WildRow {
			ref.Row = s.Tbl.CurrentRow()
		}
		s.Val = strings.TrimSpace(s.Tbl.GetValRef(&ref))
		s.Idx = s.Idx + 1
	}
	return s.Val
}

func (s *RangeIter) String() string {
	if s.IsRange() {
		return "<Iterator>"
	} else {
		return s.Next()
	}
}

func (s *RangeIter) NextVal() interface{} {
	v := s.Next()
	if s.Mode != nil {
		return s.Mode.ProcessCellVal(v)
	} else {
		if nv, err := strconv.ParseFloat(v, 64); err == nil {
			return nv
		}
	}
	return v
}

func (s *RangeIter) GetCurFloat64() float64 {
	s.EnsureFirst()
	if s.Val != "" {
		v := s.Mode.ProcessCellVal(s.Val)
		if ret, ok := v.(float64); ok {
			return ret
		}
	}
	return 0.0
}

func (s *RangeIter) GetCurBool() bool {
	s.EnsureFirst()
	if s.Val != "" {
		v := s.Mode.ProcessCellVal(s.Val)
		if ret, ok := v.(bool); ok {
			return ret
		}
	}
	return false
}

func (s *RangeIter) TestGetCurFloat64() (float64, bool) {
	s.EnsureFirst()
	if s.Val != "" {
		v := s.Mode.ProcessCellVal(s.Val)
		if ret, ok := v.(float64); ok {
			return ret, true
		}
	}
	return 0.0, false
}

func (s *RangeIter) TestGetCurTime() (time.Time, bool) {
	s.EnsureFirst()
	if s.Val != "" {
		v := s.Mode.ProcessCellVal(s.Val)
		if ret, ok := v.(time.Time); ok {
			return ret, true
		}
	}
	return time.Time{}, false
}

func (s *RangeIter) TestGetCurDuration() (common.OrgDuration, bool) {
	s.EnsureFirst()
	if s.Val != "" {
		v := s.Mode.ProcessCellVal(s.Val)
		if ret, ok := v.(common.OrgDuration); ok {
			return ret, true
		}
	}
	return common.NewDuration(0), false
}

func (s *RangeIter) TestGetCurBool() (bool, bool) {
	s.EnsureFirst()
	if s.Val != "" {
		v := s.Mode.ProcessCellVal(s.Val)
		if ret, ok := v.(bool); ok {
			return ret, true
		}
	}
	return false, false
}

func (s *RangeIter) EnsureFirst() string {
	if s.Idx < 0 {
		s.Next()
	}
	return s.Val
}

type CalcState struct {
	Format              string
	AllFieldsNumbers    bool
	ConsiderEmptyFields bool
	MoneyMode           bool
	SkipHeader          bool
}

/*
‘p20’                  Set the internal Calc calculation precision to 20 digits.
‘n3’, ‘s3’, ‘e2’, ‘f4’ Normal, scientific, engineering or fixed format of the result of Calc passed back to Org. Calc formatting is unlimited in precision as long as the Calc calculation precision is greater.
‘D’, ‘R’               Degree and radian angle modes of Calc.
‘F’, ‘S’               Fraction and symbolic modes of Calc.
‘u’                    Units simplification mode of Calc. Calc is also a symbolic calculator and is capable of working with values having a unit, represented with numerals followed by a unit string in Org table cells. This mode instructs Calc to simplify the units in the computed expression before returning the result.
‘T’, ‘t’, ‘U’          Duration computations in Calc or Lisp, Durations and time values.
‘E’                    If and how to consider empty fields. Without ‘E’ empty fields in range references are suppressed so that the Calc vector or Lisp list contains only the non-empty fields. With ‘E’ the empty fields are kept. For empty fields in ranges or empty field references the value ‘nan’ (not a number) is used in Calc formulas and the empty string is used for Lisp formulas. Add ‘N’ to use 0 instead for both formula types. For the value of a field the mode ‘N’ has higher precedence than ‘E’.
‘N’                    Interpret all fields as numbers, use 0 for non-numbers. See the next section to see how this is essential for computations with Lisp formulas. In Calc formulas it is used only occasionally because there number strings are already interpreted as numbers without ‘N’.
‘L’                    Literal, for Lisp formulas only. See the next section.
// NON ORG MODE STUFF:
'$'                    Money Mode - Treat numerical cells as monetary values and read write with $
'H'                    Skip Header Mode - If table has a header with separator it is skipped in column commands
*/
var fspec = regexp.MustCompile(`%[+-]?\d*\.?\d* ?[oxdvTbcqUefgs]`)

func (s *CalcState) ProcessCalcSpecifiers(format string) string {
	s.SkipHeader = true
	if format != "" {
		// Strip off any format specifier in the string.
		fmat := fspec.FindString(format)
		format = strings.TrimSpace(string(fspec.ReplaceAll([]byte(format), []byte(""))))
		s.Format = strings.TrimSpace(fmat)
		// Now look for mode strings. NOT ALL calc state will work and we are going to add our own!
		if format != "" {
			s.AllFieldsNumbers = strings.Contains(format, "N")
			s.ConsiderEmptyFields = strings.Contains(format, "E")
			s.MoneyMode = strings.Contains(format, "$")
			s.SkipHeader = !strings.Contains(format, "H")
		}
	}
	return s.Format
}

func (s *CalcState) ProcessCellVal(val string) interface{} {
	// Empty cell processing should sometimes return 0 if asked for.
	if val == "" {
		if s.AllFieldsNumbers {
			return 0.0
		}
		return val
	}

	// In money mode we strip off $ so we can handle money on our numeric data
	if s.MoneyMode {
		val = strings.Replace(val, "$", "", 1)
	}

	// Then fall back on potential number
	if nv, err := strconv.ParseFloat(val, 64); err == nil {
		return nv
	}

	// If we force numeric interpretation then we interpret as that
	if s.AllFieldsNumbers {
		if isTrue(val) {
			return 1.0
		}
		return 0.0
	}

	// Dates are built in to cell processing.
	if tm, err := common.ParseDateString(val); err == nil {
		return tm
	}

	// Durations are built in to cell processing.
	if d := common.ParseDuration(val); d != nil {
		return *d
	}

	// Auto convert to boolean... might be something we have to be careful with.
	if isTrue(val) {
		return true
	}
	if isFalse(val) {
		return false
	}
	return val
}

func (s *CalcState) GetCell(tbl *org.Table, r *org.RowColRef) interface{} {
	val := strings.TrimSpace(tbl.GetValRef(r))
	return s.ProcessCellVal(val)
}

func (s *CalcState) ShouldSkip(tbl *org.Table, r *org.RowColRef) bool {
	if s.SkipHeader && TableHasHeader(tbl) && r.Row == 1 {
		return true
	}
	if s.ConsiderEmptyFields {
		return false
	} else {
		val := strings.TrimSpace(tbl.GetValRef(r))
		return val == ""
	}
}

func (s *CalcState) SetCell(tbl *org.Table, tgt *org.RowColRef, val interface{}) {
	format := "%v"
	if s.Format != "" && s.Format[0] == '%' {
		format = s.Format
	}
	if s.MoneyMode {
		format = "$" + format
	}
	if d, ok := val.(time.Time); ok {
		val = fmt.Sprintf("<%s>", d.Format("2006-01-02 Mon 15:04"))
	}
	if d, ok := val.(common.OrgDuration); ok {
		val = d.ToString()
	}
	tbl.SetValRef(tgt, fmt.Sprintf(format, val))
}

func BuildParameters(ofile *common.OrgFile, sec *org.Section, tbl *org.Table) *map[string]interface{} {
	parameters := make(map[string]interface{}, 8)
	parameters["section"] = sec
	// Define these so the language parser automatically translates these to a t/f value
	// Makes it easier to work with these values with the sublime version.
	parameters["True"] = true
	parameters["False"] = false
	parameters["t"] = true
	parameters["f"] = false
	parameters["pi"] = 3.1415926535897932384626433832795
	GetConstants(&parameters, ofile, sec)
	return &parameters
}

func ProcessTableParameters(parameters *map[string]interface{}, tbl *org.Table, calcState *CalcState) {
	// Table has at least one parameters row
	if tbl.Params != nil {
		for k, v := range tbl.Params {
			// It is important we can interpret the value of the parameter through the lens of the
			// calc state parameters.
			name := strings.TrimSpace(k)
			if len(name) > 0 {
				if name[0] != '$' {
					name = "$" + name
				}
				(*parameters)[name] = calcState.ProcessCellVal(v)
			}
		}
	}
}

func ReplaceRowColLineNums(expr string, tgt *org.RowColRef) string {
	// First we have to replace the row col markers in our expressions
	expr = string(RE_ROW_TOKEN.ReplaceAll([]byte(expr), []byte(fmt.Sprintf("%d", tgt.Row))))
	expr = string(RE_COL_TOKEN.ReplaceAll([]byte(expr), []byte(fmt.Sprintf("%d", tgt.Col))))
	return expr
}

func ReplaceRemoteRanges(expr string) (string, error) {
	// We need to handle remote ranges
	// Next we handle standard row markers
	// Process REMOTE() function calls
	ms := common.ReMatch(RE_REMOTE, expr)
	if len(ms) > 0 {
		for _, m := range ms {
			if rng, ok := m["range"]; ok && rng.Have() {
				var r1 *common.Match = nil
				var r2 *common.Match = nil
				var c1 *common.Match = nil
				var c2 *common.Match = nil
				sr1 := ""
				sr2 := ""
				sc1 := ""
				sc2 := ""
				ok1 := true
				ok2 := true
				r1, ok1 = m["row"]
				c1, ok2 = m["col"]
				if !ok1 && !ok2 {
					if r1, ok1 = m["rowonly"]; ok1 {
						c1 = nil
						r2 = r1
						c2 = nil
						sr1 = r1.Capture
						sr2 = r1.Capture
					} else {
						if c1, ok1 = m["colonly"]; ok1 {
							r1 = nil
							r2 = nil
							c2 = c1
							sc1 = c1.Capture
							sc2 = c1.Capture
						} else {
							return expr, fmt.Errorf("malromed range [%s]", m["0"].Capture)
						}
					}
				} else {

					if (ok1 && !ok2) || (ok2 && !ok1) {
						return expr, fmt.Errorf("malromed start rowcol in range [%s]", m["0"].Capture)
					} else {
						// Check for single or range
						r2, ok1 = m["row2"]
						c2, ok2 = m["col2"]
						if ok1 && ok2 {
							sr1 = r1.Capture
							sc1 = c1.Capture
							sr2 = r2.Capture
							sc2 = c2.Capture
						} else if ok1 || ok2 {
							return expr, fmt.Errorf("malromed end rowcol in range [%s]", m["0"].Capture)
						} else { //if !ok1 && !ok2 {
							r2 = nil
							c2 = nil
							sr1 = r1.Capture
							sc1 = c1.Capture
						}
					}
				}
				params := fmt.Sprintf("'%s','%s','%s','%s'", sr1, sc1, sr2, sc2)
				expr = common.ReplaceSection(expr, rng.Start, rng.End, params)
				if tblnm, ok := m["tblnm"]; ok {
					// TODO: Should we check if the capture is not a variable somehow?
					// TODO: Check if already surrounded with quotes?
					expr = common.ReplaceSection(expr, tblnm.Start, tblnm.End, "'"+tblnm.Capture+"'")
				}
			}
		}
	}
	return expr, nil
}

// Is this one of the advanced row types that we should skip.
// names, parameters etc.
func ShouldSkipAdvancedRow(adv string) bool {
	return adv == "!" || adv == "^" || adv == "_" || adv == "$"
}

func ReplaceAllNamedColsAndCells(expr string, tbl *org.Table) string {
	if tbl.ColNames != nil {
		for k, v := range tbl.ColNames {
			re := regexp.MustCompile(fmt.Sprintf(`[$]%s\b`, k))
			to := fmt.Sprintf(`$$%v`, v)
			expr = re.ReplaceAllString(expr, to)
		}
	}
	if tbl.CellNames != nil {
		for k, v := range tbl.CellNames {
			re := regexp.MustCompile(fmt.Sprintf(`[$]%s\b`, k))
			to := fmt.Sprintf("@%d$$%d", v.Row, v.Col)
			expr = re.ReplaceAllString(expr, to)
		}
	}
	return expr
}

func GetFormulaDetails(tbl *org.Table) *common.TableFormulaDetails {
	tbl.Formulas.Process(tbl)
	ret := &common.TableFormulaDetails{}
	for _, frml := range tbl.Formulas.Formulas {
		if frml == nil || frml.Expr == "" {
			//return fmt.Errorf("missing formula in slot %d", idx)
			return nil
		}
		tgts := []common.CellDimensions{}
		out := frml.Target.CreateIterator(tbl)
		for {
			tgt := out()
			if tgt == nil {
				break
			}
			// It is important we set this each iteration to ensure
			// relative ranges work off the current row/col
			tbl.Cur.Row = tgt.Row
			tbl.Cur.Col = tgt.Col

			// If the first column has a special row signifier we may need to skip it.
			// Things like name rows should not get written to by formulas
			// We also skip -1 as if there are separators we may have to high a row count
			// and the iterator may go over the end of our table.
			row, col := tbl.GetRealRowCol(tbl.Cur.Row, tbl.Cur.Col)
			if row == -1 || row >= len(tbl.Rows) || ShouldSkipAdvancedRow(tbl.Rows[row].IsAdvanced) {
				continue
			}
			if col >= len(tbl.Rows[0].Columns) {
				col = len(tbl.Rows[0].Columns) - 1
			}
			start := tbl.Rows[row].Columns[col].Pos
			end := tbl.Rows[row].Columns[col].EndPos
			tgts = append(tgts, common.CellDimensions{Start: start, End: end})
		}
		ret.Formulas = append(ret.Formulas, common.CellDimensions{Start: frml.Start, End: frml.End})
		ret.Targets = append(ret.Targets, tgts)
	}
	return ret
}

func FormulaDetailsAt(db plugs.ODb, t *common.PreciseTarget) (common.ResultTableDetailsMsg, error) {
	ofile, sec, table := db.GetFromPreciseTarget(t, org.TableNode)
	res := common.ResultTableDetailsMsg{Ok: false, Msg: "Unknown table exec error"}
	if table != nil {
		var tbl *org.Table
		var tok bool
		if tbl, tok = table.(*org.Table); tok {
			if tbl.Formulas == nil {
				res.Msg = fmt.Sprintf("Cannot execute table does not have a formula [%s]:[%s]", ofile.Filename, common.GetSectionTitle(sec))
				return res, nil
			}
		} else {
			res.Msg = fmt.Sprintf("did not find table, cannot execute")
			return res, nil
		}
		dets := GetFormulaDetails(tbl)
		res.Details = *dets
		res.Ok = true
		res.Pos = tbl.Pos
		res.End = tbl.GetEnd()
	} else {
		res.Msg = "Could not locate table at position specified"
	}
	return res, nil
}

func ExecuteFormula(db plugs.ODb, sec *org.Section, ofile *common.OrgFile, tbl *org.Table) error {
	if tbl.Formulas == nil {
		fmt.Printf("Table does not have any formulas, skipping")
		return nil
	}
	parameters := BuildParameters(ofile, sec, tbl)
	tbl.Formulas.Process(tbl)
	for idx, frml := range tbl.Formulas.Formulas {
		if frml == nil || frml.Expr == "" {
			return fmt.Errorf("missing formula in slot %d", idx)
		}
		fmt.Printf("[%d] TABLE =============================\n", idx)
		out := frml.Target.CreateIterator(tbl)
		calcState := &CalcState{}
		calcState.ProcessCalcSpecifiers(strings.TrimSpace(frml.Format))
		ProcessTableParameters(parameters, tbl, calcState)
		for {
			tgt := out()
			if tgt == nil {
				break
			}
			// It is important we set this each iteration to ensure
			// relative ranges work off the current row/col
			tbl.Cur.Row = tgt.Row
			tbl.Cur.Col = tgt.Col

			// If the first column has a special row signifier we may need to skip it.
			// Things like name rows should not get written to by formulas
			// We also skip -1 as if there are separators we may have to high a row count
			// and the iterator may go over the end of our table.
			row, _ := tbl.GetRealRowCol(tbl.Cur.Row, tbl.Cur.Row)
			if row == -1 || ShouldSkipAdvancedRow(tbl.Rows[row].IsAdvanced) {
				continue
			}

			oldexpr := frml.Expr
			// Sub in the $# markers first
			frml.Expr = ReplaceRowColLineNums(frml.Expr, tgt)
			// Replace remote(RANGE) functions with propper markers
			if rexpr, err := ReplaceRemoteRanges(frml.Expr); err == nil {
				frml.Expr = rexpr
			} else {
				return err
			}
			// Replace other ranges with RangeIters
			//fmt.Printf("XXXXXXXXXXXXXXXXXXXXXXx\n")
			//fmt.Printf("%v\n", ms)
			//fmt.Printf("XXXXXXXXXXXXXXXXXXXXXXx\n")
			frml.Expr = ReplaceAllNamedColsAndCells(frml.Expr, tbl)
			rngCnt := 0
			frml.Expr = string(RE_TARGET_A.ReplaceAllFunc([]byte(frml.Expr), func(in []byte) []byte {
				name := fmt.Sprintf("rng_%d", rngCnt)
				rngCnt += 1
				txt := string(in)
				r := &RangeIter{}
				r.Mode = calcState
				r.Tbl = tbl
				r.Form = org.FormulaTarget{Raw: txt}
				r.Form.Process(tbl)
				r.Reset()
				(*parameters)[name] = r
				return []byte(fmt.Sprintf("(%s)", name))
			}))
			// HACK: double quotes are not supported for this version of the parser so we swap them.
			frml.Expr = strings.Replace(frml.Expr, "\"", "'", -1)
			//frml.Expr = string(RE_TARGET_A.ReplaceAll(, []byte("makerange('$0')")))
			// HACK: Expression parser needs ability to read range values, we replace ranges with a call to makerange.
			// .     Eventually we might want to replace with processed range so we don't have to parse it every time.
			//       For now this gets us operational
			expr, err := ParseTableFormula(tbl, frml, sec, ofile)
			if err != nil {
				return fmt.Errorf("Failed parsing formula [%s][%d](%s)", frml.Keyword.Value, idx, frml.Expr)
			}

			//fmt.Printf("GOT SLOT: %d %d\n", tgt.Row, tgt.Col)
			result, err := expr.Expression.Evaluate(*parameters)
			eout := ""
			if err != nil {
				eout = " [" + err.Error() + "] "
			}

			//fmt.Printf("   RESULT: %s => %v %s ON: [%d,%d..%d,%d]\n     >> %s", frml.Expr, result, eout, frml.Target.Start.Row, frml.Target.Start.Col, frml.Target.End.Row, frml.Target.End.Col, tbl.Formulas.Keywords[0].Value)
			fmt.Printf("   RESULT: %s => %v %s ON: [%d,%d..%d,%d]\n", frml.Expr, result, eout, frml.Target.Start.Row, frml.Target.Start.Col, frml.Target.End.Row, frml.Target.End.Col)
			if result != nil && err == nil {
				switch r := result.(type) {
				case *org.RowColRef:
					result = tbl.GetValRef(r)
				case *RangeIter:
					result = r.Next()
				}

				if !(calcState.SkipHeader && TableHasHeader(tbl) && tgt.Row == 1) {
					calcState.SetCell(tbl, tgt, result)
				}
				//fmt.Printf("---> Setting value\n")
			} else {
				fmt.Printf("HAD ERR: %v\n", err)
				return fmt.Errorf("formula execution error: [%s]", err.Error())
			}
			frml.Expr = oldexpr
		}
	}

	return nil
}

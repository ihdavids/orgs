package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// What an editor sends to have a table worked out: the table as it reads in
// the buffer, which is not necessarily what is on disk.
type TableCalc struct {
	Filename string // where the table lives: its constants and properties come from here
	Line     int    // zero based row of the table's first line in that file
	Text     string // the table's lines and its #+TBLFM: lines
}

// Run the formulas of a table given as text and answer with the table as it
// reads afterwards. Nothing is written: the editor asking is the one holding
// the text, unsaved edits and all, and it puts the answer back itself so that
// a recalculation is one undo.
// The answer: the table, and the colours its formulas gave cells.
type TableCalcResult struct {
	Ok     bool
	Msg    string
	Colors []CellColor `json:",omitempty"`
}

func CalcTableText(req *TableCalc) (res TableCalcResult) {
	res = TableCalcResult{Ok: false, Msg: "no table in the text sent"}
	// A formula that trips the evaluator over is a message for the person who
	// wrote it, not a dropped connection.
	defer func() {
		if r := recover(); r != nil {
			res = TableCalcResult{Ok: false, Msg: fmt.Sprintf("table formula failed: %v", r)}
		}
	}()
	text := strings.TrimRight(req.Text, "\n") + "\n"
	doc := org.New().Silent().Parse(strings.NewReader(text), req.Filename)
	var tbl *org.Table
	for _, n := range doc.Nodes {
		if t, ok := n.(*org.Table); ok {
			tbl = t
			break
		}
	}
	if tbl == nil {
		return res
	}
	// The text has no heading above it, so its #+TBLFM: is not attached.
	attachOrphanFormulas(tbl, strings.Split(text, "\n"))
	if tbl.Formulas == nil {
		return TableCalcResult{Ok: false, Msg: "the table has no #+TBLFM: line"}
	}
	// Constants and $PROP_ values come from the file on disk, which is where
	// they were last saved. A file the server does not know stands on its own.
	ofile := GetDb().FindByFile(req.Filename)
	var sec *org.Section
	if ofile != nil && ofile.Doc != nil {
		sec = sectionAtRow(ofile, req.Line)
	} else {
		ofile = &common.OrgFile{Filename: req.Filename, Doc: doc}
	}
	colors, err := ExecuteFormulaColors(db, sec, ofile, tbl)
	if err != nil {
		return TableCalcResult{Ok: false, Msg: err.Error()}
	}
	tbl.RecomputeColumnInfos()
	w := org.NewOrgWriter()
	w.SetLineBreak()
	org.WriteNodes(w, tbl)
	return TableCalcResult{Ok: true, Msg: w.String(), Colors: colors}
}

/* SDOC: API
* POST /table/calc — Work Out A Table Sent As Text
	#+BEGIN_EXAMPLE
	{ "Filename": "/org/budget.org", "Line": 12, "Text": "| a | b |\n| 1 |   |\n#+TBLFM: $2=$1*2\n" }
	#+END_EXAMPLE

	Runs the =#+TBLFM:= formulas of the table in =Text= and answers with the
	table as it reads afterwards in =Msg= - the rows only, the formula lines
	are not repeated. Nothing is written to disk. =Filename= and =Line= say
	where the table lives, for the file's =#+CONSTANTS:= and the =$PROP_=
	values of the heading above it; =remote()= reads other tables from the
	database as usual.

	This is what an editor uses: it holds the text, unsaved changes included,
	and putting the answer back itself keeps a recalculation one undo.

	*Response:* ={"Ok": true, "Msg": "| a | b |\n...", "Colors": [{"Row": 2, "Col": 3, "Color": "red"}]}=.
	=Colors= are the cells formulas coloured with =highlight()=, =red()=,
	=gradient()= and the rest, as =@Row$Col=, for the editor to show.
EDOC */
func PostTableCalc(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req TableCalc
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, TableCalcResult{Ok: false, Msg: err.Error()})
		return
	}
	dynJson(w, CalcTableText(&req))
}

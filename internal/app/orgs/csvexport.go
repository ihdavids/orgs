package orgs

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// The columns a query's rows get when nobody names any: what a list of tasks
// is usually wanted for in a spreadsheet. A file's own view has its
// #+COLUMNS: line instead.
const queryCsvColumns = "%ITEM %TODO %PRIORITY %TAGS %SCHEDULED %DEADLINE %EFFORT %CLOCKSUM %FILE"

// CsvOptions says how a table becomes csv.
type CsvOptions struct {
	// The separator: ',' (the default), ';' (what Excel wants where the
	// decimal point is a comma) or '\t'.
	Sep rune
	// Leave off the header row.
	NoHeader bool
	// Start with a byte order mark, without which Excel reads utf-8 as
	// Windows-1252 and every accent comes out wrong.
	BOM bool
	// Add Level, Line and Hash columns (and File, for a file's view), so the
	// rows can be found again.
	Meta bool
	// Write what each heading has itself rather than the rolled-up value a
	// summed column shows on a parent.
	Own bool
}

// ColumnsCsv is a column view - a file's, with its rollups, or a query's -
// as csv text.
func ColumnsCsv(res common.ColumnsResult, o CsvOptions) []byte {
	buf := &bytes.Buffer{}
	if o.BOM {
		buf.WriteString("\ufeff")
	}
	w := csv.NewWriter(buf)
	if o.Sep != 0 {
		w.Comma = o.Sep
	}
	if !o.NoHeader {
		head := []string{}
		if o.Meta {
			head = append(head, "Level")
		}
		for _, s := range res.Spec {
			head = append(head, s.Title)
		}
		if o.Meta {
			head = append(head, "Line", "Hash")
			if res.File != "" {
				head = append(head, "File")
			}
		}
		w.Write(head)
	}
	for _, r := range res.Rows {
		row := []string{}
		if o.Meta {
			row = append(row, strconv.Itoa(r.Level))
		}
		for _, c := range r.Cells {
			v := c.Value
			if o.Own {
				v = c.Own
			}
			row = append(row, v)
		}
		if o.Meta {
			row = append(row, strconv.Itoa(r.LineNum+1), r.Hash)
			if res.File != "" {
				row = append(row, res.File)
			}
		}
		w.Write(row)
	}
	w.Flush()
	return buf.Bytes()
}

// QueryColumnView is a column view of the headings a query finds: one row
// each, in the order found, nothing summed (the rows are not a tree).
func QueryColumnView(query, asked string) (common.ColumnsResult, error) {
	res := common.ColumnsResult{}
	line := strings.TrimSpace(asked)
	res.From = "request"
	if line == "" {
		line, res.From = queryCsvColumns, "default"
	}
	res.Columns = line
	res.Spec = ParseColumnSpec(line)
	if len(res.Spec) == 0 {
		return res, fmt.Errorf("%q names no columns", line)
	}
	lines := map[string][]string{}
	q := &common.StringQuery{Query: query}
	err := queryEachSection(q, func(sec *org.Section, f *common.OrgFile) {
		if sec == nil || sec.Headline == nil || f == nil {
			return
		}
		ls, ok := lines[f.Filename]
		if !ok {
			ls = fileLines(f.Filename)
			lines[f.Filename] = ls
		}
		res.Rows = append(res.Rows, ownRow(f, sec, propsOfSection(ls, sec), res.Spec, len(sec.Children) > 0))
	})
	if err != nil {
		return res, err
	}
	res.Ok = true
	return res, nil
}

/* SDOC: API
* GET /csv — A Column View or a Query as CSV

	=GET /csv?file=project.org= answers with the file's column view as csv:
	the =#+COLUMNS:= line in force (or =columns==), one row per heading, and
	the rolled-up value on a parent for a summed column (=%EFFORT{:}=), as
	worg's Columns tab and =orgs cols= show it.

	=GET /csv?query=IsTodo()= answers with the headings a query finds instead,
	one row each and nothing summed, in the columns
	=%ITEM %TODO %PRIORITY %TAGS %SCHEDULED %DEADLINE %EFFORT %CLOCKSUM %FILE=
	unless =columns== names others. Filters (={{WorkTasks}}=) work as in
	=/search=.

	*Query Parameters:*
	| Parameter | Description                                                     |
	|-----------+-----------------------------------------------------------------|
	| =file=    | the org file whose column view to write                         |
	| =query=   | the query whose headings to write (instead of =file=)           |
	| =columns= | a =#+COLUMNS:= line to use                                      |
	| =sep=     | =comma= (default), =semicolon= or =tab=                         |
	| =header=  | =f= leaves off the header row                                   |
	| =bom=     | =t= starts with a byte order mark, for Excel                    |
	| =meta=    | =t= adds Level, Line and Hash (and File) columns                |
	| =own=     | =t= writes each heading's own value, not a parent's rollup      |

	*Response:* =text/csv=, named after the file (or =query.csv=); a problem
	is a plain-text 400 or 404.
EDOC */
func RequestCsv(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	o := CsvOptions{NoHeader: q.Get("header") == "f", BOM: q.Get("bom") == "t", Meta: q.Get("meta") == "t", Own: q.Get("own") == "t"}
	switch strings.ToLower(q.Get("sep")) {
	case "", "comma", ",":
	case "semicolon", ";":
		o.Sep = ';'
	case "tab", "\t", "tsv":
		o.Sep = '\t'
	default:
		http.Error(w, "sep is comma, semicolon or tab", http.StatusBadRequest)
		return
	}
	var res common.ColumnsResult
	var err error
	name := "query.csv"
	switch {
	case strings.TrimSpace(q.Get("file")) != "":
		file := q.Get("file")
		res, err = BuildColumnView(file, q.Get("columns"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		name = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + ".csv"
	case strings.TrimSpace(q.Get("query")) != "":
		res, err = QueryColumnView(q.Get("query"), q.Get("columns"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "say which file (file=) or which headings (query=)", http.StatusBadRequest)
		return
	}
	ctype := "text/csv; charset=utf-8"
	if o.Sep == '\t' {
		ctype = "text/tab-separated-values; charset=utf-8"
		name = strings.TrimSuffix(name, ".csv") + ".tsv"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Orgs-Rows", strconv.Itoa(len(res.Rows)))
	w.Header().Set("X-Orgs-Columns", res.Columns)
	w.Write(ColumnsCsv(res, o))
}

package orgs

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// The separator a delimited file uses, judged from its first line: the one of
// tab, semicolon, pipe and comma that occurs most. A comma loses ties, because
// a semicolon file (what a spreadsheet in much of Europe writes) is full of
// decimal commas.
func guessSeparator(text string) rune {
	first, _, _ := strings.Cut(text, "\n")
	best, n := ',', strings.Count(first, ",")
	for _, c := range []rune{'\t', ';', '|'} {
		if k := strings.Count(first, string(c)); k >= n && k > 0 {
			best, n = c, k
		}
	}
	return best
}

// csvRows reads delimited text into rows, quotes and all. Rows may be ragged;
// the table writer pads them.
func csvRows(text, sep string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\uFEFF")))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	switch sep {
	case "":
		r.Comma = guessSeparator(text)
	case `\t`, "tab":
		r.Comma = '\t'
	default:
		r.Comma = []rune(sep)[0]
	}
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		for i, c := range row {
			// Inside a cell a newline would end the row, and a bar the cell.
			row[i] = cellText(strings.ReplaceAll(strings.ReplaceAll(c, "\r", ""), "\n", " "))
		}
	}
	return rows, nil
}

// ImportTable is the whole of POST /table/import.
func ImportTable(req *common.TableImport) (common.ResultMsg, error) {
	res := common.ResultMsg{}
	rows, err := csvRows(req.Text, req.Separator)
	if err != nil {
		return res, fmt.Errorf("could not read that as delimited text: %v", err)
	}
	if len(rows) == 0 {
		return res, fmt.Errorf("there is nothing in it")
	}
	table := [][]string{}
	for i, row := range rows {
		table = append(table, row)
		if i == 0 && req.Header && len(rows) > 1 {
			table = append(table, nil)
		}
	}
	add := orgTableLines(table)
	if name := strings.TrimSpace(req.Name); name != "" {
		add = append([]string{"#+NAME: " + name}, add...)
	}

	var path string
	var lines []string
	at := -1
	if req.Hash != "" {
		filename, ls, from, to, err := headingBodyLines(req.Hash)
		if err != nil {
			return res, err
		}
		path, lines = filename, ls
		// After the last line with something on it, so the blank lines that
		// separate this heading from the next stay where they are.
		at = from - 1
		for i := from; i <= to && i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "" {
				at = i
			}
		}
	} else {
		f := GetDb().FindByFile(req.Filename)
		if f == nil || f.Doc == nil {
			return res, fmt.Errorf("no file called %q", req.Filename)
		}
		path = f.Doc.Path
		if lines, err = ganttFileLines(path); err != nil {
			return res, err
		}
		at = len(lines) - 1
		if req.AfterLine >= 0 && req.AfterLine < len(lines) {
			at = req.AfterLine
		}
		// A file that ends with blank lines takes the table after the last
		// line with something on it.
		for req.AfterLine < 0 && at >= 0 && strings.TrimSpace(lines[at]) == "" {
			at--
		}
	}
	// A table straight under a paragraph would join it.
	if at >= 0 && at < len(lines) && strings.TrimSpace(lines[at]) != "" && !headingLineRe.MatchString(lines[at]) {
		add = append([]string{""}, add...)
	}
	if at+1 < len(lines) && strings.TrimSpace(lines[at+1]) != "" {
		add = append(add, "")
	}
	lines = splice(lines, at+1, add)
	if err := writeLines(path, lines); err != nil {
		return res, err
	}
	res.Ok = true
	res.Msg = fmt.Sprintf("%d rows written at line %d of %s", len(rows), at+2, path)
	return res, nil
}

/* SDOC: API
* POST /table/import — Write Delimited Text As A Table

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "AfterLine": -1, "Text": "a,b\n1,2\n", "Header": true, "Name": "sales" }
	{ "Hash": "hOpOB7vIg6oiYz5sMVS=", "Text": "...", "Separator": ";" }
	#+END_EXAMPLE

	Reads csv, tsv or semicolon separated text (guessed from the first line
	when =Separator= is empty) and writes it as an aligned org table: under a
	heading by =Hash=, after line =AfterLine= (zero based), or at the end of
	the file. =Header= draws a rule under the first row; =Name= writes a
	=#+NAME:= above it. Only the lines added change.
EDOC */
func PostTableImport(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.TableImport
	req.AfterLine = -1
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.ResultMsg{Msg: err.Error()})
		return
	}
	res, err := ImportTable(&req)
	if err != nil {
		res = common.ResultMsg{Msg: err.Error()}
	}
	dynJson(w, res)
}

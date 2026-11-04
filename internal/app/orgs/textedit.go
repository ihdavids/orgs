package orgs

// Edits to text the client sends, answered with the new text and written
// nowhere: the endpoints `orgs lsp` uses, so an editor with unsaved changes
// gets the server's own line edits applied to its buffer. Each is the edit the
// file endpoint makes (POST /date/change, POST /babel/exec), run on the lines
// given rather than the lines on disk; see common/textedit.go.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

var textHeadRe = regexp.MustCompile(`^(\*+)\s`)

// textLines splits a buffer the way ganttFileLines splits a file, and says
// whether it ended with a newline so the answer can too.
func textLines(text string) ([]string, bool) {
	nl := strings.HasSuffix(text, "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n"), nl
}

func joinTextLines(lines []string, nl bool) string {
	s := strings.Join(lines, "\n")
	if nl {
		s += "\n"
	}
	return s
}

// ChangeDateInText is ChangeDate on a buffer: the heading is the one whose
// lines hold req.Line.
func ChangeDateInText(req *common.TextDateChange) common.TextEditResult {
	lines, nl := textLines(req.Text)
	if req.Line < 0 || req.Line >= len(lines) {
		return common.TextEditResult{Msg: fmt.Sprintf("line %d is past the end of the text", req.Line+1)}
	}
	from := -1
	lvl := 0
	for i := req.Line; i >= 0; i-- {
		if m := textHeadRe.FindStringSubmatch(lines[i]); m != nil {
			from, lvl = i, len(m[1])
			break
		}
	}
	if from < 0 {
		return common.TextEditResult{Msg: "that line is not under a heading"}
	}
	name := strings.ToUpper(strings.TrimSpace(req.Name))
	stamp := ""
	if strings.TrimSpace(req.Value) != "" {
		var err error
		if stamp, err = dateChangeStamp(name, req.Value); err != nil {
			return common.TextEditResult{Msg: err.Error()}
		}
	}
	before := strings.Join(lines, "\n")
	out, err := changeDateLines(append([]string{}, lines...), from, lvl, name, stamp)
	if err != nil {
		return common.TextEditResult{Msg: err.Error()}
	}
	if strings.Join(out, "\n") == before {
		return common.TextEditResult{Ok: true}
	}
	return common.TextEditResult{Ok: true, Changed: true, Text: joinTextLines(out, nl)}
}

// RunInText is ExecBabel on a buffer, for one item: what is at req.Line runs
// with the buffer's own blocks, tables and header arguments, and its result is
// placed in the buffer. Babel's settings apply as they do to a file: off
// unless babel.enable is set.
func RunInText(req *common.TextRun) common.TextEditResult {
	if req.Filename == "" {
		return common.TextEditResult{Msg: "which file is this? A block runs in its file's directory"}
	}
	lines, nl := textLines(req.Text)
	var pick *babelItem
	items := babelItems(lines)
	for i := range items {
		if req.Line >= items[i].row && req.Line <= items[i].end {
			pick = &items[i]
		}
	}
	if pick == nil {
		return common.TextEditResult{Msg: fmt.Sprintf("nothing to run at line %d: no source block, #+CALL: or inline code there", req.Line+1)}
	}
	doc := GetConfig().Parse(strings.NewReader(req.Text), req.Filename)
	f := &common.OrgFile{Filename: req.Filename, Doc: doc}
	ran, after, err := runItemLines(f, req.Filename, lines, *pick, true, true)
	res := common.TextEditResult{Ok: true, Ran: ran}
	if err != nil {
		res.Ok, res.Msg = false, err.Error()
	}
	for _, r := range ran {
		if !r.Result.Ok && res.Ok {
			res.Ok, res.Msg = false, r.Result.Msg
		}
	}
	if res.Ok {
		res.Msg = fmt.Sprintf("%d ran", len(ran))
	}
	if after != nil {
		res.Changed, res.Text = true, joinTextLines(after, nl)
	}
	return res
}

/* SDOC: API
* POST /lsp/date — Set A Date In Text The Client Holds

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Text": "* TODO Call\n", "Line": 0,
	  "Name": "SCHEDULED", "Value": "<2026-11-02 Mon>" }
	#+END_EXAMPLE

	=/date/change= on text instead of a file: the heading holding =Line= (zero
	based) of =Text= gets the date, and the answer is the whole text after the
	edit. Nothing is written. =Value= is a stamp or a bare date; empty clears it.
	For an editor with unsaved changes, which applies the answer to its buffer.

	*Response:* ={Ok, Msg, Changed, Text}=. =Changed= false means the text
	already said that, and =Text= is then empty.
EDOC */
func PostTextDate(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.TextDateChange
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.TextEditResult{Msg: err.Error()})
		return
	}
	dynJson(w, ChangeDateInText(&req))
}

/* SDOC: API
* POST /lsp/run — Run What Is At A Line Of Text The Client Holds

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Text": "...the whole buffer...", "Line": 41 }
	#+END_EXAMPLE

	=/babel/exec= on text instead of a file: the source block, =#+CALL:= or
	inline code at =Line= runs with the text's own named tables and blocks, in
	the directory of =Filename=, and its result is placed in the text as it
	would be in the file. Nothing is written; the answer is the whole text. Off
	unless =babel.enable= is set, as for =/code/run=.

	*Response:* ={Ok, Msg, Changed, Text, Ran}=, =Ran= as =/babel/exec= has it.
EDOC */
func PostTextRun(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.TextRun
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.TextEditResult{Msg: err.Error()})
		return
	}
	dynJson(w, RunInText(&req))
}

/* SDOC: API
* GET /lsp/runnable — Which Languages A Block May Run In

	#+BEGIN_EXAMPLE
	GET /lsp/runnable?lang=python&lang=dot
	{"python": "", "dot": "orgs does not know how to run \"dot\". ..."}
	#+END_EXAMPLE

	For each =lang=, empty when a block in it would run here, otherwise why
	not: babel off, a language it has no runner for, or one left out of
	=babel.languages=. An editor offers "run" only where a run would happen.
EDOC */
func RequestRunnable(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, lang := range r.URL.Query()["lang"] {
		why := ""
		if _, err := babelAllows(lang); err != nil {
			why = err.Error()
		}
		out[lang] = why
	}
	dynJson(w, out)
}

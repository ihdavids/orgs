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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/orgcheck"
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

// DblockInText is POST /dblocks/update on a buffer: the dynamic block whose
// #+BEGIN:..#+END: holds req.Line is written afresh, or every block when Line
// is negative. The text is parsed under Filename, so a block's own heading
// and a :scope file clock table read the buffer; a scope naming other files
// reads them as the server has them.
func DblockInText(req *common.TextRun) common.TextEditResult {
	if req.Filename == "" {
		return common.TextEditResult{Msg: "which file is this? A block's scope is read relative to its file"}
	}
	lines, nl := textLines(req.Text)
	pick := func(b dynBlock) bool { return req.Line < 0 || (req.Line >= b.Begin && req.Line <= b.End) }
	doc := GetConfig().Silent().Parse(strings.NewReader(req.Text), req.Filename)
	f := &common.OrgFile{Filename: req.Filename, Doc: doc}
	infos, out, changed := dynBlockSplice(f, lines, pick, time.Now())
	if len(infos) == 0 {
		if req.Line < 0 {
			return common.TextEditResult{Ok: true, Msg: "no dynamic blocks in this text"}
		}
		return common.TextEditResult{Msg: fmt.Sprintf("line %d is not in a dynamic block", req.Line+1)}
	}
	res := common.TextEditResult{Ok: true, Blocks: infos}
	for _, b := range infos {
		if !b.Ok && res.Ok {
			res.Ok, res.Msg = false, fmt.Sprintf("line %d: %s", b.Line+1, b.Msg)
		}
	}
	if res.Ok {
		res.Msg = fmt.Sprintf("%d block(s) refreshed", len(infos))
	}
	if changed {
		res.Changed, res.Text = true, joinTextLines(out, nl)
	}
	return res
}

// DiagnoseText is what `orgs lsp` publishes as its structural diagnostics,
// for a client with no language server: the rules are orgcheck's, the files
// and ids those of this server's database. Columns go out as UTF-16 code
// units, which is what a JavaScript client indexes a line by.
func DiagnoseText(req *common.TextDiagnose) []common.TextDiagnostic {
	lines, _ := textLines(req.Text)
	doc := GetConfig().Silent().Parse(strings.NewReader(req.Text), req.Filename)
	path := req.Filename
	if f := GetDb().FindByFile(req.Filename); f != nil && f.Doc != nil {
		path = f.Doc.Path
	}
	probs := orgcheck.Structural(path, lines, orgcheck.Headings(doc, lines), serverCheckEnv{})
	out := make([]common.TextDiagnostic, 0, len(probs))
	for _, p := range probs {
		l := strings.TrimSuffix(lines[p.Line], "\r")
		out = append(out, common.TextDiagnostic{Line: p.Line, From: utf16Col(l, p.From), To: utf16Col(l, p.To),
			Severity: p.Severity, Code: p.Code, Message: p.Message})
	}
	return out
}

// utf16Col is a byte column of line in UTF-16 code units.
func utf16Col(line string, col int) int {
	if col > len(line) {
		col = len(line)
	}
	n := 0
	for _, r := range line[:col] {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// serverCheckEnv answers orgcheck from the server's own disk and database.
// Only files inside the org directories are read for a ::search, so a link
// cannot be used to look into the rest of the machine.
type serverCheckEnv struct{}

func (serverCheckEnv) FeatureOn(name string) bool { return featureOn(name) }

func (serverCheckEnv) File(path string) (bool, bool) {
	if GetDb().FindByFile(path) != nil {
		return true, false
	}
	if _, err := os.Stat(path); err == nil {
		return true, false
	}
	// As in orgs lsp: a folder that is not here is probably on another
	// machine, so its file's absence means nothing.
	_, derr := os.Stat(filepath.Dir(path))
	return false, derr == nil
}

func (serverCheckEnv) LinesOf(path string) []string {
	if !insideOrgDirsOk(path) {
		return nil
	}
	lines, err := ganttFileLines(path)
	if err != nil {
		return nil
	}
	return lines
}

func (serverCheckEnv) Headings(path string, lines []string) []orgcheck.Heading {
	doc := GetConfig().Silent().Parse(strings.NewReader(strings.Join(lines, "\n")), path)
	return orgcheck.Headings(doc, lines)
}

func (serverCheckEnv) Id(id string) (bool, bool) {
	return GetDb().FindByAnyId(id) != nil, true
}

/* SDOC: API
* POST /lsp/dblock — Refresh Dynamic Blocks In Text The Client Holds

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Text": "...the whole buffer...", "Line": 12 }
	#+END_EXAMPLE

	=/dblocks/update= on text instead of a file: the dynamic block
	(=#+BEGIN: clocktable …= to =#+END:=) holding =Line= (zero based) is
	written afresh, or every block in the text when =Line= is negative.
	Nothing is written; the answer is the whole text. The text is read as the
	file =Filename=, so a block's heading and a =:scope file= table are the
	buffer's; a scope naming other files reads them from disk.

	*Response:* ={Ok, Msg, Changed, Text, Blocks}=, =Blocks= as =/dblocks=
	lists them, each with its own =Ok= and =Msg=.
EDOC */
func PostTextDblock(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.TextRun
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.TextEditResult{Msg: err.Error()})
		return
	}
	dynJson(w, DblockInText(&req))
}

/* SDOC: API
* POST /lsp/diagnose — What Is Wrong With Text The Client Holds

	#+BEGIN_EXAMPLE
	{ "Filename": "/path/notes.org", "Text": "...the whole buffer..." }
	[ { "Line": 4, "From": 0, "To": 14, "Severity": "error",
	    "Code": "unclosed-block", "Message": "#+BEGIN_SRC with no #+END_SRC" } ]
	#+END_EXAMPLE

	The structural checks =orgs lsp= publishes, for an editor with no language
	server: a block with no =#+END_= (an error), a drawer with no =:END:=, a
	link to nothing (no heading, =CUSTOM_ID=, target or =#+NAME:= in the text,
	no such file, nothing of that name in the file, no heading with that id),
	and with those features on, org-edna properties that will not run and
	callouts of no known type. =Line= is zero based; =From= and =To= are
	columns in UTF-16 code units, which is what a JavaScript string counts, not
	bytes. =Severity= is =error=, =warning= or =info=; =Code= names the check
	(=unclosed-block=, =unclosed-drawer=, =broken-link=, =edna=,
	=callout-type=). Nothing is written.
EDOC */
func PostTextDiagnose(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.TextDiagnose
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		dynJson(w, []common.TextDiagnostic{})
		return
	}
	dynJson(w, DiagnoseText(&req))
}

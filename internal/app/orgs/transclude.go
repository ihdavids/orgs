package orgs

// Transclusion: a `#+transclude:` line stands for the text it names, as
// org-transclusion has it.
//
//	#+transclude: [[id:2f1c...]]                    a heading and its subtree
//	#+transclude: [[file:notes.org::*Budget]]       a heading in another file
//	#+transclude: [[file:notes.org::#budget]]       by CUSTOM_ID
//	#+transclude: [[*Budget]]                       a heading in this file
//	#+transclude: [[file:snippet.org]] :lines 3-10  some lines of a file
//	#+transclude: [[file:main.go]] :src go          a file as a source block
//	#+transclude: [[id:...]] :level 2 :only-contents
//
// Expanded in memory on the way to an exporter (transcludeView, exportFile),
// never in the file: every export, worg's rendered view and the VS Code
// preview (which are exports) draw the text, and the file keeps the line.
// Off unless `transclusion: enable: true`; POST /transclusion turns it on
// and off and writes the setting into the yaml.
//
// A transcluded heading takes the level under the heading the line is in,
// unless :level says otherwise; :only-contents leaves out the heading line,
// its planning line and its property drawer. Transclusions inside transcluded
// text are expanded too, transclusion.depth deep, and a cycle is said rather
// than followed.
//
// Each top-level transclusion is bracketed in the html by two empty spans
// (class org-transclusion, data-tx = its index in GET /transclusions), which
// is where worg puts its edit button; an edit goes back to the rows of the
// source it came from through POST /transclusion/save, a line splice checked
// against the text it was made from (see Traps: line edits).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

var (
	transcludeRe   = regexp.MustCompile(`(?i)^(\s*)#\+transclude:\s*(\[\[(?:[^\]\\]|\\.)+\](?:\[[^\]]*\])?\]|\S+)\s*(.*)$`)
	txLinkRe       = regexp.MustCompile(`^\[\[((?:[^\]\\]|\\.)+)\](?:\[[^\]]*\])?\]$`)
	txHeadingRe    = regexp.MustCompile(`^(\*+)\s`)
	txLinesRe      = regexp.MustCompile(`^(\d*)-(\d*)$`)
	txBlockBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_`)
	txBlockEndRe   = regexp.MustCompile(`(?i)^\s*#\+end_`)
	txPlanningRe   = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	txDrawerRe     = regexp.MustCompile(`(?i)^\s*:properties:\s*$`)
	txDrawerEndRe  = regexp.MustCompile(`(?i)^\s*:end:\s*$`)
	txCookieRe     = regexp.MustCompile(`\s*\[\d*(?:/\d*|%)\]`)
	txTagsRe       = regexp.MustCompile(`\s+:[\p{L}\p{N}_@#%:]+:\s*$`)
)

func transclusionOn() bool {
	return Conf().Server != nil && Conf().Server.Transclusion.Enable
}

func transclusionDepth() int {
	if Conf().Server != nil && Conf().Server.Transclusion.Depth > 0 {
		return Conf().Server.Transclusion.Depth
	}
	return 3
}

// Transclusion is one #+transclude: line, and where its text comes from.
type Transclusion struct {
	// The row of the #+transclude: line, 0 based.
	Line int
	Link string
	Opts string
	// The source: a file and its rows [From, To], 0 based, inclusive. Text is
	// those rows as they were read, which is what an edit is checked against.
	File  string
	From  int
	To    int
	Text  string
	Title string
	// Set when the link names nothing; the export says so in its place.
	Error string
}

type txOpts struct {
	level        int
	onlyContents bool
	src          string
	linesFrom    int
	linesTo      int
}

func parseTxOpts(s string) txOpts {
	o := txOpts{}
	words := strings.Fields(s)
	for i := 0; i < len(words); i++ {
		switch strings.ToLower(words[i]) {
		case ":level":
			if i+1 < len(words) {
				o.level, _ = strconv.Atoi(words[i+1])
				i++
			}
		case ":only-contents":
			o.onlyContents = true
		case ":src":
			if i+1 < len(words) {
				o.src = words[i+1]
				i++
			}
		case ":lines":
			if i+1 < len(words) {
				if m := txLinesRe.FindStringSubmatch(words[i+1]); m != nil {
					o.linesFrom, _ = strconv.Atoi(m[1])
					o.linesTo, _ = strconv.Atoi(m[2])
				}
				i++
			}
		}
	}
	return o
}

func txLevel(l string) int {
	if m := txHeadingRe.FindStringSubmatch(l); m != nil {
		return len(m[1])
	}
	return 0
}

// subtreeRows is the last row of the subtree whose heading is on row.
func subtreeRows(lines []string, row int) int {
	lvl := txLevel(lines[row])
	end := len(lines) - 1
	for i := row + 1; i < len(lines); i++ {
		if n := txLevel(lines[i]); n > 0 && n <= lvl {
			end = i - 1
			break
		}
	}
	// Trailing blank lines are the gap before the next heading, not the text.
	for end > row && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	return end
}

func txTitle(l string) string {
	t := txHeadingRe.ReplaceAllString(l, "")
	return strings.TrimSpace(txTagsRe.ReplaceAllString(t, ""))
}

// findHeadingRow is the row of the heading a search names in a file:
// *Title or #custom-id.
func findHeadingRow(path string, lines []string, search string) int {
	f := GetDb().FindByFile(path)
	var doc *org.Document
	if f != nil && f.Doc != nil {
		doc = f.Doc
	} else {
		doc = GetConfig().Silent().Parse(strings.NewReader(strings.Join(lines, "\n")), path)
	}
	norm := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(txCookieRe.ReplaceAllString(s, "")), " "))
	}
	want := ""
	cid := ""
	if strings.HasPrefix(search, "#") {
		cid = search[1:]
	} else {
		want = norm(strings.TrimPrefix(search, "*"))
	}
	var walk func(secs []*org.Section) int
	walk = func(secs []*org.Section) int {
		for _, s := range secs {
			if h := s.Headline; h != nil {
				if cid != "" && h.Properties != nil {
					if v, ok := h.Properties.Get("CUSTOM_ID"); ok && v == cid {
						return h.Pos.Row
					}
				}
				if want != "" {
					title := ""
					for _, n := range h.Title {
						title += n.String()
					}
					if norm(title) == want {
						return h.Pos.Row
					}
				}
			}
			if r := walk(s.Children); r >= 0 {
				return r
			}
		}
		return -1
	}
	if doc == nil || doc.Outline.Section == nil {
		return -1
	}
	return walk(doc.Outline.Children)
}

// resolveTransclusion is where a link's text is: the file, its lines, and the
// rows of them it means.
func resolveTransclusion(link, from string) (path string, lines []string, a, b int, err error) {
	target := link
	if m := txLinkRe.FindStringSubmatch(link); m != nil {
		target = strings.ReplaceAll(m[1], `\]`, "]")
	}
	file, search := "", ""
	switch {
	case strings.HasPrefix(strings.ToLower(target), "id:"):
		id := strings.TrimSpace(target[3:])
		sec := GetDb().FindByAnyId(id)
		if sec == nil || sec.Headline == nil {
			return "", nil, 0, 0, fmt.Errorf("no heading has the id %s", id)
		}
		of := GetDb().FileFromSection(sec)
		if of == nil {
			return "", nil, 0, 0, fmt.Errorf("the heading with id %s is in no file", id)
		}
		path = of.Filename
		if lines, err = readFileLines(path); err != nil {
			return
		}
		row := sec.Headline.Pos.Row
		if row < 0 || row >= len(lines) || txLevel(lines[row]) == 0 {
			return "", nil, 0, 0, fmt.Errorf("the heading with id %s is not where the database has it; the file may have just changed", id)
		}
		return path, lines, row, subtreeRows(lines, row), nil
	case strings.HasPrefix(strings.ToLower(target), "file:"):
		file, search, _ = strings.Cut(target[5:], "::")
	case strings.HasPrefix(target, "./"), strings.HasPrefix(target, "../"), strings.HasPrefix(target, "/"), strings.HasPrefix(target, "~/"):
		file, search, _ = strings.Cut(target, "::")
	case strings.HasPrefix(target, "*"), strings.HasPrefix(target, "#"):
		file, search = from, target
	default:
		return "", nil, 0, 0, fmt.Errorf("%s: transclusion follows id:, file: and *Heading links", link)
	}
	if strings.HasPrefix(file, "~/") {
		if home, herr := os.UserHomeDir(); herr == nil {
			file = filepath.Join(home, file[2:])
		}
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(filepath.Dir(from), file)
	}
	path = filepath.Clean(file)
	// Only what the server serves: a transclusion must not be a way to read
	// any file on the machine into a page. Org files the database has, and
	// files under its org directories.
	if !txAllowed(path) {
		return "", nil, 0, 0, fmt.Errorf("%s is not under the org directories", file)
	}
	if lines, err = readFileLines(path); err != nil {
		return "", nil, 0, 0, fmt.Errorf("cannot read %s", file)
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if search != "" {
		row := findHeadingRow(path, lines, search)
		if row < 0 || row >= len(lines) {
			return "", nil, 0, 0, fmt.Errorf("%s has no heading %s", filepath.Base(path), search)
		}
		return path, lines, row, subtreeRows(lines, row), nil
	}
	// A whole file, without the #+ settings at its top: they are the file's
	// own title and options, not text.
	a = 0
	if strings.EqualFold(filepath.Ext(path), ".org") {
		for a < len(lines) && (strings.HasPrefix(strings.TrimSpace(lines[a]), "#+") || strings.TrimSpace(lines[a]) == "") {
			a++
		}
	}
	return path, lines, a, len(lines) - 1, nil
}

func txAllowed(path string) bool {
	if f := GetDb().FindByFile(path); f != nil && f.Filename == path {
		return true
	}
	if Conf().Server == nil {
		return false
	}
	for _, d := range Conf().Server.OrgDirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(abs, path); err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// expandTransclusions is lines with every #+transclude: replaced by what it
// names. The Transclusions are the top-level ones, in order; marks puts the
// spans worg finds them by around each.
func expandTransclusions(path string, lines []string, marks bool) ([]string, []Transclusion) {
	return expandTx(path, lines, transclusionDepth(), map[string]bool{}, marks)
}

func expandTx(path string, lines []string, depth int, seen map[string]bool, marks bool) ([]string, []Transclusion) {
	out := []string{}
	txs := []Transclusion{}
	level := 0
	inBlock := false
	for i, l := range lines {
		if inBlock {
			if txBlockEndRe.MatchString(l) {
				inBlock = false
			}
			out = append(out, l)
			continue
		}
		if txBlockBeginRe.MatchString(l) {
			inBlock = true
			out = append(out, l)
			continue
		}
		if n := txLevel(l); n > 0 {
			level = n
		}
		m := transcludeRe.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			continue
		}
		tx := Transclusion{Line: i, Link: m[2], Opts: m[3]}
		o := parseTxOpts(m[3])
		src, slines, a, b, err := resolveTransclusion(m[2], path)
		if err == nil && (o.linesFrom > 0 || o.linesTo > 0) {
			lo, hi := a, b
			if o.linesFrom > 0 {
				lo = a + o.linesFrom - 1
			}
			if o.linesTo > 0 {
				hi = a + o.linesTo - 1
			}
			if hi > b {
				hi = b
			}
			if lo > hi {
				err = fmt.Errorf("%s: :lines %d-%d is past the end", m[2], o.linesFrom, o.linesTo)
			}
			a, b = lo, hi
		}
		key := fmt.Sprintf("%s:%d", src, a)
		switch {
		case err != nil:
			tx.Error = err.Error()
		case seen[key]:
			tx.Error = m[2] + " transcludes itself"
		}
		if tx.Error != "" {
			txs = append(txs, tx)
			out = append(out, m[1]+": transclusion: "+tx.Error)
			continue
		}
		tx.File, tx.From, tx.To = src, a, b
		tx.Text = strings.Join(slines[a:b+1], "\n")
		if txLevel(slines[a]) > 0 {
			tx.Title = txTitle(slines[a])
		} else {
			tx.Title = filepath.Base(src)
		}
		body := shapeTx(append([]string{}, slines[a:b+1]...), o, level)
		if depth > 1 && o.src == "" {
			inner := map[string]bool{key: true}
			for k := range seen {
				inner[k] = true
			}
			body, _ = expandTx(src, body, depth-1, inner, false)
		}
		if marks {
			out = append(out, txMark(len(txs), "start", tx))
		}
		out = append(out, body...)
		if marks {
			out = append(out, txMark(len(txs), "end", tx))
		}
		txs = append(txs, tx)
	}
	return out, txs
}

func txMark(n int, edge string, tx Transclusion) string {
	esc := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	return fmt.Sprintf("#+BEGIN_EXPORT html\n<span class=\"org-transclusion\" data-tx=\"%d\" data-edge=\"%s\" data-src=\"%s\" data-title=\"%s\"></span>\n#+END_EXPORT",
		n, edge, esc.Replace(filepath.Base(tx.File)), esc.Replace(tx.Title))
}

// shapeTx fits the source's lines to where they are going: as a block for
// :src, without the heading line for :only-contents, at the level asked for.
func shapeTx(body []string, o txOpts, under int) []string {
	if o.src != "" {
		return append(append([]string{"#+BEGIN_SRC " + o.src}, body...), "#+END_SRC")
	}
	if o.onlyContents && len(body) > 0 && txLevel(body[0]) > 0 {
		body = body[1:]
		if len(body) > 0 && txPlanningRe.MatchString(body[0]) {
			body = body[1:]
		}
		if len(body) > 0 && txDrawerRe.MatchString(body[0]) {
			for len(body) > 0 && !txDrawerEndRe.MatchString(body[0]) {
				body = body[1:]
			}
			if len(body) > 0 {
				body = body[1:]
			}
		}
	}
	first := 0
	for _, l := range body {
		if n := txLevel(l); n > 0 {
			first = n
			break
		}
	}
	if first == 0 {
		return body
	}
	want := under + 1
	if o.level > 0 {
		want = o.level
	}
	shift := want - first
	if shift == 0 {
		return body
	}
	inBlock := false
	for i, l := range body {
		if inBlock {
			inBlock = !txBlockEndRe.MatchString(l)
			continue
		}
		if txBlockBeginRe.MatchString(l) {
			inBlock = true
			continue
		}
		if n := txLevel(l); n > 0 {
			k := n + shift
			if k < 1 {
				k = 1
			}
			body[i] = strings.Repeat("*", k) + l[n:]
		}
	}
	return body
}

// ---------------------------------------------------------------------------
// Exports
// ---------------------------------------------------------------------------

// hasTransclude is whether a file has a #+transclude: line, read cheaply.
func hasTransclude(text string) bool {
	return strings.Contains(strings.ToLower(text), "#+transclude:")
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /transclusion — Whether Transclusion Is On
* POST /transclusion — Turn It On Or Off

	#+BEGIN_EXAMPLE
	GET  /transclusion          {"Ok": true, "Enable": false, "Depth": 3}
	POST /transclusion          {"Enable": true}
	#+END_EXAMPLE

	The POST takes effect at once and is written into the server's yaml
	(=transclusion: enable:=), keeping the file as it was in
	=<file>.before-transclusion=. A server started without a yaml file turns
	it on for as long as it runs, and says so in =Msg=.
EDOC */
func RequestTransclusion(w http.ResponseWriter, r *http.Request) {
	type state struct {
		Ok     bool
		Msg    string `json:",omitempty"`
		Enable bool
		Depth  int
	}
	if Conf().Server == nil {
		dynJson(w, state{Msg: "no server settings"})
		return
	}
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Enable bool }
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, state{Msg: err.Error()})
			return
		}
		Conf().Server.Transclusion.Enable = req.Enable
		msg := ""
		file, _, err := editConfigFile(".before-transclusion", func(lines []string) ([]string, error) {
			out, err := setYamlPath(lines, []string{"server", "transclusion", "enable"}, strconv.FormatBool(req.Enable))
			if err != nil {
				return nil, err
			}
			// setYamlPath quotes what it writes, and "true" is a string, not
			// a bool, to the yaml reader.
			for i, l := range out {
				if strings.TrimSpace(l) == `enable: "`+strconv.FormatBool(req.Enable)+`"` {
					out[i] = strings.Replace(l, `"`+strconv.FormatBool(req.Enable)+`"`, strconv.FormatBool(req.Enable), 1)
				}
			}
			return out, nil
		})
		if err != nil {
			msg = fmt.Sprintf("on until the server restarts: could not write %s: %v", filepath.Base(file), err)
		}
		dynJson(w, state{Ok: true, Msg: msg, Enable: req.Enable, Depth: transclusionDepth()})
		return
	}
	dynJson(w, state{Ok: true, Enable: transclusionOn(), Depth: transclusionDepth()})
}

/* SDOC: API
* GET /transclusions — A File's Transclusions

	=?file=notes.org=. Every =#+transclude:= line of the file, in order (the
	order of the =data-tx= marks in its html), with the file and rows its text
	comes from and that text, which is what =/transclusion/save= checks an
	edit against. =Error= says why one names nothing.

	*Response:* ={Ok, Msg, Enable, Transclusions: [{Line, Link, Opts, File, From, To, Text, Title, Error}]}=
EDOC */
func RequestTransclusions(w http.ResponseWriter, r *http.Request) {
	type answer struct {
		Ok            bool
		Msg           string `json:",omitempty"`
		Enable        bool
		Transclusions []Transclusion
	}
	f := GetDb().FindByFile(r.URL.Query().Get("file"))
	if f == nil {
		dynJson(w, answer{Msg: "no such org file", Transclusions: []Transclusion{}})
		return
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		dynJson(w, answer{Msg: err.Error(), Transclusions: []Transclusion{}})
		return
	}
	_, txs := expandTransclusions(f.Filename, lines, false)
	dynJson(w, answer{Ok: true, Enable: transclusionOn(), Transclusions: txs})
}

// TransclusionSave is an edit to transcluded text, going back to its source.
type TransclusionSave struct {
	File string
	From int
	To   int
	// The rows as they were when the edit began, and what they become.
	Old string
	New string
}

// SaveTransclusion splices an edit into the source's rows, if they still say
// what the edit was made from.
func SaveTransclusion(req *TransclusionSave) common.ResultMsg {
	if !transclusionOn() {
		return common.ResultMsg{Msg: "transclusion is off"}
	}
	f := GetDb().FindByFile(req.File)
	if f == nil || f.Filename != filepath.Clean(req.File) {
		return common.ResultMsg{Msg: "that is not an org file this server has"}
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		return common.ResultMsg{Msg: err.Error()}
	}
	nl := len(lines) > 0 && lines[len(lines)-1] == ""
	if nl {
		lines = lines[:len(lines)-1]
	}
	if req.From < 0 || req.To < req.From || req.To >= len(lines) ||
		strings.Join(lines[req.From:req.To+1], "\n") != req.Old {
		return common.ResultMsg{Msg: filepath.Base(f.Filename) + " has changed since this was read: reload and edit again"}
	}
	repl := strings.Split(strings.TrimRight(req.New, "\n"), "\n")
	out := append(append(append([]string{}, lines[:req.From]...), repl...), lines[req.To+1:]...)
	if err := writeLines(f.Filename, out); err != nil {
		return common.ResultMsg{Msg: err.Error()}
	}
	return common.ResultMsg{Ok: true, Msg: fmt.Sprintf("%s: %d lines", filepath.Base(f.Filename), len(repl))}
}

/* SDOC: API
* POST /transclusion/save — Write An Edit Back To Its Source

	#+BEGIN_EXAMPLE
	{ "File": "/org/projects.org", "From": 12, "To": 30, "Old": "...", "New": "..." }
	#+END_EXAMPLE

	Replaces rows =From=..=To= (0 based, as =/transclusions= gave them) of an
	org file the server has, only if they still read =Old=; otherwise it
	refuses and nothing is written. Only the rows change. Refused while
	transclusion is off.
EDOC */
func PostTransclusionSave(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req TransclusionSave
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.ResultMsg{Msg: err.Error()})
		return
	}
	dynJson(w, SaveTransclusion(&req))
}

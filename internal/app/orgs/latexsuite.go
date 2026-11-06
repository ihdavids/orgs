package orgs

// Latex Suite (features.latexSuite): quick maths typing in the
// manner of Emacs's cdlatex, for org maths. The snippets are org tables (the built-in set is
// internal/common/latexsnippets.org; the user's own file, latex-snippets.org
// under the first org directory, replaces or adds rows); the engine and the
// maths scanner are internal/common/latexsuite.go, twinned in worg and VS
// Code, which do the typing. This file serves the merged set, writes the
// user's file to start from, expands a trigger for editors with no engine of
// their own, and checks a file's maths.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

func init() {
	common.LatexSuiteOn = func() bool { return featureOn("latexSuite") }
	builtinKatex := common.LatexKatexBase
	common.LatexKatexBase = func() string {
		if k := strings.TrimSpace(latexSuiteSettings().Katex); k != "" {
			return k
		}
		return builtinKatex()
	}
}

func registerLatexSuiteRoutes(api *mux.Router) {
	api.HandleFunc("/latex/snippets", RequestLatexSnippets).Methods("GET")
	api.HandleFunc("/latex/snippets/init", RequestLatexSnippetsInit).Methods("POST")
	api.HandleFunc("/latex/expand", RequestLatexExpand).Methods("GET")
	api.HandleFunc("/latex/check", RequestLatexCheck).Methods("POST", "GET")
}

func latexSuiteSettings() common.LatexSuiteSettings {
	if s := Conf().Server; s != nil {
		return s.LatexSuite
	}
	return common.LatexSuiteSettings{}
}

// latexSnippetsFile is where the user's snippets are, relative to the first
// org directory unless absolute.
func latexSnippetsFile() string {
	name := strings.TrimSpace(latexSuiteSettings().Snippets)
	if name == "" {
		name = "latex-snippets.org"
	}
	if strings.HasPrefix(name, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			name = filepath.Join(home, name[2:])
		}
	}
	if !filepath.IsAbs(name) {
		if dirs := orgDirsOf(); len(dirs) > 0 {
			root, _ := filepath.Abs(dirs[0])
			name = filepath.Join(root, name)
		}
	}
	return filepath.Clean(name)
}

var latexUserCache struct {
	sync.Mutex
	file  string
	mod   time.Time
	size  int64
	snips []common.LatexSnippet
	probs []common.LatexTableProblem
}

// latexUserSnippets is the user's file read, again only when it changed.
func latexUserSnippets() (string, bool, []common.LatexSnippet, []common.LatexTableProblem) {
	file := latexSnippetsFile()
	st, err := os.Stat(file)
	if err != nil {
		return file, false, nil, nil
	}
	c := &latexUserCache
	c.Lock()
	defer c.Unlock()
	if c.file == file && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return file, true, c.snips, c.probs
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return file, false, nil, []common.LatexTableProblem{{File: file, Msg: err.Error()}}
	}
	c.snips, c.probs = common.ParseLatexSnippets(string(data), file)
	c.file, c.mod, c.size = file, st.ModTime(), st.Size()
	return file, true, c.snips, c.probs
}

// latexMergedSnippets is the set every client types with.
func latexMergedSnippets() []common.LatexSnippet {
	_, _, user, _ := latexUserSnippets()
	builtin := common.LatexBuiltinSnippets()
	if latexSuiteSettings().NoBuiltin {
		builtin = nil
	}
	return common.MergeLatexSnippets(user, builtin)
}

/* SDOC: API
* GET /latex/snippets — The Maths Snippets

	#+BEGIN_EXAMPLE
	GET /latex/snippets
	-> {"Ok": true, "File": "/notes/latex-snippets.org", "Exists": true,
	    "Snippets": [{"Trigger": "mk", "Replacement": "\\( $0 \\)", "Options": "twA",
	                  "Description": "inline maths", "Group": "Into and out of maths"}, ...],
	    "Problems": [{"File": "...", "Line": 12, "Msg": "trigger ([a: missing closing )"}]}
	GET /latex/snippets?builtin=1     the built-in set alone
	#+END_EXAMPLE

	The snippets worg, VS Code and =orgs latex expand= type with: the rows of
	every table in =latexSuite.snippets= (=latex-snippets.org=) whose header
	has Trigger and Replacement columns, then each built-in row the file does
	not replace (same trigger and modes; an empty replacement takes one away).
	A row of yours carries =File= and =Line=. Needs =features: latexSuite:= on.
EDOC */
func RequestLatexSnippets(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "latexSuite") {
		return
	}
	if r.URL.Query().Get("builtin") != "" {
		dynJson(w, map[string]interface{}{"Ok": true, "Snippets": common.LatexBuiltinSnippets()})
		return
	}
	file, exists, _, probs := latexUserSnippets()
	if probs == nil {
		probs = []common.LatexTableProblem{}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "File": file, "Exists": exists,
		"Snippets": latexMergedSnippets(), "Problems": probs})
}

/* SDOC: API
* POST /latex/snippets/init — Start Your Own Snippets File

	#+BEGIN_EXAMPLE
	POST /latex/snippets/init {}
	-> {"Ok": true, "File": "/notes/latex-snippets.org", "Msg": "wrote /notes/latex-snippets.org"}
	#+END_EXAMPLE

	Writes =latexSuite.snippets= with a short table to start from (a row
	that replaces a built-in one, one that adds a snippet, one that takes a
	built-in one away), when it does not exist yet; an existing file is never
	touched (=Ok= false). =orgs latex snippets -builtin= lists the rest.
EDOC */
func RequestLatexSnippetsInit(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "latexSuite") {
		return
	}
	file := latexSnippetsFile()
	if _, err := os.Stat(file); err == nil {
		dynJson(w, map[string]interface{}{"Ok": false, "File": file, "Msg": file + " is there already"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "File": file, "Msg": err.Error()})
		return
	}
	text := latexStarter
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "File": file, "Msg": err.Error()})
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "File": file, "Msg": "wrote " + file})
}

// LatexExpandAnswer is what /latex/expand says.
type LatexExpandAnswer struct {
	Ok      bool
	Msg     string `json:",omitempty"`
	Mode    string
	Found   bool
	Snippet *common.LatexSnippet `json:",omitempty"`
	Remove  int                  // bytes before the cursor the trigger took
	Body    string               // in snippet syntax: $1, ${1:x}, $0
	Text    string               // the body with its tab stops taken out
	Stops   []common.LatexStop   // in bytes of Text
}

// latexExpandText is the expansion at the end of text: as-you-type first,
// then TAB's, then the fraction of what is before a /.
func latexExpandText(snips []common.LatexSnippet, text, mode string, tab bool, fraction bool) LatexExpandAnswer {
	if mode == "" {
		mode = common.LatexModeAt(text, len(text)).Mode
	}
	a := LatexExpandAnswer{Ok: true, Mode: mode}
	e := common.LatexExpand(snips, text, mode, true)
	if e == nil && tab {
		e = common.LatexExpand(snips, text, mode, false)
	}
	if e == nil && fraction && mode != "text" {
		e = common.LatexAutoFraction(text)
	}
	if e == nil {
		return a
	}
	p := common.LatexPlace(e.Body)
	s := e.Snippet
	a.Found, a.Snippet, a.Remove, a.Body, a.Text, a.Stops = true, &s, e.Remove, e.Body, p.Text, p.Stops
	return a
}

/* SDOC: API
* GET /latex/expand — What A Trigger Becomes

	#+BEGIN_EXAMPLE
	GET /latex/expand?text=x+%2B+%40a&mode=inline
	-> {"Ok": true, "Mode": "inline", "Found": true, "Snippet": {"Trigger": "@a", ...},
	    "Remove": 2, "Body": "\\alpha", "Text": "\\alpha", "Stops": [{"Index": 0, "From": 6, "To": 6}]}
	GET /latex/expand?text=see+\(+x1        mode read from the text: inside \( it is inline
	#+END_EXAMPLE

	For an editor with no snippet engine of its own: =text= is what is
	before the cursor, ending with what was just typed. =mode= is =text=,
	=inline= or =display=; left out, it is worked out from the delimiters in
	=text=. An as-you-type snippet is tried first, then (unless =tab=0=) one
	TAB expands, then (unless =fraction=0=) =a/= as =\frac{a}{}=. Replace
	the =Remove= bytes before the cursor with =Text= and visit =Stops= in
	order (or hand =Body= to your editor's snippets).
EDOC */
func RequestLatexExpand(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "latexSuite") {
		return
	}
	q := r.URL.Query()
	dynJson(w, latexExpandText(latexMergedSnippets(), q.Get("text"), q.Get("mode"), q.Get("tab") != "0", q.Get("fraction") != "0"))
}

/* SDOC: API
* POST /latex/check — Problems In A File's Maths

	#+BEGIN_EXAMPLE
	POST /latex/check {"File": "notes.org"}
	POST /latex/check {"File": "notes.org", "Text": "...the buffer as it is..."}
	GET  /latex/check?file=notes.org
	-> {"Ok": true, "File": "/notes/notes.org", "Problems": [
	     {"Line": 12, "Col": 9, "Severity": "error", "Rule": "latex-brace", "Msg": "a { that is never closed"},
	     {"Line": 20, "Col": 4, "Severity": "warning", "Rule": "latex-env", "Msg": "KaTeX and MathJax do not know \\begin{foo}"}]}
	#+END_EXAMPLE

	Every piece of maths in the file (=\( \)=, =\[ \]=, =$ $=, =$$ $$=,
	=\begin{equation}= and the other maths environments; not in source or
	example blocks): braces that do not balance, =\left= with no =\right=,
	environments that do not end or end another, environments KaTeX and
	MathJax do not draw, maths that never closes, and =$ x$=, which org reads
	as text. =Text= checks an unsaved buffer instead of the file on disk.
	=Col= counts characters from 1.
EDOC */
func RequestLatexCheck(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "latexSuite") {
		return
	}
	var req struct {
		File string
		Text *string
	}
	if r.Method == "POST" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			dynJson(w, map[string]interface{}{"Ok": false, "Msg": "bad request: " + err.Error()})
			return
		}
	} else {
		req.File = r.URL.Query().Get("file")
	}
	if req.Text != nil {
		dynJson(w, map[string]interface{}{"Ok": true, "File": req.File, "Problems": common.LatexCheck(*req.Text)})
		return
	}
	if req.File == "" {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": "which file? (File)"})
		return
	}
	name := req.File
	if f := GetDb().FindByFile(req.File); f != nil {
		name = f.Filename
	} else if !filepath.IsAbs(name) {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": "no org file called " + req.File})
		return
	}
	data, err := os.ReadFile(name)
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "File": name, "Problems": common.LatexCheck(string(data))})
}

// latexStarter is what /latex/snippets/init writes.
const latexStarter = `#+TITLE: My latex snippets

Maths snippets for orgs's Latex Suite (=features: latexSuite:=). Every table
here with Trigger and Replacement columns is read; a row with the same trigger
and modes as a built-in one replaces it, and an empty replacement turns it
off. =orgs latex snippets -builtin= lists the built-in ones.

- Options :: =t= text, =m= maths, =n= inline maths, =M= display maths; =A= as
  you type (otherwise on TAB); =r= a pattern (=[[0]]= is its first group); =w=
  not straight after a letter; =v= wraps the selection (=${VISUAL}=).
- Replacement :: =$1=, =$2=... where TAB goes next, =${1:x}= with =x= in it,
  =$0= where the cursor ends. In a cell =\vert= is a =|= and =\n= a new line.

* Mine
| Trigger | Replacement          | Options | Description              |
|---------+----------------------+---------+--------------------------|
| sr      | ^{2}                 | mA      | squared (as built in)    |
| ip      | \langle $1, $2 \rangle$0 | mA | an inner product         |
| ooo     |                      | mA      | (no ∞ snippet)           |
`

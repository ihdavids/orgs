package orgs

// toc-org and org-make-toc: the endpoints, the `#+BEGIN: toc` dynamic block
// and the update on save. The TOC itself is worked out in toc.go, from the
// file's lines and nothing else. Only with features.toc on (features.go).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

func registerTocRoutes(api *mux.Router) {
	api.HandleFunc("/toc", RequestToc).Methods("GET")
	api.HandleFunc("/toc/update", PostTocUpdate).Methods("POST")
	api.HandleFunc("/toc/insert", PostTocInsert).Methods("POST")
	api.HandleFunc("/toc/text", PostTocText).Methods("POST")
}

// One file's TOCs.
type TocFile struct {
	File  string
	Ok    bool
	Msg   string `json:",omitempty"`
	Sites []TocSite
	Stale bool // some TOC in it is not what the outline says
	Wrote bool
	Row   int `json:",omitempty"` // an insert: the row of the TOC's headline
}

// The todo keywords a file's headlines may start with, as the server reads them.
func tocExtraKeywords(f *common.OrgFile) []string {
	if f == nil {
		return nil
	}
	done, active := keywordSets(f)
	out := []string{}
	for k := range done {
		out = append(out, k)
	}
	for k := range active {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func tocFileOf(name string) (*common.OrgFile, string) {
	f := GetDb().FindByFile(name)
	if f == nil {
		return nil, fmt.Sprintf("no org file called %q", name)
	}
	return f, ""
}

// tocUpdate brings every TOC in a file up to date, writing only when one
// changed (write false: says what would change and writes nothing).
func tocUpdate(name string, write bool) TocFile {
	f, msg := tocFileOf(name)
	if f == nil {
		return TocFile{File: name, Msg: msg, Sites: []TocSite{}}
	}
	res := TocFile{File: f.Filename, Sites: []TocSite{}}
	lines := fileLines(f.Filename)
	if lines == nil {
		res.Msg = "could not read " + f.Filename
		return res
	}
	out, sites, changed := ApplyTocs(lines, tocExtraKeywords(f))
	res.Sites, res.Stale, res.Ok = sites, changed, true
	if changed && write {
		if err := writeLines(f.Filename, out); err != nil {
			res.Ok, res.Msg = false, err.Error()
			return res
		}
		res.Wrote = true
	}
	return res
}

// ---------------------------------------------------------------------------
// The update on save
// ---------------------------------------------------------------------------

var tocTimers = struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}{timers: map[string]*time.Timer{}}

// autoTocAfterLoad is called from LoadFile for every re-read of a file (a
// save; a server starting is not one), as auto-tangle is: debounced, then
// the file's TOCs are brought up to date. It cannot loop: its own write is a
// re-read too, and that run finds nothing to change and writes nothing.
func autoTocAfterLoad(f *common.OrgFile) {
	if f == nil || !featureOn("toc") {
		return
	}
	name := f.Filename
	tocTimers.mu.Lock()
	defer tocTimers.mu.Unlock()
	if t, ok := tocTimers.timers[name]; ok {
		t.Stop()
	}
	tocTimers.timers[name] = time.AfterFunc(400*time.Millisecond, func() {
		tocTimers.mu.Lock()
		delete(tocTimers.timers, name)
		tocTimers.mu.Unlock()
		if !featureOn("toc") {
			return
		}
		if res := tocUpdate(name, true); res.Wrote {
			fmt.Fprintf(os.Stderr, "TOC %s: brought up to date\n", filepath.Base(name))
		} else if !res.Ok && res.Msg != "" {
			fmt.Fprintf(os.Stderr, "TOC %s: %s\n", filepath.Base(name), res.Msg)
		}
	})
}

// dynToc is the `#+BEGIN: toc` generator (dynblock.go's map).
func dynToc(c *dynCtx) ([]string, error) {
	if !featureOn("toc") {
		return nil, fmt.Errorf("tables of contents are turned off on this server: orgs features toc on")
	}
	return tocBlockLines(c.Lines, c.Block.Begin, c.Block.Params, tocExtraKeywords(c.File)), nil
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /toc — A File's Tables Of Contents, Worked Out

	#+BEGIN_EXAMPLE
	GET /toc?filename=notes.org
	-> {"File": "/notes/notes.org", "Ok": true, "Stale": true, "Wrote": false,
	    "Sites": [{"Kind": "toc-org", "Heading": "Contents", "Row": 3, "From": 4, "To": 9,
	               "Text": "- [[#about][About]]\n  - [[#use][Use]]\n", "Entries": 2, "Changed": true}]}
	#+END_EXAMPLE

	Every table of contents in the file - a heading tagged =:TOC:= (toc-org), a
	heading with a =:TOC:= property and a =:CONTENTS:= drawer (org-make-toc), a
	=#+BEGIN: toc= block - with what it should say. Rows count from 0; rows
	=From= to =To-1= are what =Text= replaces. Writes nothing.
EDOC */
func RequestToc(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "toc") {
		return
	}
	dynJson(w, tocUpdate(r.URL.Query().Get("filename"), false))
}

/* SDOC: API
* POST /toc/update — Bring Tables Of Contents Up To Date

	#+BEGIN_EXAMPLE
	POST /toc/update   {"Files": ["notes.org"]}          one file or several
	POST /toc/update   {}                                every file
	POST /toc/update   {"Files": [], "Check": true}      say which are stale, write nothing
	-> {"Ok": true, "Stale": 1, "Wrote": 1, "Files": [{"File": "...", "Stale": true, "Wrote": true, "Sites": [...]}]}
	#+END_EXAMPLE

	A file whose tables already say what its outline says is not written. With
	no files named, only files that have a table of contents are answered.
EDOC */
func PostTocUpdate(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "toc") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Filename string
		Files    []string
		Check    bool
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			dynJson(w, common.Result{Msg: err.Error()})
			return
		}
	}
	files := req.Files
	if req.Filename != "" {
		files = append(files, req.Filename)
	}
	all := len(files) == 0
	if all {
		files = GetDb().GetFiles()
	}
	type answer struct {
		Ok    bool
		Msg   string `json:",omitempty"`
		Stale int
		Wrote int
		Files []TocFile
	}
	out := answer{Ok: true, Files: []TocFile{}}
	for _, name := range files {
		res := tocUpdate(name, !req.Check)
		if all && len(res.Sites) == 0 {
			continue
		}
		if !res.Ok {
			out.Ok = false
			out.Msg = res.Msg
		}
		if res.Stale {
			out.Stale++
		}
		if res.Wrote {
			out.Wrote++
		}
		out.Files = append(out.Files, res)
	}
	dynJson(w, out)
}

/* SDOC: API
* POST /toc/insert — Add A Table Of Contents To A File

	#+BEGIN_EXAMPLE
	POST /toc/insert   {"Filename": "notes.org"}                                    toc-org
	POST /toc/insert   {"Filename": "notes.org", "Kind": "org-make-toc", "Row": 12}  at a heading
	-> {"File": "...", "Ok": true, "Wrote": true, "Row": 2, "Sites": [...]}
	#+END_EXAMPLE

	=toc-org= (the default) adds a =* Table of Contents :TOC:= heading above the
	first heading, unless the file has one. =org-make-toc= gives the heading
	whose headline is at =Row= (from 0; =-1= or none: a new =* Contents=
	heading above the first) a =:TOC: :include all :ignore this= property and a
	=:CONTENTS:= drawer. Either way the table is filled in at once.
EDOC */
func PostTocInsert(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "toc") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	req := struct {
		Filename string
		Kind     string
		Row      int
	}{Row: -1}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	f, msg := tocFileOf(req.Filename)
	if f == nil {
		dynJson(w, TocFile{File: req.Filename, Msg: msg, Sites: []TocSite{}})
		return
	}
	lines := fileLines(f.Filename)
	if lines == nil {
		lines = []string{}
	}
	extra := tocExtraKeywords(f)
	out, row := TocInsert(lines, req.Kind, req.Row, extra)
	res := TocFile{File: f.Filename, Ok: true, Row: row, Sites: TocSites(out, extra)}
	if strings.Join(out, "\n") != strings.Join(lines, "\n") {
		if err := writeLines(f.Filename, out); err != nil {
			res.Ok, res.Msg = false, err.Error()
		} else {
			res.Wrote = true
		}
	}
	dynJson(w, res)
}

/* SDOC: API
* POST /toc/text — Tables Of Contents In Text That Is Not Saved Yet

	#+BEGIN_EXAMPLE
	POST /toc/text   {"Text": "* Contents :TOC:\n* One\n", "Filename": "notes.org"}
	POST /toc/text   {"Text": "...", "Insert": "toc-org"}
	-> {"Ok": true, "Changed": true, "Text": "* Contents :TOC:\n- [[#one][One]]\n\n* One\n", "Row": 0, "Sites": [...]}
	#+END_EXAMPLE

	The same work as =/toc/update=, on text a client sends (an editor's
	buffer), answered and written nowhere. =Filename= is only for the file's
	todo keywords; =Insert= (=toc-org= or =org-make-toc=, with =Row=) adds a
	table first, as =/toc/insert= does.
EDOC */
func PostTocText(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "toc") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	req := struct {
		Text     string
		Filename string
		Insert   string
		Row      int
	}{Row: -1}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	var extra []string
	if req.Filename != "" {
		if f := GetDb().FindByFile(req.Filename); f != nil {
			extra = tocExtraKeywords(f)
		}
	}
	trail := strings.HasSuffix(req.Text, "\n")
	lines := strings.Split(strings.TrimSuffix(req.Text, "\n"), "\n")
	out, row := lines, -1
	if req.Insert != "" {
		out, row = TocInsert(lines, req.Insert, req.Row, extra)
	} else {
		out, _, _ = ApplyTocs(lines, extra)
	}
	text := strings.Join(out, "\n")
	if trail {
		text += "\n"
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Changed": text != req.Text, "Text": text,
		"Row": row, "Sites": TocSites(out, extra)})
}

package orgs

// The editor's writing mode (worg's WritingMode.ts and Writing*.tsx): what it
// needs kept on the server.
//
//   - Snapshots: a heading's text put aside before a rewrite, to compare with
//     or go back to. Kept beside the org file, in .snapshots/<name>.json, so
//     they travel with the file the way Scrivener's travel with a project,
//     and are seen from any browser.
//   - The writing log: how many words each file held at the start and end of
//     each day it was written in, per person, in the extensions file.
//   - Compile: the text worg assembled from the headings picked, run through
//     an exporter without being written anywhere.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Snapshot is one heading's text as it was.
type Snapshot struct {
	Id      string
	Heading string // the heading's title path, NUL separated, as worg's outline keys it
	Title   string // a name for it: what was about to change
	Time    string // RFC 3339
	Text    string // the heading and everything under it
	Words   int
}

// One writer at a time to any snapshot file.
var snapshotMu sync.Mutex

// A file's snapshots live beside it; the file must be one the server reads,
// or this is a way to write anywhere.
func snapshotPath(file string) (string, error) {
	if file == "" || GetDb().FindByFile(file) == nil {
		return "", fmt.Errorf("no org file called %s", file)
	}
	return filepath.Join(filepath.Dir(file), ".snapshots", filepath.Base(file)+".json"), nil
}

func readSnapshots(path string) ([]Snapshot, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Snapshot
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeSnapshots(path string, s []Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writingJson(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

/* SDOC: API
* GET /writing/snapshots — A File's Snapshots
	The snapshots taken of headings in one file (worg's writing mode), newest
	first. They are kept beside the file in =.snapshots/<name>.json=.

	*Query Parameters:* =file= the org file.

	*Response:* ={Ok, Msg, Snapshots: [{Id, Heading, Title, Time, Text, Words}]}=
EDOC */
func RequestSnapshots(w http.ResponseWriter, r *http.Request) {
	path, err := snapshotPath(r.URL.Query().Get("file"))
	if err != nil {
		writingJson(w, map[string]any{"Ok": false, "Msg": err.Error()})
		return
	}
	s, err := readSnapshots(path)
	if err != nil {
		writingJson(w, map[string]any{"Ok": false, "Msg": err.Error()})
		return
	}
	sort.SliceStable(s, func(i, j int) bool { return s[i].Time > s[j].Time })
	writingJson(w, map[string]any{"Ok": true, "Snapshots": s})
}

type snapshotReq struct {
	File     string
	Snapshot Snapshot
	Delete   string // an Id to take away instead
}

/* SDOC: API
* POST /writing/snapshots — Take or Delete a Snapshot
	Body ={File, Snapshot: {Heading, Title, Text, Words}}= keeps a new snapshot
	(the server gives it its Id and Time); ={File, Delete: id}= removes one.

	*Response:* ={Ok, Msg, Snapshot}=
EDOC */
func PostSnapshot(w http.ResponseWriter, r *http.Request) {
	var req snapshotReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	path, err := snapshotPath(req.File)
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	all, err := readSnapshots(path)
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if req.Delete != "" {
		kept := all[:0]
		for _, s := range all {
			if s.Id != req.Delete {
				kept = append(kept, s)
			}
		}
		if err := writeSnapshots(path, kept); err != nil {
			writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
		writingJson(w, common.ResultMsg{Ok: true})
		return
	}
	s := req.Snapshot
	id := make([]byte, 6)
	rand.Read(id)
	s.Id = hex.EncodeToString(id)
	s.Time = time.Now().Format(time.RFC3339)
	if strings.TrimSpace(s.Title) == "" {
		s.Title = "Snapshot"
	}
	all = append(all, s)
	if err := writeSnapshots(path, all); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	writingJson(w, map[string]any{"Ok": true, "Snapshot": s})
}

// ---------------------------------------------------------------------------
// The writing log
// ---------------------------------------------------------------------------

// WritingDay is a file's word count when it was first and last written in on
// one day; the difference is what was written that day.
type WritingDay struct {
	Start int `yaml:"start" json:"Start"`
	End   int `yaml:"end" json:"End"`
}

type writingLogReq struct {
	File  string
	Date  string // YYYY-MM-DD, the writer's own day rather than the server's
	Words int
}

/* SDOC: API
* POST /writing/log — Note Today's Word Count
	Body ={File, Date, Words}=: the file's word count now. The first count of a
	day is kept as its start and every later one as its end, so the day's words
	are End - Start. Kept per user in the extensions file.

	*Response:* ={Ok, Msg}=
EDOC */
func PostWritingLog(w http.ResponseWriter, r *http.Request) {
	var req writingLogReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if _, err := time.Parse("2006-01-02", req.Date); err != nil || req.File == "" {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "File and Date (YYYY-MM-DD) are needed"})
		return
	}
	ext := GetExtensions()
	if ext == nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "no extensions file to keep the log in"})
		return
	}
	ext.mu.Lock()
	u := ext.getUser(GetUsername(r))
	if u.WritingLog == nil {
		u.WritingLog = map[string]map[string]WritingDay{}
	}
	days := u.WritingLog[req.File]
	if days == nil {
		days = map[string]WritingDay{}
		u.WritingLog[req.File] = days
	}
	d, seen := days[req.Date]
	changed := !seen || d.End != req.Words
	if !seen {
		// A day's first count starts from where the file stood at the end of
		// the last day it was written in, so words written before midnight
		// and after are not lost between the two.
		d.Start = req.Words
		last := ""
		for k := range days {
			if k < req.Date && k > last {
				last = k
			}
		}
		if last != "" {
			d.Start = days[last].End
		}
	}
	d.End = req.Words
	days[req.Date] = d
	var err error
	if changed {
		err = ext.save()
	}
	ext.mu.Unlock()
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	writingJson(w, common.ResultMsg{Ok: true})
}

/* SDOC: API
* GET /writing/log — The Writing Log
	Every day this user wrote in each file: ={Ok, Files: {file: {date: {Start, End}}}}=.
	=file= narrows it to one file.
EDOC */
func RequestWritingLog(w http.ResponseWriter, r *http.Request) {
	ext := GetExtensions()
	out := map[string]map[string]WritingDay{}
	if ext != nil {
		ext.mu.RLock()
		if u, ok := ext.Users[GetUsername(r)]; ok {
			want := r.URL.Query().Get("file")
			for f, days := range u.WritingLog {
				if want != "" && f != want {
					continue
				}
				cp := map[string]WritingDay{}
				for k, v := range days {
					cp[k] = v
				}
				out[f] = cp
			}
		}
		ext.mu.RUnlock()
	}
	writingJson(w, map[string]any{"Ok": true, "Files": out})
}

// ---------------------------------------------------------------------------
// Compile
// ---------------------------------------------------------------------------

type compileReq struct {
	File  string // the file it came from: pictures, the preamble and the name
	Text  string // the org text to export
	To    string // docx, epub, or an exporter's name (html, markdown, ...)
	Style string
	Paper string
	Theme string
}

/* SDOC: API
* POST /compile — Export Text Put Together in the Editor
	Body ={File, Text, To, Style, Paper, Theme}=. =Text= is org (worg's writing
	mode builds it from the headings picked) and is exported as though it were
	=File=, without being written anywhere. =To= is =docx= or =epub= (written by
	orgs itself, see =GET /book=) or the name of a configured exporter, such as
	=html=.

	*Response:* the exported file's bytes, named for =File=.
EDOC */
func PostCompile(w http.ResponseWriter, r *http.Request) {
	var req compileReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path, err := FindFileInDb(req.File)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	doc := org.New().Silent().Parse(strings.NewReader(req.Text), path)
	doc.Path = path
	to := strings.ToLower(req.To)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch to {
	case "docx", "manuscript", "word", "epub", "epub3", "ebook":
		props := map[string]string{"style": req.Style, "paper": req.Paper, "theme": req.Theme}
		b, typ, ext, words, warns, err := bookBytes(doc, path, to, props)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Orgs-Words", fmt.Sprint(words))
		for _, warn := range warns {
			w.Header().Add("X-Orgs-Warning", warn)
		}
		w.Header().Set("Content-Type", typ)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base+"."+ext))
		w.Write(b)
		return
	}
	view := &subtreeDb{ODb: db, file: &common.OrgFile{Filename: path, Doc: doc}}
	props := map[string]string{"theme": req.Theme}
	res, _ := ExportToString(view, &common.ExportToFile{Name: to, Query: path, Props: props})
	if !res.Ok {
		http.Error(w, res.Msg, http.StatusBadRequest)
		return
	}
	ext := to
	switch to {
	case "markdown":
		ext = "md"
	case "latex":
		ext = "tex"
	case "typst":
		ext = "typ"
	}
	typ := "text/plain; charset=utf-8"
	if to == "html" {
		typ = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base+"."+ext))
	w.Write([]byte(res.Msg))
}

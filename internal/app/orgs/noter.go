package orgs

// org-noter: notes on a document kept under one heading, a "session", whose
// :NOTER_DOCUMENT: names the document (relative to the org file, as org-noter
// writes it) and whose child headings are the notes, each at its
// :NOTER_PAGE:. Read and written the way org-noter does, so a session made
// in Emacs opens here and the other way round:
//
//	* Attention is all you need
//	:PROPERTIES:
//	:NOTER_DOCUMENT: papers/attention.pdf
//	:NOTER_PAGE: 4
//	:END:
//	** Scaled dot product
//	:PROPERTIES:
//	:NOTER_PAGE: (4 . 0.31)
//	:END:
//	#+begin_quote
//	the text that was selected
//	#+end_quote
//	Why divide by √d.
//
// Only with features.noter on. The page on the session itself is where
// reading stopped (org-noter writes it when a session is closed). A note's
// location is a page, or (page . how far down it, 0 to 1), or org-noter's
// newer (page vertical . horizontal); the last is read and its horizontal
// half ignored.
//
// Only pdfs are served (GET /noter/pdf): the document can be anywhere the
// org file says, outside every org directory, so the server hands out the
// one kind of file the reader is for rather than any file a property names.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// NoterSession is a heading with a NOTER_DOCUMENT.
type NoterSession struct {
	Hash     string
	Headline string
	Filename string
	Line     int
	// As written in the property, and where a client fetches it from ("" when
	// it is not a pdf this server can read).
	Document string
	Url      string
	Exists   bool
	LastPage int
	Notes    int
}

// NoterNote is one note in a session.
type NoterNote struct {
	Hash     string
	Headline string
	Line     int
	Page     int
	// How far down the page, 0 to 1; -1 when the note is for the whole page.
	Y     float64
	Quote string
	Text  string
}

var noterPageRe = regexp.MustCompile(`^\(?\s*(\d+)\s*(?:\.\s*([0-9.]+)|([0-9.]+)\s*\.\s*[0-9.]+)?\s*\)?$`)

// parseNoterPage reads :NOTER_PAGE:. ok is false when it says nothing usable.
func parseNoterPage(v string) (page int, y float64, ok bool) {
	m := noterPageRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, -1, false
	}
	page, _ = strconv.Atoi(m[1])
	y = -1
	for _, s := range []string{m[2], m[3]} {
		if s != "" {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				y = f
			}
		}
	}
	return page, y, true
}

func noterPageText(page int, y float64) string {
	if y < 0 {
		return strconv.Itoa(page)
	}
	return fmt.Sprintf("(%d . %s)", page, strconv.FormatFloat(y, 'f', -1, 64))
}

// noterDocPath is where a session's document is on this machine.
func noterDocPath(orgFile, doc string) string {
	doc = strings.TrimSpace(doc)
	doc = strings.TrimPrefix(doc, "file:")
	if strings.HasPrefix(doc, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			doc = filepath.Join(home, doc[2:])
		}
	}
	if !filepath.IsAbs(doc) {
		doc = filepath.Join(filepath.Dir(orgFile), doc)
	}
	return filepath.Clean(doc)
}

func noterUrl(orgFile, doc string) string {
	if !strings.EqualFold(filepath.Ext(doc), ".pdf") {
		return ""
	}
	return "/noter/pdf?file=" + urlQueryEscape(orgFile) + "&doc=" + urlQueryEscape(doc)
}

func ownProp(sec *org.Section, name string) (string, bool) {
	if sec == nil || sec.Headline == nil || sec.Headline.Properties == nil {
		return "", false
	}
	return sec.Headline.Properties.Get(name)
}

func noterSessionOf(f *common.OrgFile, sec *org.Section) (NoterSession, bool) {
	doc, ok := ownProp(sec, "NOTER_DOCUMENT")
	if !ok || strings.TrimSpace(doc) == "" {
		return NoterSession{}, false
	}
	s := NoterSession{Hash: sec.Hash, Headline: strings.TrimSpace(headlineTitle(sec)), Filename: f.Filename,
		Line: sec.Headline.Pos.Row, Document: strings.TrimSpace(doc)}
	if _, err := os.Stat(noterDocPath(f.Filename, doc)); err == nil {
		s.Exists = true
		s.Url = noterUrl(f.Filename, s.Document)
	}
	if p, ok := ownProp(sec, "NOTER_PAGE"); ok {
		s.LastPage, _, _ = parseNoterPage(p)
	}
	var count func(*org.Section)
	count = func(x *org.Section) {
		for _, c := range x.Children {
			if _, ok := ownProp(c, "NOTER_PAGE"); ok {
				s.Notes++
			}
			count(c)
		}
	}
	count(sec)
	return s, true
}

// walkSections calls fn on every heading of every file the server reads.
func walkSections(fn func(f *common.OrgFile, sec *org.Section)) {
	for _, name := range GetDb().GetFiles() {
		f := GetDb().FindByFile(name)
		if f == nil || f.Doc == nil || f.Doc.Outline.Section == nil {
			continue
		}
		var walk func(*org.Section)
		walk = func(s *org.Section) {
			for _, c := range s.Children {
				if isRealHeading(c) {
					fn(f, c)
				}
				walk(c)
			}
		}
		walk(f.Doc.Outline.Section)
	}
}

// noterNotes are a session's notes in page order, as org-noter keeps them.
func noterNotes(f *common.OrgFile, sec *org.Section) []NoterNote {
	out := []NoterNote{}
	lines, _, _, ok := recordLines(f.Filename, sec)
	var walk func(*org.Section)
	walk = func(x *org.Section) {
		for _, c := range x.Children {
			if v, has := ownProp(c, "NOTER_PAGE"); has {
				page, y, _ := parseNoterPage(v)
				n := NoterNote{Hash: c.Hash, Headline: strings.TrimSpace(headlineTitle(c)), Line: c.Headline.Pos.Row, Page: page, Y: y}
				if ok {
					n.Quote, n.Text = noterBody(lines, c.Headline.Pos.Row)
				}
				out = append(out, n)
			}
			walk(c)
		}
	}
	walk(sec)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		return out[i].Y < out[j].Y
	})
	return out
}

// noterBody is a note's quote and its own words, without its drawers and
// planning.
func noterBody(lines []string, row int) (quote, text string) {
	if row < 0 || row >= len(lines) {
		return "", ""
	}
	end := ownLinesEnd(lines, row, len(lines)-1)
	inDrawer, inQuote := false, false
	q, t := []string{}, []string{}
	for i := row + 1; i <= end; i++ {
		l := lines[i]
		s := strings.TrimSpace(l)
		low := strings.ToLower(s)
		switch {
		case inDrawer:
			if strings.EqualFold(s, ":END:") {
				inDrawer = false
			}
		case drawerOpenRe.MatchString(l):
			inDrawer = true
		case planningRe.MatchString(l):
		case low == "#+begin_quote":
			inQuote = true
		case low == "#+end_quote":
			inQuote = false
		case inQuote:
			q = append(q, s)
		default:
			t = append(t, strings.TrimRight(l, " \t"))
		}
	}
	return strings.Join(q, "\n"), strings.TrimSpace(strings.Join(t, "\n"))
}

/* SDOC: API
* GET /noter/sessions — Every Document With Notes
* GET /noter/notes — One Session's Notes
* POST /noter/note — Add A Note At A Page
* POST /noter/page — Remember Where Reading Stopped
* POST /noter/start — Start Notes On A Document
* GET /noter/pdf — The Document

	#+BEGIN_EXAMPLE
	GET  /noter/sessions               {"Ok": true, "Sessions": [{Hash, Headline, Filename, Document, Url, Exists, LastPage, Notes}]}
	GET  /noter/notes?hash=...         {"Ok": true, "Session": {...}, "Notes": [{Hash, Headline, Page, Y, Quote, Text}]}
	POST /noter/note    {"Hash": session, "Page": 4, "Y": 0.31, "Title": "...", "Quote": "...", "Text": "..."}
	POST /noter/page    {"Hash": session, "Page": 12}
	POST /noter/start   {"Filename": "reading.org", "Document": "papers/x.pdf", "Title": "X"}
	GET  /noter/pdf?file=<org file>&doc=<NOTER_DOCUMENT>
	#+END_EXAMPLE

	org-noter's sessions: a heading with =:NOTER_DOCUMENT:=, and under it a
	heading per note with =:NOTER_PAGE:= (a page, or =(page . y)= with =y=
	from 0 at the top to 1 at the bottom). A new note goes in page order
	under the session; =Y= is optional. Needs =features: noter:=. The pdf is
	served only for a session that names it, and only a =.pdf=.
EDOC */
func RequestNoterSessions(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "noter") {
		return
	}
	out := []NoterSession{}
	walkSections(func(f *common.OrgFile, sec *org.Section) {
		if s, ok := noterSessionOf(f, sec); ok {
			out = append(out, s)
		}
	})
	dynJson(w, map[string]interface{}{"Ok": true, "Sessions": out})
}

func noterSessionFor(w http.ResponseWriter, hash string) (*common.OrgFile, *org.Section, NoterSession, bool) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		dynJson(w, common.Result{Msg: "no heading with that hash - it may have moved; refresh and try again"})
		return nil, nil, NoterSession{}, false
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		dynJson(w, common.Result{Msg: "could not find the file that heading is in"})
		return nil, nil, NoterSession{}, false
	}
	s, ok := noterSessionOf(f, sec)
	if !ok {
		dynJson(w, common.Result{Msg: fmt.Sprintf("%q has no NOTER_DOCUMENT", strings.TrimSpace(headlineTitle(sec)))})
		return nil, nil, NoterSession{}, false
	}
	return f, sec, s, true
}

func RequestNoterNotes(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "noter") {
		return
	}
	f, sec, s, ok := noterSessionFor(w, r.URL.Query().Get("hash"))
	if !ok {
		return
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Session": s, "Notes": noterNotes(f, sec)})
}

func PostNoterNote(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "noter") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Hash               string
		Page               int
		Y                  *float64
		Title, Quote, Text string
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	f, sec, _, ok := noterSessionFor(w, req.Hash)
	if !ok {
		return
	}
	if req.Page < 1 {
		dynJson(w, common.Result{Msg: "a note needs a page, counted from 1"})
		return
	}
	y := -1.0
	if req.Y != nil && *req.Y >= 0 && *req.Y <= 1 {
		y = float64(int(*req.Y*1000+0.5)) / 1000
	}
	title := strings.TrimSpace(strings.ReplaceAll(req.Title, "\n", " "))
	if title == "" {
		title = noterDefaultTitle(req.Quote, req.Page)
	}
	lines, err := insertNoterNote(f.Filename, sec, req.Page, y, title, req.Quote, req.Text)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	if err := writeLines(f.Filename, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true, Msg: fmt.Sprintf("note on page %d", req.Page)})
}

// org-noter's own default title is "Notes for page N"; a quote makes a
// better one.
func noterDefaultTitle(quote string, page int) string {
	q := strings.Join(strings.Fields(quote), " ")
	if q == "" {
		return fmt.Sprintf("Notes for page %d", page)
	}
	if r := []rune(q); len(r) > 60 {
		q = strings.TrimSpace(string(r[:57])) + "…"
	}
	return q
}

// insertNoterNote is the file's lines with a note added under the session,
// before the first note that comes after it on the page.
func insertNoterNote(filename string, sec *org.Section, page int, y float64, title, quote, text string) ([]string, error) {
	lines, from, end, ok := recordLines(filename, sec)
	if !ok {
		return nil, fmt.Errorf("could not read %s", filename)
	}
	lvl := sec.Headline.Lvl
	at := end + 1
	for i := from + 1; i <= end && i < len(lines); i++ {
		if headingLevel(lines[i]) != lvl+1 {
			continue
		}
		cend := subtreeEndRow(lines, i, lvl+1, i)
		props := propsFromLines(lines, i, cend)
		if v, has := props["NOTER_PAGE"]; has {
			p, py, _ := parseNoterPage(v)
			if p > page || (p == page && py > y) {
				at = i
				break
			}
		}
	}
	ind := headIndent(lines, from, ownLinesEnd(lines, from, end), lvl)
	add := []string{strings.Repeat("*", lvl+1) + " " + title, ind + ":PROPERTIES:", ind + ":NOTER_PAGE: " + noterPageText(page, y), ind + ":END:"}
	if q := strings.TrimSpace(quote); q != "" {
		add = append(add, ind+"#+begin_quote")
		for _, l := range strings.Split(q, "\n") {
			add = append(add, ind+strings.TrimSpace(l))
		}
		add = append(add, ind+"#+end_quote")
	}
	if t := strings.TrimRight(text, "\n \t"); strings.TrimSpace(t) != "" {
		for _, l := range strings.Split(t, "\n") {
			add = append(add, ind+strings.TrimRight(l, " \t"))
		}
	}
	return splice(lines, at, add), nil
}

func PostNoterPage(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "noter") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Hash string
		Page int
	}
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	f, sec, s, ok := noterSessionFor(w, req.Hash)
	if !ok {
		return
	}
	if req.Page < 1 || req.Page == s.LastPage {
		dynJson(w, common.Result{Ok: true})
		return
	}
	if err := setHeadingProperty(f, sec, "NOTER_PAGE", strconv.Itoa(req.Page)); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true})
}

func PostNoterStart(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "noter") {
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct{ Filename, Document, Title string }
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	f := GetDb().FindByFile(req.Filename)
	if f == nil {
		dynJson(w, common.Result{Msg: fmt.Sprintf("no org file %q", req.Filename)})
		return
	}
	doc := strings.TrimSpace(req.Document)
	if doc == "" {
		dynJson(w, common.Result{Msg: "which document?"})
		return
	}
	// Written relative to the org file when it is under the org file's
	// directory, which is how org-noter writes it.
	if filepath.IsAbs(doc) {
		// The org file's name may be relative (an orgDirs entry like ./notes),
		// and Rel will not relate a relative path to an absolute one.
		dir, _ := filepath.Abs(filepath.Dir(f.Filename))
		if rel, err := filepath.Rel(dir, doc); err == nil && !strings.HasPrefix(rel, "..") {
			doc = filepath.ToSlash(rel)
		}
	}
	if _, err := os.Stat(noterDocPath(f.Filename, doc)); err != nil {
		dynJson(w, common.Result{Msg: fmt.Sprintf("there is no %s", noterDocPath(f.Filename, doc))})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(doc), filepath.Ext(doc))
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	lines = append(lines, "* "+title, ":PROPERTIES:", ":NOTER_DOCUMENT: "+doc, ":END:")
	if err := writeLines(f.Filename, lines); err != nil {
		dynJson(w, common.Result{Msg: err.Error()})
		return
	}
	dynJson(w, common.Result{Ok: true, Msg: "started notes on " + doc})
}

func RequestNoterPdf(w http.ResponseWriter, r *http.Request) {
	if !featureOn("noter") {
		http.Error(w, "noter is turned off", http.StatusNotFound)
		return
	}
	file, doc := r.URL.Query().Get("file"), r.URL.Query().Get("doc")
	f := GetDb().FindByFile(file)
	if f == nil || !strings.EqualFold(filepath.Ext(doc), ".pdf") {
		http.Error(w, "no such document", http.StatusNotFound)
		return
	}
	// Only a document some session in that file actually names.
	named := false
	walkSections(func(of *common.OrgFile, sec *org.Section) {
		if of.Filename == f.Filename {
			if v, ok := ownProp(sec, "NOTER_DOCUMENT"); ok && strings.TrimSpace(v) == doc {
				named = true
			}
		}
	})
	if !named {
		http.Error(w, "no session names that document", http.StatusNotFound)
		return
	}
	path := noterDocPath(f.Filename, doc)
	fh, err := os.Open(path)
	if err != nil {
		http.Error(w, "no such document", http.StatusNotFound)
		return
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "no such document", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), fh)
}

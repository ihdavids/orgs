package orgs

// What a file asks to be shown as.
//
// Most org files are just org files, and the file view's two buttons - the
// exported page and the text - are the whole of it. Some files say what they
// are in their own header, and those get a button of their own: a character
// sheet (which the dnd module already answers for), a D&D book, or a file that
// names the html theme it is meant to be read in.
//
// A file written as a D&D book, rather than a character.
//
// `#+LATEX_CLASS: dndbook` is somebody writing an adventure, a bestiary or a
// gazetteer to be printed the way the books are - two columns, drop caps, stat
// blocks in boxes. The file view already offers a character sheet for the files
// that are characters; this is the same idea for the files that are books, and
// it needs the same two things: a way to ask which files those are, and a way
// to get at the rendered result.
//
// The pdf is the one that cannot come back as a string, so it has an endpoint
// of its own rather than going through /file/{type}: it is bytes, it takes
// pdflatex several seconds to make, and the same file asked for twice in a row
// should only be built once. Hence the cache, which is keyed on what the file
// was when it was built rather than on a timer - an org file that has not
// changed cannot have a different pdf, and one that has changed must never be
// served the old one.

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ihdavids/orgs/internal/common"
)

// `#+LATEX_CLASS: dndbook`, however it was spaced and whatever case it was
// written in - org keywords are case insensitive and people write them both
// ways.
var latexClassRe = regexp.MustCompile(`(?im)^\s*#\+LATEX_CLASS:\s*(\S+)\s*$`)
var orgTitleRe = regexp.MustCompile(`(?im)^\s*#\+TITLE:\s*(.+?)\s*$`)

// The book class this file asks to be printed as, or "" for a file that asks
// for nothing.
func latexClassOf(text string) string {
	m := latexClassRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m[1]))
}

func orgTitleOf(text string) string {
	m := orgTitleRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

/* SDOC: API
* GET /dnd/books — List Files Written as D&D Books

	Answers with every org file the server is watching that asks to be printed
	as a D&D book - that is, one carrying =#+LATEX_CLASS: dndbook=. This is the
	book-shaped sibling of =/dnd/characters=: a client showing a file can use it
	to know whether to offer the book views for that file.

	*Method:* =GET=

	*Query Parameters:* none

	*Response:* A JSON array, one entry per file.
	| Field      | Type   | Description                                    |
	|------------+--------+------------------------------------------------|
	| =filename= | string | The absolute path the server knows the file by |
	| =title=    | string | Its =#+TITLE:=, when it has one                |
	| =class=    | string | The latex class it asked for                   |

	#+BEGIN_SRC json
	[{"filename": "/org/dnd_pdf_example.org", "title": "The Sunken Tower", "class": "dndbook"}]
	#+END_SRC
EDOC */
func RequestDndBooks(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	out := []map[string]interface{}{}
	for _, fname := range GetDb().GetFiles() {
		data, err := os.ReadFile(fname)
		if err != nil {
			continue
		}
		// A cheap refusal before the regular expression, because this walks
		// every file in the database and most of them are not books.
		if !strings.Contains(strings.ToUpper(string(data)), "LATEX_CLASS") {
			continue
		}
		class := latexClassOf(string(data))
		if class != "dndbook" {
			continue
		}
		out = append(out, map[string]interface{}{
			"filename": fname,
			"title":    orgTitleOf(string(data)),
			"class":    class,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprintf("%v", out[i]["filename"]) < fmt.Sprintf("%v", out[j]["filename"])
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// `#+HTML_THEME: docs` - a file naming the stylesheet it is meant to be read
// in. Org keywords are case insensitive and people write them both ways.
var htmlThemeRe = regexp.MustCompile(`(?im)^\s*#\+HTML_THEME:\s*(\S+)\s*$`)

func htmlThemeOf(text string) string {
	m := htmlThemeRe.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

/* SDOC: API
* GET /files/themes — List Files That Name Their Own Html Theme

	Answers with every org file carrying =#+HTML_THEME:=, and the theme it asks
	for. A file that says how it wants to be read is asking for something the
	reader's own theme setting would otherwise override, so a client showing it
	can offer that reading as a view of its own.

	The theme is reported as the file wrote it, whether or not this server has a
	stylesheet by that name - =/html/themes= is the list of the ones it has, and
	answering with only those would silently swallow a typo.

	*Method:* =GET=

	*Query Parameters:* none

	*Response:* A JSON array, one entry per file.
	| Field      | Type   | Description                                    |
	|------------+--------+------------------------------------------------|
	| =filename= | string | The absolute path the server knows the file by |
	| =theme=    | string | The theme it asked for                         |
	| =title=    | string | Its =#+TITLE:=, when it has one                |

	#+BEGIN_SRC json
	[{"filename": "/org/docs.org", "theme": "docs", "title": "Orgs"}]
	#+END_SRC
EDOC */
func RequestFileThemes(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	out := []map[string]interface{}{}
	for _, fname := range GetDb().GetFiles() {
		data, err := os.ReadFile(fname)
		if err != nil {
			continue
		}
		// A cheap refusal before the regular expression: this walks every file
		// in the database and most of them name no theme.
		if !strings.Contains(strings.ToUpper(string(data)), "HTML_THEME") {
			continue
		}
		theme := htmlThemeOf(string(data))
		if theme == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"filename": fname,
			"theme":    theme,
			"title":    orgTitleOf(string(data)),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprintf("%v", out[i]["filename"]) < fmt.Sprintf("%v", out[j]["filename"])
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// One build at a time per file. Two tabs opening the same book would otherwise
// have pdflatex writing the same output twice at once, and the loser of that
// race serves half a file.
var pdfBuilding sync.Map

// Where a built pdf is kept, named for the file and the state it was in. A file
// that has not changed cannot have a different pdf; one that has changed gets a
// different name here and so is built again.
func pdfCachePath(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s|%d|%d", path, st.ModTime().UnixNano(), st.Size())
	sum := sha1.Sum([]byte(key))
	dir := filepath.Join(os.TempDir(), "orgs-pdf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%x.pdf", sum)), nil
}

/* SDOC: API
* GET /pdf — A File, Run Through LaTeX and Served as a PDF

	Exports one org file with the =pdf= exporter and answers with the pdf itself
	rather than with a path to it or a string. This is what the file view reads
	to show a =#+LATEX_CLASS: dndbook= file as the book it is asking to be.

	The result is cached against what the file was when it was built - its path,
	modification time and size - so opening the same book twice runs pdflatex
	once. Editing the file invalidates that on the next request, because the key
	changes with it.

	The =pdf= exporter must be enabled in the server's config under
	=server.exporters=, and pdflatex must be installed; without either, the
	answer is a 500 saying which.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                  |
	|------------+--------+----------+----------------------------------------------|
	| =query=    | string | yes      | The org file to export (basename or path)    |
	| =refresh=  | string | no       | =t= to rebuild even when the cache has it    |

	*Response:* =application/pdf= on success; =text/plain= with the reason and a
	500 otherwise.
EDOC */
func RequestPdfView(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	fname := r.URL.Query().Get("query")
	if fname == "" {
		fname = r.URL.Query().Get("filename")
	}
	if fname == "" {
		http.Error(w, "no file was asked for", http.StatusBadRequest)
		return
	}
	path, err := FindFileInDb(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: %v", fname, err), http.StatusNotFound)
		return
	}

	cache, err := pdfCachePath(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: %v", fname, err), http.StatusNotFound)
		return
	}

	// One at a time per file, and the one that waited looks in the cache again
	// rather than building it a second time.
	lock, _ := pdfBuilding.LoadOrStore(cache, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	if r.URL.Query().Get("refresh") == "t" {
		os.Remove(cache)
	}
	if st, serr := os.Stat(cache); serr != nil || st.Size() == 0 {
		opts := common.ExportToFile{
			Name:     "pdf",
			Query:    path,
			Filename: cache,
			Props:    map[string]string{},
		}
		res, _ := ExportToFile(db, &opts)
		if !res.Ok {
			os.Remove(cache)
			http.Error(w, res.Msg, http.StatusInternalServerError)
			return
		}
		// The exporter says it succeeded by returning no error, which is not
		// the same as having produced a pdf - pdflatex failing on the document
		// itself is the common way to get here.
		if st, serr := os.Stat(cache); serr != nil || st.Size() == 0 {
			os.Remove(cache)
			http.Error(w, "the export produced no pdf - is pdflatex installed, and does the document compile?",
				http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q",
		strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".pdf"))
	http.ServeFile(w, r, cache)
}

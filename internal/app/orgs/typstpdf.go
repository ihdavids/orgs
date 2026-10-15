package orgs

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/typst"
)

// registerTypstRoutes adds the typst pdf endpoint. Called from RestApi.
func registerTypstRoutes(api *mux.Router) {
	api.HandleFunc("/typst/pdf", RequestTypstPdf).Methods("GET")
	api.HandleFunc("/typst/templates", RequestTypstTemplates).Methods("GET")
}

// The typst exporter: the yaml's when it is configured, a default one when it
// is not (it is Standalone).
func typstExporter() *typst.Exporter {
	if e, ok := exporterNamed("typst").(*typst.Exporter); ok && e != nil {
		return e
	}
	return &typst.Exporter{}
}

var typstBuilding sync.Map

// Where a compiled pdf is kept: named for the file's state and every setting
// that changes the pdf, so an edit or another template is a different name.
func typstCachePath(path string, settings ...string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s|%d|%d|%s", path, st.ModTime().UnixNano(), st.Size(), strings.Join(settings, "|"))
	dir := filepath.Join(os.TempDir(), "orgs-typst")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%x.pdf", sha1.Sum([]byte(key)))), nil
}

/* SDOC: API
* GET /typst/pdf — A File, Typeset by Typst

	Exports one org file with the =typst= exporter, compiles it with the
	=typst= compiler on the server, and answers with the pdf. Pictures the
	file links to are copied beside the document for the compile, so they are
	found wherever the file is.

	Needs =typst= on the server's PATH (or the typst exporter's =typst:= set
	to it); without it the answer is a 500 saying how to install it. Nothing
	needs configuring otherwise.

	The pdf is cached against the file's state and the settings asked for, so
	asking twice compiles once; =refresh=t= compiles again.

	*Query Parameters:*
	| Parameter  | Description                                                  |
	|------------+--------------------------------------------------------------|
	| =filename= | the org file (=query= is read too)                           |
	| =template= | article, report, notes, or one of yours (=theme= is read too) |
	| =paper=    | a4, us-letter, a5, ...                                       |
	| =math=     | mitex or typst                                               |
	| =columns=  | 2 for two columns (the notes template)                       |
	| =refresh=  | =t= to compile again                                         |

	*Response:* =application/pdf=, or =text/plain= with typst's own error.
EDOC */
func RequestTypstPdf(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	fname := q.Get("filename")
	if fname == "" {
		fname = q.Get("query")
	}
	if fname == "" {
		http.Error(w, "say which file (filename=)", http.StatusBadRequest)
		return
	}
	path, err := FindFileInDb(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: %v", fname, err), http.StatusNotFound)
		return
	}
	// Read with its dynamic blocks and formulas current (refresh.go); the
	// stamp keeps the cache honest when only they changed.
	of, stamp := exportFileStamped(path)
	if q.Get("refresh") == "f" {
		of, stamp = GetDb().FindByFile(path), ""
	}
	if of == nil || of.Doc == nil {
		http.Error(w, fmt.Sprintf("%s is not loaded", fname), http.StatusNotFound)
		return
	}
	exp := typstExporter()
	if _, err := exp.Compiler(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	template := q.Get("template")
	if template == "" {
		template = q.Get("theme")
	}
	props := map[string]string{"template": template, "paper": q.Get("paper"), "math": q.Get("math"), "columns": q.Get("columns")}
	cache, err := typstCachePath(path, props["template"], props["paper"], props["math"], props["columns"], stamp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lock, _ := typstBuilding.LoadOrStore(cache, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if q.Get("refresh") == "t" {
		os.Remove(cache)
	}
	if st, serr := os.Stat(cache); serr != nil || st.Size() == 0 {
		doc := of.Doc
		if doc.Path == "" {
			// Pictures are found relative to the file.
			doc.Path = path
		}
		b, err := exp.PDF(doc, props)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(cache, b, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q",
		strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".pdf"))
	http.ServeFile(w, r, cache)
}

/* SDOC: API
* GET /typst/templates — The Typst Exporter's Templates

	Answers ={"Ok": true, "Templates": ["article", "notes", "report", ...]}=:
	every =typst/*.typ= in a template folder, the built in ones and yours.
EDOC */
func RequestTypstTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"Ok": true, "Templates": typst.Templates()})
}

package orgs

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/beamer"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/latex"
)

// registerBeamerRoutes adds the beamer deck as a pdf. Called from RestApi.
func registerBeamerRoutes(api *mux.Router) {
	api.HandleFunc("/beamer/pdf", RequestBeamerPdf).Methods("GET")
	api.HandleFunc("/beamer/themes", RequestBeamerThemes).Methods("GET")
}

// Decks built, by what they were built from: the tex and how it was
// compiled. Asking twice for an unchanged deck compiles once; any edit
// changes the tex and so the key. Kept small - a deck is a few hundred kB.
var (
	beamerBuild sync.Mutex
	beamerCache = map[[32]byte][]byte{}
	beamerOrder [][32]byte
)

const beamerCacheSize = 8

// The beamer exporter: the yaml's, or one with no settings.
func beamerExporter() *beamer.OrgBeamerExporter {
	if e, ok := exporterNamed("beamer").(*beamer.OrgBeamerExporter); ok {
		return e
	}
	return &beamer.OrgBeamerExporter{}
}

/* SDOC: API
* GET /beamer/pdf — A File as a Beamer Deck, Compiled

	=GET /beamer/pdf?filename=talk.org= answers with the file as a beamer
	presentation, compiled to pdf (see *Beamer* under exporters). Its own
	endpoint, as =/pdf= and =/book= are, because the answer is bytes.

	*Query Parameters:*
	| Parameter  | Description                                                   |
	|------------+---------------------------------------------------------------|
	| =filename= | the org file                                                  |
	| =theme=    | a preset (clean, dark, metropolis, classic, academic, minimal) or a beamer theme; wins over the file's |
	| =code=     | =listings= (the default) or =minted=                          |
	| =refresh=  | =t= compiles again rather than answering from the cache       |

	The latex is the =beamer= exporter's =latex= setting, else the =pdf=
	exporter's =pdfLatex=, else the first of pdflatex, xelatex and lualatex
	that is installed. A machine with no TeX answers 501 saying so; a deck
	that does not compile answers 422 with latex's first error.
EDOC */
func RequestBeamerPdf(w http.ResponseWriter, r *http.Request) {
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
	// Read with its dynamic blocks and formulas current (refresh.go).
	doc := exportFile(path)
	if doc == nil || doc.Doc == nil {
		http.Error(w, fmt.Sprintf("%s is not loaded", fname), http.StatusNotFound)
		return
	}
	exp := beamerExporter()
	props := map[string]string{"theme": q.Get("theme"), "code": q.Get("code")}
	opts := exp.Options(props)
	tex := beamer.Render(doc.Doc, opts, props["theme"] != "")

	engine := exp.Engine()
	if engine == "" {
		if p, ok := Conf().GetExporter("pdf").(*latex.OrgPdfExporter); ok && p.PdfLatex != "" {
			engine = p.PdfLatex
		}
	}
	co := beamer.CompileOptions{Engine: engine, ShellEscape: opts.Code == "minted"}
	key := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v", tex, co.Engine, co.ShellEscape)))

	beamerBuild.Lock()
	defer beamerBuild.Unlock()
	pdf, ok := beamerCache[key]
	if !ok || q.Get("refresh") == "t" {
		pdf, err = beamer.Compile(tex, co)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, beamer.ErrNoTeX) || strings.Contains(err.Error(), "not installed") {
				status = http.StatusNotImplemented
			}
			http.Error(w, err.Error(), status)
			return
		}
		if _, had := beamerCache[key]; !had {
			beamerOrder = append(beamerOrder, key)
			if len(beamerOrder) > beamerCacheSize {
				delete(beamerCache, beamerOrder[0])
				beamerOrder = beamerOrder[1:]
			}
		}
		beamerCache[key] = pdf
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", base+".pdf"))
	w.Write(pdf)
}

/* SDOC: API
* GET /beamer/themes — The Beamer Presets

	={"Ok": true, "Themes": ["clean", "dark", ...]}=: the presets a deck can
	be exported in when its file names no =#+BEAMER_THEME:=. Any installed
	beamer theme's name works as well; these are the ones orgs dresses itself.
EDOC */
func RequestBeamerThemes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"Ok":true,"Themes":["%s"]}`, strings.Join(beamer.PresetNames(), `","`))
}

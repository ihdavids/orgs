package orgs

/* SDOC: Editing
* Exporters

  TODO: Fill in information on working with exporters
EDOC */

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs/manuscript"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/pandoc"
	"github.com/ihdavids/orgs/internal/common"
)

var (
	standaloneLock sync.Mutex
	standalone     = map[string]common.Exporter{}
)

// exporterNamed is the exporter a request names: the one the yaml configured,
// or failing that a Standalone one, made (and started) the first time it is
// asked for. Nil when there is neither.
func exporterNamed(name string) common.Exporter {
	for _, exp := range Conf().Server.Exporters {
		if exp.Name == name {
			return exp.Plugin
		}
	}
	standaloneLock.Lock()
	defer standaloneLock.Unlock()
	if e, ok := standalone[name]; ok {
		return e
	}
	create := common.FindExporter(name)
	if create == nil {
		return nil
	}
	e := create()
	if s, ok := e.(common.Standalone); !ok || !s.Standalone() {
		return nil
	}
	e.Startup(Conf().PlugManager, &common.PluginOpts{})
	standalone[name] = e
	return e
}

// ExporterNames is every name /file/{type} answers to: the configured
// exporters and the standalone ones.
func ExporterNames() []string {
	seen := map[string]bool{}
	names := []string{}
	for _, e := range Conf().Server.Exporters {
		if !seen[e.Name] {
			seen[e.Name] = true
			names = append(names, e.Name)
		}
	}
	for _, n := range common.StandaloneExporters() {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

func ExportToFile(db common.ODb, args *common.ExportToFile) (common.ResultMsg, error) {
	fmt.Fprintf(os.Stderr, "EXPORT CALLED!\n")
	var didWrite = false
	var found = false
	msg := "Unknown Error"
	if exp := exporterNamed(args.Name); exp != nil {
		{
			found = true
			err := exp.Export(db, args.Query, args.Filename, args.Opts, args.Props)
			if err == nil {
				didWrite = true
				msg = "Success"
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("EXPORT: %s\n", args.Name)
		}
	}
	// Only claim the exporter is missing when it really is, otherwise the
	// reason the export failed would be thrown away.
	if !found {
		msg = fmt.Sprintf("ERROR: Did not export is %s setup in the config file?\n", args.Name)
	}
	if !didWrite {
		log.Printf("%v", msg)
	}
	return common.ResultMsg{Ok: didWrite, Msg: msg}, nil
}

func ExportToString(db common.ODb, args *common.ExportToFile) (common.ResultMsg, error) {
	fmt.Fprintf(os.Stderr, "EXPORT String CALLED!\n")
	var didWrite = false
	var found = false
	msg := "Unknown Error"
	if exp := exporterNamed(args.Name); exp != nil {
		{
			found = true
			err, txt := exp.ExportToString(db, args.Query, args.Opts, args.Props)
			if err == nil {
				didWrite = true
				msg = txt
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("EXPORT: %s\n", args.Name)
		}
	}
	// As in ExportToFile: the exporter's own reason, unless there was no
	// exporter. This used to overwrite every failure with "is it set up?",
	// which sent people to the config for errors in their document.
	if !found {
		msg = fmt.Sprintf("ERROR: Did not export is %s setup in the config file?\n", args.Name)
	}
	if !didWrite {
		log.Printf("%v", msg)
	}
	return common.ResultMsg{Ok: didWrite, Msg: msg}, nil
}

func PluginUpdateTarget(db common.ODb, args *common.Target, name string) (common.ResultMsg, error) {
	fmt.Fprintf(os.Stderr, "UPDATE CALLED!\n")
	var didWrite = false
	msg := "Unknown Error"
	for _, exp := range Conf().Server.Updaters {
		if exp.Name == name {
			res, err := exp.Plugin.UpdateTarget(db, args, Conf().PlugManager)
			if err == nil {
				didWrite = true
				msg = res.Msg
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("UPDATE: %s\n", exp.Name)
			break
		}
	}
	if !didWrite {
		msg = fmt.Sprintf("ERROR: Did not update is %s setup in the config file?\n", name)
		log.Printf("%v", msg)
	}
	return common.ResultMsg{Ok: didWrite, Msg: msg}, nil
}

/* SDOC: API
* GET /pandoc — A File In Any Format Pandoc Writes

	=GET /pandoc?filename=/path/notes.org&to=docx= answers with the file
	converted by pandoc: docx, odt, epub, pptx, rst, mediawiki, asciidoc, plain
	and the rest. Its own endpoint, as =/pdf= is, because most of these are
	bytes rather than text. Uses the =pandoc= exporter's settings when one is
	configured, and =pandoc= on the PATH otherwise.
EDOC */
func RequestPandoc(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	fname := r.URL.Query().Get("filename")
	if fname == "" {
		fname = r.URL.Query().Get("query")
	}
	to := r.URL.Query().Get("to")
	if fname == "" || to == "" {
		http.Error(w, "say which file (filename=) and which format (to=)", http.StatusBadRequest)
		return
	}
	path, err := FindFileInDb(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: %v", fname, err), http.StatusNotFound)
		return
	}
	conv := &pandoc.Exporter{}
	for _, exp := range Conf().Server.Exporters {
		if p, ok := exp.Plugin.(*pandoc.Exporter); ok {
			conv = p
		}
	}
	b, err := conv.Convert(path, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", pandoc.ContentType(to))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q",
		strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+"."+pandoc.Extension(to)))
	w.Write(b)
}

/* SDOC: API
* GET /book — A File as a Manuscript (docx) or an Ebook (epub)

	=GET /book?filename=story.org&to=docx= answers with the file as a
	manuscript, =to=epub= as an ebook (see *Manuscripts and Ebooks*). Written
	by orgs itself, so nothing needs installing; the =manuscript= and =epub=
	exporters' settings are used when they are configured, and need not be.

	*Query Parameters:*
	| Parameter  | Description                                                |
	|------------+------------------------------------------------------------|
	| =filename= | the org file                                               |
	| =to=       | =docx= (or =manuscript=) or =epub=                         |
	| =style=    | the manuscript style: modern, shunn, novel, reader         |
	| =paper=    | letter or a4                                               |
	| =theme=    | the ebook theme: classic, literary, modern, plain          |
	| =list=     | =t= answers ={"Styles": [...], "Themes": [...]}= instead   |

	*Response:* the file, with =X-Orgs-Words= (the word count) and, for an
	ebook, an =X-Orgs-Warning= header per picture that could not be put in.
EDOC */
func RequestBook(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	if q.Get("list") == "t" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"Ok": true, "Styles": manuscript.StyleNames(), "Themes": manuscript.EpubThemes()})
		return
	}
	fname := q.Get("filename")
	if fname == "" {
		fname = q.Get("query")
	}
	to := strings.ToLower(q.Get("to"))
	if fname == "" || to == "" {
		http.Error(w, "say which file (filename=) and which format (to=docx or to=epub)", http.StatusBadRequest)
		return
	}
	path, err := FindFileInDb(fname)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s: %v", fname, err), http.StatusNotFound)
		return
	}
	of := GetDb().FindByFile(path)
	if of == nil || of.Doc == nil {
		http.Error(w, fmt.Sprintf("%s is not loaded", fname), http.StatusNotFound)
		return
	}
	doc := of.Doc
	if doc.Path == "" {
		// Pictures and #+COVER are relative to the file.
		doc.Path = path
	}
	props := map[string]string{"style": q.Get("style"), "paper": q.Get("paper"), "theme": q.Get("theme")}
	docx, epub := &manuscript.DocxExporter{}, &manuscript.EpubExporter{}
	for _, exp := range Conf().Server.Exporters {
		switch p := exp.Plugin.(type) {
		case *manuscript.DocxExporter:
			docx = p
		case *manuscript.EpubExporter:
			epub = p
		}
	}
	// One file's contact details serve both: an ebook-only setup still has
	// the author's name from the manuscript exporter, and the other way about.
	if epub.Settings.Props == nil && docx.Settings.Props != nil {
		epub.Settings = docx.Settings
	}
	if docx.Settings.Props == nil && epub.Settings.Props != nil {
		docx.Settings = epub.Settings
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var b []byte
	var warns []string
	typ, ext := "", ""
	switch to {
	case "docx", "manuscript", "word":
		b, err = docx.Bytes(doc, props)
		typ, ext = "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"
	case "epub", "epub3", "ebook":
		b, warns, err = epub.Bytes(doc, props)
		typ, ext = "application/epub+zip", "epub"
	default:
		http.Error(w, fmt.Sprintf("to=%s: docx or epub", to), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	book := manuscript.Read(doc, path, docx.Defaults())
	w.Header().Set("X-Orgs-Words", fmt.Sprint(book.Words))
	for _, warn := range warns {
		w.Header().Add("X-Orgs-Warning", warn)
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base+"."+ext))
	w.Write(b)
}

package orgs

/* SDOC: Editing
* Exporters

  TODO: Fill in information on working with exporters
EDOC */

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs/pandoc"
	"github.com/ihdavids/orgs/internal/common"
)

func ExportToFile(db common.ODb, args *common.ExportToFile) (common.ResultMsg, error) {
	fmt.Fprintf(os.Stderr, "EXPORT CALLED!\n")
	var didWrite = false
	var found = false
	msg := "Unknown Error"
	for _, exp := range Conf().Server.Exporters {
		if exp.Name == args.Name {
			found = true
			err := exp.Plugin.Export(db, args.Query, args.Filename, args.Opts, args.Props)
			if err == nil {
				didWrite = true
				msg = "Success"
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("EXPORT: %s\n", exp.Name)
			break
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
	for _, exp := range Conf().Server.Exporters {
		if exp.Name == args.Name {
			found = true
			err, txt := exp.Plugin.ExportToString(db, args.Query, args.Opts, args.Props)
			if err == nil {
				didWrite = true
				msg = txt
			} else {
				didWrite = false
				msg = err.Error()
			}
			log.Printf("EXPORT: %s\n", exp.Name)
			break
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

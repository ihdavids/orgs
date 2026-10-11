package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

// registerImportRoutes adds the importers. Called from RestApi. Only
// notebooks for now; another format is another route under /import/.
func registerImportRoutes(api *mux.Router) {
	api.HandleFunc("/import/ipynb", PostImportIpynb).Methods("POST")
}

// The org file a request names: the notebook's own name when it names none,
// with .org in place of .ipynb, inside an org directory or refused.
func ipynbTarget(req *common.IpynbImport) (string, error) {
	name := strings.TrimSpace(req.Filename)
	if name == "" {
		name = filepath.Base(strings.TrimSpace(req.Name))
		if name == "" || name == "." || name == "/" {
			name = "notebook"
		}
	}
	switch ext := strings.ToLower(filepath.Ext(name)); ext {
	case ".org":
	case ".ipynb":
		name = name[:len(name)-len(ext)] + ".org"
	case "":
		name += ".org"
	default:
		return "", fmt.Errorf("the org file has to end in .org, not %s", ext)
	}
	path, ok := insideOrgDirs(name)
	if !ok {
		return "", fmt.Errorf("%s is not inside one of the org directories", name)
	}
	return path, nil
}

// ImportIpynb converts a notebook and writes it, and the pictures in it, into
// the org directories.
func ImportIpynb(req *common.IpynbImport) (common.IpynbImported, int) {
	fail := func(status int, format string, args ...interface{}) (common.IpynbImported, int) {
		return common.IpynbImported{Ok: false, Msg: fmt.Sprintf(format, args...)}, status
	}
	if strings.TrimSpace(req.Notebook) == "" {
		return fail(http.StatusBadRequest, "no notebook was sent")
	}
	path, err := ipynbTarget(req)
	if err != nil {
		return fail(http.StatusBadRequest, "%v", err)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	conv, err := convertIpynb([]byte(req.Notebook), ipynbOptions{Base: base, Source: req.Name, Now: time.Now()})
	if err != nil {
		return fail(http.StatusBadRequest, "%v", err)
	}
	res := conv.IpynbImported
	res.Filename = path
	res.Files = []string{}
	dir := filepath.Dir(path)
	assets := map[string][]byte{}
	for rel, b := range conv.Assets {
		abs, ok := insideOrgDirs(filepath.Join(dir, filepath.FromSlash(rel)))
		if !ok {
			return fail(http.StatusBadRequest, "a picture would land outside the org directories: %s", rel)
		}
		assets[abs] = b
		res.Files = append(res.Files, abs)
	}
	sort.Strings(res.Files)

	if !req.Overwrite {
		clash := ""
		if _, err := os.Stat(path); err == nil {
			clash = fmt.Sprintf("%s already exists. Pick another name, or overwrite it", path)
		}
		for _, f := range res.Files {
			if _, err := os.Stat(f); err == nil && clash == "" {
				clash = fmt.Sprintf("%s already exists, and importing would write over it. Pick another name, or overwrite it", f)
			}
		}
		// A dry run writes nothing, so a clash is something to know rather
		// than a reason to refuse.
		if clash != "" && !req.DryRun {
			return fail(http.StatusConflict, "%s", clash)
		}
		if clash != "" {
			res.Warnings = append(res.Warnings, clash)
		}
	}
	if req.DryRun {
		res.Ok = true
		res.Org = conv.Org
		res.Msg = "nothing written (a dry run): " + importSummary(&res)
		return res, http.StatusOK
	}

	// Pictures first, so the file never links to one that is not there yet.
	for f, b := range assets {
		if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
			return fail(http.StatusInternalServerError, "%v", err)
		}
		if err := os.WriteFile(f, b, 0644); err != nil {
			return fail(http.StatusInternalServerError, "%v", err)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fail(http.StatusInternalServerError, "%v", err)
	}
	if err := os.WriteFile(path, []byte(conv.Org), 0644); err != nil {
		return fail(http.StatusInternalServerError, "%v", err)
	}
	// A notebook imported over an old one starts with a fresh python, as a
	// notebook opened afresh does.
	stopSessionsFor(path, "")
	GetDb().ReloadFile(path)
	res.Ok = true
	res.Msg = importSummary(&res)
	return res, http.StatusOK
}

func importSummary(r *common.IpynbImported) string {
	s := fmt.Sprintf("%s: %s, %s, %s", countOf(r.Cells, "cell"), countOf(r.Blocks, "source block"),
		countOf(r.Outputs, "result"), countOf(r.Images, "picture"))
	if r.Errors > 0 {
		s += fmt.Sprintf(", %s kept as they were", countOf(r.Errors, "error"))
	}
	return s
}

func countOf(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

/* SDOC: API
* POST /import/ipynb — Import A Jupyter Notebook

	#+BEGIN_EXAMPLE
	{ "Notebook": "<the .ipynb text>", "Name": "analysis.ipynb",
	  "Filename": "science/analysis.org", "Overwrite": false, "DryRun": false }
	#+END_EXAMPLE

	Writes the notebook as an org file - markdown as org, code cells as
	source blocks sharing one python session, outputs as their
	=#+RESULTS:= - and its pictures into =<name>_files/= beside it, then
	loads the file.

	=Filename= is relative to the first org directory or absolute inside
	one; empty means the notebook's name in the first org directory. A file
	or picture already there is refused unless =Overwrite=. =DryRun= writes
	nothing and answers the org text in =Org=.

	The answer says where everything went (=Filename=, =Files=), what the
	notebook was (=Title=, =Language=, =Session=, =Nbformat=) and what it
	held (=Cells=, =Markdown=, =Code=, =Raw=, =Blocks=, =Outputs=, =Images=,
	=Errors=). A refusal is =Ok: false= with the reason in =Msg=.
EDOC */
func PostImportIpynb(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		answerErr(w, err)
		return
	}
	var req common.IpynbImport
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	res, status := ImportIpynb(&req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(res)
}

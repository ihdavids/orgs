package orgs

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/site"
	"github.com/ihdavids/orgs/internal/common"
)

// registerSiteRoutes adds the static site generator. Called from RestApi.
func registerSiteRoutes(api *mux.Router) {
	api.HandleFunc("/site", RequestSite).Methods("GET")
}

// siteSourceDir is the folder a site is built from: relative to the first org
// directory or absolute inside one, the org directory itself included (unlike
// insideOrgDirs, which is about files).
func siteSourceDir(dir string) (string, error) {
	dirs := Conf().Server.OrgDirs
	if len(dirs) == 0 {
		return "", fmt.Errorf("the server has no org directories")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("say which folder to publish (dir=), relative to %s; dir=. is all of it", dirs[0])
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(dirs[0], dir)
	}
	// The org directories may be relative (-orgdir ./notes), so both sides are
	// made absolute before one is compared with the other.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for _, d := range dirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, dir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return "", fmt.Errorf("%s is not a folder", dir)
			}
			return dir, nil
		}
	}
	return "", fmt.Errorf("%s is not inside one of the org directories", dir)
}

// BuildSite builds the site for a source folder from the loaded files.
func BuildSite(dir string, o site.Options) (*site.Result, error) {
	src, err := siteSourceDir(dir)
	if err != nil {
		return nil, err
	}
	o.Dir = src
	o.Roots = Conf().Server.OrgDirs
	sources := []site.Source{}
	prefix := src + string(filepath.Separator)
	for _, f := range GetDb().GetFiles() {
		abs, err := filepath.Abs(f)
		if err != nil || !strings.HasPrefix(abs, prefix) {
			continue
		}
		// Each page read with its dynamic blocks and formulas current.
		if of := exportFile(f); of != nil && of.Doc != nil {
			sources = append(sources, site.Source{Path: abs, Doc: of.Doc})
		}
	}
	return site.Build(sources, o)
}

func zipSite(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	buf := bytes.Buffer{}
	z := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := z.Create(n)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeSiteTo writes a built site into a folder on this machine.
func writeSiteTo(out string, files map[string][]byte) error {
	for name, data := range files {
		p := filepath.Join(out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

/* SDOC: API
* GET /site — A Folder of Org Files as a Static Web Site

	=GET /site?dir=blog&theme=paper= answers with a zip of a whole web site
	made from every org file under =dir= (relative to the first org directory
	or absolute inside one; =dir=.= is all of it): a page per file, a home
	page (the folder's =index.org=, or one made from its dated posts and
	pages), tag pages, an archive, backlinks, an Atom =feed.xml=, a
	=sitemap.xml= (with a base url), a =404.html= and a search box that works
	with no server. Every link is relative, so the site works opened from
	disk or served from any path.

	*Query Parameters:*
	| Parameter | Description                                                  |
	|-----------+--------------------------------------------------------------|
	| =dir=     | the folder to publish                                        |
	| =theme=   | paper, ink, tufte, garden, docs, mono or journal (default paper, or site.yaml's) |
	| =drafts=  | =t= publishes files marked =#+DRAFT: t= too                  |
	| =base=    | the url the site will live at, for the feed and the sitemap  |
	| =title=   | the site's title (default site.yaml's, else the folder's name) |
	| =local=   | =t= writes the site on the server's disk at =out= instead    |
	| =out=     | with =local=t=: the folder to write into                     |
	| =list=    | =t= answers ={"Ok": true, "Themes": [{"Name", "Description"}]}= |

	A =site.yaml= in the folder sets =title=, =description=, =author=,
	=baseURL=, =theme=, =language=, =footer=, =feedItems= and =nav= (a list
	of ={title, url}=). Per file: =#+TITLE=, =#+SUBTITLE=, =#+DATE= (makes
	it a post), =#+AUTHOR=, =#+DESCRIPTION=, =#+FILETAGS=, =#+WEIGHT= (order
	in the navigation) and =#+DRAFT=. Headings tagged =noexport= and
	=COMMENT= subtrees are left out; linked pictures and files are copied in
	(only from inside the org directories).

	*Response:* the zip, with =X-Orgs-Pages=, =X-Orgs-Theme= and an
	=X-Orgs-Warning= header per link that leads nowhere on the site; with
	=local=t= a =ResultMsg=.
EDOC */
func RequestSite(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	q := r.URL.Query()
	if q.Get("list") == "t" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"Ok": true, "Themes": site.Themes()})
		return
	}
	res, err := BuildSite(q.Get("dir"), site.Options{
		Theme: q.Get("theme"), Title: q.Get("title"), BaseURL: q.Get("base"), Drafts: q.Get("drafts") == "t",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if q.Get("local") == "t" {
		out := strings.TrimSpace(q.Get("out"))
		w.Header().Set("Content-Type", "application/json")
		if out == "" {
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "local=t needs out=, the folder to write the site into"})
			return
		}
		if err := writeSiteTo(out, res.Files); err != nil {
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
		msg := fmt.Sprintf("wrote %d pages (%d files) to %s, theme %s", res.Pages, len(res.Files), out, res.Theme)
		for _, warn := range res.Warnings {
			msg += "\n" + warn
		}
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: msg})
		return
	}
	body, err := zipSite(res.Files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("X-Orgs-Pages", fmt.Sprint(res.Pages))
	w.Header().Set("X-Orgs-Theme", res.Theme)
	for i, warn := range res.Warnings {
		if i == 50 {
			w.Header().Add("X-Orgs-Warning", fmt.Sprintf("and %d more", len(res.Warnings)-50))
			break
		}
		w.Header().Add("X-Orgs-Warning", strings.ReplaceAll(warn, "\n", " "))
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="site.zip"`)
	w.Write(body)
}

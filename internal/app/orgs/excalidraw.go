package orgs

// Drawings: Excalidraw scenes kept beside the org files (features.excalidraw,
// after Emacs's org-excalidraw).
//
// A drawing is a `.excalidraw` file - Excalidraw's own json, what
// excalidraw.com saves - or the markdown form `.excalidraw.md`, linked from org as
// a picture is:
//
//	#+CAPTION: How a request gets in
//	#+ATTR_HTML: :width 600
//	[[file:drawings/request.excalidraw]]
//
// org-excalidraw's `[[excalidraw:…]]` links are read the same way. Beside
// the drawing lives its picture, `request.excalidraw.svg` (and .png), which
// worg writes with Excalidraw's own exportToSvg every time it saves - the
// name org-excalidraw and excalidraw.com's "embed scene" export both use, so
// Emacs shows it inline with org-display-inline-images too.
//
// Exports (the export copy, exportLinesFor) point such a link at the
// picture instead. When there is no picture, or the drawing has changed
// since it was made, a plain one is drawn here (internal/common/excalidraw's
// Render: shapes, arrows, text, strokes, pictures - not the hand-drawn
// look) and written in its place. LaTeX wants a png: worg's, if it is fresh,
// else one made from the svg by rsvg-convert when that is installed; with
// neither the link is left as a link.
//
// Only with the feature on: while off, links are left as links and the
// endpoints answer Off.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common/excalidraw"
)

const (
	maxDrawingPicture = 32 << 20
	maxDrawingBody    = 128 << 20
)

// One write at a time: a save and an export drawing the same picture.
var excalidrawLock sync.Mutex

func registerExcalidrawRoutes(api *mux.Router) {
	api.HandleFunc("/excalidraw/list", RequestExcalidrawList).Methods("GET")
	api.HandleFunc("/excalidraw", RequestExcalidraw).Methods("GET")
	api.HandleFunc("/excalidraw/svg", RequestExcalidrawSvg).Methods("GET")
	api.HandleFunc("/excalidraw/save", PostExcalidrawSave).Methods("POST")
	api.HandleFunc("/excalidraw/new", PostExcalidrawNew).Methods("POST")
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

// drawingPath is where a client's name for a drawing is: an absolute path,
// a path from the first org directory, or the `/images/…` url the html
// exporter wrote for its picture (worg's click on a rendered drawing). It
// must be inside the org directories and be a drawing.
func drawingPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "file:")
	name = strings.TrimPrefix(name, "excalidraw:")
	if strings.HasPrefix(name, "/images/") {
		name = strings.TrimPrefix(name, "/images/")
	}
	for _, ext := range []string{".svg", ".png"} {
		if strings.HasSuffix(strings.ToLower(name), ".excalidraw"+ext) {
			name = name[:len(name)-len(ext)]
		}
	}
	if name == "" {
		return "", fmt.Errorf("which drawing? give file=")
	}
	if !excalidraw.IsDrawingName(name) {
		return "", fmt.Errorf("%s is not a drawing: a drawing ends in .excalidraw or .excalidraw.md", filepath.Base(name))
	}
	p := filepath.FromSlash(name)
	if !filepath.IsAbs(p) {
		dirs := Conf().Server.OrgDirs
		if len(dirs) == 0 {
			return "", fmt.Errorf("this server has no org directories")
		}
		p = filepath.Join(dirs[0], p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	clean, ok := insideOrgDirs(abs)
	if !ok {
		return "", fmt.Errorf("%s is not inside the org directories", name)
	}
	return clean, nil
}

func drawingPictures(path string) (svg, png string) {
	base := excalidraw.PictureBase(path)
	return base + ".svg", base + ".png"
}

// freshBeside says whether a picture is there and no older than the drawing.
func freshBeside(picture string, drawing os.FileInfo) bool {
	st, err := os.Stat(picture)
	return err == nil && drawing != nil && !st.ModTime().Before(drawing.ModTime())
}

// readDrawing is a drawing's scene json, its format and, for a markdown
// drawing, the markdown it came out of. The scene of a markdown drawing has the
// pictures its Embedded Files section names put in as data urls, so the
// editor and the renderer see them.
func readDrawing(path string) (scene []byte, format string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(b) > excalidraw.MaxSceneBytes*2 {
		return nil, "", fmt.Errorf("%s is too big to be a drawing", filepath.Base(path))
	}
	if !excalidraw.IsMarkdownName(path) {
		return b, "excalidraw", nil
	}
	js, info, err := excalidraw.ReadMarkdown(string(b))
	if err != nil {
		return nil, "", err
	}
	if len(info.Embedded) > 0 {
		js = withEmbeddedFiles(js, path, info.Embedded)
	}
	return js, "obsidian", nil
}

// withEmbeddedFiles adds a markdown drawing's vault pictures to its scene.
func withEmbeddedFiles(js []byte, path string, embedded map[string]string) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(js, &m) != nil {
		return js
	}
	files := map[string]json.RawMessage{}
	json.Unmarshal(m["files"], &files)
	added := false
	for id, v := range embedded {
		target := excalidraw.EmbeddedTarget(v)
		if target == "" {
			continue
		}
		p := findVaultFile(path, target)
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) > maxDrawingPicture {
			continue
		}
		mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
		if !strings.HasPrefix(mt, "image/") {
			mt = http.DetectContentType(b)
		}
		if !strings.HasPrefix(mt, "image/") {
			continue
		}
		if i := strings.Index(mt, ";"); i >= 0 {
			mt = mt[:i]
		}
		st, _ := os.Stat(p)
		f, _ := json.Marshal(excalidraw.FileData{ID: id, MimeType: mt,
			DataURL: "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b), Created: st.ModTime().UnixMilli()})
		files[id] = f
		added = true
	}
	if !added {
		return js
	}
	m["files"], _ = json.Marshal(files)
	out, err := json.Marshal(m)
	if err != nil {
		return js
	}
	return out
}

// findVaultFile is the file a markdown drawing's `[[name]]` means: beside the
// drawing, from an org directory's root (a vault path), else the first file
// of that name in the drawing's org directory (shortest-path
// links).
func findVaultFile(drawing, target string) string {
	t := filepath.FromSlash(target)
	try := []string{filepath.Join(filepath.Dir(drawing), t)}
	var home string
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		try = append(try, filepath.Join(root, t))
		if rel, err := filepath.Rel(root, drawing); err == nil && !strings.HasPrefix(rel, "..") && home == "" {
			home = root
		}
	}
	for _, p := range try {
		if c, ok := insideOrgDirs(p); ok {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
		}
	}
	if home == "" || strings.ContainsAny(target, "/\\") {
		return ""
	}
	found, seen := "", 0
	filepath.WalkDir(home, func(p string, e fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if seen++; seen > 20000 {
			return filepath.SkipAll
		}
		if e.IsDir() && p != home && (strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !e.IsDir() && e.Name() == target {
			found = p
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// writeBytesAtomic writes beside the file and renames over it, so a reader
// (the file server, another app, an export) never sees half a drawing.
func writeBytesAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	os.Chmod(name, 0644)
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// drawingSvg is the drawing's picture: the one beside it while it is fresh,
// else one drawn here. write keeps a drawn one beside the drawing for next
// time (the export copy does; GET /excalidraw/svg does not write).
func drawingSvg(path string, write bool) ([]byte, bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	svgPath, _ := drawingPictures(path)
	if freshBeside(svgPath, st) {
		if b, err := os.ReadFile(svgPath); err == nil {
			return b, false, nil
		}
	}
	js, _, err := readDrawing(path)
	if err != nil {
		return nil, false, err
	}
	sc, err := excalidraw.ParseScene(js)
	if err != nil {
		return nil, false, err
	}
	out := []byte(excalidraw.Render(sc, js))
	if write {
		excalidrawLock.Lock()
		err := writeBytesAtomic(svgPath, out)
		excalidrawLock.Unlock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "EXCALIDRAW: could not keep the picture of %s: %s\n", filepath.Base(path), err)
		}
	}
	return out, true, nil
}

// drawingPng is a fresh png of the drawing, or "" when there is no way to
// have one: worg's, else rsvg-convert over the svg.
func drawingPng(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	svgPath, pngPath := drawingPictures(path)
	if freshBeside(pngPath, st) {
		return pngPath
	}
	tool, err := exec.LookPath("rsvg-convert")
	if err != nil {
		return ""
	}
	if _, _, err := drawingSvg(path, true); err != nil {
		return ""
	}
	if !freshBeside(svgPath, st) {
		return ""
	}
	cmd := exec.Command(tool, "-z", "2", "-f", "png", "-o", pngPath, svgPath)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return ""
	}
	return pngPath
}

// ---------------------------------------------------------------------------
// The export copy
// ---------------------------------------------------------------------------

// A link with no description to a drawing, `[[file:x.excalidraw]]`,
// `[[x.excalidraw.md]]` or org-excalidraw's `[[excalidraw:x.excalidraw]]`.
var drawingLinkRe = regexp.MustCompile(`(?i)\[\[(file:|excalidraw:)?([^\[\]]+?\.excalidraw(?:\.md)?)\]\]`)

// Any link to a drawing, with a description or without
// (`[[file:bed.excalidraw][Bed layout]]`): what "linked from" counts. Only
// the bare form is a picture to the export pass, but both are links.
var drawingAnyLinkRe = regexp.MustCompile(`(?i)\[\[(file:|excalidraw:)?([^\[\]]+?\.excalidraw(?:\.md)?)\](?:\[[^\[\]]*\])?\]`)

func excalidrawMayApply(text string) bool {
	return featureOn("excalidraw") && strings.Contains(strings.ToLower(text), ".excalidraw")
}

// wantsPng is whether a backend cannot show an svg.
func wantsPng(backend string) bool {
	b := strings.ToLower(backend)
	return b == "latex" || b == "pdf" || b == "beamer" || strings.HasPrefix(b, "pandoc-latex") ||
		strings.HasPrefix(b, "pandoc-docx") || strings.HasPrefix(b, "pandoc-odt") || strings.HasPrefix(b, "pandoc-pdf")
}

var exampleBeginRe = regexp.MustCompile(`(?i)^\s*#\+begin_(src|example)\b`)
var exampleEndRe = regexp.MustCompile(`(?i)^\s*#\+end_(src|example)\b`)

// excalidrawExportLines points every drawing link in the copy at the
// drawing's picture, making the picture when it is missing or stale.
func excalidrawExportLines(filename string, lines []string, backend string) ([]string, bool) {
	changed := false
	inBlock := false
	png := wantsPng(backend)
	out := lines
	for i, l := range lines {
		if inBlock {
			if exampleEndRe.MatchString(l) {
				inBlock = false
			}
			continue
		}
		if exampleBeginRe.MatchString(l) {
			inBlock = true
			continue
		}
		if !strings.Contains(strings.ToLower(l), ".excalidraw") {
			continue
		}
		nl := drawingLinkRe.ReplaceAllStringFunc(l, func(m string) string {
			sub := drawingLinkRe.FindStringSubmatch(m)
			target := sub[2]
			p := filepath.FromSlash(strings.TrimPrefix(target, "file://"))
			if strings.HasPrefix(p, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					p = filepath.Join(home, p[2:])
				}
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(filepath.Dir(filename), p)
			}
			abs, err := filepath.Abs(p)
			if err != nil {
				return m
			}
			st, err := os.Stat(abs)
			if err != nil {
				return m
			}
			svgPath, _ := drawingPictures(abs)
			if _, inside := insideOrgDirs(abs); !inside && !freshBeside(svgPath, st) {
				// Outside every org directory: use a picture somebody made,
				// never write one there.
				return m
			}
			ext := ".svg"
			if png {
				if drawingPng(abs) == "" {
					fmt.Fprintf(os.Stderr, "EXCALIDRAW: no png of %s for %s (save it from worg, or install rsvg-convert); left as a link\n", filepath.Base(abs), backend)
					return m
				}
				ext = ".png"
			} else if _, _, err := drawingSvg(abs, true); err != nil {
				fmt.Fprintf(os.Stderr, "EXCALIDRAW: %s: %s\n", filepath.Base(abs), err)
				return m
			}
			return "[[file:" + excalidraw.PictureBase(target) + ext + "]]"
		})
		if nl != l {
			if !changed {
				out = append([]string{}, lines...)
				changed = true
			}
			out[i] = nl
		}
	}
	return out, changed
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

// DrawingInfo is one drawing for a list.
type DrawingInfo struct {
	File string
	// From the org directory it is in, with slashes.
	Rel      string
	Name     string
	Format   string // excalidraw, or obsidian for an .excalidraw.md
	Modified int64  // unix milliseconds
	Size     int64
	// The picture beside it, as a url, and whether it is as new as the
	// drawing ("" when there is none).
	Svg   string
	Fresh bool
	// The org files that link to it, and the line (from 0).
	LinkedFrom []DrawingLink
}

// DrawingLink is a place an org file links to a drawing.
type DrawingLink struct {
	File string
	Line int
}

type drawingLinkIndex struct {
	at    uint64
	links map[string][]DrawingLink
}

var drawingLinks struct {
	sync.Mutex
	idx *drawingLinkIndex
}

// drawingLinksIndex is drawing → the org lines linking it, made again when
// the database reloads (Traps: caches key on ReloadIndex).
func drawingLinksIndex() map[string][]DrawingLink {
	db := GetDb()
	drawingLinks.Lock()
	defer drawingLinks.Unlock()
	if drawingLinks.idx != nil && drawingLinks.idx.at == db.ReloadIndex {
		return drawingLinks.idx.links
	}
	links := map[string][]DrawingLink{}
	for _, f := range db.GetFiles() {
		b, err := os.ReadFile(f)
		if err != nil || !bytes.Contains(bytes.ToLower(b), []byte(".excalidraw")) {
			continue
		}
		for i, l := range strings.Split(string(b), "\n") {
			for _, m := range drawingAnyLinkRe.FindAllStringSubmatch(l, -1) {
				p := filepath.FromSlash(m[2])
				if !filepath.IsAbs(p) {
					p = filepath.Join(filepath.Dir(f), p)
				}
				if abs, err := filepath.Abs(p); err == nil {
					links[abs] = append(links[abs], DrawingLink{File: f, Line: i})
				}
			}
		}
	}
	drawingLinks.idx = &drawingLinkIndex{at: db.ReloadIndex, links: links}
	return links
}

func drawingInfo(path, root string, links map[string][]DrawingLink) DrawingInfo {
	st, _ := os.Stat(path)
	rel, _ := filepath.Rel(root, path)
	name := filepath.Base(path)
	format := "excalidraw"
	if excalidraw.IsMarkdownName(name) {
		format = "obsidian"
		name = name[:len(name)-len(".excalidraw.md")]
	} else {
		name = name[:len(name)-len(".excalidraw")]
	}
	d := DrawingInfo{File: path, Rel: filepath.ToSlash(rel), Name: name, Format: format, LinkedFrom: []DrawingLink{}}
	if st != nil {
		d.Modified = st.ModTime().UnixMilli()
		d.Size = st.Size()
	}
	svgPath, _ := drawingPictures(path)
	if _, err := os.Stat(svgPath); err == nil {
		d.Svg = mediaURL(svgPath, path)
		d.Fresh = freshBeside(svgPath, st)
	}
	if abs, err := filepath.Abs(path); err == nil {
		if l, ok := links[abs]; ok {
			d.LinkedFrom = l
		}
	}
	return d
}

/* SDOC: API
* GET /excalidraw/list — The Drawings
	Every =.excalidraw= and =.excalidraw.md= file under the org directories,
	newest first, with the org lines that link to each.

	#+BEGIN_EXAMPLE
	GET /excalidraw/list
	-> {"Ok": true, "Drawings": [{"File": "/notes/drawings/flow.excalidraw", "Rel": "drawings/flow.excalidraw",
	     "Name": "flow", "Format": "excalidraw", "Modified": 1762300000000, "Size": 5120,
	     "Svg": "/images/drawings/flow.excalidraw.svg", "Fresh": true,
	     "LinkedFrom": [{"File": "/notes/design.org", "Line": 12}]}]}
	#+END_EXAMPLE

	=Svg= is the picture beside the drawing (empty when there is none); =Fresh=
	is false when the drawing has changed since it was made. Only with
	=features.excalidraw= on.
EDOC */
func RequestExcalidrawList(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "excalidraw") {
		return
	}
	links := drawingLinksIndex()
	out := []DrawingInfo{}
	seen := map[string]bool{}
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				if p != root && (strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules") {
					return filepath.SkipDir
				}
				return nil
			}
			if !excalidraw.IsDrawingName(e.Name()) || seen[p] {
				return nil
			}
			seen[p] = true
			out = append(out, drawingInfo(p, root, links))
			return nil
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	dynJson(w, map[string]interface{}{"Ok": true, "Drawings": out})
}

/* SDOC: API
* GET /excalidraw — One Drawing
	The scene, for an editor.

	#+BEGIN_EXAMPLE
	GET /excalidraw?file=drawings/flow.excalidraw
	-> {"Ok": true, "File": "/notes/drawings/flow.excalidraw", "Format": "excalidraw",
	    "Modified": 1762300000000, "Scene": {"type": "excalidraw", "version": 2, "elements": [...], "appState": {...}, "files": {...}},
	    "Drawing": {...as in the list...}}
	#+END_EXAMPLE

	=file= is a path inside the org directories (relative ones from the
	first), or the =/images/…excalidraw.svg= url of its picture. A markdown
	=.excalidraw.md= is answered as its scene, with the pictures its Embedded
	Files section names put in as data urls. =Modified= goes back with a save,
	which is refused if the file has changed since.
EDOC */
func RequestExcalidraw(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "excalidraw") {
		return
	}
	path, err := drawingPath(r.URL.Query().Get("file"))
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	js, format, err := readDrawing(path)
	if err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	if _, err := excalidraw.ParseScene(js); err != nil {
		dynJson(w, map[string]interface{}{"Ok": false, "Msg": err.Error()})
		return
	}
	info := drawingInfo(path, drawingRoot(path), drawingLinksIndex())
	dynJson(w, map[string]interface{}{"Ok": true, "File": path, "Format": format, "Modified": info.Modified,
		"Scene": json.RawMessage(js), "Drawing": info})
}

func drawingRoot(path string) string {
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return root
		}
	}
	return filepath.Dir(path)
}

/* SDOC: API
* GET /excalidraw/svg — A Drawing As A Picture
	=image/svg+xml=: the picture saved beside the drawing while it is as new as
	the drawing, else one drawn by the server (plain shapes, arrows, text,
	strokes and pictures; not Excalidraw's hand-drawn look), with the scene in
	its metadata so it opens again in excalidraw.com. Writes nothing.

	#+BEGIN_EXAMPLE
	GET /excalidraw/svg?file=drawings/flow.excalidraw
	GET /excalidraw/svg?file=drawings/flow.excalidraw&render=server   always the server's drawing
	#+END_EXAMPLE

	=X-Orgs-Drawn: server= says the server drew it.
EDOC */
func RequestExcalidrawSvg(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "excalidraw") {
		return
	}
	path, err := drawingPath(r.URL.Query().Get("file"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var out []byte
	drawn := false
	if r.URL.Query().Get("render") == "server" {
		js, _, rerr := readDrawing(path)
		if rerr == nil {
			var sc *excalidraw.Scene
			if sc, rerr = excalidraw.ParseScene(js); rerr == nil {
				out, drawn = []byte(excalidraw.Render(sc, js)), true
			}
		}
		err = rerr
	} else {
		out, drawn, err = drawingSvg(path, false)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	// A picture someone else's file holds: no script, whatever it says.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:")
	if drawn {
		w.Header().Set("X-Orgs-Drawn", "server")
	}
	w.Write(out)
}

// ExcalidrawSave is POST /excalidraw/save's body.
type ExcalidrawSave struct {
	File  string
	Scene json.RawMessage
	// Excalidraw's exportToSvg (with the scene embedded) and a png, as base64
	// or a data url. Either may be left out.
	Svg string
	Png string
	// The Modified the drawing was read at; the save is refused when the
	// file has changed since, unless Force.
	Modified int64
	Force    bool
}

func decodePng(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i > 0 {
		s = s[i+1:]
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("the png is not base64: %s", err)
	}
	if len(b) > maxDrawingPicture {
		return nil, fmt.Errorf("the png is too big (at most %d MB)", maxDrawingPicture>>20)
	}
	if !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, fmt.Errorf("the png is not a png")
	}
	return b, nil
}

/* SDOC: API
* POST /excalidraw/save — Save A Drawing
	#+BEGIN_EXAMPLE
	POST /excalidraw/save {"File": "drawings/flow.excalidraw", "Scene": {...},
	                       "Svg": "<svg ...>", "Png": "data:image/png;base64,...", "Modified": 1762300000000}
	-> {"Ok": true, "File": "/notes/drawings/flow.excalidraw", "Modified": 1762300100000, "Svg": "/images/drawings/flow.excalidraw.svg"}
	-> {"Ok": false, "Changed": true, "Msg": "flow.excalidraw has changed since it was opened"}
	#+END_EXAMPLE

	The scene must be an Excalidraw scene (=type= "excalidraw", a list of
	elements, pictures as data urls, at most 32 MB); the svg is refused if it
	could run anything (scripts, event handlers, foreign objects; pictures
	only as data urls, links only http(s) or mailto) and the png must be a
	png. Each file is written beside the drawing and renamed into place. An
	=.excalidraw.md= gets its drawing block and its Text Elements written
	again, the rest of the markdown kept. The file must exist (=/excalidraw/new=
	makes one) and be inside the org directories. Publishes an =excalidraw=
	event.
EDOC */
func PostExcalidrawSave(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "excalidraw") {
		return
	}
	fail := func(msg string, extra ...interface{}) {
		res := map[string]interface{}{"Ok": false, "Msg": msg}
		for i := 0; i+1 < len(extra); i += 2 {
			res[extra[i].(string)] = extra[i+1]
		}
		dynJson(w, res)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxDrawingBody+1))
	if err != nil {
		fail(err.Error())
		return
	}
	if len(body) > maxDrawingBody {
		fail("that drawing is too big to save")
		return
	}
	var req ExcalidrawSave
	if err := json.Unmarshal(body, &req); err != nil {
		fail("could not read the request: " + err.Error())
		return
	}
	path, err := drawingPath(req.File)
	if err != nil {
		fail(err.Error())
		return
	}
	sc, err := excalidraw.ParseScene(req.Scene)
	if err != nil {
		fail(err.Error())
		return
	}
	_ = sc
	if req.Svg != "" {
		if len(req.Svg) > maxDrawingPicture {
			fail("the svg is too big")
			return
		}
		if err := checkSvg(req.Svg, svgRules{noun: "a drawing's svg", dataImages: true, links: true}); err != nil {
			fail(err.Error())
			return
		}
	}
	var png []byte
	if req.Png != "" {
		if png, err = decodePng(req.Png); err != nil {
			fail(err.Error())
			return
		}
	}

	excalidrawLock.Lock()
	defer excalidrawLock.Unlock()
	st, err := os.Stat(path)
	if err != nil {
		fail(filepath.Base(path) + " is not there: make it with /excalidraw/new")
		return
	}
	if req.Modified != 0 && !req.Force && st.ModTime().UnixMilli() != req.Modified {
		fail(filepath.Base(path)+" has changed since it was opened", "Changed", true, "Modified", st.ModTime().UnixMilli())
		return
	}
	var data []byte
	if excalidraw.IsMarkdownName(path) {
		old, err := os.ReadFile(path)
		if err != nil {
			fail(err.Error())
			return
		}
		text, err := excalidraw.WriteMarkdown(string(old), req.Scene)
		if err != nil {
			fail(err.Error())
			return
		}
		data = []byte(text)
	} else {
		data = excalidraw.Pretty(req.Scene)
	}
	if err := writeBytesAtomic(path, data); err != nil {
		fail(err.Error())
		return
	}
	svgPath, pngPath := drawingPictures(path)
	if req.Svg != "" {
		if err := writeBytesAtomic(svgPath, []byte(req.Svg)); err != nil {
			fail("the drawing was saved but its svg was not: " + err.Error())
			return
		}
	}
	if png != nil {
		if err := writeBytesAtomic(pngPath, png); err != nil {
			fail("the drawing was saved but its png was not: " + err.Error())
			return
		}
	}
	nst, _ := os.Stat(path)
	res := map[string]interface{}{"Ok": true, "File": path, "Msg": "saved"}
	if nst != nil {
		res["Modified"] = nst.ModTime().UnixMilli()
	}
	if req.Svg != "" {
		res["Svg"] = mediaURL(svgPath, path)
	}
	Publish(Event{Kind: "excalidraw", File: path, Msg: "saved " + filepath.Base(path)})
	dynJson(w, res)
}

// ExcalidrawNew is POST /excalidraw/new's body.
type ExcalidrawNew struct {
	// The drawing's name or path: `flow`, `drawings/flow.excalidraw`,
	// `flow.excalidraw.md` (the markdown kind). Relative to the heading's org
	// file when there is a heading, else to the first org directory.
	File string
	// A heading to put `[[file:…]]` under (its hash), and an org file to
	// link from at a line instead (the editor's cursor, from 0; -1 = only
	// make the drawing and answer the link).
	Hash     string
	Filename string
	Line     int
	// Caption written above the link as #+CAPTION:.
	Caption string
}

/* SDOC: API
* POST /excalidraw/new — A New Drawing
	#+BEGIN_EXAMPLE
	POST /excalidraw/new {"File": "drawings/flow"}                         just the drawing
	POST /excalidraw/new {"File": "flow", "Hash": "<heading hash>", "Caption": "The flow"}   and a link under the heading
	POST /excalidraw/new {"File": "flow", "Filename": "notes.org", "Line": -1}                the link to write, relative to notes.org
	-> {"Ok": true, "File": "/notes/drawings/flow.excalidraw", "Link": "[[file:drawings/flow.excalidraw]]", "Filename": "/notes/design.org", "Line": 14}
	#+END_EXAMPLE

	Makes an empty drawing (=.excalidraw= unless the name ends in
	=.excalidraw.md=), refusing one that is already there, and with =Hash=
	writes the link at the end of that heading's own text, its =#+CAPTION:=
	above it. =Line= is where the link went (from 0). Without a heading,
	=Filename= only says what the link is relative to; nothing is written to
	it (worg and VS Code put it at the cursor themselves).
EDOC */
func PostExcalidrawNew(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "excalidraw") {
		return
	}
	fail := func(msg string) { dynJson(w, map[string]interface{}{"Ok": false, "Msg": msg}) }
	var req ExcalidrawNew
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		fail("could not read the request: " + err.Error())
		return
	}
	name := strings.TrimSpace(req.File)
	if name == "" {
		name = "drawing-" + time.Now().Format("2006-01-02-150405")
	}
	if !excalidraw.IsDrawingName(name) {
		name += ".excalidraw"
	}
	// Where the link will live decides what a relative name means.
	holder := ""
	var lines []string
	var from, to, lvl int
	if req.Hash != "" {
		var err error
		holder, lines, from, to, err = headingBodyLines(req.Hash)
		if err != nil {
			fail(err.Error())
			return
		}
		if sec := GetDb().FindByHash(req.Hash); sec != nil && sec.Headline != nil {
			lvl = sec.Headline.Lvl
		}
	} else if req.Filename != "" {
		if f := GetDb().FindByFile(req.Filename); f != nil {
			holder = f.Filename
		} else if p, ok := insideOrgDirs(absOr(req.Filename)); ok {
			holder = p
		}
	}
	p := filepath.FromSlash(name)
	if !filepath.IsAbs(p) && holder != "" {
		p = filepath.Join(filepath.Dir(holder), p)
	}
	// No heading or file: drawingPath takes it from the first org directory.
	path, err := drawingPath(p)
	if err != nil {
		fail(err.Error())
		return
	}
	excalidrawLock.Lock()
	if _, err := os.Stat(path); err == nil {
		excalidrawLock.Unlock()
		fail(filepath.Base(path) + " is already there")
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		excalidrawLock.Unlock()
		fail(err.Error())
		return
	}
	data := excalidraw.Empty("")
	if excalidraw.IsMarkdownName(path) {
		data = []byte(excalidraw.NewMarkdown(data))
	}
	err = writeBytesAtomic(path, data)
	excalidrawLock.Unlock()
	if err != nil {
		fail(err.Error())
		return
	}
	target := filepath.ToSlash(path)
	if holder != "" {
		target = linkRelativeTo(path, holder)
	}
	link := "[[file:" + target + "]]"
	res := map[string]interface{}{"Ok": true, "File": path, "Link": link, "Filename": holder, "Line": -1}
	if req.Hash != "" {
		indent := bodyIndent(lines, from, to, lvl)
		add := []string{}
		if c := strings.TrimSpace(req.Caption); c != "" {
			add = append(add, indent+"#+CAPTION: "+c)
		}
		add = append(add, indent+link)
		at := to + 1
		if at > len(lines) {
			at = len(lines)
		}
		// Its own paragraph: a blank line above unless there is one, or the
		// heading line itself is right above.
		if at > from && at > 0 && strings.TrimSpace(lines[at-1]) != "" {
			add = append([]string{""}, add...)
		}
		out := append([]string{}, lines[:at]...)
		out = append(out, add...)
		out = append(out, lines[at:]...)
		if err := writeLines(holder, out); err != nil {
			res["Msg"] = "the drawing was made but the heading could not be written: " + err.Error()
		} else {
			res["Line"] = at + len(add) - 1
		}
	}
	Publish(Event{Kind: "excalidraw", File: path, Msg: "new drawing " + filepath.Base(path)})
	dynJson(w, res)
}

func absOr(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

package orgs

// Canvases: boards of cards and arrows kept in `.canvas` files (the
// open JSON Canvas 1.0 format, https://jsoncanvas.org/spec/1.0/) anywhere
// under the org directories. Only with features.canvas on.
//
// The file is the format's own, laid out the way other JSON Canvas apps
// write it, so the same board opens there and here; the org side is ordinary org - a
// heading on a board is a file node naming the org file, with the heading's
// :ID: as its subpath (`"file": "notes.org", "subpath": "#^<ID>"`), the
// heading given an :ID: when it is put on a board (as org-id would). An
// org file links to a board with `[[file:board.canvas]]`, which the html
// exporter draws as the board.
//
// Read here as well: `#Heading` / `#Parent#Child` subpaths, and
// org's search options on the file (`notes.org::*Heading`, `::#custom-id`,
// `::42`); see jcanvas.ParseRef. Paths in a board are relative to the org
// directory holding it (vault-relative paths), and are also tried
// beside the board.
//
// Text cards are markdown in the format. Here a text card is shown as org when
// it reads as org and converted from markdown (internal/common/mdorg) when it
// reads as markdown (`# `, `**`, `[x](y)`, ``` fences), then rendered by the
// html exporter - and always written back exactly as typed.

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	htmlexp "github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/jcanvas"
	"github.com/ihdavids/orgs/internal/common/mdorg"
)

func registerCanvasRoutes(api *mux.Router) {
	api.HandleFunc("/canvas/list", RequestCanvasList).Methods("GET")
	api.HandleFunc("/canvas", RequestCanvas).Methods("GET")
	api.HandleFunc("/canvas/svg", RequestCanvasSvg).Methods("GET")
	api.HandleFunc("/canvas/save", PostCanvasSave).Methods("POST")
	api.HandleFunc("/canvas/new", PostCanvasNew).Methods("POST")
	api.HandleFunc("/canvas/add", PostCanvasAdd).Methods("POST")
	api.HandleFunc("/canvas/link", PostCanvasLink).Methods("POST")
	api.HandleFunc("/canvas/ref", PostCanvasRef).Methods("POST")
	api.HandleFunc("/canvas/render", PostCanvasRender).Methods("POST")
	api.HandleFunc("/canvas/for", RequestCanvasFor).Methods("GET")
}

func init() {
	// The html exporter draws [[file:x.canvas]] while the feature is on. It
	// cannot ask the server itself (the server imports the exporters).
	htmlexp.CanvasEmbed = canvasEmbed
}

// ---------------------------------------------------------------------------
// Finding boards
// ---------------------------------------------------------------------------

func isCanvasFile(p string) bool { return strings.EqualFold(filepath.Ext(p), ".canvas") }

// CanvasInfo is one board in the list.
type CanvasInfo struct {
	File  string // absolute
	Rel   string // relative to the org directory holding it
	Name  string
	Nodes int
	Edges int
	Mod   string
	Msg   string `json:",omitempty"` // it does not read as a canvas
}

func canvasFiles() []string {
	out := []string{}
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
				if p != root && strings.HasPrefix(e.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if isCanvasFile(p) && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// The org directory a board is in: the root its relative paths start from.
func canvasRoot(p string) string {
	best := ""
	for _, d := range Conf().Server.OrgDirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") && len(root) > len(best) {
			best = root
		}
	}
	if best == "" {
		return filepath.Dir(p)
	}
	return best
}

func canvasRel(p string) string {
	if rel, err := filepath.Rel(canvasRoot(p), p); err == nil {
		return filepath.ToSlash(rel)
	}
	return p
}

// canvasPath is the board a client named: absolute, relative to an org
// directory, or a name the list has (`board` or `board.canvas`). Only inside
// the org directories.
func canvasPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("which canvas? (file=)")
	}
	if !isCanvasFile(name) {
		name += ".canvas"
	}
	try := []string{}
	if filepath.IsAbs(name) {
		try = append(try, name)
	} else {
		for _, d := range Conf().Server.OrgDirs {
			if root, err := filepath.Abs(d); err == nil {
				try = append(try, filepath.Join(root, filepath.FromSlash(name)))
			}
		}
	}
	for _, t := range try {
		if p, ok := insideOrgDirs(t); ok {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	hits := []string{}
	for _, f := range canvasFiles() {
		if strings.EqualFold(filepath.Base(f), filepath.Base(name)) || strings.HasSuffix(f, string(filepath.Separator)+filepath.FromSlash(name)) {
			hits = append(hits, f)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return "", fmt.Errorf("%s names %d canvases: %s", name, len(hits), strings.Join(hits, ", "))
	}
	if len(try) > 0 {
		if _, ok := insideOrgDirs(try[0]); !ok {
			return "", fmt.Errorf("%s is not inside an org directory", name)
		}
	}
	return "", fmt.Errorf("no canvas called %s", name)
}

func canvasVersion(b []byte) string {
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:8])
}

func readCanvas(p string) (jcanvas.Canvas, []byte, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return jcanvas.Canvas{}, nil, err
	}
	c, err := jcanvas.Parse(b)
	return c, b, err
}

// writeCanvas writes a board through a temporary file and a rename, so a
// reader (another app, a sync tool) sees the old file or the new one.
func writeCanvas(p string, c *jcanvas.Canvas) (string, error) {
	b := c.Marshal()
	tmp, err := os.CreateTemp(filepath.Dir(p), ".orgs-canvas-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	tmp.Chmod(0644)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(name, p); err != nil {
		return "", err
	}
	canvasChanged(p)
	return canvasVersion(b), nil
}

// ---------------------------------------------------------------------------
// What a card shows
// ---------------------------------------------------------------------------

// A path in a board: absolute, relative to the board's org directory
// (vault-relative), or beside the board.
func canvasTarget(canvas, p string) string {
	p = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(p), "file://"), "file:")
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	for _, base := range []string{canvasRoot(canvas), filepath.Dir(canvas)} {
		c := filepath.Join(base, filepath.FromSlash(p))
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join(canvasRoot(canvas), filepath.FromSlash(p))
}

// The path a board writes for a file: relative to its org directory when
// the file is under it, else absolute.
func canvasFileField(canvas, abs string) string {
	root := canvasRoot(canvas)
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

var canvasDrawerRe = regexp.MustCompile(`^:[A-Za-z_-]+:$`)

var canvasImageRe = regexp.MustCompile(`(?i)\.(png|gif|jpe?g|svg|webp|avif|bmp)$`)

func canvasOrgFile(p string) *common.OrgFile {
	if f := loadedFileAt(p); f != nil {
		return f
	}
	return nil
}

// The heading a ref names, and the file it is in.
func canvasHeading(file *common.OrgFile, ref jcanvas.Ref) (*org.Section, *common.OrgFile) {
	db := GetDb()
	in := func(s *org.Section) (*org.Section, *common.OrgFile) {
		if s == nil {
			return nil, nil
		}
		f := db.FileFromSection(s)
		if f == nil {
			f = file
		}
		return s, f
	}
	switch {
	case ref.ID != "":
		return in(db.FindByAnyId(ref.ID))
	case ref.CustomID != "":
		return in(db.FindByAnyId(ref.CustomID))
	case ref.Line > 0 && file != nil:
		return findHeadingByLine(file, ref.Line-1), file
	case len(ref.Olp) > 0 && file != nil && file.Doc != nil:
		cur := file.Doc.Outline.Children
		var sec *org.Section
		for _, name := range ref.Olp {
			sec = nil
			for _, c := range cur {
				if strings.EqualFold(strings.TrimSpace(common.GetHeadlineTitle(c.Headline)), name) {
					sec = c
					break
				}
			}
			if sec == nil {
				break
			}
			cur = sec.Children
		}
		if sec == nil {
			sec = findHeadingByText(file, ref.Olp[len(ref.Olp)-1])
		}
		if sec != nil {
			return sec, file
		}
	}
	return nil, nil
}

// The heading's own text as plain lines: no planning, drawers or keywords,
// cut at the first child, at most 30 lines.
func canvasPlainBody(filename string, sec *org.Section) string {
	lines, from, to, ok := recordLines(filename, sec)
	if !ok {
		return ""
	}
	end := ownLinesEnd(lines, from, to)
	out := []string{}
	inDrawer := false
	for i := from + 1; i <= end && i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		switch {
		case inDrawer:
			if strings.EqualFold(t, ":END:") {
				inDrawer = false
			}
			continue
		case canvasDrawerRe.MatchString(t):
			inDrawer = true
			continue
		case planningRe.MatchString(lines[i]):
			continue
		}
		out = append(out, lines[i])
	}
	plain := jcanvas.PlainText(strings.Join(out, "\n"))
	if len(plain) > 30 {
		plain = append(plain[:30], "…")
	}
	return strings.Join(plain, "\n")
}

func renderNodes(doc *org.Document, nodes []org.Node) string {
	if exp := htmlExporter(); exp != nil {
		return exp.RenderCardFragmentIn(doc, nodes...)
	}
	w := org.NewHTMLWriter()
	org.WriteNodes(w, nodes...)
	return w.String()
}

func headingCard(sec *org.Section, f *common.OrgFile, html bool) jcanvas.Card {
	c := jcanvas.Card{Kind: "heading", Title: common.GetHeadlineTitle(sec.Headline), Hash: sec.Hash}
	if sec.Headline != nil {
		h := sec.Headline
		c.Keyword, c.Priority, c.Tags, c.Line = h.Status, h.Priority, h.Tags, h.Pos.Row
	}
	c.Olp = outlinePath(sec)
	if f != nil {
		c.Filename = f.Filename
		if c.Keyword != "" {
			done, _ := keywordSets(f)
			c.Done = done[c.Keyword]
		}
		c.Plain = canvasPlainBody(f.Filename, sec)
		if html && sec.Headline != nil {
			nodes := sec.Headline.Children
			for i, n := range sec.Headline.Children {
				if isHeadline(n) {
					nodes = sec.Headline.Children[:i]
					break
				}
			}
			c.Html = renderNodes(f.Doc, nodes)
		}
	}
	return c
}

func orgFileCard(f *common.OrgFile, html bool) jcanvas.Card {
	c := jcanvas.Card{Kind: "org", Filename: f.Filename, Title: strings.TrimSuffix(filepath.Base(f.Filename), ".org")}
	if f.Doc == nil {
		return c
	}
	if t := strings.TrimSpace(f.Doc.Get("TITLE")); t != "" {
		c.Title = t
	}
	if tags := strings.Trim(f.Doc.Get("FILETAGS"), ": "); tags != "" {
		c.Tags = strings.FieldsFunc(tags, func(r rune) bool { return r == ':' || r == ' ' })
	}
	heads := []string{}
	for _, s := range f.Doc.Outline.Children {
		heads = append(heads, "• "+common.GetHeadlineTitle(s.Headline))
		if len(heads) == 20 {
			heads = append(heads, "…")
			break
		}
	}
	c.Plain = strings.Join(heads, "\n")
	if html {
		pre := []org.Node{}
		for _, n := range f.Doc.Nodes {
			if isHeadline(n) {
				break
			}
			pre = append(pre, n)
		}
		out := renderNodes(f.Doc, pre)
		if len(heads) > 0 {
			out += "<ul class=\"canvas-outline\">"
			for _, h := range heads {
				out += "<li>" + htmlEscape(strings.TrimPrefix(h, "• ")) + "</li>"
			}
			out += "</ul>"
		}
		c.Html = out
	}
	return c
}

// canvasCard is what one node shows. html false skips rendering (the SVG,
// the CLI's outline and the link index need only titles).
func canvasCard(canvas string, n jcanvas.Node, html bool) (jcanvas.Card, bool) {
	switch n.Type {
	case "text":
		c := jcanvas.Card{Kind: "text", Plain: strings.Join(jcanvas.PlainText(n.Text), "\n")}
		if html {
			c.Html = renderCardText(canvas, n.Text)
		}
		return c, true
	case "link":
		return jcanvas.Card{Kind: "link", Title: jcanvas.Host(n.URL)}, true
	case "group":
		if n.Background == "" {
			return jcanvas.Card{}, false
		}
		abs := canvasTarget(canvas, n.Background)
		return jcanvas.Card{Kind: "group", Src: plugs.MediaURL(nil, Conf().PlugManager, "", abs)}, true
	case "file":
	default:
		return jcanvas.Card{}, false
	}
	ref := jcanvas.ParseRef(n.File, n.Subpath)
	abs := canvasTarget(canvas, ref.Path)
	if strings.EqualFold(filepath.Ext(abs), ".org") {
		f := canvasOrgFile(abs)
		if ref.IsHeading() {
			sec, sf := canvasHeading(f, ref)
			if sec == nil {
				msg := "no such heading in " + filepath.Base(abs)
				if f == nil {
					msg = "no org file " + ref.Path
				}
				return jcanvas.Card{Kind: "missing", Msg: msg}, true
			}
			c := headingCard(sec, sf, html)
			if sf != nil && (f == nil || !samePath(sf.Filename, f.Filename)) {
				c.Moved, c.File = true, canvasFileField(canvas, sf.Filename)
			}
			return c, true
		}
		if f == nil {
			return jcanvas.Card{Kind: "missing", Msg: "no org file " + ref.Path}, true
		}
		return orgFileCard(f, html), true
	}
	if _, err := os.Stat(abs); err != nil {
		return jcanvas.Card{Kind: "missing", Filename: abs, Msg: "no file " + ref.Path}, true
	}
	src := plugs.MediaURL(nil, Conf().PlugManager, "", abs)
	if strings.HasPrefix(src, "file:") {
		src = ""
	}
	kind := "file"
	if canvasImageRe.MatchString(abs) {
		kind = "image"
	}
	return jcanvas.Card{Kind: kind, Title: filepath.Base(abs), Filename: abs, Src: src}, true
}

func canvasCards(canvas string, c *jcanvas.Canvas, html bool) map[string]jcanvas.Card {
	out := map[string]jcanvas.Card{}
	for _, n := range c.Nodes {
		if card, ok := canvasCard(canvas, n, html); ok {
			out[n.ID] = card
		}
	}
	return out
}

var (
	mdSignRe  = regexp.MustCompile("(?m)^#{1,6} |\\*\\*[^*\\n]+\\*\\*|\\[[^\\]\\n]+\\]\\([^)\\n]+\\)|^```|\\[\\[[^\\]|\\n]+\\|[^\\]\\n]+\\]\\]|^> ")
	orgSignRe = regexp.MustCompile(`(?m)^\*+ |^#\+|\[\[[^\]\n]+\]\[[^\]\n]+\]\]`)
)

// CardDialect is how a text card is read: "markdown" when it has markdown's
// marks and none of org's, else "org".
func CardDialect(text string) string {
	if mdSignRe.MatchString(text) && !orgSignRe.MatchString(text) {
		return "markdown"
	}
	return "org"
}

var titleLineRe = regexp.MustCompile(`(?m)^#\+TITLE: (.*)$`)

func renderCardText(canvas, text string) string {
	src := text
	if CardDialect(text) == "markdown" {
		src = mdorg.ConvertText("card.md", text)
		// A note's single top heading becomes its title there; on a card it
		// is the card's heading.
		src = titleLineRe.ReplaceAllString(src, "* $1")
	}
	name := filepath.Join(filepath.Dir(canvas), "card.org")
	doc := GetConfig().Parse(strings.NewReader(src), name)
	return renderNodes(doc, doc.Nodes)
}

// ---------------------------------------------------------------------------
// Watching
// ---------------------------------------------------------------------------

var (
	canvasMu     sync.Mutex
	canvasTimers = map[string]*time.Timer{}
)

// canvasChanged is called for a board written here and for every watcher
// event on one: a `canvas` event (once per burst) and the link index
// rebuilt, since boards count as links to the headings on them.
func canvasChanged(p string) {
	if !featureOn("canvas") {
		return
	}
	canvasMu.Lock()
	defer canvasMu.Unlock()
	if t := canvasTimers[p]; t != nil {
		t.Reset(150 * time.Millisecond)
		return
	}
	canvasTimers[p] = time.AfterFunc(150*time.Millisecond, func() {
		canvasMu.Lock()
		delete(canvasTimers, p)
		canvasMu.Unlock()
		bumpCanvasIndex()
		Publish(Event{Kind: "canvas", File: p})
	})
}

// bumpCanvasIndex makes every cache keyed on ReloadIndex (the link index
// among them) build again.
func bumpCanvasIndex() {
	db := GetDb()
	db.dblock.Lock()
	db.ReloadIndex += 1
	db.dblock.Unlock()
}

// ---------------------------------------------------------------------------
// Backlinks: a board naming a heading links to it
// ---------------------------------------------------------------------------

// canvasLinks are the link index's links from boards: each file node naming
// an org file or a heading, From the board (its line: one node a line, as
// boards are written), Kind "canvas". Only with the feature on.
// canvasNodeLine is the line (from 0) of a board's json where the node with
// this id starts: the line holding its "id", or, when the file is written one
// key a line (`JSON.stringify(b, null, '\t')`), the `{` above
// it. 0 when it is not found.
func canvasNodeLine(lines []string, id string) int {
	key := regexp.MustCompile(`"id"\s*:\s*` + regexp.QuoteMeta(strconv.Quote(id)))
	for i, l := range lines {
		loc := key.FindStringIndex(l)
		if loc == nil {
			continue
		}
		if strings.Contains(l[:loc[0]], "{") {
			return i
		}
		for j := i - 1; j >= 0; j-- {
			if strings.HasSuffix(strings.TrimSpace(lines[j]), "{") {
				return j
			}
		}
		return i
	}
	return 0
}

func canvasLinks(db *OrgDb) []common.OrgLink {
	out := []common.OrgLink{}
	if !featureOn("canvas") {
		return out
	}
	for _, p := range canvasFiles() {
		c, raw, err := readCanvas(p)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")
		lineOf := func(id string) int { return canvasNodeLine(lines, id) }
		for _, n := range c.Nodes {
			if n.Type != "file" {
				continue
			}
			ref := jcanvas.ParseRef(n.File, n.Subpath)
			abs := canvasTarget(p, ref.Path)
			if !strings.EqualFold(filepath.Ext(abs), ".org") {
				continue
			}
			l := common.OrgLink{Raw: "file:" + n.File + n.Subpath, Kind: "canvas", From: common.LinkEnd{Filename: p, Line: lineOf(n.ID)}}
			f := canvasOrgFile(abs)
			if ref.IsHeading() {
				if sec, sf := canvasHeading(f, ref); sec != nil {
					l.To = endForSectionIn(db, sec, sf)
				} else {
					l.Broken = true
				}
			} else if f != nil {
				l.To = common.LinkEnd{Filename: f.Filename}
			} else {
				l.Broken = true
			}
			out = append(out, l)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The html exporter's [[file:board.canvas]]
// ---------------------------------------------------------------------------

func canvasEmbed(docPath, target string) (string, bool) {
	if !featureOn("canvas") {
		return "", false
	}
	if i := strings.Index(target, "::"); i >= 0 {
		target = target[:i]
	}
	target = strings.TrimPrefix(strings.TrimPrefix(target, "file://"), "file:")
	p := target
	if !filepath.IsAbs(p) {
		base := filepath.Dir(docPath)
		if docPath == "" && len(Conf().Server.OrgDirs) > 0 {
			base = Conf().Server.OrgDirs[0]
		}
		p = filepath.Join(base, filepath.FromSlash(p))
	}
	p, ok := insideOrgDirs(p)
	if !ok {
		return "", false
	}
	c, _, err := readCanvas(p)
	if err != nil {
		return "", false
	}
	sum := sha1.Sum([]byte(p))
	return jcanvas.SVG(&c, canvasCards(p, &c, false), jcanvas.SvgOpts{IDPrefix: "cv" + hex.EncodeToString(sum[:4]), MaxWidth: 900}), true
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

type canvasAnswer struct {
	Ok       bool
	Msg      string `json:",omitempty"`
	File     string `json:",omitempty"`
	Rel      string `json:",omitempty"`
	Version  string `json:",omitempty"`
	Conflict bool   `json:",omitempty"`
	// The document as it is on disk (unknown fields and all).
	Canvas   json.RawMessage         `json:",omitempty"`
	Cards    map[string]jcanvas.Card `json:",omitempty"`
	Problems []string                `json:",omitempty"`
	Warnings []string                `json:",omitempty"`
	Node     json.RawMessage         `json:",omitempty"`
	Edge     json.RawMessage         `json:",omitempty"`
}

func canvasFail(w http.ResponseWriter, msg string, args ...interface{}) {
	dynJson(w, canvasAnswer{Msg: fmt.Sprintf(msg, args...)})
}

func canvasBody(r *http.Request, v interface{}) error {
	body, _ := io.ReadAll(r.Body)
	return json.Unmarshal(body, v)
}

/* SDOC: API
* GET /canvas/list — Every Canvas Under The Org Directories

	#+BEGIN_EXAMPLE
	GET /canvas/list
	-> {"Ok": true, "Canvases": [{"File": "/notes/boards/plan.canvas", "Rel": "boards/plan.canvas",
	    "Name": "plan", "Nodes": 12, "Edges": 5, "Mod": "2026-11-04T10:12:00-08:00"}]}
	#+END_EXAMPLE

	Every =.canvas= file (JSON Canvas 1.0) under the org
	directories; folders starting with a dot (=.git=, =.trash=) are
	skipped. Needs =features.canvas=.
EDOC */
func RequestCanvasList(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	out := []CanvasInfo{}
	for _, p := range canvasFiles() {
		info := CanvasInfo{File: p, Rel: canvasRel(p), Name: strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))}
		if st, err := os.Stat(p); err == nil {
			info.Mod = st.ModTime().Format(time.RFC3339)
		}
		if c, _, err := readCanvas(p); err != nil {
			info.Msg = err.Error()
		} else {
			info.Nodes, info.Edges = len(c.Nodes), len(c.Edges)
		}
		out = append(out, info)
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Canvases": out})
}

/* SDOC: API
* GET /canvas — A Canvas, With Its Cards Read

	#+BEGIN_EXAMPLE
	GET /canvas?file=boards/plan.canvas[&html=f]
	-> {"Ok": true, "File": "/notes/boards/plan.canvas", "Rel": "boards/plan.canvas", "Version": "3fa2…",
	    "Canvas": {"nodes": [...], "edges": [...]},
	    "Cards": {"0ba6ef0e2a6b0a3d": {"Kind": "heading", "Title": "Plans", "Keyword": "TODO",
	              "Tags": ["work"], "Html": "<p>…</p>", "Plain": "…", "Hash": "…",
	              "Filename": "/notes/plans.org", "Line": 12, "Olp": ["Projects", "Plans"]}},
	    "Warnings": []}
	#+END_EXAMPLE

	=Canvas= is the file as it is on disk, every field kept. =Cards= says what each
	node shows: =heading= (an org heading - =subpath= =#^ID=, =#Heading=,
	=#Parent#Child=, or the file written =notes.org::*Heading= / =::#custom-id= /
	=::42=), =org= (a whole org file), =image=, =file=, =link=, =text= (its =Html=
	rendered as org, or converted from markdown when it reads as markdown),
	=group= (only with a background picture), =missing=. A heading found by its
	id in another file than the node names has =Moved= and the =File= it should
	name. =Version= goes back with =/canvas/save=. =html=f= renders nothing.
EDOC */
func RequestCanvas(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	p, err := canvasPath(r.URL.Query().Get("file"))
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	c, raw, err := readCanvas(p)
	if err != nil {
		canvasFail(w, "%s: %v", filepath.Base(p), err)
		return
	}
	_, warnings := c.Validate()
	doc := json.RawMessage(raw)
	if len(strings.TrimSpace(string(raw))) == 0 {
		doc = json.RawMessage(`{}`)
	}
	dynJson(w, canvasAnswer{Ok: true, File: p, Rel: canvasRel(p), Version: canvasVersion(raw), Canvas: doc,
		Cards: canvasCards(p, &c, r.URL.Query().Get("html") != "f"), Warnings: warnings})
}

/* SDOC: API
* GET /canvas/svg — A Canvas Drawn As A Picture

	#+BEGIN_EXAMPLE
	GET /canvas/svg?file=plan.canvas[&dark=t][&width=900]
	-> image/svg+xml
	#+END_EXAMPLE

	A static drawing of the board: groups, cards (headings with their keyword,
	tags and the start of their text; pictures; links; text), edges with arrows
	and labels. The same drawing an exported page shows for =[[file:plan.canvas]]=.
EDOC */
func RequestCanvasSvg(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	p, err := canvasPath(r.URL.Query().Get("file"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	c, _, err := readCanvas(p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	width, _ := strconv.ParseFloat(r.URL.Query().Get("width"), 64)
	dark := r.URL.Query().Get("dark")
	w.Header().Set("Content-Type", "image/svg+xml")
	io.WriteString(w, jcanvas.SVG(&c, canvasCards(p, &c, false), jcanvas.SvgOpts{IDPrefix: "cv", MaxWidth: width, Dark: dark == "t" || dark == "1"}))
}

/* SDOC: API
* POST /canvas/save — Write A Whole Canvas

	#+BEGIN_EXAMPLE
	POST /canvas/save {"File": "plan.canvas", "Canvas": {"nodes": [...], "edges": [...]},
	                   "Base": "3fa2…", "Force": false}
	-> {"Ok": true, "Version": "9b1c…", "Warnings": []}
	-> {"Ok": false, "Conflict": true, "Version": "77e0…", "Msg": "plan.canvas changed since you read it"}
	-> {"Ok": false, "Problems": ["edge \"e1\": toNode \"x\" is no node"], "Msg": "…"}
	#+END_EXAMPLE

	The whole document, checked against JSON Canvas 1.0 (ids unique, the fields
	each type needs, sides, ends, colours =1=-=6= or =#hex=, edges joining nodes
	that exist) and laid out as JSON Canvas apps write it, through a temporary file
	and a rename. Fields the spec does not name are kept. =Base= is the
	=Version= the client read: a file changed since (by another app, by another tab)
	is refused with =Conflict= unless =Force=. The file must exist (=/canvas/new=).
EDOC */
func PostCanvasSave(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	var req struct {
		File   string
		Canvas json.RawMessage
		Base   string
		Force  bool
	}
	if err := canvasBody(r, &req); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	p, err := canvasPath(req.File)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	c, err := jcanvas.Parse(req.Canvas)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	problems, warnings := c.Validate()
	if len(problems) > 0 {
		dynJson(w, canvasAnswer{File: p, Problems: problems, Msg: "not a valid canvas: " + strings.Join(problems, "; ")})
		return
	}
	canvasWriteMu.Lock()
	defer canvasWriteMu.Unlock()
	if old, err := os.ReadFile(p); err == nil && req.Base != "" && !req.Force {
		if v := canvasVersion(old); v != req.Base {
			dynJson(w, canvasAnswer{File: p, Conflict: true, Version: v, Msg: filepath.Base(p) + " changed since you read it"})
			return
		}
	}
	v, err := writeCanvas(p, &c)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	dynJson(w, canvasAnswer{Ok: true, File: p, Rel: canvasRel(p), Version: v, Warnings: warnings})
}

var canvasWriteMu sync.Mutex

/* SDOC: API
* POST /canvas/new — Make An Empty Canvas

	#+BEGIN_EXAMPLE
	POST /canvas/new {"File": "boards/plan"}     .canvas added; relative to the first org directory
	-> {"Ok": true, "File": "/notes/boards/plan.canvas", "Rel": "boards/plan.canvas", "Version": "…"}
	#+END_EXAMPLE

	Refused when the file is there already or outside the org directories.
	Folders are made.
EDOC */
func PostCanvasNew(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	var req struct{ File string }
	if err := canvasBody(r, &req); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	name := strings.TrimSpace(req.File)
	if name == "" {
		canvasFail(w, "a canvas needs a name")
		return
	}
	if !isCanvasFile(name) {
		name += ".canvas"
	}
	p, ok := insideOrgDirs(name)
	if !ok {
		canvasFail(w, "%s is not inside an org directory", name)
		return
	}
	if _, err := os.Stat(p); err == nil {
		canvasFail(w, "%s is there already", canvasRel(p))
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	f.Close()
	var c jcanvas.Canvas
	v, err := writeCanvas(p, &c)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	dynJson(w, canvasAnswer{Ok: true, File: p, Rel: canvasRel(p), Version: v})
}

// The node fields for a heading put on a board: its file and `#^<ID>`, the
// heading given an :ID: first when it has neither ID nor CUSTOM_ID.
func canvasHeadingRef(canvas, hash string) (string, string, *org.Section, *common.OrgFile, error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil {
		return "", "", nil, nil, fmt.Errorf("no heading with that hash")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil {
		return "", "", nil, nil, fmt.Errorf("the heading's file is not loaded")
	}
	lines, from, to, ok := recordLines(f.Filename, sec)
	if !ok {
		return "", "", nil, nil, fmt.Errorf("could not read the heading in %s", f.Filename)
	}
	props := headingPropsIn(lines, from, ownLinesEnd(lines, from, to))
	id := props["ID"]
	if id == "" && props["CUSTOM_ID"] != "" {
		id = props["CUSTOM_ID"]
	}
	if id == "" {
		// reviewWrite's drawer: in a file whose drawers are in column zero
		// the new one is too (ensureHeadingId would indent it by depth).
		id = uuid.New().String()
		if err := reviewWrite(f, sec, []reviewProp{{"ID", id}}, nil); err != nil {
			return "", "", nil, nil, err
		}
		// The id drawer moved lines: find the heading again by its id.
		if s := GetDb().FindByAnyId(id); s != nil {
			sec = s
			if nf := GetDb().FileFromSection(s); nf != nil {
				f = nf
			}
		}
	}
	return canvasFileField(canvas, f.Filename), jcanvas.SubpathFor(id), sec, f, nil
}

/* SDOC: API
* POST /canvas/ref — The Node Fields For A Heading

	#+BEGIN_EXAMPLE
	POST /canvas/ref {"File": "plan.canvas", "Hash": "…"}
	-> {"Ok": true, "Node": {"type": "file", "file": "notes/plans.org", "subpath": "#^5f0c…"},
	    "Cards": {"": {"Kind": "heading", ...}}}
	#+END_EXAMPLE

	What a client puts in a file node to show a heading (worg's search box, a
	heading dropped on the board). A heading with no =:ID:= (and no =:CUSTOM_ID:=)
	is given one - a line edit in its org file - so the card follows it when it
	is renamed or refiled. Writes nothing to the canvas.
EDOC */
func PostCanvasRef(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	var req struct{ File, Hash string }
	if err := canvasBody(r, &req); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	p, err := canvasPath(req.File)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	file, sub, sec, f, err := canvasHeadingRef(p, req.Hash)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	node, _ := json.Marshal(map[string]string{"type": "file", "file": file, "subpath": sub})
	dynJson(w, canvasAnswer{Ok: true, File: p, Node: node, Cards: map[string]jcanvas.Card{"": headingCard(sec, f, true)}})
}

// The default size of a new card of each kind (the usual JSON Canvas sizes, roughly).
func canvasSize(kind string) (float64, float64) {
	switch kind {
	case "text":
		return 260, 140
	case "link":
		return 400, 220
	case "group":
		return 640, 420
	}
	return 400, 300
}

// findCanvasNode is a node a person named: its id, a group's label, or the
// one card whose label holds every word (case ignored).
func findCanvasNode(canvas string, c *jcanvas.Canvas, name string) (int, error) {
	name = strings.TrimSpace(name)
	for i, n := range c.Nodes {
		if n.ID == name {
			return i, nil
		}
	}
	words := strings.Fields(strings.ToLower(name))
	if len(words) == 0 {
		return -1, fmt.Errorf("which node?")
	}
	cards := canvasCards(canvas, c, false)
	hits := []int{}
	exact := []int{}
	for i, n := range c.Nodes {
		var cp *jcanvas.Card
		if cd, ok := cards[n.ID]; ok {
			cp = &cd
		}
		label := strings.ToLower(jcanvas.Label(n, cp))
		if cp != nil && cp.Title != "" && strings.EqualFold(cp.Title, name) || label == strings.ToLower(name) {
			exact = append(exact, i)
		}
		all := true
		for _, w := range words {
			if !strings.Contains(label, w) && !strings.Contains(strings.ToLower(n.File+" "+n.URL), w) {
				all = false
				break
			}
		}
		if all {
			hits = append(hits, i)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) == 0 {
		return -1, fmt.Errorf("no node on %s matches %q", filepath.Base(canvas), name)
	}
	names := []string{}
	for _, i := range hits {
		var cp *jcanvas.Card
		if cd, ok := cards[c.Nodes[i].ID]; ok {
			cp = &cd
		}
		names = append(names, fmt.Sprintf("%s (%s)", jcanvas.Label(c.Nodes[i], cp), c.Nodes[i].ID))
	}
	return -1, fmt.Errorf("%q matches %d nodes: %s - name one by its id", name, len(hits), strings.Join(names, ", "))
}

/* SDOC: API
* POST /canvas/add — Put A Card On A Canvas

	#+BEGIN_EXAMPLE
	POST /canvas/add {"File": "plan.canvas", "Kind": "heading", "Hash": "…"}
	POST /canvas/add {"File": "plan.canvas", "Kind": "file", "Path": "pics/map.png"}
	POST /canvas/add {"File": "plan.canvas", "Kind": "link", "Url": "https://jsoncanvas.org"}
	POST /canvas/add {"File": "plan.canvas", "Kind": "text", "Text": "* Idea\nsomething"}
	POST /canvas/add {"File": "plan.canvas", "Kind": "group", "Label": "Later"}
	     optional: "In": "Later" (a group's label or id), "X", "Y", "Width", "Height", "Color": "4"
	-> {"Ok": true, "Node": {...the node written...}, "Version": "…"}
	#+END_EXAMPLE

	Places the card in the first free spot, reading rows from the top left of
	the board (or of the group =In= names), unless =X=/=Y= say where. A heading
	is given an =:ID:= when it has none (see =/canvas/ref=). A file =Path= is
	relative to the canvas's org directory, or absolute.
EDOC */
func PostCanvasAdd(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	var req struct {
		File, Kind, Hash, Path, Url, Text, Label, In, Color string
		X, Y, Width, Height                                 *float64
	}
	if err := canvasBody(r, &req); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	p, err := canvasPath(req.File)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	color, ok := jcanvas.NormColor(req.Color)
	if !ok {
		canvasFail(w, "colour %q is not 1-6, a name (red orange yellow green cyan purple) or #hex", req.Color)
		return
	}
	canvasWriteMu.Lock()
	defer canvasWriteMu.Unlock()
	c, _, err := readCanvas(p)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	kind := strings.ToLower(req.Kind)
	wd, ht := canvasSize(kind)
	var n jcanvas.Node
	switch kind {
	case "heading":
		file, sub, _, _, err := canvasHeadingRef(p, req.Hash)
		if err != nil {
			canvasFail(w, "%v", err)
			return
		}
		n = jcanvas.NewFile(file, sub, 0, 0, wd, ht)
	case "file":
		if strings.TrimSpace(req.Path) == "" {
			canvasFail(w, "which file? (Path)")
			return
		}
		abs := canvasTarget(p, req.Path)
		n = jcanvas.NewFile(canvasFileField(p, abs), "", 0, 0, wd, ht)
	case "link":
		if strings.TrimSpace(req.Url) == "" {
			canvasFail(w, "which url? (Url)")
			return
		}
		n = jcanvas.NewLink(strings.TrimSpace(req.Url), 0, 0, wd, ht)
	case "text":
		n = jcanvas.NewText(req.Text, 0, 0, wd, ht)
	case "group":
		n = jcanvas.NewGroup(req.Label, 0, 0, wd, ht)
	default:
		canvasFail(w, "Kind is heading, file, link, text or group")
		return
	}
	if req.Width != nil && *req.Width > 0 {
		n.Width = *req.Width
	}
	if req.Height != nil && *req.Height > 0 {
		n.Height = *req.Height
	}
	within := -1
	if req.In != "" {
		i, err := findCanvasNode(p, &c, req.In)
		if err != nil {
			canvasFail(w, "%v", err)
			return
		}
		if c.Nodes[i].Type != "group" {
			canvasFail(w, "%q is not a group", req.In)
			return
		}
		within = i
	}
	at := c.Place(n.Width, n.Height, within)
	n.X, n.Y = at.X, at.Y
	if req.X != nil {
		n.X = *req.X
	}
	if req.Y != nil {
		n.Y = *req.Y
	}
	n.ID, n.Color = c.UniqueID(), color
	if kind == "group" {
		// Under everything, as a group drawn last would hide its cards.
		c.Nodes = append([]jcanvas.Node{n}, c.Nodes...)
	} else {
		c.Nodes = append(c.Nodes, n)
	}
	v, err := writeCanvas(p, &c)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	one := jcanvas.Canvas{Nodes: []jcanvas.Node{n}}
	dynJson(w, canvasAnswer{Ok: true, File: p, Rel: canvasRel(p), Version: v, Node: canvasFirstOf(one.Marshal(), "nodes"),
		Cards: canvasCards(p, &one, false)})
}

// canvasFirstOf picks the first element of a list out of a marshalled document.
func canvasFirstOf(doc []byte, key string) json.RawMessage {
	var m map[string][]json.RawMessage
	if json.Unmarshal(doc, &m) == nil && len(m[key]) > 0 {
		return m[key][0]
	}
	return nil
}

/* SDOC: API
* POST /canvas/link — Draw An Edge Between Two Cards

	#+BEGIN_EXAMPLE
	POST /canvas/link {"File": "plan.canvas", "From": "Plans", "To": "8132d4d894c80022",
	                   "Label": "leads to", "Color": "2", "FromSide": "right", "ToSide": "left",
	                   "Ends": "->"}            -> (default), <->, -- (no arrows), <-
	-> {"Ok": true, "Edge": {...}, "Version": "…"}
	#+END_EXAMPLE

	=From= and =To= are a node's id, or words every one of which is in the card's
	title, text, file or url (one card must match; several is refused, naming
	them). Sides left out are worked out when drawing (the facing sides).
EDOC */
func PostCanvasLink(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	var req struct{ File, From, To, Label, Color, FromSide, ToSide, Ends string }
	if err := canvasBody(r, &req); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	p, err := canvasPath(req.File)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	color, ok := jcanvas.NormColor(req.Color)
	if !ok {
		canvasFail(w, "colour %q is not 1-6, a name or #hex", req.Color)
		return
	}
	canvasWriteMu.Lock()
	defer canvasWriteMu.Unlock()
	c, _, err := readCanvas(p)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	a, err := findCanvasNode(p, &c, req.From)
	if err != nil {
		canvasFail(w, "from: %v", err)
		return
	}
	b, err := findCanvasNode(p, &c, req.To)
	if err != nil {
		canvasFail(w, "to: %v", err)
		return
	}
	e := jcanvas.Edge{ID: c.UniqueID(), FromNode: c.Nodes[a].ID, ToNode: c.Nodes[b].ID, Label: req.Label, Color: color,
		FromSide: req.FromSide, ToSide: req.ToSide}
	switch strings.TrimSpace(req.Ends) {
	case "", "->":
	case "<->":
		e.FromEnd = "arrow"
	case "--":
		e.ToEnd = "none"
	case "<-":
		e.FromEnd, e.ToEnd = "arrow", "none"
	default:
		canvasFail(w, "Ends is ->, <->, <- or --")
		return
	}
	c.Edges = append(c.Edges, e)
	if problems, _ := c.Validate(); len(problems) > 0 {
		canvasFail(w, "%s", strings.Join(problems, "; "))
		return
	}
	v, err := writeCanvas(p, &c)
	if err != nil {
		canvasFail(w, "%v", err)
		return
	}
	one := jcanvas.Canvas{Edges: []jcanvas.Edge{e}}
	dynJson(w, canvasAnswer{Ok: true, File: p, Rel: canvasRel(p), Version: v, Edge: canvasFirstOf(one.Marshal(), "edges")})
}

/* SDOC: API
* POST /canvas/render — A Text Card's Text As Html

	#+BEGIN_EXAMPLE
	POST /canvas/render {"File": "plan.canvas", "Text": "# Idea\n**big**"}
	-> {"Ok": true, "Html": "<h1>…", "Dialect": "markdown"}
	#+END_EXAMPLE

	What a text card shows after an edit: org rendered by the html exporter, or
	markdown (when it reads as markdown and not org) converted to org first.
	Pictures are found beside the canvas.
EDOC */
func PostCanvasRender(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	var req struct{ File, Text string }
	if err := canvasBody(r, &req); err != nil {
		canvasFail(w, "%v", err)
		return
	}
	p, err := canvasPath(req.File)
	if err != nil && len(Conf().Server.OrgDirs) > 0 {
		p = filepath.Join(Conf().Server.OrgDirs[0], "card.canvas")
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Html": renderCardText(p, req.Text), "Dialect": CardDialect(req.Text)})
}

/* SDOC: API
* GET /canvas/for — The Canvases A Heading Is On

	#+BEGIN_EXAMPLE
	GET /canvas/for?hash=…
	-> {"Ok": true, "Canvases": [{"File": "/notes/plan.canvas", "Rel": "plan.canvas", "Node": "0ba6ef0e2a6b0a3d"}]}
	#+END_EXAMPLE
EDOC */
func RequestCanvasFor(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "canvas") {
		return
	}
	hash := r.URL.Query().Get("hash")
	type on struct{ File, Rel, Node string }
	out := []on{}
	for _, l := range canvasLinks(GetDb()) {
		if hash != "" && l.To.Hash == hash {
			node := ""
			if c, _, err := readCanvas(l.From.Filename); err == nil {
				for _, n := range c.Nodes {
					if "file:"+n.File+n.Subpath == l.Raw {
						node = n.ID
						break
					}
				}
			}
			out = append(out, on{l.From.Filename, canvasRel(l.From.Filename), node})
		}
	}
	dynJson(w, map[string]interface{}{"Ok": true, "Canvases": out})
}

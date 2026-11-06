package orgs

// Hover previews (features.hoverEditor): resting on a link shows what it points at, and the preview can
// be switched to an editor of that heading's own lines.
//
// Nothing is stored and nothing new is written into org files: a link is an
// ordinary org link ([[id:…]], [[*Heading]], [[file:x.org::*H]],
// [[file:x.org::#cid]], [[file:x.org]], a picture, a pdf, a web url, a
// protocol of the yaml's linkProtocols), resolved the way transclusion
// resolves one (resolveTransclusion), relative to the file it is written in.
//
// GET /preview answers the target's kind, title, rendered html and the org
// text with its file and rows, and Base, a hash of those rows and the line
// on either side. POST
// /preview/save splices new text over exactly those rows when they still hash
// to Base, else refuses: the same row-checked splice as /transclusion/save
// (previewSpliceRows), never a document rewrite (see Traps: line edits).
//
// A web url's preview is its page's title, description and picture from the
// <head> (OpenGraph, twitter: and plain meta tags), fetched here, cached, http
// and https only, with a timeout, a size cap and no private addresses unless
// hoverEditor.privateHosts says so. Nothing is fetched while the feature is
// off or hoverEditor.noWeb is set.
//
// While the feature is on the html exporter wraps every link in a
// <span class="org-link" data-org-link=… data-org-from=…> (html.HoverLinksOn),
// which is how worg knows a rendered link's text and the file it is in.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	htmlexp "github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
	"github.com/ihdavids/orgs/internal/common"
)

func registerPreviewRoutes(api *mux.Router) {
	api.HandleFunc("/preview", RequestPreview).Methods("GET")
	api.HandleFunc("/preview/save", PostPreviewSave).Methods("POST")
}

func init() {
	htmlexp.HoverLinksOn = func() bool { return featureOn("hoverEditor") }
}

func previewSettings() common.HoverEditorSettings {
	if s := Conf().Server; s != nil {
		return s.HoverEditor
	}
	return common.HoverEditorSettings{}
}

// Preview is what a link points at, as a hover card shows it.
type Preview struct {
	Ok   bool
	Msg  string
	Off  bool `json:",omitempty"`
	Link string
	// heading, file, text (lines of an org file), source (a file that is not
	// org), image, pdf, media, web, command
	Kind  string
	Title string
	Todo  string   `json:",omitempty"`
	Tags  []string `json:",omitempty"`
	// The headings above it, outermost first.
	Path []string `json:",omitempty"`
	Html string
	// The org text and where it is: rows From..To (0 based) of File. Base is
	// what /preview/save checks those rows against.
	Text     string
	File     string
	From     int
	To       int
	Line     int // From+1, for an editor
	Base     string
	Editable bool
	// The text was cut short to fit a preview, so it is not offered for
	// editing.
	Truncated bool   `json:",omitempty"`
	Hash      string `json:",omitempty"`
	Lang      string `json:",omitempty"`
	// image, pdf, media and web: where the browser fetches it. web also has
	// the page's own description, picture and site name.
	Url         string `json:",omitempty"`
	Description string `json:",omitempty"`
	Image       string `json:",omitempty"`
	Site        string `json:",omitempty"`
	CommandLine string `json:",omitempty"`
}

// previewBase is the hash a save is checked against: rows a..b of the file
// and the line on either side of them. The neighbours matter: a save that
// added a line at the end of the heading leaves rows a..b reading as they
// did, and a second save from a card still holding the old rows would then
// leave that line behind under the next heading.
func previewBase(lines []string, a, b int) string {
	h := sha256.New()
	if a > 0 {
		h.Write([]byte(lines[a-1]))
	}
	h.Write([]byte{0})
	h.Write([]byte(strings.Join(lines[a:b+1], "\n")))
	h.Write([]byte{0})
	if b+1 < len(lines) {
		h.Write([]byte(lines[b+1]))
	} else {
		h.Write([]byte{1}) // the end of the file
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

const (
	previewMaxRows  = 400 // a whole file or a heading longer than this is shown cut short
	previewLineRows = 40  // ::42 and ::text show this many lines from there
)

var (
	previewImageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true, ".avif": true, ".bmp": true}
	previewMediaExt = map[string]bool{".mp4": true, ".webm": true, ".mov": true, ".m4v": true, ".mp3": true, ".m4a": true, ".ogg": true, ".wav": true, ".flac": true, ".opus": true}
	previewBareRe   = regexp.MustCompile(`^\[\[((?:[^\]\\]|\\.)+)\](?:\[([^\]]*)\])?\]$`)
)

// previewTarget is a link without its brackets and description.
func previewTarget(link string) string {
	link = strings.TrimSpace(link)
	if m := previewBareRe.FindStringSubmatch(link); m != nil {
		link = strings.ReplaceAll(m[1], `\]`, "]")
	}
	return strings.TrimSpace(link)
}

// previewFile is the file part of a file-ish link and whether it is one.
func previewFile(target string) (string, bool) {
	low := strings.ToLower(target)
	switch {
	case strings.HasPrefix(low, "file:"):
		f, _, _ := strings.Cut(strings.TrimPrefix(target[5:], "//"), "::")
		return f, true
	case strings.HasPrefix(target, "./"), strings.HasPrefix(target, "../"), strings.HasPrefix(target, "/"), strings.HasPrefix(target, "~/"):
		f, _, _ := strings.Cut(target, "::")
		return f, true
	}
	return "", false
}

// previewFrom is the file a link is read relative to: from as given, or a
// file name the database knows, or a place in the first org directory.
func previewFrom(from string) string {
	from = strings.TrimSpace(from)
	if from != "" {
		if f := GetDb().FindByFile(from); f != nil {
			return f.Filename
		}
		if filepath.IsAbs(from) {
			return filepath.Clean(from)
		}
	}
	if s := Conf().Server; s != nil && len(s.OrgDirs) > 0 {
		root, _ := filepath.Abs(s.OrgDirs[0])
		if from != "" {
			return filepath.Join(root, from)
		}
		return filepath.Join(root, ".")
	}
	return from
}

// previewHeadingPath is the titles of the headings a row is under.
func previewHeadingPath(lines []string, row int) []string {
	out := []string{}
	lvl := txLevel(lines[row])
	if lvl == 0 {
		lvl = 1 << 30
	}
	for i := row - 1; i >= 0 && lvl > 1; i-- {
		if n := txLevel(lines[i]); n > 0 && n < lvl {
			out = append([]string{txTitle(lines[i])}, out...)
			lvl = n
		}
	}
	return out
}

// previewRender is org text as the exporter renders it, relative to path.
func previewRender(text, path string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	doc := GetConfig().Silent().Parse(strings.NewReader(text+"\n"), path)
	if exp := htmlExporter(); exp != nil {
		return exp.RenderFragmentIn(doc, doc.Nodes...)
	}
	w := org.NewHTMLWriter()
	w.Document = doc
	org.WriteNodes(w, doc.Nodes...)
	return w.String()
}

func previewFileTitle(path string, lines []string) string {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if len(t) > 8 && strings.EqualFold(t[:8], "#+title:") {
			if v := strings.TrimSpace(t[8:]); v != "" {
				return v
			}
		}
		if t != "" && !strings.HasPrefix(t, "#+") && !strings.HasPrefix(t, ":") {
			break
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// LinkPreview resolves a link written in the file from and says what it
// points at.
func LinkPreview(link, from string) Preview {
	target := previewTarget(link)
	p := Preview{Link: target}
	if target == "" {
		p.Msg = "no link"
		return p
	}
	from = previewFrom(from)
	low := strings.ToLower(target)

	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return previewWeb(target)
	}
	// A protocol of the yaml's own: a url is previewed as that url, a
	// command is said, never run.
	if res, ok := userLinkProtocol(target); ok {
		if res.Url != "" {
			wp := previewWeb(res.Url)
			wp.Link = target
			return wp
		}
		p.Ok, p.Kind, p.Title, p.CommandLine = true, "command", target, res.CommandLine
		return p
	}
	if strings.HasPrefix(low, "attachment:") {
		p.Msg = "attachment links are previewed in the page they are in"
		return p
	}
	// Pictures, pdfs and recordings: a url the browser can fetch, never
	// their bytes read as lines.
	if f, ok := previewFile(target); ok {
		ext := strings.ToLower(filepath.Ext(f))
		kind := ""
		switch {
		case previewImageExt[ext]:
			kind = "image"
		case ext == ".pdf":
			kind = "pdf"
		case previewMediaExt[ext]:
			kind = "media"
		}
		if kind != "" {
			r := resolveOne(target, from)
			if r.Url == "" {
				p.Msg = r.Msg
				return p
			}
			p.Ok, p.Kind, p.Url, p.Title = true, kind, r.Url, filepath.Base(f)
			return p
		}
	}
	// [[Some heading]]: org's fuzzy link, a heading of this file.
	if !strings.Contains(target, ":") && !strings.HasPrefix(target, "*") && !strings.HasPrefix(target, "#") && !strings.HasPrefix(target, ".") && !strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "~") {
		target = "*" + target
	}
	path, lines, a, b, err := resolveTransclusion(target, from)
	if err != nil {
		p.Msg = err.Error()
		return p
	}
	// As the save reads them: without the empty string after the last newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
		b = min(b, len(lines)-1)
	}
	if a < 0 || a >= len(lines) {
		p.Ok, p.Kind, p.File, p.Title = true, "file", path, previewFileTitle(path, lines)
		return p
	}
	isOrg := strings.EqualFold(filepath.Ext(path), ".org")
	p.File, p.From, p.To, p.Line = path, a, b, a+1
	_, search, _ := strings.Cut(strings.TrimPrefix(target, "file:"), "::")
	switch {
	case !isOrg:
		p.Kind, p.Title, p.Lang = "source", filepath.Base(path), strings.TrimPrefix(filepath.Ext(path), ".")
		if strings.ContainsRune(strings.Join(lines[a:min(b+1, a+previewLineRows*2)], "\n"), 0) {
			p.Msg = filepath.Base(path) + " is not text"
			return Preview{Link: p.Link, Msg: p.Msg}
		}
	case txLevel(lines[a]) > 0 && (strings.HasPrefix(low, "id:") || strings.HasPrefix(search, "*") || strings.HasPrefix(search, "#") || strings.HasPrefix(target, "*") || strings.HasPrefix(target, "#")):
		p.Kind = "heading"
		p.Title = txTitle(lines[a])
		p.Path = previewHeadingPath(lines, a)
		if f := GetDb().FindByFile(path); f != nil {
			if sec := sectionAtRow(f, a); sec != nil && sec.Headline != nil && headlineRow(sec) == a {
				p.Title = common.GetHeadlineTitle(sec.Headline)
				p.Todo = sec.Headline.Status
				p.Tags = sec.Headline.Tags
				p.Hash = sec.Hash
			}
		}
	case search != "":
		p.Kind, p.Title = "text", fmt.Sprintf("%s, line %d", filepath.Base(path), a+1)
		if b > a+previewLineRows-1 {
			b = a + previewLineRows - 1
			p.To = b
		}
	default:
		p.Kind, p.Title = "file", previewFileTitle(path, lines)
	}
	if b-a+1 > previewMaxRows {
		b = a + previewMaxRows - 1
		p.To, p.Truncated = b, true
	}
	p.Text = strings.Join(lines[a:b+1], "\n")
	p.Base = previewBase(lines, a, b)
	p.Editable = isOrg && !p.Truncated && GetDb().FindByFile(path) != nil
	if isOrg {
		body := lines[a : b+1]
		if p.Kind == "heading" {
			// The title is the card's own; the body is what is under it.
			body = lines[min(txHeadingEnd(lines, a), b+1) : b+1]
		}
		p.Html = previewRender(strings.Join(body, "\n"), path)
	}
	p.Ok = true
	return p
}

/* SDOC: API
* GET /preview — What A Link Points At (hover previews)

	For features.hoverEditor. Resolves a link as it is written in an org file
	and answers what it points at, for a hover card: a heading ([[id:…]],
	[[*Title]], [[Title]], [[file:x.org::*Title]], [[file:x.org::#custom-id]]),
	a whole file ([[file:x.org]]), lines of one ([[file:x.org::42]]), a
	picture, a pdf, a recording, a web page or a =linkProtocols= link.

	*Query Parameters:*
	| Parameter | Required | Description                                          |
	|-----------+----------+------------------------------------------------------|
	| =link=    | yes      | The link, with or without its brackets and description |
	| =from=    | no       | The org file it is written in; relative links and =*Title= are read against it |

	*Response:* a =Preview=:
	| Field      | Description                                                       |
	|------------+-------------------------------------------------------------------|
	| =Kind=     | heading, file, text, source, image, pdf, media, web, command      |
	| =Title=, =Todo=, =Tags=, =Path= | The heading's title, keyword, tags and the headings above it |
	| =Html=     | The text rendered by the html exporter (a heading without its own line, planning and drawer) |
	| =Text=, =File=, =From=, =To= | The org text and the rows (0 based) it is in            |
	| =Base=     | A hash of =Text= and its neighbouring lines, which =/preview/save= checks |
	| =Editable= | Whether =/preview/save= will take it                              |
	| =Hash=     | The heading's hash, for the other endpoints                       |
	| =Url=      | Where a picture, pdf, recording or page is fetched                |
	| =Description=, =Image=, =Site= | A web page's own, from its meta tags          |

	Refused while the feature is off.
EDOC */
func RequestPreview(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "hoverEditor") {
		return
	}
	q := r.URL.Query()
	dynJson(w, LinkPreview(q.Get("link"), q.Get("from")))
}

// PreviewSave is an edit made in a hover card.
type PreviewSave struct {
	File string
	From int
	To   int
	Base string
	Text string
}

// previewSpliceRows replaces rows from..to (0 based) of an org file the
// database has with text, only if same says they are still what the edit was
// made from. The rows are all that changes. Shared by /transclusion/save and
// /preview/save.
func previewSpliceRows(file string, from, to int, same func(lines []string) bool, text string) (common.ResultMsg, int, []string) {
	f := GetDb().FindByFile(file)
	if f == nil || f.Filename != filepath.Clean(file) {
		return common.ResultMsg{Msg: "that is not an org file this server has"}, 0, nil
	}
	lines, err := readFileLines(f.Filename)
	if err != nil {
		return common.ResultMsg{Msg: err.Error()}, 0, nil
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if from < 0 || to < from || to >= len(lines) || !same(lines) {
		return common.ResultMsg{Msg: filepath.Base(f.Filename) + " has changed since this was read: reload and edit again"}, 0, nil
	}
	repl := strings.Split(strings.TrimRight(text, "\n"), "\n")
	out := append(append(append([]string{}, lines[:from]...), repl...), lines[to+1:]...)
	if err := writeLines(f.Filename, out); err != nil {
		return common.ResultMsg{Msg: err.Error()}, 0, nil
	}
	return common.ResultMsg{Ok: true, Msg: fmt.Sprintf("%s: %d lines", filepath.Base(f.Filename), len(repl))}, len(repl), out
}

// PreviewSaved says where the text is now, so the card can save again.
type PreviewSaved struct {
	Ok   bool
	Msg  string
	File string
	From int
	To   int
	Base string
	Text string
}

func SavePreview(req *PreviewSave) PreviewSaved {
	text := strings.TrimRight(strings.ReplaceAll(req.Text, "\r\n", "\n"), "\n")
	res, n, lines := previewSpliceRows(req.File, req.From, req.To, func(lines []string) bool { return previewBase(lines, req.From, req.To) == req.Base }, text)
	out := PreviewSaved{Ok: res.Ok, Msg: res.Msg}
	if res.Ok {
		out.File, out.From, out.To, out.Text = filepath.Clean(req.File), req.From, req.From+n-1, text
		out.Base = previewBase(lines, out.From, out.To)
	}
	return out
}

/* SDOC: API
* POST /preview/save — Save An Edit Made In A Hover Preview

	#+BEGIN_EXAMPLE
	{ "File": "/org/projects.org", "From": 12, "To": 30, "Base": "…", "Text": "* Heading\nnew text" }
	#+END_EXAMPLE

	Replaces rows =From=..=To= (0 based, as =GET /preview= gave them) of an
	org file the server has with =Text=, only if those rows and the line on
	either side of them still hash to =Base=; otherwise nothing is written and =Msg= says the file has
	changed. Only those rows change. Answers ={Ok, Msg, File, From, To, Base,
	Text}=: where the text is now, so the next save can be checked too.
	Refused while features.hoverEditor is off.
EDOC */
func PostPreviewSave(w http.ResponseWriter, r *http.Request) {
	if featureOff(w, "hoverEditor") {
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	var req PreviewSave
	if err := json.Unmarshal(body, &req); err != nil {
		dynJson(w, PreviewSaved{Msg: err.Error()})
		return
	}
	dynJson(w, SavePreview(&req))
}

// ---------------------------------------------------------------------------
// Web pages
// ---------------------------------------------------------------------------

const (
	previewWebCap    = 1 << 20 // the <head> is in the first megabyte or not worth waiting for
	previewWebTTL    = 6 * time.Hour
	previewWebErrTTL = 10 * time.Minute
	previewWebMax    = 500
)

type previewWebCached struct {
	at time.Time
	p  Preview
}

var (
	previewWebMu sync.Mutex
	previewWebCache   = map[string]previewWebCached{}
)

// previewDialer refuses addresses on this machine and its network, unless
// the yaml allows them: a preview is the server fetching a url somebody
// wrote, and that must not be a way to reach what only the server can.
func previewClient() *http.Client {
	sets := previewSettings()
	timeout := 8 * time.Second
	if d, err := time.ParseDuration(sets.Timeout); err == nil && d > 0 {
		timeout = d
	}
	dialer := &net.Dialer{Timeout: timeout}
	if !sets.PrivateHosts {
		dialer.Control = func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip != nil && previewPrivateIP(ip) {
				return fmt.Errorf("%s is a private address (hoverEditor.privateHosts allows it)", host)
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: timeout},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirected to a %s url", req.URL.Scheme)
			}
			return nil
		},
	}
}

func previewPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast()
}

func previewUserAgent() string {
	if ua := previewSettings().UserAgent; ua != "" {
		return ua
	}
	return "orgs/" + common.Version + " (link preview; +https://github.com/ihdavids/orgs)"
}

func previewWeb(raw string) Preview {
	p := Preview{Link: raw, Kind: "web", Url: raw, Title: raw}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p.Msg = "only http and https pages are previewed"
		return p
	}
	p.Site = u.Hostname()
	if previewSettings().NoWeb {
		p.Ok = true
		return p
	}
	previewWebMu.Lock()
	if c, ok := previewWebCache[raw]; ok {
		ttl := previewWebTTL
		if !c.p.Ok {
			ttl = previewWebErrTTL
		}
		if time.Since(c.at) < ttl {
			previewWebMu.Unlock()
			return c.p
		}
	}
	previewWebMu.Unlock()

	got := previewFetchWeb(p, u)
	previewWebMu.Lock()
	if len(previewWebCache) >= previewWebMax {
		for k, c := range previewWebCache { // drop the oldest
			old := k
			for k2, c2 := range previewWebCache {
				if c2.at.Before(c.at) {
					old, c = k2, c2
				}
			}
			delete(previewWebCache, old)
			break
		}
	}
	previewWebCache[raw] = previewWebCached{at: time.Now(), p: got}
	previewWebMu.Unlock()
	return got
}

func previewFetchWeb(p Preview, u *url.URL) Preview {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		p.Msg = err.Error()
		return p
	}
	req.Header.Set("User-Agent", previewUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	resp, err := previewClient().Do(req)
	if err != nil {
		// Still a card: the url and its host, which is all a page that cannot
		// be reached has to say.
		p.Ok, p.Msg = true, "could not reach "+u.Hostname()
		return p
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		p.Ok, p.Msg = true, fmt.Sprintf("%s answered %s", resp.Request.URL.Hostname(), resp.Status)
		return p
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.HasPrefix(ct, "image/") {
		p.Ok, p.Kind = true, "image"
		return p
	}
	if ct != "" && !strings.Contains(ct, "html") {
		p.Ok, p.Msg = true, ct
		return p
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, previewWebCap))
	if i := strings.Index(strings.ToLower(string(b)), "</head>"); i > 0 {
		b = b[:i]
	}
	meta := previewPageMeta(string(b), resp.Request.URL)
	p.Ok = true
	if meta.title != "" {
		p.Title = meta.title
	}
	p.Description, p.Image = meta.description, meta.image
	if meta.site != "" {
		p.Site = meta.site
	}
	return p
}

type previewMetaInfo struct{ title, description, image, site string }

// previewPageMeta reads a page's own description of itself out of its <head>:
// OpenGraph first, then twitter:, then the plain title and description.
func previewPageMeta(page string, at *url.URL) previewMetaInfo {
	props := map[string]string{}
	for _, t := range metaTag.FindAllString(page, -1) {
		a := tagAttrs(t)
		key := strings.ToLower(a["property"])
		if key == "" {
			key = strings.ToLower(a["name"])
		}
		if key != "" && a["content"] != "" {
			if _, seen := props[key]; !seen {
				props[key] = strings.TrimSpace(html.UnescapeString(a["content"]))
			}
		}
	}
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := props[k]; v != "" {
				return v
			}
		}
		return ""
	}
	out := previewMetaInfo{
		title:       first("og:title", "twitter:title"),
		description: first("og:description", "twitter:description", "description"),
		image:       first("og:image:secure_url", "og:image", "og:image:url", "twitter:image", "twitter:image:src"),
		site:        first("og:site_name", "application-name"),
	}
	if out.title == "" {
		if m := titleTag.FindStringSubmatch(page); m != nil {
			out.title = strings.Join(strings.Fields(html.UnescapeString(m[1])), " ")
		}
	}
	if len([]rune(out.description)) > 400 {
		out.description = string([]rune(out.description)[:400]) + "…"
	}
	if out.image != "" && at != nil {
		if iu, err := at.Parse(out.image); err == nil && (iu.Scheme == "http" || iu.Scheme == "https") {
			out.image = iu.String()
		} else {
			out.image = ""
		}
	}
	return out
}

// For tests: forget every page.
func previewClearWeb() {
	previewWebMu.Lock()
	previewWebCache = map[string]previewWebCached{}
	previewWebMu.Unlock()
}


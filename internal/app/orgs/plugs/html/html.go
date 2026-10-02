// EXPORTER: HTML Export

package htmlexp

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/op/go-logging.v1"
)

/* SDOC: Exporters

* Html
  The html exporter has the ability to take an entire file
  and export it as an html document using one of several templates of your choosing

  To enable the plugin you should add the following to your orgs.yaml file.


	#+BEGIN_SRC yaml
  - name: "html"
    props:
      fontfamily: "Underdog"
	#+END_SRC

  Several properties are available to you in the regular operation of the exporter.
  these include:

  - fontfamily - This can be used to control the default font. At the moment the default
    template uses google fonts as a source of fonts for your exported html document.
  - title - The default title to use on your document if none is provided by the org document
  - stylesheet - The default stylesheet name to use for styling your output html
  - hljscdn - The default cdn link to use for highlight js (the default styling mechanism for source code blocks)
  - hljsstyle - The default font style to use for highlight js blocks
  - wordcloud - If true includes a wordcloud block in your template
  - theme - The default theme template to use (html_default.tpl) (also default style in abscense of stylesheet default_style.css)

	Once enabled html export requests will first generate an html representation of your document, which in turn will expand
	the theme template (tpl file) with that html and apply the stylesheet into that template where requested.
	The result is your rendered html as requested.

** Default Style
	The default style is pretty vanilla. It is a simple rendering of your html with very few bells and whistles.

** Docs Style
	When you set the HTML_THEME to docs the docs theme is selected.
	This html template has a treeview for jumping around in your node tree
	and a search bar that can facilitate searching through all the generated text.

	#+BEGIN_SRC org
	   #+HTML_THEME: docs
	#+END_SRC



EDOC */

type OrgHtmlExporter struct {
	TemplatePath     string
	Props            map[string]interface{}
	StatusColors     map[string]string
	ExtendedHeadline func(*OrgHtmlWriter, org.Headline)
	out              *logging.Logger
	pm               *common.PluginManager
}

type OrgHeadingNode struct {
	Id       string
	Parent   string
	Name     string
	Children []OrgHeadingNode
	Lvl      int
}

type OrgHtmlWriter struct {
	*org.HTMLWriter
	Exp              *OrgHtmlExporter
	PostWriteScripts string
	Opts             string
	Nodes            []OrgHeadingNode
	isClosed         map[string]bool

	// The document being written.
	//
	// Not `w.Document`, which go-org's own `Before` would set and which this
	// exporter never calls - so it is the empty default, and its Path is "".
	// Setting it properly would be the better fix and is not this change's to
	// make: it would also switch on `:noexport:` exclusion and `#+LINK:`
	// expansion, which would quietly change every page anybody already
	// exports. This is the narrow thing attachment links need: a file to
	// resolve a relative `:DIR:` against.
	SrcDoc *org.Document
	// The heading currently being written, for `[[attachment:...]]`.
	//
	// An attachment link says which file rather than where it is, so resolving
	// one needs the heading that owns it - and a link node carries no way back
	// to its own heading. Recorded on the way past instead, which is exact:
	// every link inside a heading is written between that heading's start and
	// the next one's.
	inHeadline *org.Headline
}

var docStart = `
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}">  
<style>
{{.stylesheet | css}}
</style>
</head>
<body>
`
var docEnd = `
</body>
</html>
`

func MakeWriter() OrgHtmlWriter {
	return OrgHtmlWriter{org.NewHTMLWriter(), nil, "", "", []OrgHeadingNode{}, map[string]bool{}, nil, nil}
}

func NewOrgHtmlWriter(exp *OrgHtmlExporter) *OrgHtmlWriter {
	// This lovely bit of circular reference ensures that we get called when exporting for any methods we have overwritten
	rw := MakeWriter()
	//rw := OrgHtmlWriter{org.NewHTMLWriter(), nil, "", ""}
	rw.ExtendingWriter = &rw
	rw.Exp = exp

	// This we should probably just replace with an override as well! Way better
	rw.NoWrapCodeBlock = true
	cnt := 1
	rw.HighlightCodeBlock = func(keywords []org.Keyword, source, lang string, inline bool, params map[string]string) string {
		var attribs []string = []string{}
		for _, key := range keywords {
			// This does something strange! I don't understand why it centers the text and puts a red box around it
			if key.Key == "HTML_LINES" {
				attribs = append(attribs, fmt.Sprintf("%s=\"%s\"", "data-line-numbers", key.Value))
			}
		}
		attribStr := ""
		if len(attribs) > 0 {
			attribStr = strings.Join(attribs, " ")
		}
		if lang == "mermaid" {
			return fmt.Sprintf(`<pre class="mermaid">%s</pre>`, html.EscapeString(source))
		} else if lang == "wordcloud" {
			rw.Exp.Props["wordcloud"] = true
			rv := fmt.Sprintf(`<svg style="border: 1px dashed; border-radius: 10px; border-color: #333333" id="wordcloud_%d" onload="wordcloud('#wordcloud_%d', %s)"/>`, cnt, cnt, strings.TrimSpace(source))
			cnt += 1
			return rv
		} else {
			// The block says what language it is in; highlight.js is told.
			// Without the class it guesses from the text, and a short block
			// of json guesses css as readily as json - so every page of
			// configuration was coloured as something it was not.
			if lang != "" {
				attribStr = strings.TrimSpace(attribStr + " class=\"language-" + html.EscapeString(lang) + "\"")
			}
			if inline {
				return fmt.Sprintf("<pre><code %s>%s</code></pre>", attribStr, html.EscapeString(source))
			}
			return fmt.Sprintf("<pre><code %s>%s</code></pre>", attribStr, html.EscapeString(source))
		}
	}
	return &rw
}

func GetProp(name, revealName string, h org.Headline, secProps string) string {
	tran := h.Doc.Get(name)
	if tmp, ok := h.Properties.Get(name); ok {
		tran = tmp
	}
	if tran != "" {
		secProps = fmt.Sprintf("%s %s=\"%s\"", secProps, revealName, tran)
	}
	return secProps
}

func GetPropTag(name, revealName string, h org.Headline, secProps string) string {
	tran := h.Doc.Get(name)
	if tmp, ok := h.Properties.Get(name); ok {
		tran = tmp
	}
	if tran != "" && tran != "false" && tran != "off" && tran != "f" {
		secProps = fmt.Sprintf("%s %s", secProps, revealName)
	}
	return secProps
}

// ---------------------------------------------------------------------------
// Audio a heading points at
// ---------------------------------------------------------------------------
//
// A heading can name a recording in one of its properties - `:AUDIO:` is the
// one orgs writes itself, from a voice note - and the exported page gives it a
// player rather than a line of text nobody can listen to. Any property whose
// value points at a file with an audio extension gets one, so a heading that
// says `:INTERVIEW: [[file:takes/mira.wav]]` works without anything here
// knowing what an interview is.
//
// Nothing is fetched until it is asked for (`preload="none"`): a file with
// forty voice notes in it should cost one page, not forty recordings.

// The extensions a browser will take, and what to tell it they are. Firefox in
// particular will refuse a source whose type it cannot guess from the url, and
// a recorder's file may have no extension a server knows.
var audioTypes = map[string]string{
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".opus": "audio/ogg",
	".webm": "audio/webm",
	".weba": "audio/webm",
	".m4a":  "audio/mp4",
	".mp4a": "audio/mp4",
	".aac":  "audio/aac",
	".flac": "audio/flac",
}

// The target a property value points at, whatever shape it was written in:
// an org link with or without a description, a bare `file:` link, or a plain
// path. Anything that is not one of those comes back empty.
func LinkTarget(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "[[") {
		end := strings.Index(v, "]]")
		if end < 0 {
			return ""
		}
		v = v[2:end]
		// `[[target][description]]` - the description is not the target.
		if i := strings.Index(v, "]["); i >= 0 {
			v = v[:i]
		}
	}
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "file://")
	v = strings.TrimPrefix(v, "file:")
	return strings.TrimSpace(v)
}

// The audio type of a target, or "" when it is not one.
func AudioType(target string) string {
	i := strings.IndexAny(target, "?#")
	if i >= 0 {
		target = target[:i]
	}
	return audioTypes[strings.ToLower(filepath.Ext(target))]
}

// Where a browser can fetch a file the org file points at.
//
// A link is written relative to the org file that holds it - that is what
// makes moving the whole org directory keep every note's audio - while the
// file server is rooted at the org directory, so the two have to be put back
// together here. A link that resolves to nothing under the org file is tried
// again against the root, because plenty of org files are written that way.
//
// The three shapes are the ones WriteRegularLink already uses for images: an
// exported page on disk wants file:// urls, a vscode webview wants localhost,
// and a page served by this server wants a path of its own so that it works
// whatever host and port it was reached on.
// `[[attachment:report.pdf]]` turned into a path, or the target unchanged when
// it is not an attachment link.
//
// Resolved here, in the exporter, because an attachment link is the one link
// type whose meaning depends on the heading it sits in: the same
// `attachment:report.pdf` under two headings is two different files. The
// arithmetic itself is `internal/common`, so the server and every exporter
// reach the same folder.
func (w *OrgHtmlWriter) attachTarget(target string) string {
	name, ok := common.AttachLinkName(target)
	if !ok {
		return target
	}
	if w.inHeadline == nil {
		return target
	}
	orgFile := ""
	if w.SrcDoc != nil {
		orgFile = w.SrcDoc.Path
	} else if w.Document != nil {
		orgFile = w.Document.Path
	}
	dirs := []string{}
	set := common.AttachSettings{}
	if w.Exp != nil && w.Exp.pm != nil {
		dirs = w.Exp.pm.OrgDirs
		set = w.Exp.pm.Attach
	}
	dir, from, _ := common.AttachDirFrom(
		headlineProp(w.inHeadline, "DIR", "ATTACH_DIR"),
		headlineProp(w.inHeadline, "ID"),
		orgFile, dirs, set)
	if dir == "" || from == "" {
		// The heading owns no folder, so the link names nothing. Left as it
		// was rather than turned into a path that does not exist: a broken
		// link that still says `attachment:` is one somebody can act on.
		return target
	}
	return filepath.Join(dir, filepath.FromSlash(name))
}

// One property off a headline, by any of the names it might be written under.
func headlineProp(h *org.Headline, names ...string) string {
	if h == nil || h.Properties == nil {
		return ""
	}
	for _, p := range h.Properties.Properties {
		for _, n := range names {
			if strings.EqualFold(p[0], n) {
				return p[1]
			}
		}
	}
	return ""
}

var imageNameRe = regexp.MustCompile(`(?i)[.](png|gif|jpe?g|svg|tiff?|webp|avif|bmp|ico)$`)

// Is this attachment a picture? Its name is the only thing to go on, which is
// the same answer the rest of the media handling gives.
func isImageName(name string) bool {
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	return imageNameRe.MatchString(name)
}

func (w *OrgHtmlWriter) MediaSrc(doc *org.Document, target string) string {
	if target == "" {
		return ""
	}
	// An attachment is named relative to the heading's own attachment folder,
	// which only this writer knows how to find; after that the sum is the same
	// one every exporter needs and lives in plugs.MediaURL. It used to live
	// here, and the presentation exporters each had their own worse version of
	// it.
	return plugs.MediaURL(doc, w.Exp.pm, w.Opts, w.attachTarget(target))
}

// The players for one heading, in the order the properties were written. The
// property's own name is the label unless it is the obvious one, because
// `:AUDIO:` over a player says nothing that the player does not.
func (w *OrgHtmlWriter) WriteAudio(h org.Headline) {
	if h.Properties == nil {
		return
	}
	for _, kv := range h.Properties.Properties {
		if len(kv) < 2 {
			continue
		}
		name := strings.ToUpper(strings.TrimSpace(kv[0]))
		target := LinkTarget(kv[1])
		mime := AudioType(target)
		if mime == "" {
			continue
		}
		src := w.MediaSrc(h.Doc, target)
		if src == "" {
			continue
		}
		label := ""
		if name != "AUDIO" {
			label = fmt.Sprintf(`<span class="org-audio-label">%s</span>`, html.EscapeString(name))
		}
		w.WriteString(fmt.Sprintf(
			`<div class="org-audio">%s<audio class="org-audio-player" controls preload="none">`+
				`<source src="%s" type="%s"/>`+
				`<a href="%s">%s</a>`+
				`</audio><a class="org-audio-file" href="%s" title="%s">%s</a></div>`,
			label,
			html.EscapeString(src), mime,
			html.EscapeString(src), html.EscapeString(target),
			html.EscapeString(src), html.EscapeString(target),
			html.EscapeString(filepath.Base(target)),
		))
	}
}

func (w *OrgHtmlWriter) WriteRegularLink(l org.RegularLink) {
	// `[[attachment:report.pdf]]` - a file the heading owns. Written as a
	// picture when it is one and as a link when it is not, which is what org
	// does with it, and resolved against the heading rather than against the
	// file: the same name under two headings is two different files.
	if name, ok := common.AttachLinkName(l.URL); ok {
		src := w.MediaSrc(w.Document, l.URL)
		if src == "" {
			// The heading owns no folder, or the file is somewhere nothing can
			// serve. Say the name rather than writing a link to nowhere.
			w.WriteString(html.EscapeString(name))
			return
		}
		description := name
		if l.Description != nil {
			description = org.String(l.Description...)
		}
		if isImageName(name) {
			w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" title="%s" style="width: 70%%; height: 70%%;"/>`,
				src, html.EscapeString(description), html.EscapeString(name)))
			return
		}
		w.WriteString(fmt.Sprintf(`<a href="%s" class="attachment" download="%s">%s</a>`,
			src, html.EscapeString(name), html.EscapeString(description)))
		return
	}
	if l.Protocol == "file" && l.Kind() == "image" {

		// This bit is tricky: VSCode will not work with anything not setup as accessible in the webroot
		// Since a vscode webview is a seperate entity self signed certificates also do not work.
		// So we support localhost access over http to fix that. It's not ideal but works.

		// `file://path` and `file:path` are both written, and org itself puts
		// the shorter one in a link somebody typed. Chopping a fixed seven
		// characters eats the first two of the path on every one of those -
		// which is how `file:images/x.png` became `ages/x.png`.
		url := LinkTarget(l.URL)
		//fname, _ := filepath.Abs(url)

		//fname = "file://" + fname
		//fname := "/Users/idavids/dev/gtd/" + url
		fname := ""
		if strings.Contains(w.Opts, "httpslinks;") {
			fname = url
			fname = fmt.Sprintf("https://localhost:%d/images/%s", w.Exp.pm.TLSPort, fname)
		} else if strings.Contains(w.Opts, "filelinks;") {
			found := false
			for _, path := range w.Exp.pm.OrgDirs {
				fname = filepath.Join(path, url)
				fname, _ = filepath.Abs(fname)
				if _, err := os.Stat(fname); err != nil {
					fname = "file://" + fname
					found = true
					break
				}
			}
			if !found {
				if len(w.Exp.pm.OrgDirs) > 0 {
					path := w.Exp.pm.OrgDirs[0]
					fname = filepath.Join(path, url)
					fname, _ = filepath.Abs(fname)
					fname = "file://" + fname
				}
			}
		} else if strings.Contains(w.Opts, "httplinks;") {
			fname = url
			fname = fmt.Sprintf("http://localhost:%d/images/%s", w.Exp.pm.Port, fname)
		} else {
			// A page this server rendered and is about to serve: a path of its
			// own works whatever host and port it was reached on, and a
			// `http://localhost` one does not work at all on the phone that
			// scanned the QR code. The three explicit opts above are
			// unchanged - they are for a file on disk and for vscode.
			if src := w.MediaSrc(w.Document, url); src != "" {
				fname = src
			} else {
				fname = "/images/" + url
			}
		}
		if l.Description == nil {
			w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" title="%s" style="width: 70%%; height: 70%%;"/>`, fname, fname, url))
		} else {
			description := strings.TrimPrefix(org.String(l.Description...), "file:")
			w.WriteString(fmt.Sprintf(`<a href="%s"><img src="%s" alt="%s" /></a>`, l.URL, fname, description))
		}
	} else {
		// A protocol of the user's that maps to a url is written as that url,
		// so the page can be followed by anything. One that maps to a command
		// is left as written: only a client that can run things follows it.
		if res, ok := common.ResolveLinkProtocol(w.Exp.pm.LinkProtocols, l.URL); ok && res.Url != "" {
			if l.Description == nil {
				l.Description = []org.Node{org.Text{Content: l.URL}}
			}
			l.URL = res.Url
			l.Protocol, _ = common.SplitLinkProtocol(res.Url)
		}
		w.HTMLWriter.WriteRegularLink(l)
	}
}

func HeadlineAloneHasTag(name string, h *org.Headline) bool {
	if h != nil {
		for _, t := range h.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" && (t == name) {
				return true
			}
		}
	}
	return false
}

func (w *OrgHtmlWriter) FindParent(h org.Headline) *OrgHeadingNode {
	if h.Lvl <= 1 {
		return nil
	}
	if len(w.Nodes) > 0 {
		top := &w.Nodes[len(w.Nodes)-1]
		for top.Lvl != (h.Lvl-1) && len(top.Children) > 0 {
			top = &top.Children[len(top.Children)-1]
		}
		if top.Lvl < h.Lvl {
			return top
		}
	}
	return nil
}

func (w *OrgHtmlWriter) ShouldCloseById(id string) bool {
	if _, ok := w.isClosed[id]; ok {
		return false
	}
	w.isClosed[id] = true
	return true
}

func (w *OrgHtmlWriter) ShouldClose(n *OrgHeadingNode) bool {
	return w.ShouldCloseById(n.Id)
}

// OVERRIDE: This overrides the core method
func (w *OrgHtmlWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	// Whose attachments any `[[attachment:...]]` below belongs to, until the
	// next heading takes over. Put back on the way out rather than cleared, so
	// that a nested heading does not leave its parent's links unresolvable.
	was := w.inHeadline
	w.inHeadline = &h
	defer func() { w.inHeadline = was }()
	if w.Exp.ExtendedHeadline != nil {
		w.Exp.ExtendedHeadline(w, h)
		return
	}
	//secProps := ""
	//secProps = GetProp("REVEAL_TRANSITION", "data-transition", h, secProps)
	//w.WriteString(fmt.Sprintf(`<section %s>`, secProps))

	id := uuid.New().String()
	parent := w.FindParent(h)
	if parent != nil && w.ShouldClose(parent) {
		w.WriteString("</div>")
	}
	w.WriteString(fmt.Sprintf("<div id=\"%s\" class=\"heading-wrapper\">", id))
	w.WriteString(fmt.Sprintf("<div id=\"%s-title\" class=\"heading-title-wrapper title-level-%d\">", id, h.Lvl+1))
	w.WriteString(fmt.Sprintf("<h%d id=\"%s-heading\"><span id=\"%s-heading-start\"></span>", h.Lvl+1, id, id))

	// This is not good enough, we add a span with the status if requested, but this is
	// Kind of lame
	if w.Exp.Props["showstatus"] == true {
		statColor := ""
		if col, ok := w.Exp.StatusColors[h.Status]; ok {
			statColor = fmt.Sprintf("style=\"color:%s;\"", col)
		}
		w.WriteString(fmt.Sprintf("<span class=\"status\" %s> %s </span> ", statColor, h.Status))
	}

	// Write out our title but we need this for our node heirarchy
	title := w.WriteNodesAsString(h.Title...)
	w.WriteString(title)

	addChild := false
	if len(w.Nodes) > 0 {
		if parent != nil {
			addChild = true
			parent.Children = append(parent.Children, OrgHeadingNode{Id: id, Parent: parent.Id, Name: title, Children: []OrgHeadingNode{}, Lvl: h.Lvl})
		}
	}

	if !addChild {
		w.Nodes = append(w.Nodes, OrgHeadingNode{Id: id, Parent: "", Name: title, Children: []OrgHeadingNode{}, Lvl: h.Lvl})
	}

	w.WriteString(fmt.Sprintf("<span id=\"%s-heading-end\"></span></h%d>", id, h.Lvl+1))
	w.WriteString("</div>")
	w.WriteString(fmt.Sprintf("<div id=\"%s-content\" class=\"heading-content-wrapper content-level-%d\">", id, h.Lvl+1))
	w.WriteString(fmt.Sprintf("<div id=\"%s-text\" class=\"heading-content-text\">", id))

	// Before the text: a recording is what the heading is about when it has
	// one, and a player below three paragraphs is a player nobody finds.
	w.WriteAudio(h)

	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}

	if w.ShouldCloseById(id) {
		w.WriteString("</div>")
	}
	w.WriteString("</div>")
	w.WriteString("</div>")

	//w.WriteString("</section>\n")
}

func (w *OrgHtmlWriter) WriteTable(t org.Table) {
	w.HTMLWriter.WriteTable(t)
}

var funcMap template.FuncMap = template.FuncMap{
	"attr": func(s string) template.HTMLAttr {
		return template.HTMLAttr(s)
	},
	"safe": func(s string) template.HTML {
		return template.HTML(s)
	},
	"css": func(s string) template.CSS {
		return template.CSS(s)
	},
	"jsstr": func(s string) template.JSStr {
		return template.JSStr(s)
	},
	"js": func(s string) template.JS {
		return template.JS(s)
	},
	"url": func(s string) template.URL {
		return template.URL(s)
	},
}

func (self *OrgHtmlExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *OrgHtmlExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	fmt.Fprintf(os.Stderr, "HTML: Export called [%s] -> [%s] [%s]\n", query, to, opts)
	_, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
		log.Printf(msg)
		return fmt.Errorf(msg)
	}
	return nil
}

/*
	func ExpandTemplateIntoBuf(o *bytes.Buffer, temp string, m map[string]interface{}) {
		t := template.Must(template.New("").Funcs(funcMap).Parse(temp))
		err := t.Execute(o, m)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TEMPLATE ERROR: %s\n", err.Error())
		}
	}
*/
func (self *OrgHtmlExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	fmt.Fprintf(os.Stderr, "PPPP: %v\n", self.Props)
	self.Props = ValidateMap(self.Props)
	fmt.Fprintf(os.Stderr, "HTML: Export string called [%s]:[%s]\n", query, opts)

	defer func() { //catch or finally
		if err := recover(); err != nil { //catch
			fmt.Fprintf(os.Stderr, "Exception: %v\n", err)
			os.Exit(1)
		}
	}()

	if f := db.FindByFile(query); f != nil {
		fmt.Fprintf(os.Stderr, "File found\n")
		// The template is rendered from self.Props, so writing the title into
		// the caller's props map was writing it where nothing reads it: every
		// page came out titled "Schedule", which is ValidateMap's default and
		// a leftover from the agenda. A file that names no #+TITLE: is named
		// after itself rather than after the agenda - it is what the browser
		// tab says, and what the documentation themes put at the head of the
		// rail. Set on every export, not only when the file names one: the
		// exporter is shared between requests and would otherwise keep the
		// last page's title.
		title := f.Get("TITLE")
		if title == "" {
			name := f.Path
			if name == "" {
				name = query
			}
			title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
		}
		if title != "" {
			props["title"] = title
			self.Props["title"] = title
		}
		// Org's own keyword for the line under the title. The read-the-docs
		// theme puts it where that theme puts a version, which is the thing
		// a reader looks for first on a page of documentation; a theme with
		// nothing to do with it writes nothing, so a file that does not say
		// one is not a blank line anywhere.
		self.Props["subtitle"] = f.Get("SUBTITLE")
		theme := f.Get("HTML_THEME")
		// This overrides the theme if present
		style := f.Get("HTML_STYLE")
		// A caller can ask for a theme of its own, which wins over what the file
		// asks for. The worg files view uses this to render every file in the
		// theme the reader picked rather than the one the author chose.
		if req := props["theme"]; req != "" {
			theme = req
			style = ""
		}
		fontfamily := f.Get("HTML_FONTFAMILY")
		if fontfamily == "" {
			fontfamily = self.Props["fontfamily"].(string)
		}
		// The exporter is shared between requests, so the stylesheet is resolved
		// every time rather than only when a file names one - otherwise a file
		// with no theme keeps whatever the previous export left behind.
		name := "default"
		if theme != "" {
			name = theme
		}
		if style != "" {
			name = style
		}
		self.Props["stylesheet"] = GetStylesheet(name, fontfamily)
		attr := f.Get("ATTR_BODY_HTML")
		self.Props["havebodyattr"] = false
		if attr != "" {
			self.Props["bodyattr"] = attr
			self.Props["havebodyattr"] = true
		}
		self.Props["showstatus"] = false
		if f.Get("HTML_STATUS") != "" {
			self.Props["showstatus"] = true
		}

		// The key the template reads is hljs_style; writing hljsstyle here
		// set a key nothing uses *and* stopped ValidateMap from filling in
		// the default, so a file naming its own highlight style came out
		// with no stylesheet at all. hljs_style_default is how a theme can
		// tell "nobody asked" from "this one was asked for" - the docs
		// theme uses it to follow the reader's light or dark setting.
		hlstyle := f.Get("HTML_HIGHLIGHT_STYLE")
		self.Props["hljs_style_default"] = hlstyle == ""
		if hlstyle != "" {
			self.Props["hljs_style"] = hlstyle
		} else {
			self.Props["hljs_style"] = defaultHljsStyle
		}
		w := NewOrgHtmlWriter(self)
		w.Opts = opts
		w.SrcDoc = f
		fmt.Fprintf(os.Stderr, "Writing nodes...\n")
		org.WriteNodes(w, f.Nodes...)
		fmt.Fprintf(os.Stderr, "Done writing nodes...\n")
		res := w.String()
		self.Props["html_data"] = res
		self.Props["post_scripts"] = w.PostWriteScripts
		nodestr, _ := json.Marshal(w.Nodes)
		self.Props["nodes_json"] = string(nodestr)

		fmt.Fprintf(os.Stderr, "DOC START: ========================================\n")
		templatePath := GetTemplate(self.TemplatePath, theme)
		fmt.Fprintf(os.Stderr, "TEMPLATE: %s\n", templatePath)
		res = self.pm.Tempo.RenderTemplate(templatePath, self.Props)
		fmt.Fprintf(os.Stderr, "XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Fprintf(os.Stderr, "Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
}

func (self *OrgHtmlExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	if len(self.StatusColors) == 0 {
		self.StatusColors = map[string]string{
			"TODO":        "red",
			"INPROGRESS":  "#CC9900",
			"IN-PROGRESS": "#CC9900",
			"DOING":       "#CC9900",
			"DONE":        "#006600",
			"PAUSED":      "#dc7633",
			"BLOCKED":     "#c0392b",
			"WAITING":     "#76448a",
			"CANCELED":    "#909497",
			"CANCELLED":   "#909497",
		}
	}
	self.out = manager.Out
	self.pm = manager
}

func NewHtmlExp() *OrgHtmlExporter {
	var g *OrgHtmlExporter = new(OrgHtmlExporter)
	return g
}

var hljsver = "11.9.0"
var defaultHljsStyle = "monokai"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

// The player's own styling, appended to whatever stylesheet the page uses.
//
// Kept here rather than in the twelve theme files because it is the same on
// all of them: it borrows the page's own colour and says everything else in
// size and spacing, so a theme added later gets it without being told. The
// native control is left alone - a hand-built transport is a lot of code to
// end up worse at being a play button - and only asked to follow the reader's
// light or dark setting.
const audioStyles = `
.org-audio { display: flex; align-items: center; gap: 0.6em; flex-wrap: wrap; margin: 0.5em 0 0.9em; }
.org-audio-player { height: 34px; flex: 1 1 260px; max-width: 460px; color-scheme: light dark; }
.org-audio-label { font-size: 0.72em; letter-spacing: 0.08em; text-transform: uppercase; opacity: 0.6; }
.org-audio-file { font-size: 0.75em; opacity: 0.55; text-decoration: none; color: inherit; word-break: break-all; }
.org-audio-file:hover { opacity: 1; text-decoration: underline; }
`

func GetStylesheet(name string, fontfamily string) string {
	if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_styles/" + name + "_style.css")); err == nil {
		// HACK: We probably do not alway want to do this. Need to think of a better way to handle this!
		re := regexp.MustCompile(`url\(([^)]+)\)`)
		ff := regexp.MustCompile(`[{][{]fontfamily[}][}]`)

		return ff.ReplaceAllString(re.ReplaceAllString(string(data), "url(http://localhost:8010/${1})"), fontfamily) + audioStyles
	}
	// An unknown theme name still has to produce a styled page.
	if name != "default" {
		return GetStylesheet("default", fontfamily)
	}
	return ""
}

// HtmlThemes lists the themes the html exporter can render with: every
// "<name>_style.css" under the template folder's html_styles directory.
func HtmlThemes() []string {
	dir := plugs.PlugExpandTemplatePath("html_styles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{"default"}
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, "_style.css") {
			continue
		}
		names = append(names, strings.TrimSuffix(n, "_style.css"))
	}
	sort.Strings(names)
	return names
}

func GetTemplate(defaultTemplate string, theme string) string {
	themeTemplate := "html_" + theme + ".tpl"
	themeTemplatePath := plugs.PlugExpandTemplatePath(themeTemplate)
	if _, err := os.Stat(themeTemplatePath); err == nil {
		return themeTemplate
	}
	return defaultTemplate
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	force_reload_style := false
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["stylesheet"]; !ok || force_reload_style {
		m["stylesheet"] = GetStylesheet("default", m["fontfamily"].(string))
	}
	if _, ok := m["hljscdn"]; !ok {
		m["hljs_cdn"] = hljscdn
	}
	if _, ok := m["hljs_style"]; !ok {
		m["hljs_style"] = defaultHljsStyle
	}
	if _, ok := m["hljs_style_default"]; !ok {
		m["hljs_style_default"] = true
	}
	if _, ok := m["subtitle"]; !ok {
		m["subtitle"] = ""
	}
	if _, ok := m["wordcloud"]; !ok {
		m["wordcloud"] = false
	}
	if _, ok := m["theme"]; !ok {
		m["theme"] = "default"
	}
	return m
}

// init function is called at boot
func init() {
	common.AddExporter("html", func() common.Exporter {
		return &OrgHtmlExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "html_default.tpl"}
	})
}

// RenderFragment renders a handful of nodes the way an exported page renders
// them and hands back the html on its own - no document, no template, no
// stylesheet.
//
// It exists for the callers that show one heading rather than a file: worg's
// inspect popup and the kanban cards. Going through this writer rather than
// go-org's plain one is what makes a picture in a heading resolve to a url this
// server actually serves and a recording come back as a player, since those are
// this writer's overrides and not go-org's.
//
// The exporter is shared between requests and this reads its props, so it must
// not be called while an export is writing its own.
func (self *OrgHtmlExporter) RenderFragment(nodes ...org.Node) string {
	self.Props = ValidateMap(self.Props)
	w := NewOrgHtmlWriter(self)
	org.WriteNodes(w, nodes...)
	return w.String()
}

// RenderFragmentIn is RenderFragment for nodes parsed out of a document of
// their own - a flashcard's question, say - so that a picture named relative to
// the org file is found relative to that file and not to the org root.
func (self *OrgHtmlExporter) RenderFragmentIn(doc *org.Document, nodes ...org.Node) string {
	self.Props = ValidateMap(self.Props)
	w := NewOrgHtmlWriter(self)
	if doc != nil {
		w.Document = doc
	}
	org.WriteNodes(w, nodes...)
	return w.String()
}

// ThemeStyle is the stylesheet a named theme renders with, for a caller showing
// a fragment that has no document of its own to hang a <head> on. An empty name
// is the default theme, and a name no theme answers to falls back to it too.
func (self *OrgHtmlExporter) ThemeStyle(theme string) string {
	self.Props = ValidateMap(self.Props)
	name := theme
	if name == "" {
		name = "default"
	}
	fontfamily, _ := self.Props["fontfamily"].(string)
	return GetStylesheet(name, fontfamily)
}

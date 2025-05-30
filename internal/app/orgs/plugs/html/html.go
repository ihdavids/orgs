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
	"strings"

	"github.com/google/uuid"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
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
	pm               *plugs.PluginManager
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
	return OrgHtmlWriter{org.NewHTMLWriter(), nil, "", "", []OrgHeadingNode{}, map[string]bool{} }
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
			//attribStr = "class=\"lanugage-" + lang + "\""
			if inline {
				return fmt.Sprintf("<pre><code %s >%s</code></pre>", attribStr, html.EscapeString(source))
			}
			return fmt.Sprintf("<pre><code %s >%s</code></pre>", attribStr, html.EscapeString(source))
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
func (w *OrgHtmlWriter) WriteRegularLink(l org.RegularLink) {
	if l.Protocol == "file" && l.Kind() == "image" {

		// This bit is tricky: VSCode will not work with anything not setup as accessible in the webroot
		// Since a vscode webview is a seperate entity self signed certificates also do not work.
		// So we support localhost access over http to fix that. It's not ideal but works.

		url := l.URL[len("file://"):]
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
		} else { //if strings.Contains(w.Opts, "httplinks;") {
			fname = url
			fname = fmt.Sprintf("http://localhost:%d/images/%s", w.Exp.pm.Port, fname)
		}
		if l.Description == nil {
			w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" title="%s" style="width: 70%%; height: 70%%;"/>`, fname, fname, url))
		} else {
			description := strings.TrimPrefix(org.String(l.Description...), "file:")
			w.WriteString(fmt.Sprintf(`<a href="%s"><img src="%s" alt="%s" /></a>`, l.URL, fname, description))
		}
	} else {
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
	if _,ok := w.isClosed[id]; ok {
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
	if w.Exp.ExtendedHeadline != nil {
		w.Exp.ExtendedHeadline(w, h)
		return
	}
	//secProps := ""
	//secProps = GetProp("REVEAL_TRANSITION", "data-transition", h, secProps)
	//w.WriteString(fmt.Sprintf(`<section %s>`, secProps))

	id := uuid.New().String()
	parent := w.FindParent(h)
	if (parent != nil && w.ShouldClose(parent)) {
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

	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}

	if (w.ShouldCloseById(id)) {
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

func (self *OrgHtmlExporter) Export(db plugs.ODb, query string, to string, opts string, props map[string]string) error {
	fmt.Printf("HTML: Export called", query, to, opts)
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
			fmt.Printf("TEMPLATE ERROR: %s\n", err.Error())
		}
	}
*/
func (self *OrgHtmlExporter) ExportToString(db plugs.ODb, query string, opts string, props map[string]string) (error, string) {
	fmt.Printf("PPPP: %v\n", self.Props)
	self.Props = ValidateMap(self.Props)
	fmt.Printf("HTML: Export string called [%s]:[%s]\n", query, opts)

	defer func() { //catch or finally
		if err := recover(); err != nil { //catch
			fmt.Fprintf(os.Stderr, "Exception: %v\n", err)
			os.Exit(1)
		}
	}()

	if f := db.FindByFile(query); f != nil {
		fmt.Printf("File found\n")
		title := f.Get("TITLE")
		if title != "" {
			props["title"] = title
		}
		theme := f.Get("HTML_THEME")
		fontfamily := f.Get("HTML_FONTFAMILY")
		if fontfamily == "" {
			fontfamily = self.Props["fontfamily"].(string)
		}
		if theme != "" {
			self.Props["stylesheet"] = GetStylesheet(theme, fontfamily)
		}
		// This overrides the theme if present
		style := f.Get("HTML_STYLE")
		if style != "" {
			self.Props["stylesheet"] = GetStylesheet(style, fontfamily)
		}
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

		hlstyle := f.Get("HTML_HIGHLIGHT_STYLE")
		if hlstyle != "" {
			self.Props["hljsstyle"] = hlstyle
		}
		w := NewOrgHtmlWriter(self)
		w.Opts = opts
		fmt.Printf("Writing nodes...\n")
		org.WriteNodes(w, f.Nodes...)
		fmt.Printf("Done writing nodes...\n")
		res := w.String()
		self.Props["html_data"] = res
		self.Props["post_scripts"] = w.PostWriteScripts
		nodestr, _ := json.Marshal(w.Nodes)
		self.Props["nodes_json"] = string(nodestr)

		fmt.Printf("DOC START: ========================================\n")
		templatePath := GetTemplate(self.TemplatePath, theme)
		fmt.Printf("TEMPLATE: %s\n", templatePath)
		res = self.pm.Tempo.RenderTemplate(templatePath, self.Props)
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
}

func (self *OrgHtmlExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	if len(self.StatusColors) == 0 {
		self.StatusColors = map[string]string{
			"TODO":        "#3498db",
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
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

func GetStylesheet(name string, fontfamily string) string {
	if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_styles/" + name + "_style.css")); err == nil {
		// HACK: We probably do not alway want to do this. Need to think of a better way to handle this!
		re := regexp.MustCompile(`url\(([^)]+)\)`)
		ff := regexp.MustCompile(`[{][{]fontfamily[}][}]`)

		return ff.ReplaceAllString(re.ReplaceAllString(string(data), "url(http://localhost:8010/${1})"), fontfamily)
	}
	return ""
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
	if _, ok := m["hljsstyle"]; !ok {
		m["hljs_style"] = "monokai"
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
	plugs.AddExporter("html", func() plugs.Exporter {
		return &OrgHtmlExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "html_default.tpl"}
	})
}

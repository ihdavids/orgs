// EXPORTER: HTML Export

package gantt

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"gopkg.in/op/go-logging.v1"
)

type OrgHtmlExporter struct {
	TemplatePath string
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *plugs.PluginManager
}

type OrgHtmlWriter struct {
	*org.HTMLWriter
	exp              *OrgHtmlExporter
	PostWriteScripts string
	Opts             string
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

func NewOrgHtmlWriter(exp *OrgHtmlExporter) *OrgHtmlWriter {
	// This lovely bit of circular reference ensures that we get called when exporting for any methods we have overwritten
	rw := OrgHtmlWriter{org.NewHTMLWriter(), nil, "", ""}
	rw.ExtendingWriter = &rw
	rw.exp = exp

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
			rw.exp.Props["wordcloud"] = true
			rv := fmt.Sprintf(`<svg style="border: 1px dashed; border-radius: 10px; border-color: #333333" id="wordcloud_%d" onload="wordcloud('#wordcloud_%d', %s)"/>`, cnt, cnt, strings.TrimSpace(source))
			cnt += 1
			return rv
		} else {
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
			fname = fmt.Sprintf("https://localhost:%d/images/%s", w.exp.pm.TLSPort, fname)
		} else if strings.Contains(w.Opts, "filelinks;") {
			found := false
			for _, path := range w.exp.pm.OrgDirs {
				fname = filepath.Join(path, url)
				fname, _ = filepath.Abs(fname)
				if _, err := os.Stat(fname); err != nil {
					fname = "file://" + fname
					found = true
					break
				}
			}
			if !found {
				if len(w.exp.pm.OrgDirs) > 0 {
					path := w.exp.pm.OrgDirs[0]
					fname = filepath.Join(path, url)
					fname, _ = filepath.Abs(fname)
					fname = "file://" + fname
				}
			}
		} else { //if strings.Contains(w.Opts, "httplinks;") {
			fname = url
			fname = fmt.Sprintf("http://localhost:%d/images/%s", w.exp.pm.Port, fname)
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

// OVERRIDE: This overrides the core method
func (w *OrgHtmlWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	//secProps := ""
	//secProps = GetProp("REVEAL_TRANSITION", "data-transition", h, secProps)
	//w.WriteString(fmt.Sprintf(`<section %s>`, secProps))

	w.WriteString(fmt.Sprintf("<h%d>", h.Lvl+1))
	org.WriteNodes(w, h.Title...)
	w.WriteString(fmt.Sprintf("</h%d>", h.Lvl+1))

	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
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

func (self *OrgHtmlExporter) Export(db plugs.ODb, query string, to string, opts string) error {
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
func (self *OrgHtmlExporter) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Printf("HTML: Export string called [%s]:[%s]\n", query, opts)

	if f := db.FindByFile(query); f != nil {
		theme := f.Get("HTML_THEME")
		if theme != "" {
			self.Props["stylesheet"] = GetStylesheet(theme)
		}
		theme = f.Get("HTML_STYLE")
		if theme != "" {
			self.Props["stylesheet"] = GetStylesheet(theme)
		}
		style := f.Get("HTML_HIGHLIGHT_STYLE")
		if style != "" {
			self.Props["hljsstyle"] = style
		}
		w := NewOrgHtmlWriter(self)
		w.Opts = opts
		org.WriteNodes(w, f.Nodes...)
		res := w.String()
		self.Props["html_data"] = res
		self.Props["post_scripts"] = w.PostWriteScripts

		fmt.Printf("DOC START: ========================================\n")
		res = self.pm.Tempo.RenderTemplate(self.TemplatePath, self.Props)
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
}

func (self *OrgHtmlExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func NewHtmlExp() *OrgHtmlExporter {
	var g *OrgHtmlExporter = new(OrgHtmlExporter)
	return g
}

var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

func GetStylesheet(name string) string {
	if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_styles/" + name + "_style.css")); err == nil {
		re := regexp.MustCompile(`url\(([^)]+)\)`)
		return re.ReplaceAllString(string(data), "url(http://localhost:8010/${1})")
	}
	return ""
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
		m["stylesheet"] = GetStylesheet("default")
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
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
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

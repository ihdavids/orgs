//lint:file-ignore ST1006 allow the use of self
// EXPORTER: IMPRESS JS Export

package revealjs

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"gopkg.in/op/go-logging.v1"
)

var rver = "2.0.0"
var cdn = "https://cdn.jsdelivr.net/gh/impress/impress.js@" + rver
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

// <style>
// {{.stylesheet | css}}
// </style>
type ImpressExporter struct {
	Props        map[string]interface{}
	ThemePath    string
	TemplatePath string
	out          *logging.Logger
	pm           *plugs.PluginManager
}

type ImpressWriter struct {
	*org.HTMLWriter
	exp  *ImpressExporter
	Opts string
}

func NewImpressWriter(exp *ImpressExporter) *ImpressWriter {
	// This lovely circular reference ensures overrides are called when calling write node.
	rw := ImpressWriter{org.NewHTMLWriter(), nil, ""}
	rw.ExtendingWriter = &rw
	rw.exp = exp

	// This was a bad idea and needs to be removed!
	rw.HeadlineWriterOverride = &rw
	rw.NoWrapCodeBlock = true
	rw.HighlightCodeBlock = func(keywords []org.Keyword, source, lang string, inline bool, params map[string]string) string {
		var attribs []string = []string{}
		for _, key := range keywords {
			// This does something strange! I don't understand why it centers the text and puts a red box around it
			if key.Key == "IMPRESS_LINES" {
				attribs = append(attribs, fmt.Sprintf("%s=\"%s\"", "data-line-numbers", key.Value))
			}
		}
		attribStr := ""
		if len(attribs) > 0 {
			attribStr = strings.Join(attribs, " ")
		}
		if lang == "mermaid" {
			return fmt.Sprintf(`<pre class="mermaid">%s</pre>`, html.EscapeString(source))
		} else if lang == "d3-wordcloud" {
			return fmt.Sprintf(`<script></script>`)
		} else {
			if inline {
				return fmt.Sprintf("<div class=\"hljs\"><pre><code %s >%s</code></pre></div>", attribStr, html.EscapeString(source))
			}
			return fmt.Sprintf("<div class=\"hljs\"><pre><code %s >%s</code></pre></div>", attribStr, html.EscapeString(source))
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

func GetPropByIndex(name, revealName string, h org.Headline, secProps string) string {
	tran := h.Doc.Get(name)
	if tmp, ok := h.Properties.Get(name); ok {
		tran = tmp
	}
	if tran != "" {
		if nv, err := strconv.ParseFloat(tran, 64); err == nil {
			secProps = fmt.Sprintf("%s %s=\"%f\"", secProps, revealName, nv*float64(h.Index))
		}
	}
	return secProps
}

func GetPropVal(name, defVal string, h org.Headline) string {
	tran := h.Doc.Get(name)
	if tmp, ok := h.Properties.Get(name); ok {
		tran = tmp
	}
	if tran != "" {
		return tran
	}
	return defVal
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

func (w *ImpressWriter) WriteRegularLink(l org.RegularLink) {
	if l.Protocol == "file" && l.Kind() == "image" {

		// This bit is tricky: VSCode will not work with anything not setup as accessible in the webroot
		// Since a vscode webview is a seperate entity self signed certificates also do not work.
		// So we support localhost access over http to fix that. It's not ideal but works.

		url := l.URL[len("file://"):]
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

func (w *ImpressWriter) WriteHeadlineOverride(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	// data-auto-animate
	// none	Switch backgrounds instantly
	// fade	Cross fade — default for background transitions
	// slide	Slide between backgrounds — default for slide transitions
	// convex	Slide at a convex angle
	// concave	Slide at a concave angle
	// zoom	Scale the incoming slide up so it grows in from the center of the screen
	//data-transition="zoom"
	//data-transition-speed="fast"
	secProps := ""
	//secProps = GetProp("REVEAL_TRANSITION", "data-transition", h, secProps)
	//secProps = GetProp("REVEAL_TRANSITION_SPEED", "data-transition-speed", h, secProps)
	//secProps = GetPropTag("REVEAL_AUTO_ANIMATE", "data-auto-animate", h, secProps)
	//defY := 1000 * h.Index
	//defX := 1000 * h.Index
	//scale := 5 * (h.Index + 1)
	//scale := 1
	secProps = GetProp("IMPRESS_ROTX", "data-rel-rotate-x", h, secProps)
	secProps = GetProp("IMPRESS_ROTY", "data-rel-rotate-y", h, secProps)
	secProps = GetProp("IMPRESS_ROTZ", "data-rel-rotate-z", h, secProps)
	secProps = GetPropByIndex("IMPRESS_ROT", "data-rotate", h, secProps)
	defX := GetPropVal("IMPRESS_X", ".6", h)
	defY := GetPropVal("IMPRESS_Y", ".6", h)

	//w.WriteString(fmt.Sprintf(`<div id="slide_%d" class="step" %s data-x="%d" data-y="%d" data-rotate="%d" >`, h.Index, secProps, defX, defY, rotate))

	w.WriteString(fmt.Sprintf(`<div id="slide_%d" class="step" %s data-rel-x="%sw" data-rel-y="%sh">`, h.Index, secProps, defX, defY))

	w.WriteString(fmt.Sprintf("<h%d>", h.Lvl+1))
	org.WriteNodes(w, h.Title...)
	w.WriteString(fmt.Sprintf("</h%d>", h.Lvl+1))

	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
	w.WriteString("</div>\n")
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

func (s *ImpressExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(s)
}

func (self *ImpressExporter) Export(db plugs.ODb, query string, to string, opts string) error {
	fmt.Printf("IMPRESS: Export called")
	_, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
		log.Printf("%s\n", msg)
		return fmt.Errorf(msg)
	}
	return nil
}

func ExpandTemplateIntoBuf(o *bytes.Buffer, temp string, m map[string]interface{}) {
	t := template.Must(template.New("").Funcs(funcMap).Parse(temp))
	err := t.Execute(o, m)
	if err != nil {
		fmt.Printf("TEMPLATE ERROR: %s\n", err.Error())
	}
}

func (e *ImpressExporter) ExpandThemePath(tname string) string {
	name := "impress_theme_" + tname + ".css"
	tempFolderName, _ := filepath.Abs(path.Join(e.ThemePath, name))
	if _, err := os.Stat(tempFolderName); err == nil {
		name = tempFolderName
	}
	return name
}

func (self *ImpressExporter) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Printf("IMPRESS: Export string called [%s]:[%s]\n", query, opts)
	/*
		_, err := db.QueryTodosExpr(query)
		if err != nil {
			msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
			log.Printf(msg)
			return fmt.Errorf(msg), ""
		}
	*/

	if f := db.FindByFile(query); f != nil {
		theme := f.Get("IMPRESS_THEME")
		if theme == "" {
			theme = "impressdefault"
		}
		if theme != "" {
			self.Props["theme"] = theme
			fname := self.ExpandThemePath(theme)
			fmt.Printf("THEME PATH: %s\n", fname)
			if tdata, ferr := os.ReadFile(fname); ferr == nil {
				fmt.Printf("THEME DATA: %s\n", tdata)
				self.Props["themedata"] = string(tdata)
			}
		}

		style := f.Get("IMPRESS_HIGHLIGHT_STYLE")
		if style != "" {
			self.Props["hljsstyle"] = style
		}
		w := NewImpressWriter(self)
		org.WriteNodes(w, f.Nodes...)
		res := w.String()
		self.Props["slide_data"] = res

		fmt.Printf("DOC START: ========================================\n")
		res = self.pm.Tempo.RenderTemplate(self.TemplatePath, self.Props)
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("failed to find file in database: [%s]", query), ""
	}
}

func (self *ImpressExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func NewHtmlExp() *ImpressExporter {
	var g *ImpressExporter = new(ImpressExporter)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	force_reload_style := false
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["impress_cdn"]; !ok {
		m["impress_cdn"] = cdn
	}
	if _, ok := m["hljs_cdn"]; !ok {
		m["hljs_cdn"] = hljscdn
	}
	if _, ok := m["hljs_style"]; !ok {
		m["hljs_style"] = "monokai"
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["theme"]; !ok {
		m["theme"] = "league"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["stylesheet"]; !ok || force_reload_style {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_style.css")); err == nil {
			m["stylesheet"] = (string)(data)
		}
	}

	return m
}

// init function is called at boot
func init() {
	plugs.AddExporter("impressjs", func() plugs.Exporter {
		return &ImpressExporter{Props: ValidateMap(map[string]interface{}{}), ThemePath: "./templates", TemplatePath: "impress_default.tpl"}
	})
}

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

var docStart = `
<!DOCTYPE html>
<html>
<head>
  <style>
	.impress-supported .fallback-message {
	    display: none;
	}
	/*
    Now let's style the presentation steps.

    We start with basics to make sure it displays correctly in everywhere ...

    width: 900px;
*/

.step {
    position: relative;
    padding: 10px;
    margin: 10px auto;
	min-width: 1024px;

    -webkit-box-sizing: border-box;
    -moz-box-sizing:    border-box;
    -ms-box-sizing:     border-box;
    -o-box-sizing:      border-box;
    box-sizing:         border-box;

    font-family: 'PT Serif', georgia, serif;
    font-size: 48px;
    line-height: 1;
    text-shadow: 0 2px 2px rgba(0, 0, 0, .1);
}


/*
    ... and we enhance the styles for impress.js.

    Basically we remove the margin and make inactive steps a little bit transparent.
*/
.impress-enabled .step {
    margin: 0;
    opacity: 0.3;

    -webkit-transition: opacity 1s;
    -moz-transition:    opacity 1s;
    -ms-transition:     opacity 1s;
    -o-transition:      opacity 1s;
    transition:         opacity 1s;
}

.impress-enabled .step.active { opacity: 1 }

/*
    These 'slide' step styles were heavily inspired by HTML5 Slides:
    http://html5slides.googlecode.com/svn/trunk/styles.css

    ;)

    They cover everything what you see on first three steps of the demo.

    All impress.js steps are wrapped inside a div element of 0 size! This means that relative
    values for width and height (example: width: 100%) will not work. You need to use pixel
    values. The pixel values used here correspond to the data-width and data-height given to the
    #impress root element. When the presentation is viewed on a larger or smaller screen, impress.js
    will automatically scale the steps to fit the screen.
*/
.slide {
    display: block;

    width: 900px;
    height: 700px;
    padding: 40px 60px;

    background-color: white;
    border: 1px solid rgba(0, 0, 0, .3);
    border-radius: 10px;
    box-shadow: 0 2px 6px rgba(0, 0, 0, .1);

    color: rgb(102, 102, 102);
    text-shadow: 0 2px 2px rgba(0, 0, 0, .1);

    font-family: 'Open Sans', Arial, sans-serif;
    font-size: 30px;
    line-height: 36px;
    letter-spacing: -1px;
}

.slide q {
    display: block;
    font-size: 50px;
    line-height: 72px;

    margin-top: 100px;
}

.slide q strong {
    white-space: nowrap;
}

  </style>
  <style>
  {{.themedata|css}}
  </style>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}"> 
  <link rel="stylesheet" href="{{.hljscdn}}/styles/{{.hljsstyle}}.min.css">
  <link rel="stylesheet" href="{{.cdn}}/css/impress-common.css">
  <meta charset="utf-8" />
    <meta name="viewport" content="width=1024" />
    <meta name="apple-mobile-web-app-capable" content="yes" />
</head>
<body class="impress-not-supported"
    data-transition-duration="500"
    data-width="1024"
    data-height="768"
    data-max-scale="3"
    data-min-scale="0"
    data-perspective="100"
>
<div class="fallback-message">
    <p>Your browser <b>doesn't support the features required</b> by impress.js, so you are presented with a simplified version of this presentation.</p>
    <p>For the best experience please use the latest <b>Chrome</b>, <b>Safari</b> or <b>Firefox</b> browser.</p>
</div>
	<div id="impress">
`

var docEnd = `
	</div>
	<div id="impress-toolbar"></div>
	<div class="impress-progressbar"><div></div></div>
	<div class="impress-progress"></div>
	<script>
	if ("ontouchstart" in document.documentElement) { 
	    document.querySelector(".hint").innerHTML = "<p>Swipe left or right to navigate</p>";
	}
	</script>
	<script src="{{.hljscdn}}/highlight.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/headjs@1.0.3/dist/1.0.0/head.min.js"></script>
	<script src="{{.cdn}}/js/impress.js"></script>
    <script>impress().init();</script>
	<script>hljs.highlightAll();</script>
	<script>impress.addPreInitPlugin( rel );</script>
	<script type="module">
	  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
	  mermaid.initialize({ startOnLoad: true });
	</script>
</body>
</html>
`

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
}

func NewImpressWriter() *ImpressWriter {
	// This lovely circular reference ensures overrides are called when calling write node.
	rw := ImpressWriter{org.NewHTMLWriter()}
	rw.ExtendingWriter = &rw

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

	//w.WriteString(fmt.Sprintf(`<div id="outline-container-%s" class="outline-%d">`, h.ID(), h.Lvl+1) + "\n")
	//w.WriteString(fmt.Sprintf(`<h%d id="%s">`, h.Lvl+1, h.ID()) + "\n")
	//if w.Document.GetOption("todo") != "nil" && h.Status != "" {
	//	w.WriteString(fmt.Sprintf(`<span class="todo">%s</span>`, h.Status) + "\n")
	//}
	//if w.Document.GetOption("pri") != "nil" && h.Priority != "" {
	//	w.WriteString(fmt.Sprintf(`<span class="priority">[%s]</span>`, h.Priority) + "\n")
	//}
	w.WriteString(fmt.Sprintf("<h%d>", h.Lvl+1))
	org.WriteNodes(w, h.Title...)
	w.WriteString(fmt.Sprintf("</h%d>", h.Lvl+1))

	//if w.Document.GetOption("tags") != "nil" && len(h.Tags) != 0 {
	//	tags := make([]string, len(h.Tags))
	//	for i, tag := range h.Tags {
	//		tags[i] = fmt.Sprintf(`<span>%s</span>`, tag)
	//	}
	//	w.WriteString("&#xa0;&#xa0;&#xa0;")
	//	w.WriteString(fmt.Sprintf(`<span class="tags">%s</span>`, strings.Join(tags, "&#xa0;")))
	//}
	//w.WriteString(fmt.Sprintf("\n</h%d>\n", h.Lvl+1))
	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
	//w.WriteString("</div>\n")
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

func fileNameWithoutExt(fileName string) string {
	return fileName[:len(fileName)-len(filepath.Ext(fileName))]
}

func agendaFilenameTag(fileName string) string {
	return fileNameWithoutExt(filepath.Base(fileName))
}

func (self *ImpressExporter) Export(db plugs.ODb, query string, to string, opts string) error {
	fmt.Printf("REVEAL: Export called", query, to, opts)
	_, err := db.QueryTodosExpr(query)
	if err != nil {
		msg := fmt.Sprintf("ERROR: html failed to query expression, %v [%s]\n", err, query)
		log.Printf(msg)
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
	fmt.Printf("HTML: Export string called [%s]:[%s]\n", query, opts)
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
		w := NewImpressWriter()
		org.WriteNodes(w, f.Nodes...)
		res := w.String()
		self.Props["slide_data"] = res

		fmt.Printf("DOC START: ========================================\n")
		res = self.pm.Tempo.RenderTemplate(self.TemplatePath, self.Props)
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
	return nil, ""
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

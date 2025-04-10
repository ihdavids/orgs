//lint:file-ignore ST1006 allow the use of self
// EXPORTER: Reveal JS
/* SDOC: Exporters

* Reveal JS

	TODO More documentation on this module

	#+BEGIN_SRC yaml
    - name: "revealjs"
      templatepath: "path to reveal template"
	#+END_SRC

EDOC */

package revealjs

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"gopkg.in/op/go-logging.v1"
)

var rver = "5.0.4"
var cdn = "https://cdnjs.cloudflare.com/ajax/libs/reveal.js/" + rver
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

/*

	// Display controls in the bottom right corner
	controls: true,

	width: '100%',
	height: '100%',

	// Display a presentation progress bar
	progress: true,

	// Set default timing of 2 minutes per slide
	defaultTiming: 120,

	// Display the page number of the current slide
	slideNumber: true,

	// Push each slide change to the browser history
	history: false,

	// Enable keyboard shortcuts for navigation
	keyboard: true,

	// Enable the slide overview mode
	overview: true,

	// Vertical centering of slides
	center: true,

	// Enables touch navigation on devices with touch input
	touch: true,

	// Loop the presentation
	loop: false,

	// Change the presentation direction to be RTL
	rtl: false,

	// Randomizes the order of slides each time the presentation loads
	shuffle: false,

	// Turns fragments on and off globally
	fragments: true,

	// Flags if the presentation is running in an embedded mode,
	// i.e. contained within a limited portion of the screen
	embedded: false,

	// Flags if we should show a help overlay when the questionmark
	// key is pressed
	help: true,

	// Flags if speaker notes should be visible to all viewers
	showNotes: false,

	// Global override for autolaying embedded media (video/audio/iframe)
	// - null: Media will only autoplay if data-autoplay is present
	// - true: All media will autoplay, regardless of individual setting
	// - false: No media will autoplay, regardless of individual setting
	autoPlayMedia: null,

	// Number of milliseconds between automatically proceeding to the
	// next slide, disabled when set to 0, this value can be overwritten
	// by using a data-autoslide attribute on your slides
	autoSlide: 0,

	// Stop auto-sliding after user input
	autoSlideStoppable: true,

	// Use this method for navigation when auto-sliding
	autoSlideMethod: Reveal.navigateNext,

	// Enable slide navigation via mouse wheel
	mouseWheel: false,

	// Hides the address bar on mobile devices
	hideAddressBar: true,

	// Opens links in an iframe preview overlay
	previewLinks: true,

	// Transition style
	transition: 'slide', // none/fade/slide/convex/concave/zoom

	// Transition speed
	transitionSpeed: 'default', // default/fast/slow

	// Transition style for full page slide backgrounds
	backgroundTransition: 'fade', // none/fade/slide/convex/concave/zoom

	// Number of slides away from the current that are visible
	viewDistance: 3,

	// Parallax background image
	parallaxBackgroundImage: '', // e.g. "'https://s3.amazonaws.com/hakim-static/reveal-js/reveal-parallax-1.jpg'"

	// Parallax background size
	parallaxBackgroundSize: '', // CSS syntax, e.g. "2100px 900px"

	// Number of pixels to move the parallax background per slide
	// - Calculated automatically unless specified
	// - Set to 0 to disable movement along an axis
	parallaxBackgroundHorizontal: null,
	parallaxBackgroundVertical: null,


	// The display mode that will be used to show slides
	display: 'block',

	/*
	multiplex: {
		// Example values. To generate your own, see the socket.io server instructions.
		secret: '13652805320794272084', // Obtained from the socket.io server. Gives this (the master) control of the presentation
		id: '1ea875674b17ca76', // Obtained from socket.io server
		url: 'https://reveal-js-multiplex-ccjbegmaii.now.sh' // Location of socket.io server
	},
*/

type RevealExporter struct {
	TemplatePath string
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *plugs.PluginManager
}

type RevealWriter struct {
	*org.HTMLWriter
	exp              *RevealExporter
	PostWriteScripts string
	Opts             string
}

func NewRevealWriter(exp *RevealExporter) *RevealWriter {
	// This lovely bit of circular reference ensures that we get called when exporting for any methods
	// we have overwritten
	rw := RevealWriter{org.NewHTMLWriter(), nil, "", ""}
	rw.ExtendingWriter = &rw
	rw.exp = exp
	// This version was a bad idea and needs to get removed!
	//rw.HeadlineWriterOverride = &rw

	// This we should probably just replace with an override as well! Way better
	rw.NoWrapCodeBlock = true
	cnt := 1
	rw.HighlightCodeBlock = func(keywords []org.Keyword, source, lang string, inline bool, params map[string]string) string {
		var attribs []string = []string{}
		for _, key := range keywords {
			// This does something strange! I don't understand why it centers the text and puts a red box around it
			if key.Key == "REVEAL_LINES" {
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
func (w *RevealWriter) WriteRegularLink(l org.RegularLink) {
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
		/*
			// This works but only with a certificate
			fname := url
			if l.Description == nil {
				w.WriteString(fmt.Sprintf(`<img src="https://localhost/images/%s" alt="%s" title="%s" />`, fname, l.URL, l.URL))
			} else {
				description := strings.TrimPrefix(org.String(l.Description...), "file:")
				w.WriteString(fmt.Sprintf(`<a href="https://localhost/images/%s"><img src="%s" alt="%s" /></a>`, l.URL, description, description))
			}
		*/
	} else {
		w.HTMLWriter.WriteRegularLink(l)
	}
}

// OVERRIDE: This overrides the core method
func (w *RevealWriter) WriteHeadline(h org.Headline) {
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
	secProps = GetProp("REVEAL_TRANSITION", "data-transition", h, secProps)
	secProps = GetProp("REVEAL_TRANSITION_SPEED", "data-transition-speed", h, secProps)
	secProps = GetPropTag("REVEAL_AUTO_ANIMATE", "data-auto-animate", h, secProps)
	w.WriteString(fmt.Sprintf(`<section %s>`, secProps))

	w.WriteString(fmt.Sprintf("<h%d>", h.Lvl+1))
	org.WriteNodes(w, h.Title...)
	w.WriteString(fmt.Sprintf("</h%d>", h.Lvl+1))

	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
	w.WriteString("</section>\n")
}

func (w *RevealWriter) WriteTable(t org.Table) {
	w.HTMLWriter.WriteTable(t)
	/*
		} else {
			name := fmt.Sprintf("tabulator_table_%d", t.Pos.Row)
			w.WriteString(fmt.Sprintf("<table id=\"%s\">\n", name))
			inHead := len(t.SeparatorIndices) > 0 &&
				t.SeparatorIndices[0] != len(t.Rows)-1 &&
				(t.SeparatorIndices[0] != 0 || len(t.SeparatorIndices) > 1 && t.SeparatorIndices[len(t.SeparatorIndices)-1] != len(t.Rows)-1)
			if inHead {
				w.WriteString("<thead>\n")
			} else {
				w.WriteString("<tbody>\n")
			}
			for i, row := range t.Rows {
				if len(row.Columns) == 0 && i != 0 && i != len(t.Rows)-1 {
					if inHead {
						w.WriteString("</thead>\n<tbody>\n")
						inHead = false
					} else {
						w.WriteString("</tbody>\n<tbody>\n")
					}
				}
				if row.IsSpecial {
					continue
				}
				if inHead {
					w.writeTableColumns(row.Columns, "th")
				} else {
					w.writeTableColumns(row.Columns, "td")
				}
			}
			w.WriteString("</tbody>\n</table>\n")
		}
	*/
}

func (w *RevealWriter) writeTableColumns(columns []*org.Column, tag string) {
	w.WriteString("<tr>\n")
	for _, column := range columns {
		if column.Align == "" {
			w.WriteString(fmt.Sprintf("<%s>", tag))
		} else {
			w.WriteString(fmt.Sprintf(`<%s class="align-%s">`, tag, column.Align))
		}
		org.WriteNodes(w, column.Children...)
		w.WriteString(fmt.Sprintf("</%s>\n", tag))
	}
	w.WriteString("</tr>\n")
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

func (s *RevealExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(s)
}

func (self *RevealExporter) Export(db plugs.ODb, query string, to string, opts string, props map[string]string) error {
	fmt.Printf("REVEAL: Export called")
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

func (self *RevealExporter) ExportToString(db plugs.ODb, query string, opts string, props map[string]string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Printf("REVEAL: Export string called [%s]:[%s]\n", query, opts)

	if f := db.FindByFile(query); f != nil {
		theme := f.Get("REVEAL_THEME")
		if theme != "" {
			self.Props["theme"] = theme
		}
		style := f.Get("REVEAL_HIGHLIGHT_STYLE")
		if style != "" {
			self.Props["hljsstyle"] = style
		}
		w := NewRevealWriter(self)
		w.Opts = opts
		org.WriteNodes(w, f.Nodes...)
		res := w.String()
		self.Props["slide_data"] = res
		self.Props["post_scripts"] = w.PostWriteScripts

		fmt.Printf("DOC START: ========================================\n")
		res = self.pm.Tempo.RenderTemplate(self.TemplatePath, self.Props)
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("failed to find file in database: [%s]", query), ""
	}
}

func (self *RevealExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func NewHtmlExp() *RevealExporter {
	var g *RevealExporter = new(RevealExporter)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	force_reload_style := false
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["cdn"]; !ok {
		m["reveal_cdn"] = cdn
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
		m["theme"] = "league"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["stylesheet"]; !ok || force_reload_style {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("reveal_style.css")); err == nil {
			m["stylesheet"] = (string)(data)
		}
	}
	return m
}

// init function is called at boot
func init() {
	plugs.AddExporter("revealjs", func() plugs.Exporter {
		return &RevealExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "reveal_default.tpl"}
	})
}

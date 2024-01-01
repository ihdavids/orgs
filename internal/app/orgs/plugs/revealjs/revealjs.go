//lint:file-ignore ST1006 allow the use of self
// EXPORTER: HTML Export

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
)

var rver = "5.0.4"
var cdn = "https://cdnjs.cloudflare.com/ajax/libs/reveal.js/" + rver
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

var docStart = `
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{.fontfamily}}"> 
  <link rel="stylesheet" href="{{.cdn}}/reveal.min.css">
  <link rel="stylesheet" href="{{.cdn}}/theme/{{.theme}}.css">

  <link rel="stylesheet" href="{{.hljscdn}}/styles/{{.hljsstyle}}.min.css">
<style>
{{.stylesheet | css}}
</style>
</head>
<body>
	<div class="reveal">
		<div class="slides">
`

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

var docEnd = `
	</div></div>
	<script src="https://cdn.jsdelivr.net/npm/headjs@1.0.3/dist/1.0.0/head.min.js"></script>
	<script src="{{.cdn}}/reveal.min.js"></script>
	<script src="{{.cdn}}/plugin/highlight/highlight.min.js"></script>


	<!--<script src="index.js"></script>-->
	<script>
		// More info about config & dependencies:
		// - https://github.com/hakimel/reveal.js#configuration
		// - https://github.com/hakimel/reveal.js#dependencies
		Reveal.initialize({
			center: false,
			navigationMode: "grid",
			dependencies: [
				{ src: '{{.cdn}}/plugin/markdown/markdown.min.js' },
				{ src: '{{.cdn}}/plugin/notes/notes.min.js', async: true },
				{ src: '{{.cdn}}/plugin/math/math.min.js', async: true },
				{ src: '{{.cdn}}/plugin/search/search.min.js', async: true },
				{ src: '{{.cdn}}/plugin/zoom/zoom.min.js', async: true },
				//{ src: '{{.cdn}}/plugin/highlight/highlight.min.js', async: true},
				//{ src: '{{.cdn}}/plugin/highlight/highlight.min.js', callback: function () { hljs.initHighlightingOnLoad(); } },
				//{ src: '//cdn.socket.io/socket.io-1.3.5.js', async: true },
				//{ src: 'plugin/multiplex/master.js', async: true },
				// and if you want speaker notes
				//{ src: '{{.cdn}}/plugin/notes-server/client.js', async: true }

			],
			markdown: {
				//            renderer: myrenderer,
				smartypants: true
			},
			plugins: [RevealHighlight]
		});
		Reveal.configure({
			// PDF Configurations
			pdfMaxPagesPerSlide: 1

		});
		Reveal.getPlugins();
	</script>
	<script type="module">
	  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
	  mermaid.initialize({ startOnLoad: true });
	</script>

</body>
</html>
`

type RevealExporter struct {
	Props map[string]interface{}
}

type RevealWriter struct {
	*org.HTMLWriter
}

func NewRevealWriter() *RevealWriter {
	// This lovely bit of circular reference ensures that we get called when exporting for any methods
	// we have overwritten
	rw := RevealWriter{org.NewHTMLWriter()}
	rw.ExtendingWriter = &rw

	// This version was a bad idea and needs to get removed!
	//rw.HeadlineWriterOverride = &rw

	// This we should probably just replace with an override as well! Way better
	rw.NoWrapCodeBlock = true
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
	w.WriteString("</section>\n")
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

func fileNameWithoutExt(fileName string) string {
	return fileName[:len(fileName)-len(filepath.Ext(fileName))]
}

func agendaFilenameTag(fileName string) string {
	return fileNameWithoutExt(filepath.Base(fileName))
}

func (self *RevealExporter) Export(db plugs.ODb, query string, to string, opts string) error {
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

func (self *RevealExporter) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
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
		theme := f.Get("REVEAL_THEME")
		if theme != "" {
			self.Props["theme"] = theme
		}
		style := f.Get("REVEAL_HIGHLIGHT_STYLE")
		if style != "" {
			self.Props["hljsstyle"] = style
		}
		w := NewRevealWriter()
		org.WriteNodes(w, f.Nodes...)
		res := w.String()
		//fmt.Printf("X: [%s]", res)

		o := bytes.NewBufferString("")
		//fmt.Printf("DOC START: %s\n", docStart)
		fmt.Printf("DOC START: ========================================\n")
		ExpandTemplateIntoBuf(o, docStart, self.Props)

		end := bytes.NewBufferString("")
		ExpandTemplateIntoBuf(end, docEnd, self.Props)
		res = o.String() + res + end.String()
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
	return nil, ""
}

func (self *RevealExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
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
		m["cdn"] = cdn
	}
	if _, ok := m["hljscdn"]; !ok {
		m["hljscdn"] = hljscdn
	}
	if _, ok := m["hljsstyle"]; !ok {
		m["hljsstyle"] = "monokai"
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
	plugs.AddExporter("revealjs", func() plugs.Exporter {
		return &RevealExporter{Props: ValidateMap(map[string]interface{}{})}
	})
}

//lint:file-ignore ST1006 allow the use of self
// EXPORTER: WebSlides
/* SDOC: Exporters

* WebSlides

  [[https://github.com/webslides/webslides][WebSlides]] is the one of the four
  presentation exporters that is a *web page* rather than a slide engine: it
  scrolls, it is responsive, it reads properly on a phone, and the deck you
  show on a projector is the same file you can give somebody a link to.

  #+BEGIN_SRC yaml
    - name: "webslides"
  #+END_SRC

  Every headline is a slide (see =#+SLIDE_LEVEL=), and everything in
  =#+WEBSLIDES_*= has a =#+SLIDE_*= spelling that works in all four
  frameworks - so one file exports as WebSlides, reveal.js, impress.js or
  deck.js without being rewritten.

** Document settings

   | Keyword                       | Means                                       |
   |-------------------------------+---------------------------------------------|
   | =#+WEBSLIDES_THEME:=          | =light=, =dark=, =black=, =apple=, =blue=,  |
   |                               | =primary= - the ground every slide starts   |
   |                               | on                                          |
   | =#+WEBSLIDES_VERTICAL:=       | scroll up and down instead of sideways      |
   | =#+WEBSLIDES_AUTOSLIDE:=      | milliseconds per slide, or nil              |
   | =#+WEBSLIDES_LOOP:=           | back to the first slide at the end          |
   | =#+WEBSLIDES_SHOW_INDEX:=     | the slide index / zoom view                  |
   | =#+WEBSLIDES_CHANGE_ON_CLICK:= | a click anywhere goes forward              |
   | =#+WEBSLIDES_NAVIGATE_ON_SCROLL:= | the wheel changes slides                |
   | =#+WEBSLIDES_SCROLL_WAIT:=    | milliseconds to ignore the wheel for        |
   | =#+WEBSLIDES_SLIDE_OFFSET:=   | pixels of overlap while sliding             |
   | =#+WEBSLIDES_MIN_WHEEL_DELTA:= | how hard a wheel push has to be            |
   | =#+WEBSLIDES_ICONS:=          | load the svg icon set                       |
   | =#+WEBSLIDES_ANIMATE:=        | load animate.css, for =:ANIMATION:=         |
   | =#+WEBSLIDES_HIGHLIGHT_STYLE:= | a highlight.js style name                  |
   | =#+WEBSLIDES_FONT:=           | a Google font family                        |
   | =#+WEBSLIDES_CSS:=            | a stylesheet url to add                     |
   | =#+WEBSLIDES_HEAD:=           | raw html for the head                       |
   | =#+SLIDE_LEVEL:=              | which headline depth is a slide             |
   | =#+SLIDE_TITLE_SLIDE:=        | nil to drop the generated title slide       |

** Slide properties

   | Property           | Means                                               |
   |--------------------+-----------------------------------------------------|
   | =:CLASS:=          | classes straight onto the section - this is how you |
   |                    | reach WebSlides' forty components (=bg-apple=,      |
   |                    | =fullscreen=, =slide-top=, =card-50=)               |
   | =:ALIGN:=          | =center=, =left=, =right=                           |
   | =:BACKGROUND:=     | a colour, an image, a video or a page - worked out   |
   |                    | from the value                                      |
   | =:BACKGROUND_IMAGE:= =:BACKGROUND_COLOR:= =:BACKGROUND_VIDEO:= =:BACKGROUND_IFRAME:= | said outright |
   | =:BACKGROUND_SIZE:= =:BACKGROUND_POSITION:= =:BACKGROUND_OPACITY:= | css for an image |
   | =:WRAP:=           | =nil= for a full-bleed slide, or a size:             |
   |                    | =size-50=, =size-40=, =size-30=                     |
   | =:WRAP_CLASS:=     | classes on the inner wrap                           |
   | =:ANIMATION:=      | =zoomIn=, =fadeIn=, =slideInLeft=, ...              |
   | =:CUSTOM_ID:=      | the section's id, so it can be linked to            |
   | =:FRAGMENT:=       | this slide's bullets appear one at a time           |
   | =:NOTES:=          | a drawer of speaker notes - press =n=               |

** Themes, and making too much fit

   A **theme** is one =slides_theme_<name>.css= file in your template folder,
   and the same file works in all four frameworks - reveal.js, impress.js,
   WebSlides and deck.js - because what it sets is a palette and a type scale
   rather than one library's class names. =GET /slides/themes= lists the ones
   this server has, and =#+SLIDE_THEME:= or the framework's own
   =#+REVEAL_THEME:= / =#+IMPRESS_THEME:= / =#+WEBSLIDES_THEME:= /
   =#+DECK_THEME:= picks one. A name this server does not have is passed
   through to the framework, so reveal's =dracula= and deck.js' =swiss= still
   work.

   The ones that ship:

   | Theme       | What it is                                               |
   |-------------+----------------------------------------------------------|
   | =graphite=  | near-black, warm white, one brass accent. The one to     |
   |             | reach for when the talk matters more than the slides     |
   | =linen=     | paper and ink, Playfair over a quiet sans. Editorial     |
   | =nocturne=  | deep indigo, a cool accent, Space Grotesk. Modern tech   |
   | =blueprint= | slate and a drafting grid, headings in the code face     |
   | =bloom=     | a warm light wash, Fraunces and Outfit. Friendly         |
   | =mono=      | black on white, Swiss, one red hairline and nothing else |
   | =sage=      | muted green, Lora, unhurried. The calm one               |

   And seventeen with a **moving backdrop** behind the deck (see below):

   | Theme        | What it is                                                |
   |--------------+-----------------------------------------------------------|
   | =voidcubes=  | muted violet and blue blocks turning through space. For a |
   |              | talk about something being built                          |
   | =meridian=   | the world as dots, turning, with light running between    |
   |              | its cities. For a talk about somewhere else               |
   | =relay=      | a lattice of nodes making and breaking connections. For   |
   |              | systems talking to each other                             |
   | =sundown=    | the same lattice at dusk, warm over a violet horizon      |
   | =prism=      | shards of crystal, translucent, two sided. The showiest   |
   | =lattice=    | wireframe solids tumbling. The technical one              |
   | =lowlands=   | a low-poly field running to the horizon. The quietest at  |
   |              | the top of the screen, so the one for a long deck         |
   | =cascade=    | columns of glyphs falling away into the distance          |
   | =tidecrest=  | an open swell running to a cold horizon, crests lit       |
   | =nimbus=     | soft drifting haze and no geometry at all. The gentlest   |
   |              | behind text - there are no edges in it for a letter to    |
   |              | sit on, so it is the one for a deck that is mostly words  |
   | =erosion=    | contour lines over ground that will not hold still. Sand  |
   |              | on charcoal, line work throughout, so it sits well under  |
   |              | diagrams                                                  |
   | =halo=       | luminous hoops round a lit core. It has a bright middle,  |
   |              | so it is the one of these with a composition              |
   | =orrery=     | a faceted body rippling inside luminous hoops. The body   |
   |              | is lit and the hoops are not, which is what makes one     |
   |              | solid and the other light. For a talk with a middle to    |
   |              | it: one thing, and everything that turns around it        |
   | =emberline=  | filaments twisted round an axis, pinched at the waist,    |
   |              | flaring into a crown and roots, over a star field         |
   | =noema=      | a flower unfurling from a bud and shutting again, on a    |
   |              | starry field. The slowest, for an opening slide           |
   | =massif=     | a landscape of ridges running to a horizon, drawn as      |
   |              | contours, with smoke rising behind. Nearly monochrome,    |
   |              | so it does not argue with photographs                     |
   | =stardust=   | a planet made of dust, turning, with a ring of debris.    |
   |              | Meridian's abstract cousin, with no geography to read     |

   A theme sets a palette and a type scale, and the scale hangs off one number:
   =--slide-font-size=, which defaults to each framework's own (reveal.js and
   impress.js 40px over a stage they scale, deck.js 28px, WebSlides 24px). It is
   set here because a local theme is loaded *instead of* the framework's own
   theme stylesheet, which is where that number lives - without it every local
   theme rendered at two fifths of the size reveal's own themes do.

   A theme also names the highlight.js style and the mermaid theme it wants
   (=@hljs:= and =@mermaid:= in its own comment header), so a code block and a
   diagram land in the same palette as the slide they are on. A slide whose own
   =:BACKGROUND:= contradicts the theme has its text flipped to suit
   (=org-on-dark= / =org-on-light=); =:BACKGROUND_INK: light= says so outright
   for a picture, which cannot be measured.


*** The moving backdrop

    A theme can ask for a **scene drawn behind the deck** - the one thing a
    palette cannot say - with =@backdrop:= in its own comment header, and tune
    it with custom properties (=--backdrop-colors=, =--backdrop-fog=,
    =--backdrop-glow=, =--backdrop-veil=, =--backdrop-density=,
    =--backdrop-speed=). The scenes live in =slides_backdrop.js= and work in
    all four frameworks, so a new one of these is still one file of colours.

    | Scene       | What it draws                                            |
    |-------------+----------------------------------------------------------|
    | =cubes=     | lit blocks drifting past the camera, each turning        |
    | =globe=     | the world as dots, with pulses running between cities    |
    | =net=       | points with a line between any two that are near enough  |
    | =crystal=   | translucent shards, drawn two sided so the far facets    |
    |             | show through the near ones                               |
    | =wireframe= | solids drawn as nothing but their edges                  |
    | =lowpoly=   | a flat-shaded field of ridges running to the horizon     |
    | =waves=     | the same sheet as an open swell, crests lit and foamed   |
    | =matrix=    | columns of glyphs falling, at a range of depths          |
    | =fog=       | soft drifting haze; no geometry at all                   |
    | =topology=  | contour lines over a field that is slowly eroding        |
    | =rings=     | luminous hoops round a lit core, each with a bead of     |
    |             | light running round it, and debris between them          |
    | =orrery=    | a faceted body rippling inside luminous hoops, with      |
    |             | debris tumbling round it                                |
    | =vortex=    | filaments twisted round an axis over a star field        |
    | =bloom=     | a phyllotactic flower unfurling from a bud and shutting  |
    | =plume=     | a landscape of contour ridges running to a horizon, with |
    |             | a column of smoke rising behind                          |
    | =dust=      | a planet of dust grains with a ring of debris round it   |

    The document has the last word, which is the setting that matters - a
    backdrop is the first thing to go when a projector cannot cope or the room
    cannot read it:

    | Setting                    | Means                                       |
    |----------------------------+---------------------------------------------|
    | =#+SLIDE_BACKDROP: nil=    | no backdrop, whatever the theme asked for   |
    | =#+SLIDE_BACKDROP: globe=  | that scene instead of the theme's own       |
    | =#+SLIDE_BACKDROP_SPEED:=  | a multiplier; =0= is one frame and then stop |
    | =#+SLIDE_BACKDROP_DENSITY:= | a multiplier on how much of it there is    |
    | =#+SLIDE_BACKDROP_MOTION: always= | animate even under reduced motion   |

    It is drawn to be looked *past*: the distance fades into the deck's own
    ground, the theme can wash the whole thing down with =--backdrop-veil=, and
    text over it gets a shadow of that same ground. The loop stops when the tab
    is not in front, and =@media print= leaves it out altogether.

    **=prefers-reduced-motion= is answered with one composed frame** rather than
    with a blank - which is right, and is also the first thing to check when a
    backdrop is not moving: on macOS that is System Settings → Accessibility →
    Display → Reduce motion, and it stops every one of these dead. A deck can
    overrule it with =#+SLIDE_BACKDROP_MOTION: always=, and a theme built around
    its scene moving can with =--backdrop-motion: always=. Whatever it decided,
    =ORG_BACKDROP.state= in a browser console says which of "running", "still:
    prefers-reduced-motion" and "still: speed is 0" it is, because from the
    outside those three look identical.

    The clock is =requestAnimationFrame= **and a plain timer running alongside
    it**, whichever gets there first, because rAF is also stopped by a window
    the compositor thinks is occluded, by an iframe that is not being painted
    and by a few remote-desktop setups - all of which are states a deck is
    genuinely being looked at in, and in every one of them a single-clock
    backdrop paints its first frame and then sits there looking like a still.

*** Making too much fit

   **Too much on one slide is shrunk to fit.** Every one of these frameworks
   simply does not draw the overflow, which makes an overfull slide look
   finished right up until it is on a projector. The content of each slide is
   measured against the slide and scaled down when it does not fit.

   | Setting            | Means                                               |
   |--------------------+-----------------------------------------------------|
   | =#+SLIDE_FIT: nil= | turn it off for the whole deck                      |
   | =:FIT: nil=        | leave this slide alone                              |
   | =:FIT: grow=       | let a thin slide scale *up* as well                 |
   | =:FIT: 0.3=        | shrink as far as three tenths before giving up      |

   It only ever shrinks (a deck whose type size changes slide by slide looks
   worse than one with white space in it), and it stops at 45% - past that the
   slide is not overfull, it is a document, and you want to notice that while
   writing the deck rather than while giving it.

EDOC */
package webslides

import (
	"fmt"
	"os"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/op/go-logging.v1"
)

var wsver = "1.5.0"
var cdn = "https://cdn.jsdelivr.net/gh/webslides/webslides@" + wsver
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver
var animatecdn = "https://cdnjs.cloudflare.com/ajax/libs/animate.css/4.1.1/animate.min.css"

// The colour grounds WebSlides ships as classes. A theme name that is one of
// these is the class; anything else is taken as a css colour and written as a
// style, so a deck can be any colour without this list growing.
var bgClasses = map[string]string{
	"light": "", "white": "bg-white", "black": "bg-black", "dark": "bg-black",
	"primary": "bg-primary", "secondary": "bg-secondary", "apple": "bg-apple",
	"blue": "bg-blue", "gradient": "bg-gradient-v", "gradient-v": "bg-gradient-v",
	"gradient-h": "bg-gradient-h", "gradient-r": "bg-gradient-r",
}

type WebSlidesExporter struct {
	TemplatePath string
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *common.PluginManager
}

type WebSlidesWriter struct {
	*org.HTMLWriter
	exp   *WebSlidesExporter
	Opts  string
	conf  slides.Conf
	fitOn bool
	props map[string]interface{}
	pend  slides.Pending
	frag  slides.Fragment
}

func NewWebSlidesWriter(exp *WebSlidesExporter, props map[string]interface{}) *WebSlidesWriter {
	w := WebSlidesWriter{HTMLWriter: org.NewHTMLWriter(), exp: exp, props: props}
	// The circular reference is what makes the overrides below get called from
	// inside go-org's own walk.
	w.ExtendingWriter = &w
	w.NoWrapCodeBlock = true
	w.HighlightCodeBlock = slides.Highlighter("WEBSLIDES", props)
	return &w
}

// A keyword of ours is held for the next element rather than written out.
func (w *WebSlidesWriter) WriteKeyword(k org.Keyword) {
	if slides.IsAttrKeyword(k.Key, "WEBSLIDES") {
		w.pend.AddAttr(k.Value)
		return
	}
	w.HTMLWriter.WriteKeyword(k)
}

// WebSlides has no native per-item reveal - it is a scrolling page, and the
// thing it does instead is animate a slide as it arrives. So a fragmented list
// gets animate.css' own class per item, which is the nearest honest answer.
func (w *WebSlidesWriter) fragClass(style string) string {
	if style == "" {
		style = "fadeInUp"
	}
	return "animated " + style
}

func (w *WebSlidesWriter) WriteList(l org.List) {
	p := w.pend.Take()
	if p.Frag || w.frag.On {
		style := p.Style
		if style == "" {
			style = w.frag.Style
		}
		w.WriteString(slides.FragList(w.HTMLWriter, l, func(int) string {
			return w.fragClass(style)
		}))
		return
	}
	if p.Any() {
		w.WriteString(p.Wrap(w.WriteNodesAsString(l), "div", ""))
		return
	}
	w.HTMLWriter.WriteList(l)
}

func (w *WebSlidesWriter) WriteParagraph(p org.Paragraph) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(p), "div", w.fragClass(pend.Style)))
		return
	}
	w.HTMLWriter.WriteParagraph(p)
}

func (w *WebSlidesWriter) WriteTable(t org.Table) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(t), "div", w.fragClass(pend.Style)))
		return
	}
	w.HTMLWriter.WriteTable(t)
}

// A headline that reached the writer is one *inside* a slide - deeper than the
// slide level - so it is a heading and its children, with no section of its
// own. go-org's own version wraps it in an outline div and renumbers it, which
// is right for a document and wrong on a slide.
func (w *WebSlidesWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	lvl := h.Lvl + 1
	if lvl > 6 {
		lvl = 6
	}
	a := &slides.Attrs{}
	a.Class(w.conf.Str(slides.HeadlineProps(&h), "CLASS", ""))
	w.WriteString(fmt.Sprintf("<h%d%s>", lvl, a.String()))
	org.WriteNodes(w, h.Title...)
	w.WriteString(fmt.Sprintf("</h%d>\n", lvl))
	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
}

func (w *WebSlidesWriter) WriteRegularLink(l org.RegularLink) {
	if l.Protocol == "file" && l.Kind() == "image" {
		src := slides.Media(w.Document, w.exp.pm, w.Opts, l.URL)
		alt := strings.TrimPrefix(l.URL, "file:")
		if l.Description != nil {
			alt = strings.TrimPrefix(org.String(l.Description...), "file:")
		}
		w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s"/>`, src, alt))
		return
	}
	w.HTMLWriter.WriteRegularLink(l)
}

// ── Drawing a deck ──────────────────────────────────────────────────────────

func (self *WebSlidesExporter) renderSlide(w *WebSlidesWriter, s *slides.Slide, d *slides.Deck) string {
	c := w.conf
	// Properties come off the *slide* rather than off its headline: the parser
	// hoists a column-zero drawer out of the headline, and asking the headline
	// for it is how the other two exporters came to ignore every per-slide
	// setting in a file written by Emacs.
	h := s.Props()
	w.frag = slides.ReadFragment(c, h)

	sec := &slides.Attrs{}
	sec.ID(slides.SlideID(c, h, fmt.Sprintf("slide-%d", s.Num)))
	sec.Class("org-slide")
	// "viewport" because a WebSlides section grows with its content: its own
	// height is never smaller than what is on it, so fitting has to be against
	// a window's worth instead.
	sec.Set("data-fit-box", "viewport")
	sec.Class(slides.Classes(c, h)...)
	switch slides.Align(c, h) {
	case "center", "centre", "middle":
		sec.Class("aligncenter")
	case "left":
		sec.Class("alignleft")
	case "right":
		sec.Class("alignright")
	}

	bg := slides.ReadBackground(c, h, w.Opts, self.pm)
	sec.Class(slides.ReadInk(c, h, bg).Class())
	inner := ""
	switch bg.Kind {
	case slides.BgColor:
		if cls, ok := bgClasses[strings.ToLower(bg.Value)]; ok && cls != "" {
			sec.Class(cls)
		} else {
			sec.Style("background-color: " + bg.Value)
			sec.Class("ws-bg-custom")
		}
	case slides.BgGradient:
		sec.Style("background-image: " + bg.Value)
		sec.Class("ws-bg-custom")
	case slides.BgImage:
		// WebSlides' own way: a span that is the picture, so the slide's text
		// stays in the flow above it.
		span := &slides.Attrs{}
		span.Class("background")
		if p := bg.Position; p != "" {
			span.Class("background-" + strings.ReplaceAll(strings.ToLower(p), " ", "-"))
		}
		span.Style(fmt.Sprintf("background-image: url('%s')", bg.Value))
		if bg.Size != "" {
			span.Style("background-size: " + bg.Size)
		}
		if bg.Opacity != "" {
			span.Style("opacity: " + bg.Opacity)
		}
		inner += fmt.Sprintf("<span%s></span>\n", span.String())
	case slides.BgVideo:
		loop, muted := "", ""
		if bg.Loop {
			loop = " loop"
		}
		if bg.Muted {
			muted = " muted"
		}
		inner += fmt.Sprintf(`<video class="ws-bg-video" autoplay%s%s playsinline><source src="%s"></video>`+"\n",
			loop, muted, bg.Value)
	case slides.BgIframe:
		extra := ""
		if !bg.Interactive {
			extra = ` style="pointer-events: none"`
		}
		inner += fmt.Sprintf(`<iframe class="ws-bg-iframe" src="%s"%s></iframe>`+"\n", bg.Value, extra)
	}

	// The content, in a wrap unless the slide asked to fill the screen.
	fit := slides.ReadFit(c, h, w.fitOn)
	body := fit.Open()
	if s.Title != nil {
		lvl := 2
		if s.Level == 1 {
			lvl = 1
		}
		body += fmt.Sprintf("<h%d>%s</h%d>\n", lvl, w.WriteNodesAsString(s.Title...), lvl)
	}
	body += w.WriteNodesAsString(s.Body...)
	body += fit.Close()

	wrapOpt := c.Str(h, "WRAP", "")
	if slides.Truthy(wrapOpt) {
		wrap := &slides.Attrs{}
		wrap.Class("wrap")
		if wrapOpt != "" && !strings.EqualFold(wrapOpt, "t") && !strings.EqualFold(wrapOpt, "yes") {
			wrap.Class(wrapOpt)
		}
		wrap.Class(c.Str(h, "WRAP_CLASS", ""))
		if anim := c.Str(h, "ANIMATION", ""); anim != "" {
			wrap.Class("animated", anim)
		}
		inner += fmt.Sprintf("<div%s>\n%s</div>\n", wrap.String(), body)
	} else {
		inner += body
	}

	if notes := w.WriteNodesAsString(s.Notes...); notes != "" {
		inner += fmt.Sprintf("<aside class=\"ws-notes\" hidden>\n%s</aside>\n", notes)
	}
	return fmt.Sprintf("<section%s>\n%s</section>\n\n", sec.String(), inner)
}

func (self *WebSlidesExporter) titleSlide(w *WebSlidesWriter, d *slides.Deck) string {
	c := w.conf
	sec := &slides.Attrs{}
	sec.ID("slide-title")
	sec.Class("org-slide", "org-title-slide", "aligncenter")
	sec.Class(c.DocStr("TITLE_CLASS", ""))
	body := ""
	if d.Title != "" {
		body += fmt.Sprintf("<h1 class=\"text-landing\">%s</h1>\n", d.Title)
	}
	if d.Subtitle != "" {
		body += fmt.Sprintf("<p class=\"text-subtitle org-subtitle\">%s</p>\n", d.Subtitle)
	}
	if pre := w.WriteNodesAsString(d.Preamble...); pre != "" {
		body += pre
	}
	by := []string{}
	if d.Author != "" {
		by = append(by, d.Author)
	}
	if d.Date != "" {
		by = append(by, d.Date)
	}
	if len(by) > 0 {
		body += fmt.Sprintf("<p class=\"text-intro org-byline\">%s</p>\n", strings.Join(by, " &middot; "))
	}
	return fmt.Sprintf("<section%s>\n<div class=\"wrap\">\n%s</div>\n</section>\n\n", sec.String(), body)
}

func (s *WebSlidesExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(s)
}

func (self *WebSlidesExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, res := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(res), 0644)
}

func (self *WebSlidesExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.FindByFile(query)
	if f == nil {
		return fmt.Errorf("failed to find file in database: [%s]", query), ""
	}
	// A fresh map per export: the props carry things a block in the middle of
	// one deck switched on, and leaking those into the next deck exported by
	// the same running server is a page loading scripts it does not use.
	m := ValidateMap(copyProps(self.Props))
	c := slides.Conf{Doc: f, Prefix: "WEBSLIDES"}

	w := NewWebSlidesWriter(self, m)
	w.Opts = opts
	w.conf = c
	w.fitOn = c.DocBool("FIT", true)
	w.Document = f

	deck := slides.BuildDeck(c, f)
	out := strings.Builder{}
	if deck.TitleSlide {
		out.WriteString(self.titleSlide(w, deck))
	}
	for _, s := range deck.Flat() {
		out.WriteString(self.renderSlide(w, s, deck))
	}

	m["slide_data"] = out.String()
	m["title"] = firstNonEmpty(deck.Title, m["title"])
	m["vertical"] = c.DocBool("VERTICAL", false)
	m["icons"] = c.DocBool("ICONS", true)
	m["animate"] = c.DocBool("ANIMATE", true)
	m["extra_css"] = c.DocStr("CSS", "")
	m["head"] = c.DocStr("HEAD", "")
	m["notes"] = c.DocBool("NOTES", true)
	m["hljs_style"] = c.DocStr("HIGHLIGHT_STYLE", asString(m["hljs_style"]))
	m["fontfamily"] = c.DocStr("FONT", asString(m["fontfamily"]))
	m["options"] = options(c)
	// Too much on a slide is shrunk to fit unless the deck says otherwise. The
	// stylesheet and the script are inlined rather than linked: a deck is often
	// saved and shown from disk, and a file that stops working the moment it
	// leaves the server is not an export.
	m["autofit"] = c.DocBool("FIT", true)
	if m["autofit"] == true {
		m["autofit_css"], m["autofit_js"] = slides.AutoFitAssets()
	}
	m["theme"] = strings.ToLower(c.DocStr("THEME", "light"))
	if v := props["theme"]; v != "" {
		m["theme"] = strings.ToLower(v)
	}
	// One of ours, or one of WebSlides' own colour grounds.
	th := slides.LoadTheme(asString(m["theme"]))
	m["local_theme"] = th.Found
	m["mermaid_theme"] = th.Mermaid
	m["themedata"] = th.CSS
	if th.Hljs != "" && c.DocStr("HIGHLIGHT_STYLE", "") == "" {
		m["hljs_style"] = th.Hljs
	}
	// The animated backdrop, when the theme asked for one (`@backdrop:` in its
	// own comment header) or the file did. Inlined like the autofit pair and for
	// the same reason: a deck is often shown from a memory stick.
	bd := slides.ReadBackdrop(c, th)
	m["backdrop"] = bd.Scene
	m["backdrop_config"] = bd.Config
	m["backdrop_css"] = bd.CSS
	m["backdrop_js"] = bd.JS
	m["theme_class"] = themeClass(asString(m["theme"]))
	m["slide_count"] = deck.Count()

	return nil, self.pm.Tempo.RenderTemplate(self.TemplatePath, m)
}

// options is the WebSlides constructor argument, built here rather than in the
// template because a javascript object with pongo2 conditionals in it is a
// syntax error waiting for the one deck that sets two of them.
func options(c slides.Conf) string {
	opts := []string{}
	add := func(k, v string) { opts = append(opts, fmt.Sprintf("%s: %s", k, v)) }
	if v := c.DocStr("AUTOSLIDE", ""); v != "" {
		if slides.Truthy(v) {
			add("autoslide", v)
		} else {
			add("autoslide", "false")
		}
	}
	add("loop", boolJS(c.DocBool("LOOP", true)))
	add("showIndex", boolJS(c.DocBool("SHOW_INDEX", true)))
	add("changeOnClick", boolJS(c.DocBool("CHANGE_ON_CLICK", false)))
	add("navigateOnScroll", boolJS(c.DocBool("NAVIGATE_ON_SCROLL", true)))
	if n := c.DocInt("SCROLL_WAIT", -1); n >= 0 {
		add("scrollWait", fmt.Sprintf("%d", n))
	}
	if n := c.DocInt("SLIDE_OFFSET", -1); n >= 0 {
		add("slideOffset", fmt.Sprintf("%d", n))
	}
	if n := c.DocInt("MIN_WHEEL_DELTA", -1); n >= 0 {
		add("minWheelDelta", fmt.Sprintf("%d", n))
	}
	return "{ " + strings.Join(opts, ", ") + " }"
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func themeClass(theme string) string {
	if cls, ok := bgClasses[theme]; ok {
		return cls
	}
	return ""
}

func copyProps(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(a string, b interface{}) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return asString(b)
}

func (self *WebSlidesExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	def := func(k string, v interface{}) {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	def("title", "Presentation")
	def("webslides_cdn", cdn)
	def("hljs_cdn", hljscdn)
	def("animate_cdn", animatecdn)
	def("hljs_style", "atom-one-dark")
	def("fontfamily", "Roboto")
	def("theme", "light")
	def("wordcloud", false)
	def("mermaid", false)
	if _, ok := m["stylesheet"]; !ok {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("webslides_style.css")); err == nil {
			m["stylesheet"] = string(data)
		} else {
			m["stylesheet"] = ""
		}
	}
	return m
}

// init function is called at boot
func init() {
	common.AddExporter("webslides", func() common.Exporter {
		return &WebSlidesExporter{
			Props:        ValidateMap(map[string]interface{}{}),
			TemplatePath: "webslides_default.tpl",
		}
	})
}

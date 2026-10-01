//lint:file-ignore ST1006 allow the use of self
// EXPORTER: Reveal JS
/* SDOC: Exporters

* Reveal JS

  [[https://revealjs.com][reveal.js]] is the one with the 3d transitions, the
  overview mode, the speaker view and the pdf export. It is the default answer
  for "I need to give a talk".

  #+BEGIN_SRC yaml
    - name: "revealjs"
      templatepath: "path to reveal template"
  #+END_SRC

  Every headline is a slide, and a headline nested under another is a slide
  *below* it - reveal's vertical stacks, which is how an outline becomes two
  dimensions. =#+SLIDE_LEVEL: 2= instead makes level-3 headlines headings on
  their slide rather than slides of their own.

** Document settings

   Everything reveal.js can be configured with is reachable, and *only what the
   file asks for is written out* - reveal's own defaults stand for the rest.

   | Keyword                        | reveal.js option            |
   |--------------------------------+-----------------------------|
   | =#+REVEAL_THEME:=              | the stylesheet: =black=, =white=, =league=, =beige=, =sky=, =night=, =serif=, =simple=, =solarized=, =blood=, =moon=, =dracula= |
   | =#+REVEAL_TRANSITION:=         | =none=, =fade=, =slide=, =convex=, =concave=, =zoom= |
   | =#+REVEAL_TRANSITION_SPEED:=   | =default=, =fast=, =slow=   |
   | =#+REVEAL_BACKGROUND_TRANSITION:= | the same, for backgrounds |
   | =#+REVEAL_CONTROLS:=           | =controls=                  |
   | =#+REVEAL_PROGRESS:=           | =progress=                  |
   | =#+REVEAL_SLIDE_NUMBER:=       | =slideNumber= - =t=, or a format like =c/t= |
   | =#+REVEAL_HASH:=               | =hash= - put the slide in the url |
   | =#+REVEAL_HISTORY:=            | =history=                   |
   | =#+REVEAL_KEYBOARD:=           | =keyboard=                  |
   | =#+REVEAL_OVERVIEW:=           | =overview=                  |
   | =#+REVEAL_CENTER:=             | =center=                    |
   | =#+REVEAL_TOUCH:=              | =touch=                     |
   | =#+REVEAL_LOOP:=               | =loop=                      |
   | =#+REVEAL_RTL:=                | =rtl=                       |
   | =#+REVEAL_SHUFFLE:=            | =shuffle=                   |
   | =#+REVEAL_FRAGMENTS:=          | =fragments=                 |
   | =#+REVEAL_EMBEDDED:=           | =embedded=                  |
   | =#+REVEAL_HELP:=               | =help=                      |
   | =#+REVEAL_SHOW_NOTES:=         | =showNotes=                 |
   | =#+REVEAL_AUTO_SLIDE:=         | =autoSlide= - milliseconds, or nil |
   | =#+REVEAL_AUTO_SLIDE_STOPPABLE:= | =autoSlideStoppable=      |
   | =#+REVEAL_AUTO_ANIMATE:=       | =autoAnimate=               |
   | =#+REVEAL_MOUSE_WHEEL:=        | =mouseWheel=                |
   | =#+REVEAL_PREVIEW_LINKS:=      | =previewLinks=              |
   | =#+REVEAL_VIEW_DISTANCE:=      | =viewDistance=              |
   | =#+REVEAL_WIDTH:= =#+REVEAL_HEIGHT:= | the coordinate system |
   | =#+REVEAL_MARGIN:= =#+REVEAL_MIN_SCALE:= =#+REVEAL_MAX_SCALE:= | the scaling |
   | =#+REVEAL_NAVIGATION_MODE:=    | =default=, =linear=, =grid=  |
   | =#+REVEAL_DISPLAY:=            | =display=                   |
   | =#+REVEAL_HIDE_CURSOR_TIME:=   | =hideCursorTime=            |
   | =#+REVEAL_PARALLAX_BACKGROUND_IMAGE:= | and =_SIZE=, =_HORIZONTAL=, =_VERTICAL= |
   | =#+REVEAL_PDF_MAX_PAGES_PER_SLIDE:= | for printing            |
   | =#+REVEAL_MATH:=               | =katex=, =mathjax=, or nil   |
   | =#+REVEAL_PLUGINS:=            | which plugins to load: =notes highlight math search zoom markdown= |
   | =#+REVEAL_HIGHLIGHT_STYLE:=    | a highlight.js style name    |
   | =#+REVEAL_CSS:= =#+REVEAL_HEAD:= | a stylesheet url, and raw head html |
   | =#+SLIDE_LEVEL:=               | which headline depth is a slide |
   | =#+SLIDE_TITLE_SLIDE:=         | nil to drop the generated title slide |

** Slide properties

   | Property                 | Becomes                                     |
   |--------------------------+---------------------------------------------|
   | =:TRANSITION:=           | =data-transition=                           |
   | =:TRANSITION_SPEED:=     | =data-transition-speed=                     |
   | =:AUTO_ANIMATE:=         | =data-auto-animate=                         |
   | =:AUTO_SLIDE:=           | =data-autoslide=                            |
   | =:BACKGROUND:=           | a colour, image, video or page - worked out  |
   |                          | from the value                              |
   | =:BACKGROUND_IMAGE:= =:BACKGROUND_COLOR:= =:BACKGROUND_VIDEO:= =:BACKGROUND_IFRAME:= | said outright |
   | =:BACKGROUND_SIZE:= =:BACKGROUND_POSITION:= =:BACKGROUND_REPEAT:= =:BACKGROUND_OPACITY:= | how to draw it |
   | =:BACKGROUND_TRANSITION:= | just this slide's                          |
   | =:BACKGROUND_INTERACTIVE:= | an iframe background that takes clicks    |
   | =:STATE:=                | =data-state=, which puts a class on the page |
   | =:TIMING:=               | =data-timing=, for the speaker view's clock |
   | =:VISIBILITY:=           | =hidden= or =uncounted=                      |
   | =:CLASS:=                | classes on the section                      |
   | =:CUSTOM_ID:=            | the section's id, so it can be linked to     |
   | =:FRAGMENT:=             | this slide's bullets appear one at a time,    |
   |                          | optionally in a named style (=fade-up=,      |
   |                          | =highlight-red=, =grow=)                     |
   | =:NOTES:=                | speaker notes - a drawer, a =#+BEGIN_NOTES=  |
   |                          | block or a child heading called Notes        |

   The reveal.js speaker view (=s=) shows the notes, the next slide and the
   clock, and only works when the page is served rather than opened off disk.

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

package revealjs

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

var rver = "5.1.0"
var cdn = "https://cdnjs.cloudflare.com/ajax/libs/reveal.js/" + rver
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

// The plugins loaded unless the file says otherwise. These are the four that
// are worth having on every deck: notes is the speaker view, highlight is
// source blocks, search is ctrl-shift-f and zoom is alt-click.
//
// They are loaded with script tags and named in `plugins: [...]`, which is how
// reveal.js 4 and 5 take plugins. The old `dependencies: [...]` list this
// template used was removed in reveal 4.0 and had been doing nothing for two
// major versions: no speaker notes, no search, no zoom, and no math.
var defaultPlugins = []string{"notes", "highlight", "search", "zoom"}

var pluginObject = map[string]string{
	"notes":     "RevealNotes",
	"highlight": "RevealHighlight",
	"search":    "RevealSearch",
	"zoom":      "RevealZoom",
	"markdown":  "RevealMarkdown",
	"math":      "RevealMath.KaTeX",
}

type RevealExporter struct {
	TemplatePath string
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *common.PluginManager
}

type RevealWriter struct {
	*org.HTMLWriter
	exp              *RevealExporter
	PostWriteScripts string
	Opts             string
	conf             slides.Conf
	fitOn            bool
	props            map[string]interface{}
	pend             slides.Pending
	frag             slides.Fragment
}

func NewRevealWriter(exp *RevealExporter, props map[string]interface{}) *RevealWriter {
	// This lovely bit of circular reference ensures that we get called when
	// exporting for any methods we have overwritten.
	w := RevealWriter{HTMLWriter: org.NewHTMLWriter(), exp: exp, props: props}
	w.ExtendingWriter = &w
	w.NoWrapCodeBlock = true
	w.HighlightCodeBlock = slides.Highlighter("REVEAL", props)
	return &w
}

func (w *RevealWriter) WriteKeyword(k org.Keyword) {
	if slides.IsAttrKeyword(k.Key, "REVEAL") {
		w.pend.AddAttr(k.Value)
		return
	}
	w.HTMLWriter.WriteKeyword(k)
}

// reveal.js' fragment vocabulary is wide - fade-in, fade-up, grow, shrink,
// highlight-red, semi-fade-out - and it is passed straight through, because
// the only thing this end knows is that `fragment` has to be on the element
// reveal is going to hide.
func fragClass(style string) string {
	if style == "" {
		return "fragment"
	}
	return "fragment " + style
}

func (w *RevealWriter) WriteList(l org.List) {
	p := w.pend.Take()
	if p.Frag || w.frag.On {
		style := p.Style
		if style == "" {
			style = w.frag.Style
		}
		w.WriteString(slides.FragList(w.HTMLWriter, l, func(int) string { return fragClass(style) }))
		return
	}
	if p.Any() {
		w.WriteString(p.Wrap(w.WriteNodesAsString(l), "div", fragClass(p.Style)))
		return
	}
	w.HTMLWriter.WriteList(l)
}

func (w *RevealWriter) WriteParagraph(p org.Paragraph) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(p), "div", fragClass(pend.Style)))
		return
	}
	w.HTMLWriter.WriteParagraph(p)
}

func (w *RevealWriter) WriteTable(t org.Table) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(t), "div", fragClass(pend.Style)))
		return
	}
	w.HTMLWriter.WriteTable(t)
}

// A headline that reaches the writer is deeper than the slide level: a heading
// on a slide rather than a slide of its own.
func (w *RevealWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	lvl := h.Lvl + 1
	if lvl > 6 {
		lvl = 6
	}
	w.WriteString(fmt.Sprintf("<h%d>", lvl))
	org.WriteNodes(w, h.Title...)
	w.WriteString(fmt.Sprintf("</h%d>\n", lvl))
	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
}

func (w *RevealWriter) WriteRegularLink(l org.RegularLink) {
	if l.Protocol == "file" && l.Kind() == "image" {
		src := slides.Media(w.Document, w.exp.pm, w.Opts, l.URL)
		if l.Description == nil {
			w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s"/>`, src, strings.TrimPrefix(l.URL, "file:")))
		} else {
			desc := strings.TrimPrefix(org.String(l.Description...), "file:")
			w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s"/>`, src, desc))
		}
		return
	}
	w.HTMLWriter.WriteRegularLink(l)
}

// ── Drawing ─────────────────────────────────────────────────────────────────

// slideAttrs is everything reveal reads off a `<section>`.
func (self *RevealExporter) slideAttrs(w *RevealWriter, s *slides.Slide) *slides.Attrs {
	c := w.conf
	p := s.Props()
	a := &slides.Attrs{}
	a.ID(slides.SlideID(c, p, fmt.Sprintf("slide-%d", s.Num)))
	// The class every framework's slides share, so a theme can say something
	// about "a slide" without saying it four times.
	a.Class("org-slide")
	a.Class(slides.Classes(c, p)...)
	a.Set("data-transition", c.Str(p, "TRANSITION", ""))
	a.Set("data-transition-speed", c.Str(p, "TRANSITION_SPEED", ""))
	a.Set("data-autoslide", c.Str(p, "AUTO_SLIDE", ""))
	a.Set("data-state", c.Str(p, "STATE", ""))
	a.Set("data-timing", c.Str(p, "TIMING", ""))
	a.Set("data-visibility", c.Str(p, "VISIBILITY", ""))
	if c.Bool(p, "AUTO_ANIMATE", false) {
		a.Flag("data-auto-animate")
	}
	if v := c.Str(p, "AUTO_ANIMATE_ID", ""); v != "" {
		a.Set("data-auto-animate-id", v)
	}

	bg := slides.ReadBackground(c, p, w.Opts, self.pm)
	// Which way round the text has to go on whatever ground this slide asked
	// for. See slides.ReadInk: a dark background under a light theme is the
	// one combination that produces a slide nobody can read.
	a.Class(slides.ReadInk(c, p, bg).Class())
	switch bg.Kind {
	case slides.BgColor:
		a.Set("data-background-color", bg.Value)
	case slides.BgGradient:
		a.Set("data-background-gradient", bg.Value)
	case slides.BgImage:
		a.Set("data-background-image", bg.Value)
		a.Set("data-background-size", bg.Size)
		a.Set("data-background-position", bg.Position)
		a.Set("data-background-repeat", bg.Repeat)
		a.Set("data-background-opacity", bg.Opacity)
	case slides.BgVideo:
		a.Set("data-background-video", bg.Value)
		if bg.Loop {
			a.Flag("data-background-video-loop")
		}
		if bg.Muted {
			a.Flag("data-background-video-muted")
		}
	case slides.BgIframe:
		a.Set("data-background-iframe", bg.Value)
		if bg.Interactive {
			a.Flag("data-background-interactive")
		}
	}
	a.Set("data-background-transition", bg.Transition)
	return a
}

func (self *RevealExporter) renderSlide(w *RevealWriter, s *slides.Slide) string {
	w.frag = slides.ReadFragment(w.conf, s.Props())
	a := self.slideAttrs(w, s)

	// Everything the audience sees goes inside the fit wrapper; the speaker
	// notes do not, because they are not on the slide and scaling them would
	// be scaling the speaker view.
	fit := slides.ReadFit(w.conf, s.Props(), w.fitOn)
	inner := fit.Open()
	if s.Title != nil {
		lvl := s.Level + 1
		if lvl > 6 {
			lvl = 6
		}
		inner += fmt.Sprintf("<h%d>%s</h%d>\n", lvl, w.WriteNodesAsString(s.Title...), lvl)
	}
	inner += w.WriteNodesAsString(s.Body...)
	inner += fit.Close()
	// Speaker notes. reveal.js reads an <aside class="notes"> and shows it in
	// the speaker view and nowhere else - which is the whole point of writing
	// them in the org file rather than on a piece of paper.
	if notes := w.WriteNodesAsString(s.Notes...); notes != "" {
		inner += fmt.Sprintf("<aside class=\"notes\">\n%s</aside>\n", notes)
	}
	own := fmt.Sprintf("<section%s>\n%s</section>\n", a.String(), inner)

	if len(s.Subs) == 0 {
		return own + "\n"
	}
	// A stack: this slide, then the ones under it, inside an outer section.
	// reveal.js requires the outer one to contain *only* sections - the old
	// version of this exporter put the parent's own heading and body in the
	// stack wrapper, where reveal renders it behind the slides.
	stack := &slides.Attrs{}
	stack.Class(w.conf.Str(s.Props(), "STACK_CLASS", ""))
	out := fmt.Sprintf("<section%s>\n%s", stack.String(), own)
	for _, sub := range s.Subs {
		out += self.renderSlide(w, sub)
	}
	return out + "</section>\n\n"
}

func (self *RevealExporter) titleSlide(w *RevealWriter, d *slides.Deck) string {
	body := ""
	if d.Title != "" {
		body += fmt.Sprintf("<h1>%s</h1>\n", d.Title)
	}
	if d.Subtitle != "" {
		body += fmt.Sprintf("<h3 class=\"org-subtitle\">%s</h3>\n", d.Subtitle)
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
		body += fmt.Sprintf("<p class=\"org-byline\">%s</p>\n", strings.Join(by, " &middot; "))
	}
	a := &slides.Attrs{}
	a.ID("slide-title")
	a.Class("org-slide", "org-title-slide")
	a.Class(w.conf.DocStr("TITLE_CLASS", ""))
	bg := slides.ReadBackground(w.conf, nil, w.Opts, self.pm)
	if bg.Kind == slides.BgColor {
		a.Set("data-background-color", bg.Value)
	}
	return fmt.Sprintf("<section%s>\n%s</section>\n\n", a.String(), body)
}

func (s *RevealExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(s)
}

func (self *RevealExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, res := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(res), 0644)
}

func (self *RevealExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.FindByFile(query)
	if f == nil {
		return fmt.Errorf("failed to find file in database: [%s]", query), ""
	}
	// A fresh map per export: the props carry things a block in the middle of
	// one deck switched on, and leaking those into the next deck exported by
	// the same running server is a page loading scripts it does not use.
	m := ValidateMap(copyProps(self.Props))
	c := slides.Conf{Doc: f, Prefix: "REVEAL"}

	w := NewRevealWriter(self, m)
	w.Opts = opts
	w.conf = c
	w.fitOn = c.DocBool("FIT", true)
	w.Document = f

	deck := slides.BuildDeck(c, f)
	out := strings.Builder{}
	if deck.TitleSlide {
		out.WriteString(self.titleSlide(w, deck))
	}
	for _, s := range deck.Slides {
		out.WriteString(self.renderSlide(w, s))
	}

	m["slide_data"] = out.String()
	m["post_scripts"] = w.PostWriteScripts
	m["title"] = firstNonEmpty(deck.Title, m["title"])
	m["theme"] = c.DocStr("THEME", asString(m["theme"]))
	// A theme on the request wins: that is somebody in the presentations tab
	// trying one on, and the file keyword is what they are trying it against.
	if v := props["theme"]; v != "" {
		m["theme"] = v
	}
	// One of ours, or one of reveal's. A name this server has a file for is
	// inlined and reveal's own theme stylesheet is left off; anything else is
	// passed through, so `#+REVEAL_THEME: dracula` still means dracula.
	th := slides.LoadTheme(asString(m["theme"]))
	m["local_theme"] = th.Found
	m["mermaid_theme"] = th.Mermaid
	m["themedata"] = th.CSS
	// The animated backdrop, when the theme asked for one (`@backdrop:` in its
	// own comment header) or the file did. Inlined like the autofit pair and for
	// the same reason: a deck is often shown from a memory stick.
	bd := slides.ReadBackdrop(c, th)
	m["backdrop"] = bd.Scene
	m["backdrop_config"] = bd.Config
	m["backdrop_css"] = bd.CSS
	m["backdrop_js"] = bd.JS
	m["hljs_style"] = firstNonEmpty(c.DocStr("HIGHLIGHT_STYLE", ""),
		firstNonEmpty(th.Hljs, m["hljs_style"]))
	m["fontfamily"] = c.DocStr("FONT", asString(m["fontfamily"]))
	m["extra_css"] = c.DocStr("CSS", "")
	m["head"] = c.DocStr("HEAD", "")
	m["config"] = config(c)
	// Too much on a slide is shrunk to fit unless the deck says otherwise. The
	// stylesheet and the script are inlined rather than linked: a deck is often
	// saved and shown from disk, and a file that stops working the moment it
	// leaves the server is not an export.
	m["autofit"] = c.DocBool("FIT", true)
	if m["autofit"] == true {
		m["autofit_css"], m["autofit_js"] = slides.AutoFitAssets()
	}
	m["slide_count"] = deck.Count()

	math := strings.ToLower(c.DocStr("MATH", ""))
	plugins, scripts := pluginList(c, math)
	m["plugins"] = plugins
	m["plugin_scripts"] = scripts
	m["math"] = math
	return nil, self.pm.Tempo.RenderTemplate(self.TemplatePath, m)
}

// pluginList is which plugins to load, as script paths and as the javascript
// names that go in `plugins: [...]`.
func pluginList(c slides.Conf, math string) (string, []string) {
	want := defaultPlugins
	if v := c.DocStr("PLUGINS", ""); v != "" {
		if !slides.Truthy(v) {
			want = nil
		} else {
			want = strings.Fields(strings.ToLower(strings.NewReplacer(",", " ").Replace(v)))
		}
	}
	if math != "" && slides.Truthy(math) && !contains(want, "math") {
		want = append(want, "math")
	}
	names := []string{}
	paths := []string{}
	for _, p := range want {
		obj, ok := pluginObject[p]
		if !ok {
			continue
		}
		if p == "math" {
			switch math {
			case "mathjax", "mathjax3":
				obj = "RevealMath.MathJax3"
			case "mathjax2":
				obj = "RevealMath.MathJax2"
			}
		}
		names = append(names, obj)
		paths = append(paths, p)
	}
	return strings.Join(names, ", "), paths
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// config is the object handed to Reveal.initialize. Only what the file asked
// for is written: reveal's own defaults are better than this exporter's
// opinion, and they move between versions.
func config(c slides.Conf) string {
	o := slides.NewJSOpts(c)
	// Three the template used to hard-code. They are defaults rather than
	// fixtures now, so a deck can say otherwise - but they stay the defaults,
	// because a deck exported last week has to look the way it looked: reveal's
	// own `center: true` moves every slide.
	if !c.DocBool("CENTER", false) {
		o.Raw("center", "false")
	}
	if v := c.DocStr("NAVIGATION_MODE", ""); v == "" {
		o.Raw("navigationMode", "'grid'")
	}
	if v := c.DocStr("PDF_MAX_PAGES_PER_SLIDE", ""); v == "" {
		o.Raw("pdfMaxPagesPerSlide", "1")
	}
	o.Bool("CONTROLS", "controls")
	o.Bool("PROGRESS", "progress")
	o.Bool("HISTORY", "history")
	o.Bool("HASH", "hash")
	o.Bool("KEYBOARD", "keyboard")
	o.Bool("OVERVIEW", "overview")
	o.Bool("TOUCH", "touch")
	o.Bool("LOOP", "loop")
	o.Bool("RTL", "rtl")
	o.Bool("SHUFFLE", "shuffle")
	o.Bool("FRAGMENTS", "fragments")
	o.Bool("EMBEDDED", "embedded")
	o.Bool("HELP", "help")
	o.Bool("SHOW_NOTES", "showNotes")
	o.Bool("AUTO_SLIDE_STOPPABLE", "autoSlideStoppable")
	o.Bool("AUTO_ANIMATE", "autoAnimate")
	o.Bool("MOUSE_WHEEL", "mouseWheel")
	o.Bool("PREVIEW_LINKS", "previewLinks")
	o.Bool("HIDE_INACTIVE_CURSOR", "hideInactiveCursor")
	o.Bool("PAUSE", "pause")
	// slideNumber is either a switch or a format like 'c/t', so it is read as
	// a string and only turned into a boolean when it reads like one.
	if v := c.DocStr("SLIDE_NUMBER", ""); v != "" {
		if strings.ContainsAny(v, "/.") || strings.Contains(v, "c") && len(v) > 1 {
			o.Raw("slideNumber", "'"+v+"'")
		} else {
			o.Raw("slideNumber", fmt.Sprintf("%t", slides.Truthy(v)))
		}
	}
	o.NumOrBool("AUTO_SLIDE", "autoSlide")
	o.Num("VIEW_DISTANCE", "viewDistance")
	o.Num("MOBILE_VIEW_DISTANCE", "mobileViewDistance")
	o.Num("WIDTH", "width")
	o.Num("HEIGHT", "height")
	o.Num("MARGIN", "margin")
	o.Num("MIN_SCALE", "minScale")
	o.Num("MAX_SCALE", "maxScale")
	o.Num("HIDE_CURSOR_TIME", "hideCursorTime")
	o.Num("PDF_MAX_PAGES_PER_SLIDE", "pdfMaxPagesPerSlide")
	o.Str("TRANSITION", "transition")
	o.Str("TRANSITION_SPEED", "transitionSpeed")
	o.Str("BACKGROUND_TRANSITION", "backgroundTransition")
	o.Str("NAVIGATION_MODE", "navigationMode")
	o.Str("DISPLAY", "display")
	o.Str("PARALLAX_BACKGROUND_IMAGE", "parallaxBackgroundImage")
	o.Str("PARALLAX_BACKGROUND_SIZE", "parallaxBackgroundSize")
	o.Num("PARALLAX_BACKGROUND_HORIZONTAL", "parallaxBackgroundHorizontal")
	o.Num("PARALLAX_BACKGROUND_VERTICAL", "parallaxBackgroundVertical")
	return o.String()
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

func (self *RevealExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func NewHtmlExp() *RevealExporter {
	var g *RevealExporter = new(RevealExporter)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	def := func(k string, v interface{}) {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	def("title", "Presentation")
	def("reveal_cdn", cdn)
	def("hljs_cdn", hljscdn)
	def("hljs_style", "monokai")
	def("wordcloud", false)
	def("mermaid", false)
	def("fontfamily", "Inconsolata")
	def("theme", "league")
	def("trackheight", 30)
	if _, ok := m["stylesheet"]; !ok {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("reveal_style.css")); err == nil {
			m["stylesheet"] = string(data)
		} else {
			m["stylesheet"] = ""
		}
	}
	return m
}

// init function is called at boot
func init() {
	common.AddExporter("revealjs", func() common.Exporter {
		return &RevealExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "reveal_default.tpl"}
	})
}

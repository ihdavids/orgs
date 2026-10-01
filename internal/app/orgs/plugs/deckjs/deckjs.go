//lint:file-ignore ST1006 allow the use of self
// EXPORTER: deck.js
/* SDOC: Exporters

* deck.js

  [[https://github.com/imakewebthings/deck.js][deck.js]] is the oldest and
  plainest of the four: html, css and a handful of extensions, no 3d, no
  scrolling. What it has that the others do not is **nesting as stepping** - a
  slide inside a slide is a step inside a step - so an org outline maps onto it
  exactly, and a deck reveals itself in the shape it was written in.

  #+BEGIN_SRC yaml
    - name: "deckjs"
  #+END_SRC

** Document settings

   | Keyword                      | Means                                        |
   |------------------------------+----------------------------------------------|
   | =#+DECK_THEME:=              | =web-2.0=, =swiss=, =neon=                   |
   | =#+DECK_TRANSITION:=         | =horizontal-slide=, =vertical-slide=, =fade= |
   | =#+DECK_GOTO:=               | the go-to-slide form, and the =g= key        |
   | =#+DECK_MENU:=               | the slide grid, and the =m= key              |
   | =#+DECK_NAVIGATION:=         | the previous/next arrows                     |
   | =#+DECK_STATUS:=             | the =3 / 24= counter                         |
   | =#+DECK_SCALE:=              | scale slides to the window                   |
   | =#+DECK_PERMALINK:=          | a permalink to the current slide             |
   | =#+DECK_COUNT_NESTED:=       | whether sub-slides count in the total        |
   | =#+DECK_HASH_PREFIX:=        | the =#slide-3= prefix                        |
   | =#+DECK_SWIPE:=              | =horizontal=, =vertical= or nil              |
   | =#+DECK_BASE_HEIGHT:=        | the height the scale extension scales from   |
   | =#+DECK_HIGHLIGHT_STYLE:=    | a highlight.js style name                    |
   | =#+DECK_FONT:=               | a Google font family                         |
   | =#+DECK_JQUERY:= =#+DECK_CDN:= | where the libraries come from              |
   | =#+DECK_CSS:= =#+DECK_HEAD:= | a stylesheet url, and raw head html          |
   | =#+SLIDE_LEVEL:=             | which headline depth is a slide              |
   | =#+SLIDE_TITLE_SLIDE:=       | nil to drop the generated title slide        |

** Slide properties

   | Property        | Means                                                  |
   |-----------------+--------------------------------------------------------|
   | =:CLASS:=       | classes on the slide                                   |
   | =:ALIGN:=       | =center=, =left=, =right=                              |
   | =:BACKGROUND:=  | a colour, an image, a video or a page                  |
   | =:CUSTOM_ID:=   | the slide's id, which is also its permalink            |
   | =:FRAGMENT:=    | this slide's bullets appear one at a time, as nested    |
   |                 | deck.js slides                                         |
   | =:NOTES:=       | speaker notes - press =n=                              |

   A headline nested under a slide is a **sub-slide**: deck.js steps into it,
   so the parent stays on screen while its children arrive. That is what
   =#+SLIDE_LEVEL:= is for - with =2=, a level-3 headline is a step inside its
   slide rather than a heading in it.

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
package deckjs

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

var dver = "v1.1.0"
var cdn = "https://cdn.jsdelivr.net/gh/imakewebthings/deck.js@" + dver
var jquery = "https://cdnjs.cloudflare.com/ajax/libs/jquery/3.7.1/jquery.min.js"

// Modernizr is what deck.core.css tests for before it dares to animate
// anything: without it every `.csstransforms` rule is dead and the transition
// themes do nothing. 2.8.3 is the last of the 2.x line, which is what deck.js
// was built against.
var modernizr = "https://cdnjs.cloudflare.com/ajax/libs/modernizr/2.8.3/modernizr.min.js"
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

var styleThemes = map[string]bool{"web-2.0": true, "swiss": true, "neon": true}
var transitionThemes = map[string]bool{"horizontal-slide": true, "vertical-slide": true, "fade": true}

type DeckExporter struct {
	TemplatePath string
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *common.PluginManager
}

type DeckWriter struct {
	*org.HTMLWriter
	exp   *DeckExporter
	Opts  string
	conf  slides.Conf
	fitOn bool
	props map[string]interface{}
	pend  slides.Pending
	frag  slides.Fragment
}

func NewDeckWriter(exp *DeckExporter, props map[string]interface{}) *DeckWriter {
	w := DeckWriter{HTMLWriter: org.NewHTMLWriter(), exp: exp, props: props}
	w.ExtendingWriter = &w
	w.NoWrapCodeBlock = true
	w.HighlightCodeBlock = slides.Highlighter("DECK", props)
	return &w
}

func (w *DeckWriter) WriteKeyword(k org.Keyword) {
	if slides.IsAttrKeyword(k.Key, "DECK") {
		w.pend.AddAttr(k.Value)
		return
	}
	w.HTMLWriter.WriteKeyword(k)
}

// deck.js reveals a nested slide, so one revealed thing is a `.slide`. That is
// also why the fragment style is ignored here: there is one way to arrive.
func (w *DeckWriter) WriteList(l org.List) {
	p := w.pend.Take()
	if p.Frag || w.frag.On {
		w.WriteString(slides.FragList(w.HTMLWriter, l, func(int) string { return "slide" }))
		return
	}
	if p.Any() {
		w.WriteString(p.Wrap(w.WriteNodesAsString(l), "div", "slide"))
		return
	}
	w.HTMLWriter.WriteList(l)
}

func (w *DeckWriter) WriteParagraph(p org.Paragraph) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(p), "div", "slide"))
		return
	}
	w.HTMLWriter.WriteParagraph(p)
}

func (w *DeckWriter) WriteTable(t org.Table) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(t), "div", "slide"))
		return
	}
	w.HTMLWriter.WriteTable(t)
}

// A headline the writer sees is one deeper than the slide level: a heading on a
// slide rather than a slide of its own.
func (w *DeckWriter) WriteHeadline(h org.Headline) {
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

func (w *DeckWriter) WriteRegularLink(l org.RegularLink) {
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

// ── Drawing ─────────────────────────────────────────────────────────────────

// renderSlide draws one slide and, nested inside it, its sub-slides. The
// nesting is the point: deck.js treats a slide inside a slide as a step, so the
// outline's shape is the order things appear in.
func (self *DeckExporter) renderSlide(w *DeckWriter, s *slides.Slide, depth int) string {
	c := w.conf
	p := s.Props()
	w.frag = slides.ReadFragment(c, p)

	a := &slides.Attrs{}
	a.Class("slide", "org-slide")
	a.ID(slides.SlideID(c, p, fmt.Sprintf("slide-%d", s.Num)))
	a.Class(slides.Classes(c, p)...)
	vcenter := false
	switch slides.Align(c, p) {
	case "center", "centre", "middle":
		// `vcenter` is deck.js' own idea, and all three themes define it: a
		// *wrapper inside* the slide whose content is centred in it, with the
		// h1 put back into the flow (`.vcenter h1 { position: relative }`).
		// It has to be a wrapper rather than a class on the section - the
		// themes position `.vcenter` absolutely, and doing that to the section
		// fights deck.js' own positioning and lands the slide off-screen.
		a.Class("deck-align-center")
		vcenter = true
	case "left":
		a.Class("deck-align-left")
	case "right":
		a.Class("deck-align-right")
	}

	bg := slides.ReadBackground(c, p, w.Opts, self.pm)
	a.Class(slides.ReadInk(c, p, bg).Class())
	inner := ""
	switch bg.Kind {
	case slides.BgColor, slides.BgGradient:
		// deck.js has no idea of backgrounds, so this is css, which is all a
		// background ever was.
		a.Style(bg.CSS())
	case slides.BgImage:
		if bg.Opacity != "" {
			// A faded picture has to be on a layer of its own: fading the
			// slide's own background takes the words with it.
			a.Class("deck-bg-dim")
			a.Style(bg.CSSVars("deck-bg"))
		} else {
			a.Style(bg.CSS())
		}
	case slides.BgVideo:
		loop, muted := "", ""
		if bg.Loop {
			loop = " loop"
		}
		if bg.Muted {
			muted = " muted"
		}
		inner += fmt.Sprintf(`<video class="deck-bg-video" autoplay%s%s playsinline><source src="%s"></video>`+"\n", loop, muted, bg.Value)
	case slides.BgIframe:
		extra := ""
		if !bg.Interactive {
			extra = ` style="pointer-events: none"`
		}
		inner += fmt.Sprintf(`<iframe class="deck-bg-iframe" src="%s"%s></iframe>`+"\n", bg.Value, extra)
	}

	before := inner
	fit := slides.ReadFit(c, p, w.fitOn)
	inner = fit.Open()
	if s.Title != nil {
		lvl := depth + 1
		if lvl > 4 {
			lvl = 4
		}
		inner += fmt.Sprintf("<h%d>%s</h%d>\n", lvl, w.WriteNodesAsString(s.Title...), lvl)
	}
	inner += w.WriteNodesAsString(s.Body...)
	inner += fit.Close()
	if vcenter {
		// Only this slide's own content is centred: a sub-slide is a step of
		// its own and does its own centring, and a background element is not
		// content at all.
		inner = fmt.Sprintf("<div class=\"vcenter\">\n%s</div>\n", inner)
	}
	inner = before + inner
	if notes := w.WriteNodesAsString(s.Notes...); notes != "" {
		inner += fmt.Sprintf("<div class=\"deck-notes\" hidden>\n%s</div>\n", notes)
	}
	for _, sub := range s.Subs {
		inner += self.renderSlide(w, sub, depth+1)
	}
	return fmt.Sprintf("<section%s>\n%s</section>\n\n", a.String(), inner)
}

func (self *DeckExporter) titleSlide(w *DeckWriter, d *slides.Deck) string {
	body := ""
	if d.Title != "" {
		body += fmt.Sprintf("<h1>%s</h1>\n", d.Title)
	}
	if d.Subtitle != "" {
		body += fmt.Sprintf("<h2 class=\"deck-subtitle org-subtitle\">%s</h2>\n", d.Subtitle)
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
		body += fmt.Sprintf("<p class=\"deck-byline org-byline\">%s</p>\n", strings.Join(by, " &middot; "))
	}
	// Wrapped in vcenter for the same reason: a title slide is a title, a
	// subtitle and a byline, and the themes position a lone h1 on the
	// assumption that it is the only thing on the slide.
	return fmt.Sprintf("<section class=\"slide org-slide org-title-slide deck-title-slide\" id=\"slide-title\">\n<div class=\"vcenter\">\n%s</div>\n</section>\n\n", body)
}

func (s *DeckExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(s)
}

func (self *DeckExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, res := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(res), 0644)
}

func (self *DeckExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.FindByFile(query)
	if f == nil {
		return fmt.Errorf("failed to find file in database: [%s]", query), ""
	}
	m := ValidateMap(copyProps(self.Props))
	c := slides.Conf{Doc: f, Prefix: "DECK"}

	w := NewDeckWriter(self, m)
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
		out.WriteString(self.renderSlide(w, s, 1))
	}

	m["slide_data"] = out.String()
	m["title"] = firstNonEmpty(deck.Title, m["title"])
	m["deck_cdn"] = c.DocStr("CDN", asString(m["deck_cdn"]))
	m["jquery"] = c.DocStr("JQUERY", asString(m["jquery"]))
	m["theme"] = themeOr(c.DocStr("THEME", asString(m["theme"])), styleThemes, "web-2.0")
	m["transition"] = themeOr(c.DocStr("TRANSITION", asString(m["transition"])), transitionThemes, "horizontal-slide")
	if v := props["theme"]; v != "" {
		m["theme"] = v
	}
	if v := props["transition"]; v != "" {
		m["transition"] = v
	}
	// One of ours, or one of deck.js' three. A local theme replaces the style
	// theme and leaves the *transition* theme alone - how a slide moves is not
	// what it looks like.
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
	if th.Hljs != "" && c.DocStr("HIGHLIGHT_STYLE", "") == "" {
		m["hljs_style"] = th.Hljs
	}
	m["hljs_style"] = c.DocStr("HIGHLIGHT_STYLE", asString(m["hljs_style"]))
	m["fontfamily"] = c.DocStr("FONT", asString(m["fontfamily"]))
	m["extra_css"] = c.DocStr("CSS", "")
	m["head"] = c.DocStr("HEAD", "")
	m["goto"] = c.DocBool("GOTO", true)
	m["menu"] = c.DocBool("MENU", true)
	m["navigation"] = c.DocBool("NAVIGATION", true)
	m["status"] = c.DocBool("STATUS", true)
	m["scale"] = c.DocBool("SCALE", true)
	m["permalink"] = c.DocBool("PERMALINK", false)
	m["notes"] = c.DocBool("NOTES", true)
	m["options"] = options(c)
	// Too much on a slide is shrunk to fit unless the deck says otherwise. The
	// stylesheet and the script are inlined rather than linked: a deck is often
	// saved and shown from disk, and a file that stops working the moment it
	// leaves the server is not an export.
	m["autofit"] = c.DocBool("FIT", true)
	if m["autofit"] == true {
		m["autofit_css"], m["autofit_js"] = slides.AutoFitAssets()
	}
	m["slide_count"] = deck.Count()
	return nil, self.pm.Tempo.RenderTemplate(self.TemplatePath, m)
}

// options is the object handed to $.deck(). Only the ones somebody would
// actually change are reachable: the class and selector names are deck.js'
// own business, and a deck that renamed them would need its css renamed too.
func options(c slides.Conf) string {
	opts := []string{}
	add := func(k, v string) { opts = append(opts, fmt.Sprintf("%s: %s", k, v)) }
	add("countNested", boolJS(c.DocBool("COUNT_NESTED", true)))
	if v := c.DocStr("HASH_PREFIX", ""); v != "" {
		add("hashPrefix", fmt.Sprintf("'%s'", v))
	}
	add("preventFragmentScroll", boolJS(c.DocBool("PREVENT_FRAGMENT_SCROLL", true)))
	if n := c.DocInt("INIT_LOCK_TIMEOUT", -1); n >= 0 {
		add("initLockTimeout", fmt.Sprintf("%d", n))
	}
	touch := []string{}
	if v := c.DocStr("SWIPE", ""); v != "" {
		if slides.Truthy(v) {
			touch = append(touch, fmt.Sprintf("swipeDirection: '%s'", v))
		} else {
			touch = append(touch, "swipeDirection: 'none'")
		}
	}
	if n := c.DocInt("SWIPE_TOLERANCE", -1); n >= 0 {
		touch = append(touch, fmt.Sprintf("swipeTolerance: %d", n))
	}
	if len(touch) > 0 {
		add("touch", "{ "+strings.Join(touch, ", ")+" }")
	}
	if n := c.DocInt("BASE_HEIGHT", -1); n > 0 {
		add("baseHeight", fmt.Sprintf("%d", n))
	}
	return "{ " + strings.Join(opts, ", ") + " }"
}

func themeOr(v string, known map[string]bool, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	// A name this build does not know is passed through anyway: it may be a
	// theme somebody added to their own css, and refusing it would be this
	// code deciding what themes exist.
	if known[v] || strings.Contains(v, "/") || strings.HasSuffix(v, ".css") {
		return v
	}
	return v
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
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

func (self *DeckExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
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
	def("deck_cdn", cdn)
	def("jquery", jquery)
	def("modernizr", modernizr)
	def("hljs_cdn", hljscdn)
	def("hljs_style", "atom-one-dark")
	def("theme", "web-2.0")
	def("transition", "horizontal-slide")
	def("fontfamily", "")
	def("wordcloud", false)
	def("mermaid", false)
	if _, ok := m["stylesheet"]; !ok {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("deck_style.css")); err == nil {
			m["stylesheet"] = string(data)
		} else {
			m["stylesheet"] = ""
		}
	}
	return m
}

// init function is called at boot
func init() {
	common.AddExporter("deckjs", func() common.Exporter {
		return &DeckExporter{
			Props:        ValidateMap(map[string]interface{}{}),
			TemplatePath: "deck_default.tpl",
		}
	})
}

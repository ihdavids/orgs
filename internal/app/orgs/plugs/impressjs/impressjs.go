//lint:file-ignore ST1006 allow the use of self
// EXPORTER: IMPRESS JS Export
/* SDOC: Exporters

* ImpressJs

  [[https://github.com/impress/impress.js][impress.js]] is the one that is not
  a stack of slides at all: every step is placed somewhere on an infinite
  canvas, and the camera flies between them. Done well it is the only one of
  the four that can *show* a structure - a timeline, a map, a zoom from the
  whole system down to one box of it.

  #+BEGIN_SRC yaml
    - name: "impressjs"
  #+END_SRC

  Placing twenty steps by hand is the part nobody enjoys, so
  =#+IMPRESS_LAYOUT:= will do it:

  | Layout     | What it does                                             |
  |------------+----------------------------------------------------------|
  | =rel=      | each step down and right of the last (the default)       |
  | =line=     | straight across                                          |
  | =grid=     | a square of steps, left to right, top to bottom          |
  | =spiral=   | winding outwards, each step turned to follow the curve   |
  | =ring=     | a circle, every step facing the middle                   |
  | =zoom=     | each step deeper into the screen than the last           |
  | =random=   | scattered, but the *same* scatter every export           |
  | =none=     | nothing placed - the properties below do it all          |

  =#+IMPRESS_SPREAD:= is how far apart (default 1600), and any step can still
  say exactly where it goes.

** Document settings

   | Keyword                         | Means                                   |
   |---------------------------------+-----------------------------------------|
   | =#+IMPRESS_THEME:=              | a =templates/impress_theme_NAME.css=    |
   | =#+IMPRESS_LAYOUT:=             | see above                               |
   | =#+IMPRESS_SPREAD:=             | the distance between steps              |
   | =#+IMPRESS_WIDTH:= =#+IMPRESS_HEIGHT:= | the canvas coordinate system     |
   | =#+IMPRESS_MAX_SCALE:= =#+IMPRESS_MIN_SCALE:= | how far it may scale      |
   | =#+IMPRESS_PERSPECTIVE:=        | how strong the 3d is (0 turns it off)   |
   | =#+IMPRESS_TRANSITION_DURATION:= | milliseconds per flight                |
   | =#+IMPRESS_OVERVIEW:=           | the zoomed-out step you land on first   |
   | =#+IMPRESS_TOOLBAR:=            | impress.js' own toolbar                 |
   | =#+IMPRESS_PROGRESS:=           | the progress bar                        |
   | =#+IMPRESS_HINT:=               | the "use a space bar" hint              |
   | =#+IMPRESS_AUTOPLAY:=           | seconds per step, for an unattended deck |
   | =#+IMPRESS_HIGHLIGHT_STYLE:=    | a highlight.js style name               |
   | =#+IMPRESS_FONT:=               | a Google font family                    |
   | =#+IMPRESS_CSS:= =#+IMPRESS_HEAD:= | a stylesheet url, and raw head html  |
   | =#+SLIDE_LEVEL:=                | which headline depth is a step          |
   | =#+SLIDE_TITLE_SLIDE:=          | nil to drop the generated title step    |

** Step properties

   | Property                       | Becomes                               |
   |--------------------------------+---------------------------------------|
   | =:IMPRESS_X:= =:IMPRESS_Y:= =:IMPRESS_Z:= | =data-x=, =data-y=, =data-z= - where it is |
   | =:IMPRESS_RELX:= =:IMPRESS_RELY:= =:IMPRESS_RELZ:= | =data-rel-x= and friends - where it is *relative to the last step* |
   | =:IMPRESS_ROTX:= =:IMPRESS_ROTY:= =:IMPRESS_ROTZ:= | how it is turned |
   | =:IMPRESS_ROT:=                | =data-rotate=, multiplied by the step's index |
   | =:IMPRESS_SCALE:=              | =data-scale= - bigger is further away |
   | =:CLASS:=                      | classes on the step                   |
   | =:BACKGROUND:=                 | a colour, image, video or page         |
   | =:CUSTOM_ID:=                  | the step's id, so it can be linked to |
   | =:FRAGMENT:=                   | bullets appear one at a time, as impress substeps |
   | =:NOTES:=                      | speaker notes - press =n=, or use the impress console |

   Both spellings of the position are kept: =:IMPRESS_X: 3000= puts a step at
   3000, and =:IMPRESS_X: .6= with no layout keeps doing what it always did -
   six tenths of a screen along from the one before it.

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
package impressjs

import (
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/op/go-logging.v1"
)

var rver = "2.0.0"
var cdn = "https://cdn.jsdelivr.net/gh/impress/impress.js@" + rver
var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

type ImpressExporter struct {
	Props        map[string]interface{}
	ThemePath    string
	TemplatePath string
	out          *logging.Logger
	pm           *common.PluginManager
}

type ImpressWriter struct {
	*org.HTMLWriter
	exp   *ImpressExporter
	Opts  string
	conf  slides.Conf
	fitOn bool
	props map[string]interface{}
	// req is the *request's* properties - the theme and layout somebody picked
	// in the presentations tab - as opposed to props, which is the template's
	// map.
	req  map[string]string
	pend slides.Pending
	frag slides.Fragment
}

func NewImpressWriter(exp *ImpressExporter, props map[string]interface{}) *ImpressWriter {
	// This lovely circular reference ensures overrides are called when calling
	// write node.
	w := ImpressWriter{HTMLWriter: org.NewHTMLWriter(), exp: exp, props: props}
	w.ExtendingWriter = &w
	w.NoWrapCodeBlock = true
	w.HighlightCodeBlock = slides.Highlighter("IMPRESS", props)
	return &w
}

func (w *ImpressWriter) WriteKeyword(k org.Keyword) {
	if slides.IsAttrKeyword(k.Key, "IMPRESS") {
		w.pend.AddAttr(k.Value)
		return
	}
	w.HTMLWriter.WriteKeyword(k)
}

// impress.js calls one revealed thing a substep, and its substep plugin walks
// them in document order.
func (w *ImpressWriter) WriteList(l org.List) {
	p := w.pend.Take()
	if p.Frag || w.frag.On {
		w.WriteString(slides.FragList(w.HTMLWriter, l, func(int) string { return "substep" }))
		return
	}
	if p.Any() {
		w.WriteString(p.Wrap(w.WriteNodesAsString(l), "div", "substep"))
		return
	}
	w.HTMLWriter.WriteList(l)
}

func (w *ImpressWriter) WriteParagraph(p org.Paragraph) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(p), "div", "substep"))
		return
	}
	w.HTMLWriter.WriteParagraph(p)
}

func (w *ImpressWriter) WriteTable(t org.Table) {
	if pend := w.pend.Take(); pend.Any() {
		w.WriteString(pend.Wrap(w.WriteNodesAsString(t), "div", "substep"))
		return
	}
	w.HTMLWriter.WriteTable(t)
}

// A headline the writer sees is deeper than the slide level: a heading on a
// step rather than a step of its own.
func (w *ImpressWriter) WriteHeadline(h org.Headline) {
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

func (w *ImpressWriter) WriteRegularLink(l org.RegularLink) {
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

// ── Where a step goes ───────────────────────────────────────────────────────

// place works out one step's position from the layout the deck asked for.
//
// The whole reason this exists: impress.js is the only one of the four where a
// deck is unusable until somebody has decided where twenty things are in space,
// and doing that by hand in property drawers is the reason people try impress
// once and go back to reveal. A layout is one keyword and a deck that flies.
//
// It writes *absolute* positions, because a relative chain cannot make a ring
// or a grid - each step would be placed from the last one's offset rather than
// from the shape. Anything a step says about itself still wins: the layout is a
// starting point, not a cage.
func place(a *slides.Attrs, layout string, i, n int, spread float64) {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	switch layout {
	case "line":
		a.Set("data-x", f(float64(i)*spread))
	case "grid":
		cols := int(math.Ceil(math.Sqrt(float64(n))))
		if cols < 1 {
			cols = 1
		}
		a.Set("data-x", f(float64(i%cols)*spread))
		a.Set("data-y", f(float64(i/cols)*spread*0.75))
	case "spiral":
		// A turn and a bit per step, winding outwards, each step turned to
		// follow the curve so the camera banks through it.
		ang := float64(i) * 0.7
		r := spread*0.5 + float64(i)*spread*0.18
		a.Set("data-x", f(r*math.Cos(ang)))
		a.Set("data-y", f(r*math.Sin(ang)))
		a.Set("data-rotate", f(ang*180/math.Pi))
	case "ring":
		if n < 1 {
			n = 1
		}
		ang := float64(i) / float64(n) * 2 * math.Pi
		r := spread * float64(n) / (2 * math.Pi)
		a.Set("data-x", f(r*math.Sin(ang)))
		a.Set("data-z", f(-r*math.Cos(ang)))
		a.Set("data-rotate-y", f(-ang*180/math.Pi))
	case "zoom":
		// Each step further into the screen: the deck falls away from you.
		a.Set("data-z", f(-float64(i)*spread))
		a.Set("data-scale", f(math.Pow(1.4, float64(i))))
	case "random":
		// Deterministic on purpose. A layout that moved every time the file was
		// exported would make a deck impossible to rehearse, and the builder's
		// rule applies here too: anything random has to be reproducible.
		rx, ry, rz := hashSpread(i, 1), hashSpread(i, 2), hashSpread(i, 3)
		a.Set("data-x", f(rx*spread*float64(n)/3))
		a.Set("data-y", f(ry*spread*float64(n)/4))
		a.Set("data-rotate", f(rz*25))
	}
}

// hashSpread is a reproducible number in -1..1 from a step's index.
func hashSpread(i, salt int) float64 {
	h := uint32(i*2654435761 + salt*40503)
	h ^= h >> 13
	h *= 1274126177
	h ^= h >> 16
	return float64(h%2000)/1000.0 - 1.0
}

// layoutOf is the layout and how far apart it spreads things. A request can
// override the layout - that is the presentations tab trying one on - and the
// file is what it is being tried against.
func layoutOf(c slides.Conf, props map[string]string) (string, float64) {
	layout := strings.ToLower(c.DocStr("LAYOUT", "rel"))
	if v := props["layout"]; v != "" {
		layout = strings.ToLower(v)
	}
	spread := 1600.0
	if v, err := strconv.ParseFloat(c.DocStr("SPREAD", ""), 64); err == nil && v != 0 {
		spread = v
	}
	return layout, spread
}

func (self *ImpressExporter) stepAttrs(w *ImpressWriter, s *slides.Slide, i, n int) *slides.Attrs {
	c := w.conf
	p := s.Props()
	a := &slides.Attrs{}
	a.ID(slides.SlideID(c, p, fmt.Sprintf("step-%d", s.Num)))
	a.Class("step", "org-slide")
	a.Flag("data-fit-box")
	a.Class(slides.Classes(c, p)...)

	layout, spread := layoutOf(c, w.req)
	place(a, layout, i, n, spread)

	// Said outright, which always wins over the layout.
	abs := false
	for _, pair := range [][2]string{{"X", "data-x"}, {"Y", "data-y"}, {"Z", "data-z"}} {
		if v := c.Str(p, pair[0], ""); v != "" {
			// A bare fraction with no units is the old relative spelling: this
			// exporter has always written `:IMPRESS_X: .6` as six tenths of a
			// screen along from the previous step, and a deck that says that
			// has to keep working.
			if fv, err := strconv.ParseFloat(v, 64); err == nil && math.Abs(fv) < 10 {
				a.Set("data-rel-"+strings.ToLower(pair[0]), v+relUnit(pair[0]))
			} else {
				a.Set(pair[1], v)
				abs = true
			}
		}
	}
	for _, pair := range [][2]string{{"RELX", "data-rel-x"}, {"RELY", "data-rel-y"}, {"RELZ", "data-rel-z"}} {
		if v := c.Str(p, pair[0], ""); v != "" {
			a.Set(pair[1], v)
		}
	}
	_ = abs
	// Nothing said and no layout: the diagonal cascade this exporter has always
	// drawn, so a deck with no positions at all still goes somewhere.
	if layout == "rel" || layout == "" {
		if c.Str(p, "X", "") == "" && c.Str(p, "RELX", "") == "" {
			a.Set("data-rel-x", c.Str(p, "DEFAULT_RELX", ".6")+"w")
		}
		if c.Str(p, "Y", "") == "" && c.Str(p, "RELY", "") == "" {
			a.Set("data-rel-y", c.Str(p, "DEFAULT_RELY", ".6")+"h")
		}
	}

	a.Set("data-rotate-x", c.Str(p, "ROTX", ""))
	a.Set("data-rotate-y", c.Str(p, "ROTY", ""))
	a.Set("data-rotate-z", c.Str(p, "ROTZ", ""))
	// The old indexed rotation: a value multiplied by the step's own index, so
	// one line in the file fans the whole deck out.
	if v, err := strconv.ParseFloat(c.Str(p, "ROT", ""), 64); err == nil {
		a.Set("data-rotate", strconv.FormatFloat(v*float64(i), 'f', -1, 64))
	} else if v := c.Str(p, "ROTATE", ""); v != "" {
		a.Set("data-rotate", v)
	}
	a.Set("data-scale", c.Str(p, "SCALE", ""))
	a.Set("data-autoplay", c.Str(p, "AUTOPLAY", ""))
	a.Set("data-transition-duration", c.Str(p, "TRANSITION_DURATION", ""))

	bg := slides.ReadBackground(c, p, w.Opts, self.pm)
	a.Class(slides.ReadInk(c, p, bg).Class())
	switch bg.Kind {
	case slides.BgColor, slides.BgGradient:
		a.Style(bg.CSS())
	case slides.BgImage:
		if bg.Opacity != "" {
			a.Class("impress-bg-dim")
			a.Style(bg.CSSVars("impress-bg"))
		} else {
			a.Style(bg.CSS())
		}
	}
	return a
}

func relUnit(axis string) string {
	switch axis {
	case "X":
		return "w"
	case "Y":
		return "h"
	}
	return ""
}

func (self *ImpressExporter) renderStep(w *ImpressWriter, s *slides.Slide, i, n int) string {
	w.frag = slides.ReadFragment(w.conf, s.Props())
	a := self.stepAttrs(w, s, i, n)

	inner := ""
	bg := slides.ReadBackground(w.conf, s.Props(), w.Opts, self.pm)
	switch bg.Kind {
	case slides.BgVideo:
		loop, muted := "", ""
		if bg.Loop {
			loop = " loop"
		}
		if bg.Muted {
			muted = " muted"
		}
		inner += fmt.Sprintf(`<video class="impress-bg-video" autoplay%s%s playsinline><source src="%s"></video>`+"\n", loop, muted, bg.Value)
	case slides.BgIframe:
		extra := ""
		if !bg.Interactive {
			extra = ` style="pointer-events: none"`
		}
		inner += fmt.Sprintf(`<iframe class="impress-bg-iframe" src="%s"%s></iframe>`+"\n", bg.Value, extra)
	}

	fit := slides.ReadFit(w.conf, s.Props(), w.fitOn)
	inner += fit.Open()
	if s.Title != nil {
		lvl := s.Level + 1
		if lvl > 6 {
			lvl = 6
		}
		inner += fmt.Sprintf("<h%d>%s</h%d>\n", lvl, w.WriteNodesAsString(s.Title...), lvl)
	}
	inner += w.WriteNodesAsString(s.Body...)
	inner += fit.Close()
	// `<div class="notes">` is impress.js' own name for them, so the impress
	// console sees them too.
	if notes := w.WriteNodesAsString(s.Notes...); notes != "" {
		inner += fmt.Sprintf("<div class=\"notes\">\n%s</div>\n", notes)
	}
	return fmt.Sprintf("<div%s>\n%s</div>\n\n", a.String(), inner)
}

func (self *ImpressExporter) titleStep(w *ImpressWriter, d *slides.Deck, n int) string {
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
	a.ID("step-title")
	a.Class("step", "slide", "org-slide", "org-title-slide")
	a.Class(w.conf.DocStr("TITLE_CLASS", ""))
	layout, spread := layoutOf(w.conf, w.req)
	if layout == "rel" || layout == "" {
		a.Set("data-x", "0")
		a.Set("data-y", "0")
	} else {
		place(a, layout, 0, n, spread)
	}
	return fmt.Sprintf("<div%s>\n%s</div>\n\n", a.String(), body)
}

// overviewStep is impress.js' own trick: a step with a large scale that the
// deck opens on, so people see the shape of the talk before it starts moving.
func overviewStep(scale string) string {
	if scale == "" {
		scale = "6"
	}
	return fmt.Sprintf(`<div id="overview" class="step" data-x="0" data-y="0" data-scale="%s"></div>`+"\n\n", scale)
}

func (s *ImpressExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(s)
}

func (self *ImpressExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, res := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(res), 0644)
}

func (e *ImpressExporter) ExpandThemePath(tname string) string {
	name := "impress_theme_" + tname + ".css"
	tempFolderName, _ := filepath.Abs(path.Join(e.ThemePath, name))
	if _, err := os.Stat(tempFolderName); err == nil {
		name = tempFolderName
	}
	return name
}

func (self *ImpressExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.FindByFile(query)
	if f == nil {
		return fmt.Errorf("failed to find file in database: [%s]", query), ""
	}
	m := ValidateMap(copyProps(self.Props))
	c := slides.Conf{Doc: f, Prefix: "IMPRESS"}

	theme := c.DocStr("THEME", "impressdefault")
	if v := props["theme"]; v != "" {
		theme = v
	}
	m["theme"] = theme
	// One of the shared themes first - those work in all four frameworks - and
	// impress's own `impress_theme_<name>.css` after, which is what this
	// exporter has always looked for.
	th := slides.LoadTheme(theme)
	m["local_theme"] = th.Found
	m["mermaid_theme"] = th.Mermaid
	switch {
	case th.Found:
		m["themedata"] = th.CSS
		if th.Hljs != "" && c.DocStr("HIGHLIGHT_STYLE", "") == "" {
			m["hljs_style"] = th.Hljs
		}
	default:
		if data, err := os.ReadFile(self.ExpandThemePath(theme)); err == nil {
			m["themedata"] = string(data)
		} else {
			m["themedata"] = ""
		}
	}

	// The animated backdrop, when the theme asked for one (`@backdrop:` in its
	// own comment header) or the file did. Inlined like the autofit pair and for
	// the same reason: a deck is often shown from a memory stick.
	bd := slides.ReadBackdrop(c, th)
	m["backdrop"] = bd.Scene
	m["backdrop_config"] = bd.Config
	m["backdrop_css"] = bd.CSS
	m["backdrop_js"] = bd.JS

	w := NewImpressWriter(self, m)
	w.req = props
	w.fitOn = c.DocBool("FIT", true)
	w.Opts = opts
	w.conf = c
	w.Document = f

	deck := slides.BuildDeck(c, f)
	// impress.js steps cannot nest - only the direct children of #impress are
	// steps - so the tree is flattened. The old exporter wrote a step inside a
	// step for any nested headline, and impress.js ignored it: the content was
	// on the page and could not be reached.
	flat := deck.Flat()
	n := len(flat)
	if deck.TitleSlide {
		n++
	}
	out := strings.Builder{}
	if c.DocBool("OVERVIEW", false) {
		out.WriteString(overviewStep(c.DocStr("OVERVIEW_SCALE", "")))
	}
	i := 0
	if deck.TitleSlide {
		out.WriteString(self.titleStep(w, deck, n))
		i++
	}
	for _, s := range flat {
		out.WriteString(self.renderStep(w, s, i, n))
		i++
	}

	m["slide_data"] = out.String()
	m["title"] = firstNonEmpty(deck.Title, m["title"])
	m["hljs_style"] = c.DocStr("HIGHLIGHT_STYLE", asString(m["hljs_style"]))
	m["fontfamily"] = c.DocStr("FONT", asString(m["fontfamily"]))
	m["extra_css"] = c.DocStr("CSS", "")
	m["head"] = c.DocStr("HEAD", "")
	m["width"] = c.DocInt("WIDTH", 1024)
	m["height"] = c.DocInt("HEIGHT", 768)
	m["max_scale"] = c.DocStr("MAX_SCALE", "3")
	m["min_scale"] = c.DocStr("MIN_SCALE", "0")
	m["perspective"] = c.DocStr("PERSPECTIVE", "1000")
	m["transition_duration"] = c.DocStr("TRANSITION_DURATION", "1000")
	m["autoplay"] = c.DocStr("AUTOPLAY", "")
	m["toolbar"] = c.DocBool("TOOLBAR", true)
	m["progress"] = c.DocBool("PROGRESS", true)
	m["hint"] = c.DocBool("HINT", true)
	m["notes"] = c.DocBool("NOTES", true)
	m["slide_count"] = deck.Count()
	// Too much on a slide is shrunk to fit unless the deck says otherwise. The
	// stylesheet and the script are inlined rather than linked: a deck is often
	// saved and shown from disk, and a file that stops working the moment it
	// leaves the server is not an export.
	m["autofit"] = c.DocBool("FIT", true)
	if m["autofit"] == true {
		m["autofit_css"], m["autofit_js"] = slides.AutoFitAssets()
	}
	return nil, self.pm.Tempo.RenderTemplate(self.TemplatePath, m)
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

func (self *ImpressExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func NewHtmlExp() *ImpressExporter {
	var g *ImpressExporter = new(ImpressExporter)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	def := func(k string, v interface{}) {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	def("title", "Presentation")
	def("impress_cdn", cdn)
	def("hljs_cdn", hljscdn)
	def("hljs_style", "monokai")
	def("fontfamily", "Inconsolata")
	def("theme", "impressdefault")
	def("themedata", "")
	def("wordcloud", false)
	def("mermaid", false)
	if _, ok := m["stylesheet"]; !ok {
		if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("impress_style.css")); err == nil {
			m["stylesheet"] = string(data)
		} else {
			m["stylesheet"] = ""
		}
	}
	return m
}

// init function is called at boot
func init() {
	common.AddExporter("impressjs", func() common.Exporter {
		return &ImpressExporter{Props: ValidateMap(map[string]interface{}{}), ThemePath: "./templates", TemplatePath: "impress_default.tpl"}
	})
}

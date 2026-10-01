package slides

// What a slide looks like: backgrounds, alignment, fragments, code.
//
// One org vocabulary, four renderings. `:BACKGROUND: blue` is a blue slide in
// reveal (`data-background-color`), in impress (a css background on the step),
// in WebSlides (`.bg-blue` where it has such a class, an inline colour where it
// does not) and in deck.js (an inline colour), because somebody writing a deck
// is describing their talk and should not have to know which of four libraries
// is going to draw it.
//
// The one thing that *is* worked out rather than declared is which kind of
// background a value is. `:BACKGROUND: #223` is a colour, `:BACKGROUND:
// images/cover.jpg` is an image, `:BACKGROUND: clip.mp4` is a video and
// `:BACKGROUND: https://example.com` is a page. Guessing is what makes the
// short form worth having, and every guess can be overridden by saying which
// (`:BACKGROUND_IMAGE:`, `:BACKGROUND_VIDEO:`, `:BACKGROUND_COLOR:`,
// `:BACKGROUND_IFRAME:`) - because the guess is wrong for exactly the file
// somebody is about to show.

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// BgKind is what a background turned out to be.
type BgKind int

const (
	BgNone BgKind = iota
	BgColor
	BgImage
	BgVideo
	BgIframe
	BgGradient
)

// Background is a slide's background, read off its properties.
type Background struct {
	Kind BgKind
	// Value is the colour, the resolved url, or the gradient.
	Value string
	// Size, Position and Repeat are css, for an image.
	Size     string
	Position string
	Repeat   string
	Opacity  string
	// Transition is the framework's name for how to move between backgrounds.
	Transition string
	// Loop and Muted are for a video.
	Loop  bool
	Muted bool
	// Interactive says an iframe background should take clicks.
	Interactive bool
}

var (
	colorRe    = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|rgba?\(|hsla?\()`)
	imageExtRe = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|svg|webp|avif|bmp|tiff?)(\?.*)?$`)
	videoExtRe = regexp.MustCompile(`(?i)\.(mp4|webm|ogv|m4v|mov)(\?.*)?$`)
	// The css named colours worth recognising without a hash in front of them.
	// Not all 140 of them: this is the list somebody types at a slide, and a
	// name that is not in it still works written as a hex or through
	// :BACKGROUND_COLOR:.
	namedColors = map[string]bool{
		"black": true, "white": true, "red": true, "green": true, "blue": true,
		"yellow": true, "orange": true, "purple": true, "pink": true, "brown": true,
		"grey": true, "gray": true, "cyan": true, "magenta": true, "teal": true,
		"navy": true, "olive": true, "maroon": true, "silver": true, "gold": true,
		"transparent": true, "beige": true, "ivory": true, "coral": true,
		"crimson": true, "indigo": true, "lavender": true, "salmon": true,
		"tan": true, "turquoise": true, "violet": true, "wheat": true,
	}
)

// ReadBackground works out a slide's background from its properties.
func ReadBackground(c Conf, p PropGet, opts string, pm *common.PluginManager) Background {
	b := Background{
		Size:       c.Str(p, "BACKGROUND_SIZE", ""),
		Position:   c.Str(p, "BACKGROUND_POSITION", ""),
		Repeat:     c.Str(p, "BACKGROUND_REPEAT", ""),
		Opacity:    c.Str(p, "BACKGROUND_OPACITY", ""),
		Transition: c.Str(p, "BACKGROUND_TRANSITION", ""),
		Loop:       c.Bool(p, "BACKGROUND_LOOP", true),
		Muted:      c.Bool(p, "BACKGROUND_MUTED", true),
	}
	// Said outright, in order of how specific it is.
	if v := c.Str(p, "BACKGROUND_COLOR", ""); v != "" {
		b.Kind, b.Value = BgColor, v
		return b
	}
	if v := c.Str(p, "BACKGROUND_GRADIENT", ""); v != "" {
		b.Kind, b.Value = BgGradient, v
		return b
	}
	if v := c.Str(p, "BACKGROUND_IMAGE", ""); v != "" {
		b.Kind, b.Value = BgImage, resolveMedia(c, v, opts, pm)
		return b
	}
	if v := c.Str(p, "BACKGROUND_VIDEO", ""); v != "" {
		b.Kind, b.Value = BgVideo, resolveMedia(c, v, opts, pm)
		return b
	}
	if v := c.Str(p, "BACKGROUND_IFRAME", ""); v != "" {
		b.Kind, b.Value = BgIframe, v
		b.Interactive = c.Bool(p, "BACKGROUND_INTERACTIVE", false)
		return b
	}
	// Or guessed from the short form.
	v := c.Str(p, "BACKGROUND", "")
	if v == "" {
		return b
	}
	switch {
	case strings.Contains(v, "gradient("):
		b.Kind, b.Value = BgGradient, v
	case colorRe.MatchString(v) || namedColors[strings.ToLower(v)]:
		b.Kind, b.Value = BgColor, v
	case videoExtRe.MatchString(v):
		b.Kind, b.Value = BgVideo, resolveMedia(c, v, opts, pm)
	case imageExtRe.MatchString(v):
		b.Kind, b.Value = BgImage, resolveMedia(c, v, opts, pm)
	case IsExternal(v):
		// A url with no file extension worth recognising is a page, which is
		// what iframe backgrounds are for.
		b.Kind, b.Value = BgIframe, v
		b.Interactive = c.Bool(p, "BACKGROUND_INTERACTIVE", false)
	default:
		// Anything else is taken as an image: a file with no extension is far
		// more likely to be a picture somebody is pointing at than a colour
		// nobody can name.
		b.Kind, b.Value = BgImage, resolveMedia(c, v, opts, pm)
	}
	return b
}

func resolveMedia(c Conf, v, opts string, pm *common.PluginManager) string {
	if IsExternal(v) {
		return v
	}
	return Media(c.Doc, pm, opts, v)
}

// CSS is the background as a style declaration, for the frameworks that have no
// opinion of their own about backgrounds.
func (b Background) CSS() string {
	switch b.Kind {
	case BgColor:
		return "background-color: " + b.Value
	case BgGradient:
		return "background-image: " + b.Value
	case BgImage:
		out := fmt.Sprintf("background-image: url('%s')", b.Value)
		out += "; background-size: " + orDefault(b.Size, "cover")
		out += "; background-position: " + orDefault(b.Position, "center")
		out += "; background-repeat: " + orDefault(b.Repeat, "no-repeat")
		return out
	}
	return ""
}

// CSSVars is the background as custom properties, for a framework that has to
// draw it on a layer of its own - which is what dimming an image means: the
// picture is faded, not the words in front of it.
func (b Background) CSSVars(prefix string) string {
	if b.Kind != BgImage {
		return ""
	}
	out := []string{
		fmt.Sprintf("--%s-image: url('%s')", prefix, b.Value),
		fmt.Sprintf("--%s-size: %s", prefix, orDefault(b.Size, "cover")),
		fmt.Sprintf("--%s-position: %s", prefix, orDefault(b.Position, "center")),
		fmt.Sprintf("--%s-repeat: %s", prefix, orDefault(b.Repeat, "no-repeat")),
	}
	if b.Opacity != "" {
		out = append(out, fmt.Sprintf("--%s-opacity: %s", prefix, b.Opacity))
	}
	return strings.Join(out, "; ")
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// ── Alignment, classes and layout, said once ────────────────────────────────

// Align is `:ALIGN: center|left|right` turned into whatever this framework
// calls it. The names are given by the caller because only it knows them.
func Align(c Conf, p PropGet) string {
	return strings.ToLower(strings.TrimSpace(c.Str(p, "ALIGN", "")))
}

// Classes are the extra classes a slide asked for: `:CLASS:` for every
// framework plus the framework's own, which is how a WebSlides deck reaches the
// forty classes that library has without this code having to name them.
func Classes(c Conf, p PropGet) []string {
	return strings.Fields(c.Str(p, "CLASS", ""))
}

// ── Fragments ───────────────────────────────────────────────────────────────

// Fragment is how a slide reveals itself a piece at a time.
//
// Org has no way to mark one list item - `#+ATTR_REVEAL:` is an affiliated
// keyword and go-org does not hand affiliated keywords to a list - so the unit
// here is the *slide*: `:FRAGMENT:` on a headline means its top-level list
// items appear one at a time, which is what people reach for fragments to do.
// The value is passed through to the framework, so reveal gets its whole
// vocabulary (`fade-in`, `highlight-red`, `grow`) for free.
type Fragment struct {
	On    bool
	Style string
}

func ReadFragment(c Conf, p PropGet) Fragment {
	for _, name := range []string{"FRAGMENT", "FRAG", "FRAGMENTS", "INCREMENTAL"} {
		if v := c.Str(p, name, "\x00"); v != "\x00" {
			if !Truthy(v) {
				return Fragment{}
			}
			style := strings.TrimSpace(v)
			if style == "" || strings.EqualFold(style, "t") || strings.EqualFold(style, "yes") {
				style = ""
			}
			return Fragment{On: true, Style: style}
		}
	}
	return Fragment{}
}

// ── Code blocks ─────────────────────────────────────────────────────────────

// Highlighter is the code block renderer these exporters share: a `<pre><code>`
// with the language on it for highlight.js, a `<pre class="mermaid">` for a
// mermaid diagram, and the word cloud hook the reveal exporter grew.
//
// `prefix` is the framework's keyword prefix, so `#+REVEAL_LINES: 2-4` and
// `#+DECK_LINES: 2-4` both say which lines to point at. `props` is the
// exporter's template map, which is how a block in the middle of a file turns
// a script on at the top of the page.
func Highlighter(prefix string, props map[string]interface{}) func([]org.Keyword, string, string, bool, map[string]string) string {
	clouds := 0
	return func(keywords []org.Keyword, source, lang string, inline bool, params map[string]string) string {
		a := &Attrs{}
		for _, key := range keywords {
			switch strings.ToUpper(key.Key) {
			case prefix + "_LINES", "SLIDE_LINES", "ATTR_LINES":
				a.Set("data-line-numbers", key.Value)
			case prefix + "_CODE_CLASS", "SLIDE_CODE_CLASS":
				a.Class(key.Value)
			}
		}
		if lang != "" {
			a.Class("language-" + lang)
		}
		switch lang {
		case "mermaid":
			if props != nil {
				props["mermaid"] = true
			}
			return fmt.Sprintf(`<pre class="mermaid">%s</pre>`, html.EscapeString(source))
		case "wordcloud":
			if props != nil {
				props["wordcloud"] = true
			}
			clouds++
			return fmt.Sprintf(`<svg class="org-wordcloud" id="wordcloud_%d" onload="wordcloud('#wordcloud_%d', %s)"/>`,
				clouds, clouds, strings.TrimSpace(source))
		}
		return fmt.Sprintf("<pre><code%s>%s</code></pre>", a.String(), html.EscapeString(source))
	}
}

// ── Keeping the words readable on whatever the slide's ground turned out to be ──

// Ink is which way round a slide's text has to go for anybody to read it.
type Ink int

const (
	InkUnknown Ink = iota
	InkOnLight     // dark text, the usual way round
	InkOnDark      // light text
)

// Class is what the mapping stylesheet calls it.
func (i Ink) Class() string {
	switch i {
	case InkOnDark:
		return "org-on-dark"
	case InkOnLight:
		return "org-on-light"
	}
	return ""
}

// ReadInk decides whether this slide needs light text or dark.
//
// This exists because the two settings most likely to be used together are the
// ones most likely to contradict each other: a theme picks an ink to suit its
// own ground, and `:BACKGROUND: #102030` then puts a dark blue behind it. In
// the `mono` theme - black on white - that slide came out black on navy, which
// is a slide nobody can read and which looks like the theme is broken rather
// than like the deck asked for something odd.
//
// So the ground is measured and the ink is flipped to suit it. Three rules:
//
//  1. **Said outright wins.** `:BACKGROUND_INK: light` forces light text, which
//     is the escape hatch for the cases below.
//  2. **A picture or a video is unknowable**, so nothing is done - guessing
//     from a filename would be worse than leaving it to the author, and a
//     background image usually comes with `:CLASS:` or an opacity anyway.
//  3. **Luminance, not brightness.** The eye is far more sensitive to green
//     than to blue, so a plain average calls #0000ff light and #00ff00 dark,
//     which is backwards for exactly the saturated colours people reach for.
func ReadInk(c Conf, p PropGet, bg Background) Ink {
	switch strings.ToLower(c.Str(p, "BACKGROUND_INK", c.Str(p, "INK", ""))) {
	case "light", "white", "ondark", "on-dark", "dark-background":
		return InkOnDark
	case "dark", "black", "onlight", "on-light", "light-background":
		return InkOnLight
	}
	switch bg.Kind {
	case BgColor, BgGradient:
		if l, ok := Luminance(bg.Value); ok {
			if l < 0.42 {
				return InkOnDark
			}
			return InkOnLight
		}
	}
	return InkUnknown
}

var (
	hexRe = regexp.MustCompile(`#([0-9a-fA-F]{3,8})\b`)
	rgbRe = regexp.MustCompile(`rgba?\(\s*([0-9.]+)[,\s]+([0-9.]+)[,\s]+([0-9.]+)`)
)

// The css colours worth knowing the value of: the ones somebody types at a
// slide. Anything else has to be written as a hex, which is also the only way
// to be sure what you are going to get.
var namedValues = map[string][3]float64{
	"black": {0, 0, 0}, "white": {255, 255, 255}, "red": {255, 0, 0},
	"green": {0, 128, 0}, "blue": {0, 0, 255}, "yellow": {255, 255, 0},
	"orange": {255, 165, 0}, "purple": {128, 0, 128}, "pink": {255, 192, 203},
	"brown": {165, 42, 42}, "grey": {128, 128, 128}, "gray": {128, 128, 128},
	"cyan": {0, 255, 255}, "magenta": {255, 0, 255}, "teal": {0, 128, 128},
	"navy": {0, 0, 128}, "olive": {128, 128, 0}, "maroon": {128, 0, 0},
	"silver": {192, 192, 192}, "gold": {255, 215, 0}, "beige": {245, 245, 220},
	"ivory": {255, 255, 240}, "coral": {255, 127, 80}, "crimson": {220, 20, 60},
	"indigo": {75, 0, 130}, "lavender": {230, 230, 250}, "salmon": {250, 128, 114},
	"tan": {210, 180, 140}, "turquoise": {64, 224, 208}, "violet": {238, 130, 238},
	"wheat": {245, 222, 179},
}

// Luminance is the relative luminance of a css colour, 0 (black) to 1 (white),
// and whether it could be read at all. A gradient is measured by averaging
// every colour in it, which is rough and is the right kind of rough: what
// matters is only which side of the middle it falls on.
func Luminance(css string) (float64, bool) {
	css = strings.TrimSpace(css)
	if v, ok := namedValues[strings.ToLower(css)]; ok {
		return relLum(v[0], v[1], v[2]), true
	}
	sum, n := 0.0, 0
	for _, m := range hexRe.FindAllStringSubmatch(css, -1) {
		if r, g, b, ok := fromHex(m[1]); ok {
			sum += relLum(r, g, b)
			n++
		}
	}
	for _, m := range rgbRe.FindAllStringSubmatch(css, -1) {
		r, _ := strconv.ParseFloat(m[1], 64)
		g, _ := strconv.ParseFloat(m[2], 64)
		b, _ := strconv.ParseFloat(m[3], 64)
		sum += relLum(r, g, b)
		n++
	}
	// A gradient names directions and stops as well as colours; a word in it
	// that happens to be a colour name counts too.
	if n == 0 {
		for _, word := range strings.FieldsFunc(strings.ToLower(css), func(r rune) bool {
			return r == ' ' || r == ',' || r == '(' || r == ')' || r == ';'
		}) {
			if v, ok := namedValues[word]; ok {
				sum += relLum(v[0], v[1], v[2])
				n++
			}
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

func fromHex(h string) (float64, float64, float64, bool) {
	switch len(h) {
	case 3, 4:
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	case 6, 8:
		h = h[:6]
	default:
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64((v >> 16) & 0xff), float64((v >> 8) & 0xff), float64(v & 0xff), true
}

// relLum is the sRGB relative luminance, the one the contrast guidelines use.
func relLum(r, g, b float64) float64 {
	f := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*f(r) + 0.7152*f(g) + 0.0722*f(b)
}

// ── Fitting too much onto a slide ───────────────────────────────────────────

// Fit is whether this slide's content should be shrunk to fit the slide, and
// how far it may be shrunk.
//
// On by default, because the failure it answers is silent: every one of these
// four frameworks simply does not draw the overflow, so a slide with one bullet
// too many looks finished until it is on a projector. The arithmetic is in
// slides_autofit.js - this is only the deciding.
type Fit struct {
	On bool
	// Grow lets a thin slide scale *up*, which is off by default: a deck whose
	// type size changes slide by slide looks worse than one with white space
	// in it.
	Grow bool
	// Min is the floor, as a fraction. Past it the slide is not overfull, it is
	// a document, and shrinking it further would hide that rather than show it.
	Min float64
}

// ReadFit reads `:FIT:` on the slide and `#+SLIDE_FIT:` on the document.
//
//	:FIT: nil      leave this slide alone
//	:FIT: grow     this one may scale up to fill the slide as well
//	:FIT: 0.4      shrink as far as four tenths before giving up
func ReadFit(c Conf, p PropGet, def bool) Fit {
	f := Fit{On: def, Min: 0.45}
	v := c.Str(p, "FIT", "\x00")
	if v == "\x00" {
		return f
	}
	if !Truthy(v) {
		return Fit{}
	}
	f.On = true
	for _, word := range strings.Fields(strings.ToLower(v)) {
		switch word {
		case "grow", "up", "fill":
			f.Grow = true
		case "t", "yes", "on":
		default:
			if n, err := strconv.ParseFloat(word, 64); err == nil {
				if n > 1 {
					n = n / 100
				}
				if n > 0 && n <= 1 {
					f.Min = n
				}
			}
		}
	}
	return f
}

// Open is the wrapper a fitted slide's content goes inside, and Close ends it.
// They answer with nothing at all when fitting is off, so a caller writes them
// unconditionally and an unfitted deck carries no extra markup.
func (f Fit) Open() string {
	if !f.On {
		return ""
	}
	a := &Attrs{}
	a.Class("org-fit")
	if f.Grow {
		a.Set("data-fit-grow", "yes")
	}
	if f.Min > 0 && f.Min != 0.45 {
		a.Set("data-fit-min", strconv.FormatFloat(f.Min, 'f', -1, 64))
	}
	return "<div" + a.String() + ">\n"
}

func (f Fit) Close() string {
	if !f.On {
		return ""
	}
	return "</div>\n"
}

package slides

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func parse(t *testing.T, src string) *org.Document {
	t.Helper()
	d := org.New().Parse(strings.NewReader(src), "test.org")
	if d.Error != nil {
		t.Fatalf("parse: %v", d.Error)
	}
	return d
}

func conf(t *testing.T, src string) (Conf, *org.Document) {
	t.Helper()
	d := parse(t, src)
	return Conf{Doc: d, Prefix: "REVEAL"}, d
}

// The whole reason this package exists.
//
// go-org ends a headline's body at a drawer written in column zero and hoists
// the rest of the heading to the top level of the document, so for a file with
// `:PROPERTIES:` under its headings - which Emacs writes by default, and which
// is every file that configures a slide - `Headline.Children` is empty and
// `Headline.Properties` is nil. A builder that walked the tree would come back
// with a deck of empty slides, which is exactly what the reveal.js and
// impress.js exporters used to do.
const hoisted = `#+TITLE: A Talk
#+AUTHOR: Someone

An abstract above the first heading.

* First
:PROPERTIES:
:BACKGROUND: blue
:CUSTOM_ID: first
:END:

The body of the first slide.

- a bullet

* Second
:PROPERTIES:
:TRANSITION: zoom
:END:

The body of the second slide.
`

func TestBuildReadsAHoistedDocument(t *testing.T) {
	c, d := conf(t, hoisted)
	deck := BuildDeck(c, d)

	if deck.Title != "A Talk" || deck.Author != "Someone" {
		t.Errorf("title/author: %q / %q", deck.Title, deck.Author)
	}
	if len(deck.Preamble) == 0 {
		t.Errorf("the abstract above the first heading was lost")
	}
	if len(deck.Slides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(deck.Slides))
	}

	first := deck.Slides[0]
	if got := org.String(first.Title...); got != "First" {
		t.Errorf("first slide is %q", got)
	}
	// The body: hoisted to the top level by the parser, attributed back here.
	if len(first.Body) == 0 {
		t.Errorf("the first slide has no body")
	}
	// The properties: hoisted out of the headline, read back off the slide.
	if v, ok := first.Props()("BACKGROUND"); !ok || v != "blue" {
		t.Errorf("BACKGROUND came back %q (%v) - a hoisted drawer was not picked up", v, ok)
	}
	if got := SlideID(c, first.Props(), "fallback"); got != "first" {
		t.Errorf("the slide's own id is %q", got)
	}
	if v := c.Str(deck.Slides[1].Props(), "TRANSITION", ""); v != "zoom" {
		t.Errorf("second slide's transition is %q", v)
	}
	// And the second slide's body did not end up on the first.
	if len(deck.Slides[1].Body) == 0 {
		t.Errorf("the second slide has no body")
	}
}

// Nesting still works - plenty of headlines parse as a tree - and the two
// shapes have to come out the same.
func TestBuildReadsANestedDocument(t *testing.T) {
	c, d := conf(t, `* First
  Body one.
** Under first
   Body two.
* Second
  Body three.
`)
	deck := BuildDeck(c, d)
	if len(deck.Slides) != 2 {
		t.Fatalf("expected 2 top slides, got %d", len(deck.Slides))
	}
	if len(deck.Slides[0].Subs) != 1 {
		t.Fatalf("expected one sub-slide, got %d", len(deck.Slides[0].Subs))
	}
	if got := org.String(deck.Slides[0].Subs[0].Title...); got != "Under first" {
		t.Errorf("sub-slide is %q", got)
	}
	// Flat is document order: parent, child, next parent.
	flat := deck.Flat()
	got := []string{}
	for _, s := range flat {
		got = append(got, org.String(s.Title...))
	}
	want := "First|Under first|Second"
	if strings.Join(got, "|") != want {
		t.Errorf("flat order is %v, want %s", got, want)
	}
}

// The slide level: deeper than it, a headline is a heading *on* the slide
// rather than a slide of its own - and anything the parser hoisted after it
// stays on that slide too.
func TestSlideLevel(t *testing.T) {
	c, d := conf(t, `#+SLIDE_LEVEL: 1

* One
:PROPERTIES:
:CLASS: big
:END:

Body of one.

** A heading on the slide
:PROPERTIES:
:CLASS: small
:END:

More body, hoisted after a drawer.

* Two

Body of two.
`)
	deck := BuildDeck(c, d)
	if len(deck.Slides) != 2 {
		t.Fatalf("expected 2 slides, got %d", len(deck.Slides))
	}
	one := deck.Slides[0]
	if len(one.Subs) != 0 {
		t.Errorf("a headline below the slide level should not be a slide: %d subs", len(one.Subs))
	}
	// The heading, and the prose hoisted after it, are both on slide one.
	body := ""
	for _, n := range one.Body {
		if h := asHeadline(n); h != nil {
			body += "[heading:" + org.String(h.Title...) + "]"
			continue
		}
		body += "[node]"
	}
	if !strings.Contains(body, "[heading:A heading on the slide]") {
		t.Errorf("the deeper heading is not on the slide: %s", body)
	}
	if strings.Count(body, "[node]") < 2 {
		t.Errorf("the body hoisted after the deeper heading went missing: %s", body)
	}
}

// Speaker notes, written the three ways people write them. Getting this wrong
// puts the speaker's crib sheet on the screen behind them, which is the worst
// failure this code has available to it.
func TestNotes(t *testing.T) {
	c, d := conf(t, `* Drawer
:PROPERTIES:
:CLASS: x
:END:

On screen.

:NOTES:
Not on screen.
:END:

* Block

On screen.

#+BEGIN_NOTES
Also not on screen.
#+END_NOTES

* Heading

On screen.

** Notes

Also not on screen.

* After
Body.
`)
	deck := BuildDeck(c, d)
	if len(deck.Slides) != 4 {
		t.Fatalf("expected 4 slides, got %d", len(deck.Slides))
	}
	for i, s := range deck.Slides[:3] {
		if len(s.Notes) == 0 {
			t.Errorf("slide %d has no notes", i)
		}
		if len(s.Body) == 0 {
			t.Errorf("slide %d has no body", i)
		}
	}
	// A notes *heading* is not a slide of its own, and the heading after it is.
	names := []string{}
	for _, s := range deck.Flat() {
		names = append(names, org.String(s.Title...))
	}
	if strings.Join(names, "|") != "Drawer|Block|Heading|After" {
		t.Errorf("notes became a slide: %v", names)
	}
}

// An excluded heading takes its body with it, including the part the parser
// hoisted out of it - the test that matters, because the body is no longer
// inside the headline that said `:noexport:`.
func TestNoExportTakesItsBody(t *testing.T) {
	c, d := conf(t, `* Kept
Body kept.
* Secret                                                          :noexport:
:PROPERTIES:
:CLASS: x
:END:

Body that must not be exported.

** Also secret

More.

* Kept again
Body kept again.
`)
	deck := BuildDeck(c, d)
	names := []string{}
	for _, s := range deck.Flat() {
		names = append(names, org.String(s.Title...))
	}
	if strings.Join(names, "|") != "Kept|Kept again" {
		t.Errorf("excluded slides came through: %v", names)
	}
	for _, s := range deck.Flat() {
		for _, n := range s.Body {
			if strings.Contains(org.String(n), "must not be exported") {
				t.Errorf("an excluded heading's hoisted body landed on %q", org.String(s.Title...))
			}
		}
	}
}

func TestTitleSlide(t *testing.T) {
	c, d := conf(t, hoisted)
	if deck := BuildDeck(c, d); !deck.TitleSlide {
		t.Errorf("a file with a title and an abstract should get a title slide")
	}
	// Asked not to.
	c2, d2 := conf(t, "#+TITLE: A Talk\n#+SLIDE_TITLE_SLIDE: nil\n\n* One\nBody.\n")
	if deck := BuildDeck(c2, d2); deck.TitleSlide {
		t.Errorf("#+SLIDE_TITLE_SLIDE: nil should drop it")
	}
	// Nothing to put on one.
	c3, d3 := conf(t, "* One\nBody.\n")
	deck := BuildDeck(c3, d3)
	if deck.TitleSlide {
		t.Errorf("a file with no title and no preamble has no title slide to draw")
	}
	if deck.Count() != 1 {
		t.Errorf("count is %d, want 1", deck.Count())
	}
}

// The settings ladder: the slide's own prefixed property, then its neutral one,
// then the document's. The neutral spelling is what lets one file export as all
// four frameworks.
func TestConfLadder(t *testing.T) {
	c, d := conf(t, `#+SLIDE_BACKGROUND: grey
#+REVEAL_TRANSITION: fade

* Own prefixed
:PROPERTIES:
:REVEAL_BACKGROUND: red
:BACKGROUND: green
:END:
Body.

* Own neutral
:PROPERTIES:
:BACKGROUND: green
:END:
Body.

* From the document
Body.
`)
	deck := BuildDeck(c, d)
	want := []string{"red", "green", "grey"}
	for i, s := range deck.Slides {
		if got := c.Str(s.Props(), "BACKGROUND", ""); got != want[i] {
			t.Errorf("slide %d background is %q, want %q", i, got, want[i])
		}
	}
	// A document setting reaches a slide that says nothing.
	if got := c.Str(deck.Slides[2].Props(), "TRANSITION", ""); got != "fade" {
		t.Errorf("document transition did not reach the slide: %q", got)
	}
	// And the other framework's prefix is not read.
	other := Conf{Doc: d, Prefix: "DECK"}
	if got := other.Str(deck.Slides[0].Props(), "BACKGROUND", ""); got != "green" {
		t.Errorf("deck.js should not read :REVEAL_BACKGROUND:, got %q", got)
	}
}

func TestTruthy(t *testing.T) {
	for _, yes := range []string{"", "t", "yes", "on", "1", "fade-in"} {
		if !Truthy(yes) {
			t.Errorf("%q should be true - a property with nothing after it is somebody switching it on", yes)
		}
	}
	for _, no := range []string{"nil", "false", "off", "no", "f", "0", "none"} {
		if Truthy(no) {
			t.Errorf("%q should be false", no)
		}
	}
}

// What kind of background a value is, worked out from the value - which is what
// makes the short form worth having.
func TestBackgroundKinds(t *testing.T) {
	cases := []struct {
		prop string
		kind BgKind
	}{
		{":BACKGROUND: #223344", BgColor},
		{":BACKGROUND: blue", BgColor},
		{":BACKGROUND: rgba(0,0,0,0.5)", BgColor},
		{":BACKGROUND: linear-gradient(red, blue)", BgGradient},
		{":BACKGROUND: images/cover.png", BgImage},
		{":BACKGROUND: clip.mp4", BgVideo},
		{":BACKGROUND: https://example.com", BgIframe},
		{":BACKGROUND_COLOR: notacolour", BgColor},
		{":BACKGROUND_VIDEO: thing.bin", BgVideo},
		{":BACKGROUND_IMAGE: thing.bin", BgImage},
	}
	for _, tc := range cases {
		c, d := conf(t, "* One\n:PROPERTIES:\n"+tc.prop+"\n:END:\nBody.\n")
		deck := BuildDeck(c, d)
		bg := ReadBackground(c, deck.Slides[0].Props(), "", nil)
		if bg.Kind != tc.kind {
			t.Errorf("%s came out kind %d, want %d", tc.prop, bg.Kind, tc.kind)
		}
	}
	// Nothing said is nothing drawn.
	c, d := conf(t, "* One\nBody.\n")
	deck := BuildDeck(c, d)
	if bg := ReadBackground(c, deck.Slides[0].Props(), "", nil); bg.Kind != BgNone {
		t.Errorf("a slide with no background has one: %+v", bg)
	}
}

func TestAttrs(t *testing.T) {
	a := &Attrs{}
	a.ID("one")
	a.Class("step", "step") // the same class twice is one class
	a.Class("big small")
	a.Set("data-x", "100")
	a.Set("data-y", "")   // no value, no attribute
	a.Set("", "whatever") // no name either
	a.Flag("data-auto-animate")
	a.Style("color: red;")
	a.Style("  ")
	got := a.String()
	want := ` id="one" class="step big small" data-x="100" data-auto-animate style="color: red"`
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if (&Attrs{}).String() != "" {
		t.Errorf("nothing to say should be the empty string, not a space")
	}
	// A value with a quote in it cannot be allowed to end the attribute.
	b := &Attrs{}
	b.Set("data-x", `a" onload="alert(1)`)
	if strings.Contains(b.String(), `" onload="`) {
		t.Errorf("an attribute value escaped its quotes: %s", b.String())
	}
}

func TestPendingAttrs(t *testing.T) {
	p := &Pending{}
	p.AddAttr(":frag fade-in :class big wide :style color: red")
	if !p.Frag || p.Style != "fade-in" {
		t.Errorf("frag came out %v/%q", p.Frag, p.Style)
	}
	if strings.Join(p.Classes, ",") != "big,wide" {
		t.Errorf("classes came out %v", p.Classes)
	}
	if len(p.Styles) != 1 || p.Styles[0] != "color: red" {
		t.Errorf("styles came out %v", p.Styles)
	}
	// Anything this code has never heard of is passed through as a data
	// attribute, so a framework feature nobody here knows about is reachable.
	q := &Pending{}
	q.AddAttr(":autoplay 5 :noheading")
	joined := strings.Join(q.Attrs, " ")
	if !strings.Contains(joined, `data-autoplay="5"`) || !strings.Contains(joined, "data-noheading") {
		t.Errorf("passthrough attributes came out %q", joined)
	}
	// Spent once, and only once: attributes apply to the next element, and
	// leaving them down would decorate the rest of the slide.
	r := &Pending{}
	r.AddAttr(":frag")
	if !r.Take().Frag {
		t.Errorf("the first take should have the fragment")
	}
	if r.Take().Any() {
		t.Errorf("the second take should have nothing")
	}
	// A keyword of ours, and one that is not.
	if !IsAttrKeyword("ATTR_SLIDE", "REVEAL") || !IsAttrKeyword("ATTR_REVEAL", "REVEAL") {
		t.Errorf("ATTR_SLIDE and ATTR_REVEAL are both ours")
	}
	if IsAttrKeyword("ATTR_HTML", "REVEAL") || IsAttrKeyword("ATTR_IMPRESS", "REVEAL") {
		t.Errorf("ATTR_HTML goes through go-org, and another framework's prefix is not ours")
	}
}

func TestFragment(t *testing.T) {
	c, d := conf(t, `* Styled
:PROPERTIES:
:FRAGMENT: fade-up
:END:
Body.
* Bare
:PROPERTIES:
:FRAGMENT: t
:END:
Body.
* Off
:PROPERTIES:
:FRAGMENT: nil
:END:
Body.
* Nothing
Body.
`)
	deck := BuildDeck(c, d)
	want := []Fragment{{true, "fade-up"}, {true, ""}, {false, ""}, {false, ""}}
	for i, s := range deck.Slides {
		if got := ReadFragment(c, s.Props()); got != want[i] {
			t.Errorf("slide %d fragment %+v, want %+v", i, got, want[i])
		}
	}
}

// A list whose items appear one at a time is rebuilt rather than decorated:
// the class has to be on the <li> itself, because every one of these
// frameworks hides the element it is given and hiding a span inside a bullet
// leaves the bullet behind.
func TestFragList(t *testing.T) {
	d := parse(t, "- one\n- two\n")
	w := org.NewHTMLWriter()
	w.Document = d
	var list org.List
	for _, n := range d.Nodes {
		if l, ok := n.(org.List); ok {
			list = l
		}
	}
	if list.Items == nil {
		t.Fatalf("no list parsed")
	}
	got := FragList(w, list, func(i int) string { return "fragment" })
	if strings.Count(got, `<li class="fragment">`) != 2 {
		t.Errorf("items are not fragments:\n%s", got)
	}
	if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Errorf("content was lost:\n%s", got)
	}
	if !strings.HasPrefix(got, "<ul>") || !strings.Contains(got, "</ul>") {
		t.Errorf("the list lost its tags:\n%s", got)
	}
}

func TestJSOpts(t *testing.T) {
	c, _ := conf(t, `#+REVEAL_CONTROLS: nil
#+REVEAL_WIDTH: 1280
#+REVEAL_TRANSITION: fade
#+REVEAL_AUTO_SLIDE: 5000
#+REVEAL_MARGIN: banana
`)
	o := NewJSOpts(c)
	o.Bool("CONTROLS", "controls")
	o.Bool("PROGRESS", "progress") // not said: left out entirely
	o.Num("WIDTH", "width")
	o.Num("MARGIN", "margin") // not a number: left out rather than written
	o.Str("TRANSITION", "transition")
	o.NumOrBool("AUTO_SLIDE", "autoSlide")
	got := o.String()
	for _, want := range []string{"controls: false", "width: 1280", "transition: 'fade'", "autoSlide: 5000"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %s", want, got)
		}
	}
	for _, gone := range []string{"progress", "margin"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q should have been left out: %s", gone, got)
		}
	}
	if NewJSOpts(c).String() != "{}" {
		t.Errorf("an empty set is an empty object")
	}
}

// Which way round the text goes on a slide's own ground. The pair most likely
// to be used together is the pair most likely to contradict each other: a
// theme picks an ink to suit its ground, and `:BACKGROUND:` then puts
// something else behind it.
func TestReadInk(t *testing.T) {
	cases := []struct {
		prop string
		want Ink
	}{
		{":BACKGROUND: #102030", InkOnDark},
		{":BACKGROUND: black", InkOnDark},
		{":BACKGROUND: navy", InkOnDark},
		{":BACKGROUND: #fdf6f1", InkOnLight},
		{":BACKGROUND: white", InkOnLight},
		{":BACKGROUND: beige", InkOnLight},
		// Luminance, not brightness: the eye is far more sensitive to green
		// than to blue, so a plain average gets both of these backwards.
		{":BACKGROUND: #0000ff", InkOnDark},
		{":BACKGROUND: #00ff00", InkOnLight},
		// A picture is unknowable, and guessing from a filename would be worse
		// than leaving it to whoever wrote the slide.
		{":BACKGROUND: cover.jpg", InkUnknown},
		// Said outright wins, which is the escape hatch for exactly that case.
		{":BACKGROUND_IMAGE: cover.jpg\n:BACKGROUND_INK: light", InkOnDark},
		{":BACKGROUND: black\n:INK: dark", InkOnLight},
		// Nothing said at all is nothing done: the theme's own ink stands.
		{":CLASS: x", InkUnknown},
	}
	for _, tc := range cases {
		c, d := conf(t, "* One\n:PROPERTIES:\n"+tc.prop+"\n:END:\nBody.\n")
		deck := BuildDeck(c, d)
		p := deck.Slides[0].Props()
		got := ReadInk(c, p, ReadBackground(c, p, "", nil))
		if got != tc.want {
			t.Errorf("%q: ink %d, want %d", strings.ReplaceAll(tc.prop, "\n", " / "), got, tc.want)
		}
	}
}

func TestLuminance(t *testing.T) {
	for _, tc := range []struct {
		css  string
		dark bool
	}{
		{"#000", true}, {"#fff", false}, {"#121316", true}, {"#faf7f1", false},
		{"rgb(12, 26, 36)", true}, {"rgba(250, 247, 241, 0.9)", false},
		{"linear-gradient(160deg, #fdf1ea 0%, #f3f1fb 100%)", false},
		{"linear-gradient(#0e1130, #161a42)", true},
	} {
		l, ok := Luminance(tc.css)
		if !ok {
			t.Errorf("%q could not be read", tc.css)
			continue
		}
		if (l < 0.42) != tc.dark {
			t.Errorf("%q came out %.3f, want dark=%v", tc.css, l, tc.dark)
		}
	}
	if _, ok := Luminance("cover.jpg"); ok {
		t.Errorf("a filename is not a colour")
	}
}

// A theme is one file that works in all four frameworks. The names are read off
// the disk so one dropped into the templates folder turns up without a rebuild.
func TestThemes(t *testing.T) {
	// The themes live in the repository's templates folder, which is five
	// levels up from this package - the same place the server finds through
	// `templatePath:`.
	old := SearchPath
	SearchPath = filepath.Join("..", "..", "..", "..", "..", "templates")
	defer func() { SearchPath = old }()

	names := ThemeNames()
	if len(names) < 4 {
		t.Fatalf("expected the shipped themes, got %v", names)
	}
	for _, n := range names {
		if n == "base" {
			t.Errorf("the mapping is not a theme")
		}
		th := LoadTheme(n)
		if !th.Found || th.CSS == "" {
			t.Errorf("%s did not load", n)
		}
		// The mapping has to come with it, or the theme sets properties that
		// nothing reads.
		if !strings.Contains(th.CSS, "--slide-head-ink") {
			t.Errorf("%s loaded without the mapping", n)
		}
	}
	// A name with a path in it is not a theme name.
	for _, bad := range []string{"../secret", "a/b", "x.css", ""} {
		if LoadTheme(bad).Found {
			t.Errorf("%q should not have loaded", bad)
		}
	}
	// A name this server has no file for belongs to the framework.
	if LoadTheme("dracula").Found {
		t.Errorf("reveal's own theme names must be passed through, not claimed")
	}
}

// The backdrop: which scene a deck gets, and the rules that decide it.
//
// The one that matters most is the last - a deck has to be able to say "not
// this", because a backdrop is the first thing to go when a projector cannot
// cope or the room cannot read it.
func TestBackdrop(t *testing.T) {
	old := SearchPath
	SearchPath = filepath.Join("..", "..", "..", "..", "..", "templates")
	defer func() { SearchPath = old }()

	// A theme that asks for one, read out of its own comment header.
	th := LoadTheme("voidcubes")
	if !th.Found {
		t.Fatalf("voidcubes did not load")
	}
	if th.Backdrop != "cubes" {
		t.Errorf("voidcubes asks for cubes, got %q", th.Backdrop)
	}
	c, _ := conf(t, "#+TITLE: A Talk\n\n* One\n")
	b := ReadBackdrop(c, th)
	if b.Scene != "cubes" || b.JS == "" || b.CSS == "" {
		t.Errorf("the theme's scene did not come through: %+v", b.Scene)
	}
	if !strings.Contains(b.Config, "scene:'cubes'") {
		t.Errorf("config does not name the scene: %s", b.Config)
	}

	// A theme that does not ask for one gets none, and the page is what it was
	// before any of this existed.
	if got := ReadBackdrop(c, LoadTheme("mono")); got.Scene != "" || got.JS != "" {
		t.Errorf("mono asks for no backdrop, got %q", got.Scene)
	}

	// The document has the last word, both ways round.
	c2, _ := conf(t, "#+TITLE: A Talk\n#+SLIDE_BACKDROP: globe\n\n* One\n")
	if got := ReadBackdrop(c2, th).Scene; got != "globe" {
		t.Errorf("the file asked for globe, got %q", got)
	}
	for _, off := range []string{"nil", "none", "no", "false", "off"} {
		c3, _ := conf(t, "#+TITLE: A Talk\n#+SLIDE_BACKDROP: "+off+"\n\n* One\n")
		if got := ReadBackdrop(c3, th); got.Scene != "" || got.JS != "" {
			t.Errorf("%q should turn the backdrop off, got %q", off, got.Scene)
		}
	}

	// The scene name reaches the browser inside a javascript string literal, so
	// a name that is not of the shape ours are is refused here rather than
	// written into the page.
	for _, bad := range []string{"cubes'); alert(1)//", "../x", "Cubes Two", "9lives"} {
		c4, _ := conf(t, "#+TITLE: A Talk\n#+SLIDE_BACKDROP: "+bad+"\n\n* One\n")
		if got := ReadBackdrop(c4, th); got.Scene != "" {
			t.Errorf("%q should not have been accepted, got %q", bad, got.Scene)
		}
	}

	// Speed and density are only in the config when the *document* set them -
	// a scene with nothing overridden reads the theme's own properties.
	if strings.Contains(b.Config, "speed") {
		t.Errorf("nothing asked for a speed: %s", b.Config)
	}
	c5, _ := conf(t, "#+TITLE: A Talk\n#+SLIDE_BACKDROP_SPEED: 0.5\n#+SLIDE_BACKDROP_DENSITY: 1.5\n\n* One\n")
	cfg := ReadBackdrop(c5, th).Config
	if !strings.Contains(cfg, "speed:0.5") || !strings.Contains(cfg, "density:1.5") {
		t.Errorf("the document's tuning did not come through: %s", cfg)
	}

	// Every scene a shipped theme asks for has to exist in the javascript. The
	// two are deliberately not checked against each other at run time - the
	// scenes live in the script and a list of their names in Go would be the
	// second copy that goes out of step - so this is where that is caught.
	js, err := readThemeStr("slides_backdrop.js")
	if err != nil {
		t.Fatalf("no backdrop script: %v", err)
	}
	for _, n := range ThemeNames() {
		s := LoadTheme(n).Backdrop
		if s == "" {
			continue
		}
		if !strings.Contains(js, "SCENES."+s+" =") {
			t.Errorf("theme %s asks for a scene %q the script does not have", n, s)
		}
	}
}

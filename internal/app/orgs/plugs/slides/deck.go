package slides

// Turning a document into a deck.
//
// The question every one of these exporters has to answer first is *which
// headlines are slides*, and the answer is org's own: `#+SLIDE_LEVEL: 2` means
// a level-2 headline is a slide, a level-1 headline is the section it belongs
// to, and a level-3 headline is a heading *inside* a slide. The default is 9 -
// every headline is a slide - because that is what the two exporters that
// already existed did, and a deck somebody wrote last year has to keep looking
// the way it looked.
//
// The hard part is not the levels. It is that **a parsed org document is not a
// tree**, or not reliably one.
//
// go-org ends a headline's body at the first drawer or block written in column
// zero, and hoists everything after it to the top level of the document. Emacs
// writes property drawers in column zero by default, so for a file with
// `:PROPERTIES:` under its headings - which is every file that configures a
// slide - the parse comes back as a flat list: headline, property drawer,
// paragraph, list, headline, ... with `Headline.Children` empty and
// `Headline.Properties` nil. Some headlines nest and some do not, in the same
// file, depending on what they happen to contain.
//
// Walking `Headline.Children` is therefore not enough, and it is why the
// reveal.js and impress.js exporters lost the body of any slide that carried a
// property and never read a per-slide property at all: `:REVEAL_TRANSITION:`
// has been silently doing nothing on exactly the slides that asked for it.
//
// So this walks the nodes **in order** and attributes each one to the last
// headline that started above it - the same rule `links.go` and `code.go`
// arrived at, for the same reason - and picks up a hoisted property drawer as
// the properties of the headline it follows. The result is the tree the file
// described, however it happened to parse.

import (
	"strings"

	"github.com/ihdavids/go-org/org"
)

// PropGet is anything that can be asked for a property: a slide (whose
// properties may have been hoisted out of its headline), or a bare headline.
//
// It is a function rather than an interface because the two things that
// implement it are a struct in this package and go-org's headline, and wrapping
// the second in a type of our own would mean every caller converting.
type PropGet func(key string) (string, bool)

// HeadlineProps reads a headline's own drawer - which is right for a heading
// *inside* a slide, where nothing was hoisted.
func HeadlineProps(h *org.Headline) PropGet {
	if h == nil {
		return nil
	}
	return func(key string) (string, bool) { return h.Properties.Get(key) }
}

// Slide is one slide, or one container of slides.
type Slide struct {
	H     *org.Headline // nil for the title slide
	Level int
	Title []org.Node
	Body  []org.Node
	Notes []org.Node
	// props is the headline's property drawer wherever it ended up: nested
	// inside the headline, or hoisted to the top level by the parser.
	props *org.PropertyDrawer
	// Subs are the slides underneath this one: a vertical stack in reveal, a
	// run of further steps in the flat frameworks.
	Subs []*Slide
	// Num is the slide's position in the flattened deck, counting from 1, and
	// is what the generated ids and slide numbers are made of.
	Num int
}

// Props is how a slide is asked about itself.
func (s *Slide) Props() PropGet {
	if s == nil {
		return nil
	}
	return func(key string) (string, bool) {
		if s.props != nil {
			if v, ok := s.props.Get(key); ok {
				return v, ok
			}
		}
		if s.H != nil {
			return s.H.Properties.Get(key)
		}
		return "", false
	}
}

// Tags are the slide's own tags, which is how `:noexport:` is spotted and how a
// deck can style a whole class of slide without repeating a property.
func (s *Slide) Tags() []string {
	if s == nil || s.H == nil {
		return nil
	}
	return s.H.Tags
}

// HasContent reports whether there is anything on this slide but its title.
func (s *Slide) HasContent() bool { return len(s.Body) > 0 || len(s.Notes) > 0 }

// Deck is a whole presentation.
type Deck struct {
	Title    string
	Subtitle string
	Author   string
	Email    string
	Date     string
	// Preamble is whatever was written above the first headline: the title
	// slide's body, which is how a deck puts a subtitle, a logo or an abstract
	// on its first slide without a headline for it.
	Preamble []org.Node
	Slides   []*Slide
	// TitleSlide is whether to draw one at all: a deck whose first headline is
	// already its title page says `#+SLIDE_TITLE_SLIDE: nil`.
	TitleSlide bool
	Level      int
}

type builder struct {
	deck  *Deck
	doc   *org.Document
	level int
	// stack is the open slides, innermost last.
	stack []*Slide
	// divert sends nodes somewhere other than the current slide's body: the
	// speaker notes, when a `** Notes` headline is open.
	divert *[]org.Node
	// skipTo is the level at or above which a new headline ends a skipped
	// subtree - everything excluded from the export, body and all.
	skipping bool
	skipLvl  int
	// last is the headline a hoisted property drawer belongs to.
	last *Slide
	// divertAt is the level of the notes heading currently open.
	divertAt int
}

// BuildDeck reads a parsed document into a deck.
func BuildDeck(c Conf, doc *org.Document) *Deck {
	level := c.DocInt("LEVEL", 9)
	if level < 1 {
		level = 1
	}
	d := &Deck{
		Title:      first(c.DocStr("TITLE", ""), docGet(doc, "TITLE")),
		Subtitle:   first(c.DocStr("SUBTITLE", ""), docGet(doc, "SUBTITLE")),
		Author:     first(c.DocStr("AUTHOR", ""), docGet(doc, "AUTHOR")),
		Email:      first(c.DocStr("EMAIL", ""), docGet(doc, "EMAIL")),
		Date:       first(c.DocStr("DATE", ""), docGet(doc, "DATE")),
		Level:      level,
		TitleSlide: c.DocBool("TITLE_SLIDE", true),
	}
	if doc == nil {
		return d
	}
	b := &builder{deck: d, doc: doc, level: level}
	b.walk(doc.Nodes)
	// A deck with nothing above the first headline and no #+TITLE: has no title
	// slide to draw, whatever the setting says - an empty first slide is worse
	// than starting on the first real one.
	if d.Title == "" && len(d.Preamble) == 0 {
		d.TitleSlide = false
	}
	number(d)
	return d
}

// cur is the slide nodes are landing on.
func (b *builder) cur() *Slide {
	if len(b.stack) == 0 {
		return nil
	}
	return b.stack[len(b.stack)-1]
}

// add puts one body node where it belongs: into the speaker notes while a notes
// heading is open, into the current slide, or into the preamble when nothing is
// open yet.
func (b *builder) add(n org.Node) {
	if b.divert != nil {
		*b.divert = append(*b.divert, n)
		return
	}
	if s := b.cur(); s != nil {
		s.Body = append(s.Body, n)
		return
	}
	b.deck.Preamble = append(b.deck.Preamble, n)
}

func (b *builder) walk(nodes []org.Node) {
	for _, n := range nodes {
		if h := asHeadline(n); h != nil {
			b.headline(h)
			continue
		}
		if b.skipping {
			continue
		}
		// A property drawer is the properties of the heading above it, wherever
		// the parser decided to put it.
		if pd := asPropertyDrawer(n); pd != nil {
			if b.last != nil && b.last.props == nil {
				b.last.props = pd
			}
			continue
		}
		if notes, ok := notesOf(n); ok {
			b.notes(notes)
			continue
		}
		b.add(n)
	}
}

func (b *builder) notes(notes []org.Node) {
	if s := b.cur(); s != nil {
		s.Notes = append(s.Notes, notes...)
	}
}

func (b *builder) headline(h *org.Headline) {
	// Any headline at or above the skipped subtree's level ends the skip.
	if b.skipping {
		if h.Lvl > b.skipLvl {
			return
		}
		b.skipping = false
	}
	// A notes heading ends where the next heading at its level or above begins.
	if b.divert != nil && h.Lvl <= b.divertLvl() {
		b.divert = nil
	}

	if h.IsExcluded(b.doc) || isNoExport(h) {
		b.skipping, b.skipLvl = true, h.Lvl
		return
	}

	// Pop back to the headline this one belongs under.
	for len(b.stack) > 0 && b.stack[len(b.stack)-1].Level >= h.Lvl {
		b.stack = b.stack[:len(b.stack)-1]
	}

	if isNotesHeadline(h) {
		// The speaker's notes: this heading's own children, plus everything
		// hoisted after it, until the next heading at its level or above.
		if s := b.cur(); s != nil {
			s.Notes = append(s.Notes, h.Children...)
			b.divert = &s.Notes
			b.divertAt = h.Lvl
		}
		b.last = nil
		return
	}

	if h.Lvl > b.level {
		// Deeper than the slide level: a heading *inside* the current slide.
		// The node is added whole - the writer renders it as a heading and its
		// nested children - and the slide stays open, so anything the parser
		// hoisted after it lands on the same slide, in order.
		b.add(h)
		b.last = nil
		// Its own nested children are rendered with it, so they are not walked
		// again here.
		return
	}

	s := &Slide{H: h, Level: h.Lvl, Title: h.Title, props: h.Properties}
	if parent := b.cur(); parent != nil {
		parent.Subs = append(parent.Subs, s)
	} else {
		b.deck.Slides = append(b.deck.Slides, s)
	}
	b.stack = append(b.stack, s)
	b.last = s
	// Whatever did nest under this headline is walked in place, so a file that
	// parsed as a tree and a file that parsed flat come out the same.
	b.walk(h.Children)
}

func (b *builder) divertLvl() int { return b.divertAt }

func number(d *Deck) {
	n := 0
	if d.TitleSlide {
		n++
	}
	var walk func(s *Slide)
	walk = func(s *Slide) {
		n++
		s.Num = n
		for _, sub := range s.Subs {
			walk(sub)
		}
	}
	for _, s := range d.Slides {
		walk(s)
	}
}

// Flat is the deck as a sequence, which is what the frameworks that cannot nest
// want.
func (d *Deck) Flat() []*Slide {
	out := []*Slide{}
	var walk func(s *Slide)
	walk = func(s *Slide) {
		// A container headline with nothing of its own is still drawn: its
		// title is the only place the name of this part of the talk is written
		// down, and a section divider is a slide people expect.
		out = append(out, s)
		for _, sub := range s.Subs {
			walk(sub)
		}
	}
	for _, s := range d.Slides {
		walk(s)
	}
	return out
}

// Count is how many slides the deck has once flattened, title slide included.
func (d *Deck) Count() int {
	n := len(d.Flat())
	if d.TitleSlide {
		n++
	}
	return n
}

func docGet(doc *org.Document, key string) string {
	if doc == nil {
		return ""
	}
	return strings.TrimSpace(doc.Get(key))
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// asHeadline answers for both spellings. Only the pointer actually occurs, but
// a type switch that quietly handles one of two possibilities is how the
// tangler and the habit tracker both came to do nothing at all.
func asHeadline(n org.Node) *org.Headline {
	switch v := n.(type) {
	case *org.Headline:
		return v
	case org.Headline:
		cp := v
		return &cp
	}
	return nil
}

func asPropertyDrawer(n org.Node) *org.PropertyDrawer {
	switch v := n.(type) {
	case *org.PropertyDrawer:
		return v
	case org.PropertyDrawer:
		cp := v
		return &cp
	}
	return nil
}

func isNoExport(h *org.Headline) bool {
	for _, t := range h.Tags {
		switch strings.ToLower(t) {
		case "noexport", "noslide":
			return true
		}
	}
	return false
}

var notesNames = map[string]bool{
	"notes": true, "note": true, "speaker notes": true, "speakernotes": true,
}

func isNotesHeadline(h *org.Headline) bool {
	if h == nil {
		return false
	}
	for _, t := range h.Tags {
		if strings.EqualFold(t, "notes") {
			return true
		}
	}
	return notesNames[strings.ToLower(strings.TrimSpace(org.String(h.Title...)))]
}

// notesOf answers for the two ways notes are written inside a slide's body: a
// drawer, and a block.
func notesOf(n org.Node) ([]org.Node, bool) {
	switch v := n.(type) {
	case *org.Drawer:
		if notesNames[strings.ToLower(v.Name)] {
			return v.Children, true
		}
	case org.Drawer:
		if notesNames[strings.ToLower(v.Name)] {
			return v.Children, true
		}
	case *org.Block:
		if notesNames[strings.ToLower(v.Name)] {
			return v.Children, true
		}
	case org.Block:
		if notesNames[strings.ToLower(v.Name)] {
			return v.Children, true
		}
	}
	return nil, false
}

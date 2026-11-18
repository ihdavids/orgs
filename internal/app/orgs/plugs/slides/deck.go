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
// Each headline's own drawer is its properties and its children are what
// nested under it: go-org keeps a drawer written in column zero (what Emacs
// writes) inside the headline, so the parse is the tree the file describes.
// The walk is still in document order, so the body, the speaker notes and the
// headings inside a slide come out as written.

import (
	"strings"

	"github.com/ihdavids/go-org/org"
)

// PropGet is anything that can be asked for a property: a slide, or a bare
// headline.
//
// It is a function rather than an interface because the two things that
// implement it are a struct in this package and go-org's headline, and wrapping
// the second in a type of our own would mean every caller converting.
type PropGet func(key string) (string, bool)

// HeadlineProps reads a headline's own drawer, for a heading *inside* a slide.
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

// add puts one body node where it belongs: into the current slide, or into the
// preamble when nothing is open yet.
func (b *builder) add(n org.Node) {
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
		// The only property drawer left among the nodes is the file's own,
		// above the first heading: settings, not something to show.
		if asPropertyDrawer(n) != nil {
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
	// An excluded heading goes with everything under it.
	if h.IsExcluded(b.doc) || isNoExport(h) {
		return
	}

	// Pop back to the headline this one belongs under.
	for len(b.stack) > 0 && b.stack[len(b.stack)-1].Level >= h.Lvl {
		b.stack = b.stack[:len(b.stack)-1]
	}

	if isNotesHeadline(h) {
		// The speaker's notes: everything under this heading.
		if s := b.cur(); s != nil {
			s.Notes = append(s.Notes, h.Children...)
		}
		return
	}

	if h.Lvl > b.level {
		// Deeper than the slide level: a heading *inside* the current slide,
		// added whole - the writer renders it as a heading and its nested
		// children, so they are not walked again here.
		b.add(h)
		return
	}

	s := &Slide{H: h, Level: h.Lvl, Title: h.Title}
	if parent := b.cur(); parent != nil {
		parent.Subs = append(parent.Subs, s)
	} else {
		b.deck.Slides = append(b.deck.Slides, s)
	}
	b.stack = append(b.stack, s)
	b.walk(h.Children)
}

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

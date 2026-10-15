package manuscript

// An org file read as a book: front matter, parts, chapters and scenes, made
// of paragraphs with their emphasis, and nothing else. Both writers (docx and
// epub) work from this model and never from the org tree, so they cannot
// disagree about what a chapter is or where a scene breaks.

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ihdavids/go-org/org"
)

// Meta is everything a title page, a manuscript's first page or an epub's
// package document says about the book.
type Meta struct {
	Title       string
	Subtitle    string
	Author      string // the legal name, for the contact block
	Byline      string // the name it is published under (#+PEN_NAME)
	Email       string
	Phone       string
	Address     []string
	Website     string
	Agent       []string
	Language    string
	Date        string
	Description string
	Publisher   string
	Rights      string
	Identifier  string
	Series      string
	SeriesIndex string
	Subjects    []string
	Cover       string // an image's absolute path
	Dedication  string
	EpubCSS     string // a stylesheet's absolute path, after the theme
	SceneMark   string // what a scene break is written as, when the file says
	ChapterNums string // none, digits, words, roman
	ChapterWord string // "Chapter"
}

// Name is the byline, else the author.
func (m *Meta) Name() string {
	if m.Byline != "" {
		return m.Byline
	}
	return m.Author
}

// Surname is the last word of the author's name, as a manuscript's running
// header wants it.
func (m *Meta) Surname() string {
	f := strings.Fields(m.Name())
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

type Run struct {
	Text      string
	Italic    bool
	Bold      bool
	Underline bool
	Strike    bool
	Code      bool
	Sup       bool
	Sub       bool
	Break     bool   // a line break, not text
	Note      string // a footnote reference, by name
	Link      string // an http(s) or mailto target
}

type Block struct {
	Kind string // p, scene, heading, quote, verse, center, pre, ul, ol, dl, table, image
	Runs []Run
	// No first-line indent: the first paragraph of a chapter or scene.
	First bool
	Level int // heading: 1 is the first level under a chapter

	Children []Block   // quote, center
	Items    [][]Block // ul, ol: an item's blocks; dl: its details
	Terms    [][]Run   // dl
	Rows     [][][]Run // table: rows of cells
	HeadRows int

	Src     string // image: an absolute path, or an http(s) url
	Caption []Run
}

type Chapter struct {
	Kind   string // chapter, part, front, back
	Title  []Run
	Number int // 0 for unnumbered
	Label  string
	Blocks []Block
}

type Book struct {
	Meta      Meta
	Chapters  []*Chapter
	Notes     map[string][]Block
	NoteOrder []string
	Words     int
}

// Options a writer's caller can set, past what the file says.
type Options struct {
	// Headings below chapter level are scene breaks (fiction) rather than
	// sub-headings (non-fiction).
	Scenes bool
}

// Paragraphs that are only a scene mark. ("#" and "* * *" are not here: org
// reads a lone # as a comment and * * * as a heading.)
var sceneMarks = map[string]bool{"***": true, "⁂": true, "§": true, "~": true, "•": true, "• • •": true}

// Build reads a parsed org document as a book.
func Build(doc *org.Document, path string, defaults Meta, o Options) *Book {
	b := &Book{Notes: map[string][]Block{}}
	b.Meta = metaOf(doc, path, defaults)
	r := &reader{book: b, dir: filepath.Dir(path), doc: doc, opt: o}
	r.collectNotes(doc.Nodes)
	r.walk(doc.Nodes)
	r.finish()
	return b
}

type reader struct {
	book *Book
	dir  string
	doc  *org.Document
	opt  Options

	cur        *Chapter
	partLevel  int // 0: no parts
	chapLevel  int
	nextNumber int
	afterBreak bool // the next paragraph opens a chapter or scene
	inNotes    bool // a usage note's blocks go to the notes, not a chapter
}

func kw(doc *org.Document, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(doc.Get(k)); v != "" {
			return v
		}
	}
	return ""
}

func lines(s string) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		for _, p := range strings.Split(l, "\\\\") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

func metaOf(doc *org.Document, path string, d Meta) Meta {
	m := d
	set := func(dst *string, keys ...string) {
		if v := kw(doc, keys...); v != "" {
			*dst = v
		}
	}
	set(&m.Title, "TITLE")
	set(&m.Subtitle, "SUBTITLE")
	set(&m.Author, "AUTHOR")
	set(&m.Byline, "PEN_NAME", "BYLINE")
	set(&m.Email, "EMAIL")
	set(&m.Phone, "PHONE")
	set(&m.Website, "WEBSITE", "URL")
	set(&m.Language, "LANGUAGE")
	set(&m.Date, "DATE")
	set(&m.Description, "DESCRIPTION")
	set(&m.Publisher, "PUBLISHER")
	set(&m.Rights, "RIGHTS", "COPYRIGHT")
	set(&m.Identifier, "ISBN", "IDENTIFIER")
	set(&m.Series, "SERIES")
	set(&m.SeriesIndex, "SERIES_INDEX")
	set(&m.Dedication, "DEDICATION")
	set(&m.SceneMark, "SCENE_BREAK")
	set(&m.ChapterNums, "CHAPTER_NUMBERS")
	set(&m.ChapterWord, "CHAPTER_LABEL")
	if v := kw(doc, "ADDRESS"); v != "" {
		m.Address = lines(v)
	}
	if v := kw(doc, "AGENT"); v != "" {
		m.Agent = lines(v)
	}
	if v := kw(doc, "KEYWORDS", "SUBJECT"); v != "" {
		m.Subjects = nil
		for _, s := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
			if s = strings.TrimSpace(s); s != "" {
				m.Subjects = append(m.Subjects, s)
			}
		}
	}
	abs := func(p string) string {
		p = strings.TrimPrefix(strings.TrimSpace(p), "file:")
		if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			return p
		}
		return filepath.Join(filepath.Dir(path), p)
	}
	if v := kw(doc, "COVER", "EPUB_COVER"); v != "" {
		m.Cover = abs(strings.Trim(v, "[]"))
	}
	if v := kw(doc, "EPUB_CSS"); v != "" {
		m.EpubCSS = abs(v)
	}
	if m.Title == "" {
		base := filepath.Base(path)
		m.Title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if m.Language == "" {
		m.Language = "en"
	}
	if m.ChapterWord == "" {
		m.ChapterWord = "Chapter"
	}
	return m
}

// ----------------------------------------------------------------------------
// The outline
// ----------------------------------------------------------------------------

func asHeadline(n org.Node) *org.Headline {
	switch h := n.(type) {
	case *org.Headline:
		return h
	case org.Headline:
		return &h
	}
	return nil
}

func hasTag(h *org.Headline, names ...string) bool {
	for _, t := range h.Tags {
		for _, n := range names {
			if strings.EqualFold(t, n) {
				return true
			}
		}
	}
	return false
}

func prop(h *org.Headline, name string) string {
	if h.Properties == nil {
		return ""
	}
	for _, p := range h.Properties.Properties {
		if len(p) > 1 && strings.EqualFold(p[0], name) {
			return strings.TrimSpace(p[1])
		}
	}
	return ""
}

// The heading levels of parts and chapters: a :part: heading's children are
// chapters; otherwise the shallowest heading is.
func (r *reader) levels(nodes []org.Node) {
	min := 0
	var visit func([]org.Node)
	visit = func(ns []org.Node) {
		for _, n := range ns {
			h := asHeadline(n)
			if h == nil {
				continue
			}
			if skip(h) {
				continue
			}
			if hasTag(h, "part") && r.partLevel == 0 {
				r.partLevel = h.Lvl
			}
			if !hasTag(h, "part", "ignore") && (min == 0 || h.Lvl < min) {
				min = h.Lvl
			}
			visit(h.Children)
		}
	}
	visit(nodes)
	r.chapLevel = min
	if r.partLevel > 0 && r.chapLevel <= r.partLevel {
		r.chapLevel = r.partLevel + 1
	}
}

func skip(h *org.Headline) bool {
	return h.IsComment || hasTag(h, "noexport")
}

func isNotesHeading(h *org.Headline) bool {
	t := strings.TrimSpace(org.String(h.Title...))
	return h.Lvl == 1 && (strings.EqualFold(t, "Footnotes") || strings.EqualFold(t, "Notes") && hasTag(h, "notes"))
}

func (r *reader) walk(nodes []org.Node) {
	if r.chapLevel == 0 {
		r.levels(nodes)
	}
	for _, n := range nodes {
		if h := asHeadline(n); h != nil {
			r.heading(h)
			continue
		}
		r.body(n)
	}
}

func (r *reader) open(kind string, title []Run, h *org.Headline) {
	c := &Chapter{Kind: kind, Title: title}
	if kind == "chapter" && (h == nil || !(hasTag(h, "unnumbered") || strings.EqualFold(prop(h, "UNNUMBERED"), "t"))) {
		r.nextNumber++
		c.Number = r.nextNumber
		c.Label = chapterLabel(r.book.Meta, c.Number)
	}
	r.book.Chapters = append(r.book.Chapters, c)
	r.cur = c
	r.afterBreak = true
}

func (r *reader) heading(h *org.Headline) {
	if skip(h) || isNotesHeading(h) {
		return
	}
	title := r.inline(h.Title)
	switch {
	case hasTag(h, "ignore"):
		// The heading is only a fold in the outline; its text belongs to
		// whatever it is in.
	case r.partLevel > 0 && h.Lvl == r.partLevel && hasTag(h, "part"):
		r.open("part", title, h)
	case h.Lvl <= r.chapLevel:
		kind := "chapter"
		if hasTag(h, "frontmatter", "front", "dedication", "epigraph") {
			kind = "front"
		} else if hasTag(h, "backmatter", "back", "appendix") {
			kind = "back"
		}
		r.open(kind, title, h)
	case r.opt.Scenes && !strings.EqualFold(prop(h, "SCENE"), "title"):
		r.ensure()
		if len(r.cur.Blocks) > 0 {
			r.cur.Blocks = append(r.cur.Blocks, Block{Kind: "scene"})
		}
		r.afterBreak = true
	default:
		r.ensure()
		r.cur.Blocks = append(r.cur.Blocks, Block{Kind: "heading", Level: h.Lvl - r.chapLevel, Runs: title})
		r.afterBreak = true
	}
	for _, n := range h.Children {
		if c := asHeadline(n); c != nil {
			r.heading(c)
			continue
		}
		r.body(n)
	}
}

// ensure there is a chapter to write into: a story with no headings at all is
// one untitled chapter.
func (r *reader) ensure() {
	if r.cur == nil {
		r.cur = &Chapter{Kind: "chapter"}
		r.book.Chapters = append(r.book.Chapters, r.cur)
		r.afterBreak = true
	}
}

func (r *reader) add(b Block) {
	if b.Kind == "p" && len(b.Runs) == 0 {
		return
	}
	r.ensure()
	if b.Kind == "scene" {
		if len(r.cur.Blocks) == 0 || r.cur.Blocks[len(r.cur.Blocks)-1].Kind == "scene" {
			return
		}
		r.afterBreak = true
		r.cur.Blocks = append(r.cur.Blocks, b)
		return
	}
	if b.Kind == "p" {
		b.First = r.afterBreak
	}
	r.afterBreak = false
	r.cur.Blocks = append(r.cur.Blocks, b)
}

func (r *reader) body(n org.Node) {
	for _, b := range r.blocks(n) {
		r.add(b)
	}
}

func (r *reader) finish() {
	// A scene break at the very end of a chapter is nothing.
	for _, c := range r.book.Chapters {
		for len(c.Blocks) > 0 && c.Blocks[len(c.Blocks)-1].Kind == "scene" {
			c.Blocks = c.Blocks[:len(c.Blocks)-1]
		}
	}
	n := 0
	for _, c := range r.book.Chapters {
		for _, b := range c.Blocks {
			n += wordsIn(b)
		}
	}
	r.book.Words = n
}

// ----------------------------------------------------------------------------
// Blocks
// ----------------------------------------------------------------------------

func (r *reader) blocks(n org.Node) []Block {
	switch n := n.(type) {
	case org.Paragraph:
		return r.paragraph(n.Children, nil)
	case *org.Paragraph:
		return r.paragraph(n.Children, nil)
	case org.HorizontalRule:
		return []Block{{Kind: "scene"}}
	case org.NodeWithMeta:
		var caption []Run
		if len(n.Meta.Caption) > 0 {
			caption = r.inline(n.Meta.Caption[len(n.Meta.Caption)-1])
		}
		if p, ok := n.Node.(org.Paragraph); ok {
			return r.paragraph(p.Children, caption)
		}
		return r.blocks(n.Node)
	case org.NodeWithName:
		return r.blocks(n.Node)
	case *org.NodeWithName:
		return r.blocks(n.Node)
	case org.Block:
		return r.block(&n)
	case *org.Block:
		return r.block(n)
	case org.Example:
		return []Block{{Kind: "pre", Runs: []Run{{Text: strings.TrimRight(exampleText(n.Children), "\n")}}}}
	case org.List:
		return r.list(n)
	case org.Table:
		return r.table(&n)
	case *org.Table:
		return r.table(n)
	case org.FootnoteDefinition:
		return nil // collected beforehand
	}
	// Drawers, keywords, comments, planning, results: not prose.
	return nil
}

func exampleText(ns []org.Node) string {
	var b strings.Builder
	for _, n := range ns {
		b.WriteString(org.String(n))
	}
	return b.String()
}

func (r *reader) paragraph(ns []org.Node, caption []Run) []Block {
	// A picture on a line of its own is a figure, not a word in a sentence.
	if img := r.lone(ns); img != "" {
		return []Block{{Kind: "image", Src: img, Caption: caption}}
	}
	// The source, not the runs: *** parses as a bold star.
	if sceneMarks[strings.TrimSpace(org.String(ns...))] {
		return []Block{{Kind: "scene"}}
	}
	runs := r.inline(ns)
	if len(runs) == 0 {
		return nil
	}
	return []Block{{Kind: "p", Runs: runs}}
}

var imageExt = regexp.MustCompile(`(?i)\.(png|jpe?g|gif|svg|webp)$`)

func (r *reader) lone(ns []org.Node) string {
	var link *org.RegularLink
	for _, n := range ns {
		switch n := n.(type) {
		case org.RegularLink:
			if link != nil {
				return ""
			}
			l := n
			link = &l
		case org.Text:
			if strings.TrimSpace(n.Content) != "" {
				return ""
			}
		case org.LineBreak:
		default:
			return ""
		}
	}
	if link == nil || len(link.Description) > 0 && !imageExt.MatchString(org.String(link.Description...)) {
		return ""
	}
	u := link.URL
	if link.Protocol == "file" || link.Protocol == "" {
		u = strings.TrimPrefix(u, "file:")
		if !imageExt.MatchString(u) {
			return ""
		}
		if !filepath.IsAbs(u) {
			u = filepath.Join(r.dir, u)
		}
		return u
	}
	if (link.Protocol == "http" || link.Protocol == "https") && imageExt.MatchString(u) {
		return u
	}
	return ""
}

func (r *reader) block(b *org.Block) []Block {
	name := strings.ToUpper(b.Name)
	switch name {
	case "QUOTE":
		return []Block{{Kind: "quote", Children: r.children(b.Children)}}
	case "CENTER":
		return []Block{{Kind: "center", Children: r.children(b.Children)}}
	case "VERSE":
		return []Block{{Kind: "verse", Runs: r.verse(b.Children)}}
	case "SRC", "EXAMPLE":
		return []Block{{Kind: "pre", Runs: []Run{{Text: strings.TrimRight(exampleText(b.Children), "\n")}}}}
	}
	// COMMENT, EXPORT and the rest are not part of the book.
	return nil
}

func (r *reader) children(ns []org.Node) []Block {
	out := []Block{}
	for _, n := range ns {
		out = append(out, r.blocks(n)...)
	}
	for i := range out {
		out[i].First = true
	}
	return out
}

// verse keeps its lines: every line break in it is a break.
func (r *reader) verse(ns []org.Node) []Run {
	runs := []Run{}
	for _, n := range ns {
		var in []Run
		if p, ok := n.(org.Paragraph); ok {
			if len(runs) > 0 {
				runs = append(runs, Run{Break: true}, Run{Break: true})
			}
			in = r.inlineKeep(p.Children, true)
		} else {
			in = r.inlineKeep([]org.Node{n}, true)
		}
		runs = append(runs, in...)
	}
	for len(runs) > 0 && runs[len(runs)-1].Break {
		runs = runs[:len(runs)-1]
	}
	return runs
}

func (r *reader) list(l org.List) []Block {
	kind := "ul"
	switch l.Kind {
	case "ordered":
		kind = "ol"
	case "descriptive":
		kind = "dl"
	}
	b := Block{Kind: kind}
	for _, it := range l.Items {
		switch it := it.(type) {
		case org.ListItem:
			b.Items = append(b.Items, r.children(it.Children))
		case org.DescriptiveListItem:
			b.Terms = append(b.Terms, r.inline(it.Term))
			b.Items = append(b.Items, r.children(it.Details))
		}
	}
	return []Block{b}
}

func (r *reader) table(t *org.Table) []Block {
	b := Block{Kind: "table"}
	for i, row := range t.Rows {
		if row.IsSpecial || len(row.Columns) == 0 {
			if len(b.Rows) > 0 && b.HeadRows == 0 && i > 0 {
				b.HeadRows = len(b.Rows)
			}
			continue
		}
		cells := [][]Run{}
		for _, c := range row.Columns {
			cells = append(cells, r.inline(c.Children))
		}
		b.Rows = append(b.Rows, cells)
	}
	if b.HeadRows == len(b.Rows) {
		b.HeadRows = 0
	}
	if len(b.Rows) == 0 {
		return nil
	}
	return []Block{b}
}

// ----------------------------------------------------------------------------
// Footnotes
// ----------------------------------------------------------------------------

func (r *reader) collectNotes(nodes []org.Node) {
	var visit func([]org.Node)
	visit = func(ns []org.Node) {
		for _, n := range ns {
			switch n := n.(type) {
			case org.FootnoteDefinition:
				r.book.Notes[n.Name] = r.children(n.Children)
			case *org.Headline:
				visit(n.Children)
			case org.Headline:
				visit(n.Children)
			}
		}
	}
	visit(nodes)
}

func (r *reader) noteRef(f org.FootnoteLink) Run {
	name := f.Name
	if f.Definition != nil {
		if name == "" {
			name = "anon-" + strconv.Itoa(len(r.book.Notes)+1)
		}
		if _, ok := r.book.Notes[name]; !ok {
			r.book.Notes[name] = r.children(f.Definition.Children)
		}
	}
	seen := false
	for _, n := range r.book.NoteOrder {
		if n == name {
			seen = true
		}
	}
	if !seen {
		r.book.NoteOrder = append(r.book.NoteOrder, name)
	}
	return Run{Note: name}
}

// ----------------------------------------------------------------------------
// Inline
// ----------------------------------------------------------------------------

func (r *reader) inline(ns []org.Node) []Run { return r.inlineKeep(ns, false) }

// inlineKeep turns inline nodes into runs. A line break in a paragraph is a
// space, the way it is read; in verse it is a line break.
func (r *reader) inlineKeep(ns []org.Node, keepLines bool) []Run {
	out := []Run{}
	var walk func(ns []org.Node, st Run)
	text := func(s string, st Run) {
		if keepLines && strings.Contains(s, "\n") {
			parts := strings.Split(s, "\n")
			for i, p := range parts {
				if i > 0 {
					out = append(out, Run{Break: true})
				}
				if p != "" {
					st.Text = p
					out = append(out, st)
				}
			}
			return
		}
		st.Text = strings.ReplaceAll(s, "\n", " ")
		out = append(out, st)
	}
	walk = func(ns []org.Node, st Run) {
		for _, n := range ns {
			switch n := n.(type) {
			case org.Text:
				text(n.Content, st)
			case org.LineBreak:
				if keepLines {
					for i := 0; i < max(n.Count, 1); i++ {
						out = append(out, Run{Break: true})
					}
				} else {
					text(" ", st)
				}
			case org.ExplicitLineBreak:
				out = append(out, Run{Break: true})
			case org.Emphasis:
				s := st
				switch n.Kind {
				case "/":
					s.Italic = true
				case "*":
					s.Bold = true
				case "_":
					s.Underline = true
				case "+":
					s.Strike = true
				case "=", "~":
					s.Code = true
				case "^{}":
					s.Sup = true
				case "_{}":
					s.Sub = true
				}
				walk(n.Content, s)
			case org.RegularLink:
				s := st
				if p := n.Protocol; p == "http" || p == "https" || p == "mailto" {
					s.Link = n.URL
				}
				if len(n.Description) > 0 {
					walk(n.Description, s)
				} else if !imageExt.MatchString(n.URL) {
					text(strings.TrimPrefix(n.URL, "mailto:"), s)
				}
			case org.FootnoteLink:
				out = append(out, r.noteRef(n))
			case org.Macro:
				text(r.macro(n), st)
			case org.StatisticToken, org.Timestamp:
				// Bookkeeping, not prose.
			case org.LatexFragment:
				text(org.String(n.Content...), st)
			default:
				text(org.String(n), st)
			}
		}
	}
	walk(ns, Run{})
	return tidy(out)
}

func (r *reader) macro(m org.Macro) string {
	switch strings.ToLower(m.Name) {
	case "title":
		return r.book.Meta.Title
	case "author":
		return r.book.Meta.Name()
	case "date":
		return r.book.Meta.Date
	case "email":
		return r.book.Meta.Email
	}
	body, ok := r.doc.Macros[m.Name]
	if !ok {
		return ""
	}
	for i, p := range m.Parameters {
		body = strings.ReplaceAll(body, "$"+strconv.Itoa(i+1), p)
	}
	return body
}

// tidy joins runs of the same look, folds runs of spaces and trims the ends,
// because org's line breaks leave spaces wherever a line was wrapped.
func tidy(rs []Run) []Run {
	out := []Run{}
	for _, r := range rs {
		if !r.Break && r.Note == "" {
			r.Text = spaceRe.ReplaceAllString(r.Text, " ")
			if r.Text == "" {
				continue
			}
		}
		if n := len(out); n > 0 && !r.Break && r.Note == "" && !out[n-1].Break && out[n-1].Note == "" {
			p := out[n-1]
			pt, rt := p.Text, r.Text
			p.Text, r.Text = "", ""
			if p == r {
				out[n-1].Text = pt + rt
				if strings.HasSuffix(pt, " ") && strings.HasPrefix(rt, " ") {
					out[n-1].Text = pt + rt[1:]
				}
				continue
			}
			r.Text = rt
		}
		out = append(out, r)
	}
	// No space at either end of the paragraph, or after a break.
	for i := range out {
		if out[i].Break || out[i].Note != "" {
			continue
		}
		if i == 0 || out[i-1].Break {
			out[i].Text = strings.TrimLeft(out[i].Text, " ")
		}
		if i == len(out)-1 || out[i+1].Break {
			out[i].Text = strings.TrimRight(out[i].Text, " ")
		}
	}
	clean := out[:0]
	for _, r := range out {
		if r.Break || r.Note != "" || r.Text != "" {
			clean = append(clean, r)
		}
	}
	return clean
}

var spaceRe = regexp.MustCompile(`[ \t]+`)

func plain(rs []Run) string {
	var b strings.Builder
	for _, r := range rs {
		if r.Break {
			b.WriteString("\n")
		}
		b.WriteString(r.Text)
	}
	return b.String()
}

// PlainTitle is a chapter's title as text.
func PlainTitle(rs []Run) string { return strings.TrimSpace(plain(rs)) }

func countWords(s string) int {
	n, in := 0, false
	for _, c := range s {
		w := unicode.IsLetter(c) || unicode.IsDigit(c) || c == '\'' || c == '’' || c == '-'
		if w && !in {
			n++
		}
		in = w
	}
	return n
}

func wordsIn(b Block) int {
	n := countWords(plain(b.Runs))
	for _, c := range b.Children {
		n += wordsIn(c)
	}
	for _, it := range b.Items {
		for _, c := range it {
			n += wordsIn(c)
		}
	}
	for _, t := range b.Terms {
		n += countWords(plain(t))
	}
	for _, row := range b.Rows {
		for _, c := range row {
			n += countWords(plain(c))
		}
	}
	return n
}

// ----------------------------------------------------------------------------
// Chapter labels
// ----------------------------------------------------------------------------

var ones = []string{"Zero", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten",
	"Eleven", "Twelve", "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen"}
var tens = []string{"", "", "Twenty", "Thirty", "Forty", "Fifty", "Sixty", "Seventy", "Eighty", "Ninety"}

// NumberWords spells a chapter number: 21 is "Twenty-One".
func NumberWords(n int) string {
	switch {
	case n < 20:
		return ones[n]
	case n < 100:
		if n%10 == 0 {
			return tens[n/10]
		}
		return tens[n/10] + "-" + ones[n%10]
	case n < 1000:
		s := ones[n/100] + " Hundred"
		if n%100 != 0 {
			s += " " + NumberWords(n%100)
		}
		return s
	}
	return strconv.Itoa(n)
}

func Roman(n int) string {
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

func chapterLabel(m Meta, n int) string {
	switch strings.ToLower(m.ChapterNums) {
	case "digits", "numbers", "arabic":
		return m.ChapterWord + " " + strconv.Itoa(n)
	case "words":
		return m.ChapterWord + " " + NumberWords(n)
	case "roman":
		return m.ChapterWord + " " + Roman(n)
	}
	return ""
}

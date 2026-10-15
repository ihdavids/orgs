// Package site turns a folder of org files into a static web site: a page per
// file, a home page, tag pages, an archive, backlinks, an Atom feed, a sitemap
// and a search box that works without a server - all of it in a theme.
//
// It is handed parsed documents and answers with the site as a map of paths to
// bytes. Where those bytes go (a zip for the client, a folder on the server's
// disk) is the caller's business, which keeps this package free of the server
// and easy to test.
//
// Every link in the output is relative, so the site works opened straight from
// disk (file://), served from the root of a domain, or from any path under one.
package site

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"gopkg.in/yaml.v3"
)

// Source is one org file handed to Build.
type Source struct {
	// Absolute path of the file.
	Path string
	Doc  *org.Document
}

// NavLink is one link of the site's navigation bar.
type NavLink struct {
	Title string `yaml:"title" json:"title"`
	URL   string `yaml:"url" json:"url"`
	// Set when URL leaves the site (has a scheme, or starts with /), so the
	// templates know not to put the relative root in front of it.
	Abs bool `yaml:"-" json:"-"`
}

// Settings are a site's own: from site.yaml in the source folder, then
// whatever the request overrides.
type Settings struct {
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
	Author      string    `yaml:"author"`
	BaseURL     string    `yaml:"baseURL"`
	Theme       string    `yaml:"theme"`
	Language    string    `yaml:"language"`
	Footer      string    `yaml:"footer"`
	Nav         []NavLink `yaml:"nav"`
	// How many entries the feed and the home page's recent list carry.
	FeedItems int `yaml:"feedItems"`
}

// Options say what to build.
type Options struct {
	// The source folder, absolute. Pages are named by their path under it.
	Dir string
	// Folders a linked picture or file may be copied from. A link elsewhere
	// (file:/etc/passwd) is not published. Empty means Dir alone.
	Roots []string
	// Override site.yaml; empty values leave it alone.
	Theme, Title, BaseURL string
	// Publish files marked #+DRAFT: t as well.
	Drafts bool
	// What "now" is, for the feed and the footer's year. Zero means now.
	Now time.Time
	// Reads a linked file to copy it; os.ReadFile when nil.
	ReadFile func(string) ([]byte, error)
}

// Result is the built site.
type Result struct {
	// Relative path (forward slashes) to contents.
	Files    map[string][]byte
	Warnings []string
	Pages    int
	Theme    string
}

// Page is one org file as the templates see it.
type Page struct {
	Src         string // notes/foo.org
	URL         string // notes/foo.html
	Title       string
	Subtitle    string
	Description string
	Author      string
	Date        time.Time
	HasDate     bool
	Tags        []TagRef
	Weight      int
	Draft       bool
	Folder      string // notes, "" at the top
	Summary     string
	Words       int
	ReadingMin  int
	TOC         []TocEntry
	Backlinks   []*Page
	IsIndex     bool // index.org of its folder

	doc      *org.Document
	abs      string
	body     string
	text     string
	math     bool
	mermaid  bool
	outLinks map[string]bool
	// Heading index → anchor, and the ways a link can name a heading.
	anchors map[int]string
	byTitle map[string]string
	// Headings carrying a tag, for the tag pages.
	tagged []taggedHeading
}

type taggedHeading struct {
	Tag, Title, ID string
}

// ISODate and DateText are the page's date for a machine and for a person.
func (p *Page) ISODate() string { return p.Date.Format("2006-01-02") }
func (p *Page) DateText() string {
	return p.Date.Format("January 2, 2006")
}
func (p *Page) ShortDate() string { return p.Date.Format("Jan 2, 2006") }
func (p *Page) Year() int         { return p.Date.Year() }

// TagRef is a tag on a page, with where its page is.
type TagRef struct {
	Name string
	URL  string
}

// TocEntry is one heading in a page's contents.
type TocEntry struct {
	Title string
	ID    string
	Level int
}

type builder struct {
	o        Options
	set      Settings
	pages    []*Page
	bySrc    map[string]*Page
	byAbs    map[string]*Page
	byURL    map[string]*Page
	ids      map[string]string // :ID: → url#anchor
	custom   map[string]string // CUSTOM_ID → url#anchor
	files    map[string][]byte
	assets   map[string]string // absolute source path → output path
	warnings []string
}

func (b *builder) warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	for _, w := range b.warnings {
		if w == msg {
			return
		}
	}
	b.warnings = append(b.warnings, msg)
}

// LoadSettings reads site.yaml (or site.yml) from the source folder, if it
// has one.
func LoadSettings(dir string, read func(string) ([]byte, error)) (Settings, error) {
	s := Settings{}
	for _, name := range []string{"site.yaml", "site.yml"} {
		data, err := read(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, &s); err != nil {
			return s, fmt.Errorf("%s: %v", name, err)
		}
		return s, nil
	}
	return s, nil
}

// Build makes the site.
func Build(sources []Source, o Options) (*Result, error) {
	if o.ReadFile == nil {
		o.ReadFile = os.ReadFile
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, err
	}
	o.Dir = dir
	if len(o.Roots) == 0 {
		o.Roots = []string{dir}
	}
	set, err := LoadSettings(dir, o.ReadFile)
	if err != nil {
		return nil, err
	}
	if o.Theme != "" {
		set.Theme = o.Theme
	}
	if o.Title != "" {
		set.Title = o.Title
	}
	if o.BaseURL != "" {
		set.BaseURL = o.BaseURL
	}
	if set.Theme == "" {
		set.Theme = "paper"
	}
	// Named by the yaml or the request, else by the home page (below), and
	// only failing both by its folder.
	titled := set.Title != ""
	if !titled {
		set.Title = humanize(filepath.Base(dir))
	}
	if set.Language == "" {
		set.Language = "en"
	}
	if set.FeedItems <= 0 {
		set.FeedItems = 20
	}
	set.BaseURL = strings.TrimRight(set.BaseURL, "/")
	th, err := loadTheme(set.Theme)
	if err != nil {
		return nil, err
	}
	b := &builder{o: o, set: set, bySrc: map[string]*Page{}, byAbs: map[string]*Page{}, byURL: map[string]*Page{},
		ids: map[string]string{}, custom: map[string]string{}, files: map[string][]byte{}, assets: map[string]string{}}

	// The pages, and every name a link can give a heading, before any page is
	// written - a link can point forward.
	for _, s := range sources {
		b.addPage(s)
	}
	if len(b.pages) == 0 {
		return nil, fmt.Errorf("no org files to publish under %s", dir)
	}
	if !titled {
		for _, p := range b.pages {
			if p.Src == "index.org" && strings.TrimSpace(p.doc.Get("TITLE")) != "" {
				b.set.Title = p.Title
			}
		}
	}
	sort.SliceStable(b.pages, func(i, j int) bool { return pageLess(b.pages[i], b.pages[j]) })
	for _, p := range b.pages {
		b.index(p)
	}
	for _, p := range b.pages {
		b.render(p)
	}
	// Backlinks: who links here, in page order.
	for _, from := range b.pages {
		for src := range from.outLinks {
			if to := b.bySrc[src]; to != nil && to != from {
				to.Backlinks = append(to.Backlinks, from)
			}
		}
	}
	for _, p := range b.pages {
		sort.SliceStable(p.Backlinks, func(i, j int) bool { return pageLess(p.Backlinks[i], p.Backlinks[j]) })
	}
	if err := b.writeSite(th); err != nil {
		return nil, err
	}
	return &Result{Files: b.files, Warnings: b.warnings, Pages: len(b.pages), Theme: th.Name}, nil
}

// Pages in reading order: a #+WEIGHT: first (smallest first), then by title.
func pageLess(a, b *Page) bool {
	wa, wb := a.Weight, b.Weight
	if wa == 0 {
		wa = 1 << 30
	}
	if wb == 0 {
		wb = 1 << 30
	}
	if wa != wb {
		return wa < wb
	}
	return strings.ToLower(a.Title) < strings.ToLower(b.Title)
}

var dateInText = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)

func parseDate(s string) (time.Time, bool) {
	m := dateInText.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("2006-01-02", m[0], time.Local)
	return t, err == nil
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "t", "true", "yes", "y", "1":
		return true
	}
	return false
}

// humanize makes a file or folder name a title: my-first_post → My first post.
func humanize(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.NewReplacer("-", " ", "_", " ").Replace(name)
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "Site"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func splitTags(s string) []string {
	out := []string{}
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ':' || r == ' ' || r == ',' || r == '\t' }) {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (b *builder) addPage(s Source) {
	if s.Doc == nil {
		return
	}
	abs, err := filepath.Abs(s.Path)
	if err != nil {
		return
	}
	rel, err := filepath.Rel(b.o.Dir, abs)
	if err != nil || strings.HasPrefix(rel, "..") || strings.ToLower(filepath.Ext(rel)) != ".org" {
		return
	}
	rel = filepath.ToSlash(rel)
	// A hidden folder (.git, .obsidian) or an archive file is not content.
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") || strings.HasPrefix(part, "_") {
			return
		}
	}
	d := s.Doc
	p := &Page{Src: rel, URL: strings.TrimSuffix(rel, path.Ext(rel)) + ".html", doc: d, abs: abs, outLinks: map[string]bool{}}
	p.Draft = truthy(d.Get("DRAFT"))
	if p.Draft && !b.o.Drafts {
		return
	}
	if strings.EqualFold(d.Get("SITE"), "nil") || strings.EqualFold(d.Get("SITE"), "no") {
		return
	}
	p.Title = strings.TrimSpace(d.Get("TITLE"))
	if p.Title == "" {
		p.Title = humanize(path.Base(rel))
	}
	p.Subtitle = d.Get("SUBTITLE")
	p.Description = d.Get("DESCRIPTION")
	p.Author = d.Get("AUTHOR")
	if p.Author == "" {
		p.Author = b.set.Author
	}
	p.Date, p.HasDate = parseDate(d.Get("DATE"))
	p.Weight, _ = strconv.Atoi(strings.TrimSpace(d.Get("WEIGHT")))
	if dir := path.Dir(rel); dir != "." {
		p.Folder = dir
	}
	p.IsIndex = path.Base(rel) == "index.org"
	for _, t := range splitTags(d.Get("FILETAGS")) {
		p.Tags = append(p.Tags, TagRef{Name: t, URL: tagURL(t)})
	}
	b.pages = append(b.pages, p)
	b.bySrc[rel] = p
	b.byAbs[abs] = p
	b.byURL[p.URL] = p
}

// tagURL is where a tag's page is. A tag called index would be the tags
// index itself, so it is moved aside.
func tagURL(tag string) string {
	s := slug(tag)
	if s == "index" {
		s = "index-tag"
	}
	return "tags/" + s + ".html"
}

// slug is a heading or tag as an html id: lower case, words joined by -.
func slug(s string) string {
	b := strings.Builder{}
	dash := false
	for _, r := range strings.ToLower(s) {
		if r == '-' || r == '_' || r == ' ' || r == '\t' || r == '/' || r == '.' {
			dash = b.Len() > 0
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			if dash {
				b.WriteByte('-')
				dash = false
			}
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}

// The text of a run of inline nodes, markup removed: what a heading is called
// in a contents list, a slug and a search.
func plain(nodes []org.Node) string {
	b := strings.Builder{}
	var walk func([]org.Node)
	walk = func(ns []org.Node) {
		for _, n := range ns {
			switch v := n.(type) {
			case org.Text:
				b.WriteString(v.Content)
			case org.Emphasis:
				walk(v.Content)
			case org.RegularLink:
				if v.Description != nil {
					walk(v.Description)
				} else {
					b.WriteString(v.URL)
				}
			case org.LatexFragment:
				walk(v.Content)
			case org.LineBreak, org.ExplicitLineBreak:
				b.WriteString(" ")
			case org.Paragraph:
				walk(v.Children)
				b.WriteString(" ")
			}
		}
	}
	walk(nodes)
	return strings.Join(strings.Fields(b.String()), " ")
}

func excluded(h *org.Headline, d *org.Document) bool {
	if h == nil {
		return false
	}
	if h.IsComment || h.IsExcluded(d) {
		return true
	}
	if h.IsArchived() && d.GetOption("arch") == "nil" {
		return true
	}
	return false
}

// footnoteSection is org's "* Footnotes" heading holding nothing but
// definitions: those are written at the end of the page, not as a section.
func footnoteSection(h *org.Headline) bool {
	if h == nil || !strings.EqualFold(plain(h.Title), "footnotes") {
		return false
	}
	for _, c := range h.Children {
		switch c.(type) {
		case org.FootnoteDefinition, *org.FootnoteDefinition:
		default:
			return false
		}
	}
	return true
}

// index gives every heading of a page its anchor and registers the ways a link
// may name it.
func (b *builder) index(p *Page) {
	p.anchors = map[int]string{}
	p.byTitle = map[string]string{}
	used := map[string]int{}
	var walk func(*org.Section)
	walk = func(sec *org.Section) {
		if sec == nil {
			return
		}
		// The outline's root carries an empty level 0 headline of its own.
		if h := sec.Headline; h != nil && h.Lvl > 0 {
			if excluded(h, p.doc) {
				return
			}
			if !footnoteSection(h) {
				title := plain(h.Title)
				id := ""
				if cid, ok := h.Properties.Get("CUSTOM_ID"); ok && strings.TrimSpace(cid) != "" {
					id = strings.TrimSpace(cid)
				} else {
					id = slug(title)
				}
				if n := used[id]; n > 0 {
					used[id] = n + 1
					id = fmt.Sprintf("%s-%d", id, n+1)
				} else {
					used[id] = 1
				}
				p.anchors[h.Index] = id
				for _, k := range []string{"=" + title, "~" + loose(title)} {
					if _, ok := p.byTitle[k]; !ok {
						p.byTitle[k] = id
					}
				}
				if cid, ok := h.Properties.Get("CUSTOM_ID"); ok && cid != "" {
					if _, ok := b.custom[cid]; !ok {
						b.custom[cid] = p.URL + "#" + id
					}
				}
				if hid, ok := h.Properties.Get("ID"); ok && hid != "" {
					b.ids[strings.TrimSpace(hid)] = p.URL + "#" + id
				}
				if h.Lvl <= 3 {
					p.TOC = append(p.TOC, TocEntry{Title: title, ID: id, Level: h.Lvl})
				}
				for _, t := range h.Tags {
					if t != "ARCHIVE" && t != "noexport" {
						p.tagged = append(p.tagged, taggedHeading{Tag: t, Title: title, ID: id})
					}
				}
			}
		}
		for _, c := range sec.Children {
			walk(c)
		}
	}
	if p.doc.Outline.Section != nil {
		walk(p.doc.Outline.Section)
	}
	// A page with one or two headings has nothing a contents list would help
	// with.
	if len(p.TOC) < 3 || strings.EqualFold(p.doc.GetOption("toc"), "nil") {
		p.TOC = nil
	}
	// A file-level :ID: names the page itself.
	for _, n := range p.doc.Nodes {
		var pd *org.PropertyDrawer
		switch v := n.(type) {
		case org.PropertyDrawer:
			pd = &v
		case *org.PropertyDrawer:
			pd = v
		}
		if id, ok := pd.Get("ID"); ok && id != "" {
			b.ids[strings.TrimSpace(id)] = p.URL
		}
	}
}

func loose(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func (p *Page) headingAnchor(title string) (string, bool) {
	if id, ok := p.byTitle["="+title]; ok {
		return id, true
	}
	id, ok := p.byTitle["~"+loose(title)]
	return id, ok
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// A tag that ends a block is a gap between words; one inside a line (a link,
// emphasis) is not, or "the <a>garden</a>." would read "the garden ."
var blockTagRe = regexp.MustCompile(`(?i)</?(p|div|section|li|ul|ol|h[1-6]|br|hr|tr|td|th|table|pre|blockquote|dt|dd|dl|figure|figcaption|sup)\b[^>]*>`)
var entityRe = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#34;", `"`, "&#39;", "'", "&#xa0;", " ", "&nbsp;", " ")

// textOf is what a page says, with its markup taken off: for the search index,
// a word count and a summary.
func textOf(html string) string {
	t := blockTagRe.ReplaceAllString(html, " ")
	t = tagRe.ReplaceAllString(t, "")
	t = entityRe.Replace(t)
	return strings.Join(strings.Fields(t), " ")
}

func summaryOf(text string, words int) string {
	f := strings.Fields(text)
	if len(f) <= words {
		return strings.Join(f, " ")
	}
	return strings.Join(f[:words], " ") + "…"
}

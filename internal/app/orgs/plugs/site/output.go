package site

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SiteData is what every template sees of the site as a whole.
type SiteData struct {
	Title       string
	Description string
	Author      string
	BaseURL     string
	Language    string
	Footer      string
	Nav         []NavLink
	Year        int
	Built       time.Time
	// Every page in reading order (index pages included), the dated ones
	// newest first, and the recent few of those.
	Pages  []*Page
	Posts  []*Page
	Recent []*Page
	Tags   []*TagInfo
	Tree   []*TreeNode
	// The home page is index.org rather than generated.
	HasIndex bool
}

// TagInfo is one tag and everything carrying it.
type TagInfo struct {
	Name    string
	URL     string
	Count   int
	Entries []TagEntry
	// 1 (rare) to 5 (the commonest), for a tag cloud.
	Weight int
}

// TagEntry is a page, or a heading on one, carrying a tag.
type TagEntry struct {
	Title     string
	URL       string
	PageTitle string // set for a heading
	Page      *Page
}

// TreeNode is a page or a folder in the navigation tree.
type TreeNode struct {
	Title    string
	URL      string // "" for a folder with no index.org
	Folder   bool
	Children []*TreeNode
}

// Group is a run of pages under a heading: a year, a folder.
type Group struct {
	Name  string
	Pages []*Page
}

// View is what one template execution sees.
type View struct {
	Site        *SiteData
	Theme       *Theme
	Kind        string // page, home, tags, tag, archive, 404
	Root        string
	Title       string
	Description string
	URL         string
	Page        *Page
	Body        template.HTML
	Tag         *TagInfo
	Years       []Group
	Folders     []Group
	Math        bool
	Mermaid     bool
	// Previous and next dated posts, for a post.
	Prev, Next *Page
}

var absURL = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.-]*:|/)`)

func (b *builder) siteData() *SiteData {
	s := &SiteData{
		Title: b.set.Title, Description: b.set.Description, Author: b.set.Author,
		BaseURL: b.set.BaseURL, Language: b.set.Language, Footer: b.set.Footer,
		Year: b.o.Now.Year(), Built: b.o.Now,
	}
	s.Pages = b.pages
	for _, p := range b.pages {
		if p.Src == "index.org" {
			s.HasIndex = true
		}
		if p.HasDate && !p.IsIndex {
			s.Posts = append(s.Posts, p)
		}
	}
	sort.SliceStable(s.Posts, func(i, j int) bool { return s.Posts[i].Date.After(s.Posts[j].Date) })
	s.Recent = s.Posts
	if len(s.Recent) > b.set.FeedItems {
		s.Recent = s.Recent[:b.set.FeedItems]
	}
	s.Nav = b.nav()
	s.Tags = b.tags()
	s.Tree = b.tree()
	return s
}

// The navigation bar: site.yaml's, else every top level page that gave
// itself a #+WEIGHT:, then the archive and the tags.
func (b *builder) nav() []NavLink {
	out := []NavLink{}
	if len(b.set.Nav) > 0 {
		for _, n := range b.set.Nav {
			n.Abs = absURL.MatchString(n.URL)
			n.URL = strings.TrimPrefix(n.URL, "./")
			out = append(out, n)
		}
		return out
	}
	for _, p := range b.pages {
		if p.Weight != 0 && p.Folder == "" && !p.IsIndex {
			out = append(out, NavLink{Title: p.Title, URL: p.URL})
		}
	}
	out = append(out, NavLink{Title: "Archive", URL: "archive.html"})
	if len(b.tags()) > 0 {
		out = append(out, NavLink{Title: "Tags", URL: "tags/index.html"})
	}
	return out
}

func (b *builder) tags() []*TagInfo {
	byKey := map[string]*TagInfo{}
	add := func(name string, e TagEntry) {
		key := strings.ToLower(name)
		t := byKey[key]
		if t == nil {
			t = &TagInfo{Name: name, URL: tagURL(name)}
			byKey[key] = t
		}
		for _, have := range t.Entries {
			if have.URL == e.URL {
				return
			}
		}
		t.Entries = append(t.Entries, e)
	}
	for _, p := range b.pages {
		for _, t := range p.Tags {
			add(t.Name, TagEntry{Title: p.Title, URL: p.URL, Page: p})
		}
		for _, t := range p.tagged {
			add(t.Tag, TagEntry{Title: t.Title, URL: p.URL + "#" + t.ID, PageTitle: p.Title, Page: p})
		}
	}
	out := []*TagInfo{}
	most := 1
	for _, t := range byKey {
		t.Count = len(t.Entries)
		if t.Count > most {
			most = t.Count
		}
		out = append(out, t)
	}
	for _, t := range out {
		t.Weight = 1 + (t.Count-1)*4/maxInt(most-1, 1)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// tree is the pages by folder: a folder's index.org is the folder's own link,
// pages before folders, each in reading order.
func (b *builder) tree() []*TreeNode {
	root := &TreeNode{Folder: true}
	folders := map[string]*TreeNode{"": root}
	var folder func(string) *TreeNode
	folder = func(dir string) *TreeNode {
		if f, ok := folders[dir]; ok {
			return f
		}
		parent := folder(strings.TrimSuffix(path.Dir(dir), "."))
		if path.Dir(dir) == "." {
			parent = root
		}
		f := &TreeNode{Title: humanize(path.Base(dir)), Folder: true}
		parent.Children = append(parent.Children, f)
		folders[dir] = f
		return f
	}
	for _, p := range b.pages {
		if p.IsIndex {
			f := folder(p.Folder)
			if f != root {
				f.URL = p.URL
				if t := p.doc.Get("TITLE"); t != "" {
					f.Title = t
				}
			}
			continue
		}
		f := folder(p.Folder)
		f.Children = append(f.Children, &TreeNode{Title: p.Title, URL: p.URL})
	}
	var order func(*TreeNode)
	order = func(n *TreeNode) {
		sort.SliceStable(n.Children, func(i, j int) bool {
			return !n.Children[i].Folder && n.Children[j].Folder
		})
		for _, c := range n.Children {
			order(c)
		}
	}
	order(root)
	return root.Children
}

func (b *builder) put(name string, data []byte) {
	if _, ok := b.files[name]; ok {
		b.warn("%s is both a page of the site and one it makes; the page was kept", name)
		return
	}
	b.files[name] = data
}

func (b *builder) writeSite(th *Theme) error {
	site := b.siteData()
	cache := map[string]*template.Template{}
	render := func(v *View) ([]byte, error) {
		t, ok := cache[v.Kind]
		if !ok {
			var err error
			if t, err = th.templates(v.Kind); err != nil {
				return nil, err
			}
			cache[v.Kind] = t
		}
		v.Site, v.Theme = site, th
		if v.Description == "" {
			v.Description = site.Description
		}
		buf := bytes.Buffer{}
		if err := t.ExecuteTemplate(&buf, "layout", v); err != nil {
			return nil, fmt.Errorf("theme %s, %s page: %v", th.Name, v.Kind, err)
		}
		return buf.Bytes(), nil
	}

	// Pages, with the dated ones knowing their neighbours.
	pos := map[*Page]int{}
	for i, p := range site.Posts {
		pos[p] = i
	}
	for _, p := range b.pages {
		v := &View{Kind: "page", Root: p.root(), Title: p.Title, Description: p.Summary, URL: p.URL,
			Page: p, Body: template.HTML(p.body), Math: p.math, Mermaid: p.mermaid}
		if p.Src == "index.org" {
			v.Kind = "home"
			v.Title = ""
		}
		if i, ok := pos[p]; ok {
			if i > 0 {
				v.Next = site.Posts[i-1]
			}
			if i+1 < len(site.Posts) {
				v.Prev = site.Posts[i+1]
			}
		}
		out, err := render(v)
		if err != nil {
			return err
		}
		b.files[p.URL] = out
	}
	gen := func(name string, v *View) error {
		v.URL = name
		v.Root = strings.Repeat("../", strings.Count(name, "/"))
		out, err := render(v)
		if err != nil {
			return err
		}
		b.put(name, out)
		return nil
	}
	if !site.HasIndex {
		if err := gen("index.html", &View{Kind: "home"}); err != nil {
			return err
		}
	}
	if len(site.Tags) > 0 {
		if err := gen("tags/index.html", &View{Kind: "tags", Title: "Tags"}); err != nil {
			return err
		}
		for _, t := range site.Tags {
			if err := gen(t.URL, &View{Kind: "tag", Title: "#" + t.Name, Tag: t}); err != nil {
				return err
			}
		}
	}
	if err := gen("archive.html", &View{Kind: "archive", Title: "Archive", Years: years(site.Posts), Folders: folderGroups(b.pages)}); err != nil {
		return err
	}
	// The 404 page is served for urls at any depth, so its links cannot be
	// relative: they are absolute when the site has a base url, and from the
	// root otherwise.
	nf := &View{Kind: "404", Title: "Not found", URL: "404.html"}
	if site.BaseURL != "" {
		nf.Root = site.BaseURL + "/"
	} else {
		nf.Root = "/"
	}
	if out, err := render(nf); err == nil {
		b.put("404.html", out)
	} else {
		return err
	}

	b.put("feed.xml", b.feed(site))
	if site.BaseURL != "" {
		b.put("sitemap.xml", b.sitemap(site))
		b.put("robots.txt", []byte("User-agent: *\nAllow: /\nSitemap: "+site.BaseURL+"/sitemap.xml\n"))
	}
	idx := b.searchIndex()
	b.put("search.json", idx)
	b.put("assets/search.js", append(append([]byte("window.SITE_SEARCH="), idx...), ';', '\n'))

	css := strings.Builder{}
	if data, ok := readTemplateFile("site/base/base.css"); ok {
		css.Write(data)
		css.WriteString("\n")
	}
	if data, ok := th.ownFile("style.css"); ok {
		css.WriteString("/* theme: " + th.Name + " */\n")
		css.Write(data)
		css.WriteString("\n")
	}
	css.WriteString(th.highlightCSS())
	b.put("assets/site.css", []byte(css.String()))

	js := strings.Builder{}
	if data, ok := readTemplateFile("site/base/site.js"); ok {
		js.Write(data)
		js.WriteString("\n")
	}
	if data, ok := th.ownFile("theme.js"); ok {
		js.Write(data)
		js.WriteString("\n")
	}
	b.put("assets/site.js", []byte(js.String()))
	return nil
}

func years(posts []*Page) []Group {
	out := []Group{}
	for _, p := range posts {
		y := fmt.Sprint(p.Date.Year())
		if len(out) == 0 || out[len(out)-1].Name != y {
			out = append(out, Group{Name: y})
		}
		out[len(out)-1].Pages = append(out[len(out)-1].Pages, p)
	}
	return out
}

func folderGroups(pages []*Page) []Group {
	by := map[string][]*Page{}
	names := []string{}
	for _, p := range pages {
		if _, ok := by[p.Folder]; !ok {
			names = append(names, p.Folder)
		}
		by[p.Folder] = append(by[p.Folder], p)
	}
	sort.Strings(names)
	out := []Group{}
	for _, n := range names {
		title := humanize(n)
		if n == "" {
			title = "Pages"
		} else {
			title = n
		}
		out = append(out, Group{Name: title, Pages: by[n]})
	}
	return out
}

// ---------------------------------------------------------------------------
// The feed, the sitemap and the search index
// ---------------------------------------------------------------------------

func xmlText(s string) string {
	b := bytes.Buffer{}
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (b *builder) abs(site *SiteData, rel string) string {
	if site.BaseURL == "" {
		return rel
	}
	return site.BaseURL + "/" + rel
}

func (b *builder) feed(site *SiteData) []byte {
	out := strings.Builder{}
	out.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	out.WriteString(`<feed xmlns="http://www.w3.org/2005/Atom">` + "\n")
	out.WriteString("  <title>" + xmlText(site.Title) + "</title>\n")
	if site.Description != "" {
		out.WriteString("  <subtitle>" + xmlText(site.Description) + "</subtitle>\n")
	}
	out.WriteString(`  <link href="` + xmlText(b.abs(site, "feed.xml")) + `" rel="self"/>` + "\n")
	out.WriteString(`  <link href="` + xmlText(b.abs(site, "index.html")) + `"/>` + "\n")
	id := site.BaseURL + "/"
	if site.BaseURL == "" {
		id = "urn:orgs-site:" + slug(site.Title)
	}
	out.WriteString("  <id>" + xmlText(id) + "</id>\n")
	updated := site.Built
	if len(site.Recent) > 0 {
		updated = site.Recent[0].Date
	}
	out.WriteString("  <updated>" + updated.Format(time.RFC3339) + "</updated>\n")
	if site.Author != "" {
		out.WriteString("  <author><name>" + xmlText(site.Author) + "</name></author>\n")
	}
	out.WriteString("  <generator>orgs</generator>\n")
	for _, p := range site.Recent {
		link := b.abs(site, p.URL)
		eid := link
		if site.BaseURL == "" {
			eid = "urn:orgs-site:" + slug(site.Title) + ":" + p.URL
		}
		out.WriteString("  <entry>\n")
		out.WriteString("    <title>" + xmlText(p.Title) + "</title>\n")
		out.WriteString(`    <link href="` + xmlText(link) + `"/>` + "\n")
		out.WriteString("    <id>" + xmlText(eid) + "</id>\n")
		out.WriteString("    <updated>" + p.Date.Format(time.RFC3339) + "</updated>\n")
		if p.Author != "" {
			out.WriteString("    <author><name>" + xmlText(p.Author) + "</name></author>\n")
		}
		for _, t := range p.Tags {
			out.WriteString(`    <category term="` + xmlText(t.Name) + `"/>` + "\n")
		}
		out.WriteString("    <summary>" + xmlText(p.Summary) + "</summary>\n")
		out.WriteString(`    <content type="html">` + xmlText(p.body) + "</content>\n")
		out.WriteString("  </entry>\n")
	}
	out.WriteString("</feed>\n")
	return []byte(out.String())
}

func (b *builder) sitemap(site *SiteData) []byte {
	out := strings.Builder{}
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	out.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	urls := []string{"index.html"}
	for _, p := range b.pages {
		if p.URL != "index.html" {
			urls = append(urls, p.URL)
		}
	}
	urls = append(urls, "archive.html")
	for _, u := range urls {
		out.WriteString("  <url><loc>" + xmlText(site.BaseURL+"/"+u) + "</loc></url>\n")
	}
	out.WriteString("</urlset>\n")
	return []byte(out.String())
}

type searchEntry struct {
	T string   `json:"t"`           // title
	U string   `json:"u"`           // url from the root
	P string   `json:"p,omitempty"` // the page a heading is on
	G []string `json:"g,omitempty"` // tags
	X string   `json:"x,omitempty"` // text
	D string   `json:"d,omitempty"` // date
}

// searchIndex is every page with its text, and every heading in a contents
// list as an entry of its own, so a search lands on the section.
func (b *builder) searchIndex() []byte {
	entries := []searchEntry{}
	for _, p := range b.pages {
		e := searchEntry{T: p.Title, U: p.URL}
		for _, t := range p.Tags {
			e.G = append(e.G, t.Name)
		}
		text := p.text
		if len(text) > 6000 {
			text = text[:6000]
		}
		e.X = text
		if p.HasDate {
			e.D = p.ShortDate()
		}
		entries = append(entries, e)
		for _, h := range p.TOC {
			entries = append(entries, searchEntry{T: h.Title, U: p.URL + "#" + h.ID, P: p.Title})
		}
	}
	data, _ := json.Marshal(entries)
	return data
}

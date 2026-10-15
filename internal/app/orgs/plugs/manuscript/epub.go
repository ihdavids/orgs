package manuscript

// The book as an EPUB 3 ebook: a zip with the mimetype stored first, an OPF
// package, a navigation document (and an NCX for readers that predate EPUB 3),
// a title page, a page a chapter, and footnotes as asides that Apple Books,
// Kobo and most others show as pop-ups. Written by hand for the same reasons
// as the docx: no pandoc, and nothing in it a strict reader would refuse.

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/assets"
)

// EpubThemes are the stylesheets in epub/themes on the template path.
func EpubThemes(dirs ...string) []string {
	out := []string{}
	for _, n := range assets.GlobTemplates("epub/themes/*.css", dirs...) {
		out = append(out, strings.TrimSuffix(filepath.Base(n), ".css"))
	}
	return out
}

// EpubThemeCSS is a theme's stylesheet.
func EpubThemeCSS(name string, dirs ...string) (string, error) {
	if name == "" {
		name = "classic"
	}
	p := assets.FindTemplate("epub/themes/"+filepath.Base(name)+".css", dirs...)
	if p == "" {
		return "", fmt.Errorf("no epub theme %q (themes: %s)", name, strings.Join(EpubThemes(dirs...), ", "))
	}
	b, err := os.ReadFile(p)
	return string(b), err
}

type epub struct {
	book   *Book
	images map[string]string // source -> name in the book
	media  []epubItem
	notes  map[string]int // footnote name -> its number
	order  []string
	warn   []string
}

type epubItem struct {
	id, href, typ, props string
	data                 []byte
}

// Identifier is the book's: its ISBN or identifier as given, else a uuid made
// from its title and author, so the same book exported twice is the same book
// to a reader's library rather than a second copy.
func (e *epub) identifier() string {
	m := e.book.Meta
	if m.Identifier != "" {
		id := m.Identifier
		if !strings.Contains(id, ":") {
			id = "urn:isbn:" + strings.ReplaceAll(id, "-", "")
		}
		return id
	}
	h := sha1.Sum([]byte("orgs-epub|" + m.Title + "|" + m.Name()))
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// WriteEpub writes the book with a theme's stylesheet (and the file's own
// #+EPUB_CSS after it). Warnings are pictures that could not be put in.
func WriteEpub(b *Book, themeCSS string, now time.Time) ([]byte, []string, error) {
	e := &epub{book: b, images: map[string]string{}, notes: map[string]int{}}
	if b.Meta.SceneMark == "" {
		b.Meta.SceneMark = "⁂"
	}
	css := themeCSS
	if b.Meta.EpubCSS != "" {
		if extra, err := os.ReadFile(b.Meta.EpubCSS); err == nil {
			css += "\n/* " + filepath.Base(b.Meta.EpubCSS) + " */\n" + string(extra)
		} else {
			e.warn = append(e.warn, fmt.Sprintf("EPUB_CSS: %v", err))
		}
	}

	type page struct {
		id, href, title string
		kind            string
		html            string
		chapter         *Chapter
	}
	pages := []page{}

	cover := ""
	if b.Meta.Cover != "" {
		cover = e.image(b.Meta.Cover)
		if cover != "" {
			pages = append(pages, page{id: "cover", href: "cover.xhtml", title: "Cover", kind: "cover",
				html: e.page("Cover", "cover", `<section epub:type="cover" class="cover"><img src="`+xa(cover)+`" alt="`+xa(b.Meta.Title)+`"/></section>`)})
			for i := range e.media {
				if e.media[i].href == cover {
					e.media[i].props = "cover-image"
				}
			}
		}
	}
	pages = append(pages, page{id: "titlepage", href: "title.xhtml", title: "Title Page", kind: "titlepage", html: e.titlePage()})
	if b.Meta.Dedication != "" {
		pages = append(pages, page{id: "dedication", href: "dedication.xhtml", title: "Dedication", kind: "dedication",
			html: e.page("Dedication", "dedication", `<section epub:type="dedication" class="dedication"><p>`+xt(b.Meta.Dedication)+`</p></section>`)})
	}
	for i, c := range b.Chapters {
		id := fmt.Sprintf("ch%03d", i+1)
		title := PlainTitle(c.Title)
		if c.Label != "" {
			if title != "" {
				title = c.Label + ": " + title
			} else {
				title = c.Label
			}
		}
		if title == "" {
			title = b.Meta.Title
		}
		pages = append(pages, page{id: id, href: id + ".xhtml", title: title, kind: c.Kind, chapter: c})
	}
	for i := range pages {
		if pages[i].chapter != nil {
			pages[i].html = e.chapter(pages[i].chapter, pages[i].title)
		}
	}

	// The navigation: chapters under their parts.
	var nav strings.Builder
	nav.WriteString("<ol>")
	inPart := false
	for _, p := range pages {
		if p.chapter == nil {
			continue
		}
		if p.kind == "part" {
			if inPart {
				nav.WriteString("</ol></li>")
			}
			fmt.Fprintf(&nav, `<li><a href="%s">%s</a><ol>`, p.href, xt(p.title))
			inPart = true
			continue
		}
		fmt.Fprintf(&nav, `<li><a href="%s">%s</a></li>`, p.href, xt(p.title))
	}
	if inPart {
		nav.WriteString("</ol></li>")
	}
	nav.WriteString("</ol>")
	navOL := strings.ReplaceAll(nav.String(), "<ol></ol>", "")
	start := ""
	for _, p := range pages {
		if p.chapter != nil {
			start = p.href
			break
		}
	}
	landmarks := `<nav epub:type="landmarks" hidden="hidden"><ol>`
	if cover != "" {
		landmarks += `<li><a epub:type="cover" href="cover.xhtml">Cover</a></li>`
	}
	landmarks += `<li><a epub:type="titlepage" href="title.xhtml">Title Page</a></li><li><a epub:type="toc" href="nav.xhtml">Contents</a></li>`
	if start != "" {
		landmarks += `<li><a epub:type="bodymatter" href="` + start + `">Start</a></li>`
	}
	landmarks += `</ol></nav>`
	navDoc := e.page("Contents", "toc", `<nav epub:type="toc" id="toc" class="toc"><h1>Contents</h1>`+navOL+`</nav>`+landmarks)

	// The package.
	id := e.identifier()
	modified := now.UTC().Format("2006-01-02T15:04:05Z")
	m := b.Meta
	var opf strings.Builder
	opf.WriteString(xmlHead)
	opf.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid" xml:lang="` + xa(m.Language) + `">` + "\n")
	opf.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
	opf.WriteString(`<dc:identifier id="bookid">` + xt(id) + "</dc:identifier>\n")
	opf.WriteString(`<dc:title id="title">` + xt(m.Title) + "</dc:title>\n")
	if m.Subtitle != "" {
		opf.WriteString(`<dc:title id="subtitle">` + xt(m.Subtitle) + "</dc:title>\n")
		opf.WriteString(`<meta refines="#subtitle" property="title-type">subtitle</meta>` + "\n")
		opf.WriteString(`<meta refines="#title" property="title-type">main</meta>` + "\n")
	}
	if n := m.Name(); n != "" {
		opf.WriteString(`<dc:creator id="creator">` + xt(n) + "</dc:creator>\n")
		opf.WriteString(`<meta refines="#creator" property="role" scheme="marc:relators">aut</meta>` + "\n")
		if f := strings.Fields(n); len(f) > 1 {
			opf.WriteString(`<meta refines="#creator" property="file-as">` + xt(f[len(f)-1]+", "+strings.Join(f[:len(f)-1], " ")) + "</meta>\n")
		}
	}
	opf.WriteString(`<dc:language>` + xt(m.Language) + "</dc:language>\n")
	for _, kv := range [][2]string{{"description", m.Description}, {"publisher", m.Publisher}, {"rights", m.Rights}, {"date", isoDate(m.Date)}} {
		if kv[1] != "" {
			opf.WriteString(`<dc:` + kv[0] + `>` + xt(kv[1]) + `</dc:` + kv[0] + ">\n")
		}
	}
	for _, s := range m.Subjects {
		opf.WriteString(`<dc:subject>` + xt(s) + "</dc:subject>\n")
	}
	if m.Series != "" {
		opf.WriteString(`<meta property="belongs-to-collection" id="series">` + xt(m.Series) + "</meta>\n")
		opf.WriteString(`<meta refines="#series" property="collection-type">series</meta>` + "\n")
		// Calibre's spelling, which Kindle conversions and many libraries read.
		opf.WriteString(`<meta name="calibre:series" content="` + xa(m.Series) + `"/>` + "\n")
		if m.SeriesIndex != "" {
			opf.WriteString(`<meta refines="#series" property="group-position">` + xt(m.SeriesIndex) + "</meta>\n")
			opf.WriteString(`<meta name="calibre:series_index" content="` + xa(m.SeriesIndex) + `"/>` + "\n")
		}
	}
	opf.WriteString(`<meta property="dcterms:modified">` + modified + "</meta>\n")
	if cover != "" {
		opf.WriteString(`<meta name="cover" content="` + coverID(e.media, cover) + `"/>` + "\n")
	}
	opf.WriteString("</metadata>\n<manifest>\n")
	opf.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
	opf.WriteString(`<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>` + "\n")
	opf.WriteString(`<item id="css" href="style.css" media-type="text/css"/>` + "\n")
	for _, p := range pages {
		props := ""
		if strings.Contains(p.html, "<svg") {
			props = ` properties="svg"`
		}
		fmt.Fprintf(&opf, `<item id="%s" href="%s" media-type="application/xhtml+xml"%s/>`+"\n", p.id, p.href, props)
	}
	for _, it := range e.media {
		props := ""
		if it.props != "" {
			props = ` properties="` + it.props + `"`
		}
		fmt.Fprintf(&opf, `<item id="%s" href="%s" media-type="%s"%s/>`+"\n", it.id, it.href, it.typ, props)
	}
	opf.WriteString("</manifest>\n<spine toc=\"ncx\">\n")
	for _, p := range pages {
		linear := ""
		if p.kind == "cover" {
			linear = ` linear="no"`
		}
		fmt.Fprintf(&opf, `<itemref idref="%s"%s/>`+"\n", p.id, linear)
	}
	opf.WriteString(`<itemref idref="nav" linear="no"/>` + "\n")
	opf.WriteString("</spine>\n</package>\n")

	// The NCX, for readers older than EPUB 3.
	var ncx strings.Builder
	ncx.WriteString(xmlHead + `<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1"><head>` +
		`<meta name="dtb:uid" content="` + xa(id) + `"/><meta name="dtb:depth" content="1"/><meta name="dtb:totalPageCount" content="0"/><meta name="dtb:maxPageNumber" content="0"/>` +
		`</head><docTitle><text>` + xt(m.Title) + `</text></docTitle><navMap>`)
	n := 0
	for _, p := range pages {
		if p.chapter == nil {
			continue
		}
		n++
		fmt.Fprintf(&ncx, `<navPoint id="np%d" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="%s"/></navPoint>`, n, n, xt(p.title), p.href)
	}
	ncx.WriteString("</navMap></ncx>")

	// The zip. The mimetype is first and stored, which is how a reader knows
	// what it has been handed before it unpacks anything.
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	mt, err := z.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, nil, err
	}
	io.WriteString(mt, "application/epub+zip")
	files := []struct {
		name string
		data []byte
	}{
		{"META-INF/container.xml", []byte(containerXML)},
		{"OEBPS/content.opf", []byte(opf.String())},
		{"OEBPS/nav.xhtml", []byte(navDoc)},
		{"OEBPS/toc.ncx", []byte(ncx.String())},
		{"OEBPS/style.css", []byte(css)},
	}
	for _, p := range pages {
		files = append(files, struct {
			name string
			data []byte
		}{"OEBPS/" + p.href, []byte(p.html)})
	}
	for _, it := range e.media {
		files = append(files, struct {
			name string
			data []byte
		}{"OEBPS/" + it.href, it.data})
	}
	for _, f := range files {
		w, err := z.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, nil, err
	}
	return out.Bytes(), e.warn, nil
}

func coverID(items []epubItem, href string) string {
	for _, it := range items {
		if it.href == href {
			return it.id
		}
	}
	return ""
}

func isoDate(s string) string {
	s = strings.Trim(strings.TrimSpace(s), "<>[]")
	if len(s) >= 10 {
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return s[:10]
		}
	}
	if len(s) == 4 {
		if _, err := strconv.Atoi(s); err == nil {
			return s
		}
	}
	return ""
}

// xt escapes text, xa an attribute's value.
func xt(s string) string { return x(s) }
func xa(s string) string { return strings.ReplaceAll(x(s), "'", "&#39;") }

func (e *epub) page(title, kind, body string) string {
	return xmlHead + `<!DOCTYPE html>` + "\n" +
		`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="` + xa(e.book.Meta.Language) + `" lang="` + xa(e.book.Meta.Language) + `">` + "\n" +
		`<head><meta charset="utf-8"/><title>` + xt(title) + `</title><link rel="stylesheet" type="text/css" href="style.css"/></head>` + "\n" +
		`<body class="` + kind + `">` + "\n" + body + "\n</body></html>\n"
}

func (e *epub) titlePage() string {
	m := e.book.Meta
	var b strings.Builder
	b.WriteString(`<section epub:type="titlepage" class="titlepage">`)
	b.WriteString(`<h1 class="title">` + xt(m.Title) + `</h1>`)
	if m.Subtitle != "" {
		b.WriteString(`<p class="subtitle">` + xt(m.Subtitle) + `</p>`)
	}
	if n := m.Name(); n != "" {
		b.WriteString(`<p class="author">` + xt(n) + `</p>`)
	}
	if m.Publisher != "" {
		b.WriteString(`<p class="publisher">` + xt(m.Publisher) + `</p>`)
	}
	if m.Rights != "" {
		b.WriteString(`<p class="rights">` + xt(m.Rights) + `</p>`)
	}
	b.WriteString(`</section>`)
	return e.page(m.Title, "titlepage", b.String())
}

func (e *epub) chapter(c *Chapter, title string) string {
	var b strings.Builder
	typ := "chapter"
	switch c.Kind {
	case "part":
		typ = "part"
	case "front":
		typ = "frontmatter"
	case "back":
		typ = "backmatter"
	}
	fmt.Fprintf(&b, `<section epub:type="%s" class="%s">`+"\n", typ, c.Kind)
	if c.Label != "" || len(c.Title) > 0 {
		b.WriteString(`<header class="chapter-head">`)
		if c.Label != "" {
			b.WriteString(`<p class="chapter-label">` + xt(c.Label) + `</p>`)
		}
		if len(c.Title) > 0 {
			b.WriteString(`<h1 class="chapter-title">` + e.runs(c.Title) + `</h1>`)
		} else {
			b.WriteString(`<h1 class="chapter-title label-only">` + xt(c.Label) + `</h1>`)
		}
		b.WriteString("</header>\n")
	}
	before := len(e.order)
	for _, bl := range c.Blocks {
		b.WriteString(e.block(bl))
	}
	// This chapter's footnotes, at its end: a pop-up where the reader can,
	// a list where it cannot.
	if len(e.order) > before {
		b.WriteString(`<section class="footnotes" epub:type="footnotes" role="doc-endnotes"><hr/>` + "\n")
		for _, name := range e.order[before:] {
			n := e.notes[name]
			fmt.Fprintf(&b, `<aside epub:type="footnote" role="doc-footnote" id="fn%d" class="footnote"><p><a class="fn-back" href="#fnref%d">%d.</a> `, n, n, n)
			for i, nb := range e.book.Notes[name] {
				if i > 0 {
					b.WriteString("</p><p>")
				}
				b.WriteString(e.runs(nb.Runs))
			}
			b.WriteString("</p></aside>\n")
		}
		b.WriteString("</section>\n")
	}
	b.WriteString("</section>")
	return e.page(title, c.Kind, b.String())
}

func (e *epub) block(b Block) string {
	switch b.Kind {
	case "p":
		if b.First {
			return `<p class="first">` + e.runs(b.Runs) + "</p>\n"
		}
		return "<p>" + e.runs(b.Runs) + "</p>\n"
	case "scene":
		return `<p class="scene-break" role="separator" aria-label="scene break">` + xt(e.book.Meta.SceneMark) + "</p>\n"
	case "heading":
		h := min(b.Level+1, 6)
		return fmt.Sprintf("<h%d>%s</h%d>\n", h, e.runs(b.Runs), h)
	case "quote":
		return "<blockquote>\n" + e.blocks(b.Children) + "</blockquote>\n"
	case "center":
		return `<div class="center">` + "\n" + e.blocks(b.Children) + "</div>\n"
	case "verse":
		return `<div class="verse"><p>` + e.runs(b.Runs) + "</p></div>\n"
	case "pre":
		return "<pre>" + xt(plain(b.Runs)) + "</pre>\n"
	case "ul", "ol":
		var s strings.Builder
		s.WriteString("<" + b.Kind + ">")
		for _, it := range b.Items {
			s.WriteString("<li>" + e.blocks(it) + "</li>")
		}
		s.WriteString("</" + b.Kind + ">\n")
		return s.String()
	case "dl":
		var s strings.Builder
		s.WriteString("<dl>")
		for i, it := range b.Items {
			if i < len(b.Terms) {
				s.WriteString("<dt>" + e.runs(b.Terms[i]) + "</dt>")
			}
			s.WriteString("<dd>" + e.blocks(it) + "</dd>")
		}
		s.WriteString("</dl>\n")
		return s.String()
	case "table":
		var s strings.Builder
		s.WriteString("<table>")
		for i, row := range b.Rows {
			if i == 0 && b.HeadRows > 0 {
				s.WriteString("<thead>")
			}
			if i == b.HeadRows {
				s.WriteString("<tbody>")
			}
			s.WriteString("<tr>")
			cell := "td"
			if i < b.HeadRows {
				cell = "th"
			}
			for _, c := range row {
				s.WriteString("<" + cell + ">" + e.runs(c) + "</" + cell + ">")
			}
			s.WriteString("</tr>")
			if i == b.HeadRows-1 {
				s.WriteString("</thead>")
			}
		}
		s.WriteString("</tbody></table>\n")
		return s.String()
	case "image":
		src := e.image(b.Src)
		if src == "" {
			return ""
		}
		alt := plain(b.Caption)
		s := `<figure><img src="` + xa(src) + `" alt="` + xa(alt) + `"/>`
		if len(b.Caption) > 0 {
			s += "<figcaption>" + e.runs(b.Caption) + "</figcaption>"
		}
		return s + "</figure>\n"
	}
	return ""
}

func (e *epub) blocks(bs []Block) string {
	var s strings.Builder
	for _, b := range bs {
		s.WriteString(e.block(b))
	}
	return s.String()
}

func (e *epub) runs(rs []Run) string {
	var s strings.Builder
	for _, r := range rs {
		if r.Break {
			s.WriteString("<br/>")
			continue
		}
		if r.Note != "" {
			if _, ok := e.book.Notes[r.Note]; !ok {
				continue
			}
			n, ok := e.notes[r.Note]
			if !ok {
				n = len(e.notes) + 1
				e.notes[r.Note] = n
				e.order = append(e.order, r.Note)
			}
			fmt.Fprintf(&s, `<a epub:type="noteref" role="doc-noteref" id="fnref%d" href="#fn%d" class="noteref">%d</a>`, n, n, n)
			continue
		}
		t := xt(r.Text)
		wrap := func(open, close string) { t = open + t + close }
		if r.Code {
			wrap("<code>", "</code>")
		}
		if r.Italic {
			wrap("<em>", "</em>")
		}
		if r.Bold {
			wrap("<strong>", "</strong>")
		}
		if r.Underline {
			wrap(`<span class="underline">`, "</span>")
		}
		if r.Strike {
			wrap("<del>", "</del>")
		}
		if r.Sup {
			wrap("<sup>", "</sup>")
		} else if r.Sub {
			wrap("<sub>", "</sub>")
		}
		if r.Link != "" {
			wrap(`<a href="`+xa(r.Link)+`">`, "</a>")
		}
		s.WriteString(t)
	}
	return s.String()
}

var epubImageTypes = map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".gif": "image/gif", ".svg": "image/svg+xml", ".webp": "image/webp"}

// image puts a picture in the book once and answers with its name there. An
// ebook has to carry its pictures, so a url or a missing file is left out and
// said.
func (e *epub) image(src string) string {
	if name, ok := e.images[src]; ok {
		return name
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		e.warn = append(e.warn, "left out a picture from the web (an epub carries its own): "+src)
		e.images[src] = ""
		return ""
	}
	data, err := os.ReadFile(src)
	if err != nil {
		e.warn = append(e.warn, fmt.Sprintf("left out a picture: %v", err))
		e.images[src] = ""
		return ""
	}
	ext := strings.ToLower(filepath.Ext(src))
	typ := epubImageTypes[ext]
	if typ == "" {
		typ = mime.TypeByExtension(ext)
	}
	if typ == "" {
		typ = http.DetectContentType(data)
	}
	n := len(e.media) + 1
	item := epubItem{id: fmt.Sprintf("img%d", n), href: fmt.Sprintf("images/img%d%s", n, ext), typ: typ, data: data}
	e.media = append(e.media, item)
	e.images[src] = item.href
	return item.href
}

const containerXML = xmlHead + `<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">` +
	`<rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`

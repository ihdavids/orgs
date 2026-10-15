package site

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/ihdavids/go-org/org"
)

// pageWriter writes one org file as the body of a page.
//
// It is go-org's html writer with the parts a site needs done differently:
// headings with readable, stable ids (the html exporter's are a fresh uuid per
// export, which a site cannot link to); links resolved across pages and to
// files copied into the site; code coloured here rather than by a script.
type pageWriter struct {
	*org.HTMLWriter
	b    *builder
	page *Page
}

func newPageWriter(b *builder, p *Page) *pageWriter {
	w := &pageWriter{HTMLWriter: org.NewHTMLWriter(), b: b, page: p}
	w.ExtendingWriter = w
	w.NoWrapCodeBlock = true
	w.HighlightCodeBlock = w.highlight
	// Not Before(): that writes the title and a contents list, which are the
	// layout's to place.
	w.Document = p.doc
	return w
}

func (b *builder) render(p *Page) {
	w := newPageWriter(b, p)
	org.WriteNodes(w, p.doc.Nodes...)
	w.WriteFootnotes(p.doc)
	p.body = w.String()
	p.text = textOf(anchorRe.ReplaceAllString(p.body, ""))
	words := len(strings.Fields(p.text))
	p.Words = words
	p.ReadingMin = (words + 219) / 220
	if p.ReadingMin < 1 {
		p.ReadingMin = 1
	}
	if p.Description != "" {
		p.Summary = p.Description
	} else {
		// The opening prose, not the headings and badges between it.
		// Code, tables and figures read as noise run together in a line.
		prose := headingRe.ReplaceAllString(p.body, " ")
		prose = notProseRe.ReplaceAllString(prose, " ")
		if i := strings.Index(prose, `<div class="footnotes">`); i >= 0 {
			prose = prose[:i]
		}
		prose = footnoteRefRe.ReplaceAllString(prose, "")
		p.Summary = summaryOf(textOf(prose), 40)
	}
}

var (
	anchorRe      = regexp.MustCompile(`<a class="anchor"[^>]*>#</a>`)
	headingRe     = regexp.MustCompile(`(?s)<h[1-6][^>]*>.*?</h[1-6]>`)
	notProseRe    = regexp.MustCompile(`(?s)<pre[ >].*?</pre>|<table[ >].*?</table>|<figure[ >].*?</figure>`)
	footnoteRefRe = regexp.MustCompile(`<sup class="footnote-reference">.*?</sup>`)
)

func (p *Page) root() string { return strings.Repeat("../", strings.Count(p.URL, "/")) }

// href is a site path (notes/foo.html#x) as a link from this page.
func (p *Page) href(target string) string {
	if target == "" {
		return ""
	}
	page, frag, _ := strings.Cut(target, "#")
	if page == p.URL {
		if frag == "" {
			return "#"
		}
		return "#" + frag
	}
	out := p.root() + page
	if frag != "" {
		out += "#" + frag
	}
	return out
}

// ---------------------------------------------------------------------------
// Headings
// ---------------------------------------------------------------------------

func (w *pageWriter) WriteHeadline(h org.Headline) {
	d := w.Document
	if excluded(&h, d) {
		return
	}
	if footnoteSection(&h) {
		// Its definitions are collected for the end of the page.
		org.WriteNodes(w, h.Children...)
		return
	}
	if h.IsArchived() && d.GetOption("arch") != "t" {
		h.Children = nil
	}
	id := w.page.anchors[h.Index]
	lvl := h.Lvl + 1
	if lvl > 6 {
		lvl = 6
	}
	w.WriteString(fmt.Sprintf(`<section class="section level-%d" aria-labelledby="%s">`+"\n", h.Lvl, html.EscapeString(id)))
	w.WriteString(fmt.Sprintf(`<h%d id="%s">`, lvl, html.EscapeString(id)))
	if d.GetOption("todo") != "nil" && h.Status != "" {
		kind := "todo"
		if h.IsDone() {
			kind = "done"
		}
		w.WriteString(fmt.Sprintf(`<span class="kw kw-%s kw-%s">%s</span> `, kind, strings.ToLower(h.Status), html.EscapeString(h.Status)))
	}
	if d.GetOption("pri") != "nil" && h.Priority != "" {
		w.WriteString(fmt.Sprintf(`<span class="pri pri-%s">%s</span> `, strings.ToLower(h.Priority), html.EscapeString(h.Priority)))
	}
	w.WriteString(`<span class="htext">`)
	org.WriteNodes(w, h.Title...)
	w.WriteString(`</span>`)
	if d.GetOption("tags") != "nil" && len(h.Tags) > 0 {
		w.WriteString(` <span class="htags">`)
		for _, t := range h.Tags {
			if t == "ARCHIVE" {
				continue
			}
			w.WriteString(fmt.Sprintf(`<a class="tag" href="%s">%s</a>`,
				html.EscapeString(w.page.root()+tagURL(t)), html.EscapeString(t)))
		}
		w.WriteString(`</span>`)
	}
	w.WriteString(fmt.Sprintf(`<a class="anchor" href="#%s" aria-label="Link to this section">#</a>`, html.EscapeString(id)))
	w.WriteString(fmt.Sprintf("</h%d>\n", lvl))
	org.WriteNodes(w, h.Children...)
	w.WriteString("</section>\n")
}

// A logbook is a record kept for the writer, not something to publish.
func (w *pageWriter) WriteDrawer(d org.Drawer) {
	if strings.EqualFold(d.Name, "LOGBOOK") {
		return
	}
	org.WriteNodes(w, d.Children...)
}

func (w *pageWriter) WriteClock(org.Clock) {}

func (w *pageWriter) WriteSDC(s org.SDC) {
	if w.Document.GetOption("<") == "nil" || s.Date == nil {
		return
	}
	name := map[org.DateType]string{org.Scheduled: "Scheduled", org.Deadline: "Deadline", org.Closed: "Closed"}[s.DateType]
	w.WriteString(fmt.Sprintf(`<span class="planning planning-%s"><span class="planning-kw">%s</span> %s</span> `,
		strings.ToLower(name), name, timeTag(s.Date)))
}

func (w *pageWriter) WriteTimestamp(t org.Timestamp) {
	if w.Document.GetOption("<") == "nil" || t.Time == nil {
		return
	}
	w.WriteString(timeTag(t.Time))
}

func timeTag(d *org.OrgDate) string {
	if d == nil {
		return ""
	}
	text := d.Start.Format("Mon, Jan 2 2006")
	if d.HaveTime {
		text += d.Start.Format(" 15:04")
	}
	if !d.End.IsZero() && !d.End.Equal(d.Start) {
		if d.End.Format("2006-01-02") == d.Start.Format("2006-01-02") {
			text += "–" + d.End.Format("15:04")
		} else {
			text += " – " + d.End.Format("Mon, Jan 2 2006")
			if d.HaveTime {
				text += d.End.Format(" 15:04")
			}
		}
	}
	if r := d.RepeatString(); r != "" {
		text += " (" + r + ")"
	}
	iso := d.Start.Format("2006-01-02")
	if d.HaveTime {
		iso = d.Start.Format("2006-01-02T15:04")
	}
	return fmt.Sprintf(`<time class="timestamp" datetime="%s">%s</time>`, iso, html.EscapeString(text))
}

// Tables scroll on their own on a narrow screen rather than widening the page.
func (w *pageWriter) WriteTable(t org.Table) {
	w.WriteString(`<div class="table-wrap">`)
	w.HTMLWriter.WriteTable(t)
	w.WriteString("</div>\n")
}

func (w *pageWriter) WriteLatexFragment(l org.LatexFragment) {
	w.page.math = true
	w.HTMLWriter.WriteLatexFragment(l)
}

// ---------------------------------------------------------------------------
// Code
// ---------------------------------------------------------------------------

var formatter = chtml.New(chtml.WithClasses(true), chtml.PreventSurroundingPre(false))

func (w *pageWriter) highlight(keywords []org.Keyword, source, lang string, inline bool, params map[string]string) string {
	if lang == "mermaid" {
		w.page.mermaid = true
		return fmt.Sprintf(`<pre class="mermaid">%s</pre>`, html.EscapeString(source))
	}
	src := strings.TrimRight(source, "\n")
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(src)
	}
	label := lang
	if label == "" || label == "text" {
		label = ""
	}
	var out string
	if lexer == nil {
		out = `<pre class="chroma"><code>` + html.EscapeString(src) + `</code></pre>`
	} else {
		lexer = chroma.Coalesce(lexer)
		it, err := lexer.Tokenise(nil, src)
		buf := bytes.Buffer{}
		if err == nil {
			err = formatter.Format(&buf, styles.Fallback, it)
		}
		if err != nil {
			out = `<pre class="chroma"><code>` + html.EscapeString(src) + `</code></pre>`
		} else {
			out = buf.String()
		}
	}
	return fmt.Sprintf(`<div class="code" data-lang="%s">%s</div>`, html.EscapeString(label), out)
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

func (w *pageWriter) description(l org.RegularLink, fallback string) string {
	if l.Description != nil {
		return w.WriteNodesAsString(l.Description...)
	}
	return html.EscapeString(fallback)
}

func (w *pageWriter) link(href, desc, class string) {
	if class != "" {
		class = ` class="` + class + `"`
	}
	w.WriteString(fmt.Sprintf(`<a href="%s"%s>%s</a>`, html.EscapeString(href), class, desc))
}

// dead writes a link that leads nowhere on the site as its text alone.
func (w *pageWriter) dead(l org.RegularLink, why string) {
	w.b.warn("%s: %s", w.page.Src, why)
	desc := w.description(l, strings.TrimPrefix(l.URL, "file:"))
	w.WriteString(fmt.Sprintf(`<span class="dead-link" title="%s">%s</span>`, html.EscapeString(why), desc))
}

func (w *pageWriter) WriteRegularLink(l org.RegularLink) {
	p := w.page
	url := l.URL
	// [[target]] - a <<target>> of this document.
	if l.Protocol == "" && w.Document.Targets[url] {
		w.HTMLWriter.WriteRegularLink(l)
		return
	}
	switch {
	case l.Protocol == "" && strings.HasPrefix(url, "*"):
		title := strings.TrimSpace(url[1:])
		if id, ok := p.headingAnchor(title); ok {
			w.link("#"+id, w.description(l, title), "heading-link")
			return
		}
		w.dead(l, "no heading called "+title)
		return
	case l.Protocol == "" && strings.HasPrefix(url, "#"):
		if target, ok := w.b.custom[url[1:]]; ok {
			w.follow(target)
			w.link(p.href(target), w.description(l, url[1:]), "")
			return
		}
		w.dead(l, "no heading with CUSTOM_ID "+url[1:])
		return
	case l.Protocol == "id":
		id := strings.TrimSpace(strings.TrimPrefix(url, "id:"))
		if target, ok := w.b.ids[id]; ok {
			w.follow(target)
			w.link(p.href(target), w.description(l, id), "")
			return
		}
		w.dead(l, "id:"+id+" is not on a published page")
		return
	case l.Protocol == "file" || l.Protocol == "" || l.Protocol == "file+sys" || l.Protocol == "file+emacs":
		w.fileLink(l)
		return
	case l.Protocol == "attachment":
		w.dead(l, "attachments are not published: "+url)
		return
	}
	// A url: go-org's writer, which also expands #+LINK: abbreviations.
	if l.Protocol == "http" || l.Protocol == "https" {
		if l.Kind() == "image" {
			w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy">`, html.EscapeString(url), html.EscapeString(textOf(w.description(l, path.Base(url))))))
			return
		}
		w.link(url, w.description(l, url), "ext")
		return
	}
	w.HTMLWriter.WriteRegularLink(l)
}

// follow notes that this page links to another, for the other's backlinks.
func (w *pageWriter) follow(target string) {
	page, _, _ := strings.Cut(target, "#")
	if to := w.b.byURL[page]; to != nil && to != w.page {
		w.page.outLinks[to.Src] = true
	}
}

// The file a link names, absolute: relative to the org file it is written in.
func (w *pageWriter) resolve(target string) string {
	if strings.HasPrefix(target, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			target = filepath.Join(home, target[2:])
		}
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(w.page.abs), filepath.FromSlash(target))
	}
	return filepath.Clean(target)
}

func (w *pageWriter) fileLink(l org.RegularLink) {
	p := w.page
	target := l.URL
	for _, pre := range []string{"file+sys:", "file+emacs:", "file:"} {
		target = strings.TrimPrefix(target, pre)
	}
	target = strings.TrimPrefix(target, "//")
	file, search, _ := strings.Cut(target, "::")
	if file == "" {
		w.HTMLWriter.WriteRegularLink(l)
		return
	}
	abs := w.resolve(file)
	if strings.EqualFold(filepath.Ext(abs), ".org") {
		to := w.b.byAbs[abs]
		if to == nil {
			w.dead(l, file+" is not part of the site")
			return
		}
		dest := to.URL
		search = strings.TrimSpace(search)
		switch {
		case strings.HasPrefix(search, "*"):
			if id, ok := to.headingAnchor(strings.TrimSpace(search[1:])); ok {
				dest += "#" + id
			}
		case strings.HasPrefix(search, "#"):
			if t, ok := w.b.custom[search[1:]]; ok && strings.HasPrefix(t, to.URL+"#") {
				dest = t
			}
		case search != "":
			if id, ok := to.headingAnchor(search); ok {
				dest += "#" + id
			}
		}
		if to != p {
			p.outLinks[to.Src] = true
		}
		w.link(p.href(dest), w.description(l, to.Title), "")
		return
	}
	out, ok := w.b.asset(abs)
	if !ok {
		w.dead(l, file+" could not be published")
		return
	}
	src := p.root() + out
	switch l.Kind() {
	case "image":
		alt := path.Base(out)
		if l.Description != nil {
			alt = org.String(l.Description...)
		}
		w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" loading="lazy">`, html.EscapeString(src), html.EscapeString(alt)))
	case "video":
		w.WriteString(fmt.Sprintf(`<video src="%s" controls preload="metadata"></video>`, html.EscapeString(src)))
	default:
		if AudioExt(out) {
			w.WriteString(fmt.Sprintf(`<audio src="%s" controls preload="none"></audio>`, html.EscapeString(src)))
			return
		}
		w.link(src, w.description(l, path.Base(out)), "file")
	}
}

// AudioExt says whether a file is something an <audio> plays.
func AudioExt(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp3", ".m4a", ".ogg", ".oga", ".opus", ".wav", ".flac", ".webm":
		return true
	}
	return false
}

// asset copies a linked file into the site, once, and says where it went:
// at its own path when it is inside the source folder, under assets/files/
// when it is elsewhere in the org directories. Anything outside those is not
// published - a site is a thing one hands to the world, and a stray
// file:~/.ssh/id_rsa should not be in it.
func (b *builder) asset(abs string) (string, bool) {
	if out, ok := b.assets[abs]; ok {
		return out, true
	}
	inside := false
	for _, r := range b.o.Roots {
		root, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			inside = true
			break
		}
	}
	if !inside {
		return "", false
	}
	data, err := b.o.ReadFile(abs)
	if err != nil {
		return "", false
	}
	var out string
	if rel, err := filepath.Rel(b.o.Dir, abs); err == nil && !strings.HasPrefix(rel, "..") {
		out = filepath.ToSlash(rel)
	} else {
		base := filepath.Base(abs)
		out = "assets/files/" + base
		for i := 2; b.files[out] != nil; i++ {
			ext := filepath.Ext(base)
			out = fmt.Sprintf("assets/files/%s-%d%s", strings.TrimSuffix(base, ext), i, ext)
		}
	}
	b.files[out] = data
	b.assets[abs] = out
	return out, true
}

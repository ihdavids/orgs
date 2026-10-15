package manuscript

// The book as a Word document, written as WordprocessingML by hand: a docx is
// a zip of a few XML parts, and writing them directly means no pandoc, no
// LibreOffice, and a file that opens the same in Word, Pages, Google Docs and
// LibreOffice because it uses only the plainest parts of the format.
//
// The styles are the conventions editors and agents ask for - William Shunn's
// "Proper Manuscript Format" for short fiction and for novels - plus a
// reading copy for people who are not going to edit it.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Style is how a manuscript looks. Measures are twentieths of a point (twips)
// and font sizes half points, as Word counts them.
type Style struct {
	Name        string
	Description string
	Font        string
	Size        int // half points
	Line        int // 240 single, 480 double
	Indent      int // first line
	Justify     bool
	// Italics written as underline, the typewriter convention some editors
	// still ask for.
	UnderlineItalics bool
	// The first page is the story's: contact block, word count, title halfway
	// down. A novel has a cover page of its own instead.
	StoryFirstPage bool
	TitlePage      bool
	// A running header "Surname / TITLE / page" on every page but the first.
	RunningHeader bool
	// A page number centred at the foot instead.
	FooterNumbers  bool
	ChapterNewPage bool
	ChapterDrop    int // space above a chapter title
	SceneMark      string
	EndMark        string
	Margin         int
	// Space between paragraphs and no indent - a reading copy's look when
	// asked for.
	BlockParagraphs bool
}

var Styles = map[string]Style{
	"shunn": {
		Name: "shunn", Description: "Shunn's classic manuscript format: Courier, double spaced, italics underlined",
		Font: "Courier New", Size: 24, Line: 480, Indent: 720, UnderlineItalics: true,
		StoryFirstPage: true, RunningHeader: true, ChapterNewPage: true, ChapterDrop: 2880,
		SceneMark: "#", EndMark: "END", Margin: 1440,
	},
	"modern": {
		Name: "modern", Description: "Shunn's modern manuscript format: Times New Roman, double spaced, real italics",
		Font: "Times New Roman", Size: 24, Line: 480, Indent: 720,
		StoryFirstPage: true, RunningHeader: true, ChapterNewPage: true, ChapterDrop: 2880,
		SceneMark: "#", EndMark: "END", Margin: 1440,
	},
	"novel": {
		Name: "novel", Description: "a novel manuscript: cover page, chapters a third down a new page, Times, double spaced",
		Font: "Times New Roman", Size: 24, Line: 480, Indent: 720,
		TitlePage: true, RunningHeader: true, ChapterNewPage: true, ChapterDrop: 4320,
		SceneMark: "#", EndMark: "THE END", Margin: 1440,
	},
	"reader": {
		Name: "reader", Description: "a reading copy for beta readers: Georgia, 1.15 spacing, justified, page numbers at the foot",
		Font: "Georgia", Size: 22, Line: 276, Indent: 360, Justify: true,
		TitlePage: true, FooterNumbers: true, ChapterNewPage: true, ChapterDrop: 2160,
		SceneMark: "⁂", Margin: 1300,
	},
}

// StyleNames are the styles, in a stable order.
func StyleNames() []string {
	out := []string{}
	for n := range Styles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// RoundedWords is the word count as a manuscript states it: to the nearest
// hundred, and a novel to the nearest thousand.
func RoundedWords(n int, novel bool) int {
	switch {
	case n < 100:
		return n
	case novel && n >= 10000:
		return int(math.Round(float64(n)/1000)) * 1000
	}
	return int(math.Round(float64(n)/100)) * 100
}

func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// ----------------------------------------------------------------------------

type docx struct {
	st    Style
	book  *Book
	body  bytes.Buffer
	notes bytes.Buffer
	ids   map[string]int
	textW int // between the margins
}

// Paper sizes, in twips.
func paper(name string) (int, int) {
	if strings.EqualFold(name, "a4") {
		return 11906, 16838
	}
	return 12240, 15840
}

// WriteDocx writes the book in a style ("shunn", "modern", "novel", "reader").
func WriteDocx(b *Book, styleName, paperName string, now time.Time) ([]byte, error) {
	st, ok := Styles[strings.ToLower(styleName)]
	if styleName == "" {
		st, ok = Styles["modern"], true
	}
	if !ok {
		return nil, fmt.Errorf("no manuscript style %q (styles: %s)", styleName, strings.Join(StyleNames(), ", "))
	}
	if b.Meta.SceneMark != "" {
		st.SceneMark = b.Meta.SceneMark
	}
	w, h := paper(paperName)
	d := &docx{st: st, book: b, ids: map[string]int{}, textW: w - 2*st.Margin}
	d.document()

	var out bytes.Buffer
	z := zip.NewWriter(&out)
	put := func(name, content string) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", rootRelsXML},
		{"docProps/core.xml", d.coreXML(now)},
		{"docProps/app.xml", fmt.Sprintf(appXML, b.Words)},
		{"word/_rels/document.xml.rels", d.docRels()},
		{"word/document.xml", d.documentXML(w, h)},
		{"word/styles.xml", d.stylesXML()},
		{"word/settings.xml", settingsXML},
		{"word/footnotes.xml", d.footnotesXML()},
		{"word/header1.xml", d.headerXML()},
		{"word/header2.xml", emptyHeaderXML},
		{"word/footer1.xml", d.footerXML()},
	}
	for _, p := range parts {
		if err := put(p.name, p.content); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func x(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			// XML 1.0 has no place for most control characters.
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r == 0xFFFE || r == 0xFFFF {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ----------------------------------------------------------------------------
// Paragraphs and runs
// ----------------------------------------------------------------------------

type pp struct {
	style   string
	align   string // center, right, both
	indent  int
	left    int
	right   int
	hanging int
	before  int
	after   int
	line    int
	newPage bool
	keep    bool
	tab     int // a right-aligned tab stop here
}

func (p pp) xml() string {
	var b strings.Builder
	b.WriteString("<w:pPr>")
	if p.style != "" {
		fmt.Fprintf(&b, `<w:pStyle w:val="%s"/>`, p.style)
	}
	// Word refuses properties out of the schema's order, so this order is
	// load-bearing: pStyle, keepNext, pageBreakBefore, tabs, spacing, ind, jc.
	if p.keep {
		b.WriteString("<w:keepNext/>")
	}
	if p.newPage {
		b.WriteString("<w:pageBreakBefore/>")
	}
	if p.tab > 0 {
		fmt.Fprintf(&b, `<w:tabs><w:tab w:val="right" w:pos="%d"/></w:tabs>`, p.tab)
	}
	if p.before > 0 || p.after > 0 || p.line > 0 {
		b.WriteString("<w:spacing")
		if p.before > 0 {
			fmt.Fprintf(&b, ` w:before="%d"`, p.before)
		}
		if p.after > 0 {
			fmt.Fprintf(&b, ` w:after="%d"`, p.after)
		}
		if p.line > 0 {
			fmt.Fprintf(&b, ` w:line="%d" w:lineRule="auto"`, p.line)
		}
		b.WriteString("/>")
	}
	if p.indent >= 0 || p.left > 0 || p.right > 0 || p.hanging > 0 {
		b.WriteString("<w:ind")
		if p.left > 0 {
			fmt.Fprintf(&b, ` w:left="%d"`, p.left)
		}
		if p.right > 0 {
			fmt.Fprintf(&b, ` w:right="%d"`, p.right)
		}
		if p.hanging > 0 {
			fmt.Fprintf(&b, ` w:hanging="%d"`, p.hanging)
		} else if p.indent >= 0 {
			fmt.Fprintf(&b, ` w:firstLine="%d"`, p.indent)
		}
		b.WriteString("/>")
	}
	if p.align != "" {
		fmt.Fprintf(&b, `<w:jc w:val="%s"/>`, p.align)
	}
	b.WriteString("</w:pPr>")
	return b.String()
}

func (d *docx) para(p pp, runs string) {
	d.body.WriteString("<w:p>" + p.xml() + runs + "</w:p>\n")
}

// rp is a run's look. Written in the schema's order (rStyle, rFonts, b, i,
// strike, sz, u, vertAlign), because Word refuses a file that is not.
type rp struct {
	bold, italic, under, strike, mono bool
	size                              int
	vert                              string
}

func (r rp) xml() string {
	var b strings.Builder
	if r.mono {
		b.WriteString(`<w:rFonts w:ascii="Courier New" w:hAnsi="Courier New" w:cs="Courier New"/>`)
	}
	if r.bold {
		b.WriteString("<w:b/>")
	}
	if r.italic {
		b.WriteString("<w:i/>")
	}
	if r.strike {
		b.WriteString("<w:strike/>")
	}
	if r.size > 0 {
		fmt.Fprintf(&b, `<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, r.size, r.size)
	}
	if r.under {
		b.WriteString(`<w:u w:val="single"/>`)
	}
	if r.vert != "" {
		fmt.Fprintf(&b, `<w:vertAlign w:val="%s"/>`, r.vert)
	}
	if b.Len() == 0 {
		return ""
	}
	return "<w:rPr>" + b.String() + "</w:rPr>"
}

var (
	plainRun  = rp{}
	boldRun   = rp{bold: true}
	italicRun = rp{italic: true}
)

func (d *docx) text(s string, look rp) string {
	return "<w:r>" + look.xml() + `<w:t xml:space="preserve">` + x(s) + "</w:t></w:r>"
}

func (d *docx) runs(rs []Run, base rp) string {
	var b strings.Builder
	for _, r := range rs {
		if r.Break {
			b.WriteString("<w:r><w:br/></w:r>")
			continue
		}
		if r.Note != "" {
			id := d.note(r.Note)
			if id > 0 {
				fmt.Fprintf(&b, `<w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteReference w:id="%d"/></w:r>`, id)
			}
			continue
		}
		look := base
		look.bold = look.bold || r.Bold
		if r.Italic {
			if d.st.UnderlineItalics {
				look.under = true
			} else {
				look.italic = true
			}
		}
		look.under = look.under || r.Underline
		look.strike = look.strike || r.Strike
		look.mono = look.mono || r.Code
		if r.Sup {
			look.vert = "superscript"
		} else if r.Sub {
			look.vert = "subscript"
		}
		b.WriteString(d.text(r.Text, look))
	}
	return b.String()
}

// note numbers a footnote the first time it is referred to and writes it.
func (d *docx) note(name string) int {
	if id, ok := d.ids[name]; ok {
		return id
	}
	blocks, ok := d.book.Notes[name]
	if !ok {
		return 0
	}
	id := len(d.ids) + 2 // 0 and 1 are the separators
	d.ids[name] = id
	fmt.Fprintf(&d.notes, `<w:footnote w:id="%d">`, id)
	for i, b := range blocks {
		ref := ""
		if i == 0 {
			ref = `<w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteRef/></w:r><w:r><w:t xml:space="preserve"> </w:t></w:r>`
		}
		fmt.Fprintf(&d.notes, `<w:p><w:pPr><w:pStyle w:val="FootnoteText"/></w:pPr>%s%s</w:p>`, ref, d.runs(b.Runs, plainRun))
	}
	if len(blocks) == 0 {
		d.notes.WriteString(`<w:p><w:pPr><w:pStyle w:val="FootnoteText"/></w:pPr><w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteRef/></w:r></w:p>`)
	}
	d.notes.WriteString("</w:footnote>\n")
	return id
}

// ----------------------------------------------------------------------------
// The document
// ----------------------------------------------------------------------------

func (d *docx) single() pp { return pp{indent: 0, line: 240} }

// contact is the block at the top left of the first page, single spaced, with
// the word count across from its first line.
func (d *docx) contact(lines []string, about string) {
	if len(lines) == 0 {
		d.para(pp{indent: 0, line: 240, align: "right"}, d.text(about, plainRun))
		return
	}
	for i, l := range lines {
		p := d.single()
		if i == 0 {
			p.tab = d.textW
			d.para(p, d.text(l, plainRun)+`<w:r><w:tab/></w:r>`+d.text(about, plainRun))
			continue
		}
		d.para(p, d.text(l, plainRun))
	}
}

func (d *docx) document() {
	m := d.book.Meta
	st := d.st
	words := RoundedWords(d.book.Words, st.TitlePage)
	about := "about " + commas(words) + " words"

	contact := []string{}
	if m.Author != "" {
		contact = append(contact, m.Author)
	}
	contact = append(contact, m.Address...)
	for _, s := range []string{m.Phone, m.Email, m.Website} {
		if s != "" {
			contact = append(contact, s)
		}
	}

	byline := m.Name()
	switch {
	case st.StoryFirstPage:
		// Contact top left, the word count against it on the right, then the
		// title about halfway down the page.
		d.contact(contact, about)
		d.para(pp{indent: 0, align: "center", before: 4320 - 240*min(len(contact), 12), keep: true}, d.text(m.Title, plainRun))
		if m.Subtitle != "" {
			d.para(pp{indent: 0, align: "center", keep: true}, d.text(m.Subtitle, plainRun))
		}
		if byline != "" {
			d.para(pp{indent: 0, align: "center", after: 480, keep: true}, d.text("by "+byline, plainRun))
		}
	case st.TitlePage:
		d.contact(contact, about)
		d.para(pp{indent: 0, align: "center", before: 4320}, d.text(strings.ToUpper(m.Title), boldRun))
		if m.Subtitle != "" {
			d.para(pp{indent: 0, align: "center"}, d.text(m.Subtitle, italicRun))
		}
		if byline != "" {
			d.para(pp{indent: 0, align: "center", before: 240}, d.text("by "+byline, plainRun))
		}
		if len(m.Agent) > 0 {
			d.para(pp{indent: 0, before: 3600, line: 240}, d.text("Represented by", italicRun))
			for _, l := range m.Agent {
				d.para(d.single(), d.text(l, plainRun))
			}
		}
		if m.Dedication != "" {
			d.para(pp{indent: 0, align: "center", newPage: true, before: 4320}, d.text(m.Dedication, italicRun))
		}
	}

	first := true
	for _, c := range d.book.Chapters {
		d.chapter(c, first)
		first = false
	}
	if st.EndMark != "" {
		d.para(pp{indent: 0, align: "center", before: 480}, d.text(st.EndMark, plainRun))
	}
}

func (d *docx) chapter(c *Chapter, first bool) {
	st := d.st
	title := c.Title
	// A short story's first chapter follows its title on the first page; a
	// novel's, and every later one, starts a page of its own.
	newPage := st.ChapterNewPage && !(first && st.StoryFirstPage)
	drop := st.ChapterDrop
	if first && st.StoryFirstPage {
		drop = 0
	}
	if c.Kind == "part" {
		d.para(pp{style: "Heading1", indent: 0, align: "center", newPage: true, before: 5760}, d.runs(title, plainRun))
		return
	}
	if c.Label != "" || len(title) > 0 {
		if c.Label != "" {
			d.para(pp{style: "Heading1", indent: 0, align: "center", newPage: newPage, before: drop, keep: true}, d.text(c.Label, plainRun))
			newPage, drop = false, 0
			if len(title) > 0 {
				d.para(pp{style: "Subtitle", indent: 0, align: "center", keep: true, after: 480}, d.runs(title, plainRun))
			}
		} else {
			d.para(pp{style: "Heading1", indent: 0, align: "center", newPage: newPage, before: drop, after: 480, keep: true}, d.runs(title, plainRun))
		}
	} else if newPage {
		d.para(pp{indent: 0, newPage: true, before: drop}, "")
	}
	for _, b := range c.Blocks {
		d.block(b, 0)
	}
}

func (d *docx) body0(first bool, left int) pp {
	p := pp{indent: d.st.Indent, left: left}
	if first || d.st.BlockParagraphs {
		p.indent = 0
	}
	if d.st.Justify {
		p.align = "both"
	}
	if d.st.BlockParagraphs {
		p.after = 160
	}
	return p
}

func (d *docx) block(b Block, left int) {
	st := d.st
	switch b.Kind {
	case "p":
		d.para(d.body0(b.First, left), d.runs(b.Runs, plainRun))
	case "scene":
		d.para(pp{indent: 0, align: "center", keep: true}, d.text(st.SceneMark, plainRun))
	case "heading":
		style := "Heading2"
		if b.Level > 1 {
			style = "Heading3"
		}
		d.para(pp{style: style, indent: 0, keep: true}, d.runs(b.Runs, plainRun))
	case "quote", "center":
		for _, c := range b.Children {
			if b.Kind == "center" {
				d.para(pp{indent: 0, align: "center"}, d.runs(c.Runs, plainRun))
				continue
			}
			p := d.body0(true, left+720)
			p.right = 720
			if c.Kind == "p" {
				d.para(p, d.runs(c.Runs, plainRun))
			} else {
				d.block(c, left+720)
			}
		}
	case "verse":
		d.para(pp{indent: 0, left: left + 720}, d.runs(b.Runs, plainRun))
	case "pre":
		lines := strings.Split(plain(b.Runs), "\n")
		var r strings.Builder
		for i, l := range lines {
			if i > 0 {
				r.WriteString("<w:r><w:br/></w:r>")
			}
			r.WriteString(d.text(l, rp{mono: true, size: 20}))
		}
		d.para(pp{indent: 0, left: left + 360, line: 240}, r.String())
	case "ul", "ol", "dl":
		for i, item := range b.Items {
			mark := "•"
			if b.Kind == "ol" {
				mark = strconv.Itoa(i+1) + "."
			}
			if b.Kind == "dl" {
				term := ""
				if i < len(b.Terms) {
					term = d.runs(b.Terms[i], boldRun)
				}
				d.para(pp{indent: 0, left: left + 360, keep: true}, term)
				for _, c := range item {
					if c.Kind == "p" {
						d.para(pp{indent: 0, left: left + 720}, d.runs(c.Runs, plainRun))
					} else {
						d.block(c, left+720)
					}
				}
				continue
			}
			for j, c := range item {
				if c.Kind != "p" {
					d.block(c, left+720)
					continue
				}
				lead := ""
				if j == 0 {
					lead = d.text(mark, plainRun) + "<w:r><w:tab/></w:r>"
				}
				d.para(pp{indent: 0, left: left + 720, hanging: 360}, lead+d.runs(c.Runs, plainRun))
			}
		}
	case "table":
		d.table(b)
	case "image":
		cap := plain(b.Caption)
		if cap == "" {
			cap = b.Src[strings.LastIndexAny(b.Src, `/\`)+1:]
		}
		d.para(pp{indent: 0, align: "center"}, d.text("[Figure: "+cap+"]", italicRun))
	}
}

func (d *docx) table(b Block) {
	cols := 0
	for _, r := range b.Rows {
		cols = max(cols, len(r))
	}
	d.body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>` +
		`<w:top w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:left w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:right w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="auto"/><w:insideV w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
		`</w:tblBorders><w:tblCellMar><w:left w:w="80" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`)
	for i := 0; i < cols; i++ {
		fmt.Fprintf(&d.body, `<w:gridCol w:w="%d"/>`, d.textW/max(cols, 1))
	}
	d.body.WriteString("</w:tblGrid>")
	for i, row := range b.Rows {
		d.body.WriteString("<w:tr>")
		if i < b.HeadRows {
			d.body.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for c := 0; c < cols; c++ {
			var cell []Run
			if c < len(row) {
				cell = row[c]
			}
			bold := plainRun
			if i < b.HeadRows {
				bold = boldRun
			}
			d.body.WriteString("<w:tc><w:p>" + pp{indent: 0, line: 240}.xml() + d.runs(cell, bold) + "</w:p></w:tc>")
		}
		d.body.WriteString("</w:tr>")
	}
	d.body.WriteString("</w:tbl>\n")
	d.para(pp{indent: 0, line: 240}, "")
}

func (d *docx) documentXML(w, h int) string {
	st := d.st
	var b strings.Builder
	b.WriteString(xmlHead + `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>` + "\n")
	b.Write(d.body.Bytes())
	b.WriteString("<w:sectPr>")
	if st.RunningHeader {
		b.WriteString(`<w:headerReference w:type="default" r:id="rIdHeader"/><w:headerReference w:type="first" r:id="rIdHeaderFirst"/>`)
	}
	if st.FooterNumbers {
		b.WriteString(`<w:footerReference w:type="default" r:id="rIdFooter"/>`)
	}
	b.WriteString(`<w:footnotePr><w:numFmt w:val="decimal"/></w:footnotePr>`)
	fmt.Fprintf(&b, `<w:pgSz w:w="%d" w:h="%d"/>`, w, h)
	fmt.Fprintf(&b, `<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="720" w:footer="720" w:gutter="0"/>`,
		st.Margin, st.Margin, st.Margin, st.Margin)
	if st.TitlePage {
		// The cover page is page zero, so chapter one starts on page one.
		b.WriteString(`<w:pgNumType w:start="0"/>`)
	}
	if st.RunningHeader || st.TitlePage {
		b.WriteString("<w:titlePg/>")
	}
	b.WriteString("</w:sectPr></w:body></w:document>")
	return b.String()
}

func (d *docx) headerXML() string {
	m := d.book.Meta
	key := strings.ToUpper(m.Title)
	// A long title is shortened to its first words, as the header has only
	// a line.
	if f := strings.Fields(key); len(f) > 4 {
		key = strings.Join(f[:4], " ")
	}
	label := key
	if s := m.Surname(); s != "" {
		label = s + " / " + key + " / "
	} else {
		label += " / "
	}
	return xmlHead + `<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:pStyle w:val="Header"/><w:jc w:val="right"/></w:pPr>` +
		d.text(label, plainRun) + `<w:fldSimple w:instr=" PAGE "><w:r><w:t>2</w:t></w:r></w:fldSimple></w:p></w:hdr>`
}

func (d *docx) footerXML() string {
	return xmlHead + `<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:pStyle w:val="Footer"/><w:jc w:val="center"/></w:pPr>` +
		`<w:fldSimple w:instr=" PAGE "><w:r><w:t>1</w:t></w:r></w:fldSimple></w:p></w:ftr>`
}

func (d *docx) footnotesXML() string {
	return xmlHead + `<w:footnotes xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:footnote w:type="separator" w:id="0"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:separator/></w:r></w:p></w:footnote>` +
		`<w:footnote w:type="continuationSeparator" w:id="1"><w:p><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>` +
		d.notes.String() + `</w:footnotes>`
}

func (d *docx) docRels() string {
	return xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		`<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>` +
		`<Relationship Id="rIdFootnotes" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes" Target="footnotes.xml"/>` +
		`<Relationship Id="rIdHeader" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>` +
		`<Relationship Id="rIdHeaderFirst" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header2.xml"/>` +
		`<Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>` +
		`</Relationships>`
}

func (d *docx) coreXML(now time.Time) string {
	m := d.book.Meta
	stamp := now.UTC().Format("2006-01-02T15:04:05Z")
	return xmlHead + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + x(m.Title) + `</dc:title><dc:creator>` + x(m.Name()) + `</dc:creator>` +
		`<dc:description>` + x(m.Description) + `</dc:description><dc:language>` + x(m.Language) + `</dc:language>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + stamp + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

func (d *docx) stylesXML() string {
	st := d.st
	font := x(st.Font)
	return xmlHead + `<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="` + font + `" w:hAnsi="` + font + `" w:eastAsia="` + font + `" w:cs="` + font + `"/>` +
		fmt.Sprintf(`<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, st.Size, st.Size) + `<w:lang w:val="` + x(d.book.Meta.Language) + `"/></w:rPr></w:rPrDefault>` +
		fmt.Sprintf(`<w:pPrDefault><w:pPr><w:spacing w:before="0" w:after="0" w:line="%d" w:lineRule="auto"/><w:widowControl/></w:pPr></w:pPrDefault></w:docDefaults>`, st.Line) +
		// The same again on Normal: Word reads the defaults, but Quick Look,
		// Pages and some converters only read the style.
		`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/>` +
		fmt.Sprintf(`<w:pPr><w:spacing w:before="0" w:after="0" w:line="%d" w:lineRule="auto"/></w:pPr>`, st.Line) +
		`<w:rPr><w:rFonts w:ascii="` + font + `" w:hAnsi="` + font + `" w:eastAsia="` + font + `" w:cs="` + font + `"/>` +
		fmt.Sprintf(`<w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:style>`, st.Size, st.Size) +
		`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:outlineLvl w:val="0"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:spacing w:before="240"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:outlineLvl w:val="2"/></w:pPr><w:rPr><w:b/><w:i/></w:rPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Subtitle"><w:name w:val="Subtitle"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Header"><w:name w:val="header"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:line="240" w:lineRule="auto"/></w:pPr></w:style>` +
		`<w:style w:type="paragraph" w:styleId="Footer"><w:name w:val="footer"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:line="240" w:lineRule="auto"/></w:pPr></w:style>` +
		fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="FootnoteText"><w:name w:val="footnote text"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:line="240" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="%d"/></w:rPr></w:style>`, st.Size-4) +
		`<w:style w:type="character" w:styleId="FootnoteReference"><w:name w:val="footnote reference"/><w:rPr><w:vertAlign w:val="superscript"/></w:rPr></w:style>` +
		`</w:styles>`
}

const xmlHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const contentTypesXML = xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
	`<Override PartName="/word/footnotes.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml"/>` +
	`<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>` +
	`<Override PartName="/word/header2.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>` +
	`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>` +
	`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
	`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
	`</Types>`

const rootRelsXML = xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
	`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
	`</Relationships>`

const appXML = xmlHead + `<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>orgs</Application><Words>%d</Words></Properties>`

// The tab stop at the right margin is for the word count across from the
// author's name.
const settingsXML = xmlHead + `<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:defaultTabStop w:val="720"/><w:characterSpacingControl w:val="doNotCompress"/>` +
	`<w:footnotePr><w:footnote w:id="0"/><w:footnote w:id="1"/></w:footnotePr><w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat></w:settings>`

const emptyHeaderXML = xmlHead + `<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p/></w:hdr>`

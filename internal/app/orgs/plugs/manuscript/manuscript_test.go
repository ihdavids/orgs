package manuscript

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/go-org/org"
)

const novel = `#+TITLE: The Harbour Light
#+AUTHOR: Ian Davids
#+PEN_NAME: I. H. Davids
#+ADDRESS: 12 Harbour St
#+ADDRESS: Halifax NS
#+CHAPTER_NUMBERS: words

* Part One  :part:
** The Keeper
The light had /burned/ for years.[fn:1]

*** Night
The storm arrived.

#+BEGIN_VERSE
line one
line two
#+END_VERSE

-----

He stayed.
** Unnumbered  :unnumbered:
An aside.
* Part Two  :part:
** Thaw
- tea
- letters
** Secret  :noexport:
Hidden words.
* COMMENT Not this
Nor this.
* Footnotes
[fn:1] Two hundred and twelve.
`

func parse(t *testing.T, s string) *Book {
	t.Helper()
	doc := org.New().Parse(strings.NewReader(s), "/books/harbour.org")
	if doc.Error != nil {
		t.Fatal(doc.Error)
	}
	return Read(doc, "/books/harbour.org", Meta{Email: "me@example.com"})
}

func TestStructure(t *testing.T) {
	b := parse(t, novel)
	if b.Meta.Name() != "I. H. Davids" || b.Meta.Surname() != "Davids" || len(b.Meta.Address) != 2 || b.Meta.Email != "me@example.com" {
		t.Errorf("meta: %+v", b.Meta)
	}
	kinds := []string{}
	for _, c := range b.Chapters {
		kinds = append(kinds, c.Kind+":"+PlainTitle(c.Title)+":"+c.Label)
	}
	want := "part:Part One:|chapter:The Keeper:Chapter One|chapter:Unnumbered:|part:Part Two:|chapter:Thaw:Chapter Two"
	if got := strings.Join(kinds, "|"); got != want {
		t.Fatalf("chapters:\n%s\nwant\n%s", got, want)
	}
	keeper := b.Chapters[1].Blocks
	order := []string{}
	for _, bl := range keeper {
		order = append(order, bl.Kind)
	}
	// The level-three heading and the ----- are both scene breaks.
	if got := strings.Join(order, ","); got != "p,scene,p,verse,scene,p" {
		t.Errorf("blocks %s", got)
	}
	if !keeper[0].First || keeper[2].First != true || keeper[5].First != true {
		t.Errorf("a paragraph after a break is a first paragraph: %+v", keeper)
	}
	if r := keeper[0].Runs; len(r) < 3 || !r[1].Italic || r[1].Text != "burned" || r[len(r)-1].Note != "1" {
		t.Errorf("runs %+v", r)
	}
	if v := keeper[3].Runs; len(v) != 3 || !v[1].Break {
		t.Errorf("verse keeps its lines: %+v", v)
	}
	if n := b.Notes["1"]; len(n) != 1 || plain(n[0].Runs) != "Two hundred and twelve." {
		t.Errorf("notes %+v", b.Notes)
	}
	for _, c := range b.Chapters {
		for _, bl := range c.Blocks {
			if strings.Contains(plain(bl.Runs), "Hidden") || strings.Contains(plain(bl.Runs), "Nor this") {
				t.Errorf("noexport or COMMENT text exported: %+v", bl)
			}
		}
	}
	if b.Words != 19 {
		t.Errorf("words %d", b.Words)
	}
}

func TestStoryWithoutHeadings(t *testing.T) {
	b := parse(t, "#+TITLE: Short\n\nOne paragraph.\n\n***\n\nAnother.\n\n⁂\n\nLast.\n")
	if len(b.Chapters) != 1 || len(b.Chapters[0].Title) != 0 || len(b.Chapters[0].Blocks) != 5 || b.Chapters[0].Blocks[1].Kind != "scene" || b.Chapters[0].Blocks[3].Kind != "scene" {
		t.Fatalf("%+v", b.Chapters)
	}
}

func TestHeadingsNotScenes(t *testing.T) {
	doc := org.New().Parse(strings.NewReader("#+MANUSCRIPT_SCENES: nil\n* One\n** Section\nText.\n"), "/x.org")
	b := Read(doc, "/x.org", Meta{})
	if bl := b.Chapters[0].Blocks; len(bl) != 2 || bl[0].Kind != "heading" || PlainTitle(bl[0].Runs) != "Section" {
		t.Errorf("%+v", bl)
	}
}

func TestNumbers(t *testing.T) {
	for n, w := range map[int]string{1: "One", 13: "Thirteen", 20: "Twenty", 42: "Forty-Two", 101: "One Hundred One"} {
		if got := NumberWords(n); got != w {
			t.Errorf("%d: %s", n, got)
		}
	}
	if Roman(1994) != "MCMXCIV" {
		t.Error(Roman(1994))
	}
	for _, c := range []struct {
		n     int
		novel bool
		want  int
	}{{87, false, 87}, {4250, false, 4300}, {4249, false, 4200}, {87654, true, 88000}, {8765, true, 8800}} {
		if got := RoundedWords(c.n, c.novel); got != c.want {
			t.Errorf("RoundedWords(%d, %v) = %d", c.n, c.novel, got)
		}
	}
}

func parts(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for i, f := range z.File {
		if i == 0 {
			out["#first"] = []byte(f.Name)
			if f.Method == zip.Store {
				out["#stored"] = nil
			}
		}
		r, _ := f.Open()
		out[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	return out
}

func wellFormed(t *testing.T, name string, b []byte) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(b))
	d.Strict = true
	d.Entity = xml.HTMLEntity
	for {
		if _, err := d.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
	}
}

func TestDocx(t *testing.T) {
	b := parse(t, novel)
	for _, st := range StyleNames() {
		out, err := WriteDocx(b, st, "a4", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		ps := parts(t, out)
		for n, p := range ps {
			if strings.HasSuffix(n, ".xml") || strings.HasSuffix(n, ".rels") {
				wellFormed(t, st+":"+n, p)
			}
		}
		doc := string(ps["word/document.xml"])
		if !strings.Contains(doc, `<w:pgSz w:w="11906"`) || !strings.Contains(doc, "Chapter One") || !strings.Contains(doc, `<w:footnoteReference w:id="2"/>`) {
			t.Errorf("%s: document.xml is missing paper size, chapter label or footnote", st)
		}
		if strings.Contains(doc, "Hidden") {
			t.Errorf("%s: noexport text", st)
		}
		italic := `<w:i/>`
		if Styles[st].UnderlineItalics {
			italic = `<w:u w:val="single"/>`
		}
		if !strings.Contains(doc, "<w:rPr>"+italic+`</w:rPr><w:t xml:space="preserve">burned`) {
			t.Errorf("%s: italics should be %s", st, italic)
		}
	}
	if _, err := WriteDocx(b, "fancy", "", time.Now()); err == nil {
		t.Error("an unknown style should be refused")
	}
}

func TestEpub(t *testing.T) {
	b := parse(t, novel)
	out, warns, err := WriteEpub(b, "p { margin: 0 }", time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	ps := parts(t, out)
	if string(ps["#first"]) != "mimetype" || string(ps["mimetype"]) != "application/epub+zip" {
		t.Fatal("mimetype must be the first entry")
	}
	if _, ok := ps["#stored"]; !ok {
		t.Error("mimetype must be stored, not compressed")
	}
	for n, p := range ps {
		if strings.HasSuffix(n, ".xhtml") || strings.HasSuffix(n, ".opf") || strings.HasSuffix(n, ".ncx") || strings.HasSuffix(n, ".xml") {
			wellFormed(t, n, p)
		}
	}
	opf := string(ps["OEBPS/content.opf"])
	for _, want := range []string{`<dc:title id="title">The Harbour Light</dc:title>`, `>I. H. Davids</dc:creator>`,
		`<meta property="dcterms:modified">2026-10-14T00:00:00Z</meta>`, `properties="nav"`, `<itemref idref="ch002"/>`} {
		if !strings.Contains(opf, want) {
			t.Errorf("opf lacks %s", want)
		}
	}
	ch := string(ps["OEBPS/ch002.xhtml"])
	if !strings.Contains(ch, `epub:type="noteref"`) || !strings.Contains(ch, `<aside epub:type="footnote"`) || !strings.Contains(ch, "<em>burned</em>") {
		t.Errorf("chapter:\n%s", ch)
	}
	nav := string(ps["OEBPS/nav.xhtml"])
	if !strings.Contains(nav, `Part One</a><ol><li><a href="ch002.xhtml">Chapter One: The Keeper</a>`) {
		t.Errorf("nav nests chapters under parts:\n%s", nav)
	}
	if len(warns) != 0 {
		t.Errorf("warnings %v", warns)
	}
	// The same book is the same book: its id does not change between exports.
	again, _, _ := WriteEpub(b, "", time.Now())
	if !strings.Contains(string(parts(t, again)["OEBPS/content.opf"]), (&epub{book: b}).identifier()) {
		t.Error("identifier should be stable")
	}
}

package manuscript

/* SDOC: Exporters
* Manuscripts and Ebooks

  One org file as a manuscript (=.docx=) to send to an editor or agent, or as
  an ebook (=.epub=). Both are written by orgs itself - no pandoc, Word or
  LibreOffice needed.

  #+BEGIN_EXAMPLE
  orgs export -f manuscript story.org                    story.docx, modern format
  orgs export -f manuscript novel.org -style novel -paper a4
  orgs export -f epub novel.org -theme literary          novel.epub
  orgs export -f manuscript -list-themes                 the styles
  orgs export -f epub -list-themes                       the themes
  #+END_EXAMPLE

** How the file is read

  - The shallowest headings are *chapters*. A heading tagged =:part:= is a
    part, and the headings under it are its chapters.
  - Headings below a chapter are *scene breaks* (their titles are not
    printed), unless =#+MANUSCRIPT_SCENES: nil= makes them sub-headings, or a
    heading has =:SCENE: title=.
  - A line of =-----=, or a paragraph that is only =***=, =⁂= or =§=, is a
    scene break too. (Not =#= or =* * *=: org reads those as a comment and a
    heading.)
  - =:frontmatter:= / =:backmatter:= headings are unnumbered front and back
    matter; =:unnumbered:= keeps a chapter out of the count; =:ignore:= drops
    the heading but keeps its text; =:noexport:= and =COMMENT= headings are
    left out, as are drawers, planning lines, comments and =#+BEGIN_COMMENT=.
  - Text before the first heading opens the book, so a short story needs no
    headings at all.
  - Emphasis, footnotes (=[fn:1]=, real footnotes in Word, pop-ups in an
    ebook), quotes, verse, lists, tables and pictures are kept.

** What the file can say

  | Keyword              | Means                                                    |
  |----------------------+----------------------------------------------------------|
  | =#+TITLE=, =#+SUBTITLE= | the title                                             |
  | =#+AUTHOR=           | your legal name, for the contact block                   |
  | =#+PEN_NAME=         | the name on the byline, when it is not your own          |
  | =#+EMAIL=, =#+PHONE=, =#+WEBSITE= | contact details                             |
  | =#+ADDRESS=          | one line each (repeat it, or separate lines with =\\=)   |
  | =#+AGENT=            | your agent's name and agency, on a novel's cover page    |
  | =#+DEDICATION=       | a dedication page                                        |
  | =#+CHAPTER_NUMBERS=  | =words= (Chapter One), =digits=, =roman=, or none        |
  | =#+CHAPTER_LABEL=    | the word before the number (default Chapter)             |
  | =#+SCENE_BREAK=      | what a scene break is written as (default =#=, ebooks =⁂=) |
  | =#+LANGUAGE=, =#+DATE=, =#+DESCRIPTION=, =#+PUBLISHER=, =#+RIGHTS= | ebook metadata |
  | =#+ISBN=, =#+SERIES=, =#+SERIES_INDEX=, =#+KEYWORDS= | ebook metadata             |
  | =#+COVER=            | the ebook's cover picture                                |
  | =#+EPUB_CSS=         | a stylesheet of your own, after the theme                |

  Contact details that are the same for everything you write go in the
  yaml instead, and the file's own keywords win:

  #+BEGIN_SRC yaml
  server:
    exporters:
      - name: manuscript
        props:
          author: "Jane Q. Writer"
          penName: "J. Q. Writer"
          email: jane@example.com
          phone: "+1 555 0100"
          address: ["123 Any Street", "Springfield, ST 12345"]
          agent: ["Ann Agent", "Agency & Co."]
          style: modern
          paper: letter
          theme: classic
  #+END_SRC

** Manuscript styles

  | Style    | For                                                                     |
  |----------+-------------------------------------------------------------------------|
  | =modern= | short fiction in Shunn's modern format: Times New Roman 12, double spaced, contact block and word count on page one, title halfway down, =Surname / TITLE / page= header, =#= scene breaks (the default) |
  | =shunn=  | the same in the classic format: Courier, italics underlined             |
  | =novel=  | a novel: a cover page (contact, word count to the nearest thousand, agent), every chapter a third of the way down a new page, =THE END= |
  | =reader= | a reading copy for beta readers: Georgia, 1.15 spacing, justified, page numbers at the foot |

** Ebook themes

  =classic= (serif, indented paragraphs, centred chapter heads),
  =literary= (small capitals opening each chapter, a large first letter),
  =modern= (sans-serif headings, open spacing), and =plain= (almost no
  styling: the reader's own font and settings win). A theme is one
  stylesheet in =epub/themes/= on the template path, so a new one is a file.
EDOC */

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Settings is the exporter's yaml: defaults for every file.
type Settings struct {
	Author  string   `yaml:"author"`
	PenName string   `yaml:"penName"`
	Email   string   `yaml:"email"`
	Phone   string   `yaml:"phone"`
	Website string   `yaml:"website"`
	Address []string `yaml:"address"`
	Agent   []string `yaml:"agent"`
	Style   string   `yaml:"style"`
	Paper   string   `yaml:"paper"`
	Theme   string   `yaml:"theme"`
	// Props is how exporters are configured (exporters: - name: x props:).
	Props map[string]interface{} `yaml:"props"`
}

func (s *Settings) str(key string, dst *string) {
	if v, ok := s.Props[key]; ok {
		if t, ok := v.(string); ok && t != "" {
			*dst = t
		}
	}
}

func (s *Settings) list(key string, dst *[]string) {
	v, ok := s.Props[key]
	if !ok {
		return
	}
	switch v := v.(type) {
	case string:
		*dst = lines(v)
	case []interface{}:
		*dst = nil
		for _, x := range v {
			*dst = append(*dst, fmt.Sprint(x))
		}
	}
}

// Resolved folds props: into the fields, which is where Unmarshal leaves them.
func (s *Settings) Resolved() Settings {
	r := *s
	r.str("author", &r.Author)
	r.str("penName", &r.PenName)
	r.str("email", &r.Email)
	r.str("phone", &r.Phone)
	r.str("website", &r.Website)
	r.str("style", &r.Style)
	r.str("paper", &r.Paper)
	r.str("theme", &r.Theme)
	r.list("address", &r.Address)
	r.list("agent", &r.Agent)
	return r
}

// Defaults are the book's metadata before the file has its say.
func (s *Settings) Defaults() Meta {
	r := s.Resolved()
	return Meta{Author: r.Author, Byline: r.PenName, Email: r.Email, Phone: r.Phone, Website: r.Website,
		Address: r.Address, Agent: r.Agent}
}

// Read builds the book from an org file.
func Read(doc *org.Document, path string, defaults Meta) *Book {
	scenes := true
	if v := strings.ToLower(strings.TrimSpace(doc.Get("MANUSCRIPT_SCENES"))); v == "nil" || v == "no" || v == "false" || v == "headings" {
		scenes = false
	}
	return Build(doc, path, defaults, Options{Scenes: scenes})
}

// ----------------------------------------------------------------------------
// The exporters: manuscript (docx) and epub
// ----------------------------------------------------------------------------

type DocxExporter struct {
	Settings `yaml:",inline"`
}

type EpubExporter struct {
	Settings `yaml:",inline"`
}

func load(db common.ODb, query string) (*org.Document, error) {
	d := db.FindByFile(query)
	if d == nil {
		return nil, fmt.Errorf("no file called %q", query)
	}
	return d, nil
}

func pick(props map[string]string, key, fallback string) string {
	if v := props[key]; v != "" {
		return v
	}
	return fallback
}

func (self *DocxExporter) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *DocxExporter) Startup(m *common.PluginManager, o *common.PluginOpts) {}

// Bytes is the manuscript of a file.
func (self *DocxExporter) Bytes(doc *org.Document, props map[string]string) ([]byte, error) {
	r := self.Resolved()
	b := Read(doc, doc.Path, self.Defaults())
	return WriteDocx(b, pick(props, "style", r.Style), pick(props, "paper", r.Paper), time.Now())
}

func (self *DocxExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	f, err := load(db, query)
	if err != nil {
		return err
	}
	b, err := self.Bytes(f, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

func (self *DocxExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	return fmt.Errorf("a manuscript is a docx, not text: ask GET /book?filename=...&to=docx"), ""
}

func (self *EpubExporter) Unmarshal(u func(interface{}) error) error { return u(self) }
func (self *EpubExporter) Startup(m *common.PluginManager, o *common.PluginOpts) {}

// Bytes is the ebook of a file, and anything left out of it.
func (self *EpubExporter) Bytes(doc *org.Document, props map[string]string) ([]byte, []string, error) {
	r := self.Resolved()
	theme := pick(props, "theme", r.Theme)
	if v := strings.TrimSpace(doc.Get("EPUB_THEME")); v != "" && props["theme"] == "" {
		theme = v
	}
	css, err := EpubThemeCSS(theme)
	if err != nil {
		return nil, nil, err
	}
	b := Read(doc, doc.Path, self.Defaults())
	return WriteEpub(b, css, time.Now())
}

func (self *EpubExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	f, err := load(db, query)
	if err != nil {
		return err
	}
	b, _, err := self.Bytes(f, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}

func (self *EpubExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	return fmt.Errorf("an epub is a zip, not text: ask GET /book?filename=...&to=epub"), ""
}

func init() {
	common.AddExporter("manuscript", func() common.Exporter { return &DocxExporter{} })
	common.AddExporter("epub", func() common.Exporter { return &EpubExporter{} })
}

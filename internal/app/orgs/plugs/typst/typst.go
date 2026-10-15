// EXPORTER: Typst Export

/* SDOC: Exporters

* Typst
  The typst exporter writes an org file as a [[https://typst.app][Typst]]
  document: a modern typesetting language that compiles to pdf in a fraction
  of a second, with no LaTeX distribution to install. It needs nothing
  configured.

	#+BEGIN_SRC sh
  orgs export -f typst notes.org -out notes.typ      # the .typ, to edit or compile yourself
  orgs export -f typst-pdf notes.org -out notes.pdf  # compiled by the server (needs typst there)
  orgs export -f typst notes.org -theme report       # pick a template
	#+END_SRC

  The yaml is optional; with it the defaults change for every file:

	#+BEGIN_SRC yaml
    exporters:
      - name: "typst"
        template: "article"   # article (default), report, notes, or one of your own
        paper: "a4"           # a4 (default), us-letter, a5, ...
        math: "mitex"         # mitex (default) or typst
        imageWidth: "80%"     # pictures with no width of their own
        typst: "typst"        # the compiler, for typst-pdf
	#+END_SRC

** Per file settings

	#+BEGIN_SRC org
    #+TITLE: Field notes
    #+SUBTITLE: Spring survey
    #+AUTHOR: Jane Roe
    #+DATE: 2026-10-14
    #+TYPST_TEMPLATE: report
    #+TYPST_PAPER: us-letter
    #+TYPST_MATH: typst
    #+TYPST_HEADER: #set text(font: "Inter")
    #+OPTIONS: toc:2 num:nil H:3 todo:t tags:nil p:t
	#+END_SRC

  - =#+TITLE=, =#+SUBTITLE=, =#+AUTHOR= (several with "and" or commas),
    =#+DATE=, =#+EMAIL=, =#+LANGUAGE=, =#+DESCRIPTION= and =#+KEYWORDS= go into
    the title block and the pdf's own metadata.
  - =#+TYPST_HEADER:= lines go into the preamble, after the template and before
    the document starts, so a =#set= or =#show= there restyles everything.
  - =#+TYPST:= lines and =#+BEGIN_EXPORT typst= blocks are written as they are,
    where they are.
  - =#+OPTIONS:= as org reads them: =toc= (=nil=, =t= or a depth), =num=
    (numbered headings, or to a depth), =H= (deeper headings become bold
    lines), =todo=, =pri=, =tags=, =p= (planning lines), =c= (clocks), =prop=
    (property drawers), =d= (drawers), =f= (footnotes), =<= (timestamps), =e=
    (entities like =\alpha=), =arch=.

** Pictures, tables and figures
   A picture on its own line is centred. =#+CAPTION:= and =#+NAME:= make it -
   or a table, or a source block - a numbered figure that =[[name]]= links to.
   =#+ATTR_TYPST: :width 60%= sets a size (=#+ATTR_LATEX: :width 0.6\textwidth=
   is read too). Put =#+ATTR_TYPST:= *before* =#+CAPTION:=: go-org drops a
   caption that is followed by a keyword it does not know.

   Tables keep their header row (everything above the first rule) and their
   =<l>= =<c>= =<r>= cookies; a column of numbers with no cookie is right
   aligned.

** Maths
   LaTeX fragments (=$x^2$=, =\(...\)=, =\[...\]=, =\begin{align}=) are handed
   to the [[https://typst.app/universe/package/mitex][mitex]] package, which
   reads LaTeX maths; typst fetches it on the first compile.
   =#+TYPST_MATH: typst= writes plain typst maths instead wherever the formula is
   simple enough to translate (letters, fractions, roots, scripts, Greek, the
   usual operators), and still uses mitex for the rest.

** Templates
   A template is one =.typ= file in =typst/= in a template folder, so a file of
   your own (=templatePath/typst/thesis.typ=) is a template called =thesis=. It
   defines =org-template= taking =title=, =subtitle=, =authors=, =date=,
   =email=, =lang=, =paper=, =toc=, =toc-depth=, =numbered=, =num-depth=,
   =keywords=, =description= and the body; it may redefine any =org-*= helper
   (=org-todo=, =org-tags=, =org-special=...) to dress those parts. The three
   built in:

   - =article= - a title block on the first page, numbered sections.
   - =report= - a title page, a contents page, chapters on new pages.
   - =notes= - compact, small margins, two columns with =#+TYPST_COLUMNS: 2=.

EDOC */

package typst

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/common"
)

// MitexImport is the line a document with LaTeX maths starts with.
const MitexImport = `#import "@preview/mitex:0.2.7": mi, mitex`

type Exporter struct {
	Template   string `yaml:"template"`
	Paper      string `yaml:"paper"`
	Math       string `yaml:"math"`
	ImageWidth string `yaml:"imageWidth"`
	// The compiler for typst-pdf; "typst" on the PATH when empty.
	Typst string                 `yaml:"typst"`
	Props map[string]interface{} `yaml:"props"`

	pm *common.PluginManager
}

// What one document is written with. Kept apart from the exporter, which is
// shared between requests, so one file's settings never stay for the next.
type settings struct {
	template   string
	paper      string
	math       string
	imageWidth string
	columns    int
}

func (e *Exporter) settingsFor(doc *org.Document, props map[string]string) settings {
	first := func(vs ...string) string {
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
		return ""
	}
	paper := first(props["paper"], doc.Get("TYPST_PAPER"), e.Paper)
	if paper == "" {
		// A file written for LaTeX says its paper there.
		opts := doc.Get("LATEX_CLASS_OPTIONS")
		switch {
		case strings.Contains(opts, "letterpaper"):
			paper = "us-letter"
		case strings.Contains(opts, "a5paper"):
			paper = "a5"
		default:
			paper = "a4"
		}
	}
	switch strings.ToLower(paper) {
	case "letter", "usletter":
		paper = "us-letter"
	case "legal", "uslegal":
		paper = "us-legal"
	}
	math := strings.ToLower(first(props["math"], doc.Get("TYPST_MATH"), e.Math, "mitex"))
	if math != "typst" {
		math = "mitex"
	}
	cols, _ := strconv.Atoi(first(props["columns"], doc.Get("TYPST_COLUMNS")))
	return settings{
		template:   first(props["template"], props["theme"], doc.Get("TYPST_TEMPLATE"), e.Template, "article"),
		paper:      strings.ToLower(paper),
		math:       math,
		imageWidth: width(first(doc.Get("TYPST_IMAGE_WIDTH"), e.ImageWidth)),
		columns:    cols,
	}
}

func (e *Exporter) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(e) }
func (e *Exporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	e.pm = manager
}

// Standalone: the exporter needs nothing configured.
func (e *Exporter) Standalone() bool { return true }

func (e *Exporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, s := e.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(s), 0644)
}

func (e *Exporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	doc := db.FindByFile(query)
	if doc == nil {
		return fmt.Errorf("typst: no file named %s", query), ""
	}
	src, _, err := e.Document(doc, props, nil)
	if err != nil {
		return err, ""
	}
	return nil, src
}

// Templates lists the templates: every typst/*.typ in a template folder.
func Templates() []string {
	out := []string{}
	for _, n := range assets.GlobTemplates("typst/*.typ") {
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(n, "typst/"), ".typ"))
	}
	return out
}

var templateNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// templateText is a template's source. An unknown name is the article, so a
// typo still gives a document; the article itself is built in as a last resort.
func templateText(name string) (string, string) {
	for _, n := range []string{name, "article"} {
		if !templateNameRe.MatchString(n) {
			continue
		}
		if p := assets.FindTemplate("typst/" + n + ".typ"); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				return string(b), n
			}
		}
	}
	return fallbackTemplate, "builtin"
}

// Document writes a whole typst document: the preamble, the template, the
// title block's settings and the body. imagePath, when given, decides where
// each picture link points (the pdf build copies them).
func (e *Exporter) Document(doc *org.Document, props map[string]string, imagePath func(string) string) (string, []string, error) {
	if props == nil {
		props = map[string]string{}
	}
	set := e.settingsFor(doc, props)
	w := newWriter(doc, set)
	if imagePath != nil {
		w.ImagePath = imagePath
	}
	org.WriteNodes(w, doc.Nodes...)
	body := strings.TrimSpace(w.String())

	tpl, used := templateText(set.template)
	out := strings.Builder{}
	name := filepath.Base(doc.Path)
	fmt.Fprintf(&out, "// %s, exported by orgs with the %q template.\n", name, used)
	if w.usesMitex {
		out.WriteString(MitexImport + "\n")
	}
	out.WriteString("\n" + prelude + "\n")
	out.WriteString(strings.TrimSpace(tpl) + "\n\n")
	for _, l := range strings.Split(doc.Get("TYPST_HEADER"), "\n") {
		if strings.TrimSpace(l) != "" {
			out.WriteString(l + "\n")
		}
	}
	out.WriteString(e.showRule(doc, set, w))
	out.WriteString("\n")
	out.WriteString(body)
	out.WriteString("\n")
	return out.String(), w.Images, nil
}

// The #show line that hands the document to the template, with what the
// file's keywords say.
func (e *Exporter) showRule(doc *org.Document, set settings, w *Writer) string {
	args := []string{}
	content := func(key string) string {
		v := strings.TrimSpace(doc.Get(key))
		if v == "" {
			return "none"
		}
		return "[" + w.inner(inlineOf(doc, v)...) + "]"
	}
	title := strings.TrimSpace(doc.Get("TITLE"))
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(doc.Path), filepath.Ext(doc.Path))
		args = append(args, "title: ["+escapeMarkup(title, true)+"]")
	} else {
		args = append(args, "title: "+content("TITLE"))
	}
	args = append(args, "title-text: "+str(plainOf(doc, title)))
	args = append(args, "subtitle: "+content("SUBTITLE"))
	authors := []string{}
	for _, a := range splitAuthors(doc.Get("AUTHOR")) {
		authors = append(authors, str(a))
	}
	args = append(args, "authors: ("+strings.Join(authors, ", ")+func() string {
		if len(authors) == 1 {
			return ","
		}
		return ""
	}()+")")
	date := strings.Trim(strings.TrimSpace(doc.Get("DATE")), "<>[]")
	if date == "" {
		args = append(args, "date: none")
	} else {
		args = append(args, "date: ["+escapeMarkup(date, true)+"]")
	}
	args = append(args, "email: "+strOrNone(doc.Get("EMAIL")))
	lang := strings.ToLower(strings.TrimSpace(doc.Get("LANGUAGE")))
	if len(lang) < 2 {
		lang = "en"
	}
	args = append(args, "lang: "+str(lang[:2]))
	args = append(args, "paper: "+str(set.paper))
	args = append(args, "description: "+strOrNone(doc.Get("DESCRIPTION")))
	kws := []string{}
	for _, k := range strings.FieldsFunc(doc.Get("KEYWORDS"), func(r rune) bool { return r == ',' || r == ';' }) {
		if k = strings.TrimSpace(k); k != "" {
			kws = append(kws, str(k))
		}
	}
	args = append(args, "keywords: ("+strings.Join(kws, ", ")+func() string {
		if len(kws) == 1 {
			return ","
		}
		return ""
	}()+")")

	toc := option(doc, "toc")
	switch {
	case toc == "nil":
		args = append(args, "toc: false")
	case toc == "" || toc == "t":
		args = append(args, "toc: true")
	default:
		if n, err := strconv.Atoi(toc); err == nil && n > 0 {
			args = append(args, "toc: true", fmt.Sprintf("toc-depth: %d", n))
		} else {
			args = append(args, "toc: true")
		}
	}
	num := option(doc, "num")
	switch {
	case num == "nil":
		args = append(args, "numbered: false")
	case num == "" || num == "t":
		args = append(args, "numbered: true")
	default:
		if n, err := strconv.Atoi(num); err == nil && n > 0 {
			args = append(args, "numbered: true", fmt.Sprintf("num-depth: %d", n))
		} else {
			args = append(args, "numbered: true")
		}
	}
	if set.columns > 1 {
		args = append(args, fmt.Sprintf("columns: %d", set.columns))
	}
	return "#show: org-template.with(\n  " + strings.Join(args, ",\n  ") + ",\n)\n"
}

func strOrNone(s string) string {
	if s = strings.TrimSpace(s); s == "" {
		return "none"
	}
	return str(s)
}

// plainOf is a keyword's text without its markup, for the pdf's metadata.
func plainOf(doc *org.Document, s string) string {
	re := regexp.MustCompile(`\[\[(?:[^\]]*\]\[)?([^\]]*)\]\]`)
	s = re.ReplaceAllString(s, "$1")
	return strings.NewReplacer("*", "", "/", "", "=", "", "~", "", "+", "").Replace(s)
}

// #+AUTHOR: Jane Roe and John Doe, or Jane Roe, John Doe.
func splitAuthors(s string) []string {
	out := []string{}
	for _, part := range regexp.MustCompile(`\s+and\s+|,|;`).Split(s, -1) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Compiling
// ---------------------------------------------------------------------------

// Compiler is the typst binary, or an error saying it is not installed.
func (e *Exporter) Compiler() (string, error) {
	bin := strings.TrimSpace(e.Typst)
	if bin == "" {
		bin = "typst"
	}
	p, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("typst is not installed on the server (looked for %q): install it from https://typst.app or your package manager (brew install typst, cargo install typst-cli), or set the typst exporter's typst: to its path", bin)
	}
	return p, nil
}

// PDF compiles doc in a scratch folder - the .typ and a copy of each picture -
// and answers the pdf's bytes.
func (e *Exporter) PDF(doc *org.Document, props map[string]string) ([]byte, error) {
	bin, err := e.Compiler()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "orgs-typst-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	base := filepath.Dir(doc.Path)
	copied := map[string]string{}
	n := 0
	imagePath := func(target string) string {
		if p, ok := copied[target]; ok {
			return p
		}
		src := target
		if !filepath.IsAbs(src) {
			src = filepath.Join(base, src)
		}
		if strings.HasPrefix(src, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				src = filepath.Join(home, src[2:])
			}
		}
		n++
		name := fmt.Sprintf("img/%d-%s", n, filepath.Base(src))
		if b, err := os.ReadFile(src); err == nil {
			os.MkdirAll(filepath.Join(dir, "img"), 0o755)
			os.WriteFile(filepath.Join(dir, name), b, 0o644)
		}
		// A picture that is not there is still pointed at, so typst names it
		// in its error rather than the document losing it silently.
		copied[target] = name
		return name
	}
	src, _, err := e.Document(doc, props, imagePath)
	if err != nil {
		return nil, err
	}
	in := filepath.Join(dir, "doc.typ")
	if err := os.WriteFile(in, []byte(src), 0o644); err != nil {
		return nil, err
	}
	out := filepath.Join(dir, "doc.pdf")
	cmd := exec.Command(bin, "compile", "--root", dir, in, out)
	cmd.Dir = dir
	msg, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("typst could not compile %s:\n%s", filepath.Base(doc.Path), strings.TrimSpace(string(msg)))
	}
	return os.ReadFile(out)
}

func init() {
	common.AddExporter("typst", func() common.Exporter { return &Exporter{} })
}

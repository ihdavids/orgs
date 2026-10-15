package export

// Run a file through an exporter.
//
//	orgs export -f html notes.org -out notes.html
//	orgs export -f html notes.org -theme docs -stdout
//	orgs export -f pdf adventure.org -out adventure.pdf
//	orgs export -f html docs.org -out docs/docs.html -rel   pictures as images/x.png
//	orgs export -f mermaid -query 'IsTask()' -out plan.mmd
//	orgs export -list                    what this server can export to
//	orgs export -list-themes             what the html exporter can wear
//	orgs export -f manuscript story.org  a docx in manuscript format (-style novel)
//	orgs export -f epub novel.org        an ebook (-theme literary)
//
// Two things to know before changing anything here.
//
// The **query** is the exporter's argument, not a filter this command applies:
// a file exporter reads it as the file to export and a query exporter (gantt,
// mermaid) reads it as the expression to select headings with. That is why it
// can be written as a bare word after the flags - for the exporters people
// reach for most, it is a filename.
//
// Whether the **server** or **this machine** writes the answer is `-local`,
// and the default changed. It used to be the server, which is right for a
// server and a client on the same machine and surprising everywhere else:
// `orgs -url https://box:8010 export -out ./notes.html` wrote notes.html on
// box. Now the bytes come back and are written here unless -local is asked
// for, and -local says out loud that the path is the server's.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	sitecmd "github.com/ihdavids/orgs/cmd/oc/commands/site"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/pandoc"
	"github.com/ihdavids/orgs/internal/common"
)

type Export struct {
	fset *flag.FlagSet

	Filename   string
	Query      string
	Format     string
	Theme      string
	Parent     string
	Local      bool
	Stdout     bool
	Printable  bool
	Refresh    bool
	NoRefresh  bool
	FileLinks  bool
	HttpsLinks bool
	Rel        bool
	List       bool
	ListThemes bool
	To         string
	Style      string
	Paper      string
}

func (self *Export) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Export) StartPlugin(manager *common.PluginManager)         {}

func (self *Export) SetupParameters(fset *flag.FlagSet) {
	self.fset = fset
	fset.StringVar(&self.Filename, "out", "", "write the answer here (default: stdout)")
	fset.StringVar(&self.Query, "query", "", "the file to export, or the expression to select headings")
	fset.StringVar(&self.Format, "f", "html", "html, latex, pdf, revealjs, impressjs, mermaid, gantt, tangle, dndsheet …")
	fset.StringVar(&self.Theme, "theme", "", "html theme to render with (see -list-themes)")
	fset.StringVar(&self.Parent, "parent", "", "parent identifier, for the exporters that take one")
	fset.BoolVar(&self.Local, "local", false, "let the server write the file, at -out on its own disk")
	fset.BoolVar(&self.Stdout, "stdout", false, "print the answer even when -out was given")
	fset.BoolVar(&self.Printable, "printable", false, "the printable variant, where the exporter has one")
	fset.BoolVar(&self.Refresh, "refresh", false, "build again rather than answering from the pdf cache")
	fset.BoolVar(&self.NoRefresh, "no-refresh", false, "export the file as it stands, without refreshing its dynamic blocks and formulas first")
	fset.BoolVar(&self.FileLinks, "filelinks", false, "write links as file:// urls")
	fset.BoolVar(&self.HttpsLinks, "httpslinks", false, "write links as https urls to this server")
	fset.BoolVar(&self.Rel, "rel", false, "write image links relative to the -out file, for a page published beside its pictures")
	fset.BoolVar(&self.List, "list", false, "what this server can export to")
	fset.BoolVar(&self.ListThemes, "list-themes", false, "what the html exporter can wear")
	fset.StringVar(&self.To, "to", "", "with -f pandoc: the format pandoc writes (docx, odt, epub, rst, ...)")
	fset.StringVar(&self.Style, "style", "", "with -f manuscript: modern, shunn, novel or reader")
	fset.StringVar(&self.Paper, "paper", "", "with -f manuscript: letter or a4; with -f typst-pdf: a4 or us-letter")
}

func (self *Export) Exec(core *commands.Core) {
	if free := commands.FreeText(self.fset); free != "" && self.Query == "" {
		self.Query = free
	}
	// A site is many files, so it is /site's zip rather than an exporter's
	// string; orgs site is the command, and this is the same thing.
	if strings.EqualFold(self.Format, "site") {
		if self.ListThemes {
			sitecmd.ListThemes(core)
			return
		}
		sitecmd.Build(core, sitecmd.Options{Dir: self.Query, Out: self.Filename, Theme: self.Theme, Local: self.Local})
		return
	}
	switch {
	case self.List:
		self.list(core)
		return
	case self.ListThemes && strings.HasPrefix(strings.ToLower(self.Format), "typst"):
		self.typstTemplates(core)
		return
	case self.ListThemes && self.bookFormat() != "":
		self.bookThemes(core)
		return
	case self.ListThemes:
		self.themes(core)
		return
	}
	if strings.TrimSpace(self.Query) == "" {
		commands.Fail("orgs export: nothing to export.\n\n%s", usage)
	}
	if strings.EqualFold(self.Format, "pdf") {
		self.pdf(core)
		return
	}
	if to := self.bookFormat(); to != "" {
		self.book(core, to)
		return
	}
	if f := strings.ToLower(self.Format); f == "beamer-pdf" || f == "beamerpdf" {
		self.beamerPdf(core)
		return
	}
	if f := strings.ToLower(self.Format); f == "typst-pdf" || f == "typstpdf" {
		self.typstPdf(core)
		return
	}
	if to := self.pandocFormat(); to != "" {
		self.pandoc(core, to)
		return
	}
	self.run(core)
}

// ---------------------------------------------------------------------------

func (self *Export) list(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[struct {
		Ok    bool
		Names []string
	}](core, "exporters", nil)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	// pdf is not an exporter plugin from a client's point of view - it is its
	// own endpoint, because the answer is bytes and an exporter answers with a
	// string. It is a format you can ask for, so it belongs in the list.
	names := append([]string{}, res.Names...)
	if !has(names, "pdf") {
		names = append(names, "pdf")
	}
	commands.Render(names, func() {
		for _, n := range names {
			fmt.Println(n)
		}
	})
}

func (self *Export) themes(core *commands.Core) {
	// Each format with themes of its own answers for them; html is the rest.
	path := "html/themes"
	switch strings.ToLower(self.Format) {
	case "beamer", "beamer-pdf", "beamerpdf":
		path = "beamer/themes"
	}
	res, err := commands.SendReceiveGetErr[struct {
		Ok     bool
		Themes []string
	}](core, path, nil)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	commands.Render(res.Themes, func() {
		for _, t := range res.Themes {
			fmt.Println(t)
		}
	})
}

// ---------------------------------------------------------------------------

func (self *Export) run(core *commands.Core) {
	ps := map[string]string{
		"query":  self.Query,
		"parent": self.Parent,
		"theme":  self.Theme,
	}
	if self.Printable {
		ps["printable"] = "t"
	}
	if self.FileLinks {
		ps["filelinks"] = "t"
	}
	if self.HttpsLinks {
		ps["httpslinks"] = "t"
	}
	if self.Rel {
		ps["relto"] = self.relTo()
	}
	if self.NoRefresh {
		ps["refresh"] = "f"
	}

	if self.Local {
		if self.Filename == "" {
			commands.Fail("orgs export -local: -out says where on the server to write it")
		}
		ps["local"] = "t"
		ps["filename"] = self.Filename
		if commands.Wrote(fmt.Sprintf("export %s → %s on the server", self.Query, self.Filename), ps) {
			return
		}
		res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "file/"+self.Format, ps)
		if err != nil {
			commands.Fail("orgs export: %v", err)
		}
		if !res.Ok {
			commands.Fail("orgs export: %s", strings.TrimSpace(res.Msg))
		}
		commands.RenderOne(res, func() {
			fmt.Fprintf(os.Stderr, "wrote %s on the server\n", self.Filename)
		})
		return
	}

	ps["local"] = "f"
	res, err := commands.SendReceiveGetErr[common.ResultMsg](core, "file/"+self.Format, ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if !res.Ok {
		// The exporter puts its refusal in the same field it puts the answer
		// in, so this is the only place the two are told apart.
		commands.Fail("orgs export: %s", strings.TrimSpace(res.Msg))
	}
	self.deliver([]byte(res.Msg))
}

// The directory -rel makes links relative to: the one -out writes into. With
// -local that is the server's path and the server resolves it; otherwise the
// file is written here, so it is made absolute here - which is only the same
// place on the server's disk when the server is on this machine.
func (self *Export) relTo() string {
	if self.Filename == "" {
		commands.Fail("orgs export -rel: -out says what the links are relative to")
	}
	dir := filepath.Dir(self.Filename)
	if self.Local {
		return dir
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		commands.Fail("orgs export -rel: %v", err)
	}
	return abs
}

// A pdf is the one format that is not an exporter call: the pdf exporter
// writes a file and returns nothing to a string, so /pdf is its own endpoint
// answering with the bytes. It is cached against what the file was when it was
// built - path, modification time and size - so asking twice builds once.
func (self *Export) pdf(core *commands.Core) {
	ps := map[string]string{"filename": self.Query}
	if self.Refresh {
		ps["refresh"] = "t"
	}
	body, ctype, status, err := core.Rest.GetRaw("pdf", ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if status >= 400 || !strings.Contains(ctype, "pdf") {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		commands.Fail("orgs export -f pdf: %s", msg)
	}
	self.deliver(body)
}

// The formats that are pandoc's rather than an exporter of orgs' own. typst
// left this list when orgs got its own typst exporter; pandoc's is still
// -f pandoc -to typst.
var pandocFormats = map[string]bool{
	"docx": true, "word": true, "odt": true, "epub2": true, "pptx": true,
	"rtf": true, "rst": true, "mediawiki": true, "wiki": true, "asciidoc": true, "adoc": true,
	"plain": true, "txt": true, "textile": true, "dokuwiki": true, "jira": true, "man": true,
	"docbook": true, "icml": true, "opml": true, "commonmark": true, "gfm": true,
	"ipynb": true, "texinfo": true, "fb2": true,
}

// What pandoc should write, or "" when this export is not pandoc's.
func (self *Export) pandocFormat() string {
	f := strings.ToLower(self.Format)
	if f == "pandoc" {
		if self.To == "" {
			commands.Fail("orgs export -f pandoc: say which format with -to (docx, odt, epub, rst, ...)")
		}
		return self.To
	}
	if pandocFormats[f] {
		return f
	}
	return ""
}

// pandoc answers with bytes, as a pdf does, so it has its own endpoint.
func (self *Export) pandoc(core *commands.Core, to string) {
	if self.Local {
		commands.Fail("orgs export -f %s: -local is not supported for pandoc formats; -out writes here", to)
	}
	if self.Filename == "" && pandoc.IsBinary(to) {
		// A docx on a terminal is noise; one named after the file is what was meant.
		base := filepath.Base(self.Query)
		self.Filename = strings.TrimSuffix(base, filepath.Ext(base)) + "." + pandoc.Extension(to)
	}
	body, _, status, err := core.Rest.GetRaw("pandoc", map[string]string{"filename": self.Query, "to": to})
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if status >= 400 {
		commands.Fail("orgs export -f %s: %s", to, strings.TrimSpace(string(body)))
	}
	self.deliver(body)
}

// The formats orgs writes itself (/book): a manuscript and an ebook. epub was
// pandoc's; pandoc's is still -f pandoc -to epub.
func (self *Export) bookFormat() string {
	switch strings.ToLower(self.Format) {
	case "manuscript", "ms":
		return "docx"
	case "epub", "epub3", "ebook":
		return "epub"
	}
	return ""
}

func (self *Export) bookThemes(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[struct {
		Ok     bool
		Styles []string
		Themes []string
	}](core, "book", map[string]string{"list": "t"})
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	names := res.Themes
	if self.bookFormat() == "docx" {
		names = res.Styles
	}
	commands.Render(names, func() {
		for _, n := range names {
			fmt.Println(n)
		}
	})
}

func (self *Export) book(core *commands.Core, to string) {
	if self.Local {
		commands.Fail("orgs export -f %s: -local is not supported here; -out writes here", self.Format)
	}
	if self.Filename == "" {
		base := filepath.Base(self.Query)
		self.Filename = strings.TrimSuffix(base, filepath.Ext(base)) + "." + to
	}
	ps := map[string]string{"filename": self.Query, "to": to, "style": self.Style, "paper": self.Paper, "theme": self.Theme}
	body, h, status, err := core.Rest.GetRawHeader("book", ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if status >= 400 {
		commands.Fail("orgs export -f %s: %s", self.Format, strings.TrimSpace(string(body)))
	}
	for _, w := range h.Values("X-Orgs-Warning") {
		fmt.Fprintf(os.Stderr, "%s\n", w)
	}
	if n := h.Get("X-Orgs-Words"); n != "" {
		fmt.Fprintf(os.Stderr, "%s words\n", n)
	}
	self.deliver(body)
}

// A beamer deck compiled to pdf: bytes, so its own endpoint, as /pdf is.
func (self *Export) beamerPdf(core *commands.Core) {
	if self.Local {
		commands.Fail("orgs export -f beamer-pdf: -local is not supported here; -out writes here")
	}
	if self.Filename == "" {
		base := filepath.Base(self.Query)
		self.Filename = strings.TrimSuffix(base, filepath.Ext(base)) + ".pdf"
	}
	ps := map[string]string{"filename": self.Query, "theme": self.Theme}
	if self.Refresh {
		ps["refresh"] = "t"
	}
	body, ctype, status, err := core.Rest.GetRaw("beamer/pdf", ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if status >= 400 || !strings.Contains(ctype, "pdf") {
		commands.Fail("orgs export -f beamer-pdf: %s", strings.TrimSpace(string(body)))
	}
	self.deliver(body)
}

// typst-pdf is compiled on the server (/typst/pdf), which answers with bytes
// as /pdf does.
func (self *Export) typstPdf(core *commands.Core) {
	if self.Local {
		commands.Fail("orgs export -f typst-pdf: -local is not supported; -out writes here")
	}
	if self.Filename == "" {
		base := filepath.Base(self.Query)
		self.Filename = strings.TrimSuffix(base, filepath.Ext(base)) + ".pdf"
	}
	ps := map[string]string{"filename": self.Query, "template": self.Theme, "paper": self.Paper}
	if self.Refresh {
		ps["refresh"] = "t"
	} else if self.NoRefresh {
		ps["refresh"] = "f"
	}
	body, ctype, status, err := core.Rest.GetRaw("typst/pdf", ps)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	if status >= 400 || !strings.Contains(ctype, "pdf") {
		commands.Fail("orgs export -f typst-pdf: %s", strings.TrimSpace(string(body)))
	}
	self.deliver(body)
}

func (self *Export) typstTemplates(core *commands.Core) {
	res, err := commands.SendReceiveGetErr[struct {
		Ok        bool
		Templates []string
	}](core, "typst/templates", nil)
	if err != nil {
		commands.Fail("orgs export: %v", err)
	}
	commands.Render(res.Templates, func() {
		for _, t := range res.Templates {
			fmt.Println(t)
		}
	})
}

// Where the bytes end up: a file when -out was given, stdout otherwise, and
// both when -stdout was asked for as well.
func (self *Export) deliver(body []byte) {
	if self.Filename == "" || self.Stdout {
		os.Stdout.Write(body)
		if len(body) > 0 && body[len(body)-1] != '\n' {
			fmt.Println()
		}
		if self.Filename == "" {
			return
		}
	}
	if commands.Wrote(fmt.Sprintf("write %d bytes to %s", len(body), self.Filename), nil) {
		return
	}
	if dir := filepath.Dir(self.Filename); dir != "." {
		os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(self.Filename, body, 0644); err != nil {
		commands.Fail("orgs export: could not write %s: %v", self.Filename, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", self.Filename, len(body))
}

func has(vs []string, want string) bool {
	for _, v := range vs {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

const usage = `  orgs export -f html notes.org -out notes.html
  orgs export -f html notes.org -theme docs        (to stdout)
  orgs export -f html docs.org -out docs/docs.html -rel   (links relative to docs/)
  orgs export -f pdf  adventure.org -out book.pdf
  orgs export -f manuscript story.org              (a docx: -style modern, shunn, novel, reader)
  orgs export -f epub novel.org                    (an ebook: -theme classic, literary, modern, plain)
  orgs export -f docx notes.org                    (pandoc: docx, odt, rst, ...)
  orgs export -f mermaid -query 'IsTask()' -out plan.mmd
  orgs export -f beamer talk.org -out talk.tex     (a LaTeX Beamer deck; -f beamer-pdf compiles it)
  orgs export -f typst notes.org -out notes.typ -theme report   (-f typst-pdf compiles it: article, report, notes)
  orgs export -f todotxt inbox.org -out todo.txt   (tasks as todo.txt; a query works too)
  orgs export -f ics 'HasDeadline()' -out due.ics  (events and to-dos for a calendar)
  orgs export -f site notes/blog -out public -theme paper   (a static web site; see orgs site)
  orgs export -list          what this server can export to
  orgs export -list-themes   what the html exporter can wear

-out writes here. -local writes on the server instead, at that path on its
own disk.`

func init() {
	commands.AddCmd("export", "run a file or a query through an exporter",
		func() commands.Cmd { return &Export{Format: "html"} })
}

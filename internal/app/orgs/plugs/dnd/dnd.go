// EXPORTER: Dungeons & Dragons character sheets
package dnd

/* SDOC: Exporters

* Dnd Character Sheets

  The dnd module turns an org mode character sheet into a printable character
  sheet. Three exporters are registered:

  | name       | output                                                  |
  |------------+---------------------------------------------------------|
  | dndsheet   | a styled html character sheet (the default)             |
  | dndlatex   | a .tex file laid out like an official character sheet   |
  | dndpdf     | the same sheet run through pdflatex                     |

  Enable them in your orgs.yaml:

  #+BEGIN_SRC yaml
  server:
    exporters:
      - name: "dndsheet"
      - name: "dndlatex"
      - name: "dndpdf"
        props:
          pdflatex: "/Library/TeX/texbin/pdflatex"
    dndPaths:
      - "~/dnd/homebrew"
  #+END_SRC

  Then export a sheet from the command line:

  #+BEGIN_SRC bash
  orgs dnd sheet -file lyra.org -format pdf -out lyra.pdf
  # or through the generic export command
  orgs export -f dndsheet -query lyra.org -out lyra.html -l t
  #+END_SRC

  The templates used are =dnd_character_html.tpl= and =dnd_character.tpl= in
  your template folder. Both are ordinary pongo2 templates and receive the
  fully computed sheet, so you can restyle them without touching any go code.

EDOC */

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/internal/common/dnd"
	"gopkg.in/op/go-logging.v1"
)

// ----------------------------------------------------------------------------
// Ruleset library, shared by the exporters and the REST endpoints
// ----------------------------------------------------------------------------

var (
	libLock  sync.Mutex
	library  *dnd.Library
	libPaths []string
)

// SetPaths configures where ruleset modules are loaded from. Call before the
// first use of Library, additional calls force a reload.
func SetPaths(paths []string) {
	libLock.Lock()
	defer libLock.Unlock()
	libPaths = append([]string{}, paths...)
	library = nil
}

// Paths reports the ruleset module search path.
func Paths() []string {
	libLock.Lock()
	defer libLock.Unlock()
	return append([]string{}, libPaths...)
}

// Library returns the shared ruleset library, loading it on first use.
func Library() *dnd.Library {
	libLock.Lock()
	defer libLock.Unlock()
	if library == nil {
		library = dnd.NewLibrary(libPaths)
	}
	return library
}

// Reload throws away the cached library so the next call re-reads the yaml.
func Reload() *dnd.Library {
	libLock.Lock()
	library = nil
	libLock.Unlock()
	return Library()
}

// DefaultPaths is the search path used when nothing is configured.
func DefaultPaths(templatePath string) []string {
	paths := []string{}
	if templatePath != "" {
		paths = append(paths, filepath.Join(templatePath, "dnd"))
	}
	paths = append(paths, "./templates/dnd")
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".orgs", "dnd"))
	}
	out := []string{}
	for _, p := range paths {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if !containsPath(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func containsPath(list []string, v string) bool {
	for _, p := range list {
		if p == v {
			return true
		}
	}
	return false
}

// LoadCharacter reads a character sheet out of the org database.
func LoadCharacter(db common.ODb, filename string) (*dnd.Character, *dnd.Ruleset, error) {
	f := db.GetFile(filename)
	path := filename
	if f != nil && f.Filename != "" {
		path = f.Filename
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read character sheet %s: %s", filename, err)
	}
	return ParseCharacter(string(data))
}

// ParseCharacter parses sheet text and resolves the ruleset it names.
func ParseCharacter(text string) (*dnd.Character, *dnd.Ruleset, error) {
	// A first pass with no ruleset tells us which ruleset the sheet wants.
	first, err := dnd.ParseOrg(text, nil)
	if err != nil {
		return nil, nil, err
	}
	rs := Library().Get(first.Ruleset)
	c, err := dnd.ParseOrg(text, rs)
	if err != nil {
		return nil, nil, err
	}
	return c, rs, nil
}

// ----------------------------------------------------------------------------
// Exporters
// ----------------------------------------------------------------------------

// SheetExporter renders a character sheet in a given format.
type SheetExporter struct {
	Format       string
	TemplatePath string
	PdfLatex     string
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *common.PluginManager
}

func (self *SheetExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *SheetExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
	if self.Props == nil {
		self.Props = map[string]interface{}{}
	}
}

// ExportToString renders the sheet and hands it back as text.
func (self *SheetExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	c, rs, err := LoadCharacter(db, query)
	if err != nil {
		return err, ""
	}
	sheet := dnd.Compute(c, rs)
	switch self.Format {
	case "latex", "pdf":
		return self.renderLatex(sheet, props)
	default:
		return self.renderHtml(sheet, props)
	}
}

// Export writes the sheet to a file. The pdf format shells out to pdflatex.
func (self *SheetExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, res := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	if self.Format == "pdf" {
		return self.renderPdf(res, to)
	}
	return os.WriteFile(to, []byte(res), 0644)
}

func (self *SheetExporter) context(sheet *dnd.Sheet, props map[string]string) map[string]interface{} {
	ctx := map[string]interface{}{}
	for k, v := range self.Props {
		ctx[k] = v
	}
	for k, v := range props {
		if v != "" {
			ctx[k] = v
		}
	}
	if _, ok := ctx["fontfamily"]; !ok {
		ctx["fontfamily"] = "Cinzel"
	}
	ctx["title"] = sheet.Name
	return ctx
}

func (self *SheetExporter) renderHtml(sheet *dnd.Sheet, props map[string]string) (error, string) {
	ctx := self.context(sheet, props)
	ctx["sheet"] = dnd.SheetMap(sheet, nil)
	template := self.TemplatePath
	if template == "" {
		template = "dnd_character_html.tpl"
	}
	if v, ok := props["template"]; ok && v != "" {
		template = v
	}
	res := self.pm.Tempo.RenderTemplate(template, ctx)
	return nil, res
}

func (self *SheetExporter) renderLatex(sheet *dnd.Sheet, props map[string]string) (error, string) {
	ctx := self.context(sheet, props)
	ctx["sheet"] = dnd.SheetMap(sheet, dnd.LatexEscape)
	template := self.TemplatePath
	if template == "" || strings.HasSuffix(template, "_html.tpl") {
		template = "dnd_character.tpl"
	}
	if v, ok := props["template"]; ok && v != "" {
		template = v
	}
	res := self.pm.Tempo.RenderTemplate(template, ctx)
	return nil, res
}

// renderPdf writes the tex to a temp file and runs pdflatex over it twice so
// that page references settle.
func (self *SheetExporter) renderPdf(tex string, to string) error {
	dir, err := os.MkdirTemp("", "dndsheet")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "sheet.tex")
	if err := os.WriteFile(src, []byte(tex), 0644); err != nil {
		return err
	}
	pdflatex := self.PdfLatex
	if pdflatex == "" {
		pdflatex = FindPdfLatex()
	}
	if pdflatex == "" {
		return fmt.Errorf("pdflatex was not found, set the pdflatex property on the dndpdf exporter")
	}
	if err := RunPdfLatex(pdflatex, dir, src); err != nil {
		return err
	}
	out := filepath.Join(dir, "sheet.pdf")
	data, err := os.ReadFile(out)
	if err != nil {
		return fmt.Errorf("pdflatex produced no output: %s", err)
	}
	return os.WriteFile(to, data, 0644)
}

// init registers the three exporters.
func init() {
	common.AddExporter("dndsheet", func() common.Exporter {
		return &SheetExporter{Format: "html", TemplatePath: "dnd_character_html.tpl",
			Props: map[string]interface{}{}}
	})
	common.AddExporter("dndlatex", func() common.Exporter {
		return &SheetExporter{Format: "latex", TemplatePath: "dnd_character.tpl",
			Props: map[string]interface{}{}}
	})
	common.AddExporter("dndpdf", func() common.Exporter {
		return &SheetExporter{Format: "pdf", TemplatePath: "dnd_character.tpl",
			Props: map[string]interface{}{}}
	})
}

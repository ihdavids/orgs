package pandoc

/* SDOC: Exporters
* Pandoc

  Hands an org file to [[https://pandoc.org][pandoc]], which writes it as
  nearly anything: Word, OpenDocument, EPUB, reStructuredText, MediaWiki,
  AsciiDoc, plain text, PowerPoint. orgs has its own html, latex and markdown
  exporters; pandoc is for the formats it does not.

  #+BEGIN_SRC yaml
  server:
    exporters:
      - name: pandoc
        bin: pandoc            # where it is, when it is not on the PATH
        format: gfm            # what /file/pandoc writes when not told
        args: ["--toc"]        # anything else pandoc should be given
  #+END_SRC

  From the command line any format no built in exporter claims goes to pandoc:

  #+BEGIN_EXAMPLE
  orgs export -f docx -query notes.org -out notes.docx
  orgs export -f rst  -query notes.org
  #+END_EXAMPLE

  The file is converted where it is, so pictures linked from it are found.
EDOC */

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

type Exporter struct {
	Bin    string   `yaml:"bin"`
	Format string   `yaml:"format"`
	Args   []string `yaml:"args"`
	// Seconds a conversion may take.
	Timeout int `yaml:"timeout"`
}

// The names people use for a format, and the writer pandoc calls it.
var writers = map[string]string{
	"md": "gfm", "markdown": "gfm", "txt": "plain", "text": "plain", "tex": "latex",
	"wiki": "mediawiki", "adoc": "asciidoc", "epub": "epub3", "word": "docx",
	"rest": "rst", "htm": "html", "ppt": "pptx",
}

// The file extension a writer's output takes.
var extensions = map[string]string{
	"gfm": "md", "commonmark": "md", "markdown": "md", "plain": "txt", "latex": "tex",
	"mediawiki": "wiki", "asciidoc": "adoc", "epub3": "epub", "epub2": "epub",
}

// Formats whose output is not text, and so cannot travel as a string.
var binary = map[string]bool{"docx": true, "odt": true, "epub": true, "epub2": true, "epub3": true, "pptx": true, "pdf": true}

var contentTypes = map[string]string{
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"odt":  "application/vnd.oasis.opendocument.text",
	"epub": "application/epub+zip", "epub2": "application/epub+zip", "epub3": "application/epub+zip",
	"html": "text/html; charset=utf-8",
}

// Writer is pandoc's name for a format somebody asked for.
func Writer(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	if w, ok := writers[f]; ok {
		return w
	}
	return f
}

// Extension is what a file of this format is called.
func Extension(format string) string {
	w := Writer(format)
	if e, ok := extensions[w]; ok {
		return e
	}
	return w
}

// ContentType is what to serve the format as.
func ContentType(format string) string {
	if c, ok := contentTypes[Writer(format)]; ok {
		return c
	}
	return "text/plain; charset=utf-8"
}

func IsBinary(format string) bool { return binary[Writer(format)] }

// Convert runs pandoc over an org file and answers with what it wrote.
func (self *Exporter) Convert(file, format string) ([]byte, error) {
	w := Writer(format)
	if w == "" {
		w = Writer(self.Format)
	}
	if w == "" {
		w = "gfm"
	}
	bin := self.Bin
	if bin == "" {
		bin = "pandoc"
	}
	dir, err := os.MkdirTemp("", "orgspandoc")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "out."+Extension(w))

	args := []string{"-f", "org", "-t", w, "-o", out, "--resource-path", filepath.Dir(file)}
	if !IsBinary(w) && w != "plain" {
		args = append(args, "--standalone")
	}
	args = append(args, self.Args...)
	args = append(args, file)

	timeout := time.Duration(self.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = filepath.Dir(file)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("pandoc is not installed on the server (or not on its PATH); " +
				"install it, or say where it is with bin: under the pandoc exporter")
		}
		return nil, fmt.Errorf("pandoc could not write %s: %v %s", w, err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(out)
}

func (self *Exporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Exporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {}

func (self *Exporter) file(db common.ODb, query string) (string, error) {
	doc := db.FindByFile(query)
	if doc == nil {
		return "", fmt.Errorf("no file called %q", query)
	}
	return doc.Path, nil
}

// Export writes the file in the format its name asks for (notes.docx), or
// the one given as props["to"].
func (self *Exporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	path, err := self.file(db, query)
	if err != nil {
		return err
	}
	format := props["to"]
	if format == "" {
		format = strings.TrimPrefix(filepath.Ext(to), ".")
	}
	b, err := self.Convert(path, format)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0644)
}

// ExportToString answers with a text format. A docx is not text, and asking
// for one here says to use /pandoc, which answers with bytes.
func (self *Exporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	path, err := self.file(db, query)
	if err != nil {
		return err, ""
	}
	format := props["to"]
	if format == "" {
		format = self.Format
	}
	if IsBinary(format) {
		return fmt.Errorf("%s is not text: ask GET /pandoc?filename=...&to=%s, which answers with the file", format, format), ""
	}
	b, err := self.Convert(path, format)
	if err != nil {
		return err, ""
	}
	return nil, string(b)
}

func init() {
	common.AddExporter("pandoc", func() common.Exporter { return &Exporter{} })
}

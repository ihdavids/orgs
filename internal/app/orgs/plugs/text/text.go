// EXPORTER: TEXT Export

/* SDOC: Exporters

* Plain Text
  The =txt= exporter (also =ascii=) writes a file as org's ASCII/UTF-8 export
  does: a centred title, numbered headings underlined with === and ---
  (deeper ones as list items), paragraphs wrapped at 72 columns, links as
  =description <url>=, emphasis kept as org writes it, code in =`...'=, and
  drawers, planning lines, clocks and keywords left out. Tables and source
  blocks are kept as written. It needs no settings:

	#+BEGIN_SRC sh
    orgs export -f txt notes.org
	#+END_SRC

	#+BEGIN_SRC yaml
    exporters:
      - name: "txt"
        width: 72          # the column paragraphs wrap at; 0 leaves them
        numbered: true     # 1, 1.1 before headings (#+OPTIONS: num:nil turns it off)
	#+END_SRC
EDOC */
package text

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type Exporter struct {
	Width    *int  `yaml:"width"`
	Numbered *bool `yaml:"numbered"`
}

func (self *Exporter) Unmarshal(unmarshal func(interface{}) error) error { return unmarshal(self) }
func (self *Exporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {}
func (self *Exporter) Standalone() bool                                               { return true }

func (self *Exporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, s := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(s), 0o644)
}

func (self *Exporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	f := db.FindByFile(query)
	if f == nil {
		return fmt.Errorf("txt: no file named %s", query), ""
	}
	return nil, self.Text(f)
}

// Text is a document as plain text.
func (self *Exporter) Text(doc *org.Document) string {
	width := 72
	if self.Width != nil {
		width = *self.Width
	}
	numbered := self.Numbered == nil || *self.Numbered
	if option(doc, "num") == "nil" {
		numbered = false
	}
	levels := 3
	if h, err := strconv.Atoi(option(doc, "H")); err == nil && h > 0 {
		levels = h
	}
	w := &Writer{OrgWriter: org.NewOrgWriter(), width: width, numbered: numbered, levels: levels}
	w.ExtendingWriter = w
	w.Document = doc
	var b strings.Builder
	if t := strings.TrimSpace(doc.Get("TITLE")); t != "" && option(doc, "title") != "nil" {
		b.WriteString(center(t, width) + "\n")
		b.WriteString(center(strings.Repeat("=", len([]rune(t))), width) + "\n")
		for _, k := range []string{"AUTHOR", "DATE"} {
			if v := strings.TrimSpace(doc.Get(k)); v != "" && option(doc, strings.ToLower(k)) != "nil" {
				b.WriteString(center(v, width) + "\n")
			}
		}
		b.WriteString("\n\n")
	}
	org.WriteNodes(w, doc.Nodes...)
	b.WriteString(w.String())
	return tidy(b.String())
}

// option is an #+OPTIONS: key's value, "" when the file does not set it.
// (go-org's GetOption answers "nil" for a key nobody set, and logs it.)
func option(doc *org.Document, key string) string {
	for _, settings := range []map[string]string{doc.BufferSettings, doc.DefaultSettings} {
		for _, field := range strings.Fields(settings["OPTIONS"]) {
			if strings.HasPrefix(field, key+":") {
				return field[len(key)+1:]
			}
		}
	}
	return ""
}

func center(s string, width int) string {
	n := len([]rune(s))
	if width <= n {
		return s
	}
	return strings.Repeat(" ", (width-n)/2) + s
}

// tidy keeps blank lines to one between things.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	out := []string{}
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.Trim(strings.Join(out, "\n"), "\n") + "\n"
}

// Writer is go-org's org writer with org's ASCII export's choices on top.
type Writer struct {
	*org.OrgWriter
	Document *org.Document
	width    int
	numbered bool
	levels   int
	counts   []int
}

func (w *Writer) WriterWithExtensions() org.Writer { return w }

func (w *Writer) number(lvl int) string {
	for len(w.counts) < lvl {
		w.counts = append(w.counts, 0)
	}
	w.counts = w.counts[:lvl]
	w.counts[lvl-1]++
	parts := []string{}
	for _, c := range w.counts {
		parts = append(parts, strconv.Itoa(c))
	}
	return strings.Join(parts, ".")
}

func (w *Writer) WriteHeadline(h org.Headline) {
	if w.Document != nil && h.IsExcluded(w.Document) {
		return
	}
	title := strings.TrimSpace(w.WriteNodesAsString(h.Title...))
	if h.Priority != "" && option(w.Document, "pri") != "nil" {
		title = "[#" + h.Priority + "] " + title
	}
	if h.Status != "" && option(w.Document, "todo") != "nil" {
		title = h.Status + " " + title
	}
	if len(h.Tags) > 0 && option(w.Document, "tags") != "nil" {
		title += "  :" + strings.Join(h.Tags, ":") + ":"
	}
	if h.Lvl > w.levels {
		w.WriteString(strings.Repeat("  ", h.Lvl-w.levels-1) + "- " + title + "\n")
		body := w.WriteNodesAsString(h.Children...)
		for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
			if l != "" {
				w.WriteString(strings.Repeat("  ", h.Lvl-w.levels) + l)
			}
			w.WriteString("\n")
		}
		return
	}
	if w.numbered {
		title = w.number(h.Lvl) + " " + title
	}
	under := "-"
	switch h.Lvl {
	case 1:
		under = "="
	case 2:
		under = "~"
	}
	w.WriteString("\n" + title + "\n" + strings.Repeat(under, len([]rune(title))) + "\n\n")
	org.WriteNodes(w, h.Children...)
}

func (w *Writer) WriteParagraph(p org.Paragraph) {
	text := strings.TrimSpace(w.WriteNodesAsString(p.Children...))
	if w.width > 0 {
		text = wrap(text, w.width)
	}
	w.WriteString(text + "\n\n")
}

// wrap breaks text at width, keeping the line breaks that are there.
func wrap(text string, width int) string {
	out := []string{}
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		line := ""
		for _, word := range words {
			if line != "" && len([]rune(line))+1+len([]rune(word)) > width {
				out = append(out, line)
				line = word
				continue
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func (w *Writer) WriteRegularLink(l org.RegularLink) {
	url := l.URL
	if strings.HasPrefix(url, "file:") {
		url = strings.TrimPrefix(url, "file:")
	}
	if l.Description == nil {
		w.WriteString("<" + url + ">")
		return
	}
	desc := strings.TrimSpace(w.WriteNodesAsString(l.Description...))
	switch l.Protocol {
	case "", "id", "file":
		// Inside the document, or a file: the words say enough.
		if l.Protocol == "file" {
			w.WriteString(desc + " <" + url + ">")
		} else {
			w.WriteString(desc)
		}
	default:
		w.WriteString(desc + " <" + url + ">")
	}
}

func (w *Writer) WriteEmphasis(e org.Emphasis) {
	inner := w.WriteNodesAsString(e.Content...)
	switch e.Kind {
	case "*":
		w.WriteString("*" + inner + "*")
	case "/":
		w.WriteString("/" + inner + "/")
	case "_":
		w.WriteString("_" + inner + "_")
	case "+":
		w.WriteString("+" + inner + "+")
	case "~", "=":
		w.WriteString("`" + inner + "'")
	default:
		w.WriteString(inner)
	}
}

func (w *Writer) WriteKeyword(k org.Keyword)               {}
func (w *Writer) WriteComment(c org.Comment)               {}
func (w *Writer) WritePropertyDrawer(d org.PropertyDrawer) {}
func (w *Writer) WriteSDC(s org.SDC)                       {}
func (w *Writer) WriteClock(c org.Clock)                   {}

func (w *Writer) WriteDrawer(d org.Drawer) {
	if strings.EqualFold(d.Name, "LOGBOOK") {
		return
	}
	org.WriteNodes(w, d.Children...)
}

func (w *Writer) WriteBlock(b org.Block) {
	switch strings.ToUpper(b.Name) {
	case "SRC", "EXAMPLE":
		body := strings.TrimRight(w.WriteNodesAsString(b.Children...), "\n")
		for _, l := range strings.Split(body, "\n") {
			w.WriteString("  " + l + "\n")
		}
		w.WriteString("\n")
	case "EXPORT":
		if len(b.Parameters) > 0 && strings.EqualFold(b.Parameters[0], "ascii") {
			w.WriteString(w.WriteNodesAsString(b.Children...))
		}
	case "COMMENT":
	case "QUOTE", "VERSE":
		body := strings.TrimRight(w.WriteNodesAsString(b.Children...), "\n")
		for _, l := range strings.Split(body, "\n") {
			w.WriteString("    " + strings.TrimLeft(l, " ") + "\n")
		}
		w.WriteString("\n")
	default:
		org.WriteNodes(w, b.Children...)
	}
}

func init() {
	common.AddExporter("txt", func() common.Exporter { return &Exporter{} })
	common.AddExporter("ascii", func() common.Exporter { return &Exporter{} })
}

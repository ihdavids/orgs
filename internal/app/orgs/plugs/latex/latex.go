// EXPORTER: Latex Export

/*
	SDOC: Exporters

* Latex

		TODO More documentation on this module

		The latex exporter can be used to generate a .tex file
		from a set of org nodes. This can be for a variety of typesetting reasons
		or simply as a stepping stone on the way to a pdf using any number of latex
		tools for converting to pdf.

		To set it up in your config file you do the following:

		#+BEGIN_SRC yaml
		- name: "latex"
	      templatepath: "latex template path"
		#+END_SRC

		Converting to a pdf can be done with a variety of latex tools.
		MacTex for instance:

		#+BEGIN_SRC bash
	    pdflatex --shell-escape ./docs.tex -output-format=pdf -o=docs.pdf
		#+END_SRC

		The Latex module uses a cascading templates as a means of
		facilitating expansing. There is a default template (book_templates.yaml)
		that is the fallback for all templates used in generating latex. You can
		tweak that but beware, pongo2 templates can be a bit temperamental and
		will expand in odd ways if you add a second set of {} on a for loop or
		miss out on quotes in a parameter.

EDOC
*/
package latex

import (
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/flosch/pongo2/v5"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/op/go-logging.v1"
	"gopkg.in/yaml.v3"
)

func isRawTextBlock(name string) bool { return name == "SRC" || name == "EXAMPLE" || name == "EXPORT" }

func isImageOrVideoLink(n org.Node) bool {
	if l, ok := n.(org.RegularLink); ok && l.Kind() == "video" || l.Kind() == "image" {
		return true
	}
	return false
}

type OrgLatexExporter struct {
	TemplatePath string
	Props        map[string]any
	out          *logging.Logger
	pm           *common.PluginManager
}

type EnvData struct {
	Env     string
	Attribs [][]string
	Caption string
}

func (s EnvData) HaveCaption() bool {
	return s.Caption != ""
}

func (s EnvData) HaveEnv() bool {
	return s.Env != ""
}

func (s EnvData) HaveAttribs() bool {
	return len(s.Attribs) > 0
}

func (s EnvData) GetEnv(def string) string {
	if s.Env == "" {
		return def
	}
	return s.Env
}

func (s EnvData) GetCaption() string {
	return s.Caption
}

func (s EnvData) GetAttribs() [][]string {
	return s.Attribs
}

type EnvironmentStack []EnvData

func (s *EnvironmentStack) Push(v EnvData) {
	*s = append(*s, v)
}

func (s *EnvironmentStack) Pop() EnvData {
	res := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return res
}

func (s *EnvironmentStack) IsEmpty() bool {
	return len(*s) <= 0
}

func (s *EnvironmentStack) Peek() *EnvData {
	if s.IsEmpty() {
		return nil
	}
	res := &(*s)[len(*s)-1]
	return res
}

func (s *EnvironmentStack) GetEnv(def string) string {
	if env := s.Peek(); env != nil {
		return env.GetEnv(def)
	}
	return def
}

func (s *EnvironmentStack) GetCaption() string {
	if env := s.Peek(); env != nil {
		return env.GetCaption()
	}
	return ""
}

func (s *EnvironmentStack) GetAttribs() [][]string {
	if env := s.Peek(); env != nil {
		return env.GetAttribs()
	}
	return [][]string{}
}

func (s *EnvironmentStack) HaveCaption() bool {
	if env := s.Peek(); env != nil && env.HaveCaption() {
		return true
	}
	return false
}

func (s *EnvironmentStack) HaveEnv() bool {
	if env := s.Peek(); env != nil && env.HaveEnv() {
		return true
	}
	return false
}

func (s *EnvironmentStack) HaveAttribs() bool {
	if env := s.Peek(); env != nil && env.HaveAttribs() {
		return true
	}
	return false
}

type TableTemplate struct {
	Vertical bool
	Template string
}

// These get properties
// and the heading as parameters
// Properties are ALWAYS uppercase
type StringTemplate struct {
	Template string
}

type DocClassConf struct {
	Tables    map[string]TableTemplate
	Headings  map[string]StringTemplate
	Paragraph map[string]StringTemplate
	Blocks    map[string]StringTemplate
}

type SubTemplates struct {
	defaultDocConf *DocClassConf
	docconf        *DocClassConf
}

// TODO: Get rid of this!
func (s *SubTemplates) HaveHeadingOverrides() bool {
	return s.docconf != nil && len(s.docconf.Headings) > 0
}

func (s *SubTemplates) HeadingOverrides() map[string]StringTemplate {
	return s.docconf.Headings
}

func (s *SubTemplates) BlockTemplate(name string, usedefault ...bool) (StringTemplate, bool) {
	useDefault := true
	if len(usedefault) > 0 {
		useDefault = usedefault[0]
	}
	if s.docconf != nil && s.docconf.Blocks != nil {
		if tbl, ok := s.docconf.Blocks[name]; ok {
			return tbl, ok
		}
		if tbl, ok := s.docconf.Blocks["default"]; useDefault && ok {
			return tbl, ok
		}
	}
	if tbl, ok := s.defaultDocConf.Blocks[name]; ok {
		return tbl, ok
	}
	if tbl, ok := s.defaultDocConf.Blocks["default"]; useDefault && ok {
		return tbl, ok
	}
	return StringTemplate{""}, false
}

func (s *SubTemplates) TableTemplate(name string, usedefault ...bool) (TableTemplate, bool) {
	useDefault := true
	if len(usedefault) > 0 {
		useDefault = usedefault[0]
	}
	if s.docconf != nil && s.docconf.Tables != nil {
		if tbl, ok := s.docconf.Tables[name]; ok {
			return tbl, ok
		}
		if tbl, ok := s.docconf.Tables["default"]; useDefault && ok {
			return tbl, ok
		}
	}
	if tbl, ok := s.defaultDocConf.Tables[name]; ok {
		return tbl, ok
	}
	if tbl, ok := s.defaultDocConf.Tables["default"]; useDefault && ok {
		return tbl, ok
	}
	return TableTemplate{false, ""}, false
}

func (s *SubTemplates) HeadingTemplate(name string, usedefault ...bool) (StringTemplate, bool) {
	useDefault := true
	if len(usedefault) > 0 {
		useDefault = usedefault[0]
	}
	if s.docconf != nil && s.docconf.Headings != nil {
		if tbl, ok := s.docconf.Headings[name]; ok {
			return tbl, ok
		}
		if tbl, ok := s.docconf.Headings["default"]; useDefault && ok {
			return tbl, ok
		}
	}
	if tbl, ok := s.defaultDocConf.Headings[name]; ok {
		return tbl, ok
	}
	if tbl, ok := s.defaultDocConf.Headings["default"]; useDefault && ok {
		return tbl, ok
	}
	return StringTemplate{""}, false
}

func (s *SubTemplates) ParagraphTemplate(name string, usedefault ...bool) (StringTemplate, bool) {
	useDefault := true
	if len(usedefault) > 0 {
		useDefault = usedefault[0]
	}
	if s.docconf != nil && s.docconf.Paragraph != nil {
		if tbl, ok := s.docconf.Paragraph[name]; ok {
			return tbl, ok
		}
		if tbl, ok := s.docconf.Paragraph["default"]; useDefault && ok {
			return tbl, ok
		}
	}
	if tbl, ok := s.defaultDocConf.Paragraph[name]; ok {
		return tbl, ok
	}
	if tbl, ok := s.defaultDocConf.Paragraph["default"]; useDefault && ok {
		return tbl, ok
	}
	return StringTemplate{""}, false
}

func MakeTemplateRegistry(defaultPath, classPath string) *SubTemplates {
	defer func() {
		if err := recover(); err != nil {
			log.Println("\n!!!!!!!!!!!!!!!!!\n!!!!!!!!!!!!!!!!!!!!!!!!!\npanic occurred:", err, "\n", "\n+++++++++++++++++++++++++++++++++++++++++++\n")
		}
	}()
	var t *SubTemplates = &SubTemplates{}
	t.docconf = GetDocConf(classPath)
	// This is our fallback, if we do not find it in our specific template, we fall back on this
	// to make the normal export work
	t.defaultDocConf = GetDocConf(defaultPath)

	//t.docconf = GetDocConf(self.pm.Tempo.ExpandTemplatePath(w.docclass + "_templates.yaml"))
	// This is our fallback, if we do not find it in our specific template, we fall back on this
	// to make the normal export work
	//t.defaultDocConf = GetDocConf(self.pm.Tempo.ExpandTemplatePath("book_templates.yaml"))
	return t
}

type OrgLatexWriter struct {
	ExtendingWriter org.Writer
	strings.Builder
	Document            *org.Document
	log                 *log.Logger
	footnotes           *footnotes
	PrettyRelativeLinks bool
	envs                EnvironmentStack
	docclass            string
	templateRegistry    *SubTemplates
	exporter            *OrgLatexExporter
}

func (w *OrgLatexWriter) TemplateProps() *map[string]interface{} {
	return &w.exporter.Props
}

func NewOrgLatexWriter(exp *OrgLatexExporter) *OrgLatexWriter {
	defaultConfig := org.New()
	return &OrgLatexWriter{
		Document: &org.Document{Configuration: defaultConfig},
		footnotes: &footnotes{
			mapping: map[string]int{},
		},
		exporter: exp,
	}
}

func EscapeString(out string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(out,
		"_", "\\_"),
		"&", "\\&"),
		"$", "\\$")
}

func GetProp(name, revealName string, h org.Headline, secProps string) string {
	tran := h.Doc.Get(name)
	if tmp, ok := h.Properties.Get(name); ok {
		tran = tmp
	}
	if tran != "" {
		secProps = fmt.Sprintf("%s %s=\"%s\"", secProps, revealName, tran)
	}
	return secProps
}

func GetPropTag(name, revealName string, h org.Headline, secProps string) string {
	tran := h.Doc.Get(name)
	if tmp, ok := h.Properties.Get(name); ok {
		tran = tmp
	}
	if tran != "" && tran != "false" && tran != "off" && tran != "f" {
		secProps = fmt.Sprintf("%s %s", secProps, revealName)
	}
	return secProps
}

func (self *OrgLatexExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *OrgLatexExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	fmt.Printf("LATEX: Export called", query, to, opts)
	err, str := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	return os.WriteFile(to, []byte(str), 0644)

	/*
		_, err := db.QueryTodosExpr(query)
		if err != nil {
			msg := fmt.Sprintf("ERROR: latex failed to query expression, %v [%s]\n", err, query)
			log.Printf(msg)
			return fmt.Errorf(msg)
		}
	*/

}

func GetDocConf(path string) *DocClassConf {
	fmt.Printf("[GetDocConf] %s\n", path)
	var c *DocClassConf = &DocClassConf{}
	if _, errStat := os.Stat(path); errStat == nil {
		fmt.Printf("Trying to load config file...\n")
		yamlFile, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("ERROR: yamlFile.Get err   #%v ", err)
			return nil
		}
		err = yaml.Unmarshal(yamlFile, c)
		if err != nil {
			log.Printf("ERROR: Unmarshal: %v", err)
			return nil
		}
	}
	return c
}

// ----------- [ Expression Filters ] -----------------------

func StartEnv(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	var res *pongo2.Error = nil
	if in != nil && param != nil {
		e := in.Interface().(*EnvironmentStack)
		s := param.String()
		if e == nil {
			res = &pongo2.Error{Sender: "startenv", OrigError: fmt.Errorf("startenv: Environment stack is missing, abort!")}
		}
		if s != "" {
			s = strings.TrimSpace(strings.ReplaceAll(s, "\u00a0", "\n"))
			s = strings.TrimSpace((*e).startEnvAsString(s))
			return pongo2.AsValue(s), res
		} else {
			res = &pongo2.Error{Sender: "startenv", OrigError: fmt.Errorf("Environment stack requires an environment name")}
		}
	}
	return pongo2.AsValue(""), res
}

func EndEnv(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	var res *pongo2.Error = nil
	if in != nil && param != nil {
		e := in.Interface().(*EnvironmentStack)
		s := param.String()
		if e == nil {
			res = &pongo2.Error{Sender: "endenv", OrigError: fmt.Errorf("Environment stack is missing, abort!")}
		}
		if s != "" {
			s = strings.TrimSpace(strings.ReplaceAll(s, "\u00a0", "\n"))
			s = strings.TrimSpace((*e).endEnvAsString(s))
			return pongo2.AsValue(s), res
		} else {
			res = &pongo2.Error{Sender: "endenv", OrigError: fmt.Errorf("Environment stack requires an environment name")}
		}
	}
	return pongo2.AsValue(""), res
}

// ----------- [ Exporter System ] -----------------------

func (self *OrgLatexExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Printf("LATEX: Export string called [%s]:[%s]\n", query, opts)

	if f := db.FindByFile(query); f != nil {
		for k, v := range f.BufferSettings {
			self.Props[k] = v
		}
		theme := f.Get("LATEX_THEME")
		if theme == "" {
			theme = f.Get("LATEX_CLASS")
		}
		// Shorthand for braces when spaces are a problem
		self.Props["o"] = "{"
		self.Props["c"] = "}"
		self.Props["docclass"] = "book"
		if theme != "" {
			self.Props["docclass"] = theme
			self.Props["theme"] = theme
		}
		classOpts := ""
		clsOpts := f.Get("LATEX_CLASS_OPTIONS")
		if clsOpts != "" {
			classOpts = fmt.Sprintf("[%s]", clsOpts)
		}
		self.Props["docclass_opts"] = classOpts

		temp := f.Get("LATEX_TEMPLATE")
		if temp != "" {
			self.TemplatePath = temp
		}
		w := NewOrgLatexWriter(self)
		w.docclass = self.Props["docclass"].(string)
		classPath := self.pm.Tempo.ExpandTemplatePath(w.docclass + "_templates.yaml")
		defaultPath := self.pm.Tempo.ExpandTemplatePath("book_templates.yaml")
		w.templateRegistry = MakeTemplateRegistry(classPath, defaultPath)

		self.Props["envs"] = &w.envs
		pongo2.RegisterFilter("startenv", StartEnv)
		pongo2.RegisterFilter("endenv", EndEnv)
		// TODO: w.Opts = opts
		f.Write(w)
		//org.WriteNodes(w, f.Nodes...)
		res := w.String()
		self.Props["latex_data"] = res
		//self.Props["post_scripts"] = w.PostWriteScripts

		fmt.Printf("DOC START: ========================================\n")
		fmt.Printf("TEMP: %s\n", self.TemplatePath)
		res = self.pm.Tempo.RenderTemplate(self.TemplatePath, self.Props)
		fmt.Printf("XXX: %s\n", res)
		return nil, res
	} else {
		fmt.Printf("Failed to find file in database: [%s]", query)
		return fmt.Errorf("Failed to find file in database: [%s]", query), ""
	}
}

func (self *OrgLatexExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.out = manager.Out
	self.pm = manager

}

func NewLatexExp() *OrgLatexExporter {
	var g *OrgLatexExporter = new(OrgLatexExporter)
	return g
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["wordcloud"]; !ok {
		m["wordcloud"] = false
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["theme"]; !ok {
		m["theme"] = "default"
	}
	return m
}

// init function is called at boot
func init() {
	common.AddExporter("latex", func() common.Exporter {
		return &OrgLatexExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "latex_default.tpl"}
	})
}

// ----------- [ Writer ] -----------------------------
// //////////////////////// WRITER //////////////////////////////////
func (n *OrgLatexWriter) NodeIdx(_ int) {
	// We do not need the node idx at this point in time
}

func (n *OrgLatexWriter) ResetLineBreak() {
	// We do not need the reset line break at this point in time
}

type footnotes struct {
	mapping map[string]int
	list    []*org.FootnoteDefinition
}

var emphasisTags = map[string]string{
	"/":   `\textit{%s}`,
	"*":   `\textbf{%s}`,
	"+":   `\sout{%s}`,
	"~":   `\texttt{%s}`,
	"=":   `\texttt{%s}`,
	"_":   `\underline{%s}`,
	"_{}": `\textsubscript{%s}`,
	"^{}": `\textsuperscript{%s}`,
}

var listTags = map[string][]string{
	"unordered":   []string{`\begin{itemize}`, `\end{itemize}`},
	"ordered":     []string{`\begin{enumerate}`, `\end{enumerate}`},
	"descriptive": []string{`\begin{description}`, `\end{description}`},
}

var listItemStatuses = map[string]string{
	" ": "unchecked",
	"-": "indeterminate",
	"X": "checked",
}
var sectionTypes = []string{
	`\chapter%s{%s}`,
	`\section%s{%s}`,
	`\subsection%s{%s}`,
	`\subsubsection%s{%s}`,
	`\paragraph%s{%s}`,
	`\subparagraph%s{%s}`,
}

var cleanHeadlineTitleForHTMLAnchorRegexp = regexp.MustCompile(`</?a[^>]*>`) // nested a tags are not valid HTML
var tocHeadlineMaxLvlRegexp = regexp.MustCompile(`headlines\s+(\d+)`)

func (w *OrgLatexWriter) RenderContentTemplate(temp string, content string) {
	tp := w.TemplateProps()
	(*tp)["content"] = strings.TrimSpace(content)
	res := w.exporter.pm.Tempo.RenderTemplateString(temp, *tp)
	w.WriteString(res)
}

func (w *OrgLatexWriter) WriteNodesAsString(nodes ...org.Node) string {
	original := w.Builder
	w.Builder = strings.Builder{}
	org.WriteNodes(w, nodes...)
	out := w.String()
	w.Builder = original
	return out
}

func (w *OrgLatexWriter) WriterWithExtensions() org.Writer {
	if w.ExtendingWriter != nil {
		return w.ExtendingWriter
	}
	return w
}

func (w *OrgLatexWriter) HaveTitle(d *org.Document) bool {
	if title := d.Get("TITLE"); title != "" && w.Document.GetOption("title") != "nil" {
		return true
	}
	return false
}

func (w *OrgLatexWriter) Before(d *org.Document) {
	w.Document = d
	w.log = d.Log
	tp := w.TemplateProps()
	haveTitle := w.docclass != "dndbook" || w.HaveTitle(d)
	(*tp)["havetitle"] = haveTitle
	title := d.Get("TITLE")
	if haveTitle {
		titleDocument := d.Parse(strings.NewReader(title), d.Path)
		if titleDocument.Error == nil {
			title = w.WriteNodesAsString(titleDocument.Nodes...)
		}
	}
	(*tp)["title"] = title
	if tmp, ok := w.templateRegistry.HeadingTemplate("TITLE"); ok {
		res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
		w.WriteString(res)
	} else {
		// DEPRECATED
		if title = d.Get("TITLE"); w.HaveTitle(d) {
			titleDocument := d.Parse(strings.NewReader(title), d.Path)
			if titleDocument.Error == nil {
				title = w.WriteNodesAsString(titleDocument.Nodes...)
			}
			w.WriteString(fmt.Sprintf(`\title{%s}`+"\n", title))
		}
	}

	auth := d.Get("AUTHOR")
	haveAuth := auth != "" && w.Document.GetOption("author") != "nil"
	(*tp)["haveauthor"] = haveAuth
	if haveAuth {
		doc := d.Parse(strings.NewReader(auth), d.Path)
		if doc.Error == nil {
			auth = w.WriteNodesAsString(doc.Nodes...)
		}
	}
	(*tp)["title"] = auth
	if tmp, ok := w.templateRegistry.HeadingTemplate("AUTHOR"); ok {
		res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
		w.WriteString(res)
	} else {
		// DEPRECATED
		if auth := d.Get("AUTHOR"); auth != "" && w.Document.GetOption("author") != "nil" {
			titleDocument := d.Parse(strings.NewReader(auth), d.Path)
			if titleDocument.Error == nil {
				auth = w.WriteNodesAsString(titleDocument.Nodes...)
			}
			w.WriteString(fmt.Sprintf(`\author{%s}`+"\n", auth))
		}
	}

	dt := d.Get("DATE")
	haveDate := dt != "" && w.Document.GetOption("date") != "nil"
	(*tp)["havedate"] = haveDate
	if haveDate {
		doc := d.Parse(strings.NewReader(dt), d.Path)
		if doc.Error == nil {
			dt = w.WriteNodesAsString(doc.Nodes...)
		}
	}
	(*tp)["date"] = dt
	if tmp, ok := w.templateRegistry.HeadingTemplate("DATE"); ok {
		res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
		w.WriteString(res)
	} else {
		// DEPRECATED
		if dt := d.Get("DATE"); dt != "" && w.Document.GetOption("date") != "nil" {
			titleDocument := d.Parse(strings.NewReader(dt), d.Path)
			if titleDocument.Error == nil {
				dt = w.WriteNodesAsString(titleDocument.Nodes...)
			}
			w.WriteString(fmt.Sprintf(`\date{%s}`+"\n", dt))
		}
	}

	w.WriteString("\n" + `\begin{document}` + "\n")
	if w.HaveTitle(d) {
		w.WriteString(`\maketitle` + "\n")
	}
	if w.Document.GetOption("toc") != "nil" {
		maxLvl, _ := strconv.Atoi(w.Document.GetOption("toc"))
		w.WriteOutline(d, maxLvl)
	}

}

func (w *OrgLatexWriter) After(d *org.Document) {
	w.WriteFootnotes(d)
	w.WriteString("\n" + `\end{document}` + "\n")
}

func (w *OrgLatexWriter) WriteComment(org.Comment)               {}
func (w *OrgLatexWriter) WritePropertyDrawer(org.PropertyDrawer) {}

// TODO DEPRECATE
func (w *OrgLatexWriter) startEnv(name string) {
	name = w.envs.GetEnv(name)
	vals := strings.Split(name, "|")
	if len(vals) > 1 {
		name = strings.TrimSpace(vals[0])
		w.WriteString("\n" + fmt.Sprintf(`\begin{%s}`, name))
		for _, txt := range vals[1:] {
			txt = strings.TrimSpace(txt)
			if strings.Contains(txt, "[") {
				w.WriteString(txt)
			} else {
				w.WriteString(fmt.Sprintf("{%s}", txt))
			}
		}
		w.WriteString("\n")
	} else {
		w.WriteString("\n" + fmt.Sprintf(`\begin{%s}`, name) + "\n")
	}
}

func (e *EnvironmentStack) startEnvAsString(name string) string {
	out := ""
	name = e.GetEnv(name)
	vals := strings.Split(name, "|")
	if len(vals) > 1 {
		name = strings.TrimSpace(vals[0])
		out += "\n" + fmt.Sprintf(`\begin{%s}`, name)
		for _, txt := range vals[1:] {
			txt = strings.TrimSpace(txt)
			if strings.Contains(txt, "[") {
				out += txt
			} else {
				out += fmt.Sprintf("{%s}", txt)
			}
		}
		out += "\n"
	} else {
		out += "\n" + fmt.Sprintf(`\begin{%s}`, name) + "\n"
	}
	return out
}

// TODO DEPRECATE
func (w *OrgLatexWriter) endEnv(name string) {
	name = w.envs.GetEnv(name)
	vals := strings.Split(name, "|")
	if len(vals) > 1 {
		name = strings.TrimSpace(vals[0])
		w.WriteString("\n" + fmt.Sprintf(`\end{%s}`, name) + "\n")
	} else {
		w.WriteString("\n" + fmt.Sprintf(`\end{%s}`, name) + "\n")
	}
}

func (e *EnvironmentStack) endEnvAsString(name string) string {
	out := ""
	name = e.GetEnv(name)
	vals := strings.Split(name, "|")
	if len(vals) > 1 {
		name = strings.TrimSpace(vals[0])
		out += "\n" + fmt.Sprintf(`\end{%s}`, name) + "\n"
	} else {
		out += "\n" + fmt.Sprintf(`\end{%s}`, name) + "\n"
	}
	return out
}

/*
	SDOC: Exporters::Latex

** Latex Blocks

		Org Mode supports various block types these all appear
		in an org file with BEGIN and END comments:

		#+BEGIN_SRC org
			#+BEGIN_QUOTE
			Info goes here
			#+END_QUOTE
		#+END_SRC

		The latex exporter supports the gamut of these block types
		in its yaml templates as so:

		#+BEGIN_SRC yaml
	    blocks:
	      SRC:
	        template: |+
	          {{ envs | startenv: "minted" }}{ {{ lang }} }
	          {{content | safe}}
	          {{ envs | endenv: "minted" }}
	      EXAMPLE:
	        template: |+
	          {{ envs | startenv: "verbatim" }}
	          {{content | safe}}
	          {{ envs | endenv: "verbatim" }}
	      QUOTE:
	        template: |+
	          {{ envs | startenv: "displayquote" }}
	          {{content | safe}}
	          {{ envs | endenv: "displayquote" }}
	    #+END_SRC

EDOC
*/
func (w *OrgLatexWriter) WriteBlock(b org.Block) {
	content, params := w.blockContent(b.Name, b.Children), b.ParameterMap()
	props := w.TemplateProps()
	// Copy over local properties for this block so they can be
	// used in this template
	tp := &map[string]any{}
	for k, v := range *props {
		(*tp)[k] = v
	}
	for k, v := range params {
		(*tp)[k] = v
	}
	switch b.Name {
	case "SRC":
		if params[":exports"] == "results" || params[":exports"] == "none" {
			break
		}
		lang := "text"
		if len(b.Parameters) >= 1 {
			lang = strings.ToLower(b.Parameters[0])
		}
		if tmp, ok := w.templateRegistry.BlockTemplate("SRC"); ok {
			(*tp)["lang"] = lang
			(*tp)["content"] = content
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			// TODO: Deprecated
			w.startEnv("minted")
			w.WriteString(fmt.Sprintf("{%s}\n", lang))
			w.WriteString(content)
			w.endEnv("minted")
		}

	case "EXAMPLE":
		if tmp, ok := w.templateRegistry.BlockTemplate("EXAMPLE"); ok {
			(*tp)["content"] = EscapeString(content)
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			w.startEnv("verbatim")
			w.WriteString(EscapeString(content))
			w.endEnv("verbatim")
		}
	case "EXPORT":
		if len(b.Parameters) >= 1 && strings.ToLower(b.Parameters[0]) == "latex" {
			w.WriteString(content + "\n")
		}
	case "QUOTE":
		if tmp, ok := w.templateRegistry.BlockTemplate("QUOTE"); ok {
			(*tp)["content"] = content
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			w.startEnv("displayquote")
			w.WriteString(content)
			w.endEnv("displayquote")
		}
	case "CENTER":
		if tmp, ok := w.templateRegistry.BlockTemplate("CENTER"); ok {
			(*tp)["content"] = content
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			w.WriteString("\n" + `\begin{center}\n\centering\n`)
			w.WriteString(content + "\n" + `\end{center}\n`)
		}
	case "MONSTERTYPE":
		if tmp, ok := w.templateRegistry.BlockTemplate("MONSTERTYPE"); ok {
			(*tp)["content"] = content
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			w.WriteString("\n" + fmt.Sprintf(`\DndMonsterType{%s}`, content) + "\n")
		}
	default:
		if tmp, ok := w.templateRegistry.BlockTemplate("default"); ok {
			(*tp)["content"] = content
			(*tp)["envname"] = strings.ToLower(b.Name)
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			w.startEnv(strings.ToLower(b.Name))
			w.WriteString(content)
			w.endEnv(strings.ToLower(b.Name))
		}
	}

	// TODO: Do we need to handle this?
	if b.Result != nil && params[":exports"] != "code" && params[":exports"] != "none" {
		org.WriteNodes(w, b.Result)
	}
}

func (w *OrgLatexWriter) WriteResult(r org.Result) {
	org.WriteNodes(w, r.Node)
}

func (w *OrgLatexWriter) WriteInlineBlock(b org.InlineBlock) {
	tp := w.TemplateProps()
	content := w.blockContent(strings.ToUpper(b.Name), b.Children)
	switch b.Name {
	case "src":
		if tmp, ok := w.templateRegistry.BlockTemplate("inline_src"); ok {
			(*tp)["content"] = content
			res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
			w.WriteString(res)
		} else {
			w.WriteString(` \begin{verbatim} ` + content + ` \end{verbatim}` + "\n")
		}
	case "export":
		if strings.ToLower(b.Parameters[0]) == "latex" {
			w.WriteString(content)
		}
	}
}

func (w *OrgLatexWriter) WriteDrawer(d org.Drawer) {
	org.WriteNodes(w, d.Children...)
}

func (w *OrgLatexWriter) WriteKeyword(k org.Keyword) {
	if k.Key == "LATEX" {
		w.WriteString(k.Value + "\n")
	} else if k.Key == "TOC" {
		if m := tocHeadlineMaxLvlRegexp.FindStringSubmatch(k.Value); m != nil {
			maxLvl, _ := strconv.Atoi(m[1])
			w.WriteOutline(w.Document, maxLvl)
		}
	}
}

func (w *OrgLatexWriter) WriteInclude(i org.Include) {
	org.WriteNodes(w, i.Resolve())
}

func (w *OrgLatexWriter) WriteFootnoteDefinition(f org.FootnoteDefinition) {
	w.footnotes.updateDefinition(f)
}

func (w *OrgLatexWriter) WriteFootnotes(d *org.Document) {
	if w.Document.GetOption("f") == "nil" || len(w.footnotes.list) == 0 {
		return
	}
	//w.WriteString(`<div class="footnotes">` + "\n")
	//w.WriteString(`<hr class="footnotes-separatator">` + "\n")
	//w.WriteString(`<div class="footnote-definitions">` + "\n")
	for i, definition := range w.footnotes.list {
		id := i + 1
		if definition == nil {
			name := ""
			for k, v := range w.footnotes.mapping {
				if v == i {
					name = k
				}
			}
			w.log.Printf("Missing footnote definition for [fn:%s] (#%d)", name, id)
			continue
		}
		//w.WriteString(`<div class="footnote-definition">` + "\n")
		//w.WriteString(fmt.Sprintf(`<sup id="footnote-%d"><a href="#footnote-reference-%d">%d</a></sup>`, id, id, id) + "\n")
		//w.WriteString(`<div class="footnote-body">` + "\n")
		w.WriteString(fmt.Sprintf(`\footnotetext[%d]{`, id))
		org.WriteNodes(w, definition.Children...)
		w.WriteString("}")
		//w.WriteString("</div>\n</div>\n")
	}
	//w.WriteString("</div>\n</div>\n")
}

func (w *OrgLatexWriter) WriteOutline(d *org.Document, maxLvl int) {
	// Need to exclude on basis of toc:nil parameter as well
	// Not compatible with DnDBook class for some reason.
	// Presence of a title allow TOC to work.
	tp := w.TemplateProps()
	if tmp, ok := w.templateRegistry.BlockTemplate("toc"); ok {
		res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
		w.WriteString(res)
	} else {
		if w.docclass != "dndbook" || w.HaveTitle(d) {
			w.WriteString("\n" + `\tableofcontents` + "\n")
		}
	}
	//w.WriteString(`\listoffigures` + "\n")
	//w.WriteString(`\listoftables` + "\n")
}

var IsLetter = regexp.MustCompile(`^[a-zA-Z0-9]+$`).MatchString

func HeadlineHasTag(name string, p org.Headline) bool {
	tagName := name
	if IsLetter(tagName) {
		tagName = fmt.Sprintf("\\b%s\\b", name)
	}
	for _, t := range p.Tags {
		if ok, err := regexp.MatchString(tagName, t); err == nil && ok {
			return true
		}
	}
	return false
}
func (w *OrgLatexWriter) WriteDndBookAreas(name string, latexformat string, h org.Headline) bool {
	if HeadlineHasTag(name, h) {
		head := w.WriteNodesAsString(h.Title...)
		w.WriteString(fmt.Sprintf(latexformat, head) + "\n")
		if content := w.WriteNodesAsString(h.Children...); content != "" {
			w.WriteString(content)
		}
		return true
	}
	return false
}

func (w *OrgLatexWriter) WriteDndPropertyHeadline(p PropHead, h org.Headline) bool {
	if HeadlineHasTag(p.Tag, h) {
		head := w.WriteNodesAsString(h.Title...)
		w.WriteString("\n" + fmt.Sprintf(p.Format, head) + "\n")
		for _, prop := range p.Props {
			if v, ok := h.Properties.Get(prop); ok {
				fmt.Printf("HAVE PROP %s!!!\n", prop)
				if p.PropSquare {
					w.WriteString(fmt.Sprintf("  [%s]\n", v))
				} else {
					w.WriteString(fmt.Sprintf("  {%s}\n", v))
				}
			}
		}
		if content := w.WriteNodesAsString(h.Children...); content != "" {
			w.WriteString(content)
		}
		return true
	}
	return false
}

func (w *OrgLatexWriter) WriteDndRegionHeadline(p RegionHead, h org.Headline) bool {
	if HeadlineHasTag(p.Tag, h) {
		head := w.WriteNodesAsString(h.Title...)
		w.WriteString("\n" + fmt.Sprintf(p.Format, head) + "\n")
		for _, prop := range p.Props {
			if v, ok := h.Properties.Get(prop); ok {
				if p.PropSquare {
					w.WriteString(fmt.Sprintf("  [%s]\n", v))
				} else {
					w.WriteString(fmt.Sprintf("  {%s}\n", v))
				}
			}
		}
		if content := w.WriteNodesAsString(h.Children...); content != "" {
			w.WriteString(content)
		}
		w.WriteString("\n" + fmt.Sprintf(p.EndFormat, head) + "\n")
		return true
	}
	return false
}

// TODO: Can this be removed?
var simpleDndHeadlines = [][]string{
	{"SUBAREA", `\DndSubArea{%s}`},
	{"AREA", `\DndArea{%s}`},
}

type PropHead struct {
	Tag        string
	Format     string
	Props      []string
	PropSquare bool // Are properties in {} or []
}

type RegionHead struct {
	Tag        string
	Format     string
	EndFormat  string
	Props      []string
	PropSquare bool // Are properties in {} or []
}

func (w *OrgLatexWriter) SpecialHeaders(h org.Headline) bool {
	defer func() {
		if err := recover(); err != nil {
			head := w.WriteNodesAsString(h.Title...)
			log.Println("\n!!!!!!!!!!!!!!!!!\n!!!!!!!!!!!!!!!!!!!!!!!!!\npanic occurred:", err, "\n", head, "\n+++++++++++++++++++++++++++++++++++++++++++\n")
		}
	}()
	if w.templateRegistry.HaveHeadingOverrides() {
		// TODO Rethink how this works so we can work with fallbacks!
		for tag, temp := range w.templateRegistry.HeadingOverrides() {
			if HeadlineHasTag(tag, h) {
				props := map[string]interface{}{}
				if h.Properties != nil && len(h.Properties.Properties) > 0 {
					for _, prop := range h.Properties.Properties {
						if len(prop[0]) > 0 {
							props[prop[0]] = prop[1]
						}
					}
				}
				head := ""
				if len(h.Title) > 0 {
					head = w.WriteNodesAsString(h.Title...)
				}
				content := ""
				if len(h.Children) > 0 {
					content = w.WriteNodesAsString(h.Children...)
				}
				props["heading"] = head
				props["content"] = content
				res := w.exporter.pm.Tempo.RenderTemplateString(temp.Template, props)
				if res != "" {
					fmt.Printf("HEADING EXPANSION: \n[%s]\n------------------------\n", res)
					w.WriteString(res)
					return true
				} else {
					fmt.Printf("ERROR: Heading template failed to expand! %s\n", tag)
				}
				break
			}
		}
	}
	return false
}

func (w *OrgLatexWriter) WriteDndBookSpecialHeadlines(h org.Headline) bool {
	return w.SpecialHeaders(h)
}

func (w *OrgLatexWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}
	if w.WriteDndBookSpecialHeadlines(h) {
		return
	}
	// Clamp to max level
	lvl := h.Lvl
	if lvl > len(sectionTypes)-1 {
		lvl = len(sectionTypes) - 1
	}
	tlvl := h.Lvl
	// This is tricky, lets you shift heading values.
	// So if you want to start at 1 then you just set this
	// to 1. Lets you use book but skip chapters.
	shift := w.Document.Get("LATEX_HEADING_SHIFT")
	if shift != "" {
		if s, err := strconv.Atoi(shift); err == nil && s <= (len(sectionTypes)-2) {
			tlvl += s
		}
	}
	if tlvl < 1 {
		tlvl = 1
	}
	if tlvl > len(sectionTypes) {
		tlvl = len(sectionTypes)
	}
	if tmp, ok := w.templateRegistry.HeadingTemplate(fmt.Sprintf("%d", tlvl), false); ok {
		props := w.TemplateProps()
		// Copy over local properties for this headline so they can be
		// used in this template
		tp := &map[string]any{}
		for k, v := range *props {
			(*tp)[k] = v
		}
		if h.Properties != nil {
			for _, d := range (*h.Properties).Properties {
				if d != nil && len(d) >= 1 {
					k := d[0]
					v := []string{}
					if len(d) > 1 {
						v = d[1:]
					}
					(*tp)[k] = v
				}
			}
		}
		showtodo := w.Document.GetOption("todo") != "nil" && h.Status != ""
		(*tp)["showtodo"] = showtodo
		(*tp)["status"] = ""
		if showtodo {
			(*tp)["status"] = h.Status
		}
		showpriority := w.Document.GetOption("pri") != "nil" && h.Priority != ""
		(*tp)["showpriority"] = showpriority
		(*tp)["priority"] = ""
		if showtodo {
			(*tp)["priority"] = h.Priority
		}
		head := w.WriteNodesAsString(h.Title...)
		(*tp)["heading"] = head
		showtags := w.Document.GetOption("tags") != "nil" && len(h.Tags) != 0
		(*tp)["showtags"] = showtags
		(*tp)["tags"] = []string{}
		if showtodo {
			(*tp)["tags"] = h.Tags
		}
		numberPrefix := ""
		if w.Document.GetOption("num") != "nil" {
			if num, err := strconv.Atoi(w.Document.GetOption("num")); err == nil {
				if lvl > num {
					numberPrefix = "*"
				}
			}
		}
		(*tp)["numprefix"] = numberPrefix
		(*tp)["content"] = ""
		if content := w.WriteNodesAsString(h.Children...); content != "" {
			(*tp)["content"] = content
		}
		res := w.exporter.pm.Tempo.RenderTemplateString(tmp.Template, *tp)
		w.WriteString(res)
	} else {
		sectionFormat := sectionTypes[lvl]
		head := ""

		if w.Document.GetOption("todo") != "nil" && h.Status != "" {
			head += fmt.Sprintf("%s ", h.Status)
		}
		if w.Document.GetOption("pri") != "nil" && h.Priority != "" {
			head += fmt.Sprintf(`[%s] `, h.Priority)
		}

		head += w.WriteNodesAsString(h.Title...)
		if w.Document.GetOption("tags") != "nil" && len(h.Tags) != 0 {
			head += strings.Join(h.Tags, " ")
		}
		numberPrefix := ""
		fmt.Printf("EXPORT TEST: %s\n", w.Document.GetOption("num"))
		if w.Document.GetOption("num") != "nil" {
			if num, err := strconv.Atoi(w.Document.GetOption("num")); err == nil {
				if lvl > num {
					numberPrefix = "*"
				}
			}
		}
		w.WriteString("\n" + fmt.Sprintf(sectionFormat, numberPrefix, head))
		w.WriteString("\n")
		if content := w.WriteNodesAsString(h.Children...); content != "" {
			w.WriteString(content)
		}
	}
}

func (w *OrgLatexWriter) WriteText(t org.Text) {
	fmt.Printf("%s\n", t.Content)
	w.WriteString(t.Content)
}

// DONE
func (w *OrgLatexWriter) WriteEmphasis(e org.Emphasis) {
	tags, ok := emphasisTags[e.Kind]
	if !ok {
		panic(fmt.Sprintf("bad emphasis %#v", e))
	}
	out := w.WriteNodesAsString(e.Content...)
	w.WriteString(fmt.Sprintf(tags, out))
}

func (w *OrgLatexWriter) WriteLatexFragment(l org.LatexFragment) {
	w.WriteString(l.OpeningPair)
	org.WriteNodes(w, l.Content...)
	w.WriteString(l.ClosingPair)
}

// TODO: Statistic? What is this? Can we format this better?
func (w *OrgLatexWriter) WriteStatisticToken(s org.StatisticToken) {
	w.WriteString(fmt.Sprintf(`\begin{verbatim}[%s]\end{verbatim}`, s.Content))
}

func (w *OrgLatexWriter) WriteLineBreak(l org.LineBreak) {
	if w.Document.GetOption("ealb") == "nil" || !l.BetweenMultibyteCharacters {
		w.WriteString(strings.Repeat("\n", l.Count))
	}
}

func (w *OrgLatexWriter) WriteExplicitLineBreak(l org.ExplicitLineBreak) {
	//w.WriteString(`\newline\noindent\rule{\textwidth}{0.5pt}\n`)
	w.WriteString(`\break`)
}

func (w *OrgLatexWriter) WriteFootnoteLink(l org.FootnoteLink) {
	if w.Document.GetOption("f") == "nil" {
		return
	}
	i := w.footnotes.add(l)
	id := i + 1
	w.WriteString(fmt.Sprintf(`\footnotemark[%d]`, id))
}

func (w *OrgLatexWriter) WriteTimestamp(t org.Timestamp) {
	if w.Document.GetOption("<") == "nil" {
		return
	}
	// TODO: Add datetimestamp capability
	/*
		w.WriteString(`<span class="timestamp">`)
		bs, be := "", ""
		if t.Time.TimestampType == org.Active {
			bs, be = "&lt;", "&gt;"
		} else if t.Time.TimestampType == org.Inactive {
			bs, be = "&lsqb;", "&rsqb;"
		}
		var od org.OrgDate = *t.Time
		od.TimestampType = org.NoBracket
		w.WriteString(fmt.Sprintf("%s%s%s</span>", bs, od.ToString(), be))
	*/
	/*
		if t.IsDate {
			w.WriteString(t.Time.Format(datestampFormat))
		} else {
			w.WriteString(t.Time.Format(timestampFormat))
		}
		if t.Interval != "" {
			w.WriteString(" " + t.Interval)
		}*/
}

func (w *OrgLatexWriter) WriteSDC(s org.SDC) {
	if w.Document.GetOption("<") == "nil" {
		return
	}
	/*
		name := ""
		switch s.DateType {
		case org.Scheduled:
			name = "SCHEDULED"
			break
		case org.Deadline:
			name = "DEADLINE"
			break
		case org.Closed:
			name = "CLOSED"
			break
		}
		w.WriteString(fmt.Sprintf(`<span class="tags">%s`, name))
		w.WriteString(`</span>`)
		bs, be := "", ""
		if s.Date.TimestampType == org.Active {
			bs, be = "&lt;", "&gt;"
		} else if s.Date.TimestampType == org.Inactive {
			bs, be = "&lsqb;", "&rsqb;"
		}
		w.WriteString(fmt.Sprintf(`<span class="timestamp">%s`, bs))
		dt := s.Date
		dt.TimestampType = org.NoBracket
		w.WriteString(fmt.Sprintf("%s", dt.ToString()))
		w.WriteString(fmt.Sprintf(`%s</span>`, be))
	*/
}

func (w *OrgLatexWriter) WriteClock(s org.Clock) {
	if w.Document.GetOption("<") == "nil" {
		return
	}
	/*
		name := "CLOCK"
		w.WriteString(fmt.Sprintf(`<span class="tags">%s`, name))
		w.WriteString(`</span>`)
		bs, be := "&lsqb;", "&rsqb;"
		w.WriteString(fmt.Sprintf(`<span class="timestamp">%s`, bs))
		dt := s.Date
		end := ""
		if !dt.End.IsZero() {
			end = "--" + bs + dt.End.Format("2006-01-02 Mon 15:04") + be
		}
		tm := bs + dt.Start.Format("2006-01-02 Mon 15:04") + be + end
		w.WriteString(tm)
		w.WriteString(`</span>`)
	*/
}

func (w *OrgLatexWriter) WriteRegularLink(l org.RegularLink) {
	url := l.URL

	if l.Protocol == "file" {
		url = url[len("file:"):]
	}
	if isRelative := l.Protocol == "file" || l.Protocol == ""; isRelative && w.PrettyRelativeLinks {
		if !strings.HasPrefix(url, "/") {
			url = "../" + url
		}
		if strings.HasSuffix(url, ".org") {
			url = strings.TrimSuffix(url, ".org") + "/"
		}
	} else if isRelative && strings.HasSuffix(url, ".org") {
		url = strings.TrimSuffix(url, ".org") + ".html"
	}
	if prefix := w.Document.Links[l.Protocol]; prefix != "" {
		if tag := strings.TrimPrefix(l.URL, l.Protocol+":"); strings.Contains(prefix, "%s") || strings.Contains(prefix, "%h") {
			url = html.EscapeString(strings.ReplaceAll(strings.ReplaceAll(prefix, "%s", tag), "%h", tag))
		} else {
			url = html.EscapeString(prefix) + tag
		}
	} else if prefix := w.Document.Links[l.URL]; prefix != "" {
		url = html.EscapeString(strings.ReplaceAll(strings.ReplaceAll(prefix, "%s", ""), "%h", ""))
	}
	switch l.Kind() {
	case "image":
		if l.Description == nil {
			w.WriteString(fmt.Sprintf(`\begin{figure}
  \centering
    \includegraphics[width=.5\textwidth]{%s}
\end{figure}
`, url))
		} else {
			description := strings.TrimPrefix(org.String(l.Description...), "file:")
			w.WriteString(fmt.Sprintf(`\begin{figure}[h!]
  \centering
    \includegraphics[width=.5\textwidth]{%s}
    \caption{%s}
\end{figure}
`, url, description))
		}
	case "video":
		/*
			if l.Description == nil {
				w.WriteString(fmt.Sprintf(`<video src="%s" title="%s">%s</video>`, url, url, url))
			} else {
				description := strings.TrimPrefix(String(l.Description...), "file:")
				w.WriteString(fmt.Sprintf(`<a href="%s"><video src="%s" title="%s"></video></a>`, url, description, description))
			}
		*/
	default:
		description := url
		if l.Description != nil {
			description = w.WriteNodesAsString(l.Description...)
		}
		w.WriteString(fmt.Sprintf(`\ref{%s}{%s}`, url, description))
	}
}

func (w *OrgLatexWriter) WriteMacro(m org.Macro) {
	if macro := w.Document.Macros[m.Name]; macro != "" {
		for i, param := range m.Parameters {
			macro = strings.Replace(macro, fmt.Sprintf("$%d", i+1), param, -1)
		}
		macroDocument := w.Document.Parse(strings.NewReader(macro), w.Document.Path)
		if macroDocument.Error != nil {
			w.log.Printf("bad macro: %s -> %s: %v", m.Name, macro, macroDocument.Error)
		}
		org.WriteNodes(w, macroDocument.Nodes...)
	}
}

func (w *OrgLatexWriter) WriteList(l org.List) {
	tags, ok := listTags[l.Kind]
	if !ok {
		panic(fmt.Sprintf("bad list kind %#v", l))
	}
	w.WriteString(tags[0] + "\n")
	org.WriteNodes(w, l.Items...)
	w.WriteString(tags[1] + "\n")
}

func (w *OrgLatexWriter) WriteListItem(li org.ListItem) {
	//attributes := ""
	//if li.Value != "" {
	//	attributes += fmt.Sprintf(` value="%s"`, li.Value)
	//}
	//if li.Status != "" {
	//	attributes += fmt.Sprintf(` class="%s"`, listItemStatuses[li.Status])
	//}
	//w.WriteString(fmt.Sprintf("<li%s>", attributes))
	w.WriteString(`\item `)
	w.writeListItemContent(li.Children)
	w.WriteString("\n")
}

func (w *OrgLatexWriter) WriteDescriptiveListItem(di org.DescriptiveListItem) {
	//if di.Status != "" {
	//	w.WriteString(fmt.Sprintf("<dt class=\"%s\">\n", listItemStatuses[di.Status]))
	//} else {
	//	w.WriteString("<dt>\n")
	//}

	w.WriteString(`\item{`)
	if len(di.Term) != 0 {
		org.WriteNodes(w, di.Term...)
	} else {
		w.WriteString("?")
	}
	w.WriteString(`} `)
	w.writeListItemContent(di.Details)
	w.WriteString("\n")
}

func (w *OrgLatexWriter) writeListItemContent(children []org.Node) {
	if isParagraphNodeSlice(children) {
		for i, c := range children {
			out := w.WriteNodesAsString(c.(org.Paragraph).Children...)
			if i != 0 && out != "" {
				w.WriteString("\n")
			}
			w.WriteString(EscapeString(out))
		}
	} else {
		w.WriteString("\n")
		org.WriteNodes(w, children...)
	}
}

/*
	SDOC: Exporters::Latex

** Latex Paragraph

		Paragraphs, much like most org nodes
		can be customized. By default the use the
		standard latex par tag. Note the safe
		filter. Paragraphs are run through latex escaping
		by default so you will want to avoid the pongo template
		escaping to avoid ampersand and other html style escaping
		that are default for this framwork.

		#+BEGIN_SRC yaml
	    paragraph:
	      default:
	        template: |+
	          \par {{content | safe}}
	    #+END_SRC

EDOC
*/
func (w *OrgLatexWriter) WriteParagraph(p org.Paragraph) {
	if len(p.Children) == 0 {
		return
	}
	out := EscapeString(w.WriteNodesAsString(p.Children...))
	if tmp, ok := w.templateRegistry.ParagraphTemplate("default"); ok {
		w.RenderContentTemplate(tmp.Template, out)
	} else {
		// TODO: Remove this!
		if w.docclass != "dndbook" {
			w.WriteString(`\par `)
		}
		w.WriteString(out)
	}
}

func (w *OrgLatexWriter) WriteExample(e org.Example) {
	w.WriteString(`\begin{verbatim}`)
	if len(e.Children) != 0 {
		for _, n := range e.Children {
			org.WriteNodes(w, n)
			w.WriteString("\n")
		}
	}
	w.WriteString(`\end{verbatim}`)
}

func (w *OrgLatexWriter) WriteHorizontalRule(h org.HorizontalRule) {
	//w.WriteString(`\rulefill` + "\n")
	w.WriteString(`\newline\noindent\rule{\textwidth}{0.5pt}` + "\n")
}

func (w *OrgLatexWriter) WriteNodeWithMeta(n org.NodeWithMeta) {
	caption := ""
	if len(n.Meta.Caption) != 0 {
		for i, ns := range n.Meta.Caption {
			if i != 0 {
				caption += " "
			}
			caption += w.WriteNodesAsString(ns...)
		}
	}

	d := EnvData{n.Meta.LatexEnv, n.Meta.LatexAttributes, caption}
	w.envs.Push(d)
	out := w.WriteNodesAsString(n.Node)
	if p, ok := n.Node.(org.Paragraph); ok {
		if len(p.Children) == 1 && isImageOrVideoLink(p.Children[0]) {
			out = w.WriteNodesAsString(p.Children[0])
		}
	}
	w.WriteString(out)
	w.envs.Pop()
}

func (w *OrgLatexWriter) WriteNodeWithName(n org.NodeWithName) {
	org.WriteNodes(w, n.Node)
}

func GetAlign(i int, t org.Table) string {
	if i >= 0 && i < len(t.ColumnInfos) {
		switch t.ColumnInfos[i].Align {
		case "left":
			return "l"
		case "right":
			return "r"
		case "center":
			return "c"
		default:
			return "c"
		}
	}
	return "c"
}

/*
func dndFormatSpell(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		inVar, ok := in.Interface().(map[string]interface{})
		if ok && inVar != nil {
			front := ""
			end := ""
			output := ""
			for k, v := range inVar {
				key := strings.TrimSpace(k)
				val, vok := v.(string)
				if vok && key != "" && key != "cantrip" {
					val = strings.TrimSpace(val)
					if val != "" {
						front += "[" + key + "]"
						if end != "" {
							end += ", "
						}
						end += val
					}
				}
			}
			output = front + "{" + end + "}"
			output = strings.TrimSpace(output)
			return pongo2.AsValue(output), nil
		}
	}
	return in, nil
}
*/

type KeyVal struct {
	Key string
	Val []string
}

func (w *OrgLatexWriter) SpecialTable(name string, t org.Table) bool {
	defer func() {
		if err := recover(); err != nil {
			log.Println("\n!!!!!!!!!!!!!!!!!\n!!!!!!!!!!!!!!!!!!!!!!!!!\npanic occurred:", err)
		}
	}()
	fmt.Printf("  == [Special Table] ==\n")
	fmt.Printf("    == [CONF] ==\n")
	if tbl, ok := w.templateRegistry.TableTemplate(name); ok {
		props := map[string]interface{}{}
		if !tbl.Vertical {
			// Single row tables can access by name
			// without a table reference
			// Name is 0 row
			// | A  | B  | C  | D  |
			// | v1 | v2 | v3 | v4 |
			for i, nameNode := range t.Rows[0].Columns {
				cellName := w.WriteNodesAsString(nameNode.Children...)
				cellName = strings.ToLower(cellName)
				cellName = strings.ReplaceAll(cellName, "[", "")
				cellName = strings.ReplaceAll(cellName, "]", "")
				cellName = strings.ReplaceAll(cellName, "{", "")
				cellName = strings.ReplaceAll(cellName, "}", "")
				cellName = strings.ReplaceAll(cellName, " ", "")
				fmt.Printf("  NAME: %s\n", cellName)
				val := w.WriteNodesAsString(t.Rows[1].Columns[i].Children...)
				props[cellName] = val
			}
			tbl := []map[string]interface{}{}
			for r, row := range t.Rows {
				if r == 0 {
					continue
				}
				rr := map[string]interface{}{}
				for c, nameNode := range t.Rows[0].Columns {
					cellName := w.WriteNodesAsString(nameNode.Children...)
					cellName = strings.ToLower(cellName)
					cellName = strings.ReplaceAll(cellName, " ", "")
					val := w.WriteNodesAsString(row.Columns[c].Children...)
					rr[cellName] = val
				}
				tbl = append(tbl, rr)
			}
			props["tbl"] = tbl
			ctbl := []KeyVal{}
			for c, nameNode := range t.Rows[0].Columns {
				cellName := w.WriteNodesAsString(nameNode.Children...)
				cellName = strings.ToLower(cellName)
				cellName = strings.ReplaceAll(cellName, " ", "")
				rr := []string{}
				for r, row := range t.Rows {
					if r == 0 {
						continue
					}
					val := w.WriteNodesAsString(row.Columns[c].Children...)
					val = strings.TrimSpace(val)
					if val != "" {
						rr = append(rr, val)
					}
				}
				if cellName == "cantrip" {
					cellName = ""
				}
				pair := KeyVal{Key: cellName, Val: rr}
				ctbl = append(ctbl, pair)
			}
			props["ctbl"] = ctbl
		} else {
			// Vertical table does not get the multiple row option
			// It's a key value pairing
			//
			// | key1 | value1 |
			// | key2 | value2 |
			//
			for _, nameRow := range t.Rows {
				nameNode := nameRow.Columns[0]
				name := w.WriteNodesAsString(nameNode.Children...)
				name = strings.ToLower(name)
				name = strings.TrimSpace(strings.ReplaceAll(name, " ", ""))
				if name != "" {
					fmt.Printf("  NAME: %s\n", name)
					val := ""
					for j, colNode := range nameRow.Columns {
						if j > 0 {
							tmp := strings.TrimSpace(w.WriteNodesAsString(colNode.Children...))
							if tmp != "" {
								if val != "" {
									val += ", "
								}
								val += tmp
							}
						}
					}
					if val != "" {
						props[name] = val
					}
				}
			}
		}
		res := w.exporter.pm.Tempo.RenderTemplateString(tbl.Template, props)
		if res != "" {
			fmt.Printf("TABLE EXPANSION: \n[%s]\n------------------------\n", res)
			w.WriteString(res)
			return true
		} else {
			fmt.Printf("ERROR: Table template failed to expand! %s\n", name)
		}
	} else {
		fmt.Printf("[%s] is NOT in conf table entries\n", name)
	}
	return false
}

func (w *OrgLatexWriter) HandleDndSpecialTables(t org.Table) bool {
	e := w.envs.GetEnv("")
	fmt.Printf("CHECKING FOR STATS: %s\n", e)
	if e != "" && w.SpecialTable(e, t) {
		return true
	}
	return false
}

type TableRow struct {
	Isspecial bool
	Cols      []string
}

/*
	SDOC: Exporters::Latex

** Latex Tables

		There are several methods of working with tables.
		In on mode the table is exported verbatim

		In another the table is exported using heading lookups
		as properties for the table. This supports the macro
		approach found in the DND Latex module AND the standard
		latex table model.

		This is the default template found in book_templates.yaml
		Since book is the default export mode used by the latex exporter

		This table is built as a tabular object with & separators
		(See pongo2 for more details on the template format)

		#+BEGIN_SRC yaml
	  default:
	    vertical: false
	    template: |+
	      {% if havecaption %}\begin{table}[!h]{% endif %}
	      \begin{center}
	      {{ envs | startenv: "tabular" }}{{ separators }}
	      {% for row in rows %}
	        {{% if row.Isspecial %}}
	          \hline
	        {% else %}
	          {{ row.Cols | sepList: "&"" }} \\
	        {% endif %}
	      {% endfor %}
	      {{ envs | endenv: "tabular" }}
	      {% if havecaption %}\caption{ {{caption}} }{% endif %}
	      \end{center}
	      {% if havecaption %}\end{table}{% endif %}
		#+END_SRC

EDOC
*/
func (w *OrgLatexWriter) WriteTable(t org.Table) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("\n!!!!!!!!!!!!!!!!!\n!!!!!!!!!!!!!!!!!!!!!!!!!\npanic occurred in table:", err, "\n", "\n+++++++++++++++++++++++++++++++++++++++++++\n")
			panic("PANIC WRITING TABLE")
		}
	}()
	// Dndbook does its own formatting
	if w.docclass == "dndbook" {
		// TODO Handle this one!
		if w.HandleDndSpecialTables(t) {
			return
		}
	}
	// TODO: Handle this one in a template!
	if w.docclass == "dndbook" {
		name := ""
		if w.envs.HaveCaption() {
			name = w.envs.GetCaption()
		}
		//tableEnv = fmt.Sprintf("DndTable | [header=%s]", name)
		fmt.Sprintf("DndTable | [header=%s]", name)
		//w.startEnv(tableEnv)
	}
	cnt := len(t.ColumnInfos)
	sep := ""
	// Dndbook does its own formatting
	if w.docclass == "dndbook" {
		for i := 0; i < cnt; i++ {
			sep += "X"
		}
	} else {
		for i := 0; i < cnt; i++ {
			sep += " | " + GetAlign(i, t)
		}
		sep += " |"
	}
	tp := w.TemplateProps()
	(*tp)["separators"] = strings.TrimSpace(sep)
	// REMOVED IN FAVOR OF {{ separators }} w.WriteString(fmt.Sprintf("{%s}\n", sep))
	// TODO DND Flow will need this somehow?

	//inHead := len(t.SeparatorIndices) > 0 &&
	//	t.SeparatorIndices[0] != len(t.Rows)-1 &&
	//	(t.SeparatorIndices[0] != 0 || len(t.SeparatorIndices) > 1 && t.SeparatorIndices[len(t.SeparatorIndices)-1] != len(t.Rows)-1)

	(*tp)["curtable"] = t

	rows := []*TableRow{}
	for _, row := range t.Rows {
		cols := []string{}
		for _, col := range row.Columns {
			txt := w.WriteNodesAsString(col.Children...)
			cols = append(cols, txt)
		}
		row := TableRow{
			Isspecial: row.IsSpecial,
			Cols:      cols}
		rows = append(rows, &row)
	}
	(*tp)["rows"] = rows

	/*
		for i, row := range t.Rows {
			if len(row.Columns) == 0 && i != 0 && i != len(t.Rows)-1 {
				if inHead {
					inHead = false
				}
			}
			if row.IsSpecial {
				w.WriteString(`\hline` + "\n")
				continue
			}
			if inHead {
				w.writeTableColumns(row.Columns)
			} else {
				w.writeTableColumns(row.Columns)
			}
		}

		w.endEnv(tableEnv)

	*/
	// TODO: Make this work if special tables did not expand

	if tbl, ok := w.templateRegistry.TableTemplate("default"); ok {
		if w.envs.HaveCaption() {
			(*tp)["havecaption"] = true
			(*tp)["caption"] = w.envs.GetCaption()
		} else {
			(*tp)["havecaption"] = false
		}
		fmt.Printf("RENDERING TEMPLATE: %s\n", "default")
		fmt.Printf("\n%v\n", tbl.Template)
		res := w.exporter.pm.Tempo.RenderTemplateString(tbl.Template, *tp)
		fmt.Printf("RENDERED\n%v\n", res)
		w.WriteString(res)

	} else {
		fmt.Printf("FAILED TO RENDER TEMPLATE: %s\n", "default")
		// TODO: Show an error here! We failed
	}

	// TODO: Facilitate this write string!
	/*
		if haveTable && w.envs.HaveCaption() && w.docclass != "dndbook" {
			w.WriteString(fmt.Sprintf(`\caption{%s}`, w.envs.GetCaption()) + "\n")
		}
		// dndbook does its own formatting
		if w.docclass != "dndbook" {
			w.WriteString(`\end{center}` + "\n")
			if haveTable {
				w.WriteString(`\end{table}` + "\n")
			}
		}
	*/
}

// DEPRECATED REMOVE
func (w *OrgLatexWriter) writeTableColumns(columns []*org.Column) {

	for i, column := range columns {
		/*
			if column.Align == "" {
				w.WriteString(fmt.Sprintf("<%s>", tag))
			} else {
				w.WriteString(fmt.Sprintf(`<%s class="align-%s">`, tag, column.Align))
			}
		*/
		if i > 0 {
			w.WriteString(" & ")
		}
		org.WriteNodes(w, column.Children...)
	}
	w.WriteString(` \\ ` + "\n")
}

func (w *OrgLatexWriter) withHTMLAttributes(input string, kvs ...string) string {
	/*
		if len(kvs)%2 != 0 {
			w.log.Printf("withHTMLAttributes: Len of kvs must be even: %#v", kvs)
			return input
		}
		context := &h.Node{Type: h.ElementNode, Data: "body", DataAtom: atom.Body}
		nodes, err := h.ParseFragment(strings.NewReader(strings.TrimSpace(input)), context)
		if err != nil || len(nodes) != 1 {
			w.log.Printf("withHTMLAttributes: Could not extend attributes of %s: %v (%s)", input, nodes, err)
			return input
		}
		out, node := strings.Builder{}, nodes[0]
		for i := 0; i < len(kvs)-1; i += 2 {
			node.Attr = setHTMLAttribute(node.Attr, strings.TrimPrefix(kvs[i], ":"), kvs[i+1])
		}
		err = h.Render(&out, nodes[0])
		if err != nil {
			w.log.Printf("withHTMLAttributes: Could not extend attributes of %s: %v (%s)", input, node, err)
			return input
		}
		return out.String()
	*/
	return ""
}

func (w *OrgLatexWriter) blockContent(name string, children []org.Node) string {
	if isRawTextBlock(name) {
		builder := w.Builder
		w.Builder = strings.Builder{}
		org.WriteNodes(w, children...)
		out := w.String()
		w.Builder = builder
		return strings.TrimRightFunc(out, unicode.IsSpace)
	} else {
		return w.WriteNodesAsString(children...)
	}
}

/*
func setHTMLAttribute(attributes []h.Attribute, k, v string) []h.Attribute {
	for i, a := range attributes {
		if strings.ToLower(a.Key) == strings.ToLower(k) {
			switch strings.ToLower(k) {
			case "class", "style":
				attributes[i].Val += " " + v
			default:
				attributes[i].Val = v
			}
			return attributes
		}
	}
	return append(attributes, h.Attribute{Namespace: "", Key: k, Val: v})
}
*/

func isParagraphNodeSlice(ns []org.Node) bool {
	for _, n := range ns {
		if reflect.TypeOf(n).Name() != "Paragraph" {
			return false
		}
	}
	return true
}

func (fs *footnotes) add(f org.FootnoteLink) int {
	if i, ok := fs.mapping[f.Name]; ok && f.Name != "" {
		return i
	}
	fs.list = append(fs.list, f.Definition)
	i := len(fs.list) - 1
	if f.Name != "" {
		fs.mapping[f.Name] = i
	}
	return i
}

func (fs *footnotes) updateDefinition(f org.FootnoteDefinition) {
	if i, ok := fs.mapping[f.Name]; ok {
		fs.list[i] = &f
	}
}

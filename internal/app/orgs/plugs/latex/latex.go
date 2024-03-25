// EXPORTER: Latex Export

package gantt

import (
	"fmt"
	"html"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"gopkg.in/op/go-logging.v1"
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
	Props        map[string]interface{}
	out          *logging.Logger
	pm           *plugs.PluginManager
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

type OrgLatexWriter struct {
	ExtendingWriter org.Writer
	strings.Builder
	Document            *org.Document
	log                 *log.Logger
	footnotes           *footnotes
	PrettyRelativeLinks bool
	envs                EnvironmentStack
}

func NewOrgLatexWriter(exp *OrgLatexExporter) *OrgLatexWriter {
	defaultConfig := org.New()
	return &OrgLatexWriter{
		Document: &org.Document{Configuration: defaultConfig},
		footnotes: &footnotes{
			mapping: map[string]int{},
		},
	}
}

func EscapeString(out string) string {
	return out
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

func (self *OrgLatexExporter) Export(db plugs.ODb, query string, to string, opts string) error {
	fmt.Printf("LATEX: Export called", query, to, opts)
	err, str := self.ExportToString(db, query, opts)
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

func (self *OrgLatexExporter) ExportToString(db plugs.ODb, query string, opts string) (error, string) {
	self.Props = ValidateMap(self.Props)
	fmt.Printf("LATEX: Export string called [%s]:[%s]\n", query, opts)

	if f := db.FindByFile(query); f != nil {
		theme := f.Get("LATEX_THEME")
		if theme == "" {
			theme = f.Get("LATEX_CLASS")
		}
		self.Props["docclass"] = "book"
		if theme != "" {
			self.Props["docclass"] = theme
		}
		w := NewOrgLatexWriter(self)
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

func (self *OrgLatexExporter) Startup(manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	self.out = manager.Out
	self.pm = manager
}

func NewLatexExp() *OrgLatexExporter {
	var g *OrgLatexExporter = new(OrgLatexExporter)
	return g
}

var hljsver = "11.9.0"
var hljscdn = "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/" + hljsver

func GetStylesheet(name string) string {
	if data, err := os.ReadFile(plugs.PlugExpandTemplatePath("html_styles/" + name + "_style.css")); err == nil {
		// HACK: We probably do not alway want to do this. Need to think of a better way to handle this!
		re := regexp.MustCompile(`url\(([^)]+)\)`)
		return re.ReplaceAllString(string(data), "url(http://localhost:8010/${1})")
	}
	return ""
}

func ValidateMap(m map[string]interface{}) map[string]interface{} {
	force_reload_style := false
	if _, ok := m["title"]; !ok {
		m["title"] = "Schedule"
	}
	if _, ok := m["fontfamily"]; !ok {
		m["fontfamily"] = "Inconsolata"
	}
	if _, ok := m["trackheight"]; !ok {
		m["trackheight"] = 30
	}
	if _, ok := m["stylesheet"]; !ok || force_reload_style {
		m["stylesheet"] = GetStylesheet("default")
	}
	if _, ok := m["hljscdn"]; !ok {
		m["hljs_cdn"] = hljscdn
	}
	if _, ok := m["hljsstyle"]; !ok {
		m["hljs_style"] = "monokai"
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
	plugs.AddExporter("latex", func() plugs.Exporter {
		return &OrgLatexExporter{Props: ValidateMap(map[string]interface{}{}), TemplatePath: "latex_default.tpl"}
	})
}

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

func (w *OrgLatexWriter) Before(d *org.Document) {
	w.Document = d
	w.log = d.Log
	if title := d.Get("TITLE"); title != "" && w.Document.GetOption("title") != "nil" {
		titleDocument := d.Parse(strings.NewReader(title), d.Path)
		if titleDocument.Error == nil {
			title = w.WriteNodesAsString(titleDocument.Nodes...)
		}
		w.WriteString(fmt.Sprintf(`\title{%s}`+"\n", title))
	}
	if auth := d.Get("AUTHOR"); auth != "" && w.Document.GetOption("author") != "nil" {
		titleDocument := d.Parse(strings.NewReader(auth), d.Path)
		if titleDocument.Error == nil {
			auth = w.WriteNodesAsString(titleDocument.Nodes...)
		}
		w.WriteString(fmt.Sprintf(`\author{%s}`+"\n", auth))
	}
	if dt := d.Get("DATE"); dt != "" && w.Document.GetOption("date") != "nil" {
		titleDocument := d.Parse(strings.NewReader(dt), d.Path)
		if titleDocument.Error == nil {
			dt = w.WriteNodesAsString(titleDocument.Nodes...)
		}
		w.WriteString(fmt.Sprintf(`\date{%s}`+"\n", dt))
	}
	w.WriteString(`\begin{document}`)
	if w.Document.GetOption("toc") != "nil" {
		maxLvl, _ := strconv.Atoi(w.Document.GetOption("toc"))
		w.WriteOutline(d, maxLvl)
	}
}

func (w *OrgLatexWriter) After(d *org.Document) {
	w.WriteFootnotes(d)
	w.WriteString(`\end{document}`)
}

func (w *OrgLatexWriter) WriteComment(org.Comment)               {}
func (w *OrgLatexWriter) WritePropertyDrawer(org.PropertyDrawer) {}

func (w *OrgLatexWriter) startEnv(name string) {
	name = w.envs.GetEnv(name)
	w.WriteString(fmt.Sprintf(`\begin{%s}`, name) + "\n")
}

func (w *OrgLatexWriter) endEnv(name string) {
	name = w.envs.GetEnv(name)
	w.WriteString(fmt.Sprintf(`\end{%s}`, name) + "\n")
}

func (w *OrgLatexWriter) WriteBlock(b org.Block) {
	content, params := w.blockContent(b.Name, b.Children), b.ParameterMap()

	switch b.Name {
	case "SRC":
		// TODO: Source blocks still have to be converted... Going to need their own export flow
		if params[":exports"] == "results" || params[":exports"] == "none" {
			break
		}
		lang := "text"
		if len(b.Parameters) >= 1 {
			lang = strings.ToLower(b.Parameters[0])
		}
		// TODO content = w.HighlightCodeBlock(b.Keywords, content, lang, false, params)
		content = ""
		w.WriteString(fmt.Sprintf("<div class=\"src src-%s\">\n%s\n</div>\n", lang, content))
	case "EXAMPLE":
		w.startEnv("verbatim")
		w.WriteString(EscapeString(content))
		w.endEnv("verbatim")
	case "EXPORT":
		if len(b.Parameters) >= 1 && strings.ToLower(b.Parameters[0]) == "html" {
			w.WriteString(content + "\n")
		}
	case "QUOTE":
		w.startEnv("displayquote")
		w.WriteString(content)
		w.endEnv("displayquote")
	case "CENTER":
		w.WriteString(`\begin{center}\n\centering\n`)
		w.WriteString(content + `\end{center}\n`)
	default:
		w.startEnv(strings.ToLower(b.Name))
		w.WriteString(content)
		w.endEnv(strings.ToLower(b.Name))
	}

	if b.Result != nil && params[":exports"] != "code" && params[":exports"] != "none" {
		org.WriteNodes(w, b.Result)
	}
}

func (w *OrgLatexWriter) WriteResult(r org.Result) { org.WriteNodes(w, r.Node) }

func (w *OrgLatexWriter) WriteInlineBlock(b org.InlineBlock) {
	content := w.blockContent(strings.ToUpper(b.Name), b.Children)
	switch b.Name {
	case "src":
		// TODO: is there a better source block to be using here.
		//lang := strings.ToLower(b.Parameters[0])
		//TODO Convert content = w.HighlightCodeBlock(b.Keywords, content, lang, true, nil)
		//content = ""
		w.WriteString(`\begin{verbatim} ` + content + `\end{verbatim}` + "\n")
	case "export":
		if strings.ToLower(b.Parameters[0]) == "html" {
			w.WriteString(content)
		}
	}
}

func (w *OrgLatexWriter) WriteDrawer(d org.Drawer) {
	org.WriteNodes(w, d.Children...)
}

func (w *OrgLatexWriter) WriteKeyword(k org.Keyword) {
	if k.Key == "HTML" {
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
	w.WriteString(`\tableofcontents` + "\n")
	//w.WriteString(`\listoffigures` + "\n")
	//w.WriteString(`\listoftables` + "\n")
}

func (w *OrgLatexWriter) WriteHeadline(h org.Headline) {
	if h.IsExcluded(w.Document) {
		return
	}

	// Clamp to max level
	lvl := h.Lvl
	if lvl > len(sectionTypes)-1 {
		lvl = len(sectionTypes) - 1
	}
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
	w.WriteString(fmt.Sprintf(sectionFormat, numberPrefix, head))
	w.WriteString("\n")
	if content := w.WriteNodesAsString(h.Children...); content != "" {
		w.WriteString(content)
	}
}

func (w *OrgLatexWriter) WriteText(t org.Text) {
	if w.Document.GetOption("e") == "nil" || t.IsRaw {
		w.WriteString(EscapeString(t.Content))
	} else {
		w.WriteString(EscapeString(t.Content))
	}
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
	w.WriteString(`\newline\noindent\rule{\textwidth}{0.5pt}\n`)
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
	url := html.EscapeString(l.URL)

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
	/*
		case "image":
			if l.Description == nil {
				w.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" title="%s" />`, url, url, url))
			} else {
				description := strings.TrimPrefix(String(l.Description...), "file:")
				w.WriteString(fmt.Sprintf(`<a href="%s"><img src="%s" alt="%s" /></a>`, url, description, description))
			}
		case "video":
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
			w.WriteString(out)
		}
	} else {
		w.WriteString("\n")
		org.WriteNodes(w, children...)
	}
}

func (w *OrgLatexWriter) WriteParagraph(p org.Paragraph) {
	if len(p.Children) == 0 {
		return
	}
	w.WriteString(`\par `)
	org.WriteNodes(w, p.Children...)
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
	w.WriteString(`\rulefill` + "\n")
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

func (w *OrgLatexWriter) WriteTable(t org.Table) {
	haveTable := false
	if w.envs.HaveCaption() {
		w.WriteString(`\begin{table}[!h]` + "\n")
		haveTable = true
		w.WriteString(fmt.Sprintf(`\caption{%s}`, w.envs.GetCaption()) + "\n")
	}

	w.WriteString(`\begin{center}` + "\n")
	w.startEnv("tabular")
	cnt := len(t.ColumnInfos)
	sep := ""
	for i := 0; i < cnt; i++ {
		sep += " | " + GetAlign(i, t)
	}
	sep += " |"
	w.WriteString(fmt.Sprintf("{%s}\n", sep))

	inHead := len(t.SeparatorIndices) > 0 &&
		t.SeparatorIndices[0] != len(t.Rows)-1 &&
		(t.SeparatorIndices[0] != 0 || len(t.SeparatorIndices) > 1 && t.SeparatorIndices[len(t.SeparatorIndices)-1] != len(t.Rows)-1)
	/*
		if inHead {
			w.WriteString("<thead>\n")
		} else {
			w.WriteString("<tbody>\n")
		}
	*/
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
	w.endEnv("tabular")
	w.WriteString(`\end{center}` + "\n")
	if haveTable {
		w.WriteString(`\end{table}` + "\n")
	}
}

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

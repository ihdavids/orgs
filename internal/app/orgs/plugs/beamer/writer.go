package beamer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ihdavids/go-org/org"
)

// writer turns an org document into the body of a beamer presentation.
//
// It is its own writer rather than the latex exporter's, which renders every
// node through pongo2 templates chosen per document class and carries the D&D
// book's special headings: a deck needs none of that, and does need control
// over where frames, columns and blocks open and close, which a heading
// written one at a time by go-org cannot give. Inline nodes (text, emphasis,
// links) come through the org.Writer interface as usual; the outline is walked
// here, with the frame level in hand.
type writer struct {
	strings.Builder
	doc  *org.Document
	opts Options

	frameLevel int
	inFrame    bool
	// Set while a frame's body is written when it holds something verbatim,
	// which a frame has to be told about ([fragile]).
	fragile bool

	footdefs map[string]*org.FootnoteDefinition
	// Frame labels, by heading Index; and the heading a [[*Title]] or
	// [[#custom]] link names.
	labels   map[int]string
	byTitle  map[string]int
	byCustom map[string]int
	// Environments the file defines for itself (\newtheorem, \newenvironment
	// in its headers), which a special block may then name.
	userEnvs map[string]bool

	// #+ATTR_BEAMER: is not an affiliated keyword to go-org, so it arrives as
	// a keyword node of its own; it applies to the next list or table.
	attrBeamer map[string]string
	// The #+ATTR_LATEX: and #+CAPTION: of the node being written.
	attrLatex map[string]string
	caption   string
}

func newWriter(doc *org.Document, opts Options) *writer {
	w := &writer{
		doc:        doc,
		opts:       opts,
		frameLevel: opts.FrameLevel,
		footdefs:   map[string]*org.FootnoteDefinition{},
		labels:     map[int]string{},
		byTitle:    map[string]int{},
		byCustom:   map[string]int{},
		userEnvs:   map[string]bool{},
	}
	w.prescan()
	return w
}

// ---------------------------------------------------------------------------
// Before anything is written
// ---------------------------------------------------------------------------

var newEnvRe = regexp.MustCompile(`\\(?:new|renew|provide)(?:environment|theorem)\*?\{([^}]+)\}`)

func (w *writer) prescan() {
	var walk func(nodes []org.Node)
	walk = func(nodes []org.Node) {
		for _, n := range nodes {
			switch n := n.(type) {
			case org.FootnoteDefinition:
				d := n
				w.footdefs[n.Name] = &d
			case *org.FootnoteDefinition:
				w.footdefs[n.Name] = n
			case org.NodeWithMeta:
				walk([]org.Node{n.Node})
			case org.NodeWithName:
				walk([]org.Node{n.Node})
			}
			if h, ok := headlineOf(n); ok {
				title := strings.TrimSpace(org.String(h.Title...))
				if _, seen := w.byTitle[title]; !seen {
					w.byTitle[title] = h.Index
				}
				label := ""
				if id := prop(h, "CUSTOM_ID"); id != "" {
					w.byCustom[id] = h.Index
					label = id
				}
				if label == "" {
					label = fmt.Sprintf("sec-%d", h.Index)
				}
				w.labels[h.Index] = label
				walk(h.Children)
			}
		}
	}
	walk(w.doc.Nodes)
	for _, key := range []string{"LATEX_HEADER", "BEAMER_HEADER", "LATEX_HEADER_EXTRA"} {
		for _, m := range newEnvRe.FindAllStringSubmatch(w.doc.Get(key), -1) {
			w.userEnvs[strings.ToLower(m[1])] = true
		}
	}
}

// headlineOf is the heading a node is, whether go-org handed it over as a
// value (the top level) or a pointer (nested; see Traps: pointer trap).
func headlineOf(n org.Node) (org.Headline, bool) {
	switch h := n.(type) {
	case org.Headline:
		return h, true
	case *org.Headline:
		if h != nil {
			return *h, true
		}
	}
	return org.Headline{}, false
}

// ---------------------------------------------------------------------------
// The outline: sections, frames, blocks, columns
// ---------------------------------------------------------------------------

// prop is a heading property, "" when it has none. go-org upper-cases the
// names it reads, and org itself ignores their case.
func prop(h org.Headline, name string) string {
	if h.Properties == nil {
		return ""
	}
	for _, kv := range h.Properties.Properties {
		if len(kv) > 1 && strings.EqualFold(kv[0], name) {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

// The BEAMER_env of a heading, lower case: from its property, or from the
// B_env tag org puts beside it.
func envOf(h org.Headline) string {
	if e := prop(h, "BEAMER_env"); e != "" {
		return strings.ToLower(e)
	}
	for _, t := range h.Tags {
		if strings.HasPrefix(t, "B_") && len(t) > 2 {
			return strings.ToLower(t[2:])
		}
	}
	return ""
}

// An overlay specification as it is written into the tex: <2-> as is, 2- in
// angle brackets, and [<+->] (a default overlay) as is.
func overlay(act string) string {
	act = strings.TrimSpace(act)
	switch {
	case act == "":
		return ""
	case strings.HasPrefix(act, "<"), strings.HasPrefix(act, "["):
		return act
	}
	return "<" + act + ">"
}

func bracket(opt string) string {
	opt = strings.TrimSpace(opt)
	opt = strings.TrimSuffix(strings.TrimPrefix(opt, "["), "]")
	if opt == "" {
		return ""
	}
	return "[" + opt + "]"
}

func (w *writer) exported(h org.Headline) bool {
	// org's footnote section holds definitions, which are written where
	// they are referenced; the heading itself is never exported.
	if h.Lvl == 1 && strings.TrimSpace(org.String(h.Title...)) == "Footnotes" {
		return false
	}
	return !(h.IsComment || h.IsExcluded(w.doc) || h.IsArchived())
}

// The heading's title as it goes in the tex, with its keyword when the file
// exports keywords (org's todo: option).
func (w *writer) title(h org.Headline) string {
	t := strings.TrimSpace(w.WriteNodesAsString(h.Title...))
	if h.Status != "" && w.option("todo", "t") != "nil" {
		t = `\textbf{` + escape(h.Status) + `} ` + t
	}
	return t
}

// option is an #+OPTIONS: value, read from the file and then the defaults
// the document was parsed with, without go-org's complaint for a missing one.
func (w *writer) option(key, def string) string {
	for _, src := range []string{w.doc.BufferSettings["OPTIONS"], w.doc.DefaultSettings["OPTIONS"]} {
		for _, f := range strings.Fields(src) {
			if strings.HasPrefix(f, key+":") {
				return f[len(key)+1:]
			}
		}
	}
	return def
}

func splitBody(h org.Headline) (body []org.Node, kids []org.Node) {
	for _, n := range h.Children {
		if _, ok := headlineOf(n); ok {
			kids = append(kids, n)
		} else {
			body = append(body, n)
		}
	}
	return
}

// WriteHeadline is where go-org hands over each top level heading; the rest
// of the outline is walked from here.
func (w *writer) WriteHeadline(h org.Headline) { w.headline(h, "") }

func (w *writer) headline(h org.Headline, parentEnv string) {
	if !w.exported(h) {
		return
	}
	env := envOf(h)
	if env == "appendix" {
		w.WriteString("\\appendix\n")
		env = ""
	}
	body, kids := splitBody(h)
	switch env {
	case "ignoreheading":
		// The heading goes and its contents stay, where they are - in a
		// frame of their own when they are not inside one already.
		if !w.inFrame && strings.TrimSpace(w.WriteNodesAsString(body...)) != "" {
			w.frameOf("", "", "", body, nil, "")
		} else {
			w.nodes(body)
		}
		w.children(kids, parentEnv)
		return
	case "note", "notenh":
		w.WriteString("\\note")
		w.WriteString(overlay(prop(h, "BEAMER_act")))
		w.WriteString("{")
		if env == "note" {
			w.WriteString(`\textbf{` + w.title(h) + "}\n\n")
		}
		w.nodes(body)
		w.children(kids, env)
		w.WriteString("}\n")
		return
	case "againframe":
		ref := w.refLabel(prop(h, "BEAMER_ref"))
		if ref == "" {
			ref = w.refLabel(strings.TrimSpace(org.String(h.Title...)))
		}
		if ref == "" {
			w.WriteString("% againframe: " + escapeComment(prop(h, "BEAMER_ref")) + " names no frame\n")
			return
		}
		w.WriteString("\\againframe" + overlay(prop(h, "BEAMER_act")) + bracket(prop(h, "BEAMER_opt")) + "{" + ref + "}\n")
		return
	}
	switch {
	case !w.inFrame && (env == "frame" || env == "fullframe" || h.Lvl >= w.frameLevel):
		w.frame(h, env, body, kids)
	case !w.inFrame:
		w.section(h, body, kids)
	default:
		w.block(h, env, body, kids, parentEnv)
	}
}

func (w *writer) section(h org.Headline, body, kids []org.Node) {
	cmd := "section"
	switch {
	case h.Lvl == 2:
		cmd = "subsection"
	case h.Lvl >= 3:
		cmd = "subsubsection"
	}
	star := ""
	if w.option("num", "t") == "nil" {
		star = "*"
	}
	w.WriteString(fmt.Sprintf("\\%s%s{%s}\n", cmd, star, w.title(h)))
	// Text under a section heading, above its first frame, has nowhere to
	// go in beamer: a frame of its own, untitled, rather than lost.
	if strings.TrimSpace(w.WriteNodesAsString(body...)) != "" {
		w.frameOf("", "", "", body, nil, "")
	}
	w.children(kids, "")
}

func (w *writer) frame(h org.Headline, env string, body, kids []org.Node) {
	title := w.title(h)
	if env == "fullframe" {
		title = ""
	}
	opts := prop(h, "BEAMER_opt")
	if !strings.Contains(opts, "label=") {
		if l, ok := w.labels[h.Index]; ok {
			opts = joinOpts(opts, "label="+l)
		}
	}
	w.frameOf(title, overlay(prop(h, "BEAMER_act")), opts, body, kids, env)
}

func joinOpts(a, b string) string {
	a = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(a), "["), "]")
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "," + b
}

func hasOpt(opts, name string) bool {
	for _, o := range strings.Split(opts, ",") {
		o = strings.TrimSpace(o)
		if o == name || strings.HasPrefix(o, name+"=") {
			return true
		}
	}
	return false
}

func (w *writer) frameOf(title, act, opts string, body, kids []org.Node, env string) {
	w.inFrame, w.fragile = true, false
	saved := w.Builder
	w.Builder = strings.Builder{}
	w.nodes(body)
	w.children(kids, env)
	inner := w.String()
	w.Builder = saved
	if w.fragile && !hasOpt(opts, "fragile") {
		opts = joinOpts(opts, "fragile")
	}
	w.inFrame, w.fragile = false, false
	w.WriteString("\\begin{frame}" + act + bracket(opts))
	if title != "" {
		w.WriteString("{" + title + "}")
	}
	w.WriteString("\n")
	w.WriteString(strings.TrimRight(inner, "\n") + "\n")
	// A fragile frame is read verbatim until a line that is exactly this.
	w.WriteString("\\end{frame}\n\n")
}

// children writes the headings under a frame (or section), putting a run of
// columns into a columns environment of their own when the heading above
// them is not one already.
func (w *writer) children(kids []org.Node, parentEnv string) {
	open := false
	for _, n := range kids {
		h, ok := headlineOf(n)
		if !ok || !w.exported(h) {
			continue
		}
		isCol := w.inFrame && (envOf(h) == "column" || prop(h, "BEAMER_col") != "")
		if isCol && parentEnv != "columns" && !open {
			w.WriteString("\\begin{columns}\n")
			open = true
		}
		if !isCol && open {
			w.WriteString("\\end{columns}\n")
			open = false
		}
		w.headline(h, parentEnv)
	}
	if open {
		w.WriteString("\\end{columns}\n")
	}
}

// The environments a heading inside a frame may be, as ox-beamer spells
// them: %a is the overlay, %o the options, %h the title.
var blockEnvs = map[string][2]string{
	"block":          {`\begin{block}%a{%h}`, `\end{block}`},
	"alertblock":     {`\begin{alertblock}%a{%h}`, `\end{alertblock}`},
	"exampleblock":   {`\begin{exampleblock}%a{%h}`, `\end{exampleblock}`},
	"theorem":        {`\begin{theorem}%a%t`, `\end{theorem}`},
	"definition":     {`\begin{definition}%a%t`, `\end{definition}`},
	"example":        {`\begin{example}%a%t`, `\end{example}`},
	"proof":          {`\begin{proof}%a%t`, `\end{proof}`},
	"lemma":          {`\begin{lemma}%a%t`, `\end{lemma}`},
	"corollary":      {`\begin{corollary}%a%t`, `\end{corollary}`},
	"fact":           {`\begin{fact}%a%t`, `\end{fact}`},
	"quote":          {`\begin{quote}%a`, `\end{quote}`},
	"quotation":      {`\begin{quotation}%a`, `\end{quotation}`},
	"verse":          {`\begin{verse}%a`, `\end{verse}`},
	"structureenv":   {`\begin{structureenv}%a\structure{%h}`, `\end{structureenv}`},
	"beamercolorbox": {`\begin{beamercolorbox}%o{%h}`, `\end{beamercolorbox}`},
}

func expandEnv(tpl, act, opt, title string) string {
	t := ""
	if title != "" {
		t = "[" + title + "]"
	}
	r := strings.NewReplacer("%a", act, "%o", bracket(opt), "%h", title, "%t", t)
	return r.Replace(tpl)
}

func (w *writer) block(h org.Headline, env string, body, kids []org.Node, parentEnv string) {
	act := overlay(prop(h, "BEAMER_act"))
	opt := prop(h, "BEAMER_opt")
	switch env {
	case "columns":
		w.WriteString("\\begin{columns}" + bracket(opt) + "\n")
		w.nodes(body)
		w.children(kids, "columns")
		w.WriteString("\\end{columns}\n")
		return
	case "column":
		w.WriteString(fmt.Sprintf("\\begin{column}%s{%s}\n", bracket(opt), columnWidth(prop(h, "BEAMER_col"))))
		w.nodes(body)
		w.children(kids, "column")
		w.WriteString("\\end{column}\n")
		return
	}
	// A heading with a width but another environment is that environment,
	// in a column of that width (org's BEAMER_col alone).
	if col := prop(h, "BEAMER_col"); col != "" {
		w.WriteString(fmt.Sprintf("\\begin{column}{%s}\n", columnWidth(col)))
		defer w.WriteString("\\end{column}\n")
	}
	if env == "" || env == "frame" || env == "fullframe" {
		env = "block"
	}
	spec, ok := blockEnvs[env]
	if !ok {
		if w.userEnvs[env] {
			spec = [2]string{`\begin{` + env + `}%a%t`, `\end{` + env + `}`}
		} else {
			spec = blockEnvs["block"]
		}
	}
	title := w.title(h)
	envTitle := title
	// A theorem kind prints its own name; a heading that only repeats it
	// ("Definition" over a definition) would print it twice.
	if strings.EqualFold(strings.TrimSpace(org.String(h.Title...)), env) && strings.Contains(spec[0], "%t") {
		envTitle = ""
	}
	w.WriteString(expandEnv(spec[0], act, opt, envTitle) + "\n")
	if strings.Contains(spec[0], "quot") || strings.Contains(spec[0], "verse") {
		if title != "" {
			w.WriteString(`\textbf{` + title + "}\n\n")
		}
	}
	w.nodes(body)
	w.children(kids, env)
	w.WriteString(spec[1] + "\n")
}

var numberRe = regexp.MustCompile(`^[0-9]*\.?[0-9]+$`)

func columnWidth(col string) string {
	col = strings.TrimSpace(col)
	if col == "" {
		return `0.5\columnwidth`
	}
	if numberRe.MatchString(col) {
		return col + `\columnwidth`
	}
	return col
}

// refLabel is the frame label a BEAMER_ref or a link names: [[#id]], #id,
// [[*Title]], *Title, or a bare label.
func (w *writer) refLabel(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimSuffix(strings.TrimPrefix(ref, "[["), "]]")
	if i := strings.Index(ref, "]["); i >= 0 {
		ref = ref[:i]
	}
	switch {
	case ref == "":
		return ""
	case strings.HasPrefix(ref, "#"):
		if i, ok := w.byCustom[ref[1:]]; ok {
			return w.labels[i]
		}
		return ref[1:]
	case strings.HasPrefix(ref, "*"):
		if i, ok := w.byTitle[strings.TrimSpace(ref[1:])]; ok {
			return w.labels[i]
		}
		return ""
	}
	if i, ok := w.byTitle[ref]; ok {
		return w.labels[i]
	}
	if i, ok := w.byCustom[ref]; ok {
		return w.labels[i]
	}
	return ""
}

func (w *writer) nodes(nodes []org.Node) { org.WriteNodes(w, nodes...) }

// ---------------------------------------------------------------------------
// org.Writer
// ---------------------------------------------------------------------------

func (w *writer) Before(*org.Document)             {}
func (w *writer) After(*org.Document)              {}
func (w *writer) WriterWithExtensions() org.Writer { return w }
func (w *writer) NodeIdx(int)                      {}
func (w *writer) ResetLineBreak()                  {}

func (w *writer) WriteNodesAsString(nodes ...org.Node) string {
	saved := w.Builder
	w.Builder = strings.Builder{}
	org.WriteNodes(w, nodes...)
	out := w.String()
	w.Builder = saved
	return out
}

func (w *writer) WriteKeyword(k org.Keyword) {
	switch strings.ToUpper(k.Key) {
	case "LATEX", "BEAMER":
		w.WriteString(k.Value + "\n")
	case "ATTR_BEAMER":
		w.attrBeamer = attrs(k.Value)
	case "TOC":
		if strings.HasPrefix(strings.TrimSpace(k.Value), "headlines") {
			w.WriteString("\\tableofcontents\n")
		}
	}
}

// attrs reads `:overlay <+-> :environment itemize` into a map.
func attrs(s string) map[string]string {
	out := map[string]string{}
	key := ""
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, ":") {
			key = f[1:]
			out[key] = ""
			continue
		}
		if key != "" {
			out[key] = strings.TrimSpace(out[key] + " " + f)
		}
	}
	return out
}

func pairAttrs(lists [][]string) map[string]string {
	out := map[string]string{}
	for _, l := range lists {
		for i := 0; i+1 < len(l); i += 2 {
			out[strings.TrimPrefix(l[i], ":")] = strings.TrimSpace(l[i+1])
		}
		if len(l)%2 == 1 {
			out[strings.TrimPrefix(l[len(l)-1], ":")] = ""
		}
	}
	return out
}

func (w *writer) WriteInclude(i org.Include) {
	if i.Resolve != nil {
		org.WriteNodes(w, i.Resolve())
	}
}

func (w *writer) WriteComment(org.Comment)               {}
func (w *writer) WritePropertyDrawer(org.PropertyDrawer) {}
func (w *writer) WriteSDC(org.SDC)                       {}
func (w *writer) WriteClock(org.Clock)                   {}

func (w *writer) WriteDrawer(d org.Drawer) {
	if strings.EqualFold(d.Name, "LOGBOOK") || strings.EqualFold(d.Name, "PROPERTIES") {
		return
	}
	org.WriteNodes(w, d.Children...)
}

func (w *writer) WriteNodeWithMeta(n org.NodeWithMeta) {
	w.attrLatex = pairAttrs(n.Meta.LatexAttributes)
	w.caption = ""
	if len(n.Meta.Caption) > 0 {
		parts := []string{}
		for _, c := range n.Meta.Caption {
			parts = append(parts, w.WriteNodesAsString(c...))
		}
		w.caption = strings.TrimSpace(strings.Join(parts, " "))
	}
	org.WriteNodes(w, n.Node)
	w.attrLatex, w.caption = nil, ""
}

func (w *writer) WriteNodeWithName(n org.NodeWithName) { org.WriteNodes(w, n.Node) }

// go-org's text of a block: its children as plain strings, which for a
// source or example block are its lines.
func rawContent(children []org.Node) string {
	return strings.TrimRight(org.String(children...), "\n")
}

// The org language names listings knows by another name, or not at all.
var listingsLang = map[string]string{
	"python": "Python", "py": "Python", "sh": "bash", "bash": "bash", "shell": "bash", "zsh": "bash",
	"emacs-lisp": "Lisp", "elisp": "Lisp", "lisp": "Lisp", "scheme": "Lisp", "clojure": "Lisp",
	"c": "C", "cpp": "C++", "c++": "C++", "java": "Java", "ruby": "Ruby", "perl": "Perl",
	"sql": "SQL", "sqlite": "SQL", "html": "HTML", "xml": "XML", "latex": "TeX", "tex": "TeX",
	"haskell": "Haskell", "matlab": "Matlab", "octave": "Octave", "r": "R", "fortran": "Fortran",
	"pascal": "Pascal", "make": "make", "makefile": "make", "lua": "[5.2]Lua", "php": "PHP",
	"tcl": "tcl", "csharp": "[Sharp]C", "awk": "Awk", "erlang": "erlang", "ocaml": "[Objective]Caml",
	"go": "orgsGo", "golang": "orgsGo", "javascript": "orgsJavaScript", "js": "orgsJavaScript",
	"typescript": "orgsJavaScript", "ts": "orgsJavaScript", "rust": "orgsRust", "yaml": "orgsYAML", "json": "orgsJSON",
}

// The minted lexer for an org language name.
var mintedLang = map[string]string{
	"emacs-lisp": "common-lisp", "elisp": "common-lisp", "sh": "bash", "shell": "bash",
	"js": "javascript", "ts": "typescript", "py": "python", "c++": "cpp", "golang": "go",
}

func (w *writer) code(lang, src string, numbered bool) {
	w.fragile = true
	lang = strings.ToLower(strings.TrimSpace(lang))
	if w.opts.Code == "minted" {
		lex := lang
		if l, ok := mintedLang[lang]; ok {
			lex = l
		}
		if lex == "" {
			lex = "text"
		}
		opt := ""
		if numbered {
			opt = "[linenos]"
		}
		w.WriteString(fmt.Sprintf("\\begin{minted}%s{%s}\n%s\n\\end{minted}\n", opt, lex, src))
		return
	}
	opts := []string{}
	if l, ok := listingsLang[lang]; ok {
		opts = append(opts, "language="+l)
	}
	if numbered {
		// Numbers sit in the margin unless the listing makes room for them.
		opts = append(opts, "numbers=left", "xleftmargin=2em")
	}
	opt := ""
	if len(opts) > 0 {
		opt = "[" + strings.Join(opts, ",") + "]"
	}
	w.WriteString(fmt.Sprintf("\\begin{lstlisting}%s\n%s\n\\end{lstlisting}\n", opt, listingText(src)))
}

// listings reads its text a byte at a time, so anything that is not ascii
// (ü, —, “) stops pdflatex. Each run of it is handed back to latex through
// the escapeinside the preamble sets up, where inputenc (or xelatex) reads
// it as the text it is.
func listingText(src string) string {
	out := strings.Builder{}
	in := false
	for _, r := range src {
		wide := r > 127
		if wide && !in {
			out.WriteString("(*@")
		}
		if !wide && in {
			out.WriteString("@*)")
		}
		in = wide
		if wide {
			out.WriteString(escape(string(r)))
		} else {
			out.WriteRune(r)
		}
	}
	if in {
		out.WriteString("@*)")
	}
	return out.String()
}

func verbatim(src string) string {
	return "\\begin{verbatim}\n" + src + "\n\\end{verbatim}\n"
}

func (w *writer) WriteBlock(b org.Block) {
	params := b.ParameterMap()
	name := strings.ToUpper(b.Name)
	switch name {
	case "SRC":
		exports := params[":exports"]
		if exports != "results" && exports != "none" {
			lang := ""
			if len(b.Parameters) > 0 {
				lang = b.Parameters[0]
			}
			numbered := false
			for _, s := range b.Switches {
				if s == "-n" || s == "+n" {
					numbered = true
				}
			}
			w.code(lang, rawContent(b.Children), numbered)
		}
		if b.Result != nil && exports != "code" && exports != "none" {
			org.WriteNodes(w, b.Result)
		}
		return
	case "EXAMPLE":
		w.fragile = true
		w.WriteString(verbatim(rawContent(b.Children)))
	case "EXPORT":
		if len(b.Parameters) > 0 {
			switch strings.ToLower(b.Parameters[0]) {
			case "latex", "beamer", "tex":
				w.WriteString(rawContent(b.Children) + "\n")
			}
		}
	case "COMMENT":
	case "QUOTE":
		w.WriteString("\\begin{quote}\n" + w.WriteNodesAsString(b.Children...) + "\\end{quote}\n")
	case "CENTER":
		w.WriteString("\\begin{center}\n" + w.WriteNodesAsString(b.Children...) + "\\end{center}\n")
	case "VERSE":
		w.WriteString("\\begin{verse}\n" + strings.ReplaceAll(strings.TrimRight(w.WriteNodesAsString(b.Children...), "\n"), "\n", "\\\\\n") + "\n\\end{verse}\n")
	case "NOTES", "NOTE":
		// Speaker notes, as the html slide exporters read them.
		w.WriteString("\\note{" + w.WriteNodesAsString(b.Children...) + "}\n")
	default:
		env := strings.ToLower(b.Name)
		body := w.WriteNodesAsString(b.Children...)
		if _, ok := blockEnvs[env]; ok || w.userEnvs[env] {
			title := ""
			if len(b.Parameters) > 0 {
				title = escape(strings.Join(b.Parameters, " "))
			}
			act := overlay(w.attrBeamer["overlay"])
			w.attrBeamer = nil
			spec, ok := blockEnvs[env]
			if !ok {
				spec = [2]string{`\begin{` + env + `}%a%t`, `\end{` + env + `}`}
			}
			w.WriteString(expandEnv(spec[0], act, "", title) + "\n" + body + spec[1] + "\n")
			return
		}
		// An environment nothing defines would stop the compile: its text.
		w.WriteString(body)
	}
}

func (w *writer) WriteResult(r org.Result) { org.WriteNodes(w, r.Node) }

func (w *writer) WriteInlineBlock(b org.InlineBlock) {
	switch strings.ToLower(b.Name) {
	case "src":
		w.WriteString(`\texttt{` + escape(org.String(b.Children...)) + `}`)
	case "export":
		if len(b.Parameters) > 0 {
			switch strings.ToLower(b.Parameters[0]) {
			case "latex", "beamer", "tex":
				w.WriteString(org.String(b.Children...))
			}
		}
	}
}

func (w *writer) WriteExample(e org.Example) {
	w.fragile = true
	lines := []string{}
	for _, c := range e.Children {
		lines = append(lines, org.String(c))
	}
	w.WriteString(verbatim(strings.TrimRight(strings.Join(lines, "\n"), "\n")))
}

func (w *writer) WriteFootnoteDefinition(org.FootnoteDefinition) {}

func (w *writer) WriteFootnoteLink(l org.FootnoteLink) {
	def := l.Definition
	if def == nil {
		def = w.footdefs[l.Name]
	}
	if def == nil {
		return
	}
	body := strings.TrimSpace(w.WriteNodesAsString(def.Children...))
	w.WriteString(`\footnote{` + body + `}`)
}

func (w *writer) WriteList(l org.List) {
	env := "itemize"
	switch l.Kind {
	case "ordered":
		env = "enumerate"
	case "descriptive":
		env = "description"
	}
	a := w.attrBeamer
	w.attrBeamer = nil
	if e := a["environment"]; e != "" {
		env = e
	}
	opt := ""
	if o := strings.TrimSpace(a["overlay"]); o != "" {
		if !strings.HasPrefix(o, "<") && !strings.HasPrefix(o, "[") {
			o = "<" + o + ">"
		}
		if !strings.HasPrefix(o, "[") {
			o = "[" + o + "]"
		}
		opt = o
	}
	if o := a["options"]; o != "" && opt == "" {
		opt = bracket(o)
	}
	w.WriteString("\\begin{" + env + "}" + opt + "\n")
	org.WriteNodes(w, l.Items...)
	w.WriteString("\\end{" + env + "}\n")
}

// Matched in the written item, where the brackets are already escaped.
var itemOverlayRe = regexp.MustCompile(`^\s*\\textless\{\}([0-9+\-,|.a-z ]+)\\textgreater\{\}\s*`)

func checkbox(status string) string {
	switch status {
	case "X":
		return `$\boxtimes$ `
	case "-":
		return `$\boxminus$ `
	case " ":
		return `$\square$ `
	}
	return ""
}

func (w *writer) WriteListItem(li org.ListItem) {
	content := w.WriteNodesAsString(li.Children...)
	item := `\item`
	// `- <2-> point` is a point that arrives on the second step.
	if m := itemOverlayRe.FindStringSubmatch(content); m != nil {
		item += "<" + m[1] + ">"
		content = content[len(m[0]):]
	}
	w.WriteString(item + " " + checkbox(li.Status) + strings.TrimRight(content, "\n") + "\n")
}

func (w *writer) WriteDescriptiveListItem(di org.DescriptiveListItem) {
	term := strings.TrimSpace(w.WriteNodesAsString(di.Term...))
	w.WriteString(`\item[` + checkbox(di.Status) + term + "] " + strings.TrimRight(w.WriteNodesAsString(di.Details...), "\n") + "\n")
}

func (w *writer) WriteParagraph(p org.Paragraph) {
	if img, ok := loneImage(p.Children); ok {
		w.image(img, true)
		return
	}
	text := strings.TrimSpace(w.WriteNodesAsString(p.Children...))
	if text != "" {
		w.WriteString(text + "\n\n")
	}
}

// A paragraph that is nothing but a picture is a figure.
func loneImage(nodes []org.Node) (org.RegularLink, bool) {
	var found *org.RegularLink
	for _, n := range nodes {
		switch n := n.(type) {
		case org.RegularLink:
			if found != nil || n.Kind() != "image" {
				return org.RegularLink{}, false
			}
			l := n
			found = &l
		case org.Text:
			if strings.TrimSpace(n.Content) != "" {
				return org.RegularLink{}, false
			}
		case org.LineBreak:
		default:
			return org.RegularLink{}, false
		}
	}
	if found == nil {
		return org.RegularLink{}, false
	}
	return *found, true
}

// The file a picture link names, absolute, so the deck compiles wherever it
// is compiled: relative to the org file, as org reads it. A .svg has a .pdf
// beside it more often than pdflatex can read the svg.
func (w *writer) imagePath(l org.RegularLink) string {
	p := l.URL
	if l.Protocol == "file" {
		p = strings.TrimPrefix(strings.TrimPrefix(p, "file:"), "//")
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) && w.doc != nil && w.doc.Path != "" {
		p = filepath.Join(filepath.Dir(w.doc.Path), p)
	}
	if strings.EqualFold(filepath.Ext(p), ".svg") {
		if alt := strings.TrimSuffix(p, filepath.Ext(p)) + ".pdf"; fileExists(alt) {
			p = alt
		}
	}
	return filepath.ToSlash(p)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (w *writer) image(l org.RegularLink, alone bool) {
	if l.Protocol == "http" || l.Protocol == "https" {
		// A picture on the web cannot be put in a pdf by latex.
		w.WriteString(`\url{` + escapeURL(l.URL) + "}\n")
		return
	}
	opts := []string{}
	a := w.attrLatex
	for _, k := range []string{"width", "height", "scale", "angle"} {
		if v := a[k]; v != "" {
			opts = append(opts, k+"="+v)
		}
	}
	if o := a["options"]; o != "" {
		opts = append(opts, strings.TrimSuffix(strings.TrimPrefix(o, "["), "]"))
	}
	if len(opts) == 0 {
		// Fits the frame whichever way the picture is shaped.
		opts = []string{`width=.9\linewidth`, `height=.7\textheight`, "keepaspectratio"}
	}
	inc := fmt.Sprintf(`\includegraphics[%s]{%s}`, strings.Join(opts, ","), w.imagePath(l))
	if !alone {
		w.WriteString(inc)
		return
	}
	if w.caption != "" {
		w.WriteString("\\begin{figure}\n\\centering\n" + inc + "\n\\caption{" + w.caption + "}\n\\end{figure}\n")
		return
	}
	w.WriteString("\\begin{center}\n" + inc + "\n\\end{center}\n")
}

func (w *writer) WriteTable(t org.Table) {
	cols := 0
	for _, r := range t.Rows {
		if len(r.Columns) > cols {
			cols = len(r.Columns)
		}
	}
	if cols == 0 {
		return
	}
	align := w.attrLatex["align"]
	if align == "" {
		spec := strings.Builder{}
		for i := 0; i < cols; i++ {
			a := "l"
			if i < len(t.ColumnInfos) {
				switch t.ColumnInfos[i].Align {
				case "right":
					a = "r"
				case "center":
					a = "c"
				}
			}
			spec.WriteString(a)
		}
		align = spec.String()
	}
	w.attrBeamer = nil
	body := strings.Builder{}
	body.WriteString("\\begin{tabular}{" + align + "}\n\\toprule\n")
	for i, r := range t.Rows {
		if len(r.Columns) == 0 {
			// A rule; the first and last are the table's own top and bottom.
			if i != 0 && i != len(t.Rows)-1 {
				body.WriteString("\\midrule\n")
			}
			continue
		}
		if r.IsSpecial {
			continue
		}
		cells := []string{}
		for _, c := range r.Columns {
			cells = append(cells, strings.TrimSpace(w.WriteNodesAsString(c.Children...)))
		}
		for len(cells) < cols {
			cells = append(cells, "")
		}
		body.WriteString(strings.Join(cells, " & ") + " \\\\\n")
	}
	body.WriteString("\\bottomrule\n\\end{tabular}\n")
	if w.caption != "" {
		w.WriteString("\\begin{table}\n\\centering\n" + body.String() + "\\caption{" + w.caption + "}\n\\end{table}\n")
		return
	}
	w.WriteString("\\begin{center}\n" + body.String() + "\\end{center}\n")
}

func (w *writer) WriteHorizontalRule(org.HorizontalRule) {
	w.WriteString("\\par\\noindent\\rule{\\linewidth}{0.4pt}\\par\n")
}

func (w *writer) WriteText(t org.Text) {
	if t.IsRaw {
		w.WriteString(escape(t.Content))
		return
	}
	w.WriteString(entities(t.Content))
}

var emphasis = map[string][2]string{
	"*":   {`\textbf{`, `}`},
	"/":   {`\emph{`, `}`},
	"_":   {`\uline{`, `}`},
	"+":   {`\sout{`, `}`},
	"_{}": {`\textsubscript{`, `}`},
	"^{}": {`\textsuperscript{`, `}`},
}

func (w *writer) WriteEmphasis(e org.Emphasis) {
	switch e.Kind {
	case "~", "=":
		w.WriteString(`\texttt{` + escape(org.String(e.Content...)) + `}`)
		return
	}
	tags, ok := emphasis[e.Kind]
	if !ok {
		org.WriteNodes(w, e.Content...)
		return
	}
	w.WriteString(tags[0])
	org.WriteNodes(w, e.Content...)
	w.WriteString(tags[1])
}

// Mathematics is latex already, and is written as it was.
func (w *writer) WriteLatexFragment(l org.LatexFragment) {
	w.WriteString(l.OpeningPair + org.String(l.Content...) + l.ClosingPair)
}

func (w *writer) WriteStatisticToken(s org.StatisticToken) {
	w.WriteString(escape("[" + s.Content + "]"))
}

func (w *writer) WriteExplicitLineBreak(org.ExplicitLineBreak) { w.WriteString("\\\\\n") }

func (w *writer) WriteLineBreak(l org.LineBreak) {
	w.WriteString(strings.Repeat("\n", l.Count))
}

func (w *writer) WriteTimestamp(t org.Timestamp) {
	if t.Time == nil {
		return
	}
	w.WriteString(escape(t.Time.ToString()))
}

func (w *writer) WriteRegularLink(l org.RegularLink) {
	desc := ""
	if l.Description != nil {
		desc = strings.TrimSpace(w.WriteNodesAsString(l.Description...))
	}
	if l.Kind() == "image" && l.Description == nil {
		w.image(l, false)
		return
	}
	switch l.Protocol {
	case "http", "https", "mailto", "ftp", "doi":
		u := l.URL
		if l.Protocol == "doi" {
			u = "https://doi.org/" + strings.TrimPrefix(u, "doi:")
		}
		if desc == "" {
			w.WriteString(`\url{` + escapeURL(u) + `}`)
			return
		}
		w.WriteString(`\href{` + escapeURL(u) + `}{` + desc + `}`)
		return
	case "", "file":
		// [[*Title]] and [[#id]] are frames of this deck.
		if l.Protocol == "" && (strings.HasPrefix(l.URL, "*") || strings.HasPrefix(l.URL, "#")) {
			if label := w.refLabel(l.URL); label != "" {
				if desc == "" {
					desc = escape(strings.TrimLeft(l.URL, "*#"))
				}
				w.WriteString(`\hyperlink{` + label + `}{` + desc + `}`)
				return
			}
		}
	}
	if desc == "" {
		desc = escape(l.URL)
	}
	w.WriteString(desc)
}

func (w *writer) WriteMacro(m org.Macro) {
	def, ok := w.doc.Macros[m.Name]
	if !ok {
		switch strings.ToLower(m.Name) {
		case "title", "author", "email", "date", "subtitle":
			w.WriteString(escape(w.doc.Get(strings.ToUpper(m.Name))))
		}
		return
	}
	for i, p := range m.Parameters {
		def = strings.ReplaceAll(def, fmt.Sprintf("$%d", i+1), p)
	}
	d := w.doc.Parse(strings.NewReader(def), w.doc.Path)
	if len(d.Nodes) == 1 {
		if p, ok := d.Nodes[0].(org.Paragraph); ok {
			org.WriteNodes(w, p.Children...)
			return
		}
	}
	org.WriteNodes(w, d.Nodes...)
}

func (w *writer) WriteTarget(t org.Target) {
	w.WriteString(`\hypertarget{` + escapeLabel(t.Name) + `}{}`)
}

func (w *writer) WriteBabelCall(c org.BabelCall) {
	if c.Result != nil {
		org.WriteNodes(w, c.Result)
	}
}

// ---------------------------------------------------------------------------
// Escaping
// ---------------------------------------------------------------------------

var escaper = strings.NewReplacer(
	`\`, `\textbackslash{}`,
	`{`, `\{`, `}`, `\}`,
	`$`, `\$`, `&`, `\&`, `#`, `\#`, `%`, `\%`, `_`, `\_`,
	`~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`,
	`<`, `\textless{}`, `>`, `\textgreater{}`,
)

// escape makes text safe to put in a tex file as text.
func escape(s string) string { return escaper.Replace(s) }

// \url and \href read their argument nearly verbatim; these are what still
// has to be escaped in one.
var urlEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `#`, `\#`, `{`, `\{`, `}`, `\}`)

func escapeURL(s string) string { return urlEscaper.Replace(s) }

var labelBad = regexp.MustCompile(`[^A-Za-z0-9:._-]`)

func escapeLabel(s string) string { return labelBad.ReplaceAllString(s, "-") }

func escapeComment(s string) string { return strings.ReplaceAll(s, "\n", " ") }

// Org's entities (\alpha, \to, \nbsp) as latex. go-org leaves them in the
// text; anything not in the table is text with its backslash.
var entityTable = map[string]string{
	"alpha": `$\alpha$`, "beta": `$\beta$`, "gamma": `$\gamma$`, "delta": `$\delta$`, "epsilon": `$\epsilon$`,
	"zeta": `$\zeta$`, "eta": `$\eta$`, "theta": `$\theta$`, "iota": `$\iota$`, "kappa": `$\kappa$`,
	"lambda": `$\lambda$`, "mu": `$\mu$`, "nu": `$\nu$`, "xi": `$\xi$`, "pi": `$\pi$`, "rho": `$\rho$`,
	"sigma": `$\sigma$`, "tau": `$\tau$`, "upsilon": `$\upsilon$`, "phi": `$\phi$`, "chi": `$\chi$`,
	"psi": `$\psi$`, "omega": `$\omega$`, "Gamma": `$\Gamma$`, "Delta": `$\Delta$`, "Theta": `$\Theta$`,
	"Lambda": `$\Lambda$`, "Pi": `$\Pi$`, "Sigma": `$\Sigma$`, "Phi": `$\Phi$`, "Psi": `$\Psi$`, "Omega": `$\Omega$`,
	"to": `$\to$`, "rarr": `$\rightarrow$`, "larr": `$\leftarrow$`, "rArr": `$\Rightarrow$`, "lArr": `$\Leftarrow$`,
	"harr": `$\leftrightarrow$`, "uarr": `$\uparrow$`, "darr": `$\downarrow$`,
	"le": `$\le$`, "ge": `$\ge$`, "ne": `$\ne$`, "approx": `$\approx$`, "pm": `$\pm$`, "times": `$\times$`,
	"cdot": `$\cdot$`, "infin": `$\infty$`, "infty": `$\infty$`, "deg": `\textdegree{}`, "middot": `\textperiodcentered{}`,
	"nbsp": `~`, "ldots": `\ldots{}`, "dots": `\dots{}`, "hellip": `\dots{}`, "mdash": `---`, "ndash": `--`,
	"copy": `\textcopyright{}`, "reg": `\textregistered{}`, "trade": `\texttrademark{}`, "euro": `\texteuro{}`,
	"pound": `\pounds{}`, "S": `\S{}`, "checkmark": `$\checkmark$`, "star": `$\star$`, "dagger": `\dag{}`,
}

var entityRe = regexp.MustCompile(`\\([A-Za-z]+)(\{\})?`)

func entities(s string) string {
	out := strings.Builder{}
	last := 0
	for _, m := range entityRe.FindAllStringSubmatchIndex(s, -1) {
		name := s[m[2]:m[3]]
		rep, ok := entityTable[name]
		if !ok {
			continue
		}
		out.WriteString(escape(s[last:m[0]]))
		out.WriteString(rep)
		last = m[1]
	}
	out.WriteString(escape(s[last:]))
	return out.String()
}

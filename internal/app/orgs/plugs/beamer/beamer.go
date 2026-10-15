// EXPORTER: Beamer

package beamer

/* SDOC: Exporters

* Beamer
	An org file as a LaTeX Beamer presentation, read the way Emacs' ox-beamer
	reads it, so a deck written for =C-c C-e l b= comes out the same.

	#+BEGIN_SRC sh
	orgs export -f beamer talk.org -out talk.tex
	orgs export -f beamer-pdf talk.org -out talk.pdf
	orgs export -f beamer talk.org -theme dark
	#+END_SRC

	It needs no settings, so it is there whether or not the yaml names it.
	To change its defaults:

	#+BEGIN_SRC yaml
  - name: "beamer"
    theme: "clean"        # a preset (below) or any beamer theme's name
    code: "listings"      # or minted (needs -shell-escape and pygments)
    latex: "xelatex"      # the engine the pdf is made with
    frameLevel: 1         # org's H: when the file does not say
    aspectRatio: "169"    # when the file has no #+LATEX_CLASS_OPTIONS:
	#+END_SRC

** The outline
	Headings above the frame level are =\section=, =\subsection=,
	=\subsubsection=; headings at it are frames; headings below it are blocks.
	The frame level is =H:= in =#+OPTIONS:= (1 when the file says nothing, as
	in org: every top heading a frame). =toc:t= puts an outline frame after the
	title, =num:nil= unnumbers the sections, =todo:nil= leaves keywords out
	of titles.

	A heading's =:BEAMER_env:= property (or org's =B_env= tag) says what it is:

	| =frame=, =fullframe=      | a frame here, whatever its level; fullframe has no title |
	| =block=, =alertblock=, =exampleblock= | beamer's blocks (=block= is the default below a frame) |
	| =theorem=, =definition=, =example=, =proof=, =lemma=, =corollary=, =fact= | the theorem kinds, titled |
	| =quote=, =quotation=, =verse=, =structureenv=, =beamercolorbox= | as beamer has them |
	| =column=, =columns=       | a column (=:BEAMER_col: 0.5= is half the width) and the row of them; a run of columns gets its =columns= without asking |
	| =note=, =noteNH=          | speaker notes, with and without the heading's title |
	| =ignoreheading=           | the heading goes and what is under it stays |
	| =appendix=                | =\appendix= before it |
	| =againframe=              | the frame =:BEAMER_ref:= names (=[[#id]]= or =[[*Title]]=), shown again |

	=:BEAMER_act:= is an overlay (=<2->=, or =[<+->]= for a default one),
	=:BEAMER_opt:= the frame's or block's options (=allowframebreaks=,
	=label=...). A frame with a source or example block in it is made
	=fragile= without being asked.

** The rest of the file
	=#+ATTR_BEAMER: :overlay <+->= before a list uncovers it a point at a time
	(=:environment= changes the list's environment); =- <2-> point= is one
	point's own overlay. =#+ATTR_LATEX: :width .5\linewidth= sizes a picture,
	=#+CAPTION:= captions a picture or table. Source blocks are =listings=
	(=-n= numbers them), or minted when the exporter says so. =#+BEGIN_NOTES=
	is speaker notes, as the html slide exporters read it.
	=#+BEGIN_EXPORT latex= and =@@beamer:...@@= go in as they are, and so do
	=#+LATEX:= and =#+BEAMER:= lines.

	The head of the deck: =#+TITLE=, =#+SUBTITLE=, =#+AUTHOR=, =#+EMAIL=,
	=#+DATE=, =#+INSTITUTE= (or =#+BEAMER_INSTITUTE=), =#+BEAMER_THEME:
	Madrid [options]=, =#+BEAMER_COLOR_THEME=, =#+BEAMER_FONT_THEME=,
	=#+BEAMER_INNER_THEME=, =#+BEAMER_OUTER_THEME=, =#+BEAMER_HEADER= and
	=#+LATEX_HEADER= lines, and =#+LATEX_CLASS_OPTIONS: [aspectratio=43]=.

** Themes
	A file that names a =#+BEAMER_THEME:= gets that theme. One that does not
	gets a preset: the exporter's =theme=, or =-theme= on the command line
	(which wins over the file). With neither, =metropolis= when it is
	installed and =clean= when it is not.

	| =clean=      | white, one blue, a short rule under each title, page numbers; plain beamer, nothing to install |
	| =dark=       | =clean= on a dark ground                                                  |
	| =metropolis= | the metropolis theme with a progress bar under the title (=clean= when it is not installed) |
	| =classic=    | Madrid: the conference look                                               |
	| =academic=   | Boadilla in dolphin blues, a footline with author, title and page         |
	| =minimal=    | black on white and nothing else                                           |
EDOC */

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Options are what a deck is written with, beyond what its file says.
type Options struct {
	// A preset name, or a beamer theme's.
	Theme string
	// "listings" or "minted".
	Code string
	// The frame level when the file has no H:.
	FrameLevel int
	// aspectratio= when the file has no #+LATEX_CLASS_OPTIONS:.
	AspectRatio string
}

// OrgBeamerExporter is the exporter's settings, as the yaml gives them.
type OrgBeamerExporter struct {
	Theme       string `yaml:"theme"`
	Code        string `yaml:"code"`
	Latex       string `yaml:"latex"`
	FrameLevel  int    `yaml:"frameLevel"`
	AspectRatio string `yaml:"aspectRatio"`
	pm          *common.PluginManager
}

func (self *OrgBeamerExporter) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *OrgBeamerExporter) Startup(manager *common.PluginManager, opts *common.PluginOpts) {
	self.pm = manager
}

// Standalone: a deck needs no settings, so the exporter is there whether or
// not the yaml lists it.
func (self *OrgBeamerExporter) Standalone() bool { return true }

// Engine is the latex the pdf is made with, as the yaml says it ("" when it
// does not).
func (self *OrgBeamerExporter) Engine() string { return self.Latex }

// Options for one export: the exporter's, with what the request asks for on
// top.
func (self *OrgBeamerExporter) Options(props map[string]string) Options {
	o := Options{Theme: self.Theme, Code: self.Code, FrameLevel: self.FrameLevel, AspectRatio: self.AspectRatio}
	if t := strings.TrimSpace(props["theme"]); t != "" {
		o.Theme = t
	}
	if c := strings.TrimSpace(props["code"]); c != "" {
		o.Code = c
	}
	return o
}

func (self *OrgBeamerExporter) ExportToString(db common.ODb, query string, opts string, props map[string]string) (error, string) {
	doc := db.FindByFile(query)
	if doc == nil {
		return fmt.Errorf("beamer: no org file called %s", query), ""
	}
	return nil, Render(doc, self.Options(props), props["theme"] != "")
}

// Export writes the tex at to, or the pdf when to ends in .pdf.
func (self *OrgBeamerExporter) Export(db common.ODb, query string, to string, opts string, props map[string]string) error {
	err, tex := self.ExportToString(db, query, opts, props)
	if err != nil {
		return err
	}
	if strings.HasSuffix(strings.ToLower(to), ".pdf") {
		pdf, err := Compile(tex, CompileOptions{Engine: self.Latex, ShellEscape: self.Options(props).Code == "minted"})
		if err != nil {
			return err
		}
		return os.WriteFile(to, pdf, 0644)
	}
	return os.WriteFile(to, []byte(tex), 0644)
}

func init() {
	common.AddExporter("beamer", func() common.Exporter { return &OrgBeamerExporter{} })
}

// ---------------------------------------------------------------------------
// The deck
// ---------------------------------------------------------------------------

// Render writes a document as a beamer deck. themeAsked says the theme in
// opts was asked for by this request, and so wins over the file's own.
func Render(doc *org.Document, opts Options, themeAsked bool) string {
	if opts.Code != "minted" {
		opts.Code = "listings"
	}
	w := newWriter(doc, opts)
	// org's H: - the file's, then the exporter's, then org's own 1.
	w.frameLevel = 1
	if opts.FrameLevel > 0 {
		w.frameLevel = opts.FrameLevel
	}
	if h := w.option("H", ""); h != "" {
		fmt.Sscanf(h, "%d", &w.frameLevel)
	}
	if w.frameLevel < 1 {
		w.frameLevel = 1
	}

	// The body first: the head depends on nothing in it, but writing it is
	// what finds out whether the deck has any code in it.
	body := strings.Builder{}
	for _, n := range doc.Nodes {
		if _, ok := headlineOf(n); ok {
			body.WriteString(w.WriteNodesAsString(n))
			continue
		}
		// Above the first heading: keywords (#+LATEX: lines) are written,
		// and text has no frame to go in, so it gets one.
		switch n.(type) {
		case org.Keyword, org.Comment:
			body.WriteString(w.WriteNodesAsString(n))
		default:
			if s := strings.TrimSpace(w.WriteNodesAsString(n)); s != "" {
				saved := w.Builder
				w.Builder = strings.Builder{}
				w.frameOf("", "", "", []org.Node{n}, nil, "")
				body.WriteString(w.String())
				w.Builder = saved
			}
		}
	}

	out := strings.Builder{}
	out.WriteString(preamble(doc, opts, w, themeAsked))
	out.WriteString("\\begin{document}\n\n")
	if doc.Get("TITLE") != "" && w.option("title", "t") != "nil" {
		out.WriteString("\\maketitle\n\n")
	}
	if toc := w.option("toc", "t"); toc != "nil" {
		depth := ""
		if n := 0; strings.Trim(toc, "0123456789") == "" {
			fmt.Sscanf(toc, "%d", &n)
			depth = fmt.Sprintf("\\setcounter{tocdepth}{%d}\n", n)
		}
		if hasSections(doc, w.frameLevel) {
			out.WriteString("\\begin{frame}{" + outlineTitle(doc) + "}\n" + depth + "\\tableofcontents\n\\end{frame}\n\n")
		}
	}
	out.WriteString(body.String())
	out.WriteString("\\end{document}\n")
	return out.String()
}

// An outline frame lists sections; a deck with none would get an empty one.
func hasSections(doc *org.Document, frameLevel int) bool {
	found := false
	var walk func(nodes []org.Node)
	walk = func(nodes []org.Node) {
		for _, n := range nodes {
			if h, ok := headlineOf(n); ok {
				env := envOf(h)
				if h.Lvl < frameLevel && env != "frame" && env != "fullframe" && env != "ignoreheading" {
					found = true
					return
				}
				walk(h.Children)
			}
		}
	}
	walk(doc.Nodes)
	return found
}

func outlineTitle(doc *org.Document) string {
	if t := doc.Get("BEAMER_OUTLINE_TITLE"); t != "" {
		return escape(t)
	}
	return "Outline"
}

// `Madrid [height=20pt]` is the theme Madrid with those options.
var themeArgRe = regexp.MustCompile(`^\s*(\S+)\s*(\[.*\])?\s*$`)

func useTheme(kind, value string) string {
	m := themeArgRe.FindStringSubmatch(value)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("\\use%stheme%s{%s}\n", kind, m[2], m[1])
}

// A stamp in #+DATE: as a date a person reads; anything else as written.
var dateStampRe = regexp.MustCompile(`^[<\[](\d{4}-\d{2}-\d{2})[^>\]]*[>\]]$`)

func dateOf(s string) string {
	s = strings.TrimSpace(s)
	if m := dateStampRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return escape(s)
}

func inline(doc *org.Document, w *writer, s string) string {
	d := doc.Parse(strings.NewReader(s), doc.Path)
	if len(d.Nodes) == 1 {
		if p, ok := d.Nodes[0].(org.Paragraph); ok {
			return strings.TrimSpace(w.WriteNodesAsString(p.Children...))
		}
	}
	return escape(s)
}

func preamble(doc *org.Document, opts Options, w *writer, themeAsked bool) string {
	b := strings.Builder{}
	classOpts := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(doc.Get("LATEX_CLASS_OPTIONS")), "["), "]")
	if classOpts == "" {
		ar := opts.AspectRatio
		if ar == "" {
			ar = "169"
		}
		classOpts = "aspectratio=" + ar
	}
	b.WriteString("\\documentclass[" + classOpts + "]{beamer}\n")
	b.WriteString(`\usepackage{iftex}
\ifPDFTeX
  \usepackage[utf8]{inputenc}
  \usepackage[T1]{fontenc}
  \usepackage{lmodern}
  \usepackage{textcomp}
\else
  \usepackage{fontspec}
\fi
\usepackage{graphicx}
\usepackage{booktabs}
\usepackage{amsmath}
\usepackage{amssymb}
\usepackage[normalem]{ulem}
`)
	if opts.Code == "minted" {
		b.WriteString("\\usepackage{minted}\n\\setminted{fontsize=\\footnotesize,breaklines}\n")
	} else {
		b.WriteString("\\usepackage{listings}\n" + listingsSetup)
	}

	// The theme: the file's, unless the request asked for one; then a preset.
	fileTheme := strings.TrimSpace(doc.Get("BEAMER_THEME"))
	preset := ""
	switch {
	case themeAsked && opts.Theme != "":
		preset = opts.Theme
	case fileTheme != "":
		b.WriteString(useTheme("", fileTheme))
	case opts.Theme != "":
		preset = opts.Theme
	default:
		preset = "metropolis"
	}
	if preset != "" {
		if p, ok := presetNamed(preset); ok {
			b.WriteString(p)
		} else {
			// Not one of ours: a beamer theme's own name.
			b.WriteString(useTheme("", preset))
		}
	}
	for _, kind := range []string{"COLOR", "FONT", "INNER", "OUTER"} {
		if v := strings.TrimSpace(doc.Get("BEAMER_" + kind + "_THEME")); v != "" {
			b.WriteString(useTheme(strings.ToLower(kind), v))
		}
	}
	// Code keywords in the theme's colour, when it named one.
	b.WriteString("\\providecolor{orgsAccent}{HTML}{2B6CB0}\n")
	if opts.Code != "minted" {
		b.WriteString("\\lstset{keywordstyle=\\color{orgsAccent}\\bfseries,commentstyle=\\color{gray}\\itshape,stringstyle=\\color{orgsAccent!70!black}}\n")
	}
	for _, key := range []string{"LATEX_HEADER", "LATEX_HEADER_EXTRA", "BEAMER_HEADER"} {
		if v := doc.Get(key); v != "" {
			b.WriteString(v + "\n")
		}
	}

	title := inline(doc, w, doc.Get("TITLE"))
	b.WriteString("\n\\title{" + title + "}\n")
	if s := doc.Get("SUBTITLE"); s != "" {
		b.WriteString("\\subtitle{" + inline(doc, w, s) + "}\n")
	}
	author := escape(doc.Get("AUTHOR"))
	if e := strings.TrimSpace(doc.Get("EMAIL")); e != "" && w.option("email", "nil") != "nil" || (e != "" && author == "") {
		author += "\\\\\\href{mailto:" + escapeURL(e) + "}{\\texttt{" + escape(e) + "}}"
	}
	b.WriteString("\\author{" + author + "}\n")
	inst := doc.Get("INSTITUTE")
	if inst == "" {
		inst = doc.Get("BEAMER_INSTITUTE")
	}
	if inst != "" {
		b.WriteString("\\institute{" + inline(doc, w, inst) + "}\n")
	}
	if d := doc.Get("DATE"); d != "" {
		b.WriteString("\\date{" + dateOf(d) + "}\n")
	} else {
		b.WriteString("\\date{\\today}\n")
	}
	b.WriteString("\\hypersetup{pdftitle={" + escape(doc.Get("TITLE")) + "},pdfauthor={" + escape(doc.Get("AUTHOR")) + "},pdfcreator={orgs}}\n\n")
	return b.String()
}

// listings knows neither JavaScript nor the data formats in older TeX
// installs (Go only lately), and a language it does not know stops the
// compile; these are enough for keywords and comments. They are named orgsX so
// as not to meet a driver file's own definition of the same language.
// escapeinside is how text that is not ascii gets through: see listingText.
const listingsSetup = `\lstset{basicstyle=\ttfamily\footnotesize,breaklines=true,columns=fullflexible,keepspaces=true,showstringspaces=false,upquote=true,numberstyle=\tiny\color{gray},numbersep=6pt,escapeinside={(*@}{@*)}}
\lstdefinelanguage{orgsGo}{morekeywords={break,case,chan,const,continue,default,defer,else,fallthrough,for,func,go,goto,if,import,interface,map,package,range,return,select,struct,switch,type,var,nil,true,false},sensitive=true,morecomment=[l]{//},morecomment=[s]{/*}{*/},morestring=[b]",morestring=[b]',morestring=[b]` + "`" + `}
\lstdefinelanguage{orgsJavaScript}{morekeywords={async,await,break,case,catch,class,const,continue,default,delete,do,else,export,extends,finally,for,function,if,import,in,instanceof,let,new,of,return,super,switch,this,throw,try,typeof,var,void,while,yield,null,true,false,undefined},sensitive=true,morecomment=[l]{//},morecomment=[s]{/*}{*/},morestring=[b]",morestring=[b]',morestring=[b]` + "`" + `}
\lstdefinelanguage{orgsRust}{morekeywords={as,break,const,continue,crate,else,enum,extern,false,fn,for,if,impl,in,let,loop,match,mod,move,mut,pub,ref,return,self,Self,static,struct,super,trait,true,type,unsafe,use,where,while,async,await,dyn},sensitive=true,morecomment=[l]{//},morecomment=[s]{/*}{*/},morestring=[b]"}
\lstdefinelanguage{orgsYAML}{morekeywords={true,false,null,yes,no},sensitive=false,morecomment=[l]{\#},morestring=[b]",morestring=[b]'}
\lstdefinelanguage{orgsJSON}{morekeywords={true,false,null},sensitive=true,morestring=[b]"}
`

// ---------------------------------------------------------------------------
// Presets
// ---------------------------------------------------------------------------

// The look shared by clean and dark: a short accent rule under each frame
// title, blocks as tinted panels, page numbers bottom right, no navigation
// symbols. Only stock beamer, so it compiles on any TeX install.
const cleanBase = `\usetheme{default}
\setbeamertemplate{navigation symbols}{}
\setbeamercolor{structure}{fg=orgsAccent}
\setbeamercolor{normal text}{fg=orgsInk,bg=orgsPaper}
\setbeamercolor{background canvas}{bg=orgsPaper}
\setbeamercolor{frametitle}{fg=orgsInk}
\setbeamercolor{title}{fg=orgsInk}
\setbeamercolor{subtitle}{fg=orgsAccent}
\setbeamercolor{alerted text}{fg=orgsAlert}
\setbeamercolor{block title}{fg=orgsInk,bg=orgsAccent!18!orgsPaper}
\setbeamercolor{block body}{fg=orgsInk,bg=orgsAccent!6!orgsPaper}
\setbeamercolor{block title alerted}{fg=orgsInk,bg=orgsAlert!22!orgsPaper}
\setbeamercolor{block body alerted}{fg=orgsInk,bg=orgsAlert!7!orgsPaper}
\setbeamercolor{block title example}{fg=orgsInk,bg=orgsGreen!22!orgsPaper}
\setbeamercolor{block body example}{fg=orgsInk,bg=orgsGreen!7!orgsPaper}
\setbeamertemplate{blocks}[rounded][shadow=false]
\setbeamertemplate{itemize items}[circle]
\setbeamertemplate{enumerate items}[default]
\setbeamertemplate{section in toc}[sections numbered]
\setbeamerfont{frametitle}{series=\bfseries,size=\Large}
\setbeamerfont{title}{series=\bfseries,size=\huge}
\setbeamertemplate{frametitle}{\vspace*{0.7em}\insertframetitle\par\vspace*{0.15em}{\color{orgsAccent}\rule{2.5em}{2pt}}\par}
\setbeamertemplate{footline}{\hfill{\usebeamercolor[fg]{structure}\footnotesize\insertframenumber\,/\,\inserttotalframenumber}\hspace*{1.2em}\vspace*{0.9em}}
\setbeamertemplate{title page}{\vfill{\usebeamerfont{title}\usebeamercolor[fg]{title}\inserttitle\par}\vspace{0.3em}{\color{orgsAccent}\rule{4em}{3pt}}\par\vspace{0.6em}{\usebeamercolor[fg]{subtitle}\large\insertsubtitle\par}\vspace{1.5em}{\insertauthor\par}\vspace{0.3em}{\small\insertinstitute\par}\vspace{0.3em}{\small\color{orgsInk!70!orgsPaper}\insertdate\par}\vfill}
`

var presets = map[string]string{
	"clean": `\definecolor{orgsAccent}{HTML}{2B6CB0}
\definecolor{orgsInk}{HTML}{1F2933}
\definecolor{orgsPaper}{HTML}{FFFFFF}
\definecolor{orgsAlert}{HTML}{C53030}
\definecolor{orgsGreen}{HTML}{2F855A}
` + cleanBase,
	"dark": `\definecolor{orgsAccent}{HTML}{89B4FA}
\definecolor{orgsInk}{HTML}{E6E9EF}
\definecolor{orgsPaper}{HTML}{1E1E2E}
\definecolor{orgsAlert}{HTML}{F38BA8}
\definecolor{orgsGreen}{HTML}{A6E3A1}
` + cleanBase,
	"classic": `\usetheme{Madrid}
\setbeamertemplate{navigation symbols}{}
\definecolor{orgsAccent}{HTML}{3333B3}
`,
	"academic": `\usetheme{Boadilla}
\usecolortheme{dolphin}
\setbeamertemplate{navigation symbols}{}
\setbeamertemplate{blocks}[rounded][shadow=false]
\definecolor{orgsAccent}{HTML}{1F4E8C}
`,
	"minimal": `\usetheme{default}
\setbeamertemplate{navigation symbols}{}
\setbeamercolor{structure}{fg=black}
\setbeamercolor{frametitle}{fg=black}
\setbeamerfont{frametitle}{series=\bfseries}
\setbeamertemplate{itemize items}{\textendash}
\setbeamertemplate{footline}{\hfill{\color{gray}\footnotesize\insertframenumber}\hspace*{1em}\vspace*{0.8em}}
\definecolor{orgsAccent}{HTML}{000000}
`,
	"metropolis": `\usetheme[progressbar=frametitle,numbering=fraction]{metropolis}
\setbeamertemplate{navigation symbols}{}
\definecolor{orgsAccent}{HTML}{EB811B}
`,
}

// PresetNames is every preset's name, for a list to pick from.
func PresetNames() []string {
	return []string{"clean", "dark", "metropolis", "classic", "academic", "minimal"}
}

// presetNamed is a preset's preamble. metropolis is not part of every TeX
// install; when it is missing the deck gets clean rather than an error.
func presetNamed(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	p, ok := presets[name]
	if !ok {
		return "", false
	}
	if name == "metropolis" && !haveTexFile("beamerthememetropolis.sty") {
		return "% metropolis is not installed: clean instead\n" + presets["clean"], true
	}
	return p, true
}

var (
	texFilesLock sync.Mutex
	texFiles     = map[string]bool{}
)

// haveTexFile asks kpsewhich whether TeX can find a file. With no TeX at all
// the answer is no, and a deck written here to be compiled elsewhere uses the
// preset that needs nothing.
func haveTexFile(name string) bool {
	texFilesLock.Lock()
	defer texFilesLock.Unlock()
	if v, ok := texFiles[name]; ok {
		return v
	}
	found := false
	if kpse, err := findTool("kpsewhich"); err == nil {
		out, err := exec.Command(kpse, name).Output()
		found = err == nil && strings.TrimSpace(string(out)) != ""
	}
	texFiles[name] = found
	return found
}

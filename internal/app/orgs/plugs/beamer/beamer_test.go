package beamer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func parse(t *testing.T, text, path string) *org.Document {
	t.Helper()
	d := org.New().Parse(strings.NewReader(text), path)
	if d.Error != nil {
		t.Fatal(d.Error)
	}
	return d
}

func render(t *testing.T, text string, o Options) string {
	t.Helper()
	return Render(parse(t, text, "/tmp/deck.org"), o, o.Theme != "")
}

func mustHave(t *testing.T, tex string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(tex, w) {
			t.Errorf("missing %q in\n%s", w, tex)
		}
	}
}

func mustNotHave(t *testing.T, tex string, nots ...string) {
	t.Helper()
	for _, w := range nots {
		if strings.Contains(tex, w) {
			t.Errorf("should not have %q in\n%s", w, tex)
		}
	}
}

const sample = `#+TITLE: Talk
#+OPTIONS: H:2 toc:nil
* Part one
** First frame
Some text_with & specials 50% #1 {x} ~tilde~ $5.
*** A block
    Inside.
*** Watch out                                                    :B_alertblock:
    :PROPERTIES:
    :BEAMER_env: alertblock
    :BEAMER_act: <2->
    :END:
    Careful.
** Code frame
#+BEGIN_SRC python
print("hi")
#+END_SRC
** Columns
*** Left
    :PROPERTIES:
    :BEAMER_col: 0.4
    :BEAMER_env: column
    :END:
    L
*** Right
    :PROPERTIES:
    :BEAMER_col: 0.6
    :BEAMER_env: column
    :END:
    R
* Hidden                                                            :noexport:
** Nope
`

func TestFramesSectionsAndBlocks(t *testing.T) {
	tex := render(t, sample, Options{Theme: "clean"})
	mustHave(t, tex,
		`\section{Part one}`,
		`\begin{frame}[label=sec-`, `]{First frame}`,
		`\begin{block}{A block}`,
		`\begin{alertblock}<2->{Watch out}`,
		`\end{frame}`,
	)
	mustNotHave(t, tex, "Nope", "tableofcontents", "Hidden")
	// Escaping: specials are text.
	mustHave(t, tex, `text\_with \& specials 50\% \#1 \{x\}`, `\texttt{tilde}`, `\$5`)
}

func TestFragileIsAddedForCode(t *testing.T) {
	tex := render(t, sample, Options{Theme: "clean"})
	i := strings.Index(tex, "{Code frame}")
	if i < 0 {
		t.Fatal("no code frame")
	}
	head := tex[strings.LastIndex(tex[:i], `\begin{frame}`):i]
	if !strings.Contains(head, "fragile") {
		t.Errorf("code frame is not fragile: %s", head)
	}
	mustHave(t, tex, "\\begin{lstlisting}[language=Python]\nprint(\"hi\")\n\\end{lstlisting}")
	first := tex[strings.Index(tex, "{First frame}")-80 : strings.Index(tex, "{First frame}")]
	if strings.Contains(first, "fragile") {
		t.Errorf("a frame without code was made fragile: %s", first)
	}
}

func TestColumnsAreWrapped(t *testing.T) {
	tex := render(t, sample, Options{Theme: "clean"})
	mustHave(t, tex, "\\begin{columns}\n\\begin{column}{0.4\\columnwidth}\nL\n\n\\end{column}\n\\begin{column}{0.6\\columnwidth}\nR\n\n\\end{column}\n\\end{columns}")
}

func TestColWithoutEnvIsABlockInAColumn(t *testing.T) {
	tex := render(t, `#+OPTIONS: H:1
* Frame
** One
   :PROPERTIES:
   :BEAMER_col: 0.5
   :END:
   a
** Two
   :PROPERTIES:
   :BEAMER_col: 0.5
   :BEAMER_env: exampleblock
   :END:
   b
`, Options{Theme: "clean"})
	mustHave(t, tex, "\\begin{columns}\n\\begin{column}{0.5\\columnwidth}\n\\begin{block}{One}", "\\begin{exampleblock}{Two}", "\\end{columns}")
}

func TestFrameLevelOne(t *testing.T) {
	doc := "* Alpha\n** Sub\nx\n* Beta\n"
	tex := render(t, doc, Options{Theme: "clean"})
	mustHave(t, tex, "]{Alpha}", "]{Beta}", `\begin{block}{Sub}`)
	mustNotHave(t, tex, `\section`)
	tex = render(t, "#+OPTIONS: H:2\n"+doc, Options{Theme: "clean"})
	mustHave(t, tex, `\section{Alpha}`, "]{Sub}")
	// The exporter's own frame level, when the file says nothing.
	tex = render(t, doc, Options{Theme: "clean", FrameLevel: 2})
	mustHave(t, tex, `\section{Alpha}`)
}

func TestFrameEnvOverridesLevel(t *testing.T) {
	tex := render(t, `#+OPTIONS: H:3
* Whole frame
  :PROPERTIES:
  :BEAMER_env: frame
  :BEAMER_opt: allowframebreaks
  :END:
** Inside
text
`, Options{Theme: "clean"})
	mustHave(t, tex, `\begin{frame}[allowframebreaks,label=sec-`, `\begin{block}{Inside}`)
}

func TestOverlaysOnLists(t *testing.T) {
	tex := render(t, `* F
#+ATTR_BEAMER: :overlay <+->
- one
- <3-> three
- [X] done
`, Options{Theme: "clean"})
	mustHave(t, tex, `\begin{itemize}[<+->]`, `\item<3-> three`, `\item $\boxtimes$ done`)
	mustNotHave(t, tex, "ATTR")
}

func TestNotesIgnoreheadingAgainframe(t *testing.T) {
	tex := render(t, `#+OPTIONS: H:1
* Target
  :PROPERTIES:
  :CUSTOM_ID: tgt
  :END:
body
** Speaker
   :PROPERTIES:
   :BEAMER_env: note
   :END:
   say this
** Gone
   :PROPERTIES:
   :BEAMER_env: ignoreheading
   :END:
   kept text
* Again
  :PROPERTIES:
  :BEAMER_env: againframe
  :BEAMER_ref: [[#tgt]]
  :BEAMER_act: <2>
  :END:
`, Options{Theme: "clean"})
	mustHave(t, tex, `\begin{frame}[label=tgt]{Target}`, `\note{\textbf{Speaker}`, "say this", "kept text", `\againframe<2>{tgt}`)
	mustNotHave(t, tex, "{Gone}")
}

func TestHeadAndThemes(t *testing.T) {
	doc := `#+TITLE: My *talk*
#+AUTHOR: A & B
#+DATE: <2026-10-14 Wed>
#+LATEX_CLASS_OPTIONS: [aspectratio=43]
#+BEAMER_COLOR_THEME: beaver
* F
x
`
	tex := render(t, doc, Options{Theme: "classic"})
	mustHave(t, tex, `\documentclass[aspectratio=43]{beamer}`, `\usetheme{Madrid}`, `\usecolortheme{beaver}`,
		`\title{My \textbf{talk}}`, `\author{A \& B}`, `\date{2026-10-14}`, `\maketitle`)
	// The file's own theme wins over the exporter's, a request's over both.
	withTheme := "#+BEAMER_THEME: Rochester [height=20pt]\n" + doc
	tex = Render(parse(t, withTheme, "/tmp/d.org"), Options{Theme: "classic"}, false)
	mustHave(t, tex, `\usetheme[height=20pt]{Rochester}`)
	mustNotHave(t, tex, "Madrid")
	tex = Render(parse(t, withTheme, "/tmp/d.org"), Options{Theme: "dark"}, true)
	mustHave(t, tex, `\definecolor{orgsPaper}{HTML}{1E1E2E}`)
	mustNotHave(t, tex, "Rochester")
	// An unknown name is a beamer theme's.
	tex = render(t, doc, Options{Theme: "Warsaw"})
	mustHave(t, tex, `\usetheme{Warsaw}`)
	// Default 16:9.
	tex = render(t, "* F\n", Options{Theme: "clean"})
	mustHave(t, tex, `\documentclass[aspectratio=169]{beamer}`)
}

func TestFootnotesLinksMathImages(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "p.png"), []byte("x"), 0644)
	d := parse(t, `* F
See [[https://orgmode.org/a_b%20c][Org]] and https://x.org/y and [[*G][there]].[fn:1]
Math $a_1^2$ and \(x_y\) and \alpha \to \beta.

#+CAPTION: A picture
#+ATTR_LATEX: :width 3cm
[[file:p.png]]
* G
x
* Footnotes
[fn:1] A note & more.
`, filepath.Join(dir, "d.org"))
	tex := Render(d, Options{Theme: "clean"}, true)
	mustHave(t, tex,
		`\href{https://orgmode.org/a_b\%20c}{Org}`, `\url{https://x.org/y}`,
		`\hyperlink{sec-`, `\footnote{A note \& more.}`,
		`$a_1^2$`, `\(x_y\)`, `$\alpha$ $\to$ $\beta$`,
		`\includegraphics[width=3cm]{`+filepath.ToSlash(filepath.Join(dir, "p.png"))+`}`, `\caption{A picture}`,
	)
	mustNotHave(t, tex, "{Footnotes}")
}

func TestMinted(t *testing.T) {
	tex := render(t, "* F\n#+BEGIN_SRC emacs-lisp\n(message \"x\")\n#+END_SRC\n", Options{Theme: "clean", Code: "minted"})
	mustHave(t, tex, `\usepackage{minted}`, "\\begin{minted}{common-lisp}")
}

// ORGS_BEAMER_PDF=out.pdf ORGS_BEAMER_ORG=deck.org go test -run TestCompileSample
// compiles a deck, for looking at.
func TestCompileSample(t *testing.T) {
	out, src := os.Getenv("ORGS_BEAMER_PDF"), os.Getenv("ORGS_BEAMER_ORG")
	if out == "" || src == "" {
		t.Skip("set ORGS_BEAMER_PDF and ORGS_BEAMER_ORG")
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(src)
	theme := os.Getenv("ORGS_BEAMER_THEME")
	tex := Render(parse(t, string(b), abs), Options{Theme: theme}, theme != "")
	os.WriteFile(strings.TrimSuffix(out, ".pdf")+".tex", []byte(tex), 0644)
	pdf, err := Compile(tex, CompileOptions{Engine: os.Getenv("ORGS_BEAMER_ENGINE")})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(out, pdf, 0644)
}

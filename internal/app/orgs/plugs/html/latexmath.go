package htmlexp

// Maths for features.latexSuite: each LaTeX fragment is written as a span
// holding its TeX (data-tex) and, as its text, the maths as typed - what a
// reader without script sees. LatexMathLoader (appended once by
// ExportToString when the page has any) draws them with KaTeX; worg, which
// shows this html without running its scripts, draws the same spans with
// its own copy. Off, the fragment is written as go-org writes it.

import (
	"html"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/go-org/org"
)

func (w *OrgHtmlWriter) WriteLatexFragment(l org.LatexFragment) {
	if !common.LatexSuiteActive() {
		w.HTMLWriter.WriteLatexFragment(l)
		return
	}
	inner := org.String(l.Content...)
	// Org reads $ x$ (a space inside a dollar) as text; go-org does not.
	if l.OpeningPair == "$" && inner != strings.TrimSpace(inner) {
		w.WriteString(html.EscapeString(l.OpeningPair + inner + l.ClosingPair))
		return
	}
	tex, display := inner, false
	switch {
	case l.OpeningPair == `\[` || l.OpeningPair == "$$":
		display = true
	case strings.HasPrefix(l.OpeningPair, `\begin{`):
		tex, display = l.OpeningPair+inner+l.ClosingPair, true
	}
	cls := "org-math org-math-inline"
	if display {
		cls = "org-math org-math-display"
	}
	w.WriteString(`<span class="` + cls + `" data-tex="` + html.EscapeString(strings.TrimSpace(tex)) + `">` +
		html.EscapeString(l.OpeningPair+inner+l.ClosingPair) + `</span>`)
}

// LatexMathLoader is the stylesheet and script that draw a page's
// .org-math spans with KaTeX, or "" when the setting says off.
func LatexMathLoader() string {
	base := strings.TrimRight(common.LatexKatexBase(), "/")
	if base == "" || base == "off" {
		return ""
	}
	b := html.EscapeString(base)
	return `<link rel="stylesheet" href="` + b + `/katex.min.css">
<style>.org-math-display{display:block;text-align:center;margin:.8em 0;overflow-x:auto}</style>
<script>(function(){function go(){document.querySelectorAll('.org-math').forEach(function(e){if(e.getAttribute('data-done'))return;try{katex.render(e.getAttribute('data-tex'),e,{displayMode:e.classList.contains('org-math-display'),throwOnError:false});e.setAttribute('data-done','1')}catch(x){}})}var s=document.createElement('script');s.src='` + b + `/katex.min.js';s.onload=go;document.head.appendChild(s)})();</script>
`
}

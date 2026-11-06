package htmlexp

import (
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

const latexMathSrc = "* A\nInline \\( a<b \\) and $x^2$.\n\n\\[ \\frac{1}{2} \\]\n\n\\begin{align}\na &= b\n\\end{align}\n"

func writeLatexMathDoc(on bool) string {
	common.LatexSuiteOn = func() bool { return on }
	defer func() { common.LatexSuiteOn = nil }()
	doc := org.New().Parse(strings.NewReader(latexMathSrc), "test.org")
	w := NewOrgHtmlWriter(&OrgHtmlExporter{Props: map[string]interface{}{}, pm: &common.PluginManager{}})
	w.SrcDoc = doc
	org.WriteNodes(w, doc.Nodes...)
	return w.String()
}

func TestLatexMathInHtml(t *testing.T) {
	if off := writeLatexMathDoc(false); strings.Contains(off, "org-math") || !strings.Contains(off, `\( a&lt;b \)`) {
		t.Errorf("off: want go-org's fragment, got:\n%s", off)
	}
	on := writeLatexMathDoc(true)
	for _, want := range []string{
		`<span class="org-math org-math-inline" data-tex="a&lt;b">\( a&lt;b \)</span>`,
		`<span class="org-math org-math-inline" data-tex="x^2">$x^2$</span>`,
		`<span class="org-math org-math-display" data-tex="\frac{1}{2}">`,
		`<span class="org-math org-math-display" data-tex="\begin{align}` + "\n" + `a &amp;= b` + "\n" + `\end{align}">`,
	} {
		if !strings.Contains(on, want) {
			t.Errorf("on: missing %s in:\n%s", want, on)
		}
	}
	if l := LatexMathLoader(); !strings.Contains(l, "katex.min.js") {
		t.Errorf("loader: %s", l)
	}
}

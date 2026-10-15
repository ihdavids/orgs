package typst

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ihdavids/go-org/org"
)

func parse(t *testing.T, src, path string) *org.Document {
	t.Helper()
	conf := org.New().Silent()
	conf.DefaultSettings["EXCLUDE_TAGS"] = "noexport"
	conf.DefaultSettings["OPTIONS"] = "toc:t <:t e:t f:t pri:t todo:t tags:t title:t ealb:nil"
	d := conf.Parse(strings.NewReader(src), path)
	if d.Error != nil {
		t.Fatalf("parse: %v", d.Error)
	}
	return d
}

func sample(t *testing.T) *org.Document {
	t.Helper()
	path, _ := filepath.Abs("testdata/sample.org")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parse(t, string(b), path)
}

func export(t *testing.T, d *org.Document, props map[string]string) string {
	t.Helper()
	src, _, err := (&Exporter{}).Document(d, props, nil)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestEscapeMarkup(t *testing.T) {
	cases := []struct {
		in   string
		bol  bool
		want string
	}{
		{`a # b`, false, `a \# b`},
		{`$5 and @me <x> [y] ~z ` + "`t`" + ` _u_ *s* \`, false, `\$5 and \@me \<x\> \[y\] \~z ` + "\\`t\\`" + ` \_u\_ \*s\* \\`},
		{`a // b /* c */ d/e`, false, `a \// b \/\* c \*/ d/e`},
		{`= heading`, true, `\= heading`},
		{`= not at start`, false, `= not at start`},
		{"x\n- item\n+ plus\n12. num\n/ term: x", false, "x\n\\- item\n\\+ plus\n12\\. num\n\\/ term: x"},
		{`  - indented`, true, `  \- indented`},
	}
	for _, c := range cases {
		if got := escapeMarkup(c.in, c.bol); got != c.want {
			t.Errorf("escapeMarkup(%q, %v)\n got %q\nwant %q", c.in, c.bol, got, c.want)
		}
	}
}

func TestFence(t *testing.T) {
	if f := fence("no ticks"); f != "```" {
		t.Errorf("fence = %q", f)
	}
	if f := fence("has ```` four"); f != "`````" {
		t.Errorf("fence = %q", f)
	}
}

func TestStr(t *testing.T) {
	if got := str("a \"b\" \\c\nd"); got != `"a \"b\" \\c\nd"` {
		t.Errorf("str = %s", got)
	}
}

func TestTexToTypst(t *testing.T) {
	cases := map[string]string{
		`a^2 + b^2 = c^2`:          `a^(2) + b^(2) = c^(2)`,
		`\frac{1}{2}\alpha`:        `frac(1, 2) alpha`,
		`\sum_{i=1}^{n} i`:         `sum_(i = 1)^(n) i`,
		`\sqrt[3]{x} \le \infty`:   `root(3, x) <= infinity`,
		`xy`:                       `x y`,
		`\mathbb{R} \to \text{ok}`: `bb(R) -> "ok"`,
	}
	for in, want := range cases {
		got, err := texToTypst(in)
		if err != nil || got != want {
			t.Errorf("texToTypst(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`a &= b`, `\begin{x}`, `\unknowncmd`, `{x`, `\frac{1}`} {
		if got, err := texToTypst(bad); err == nil {
			t.Errorf("texToTypst(%q) = %q, want an error", bad, got)
		}
	}
}

func TestDocument(t *testing.T) {
	src := export(t, sample(t), nil)
	for _, want := range []string{
		MitexImport,
		"#show: org-template.with(",
		`title: [Field Notes: #strong[Spring] Survey \[2026\]]`,
		`title-text: "Field Notes: Spring Survey [2026]"`,
		`authors: ("Jane Roe", "John #Doe")`,
		`date: [2026-10-14 Wed]`,
		`keywords: ("birds", "survey", "spring")`,
		"toc: true",
		"toc-depth: 2",
		// Headings, labels, keywords, tags.
		"= Introduction #org-tags((\"intro\",)); <intro>",
		`= #org-todo("TODO", done: false); #org-priority("A"); Count the birds #org-tags(("work", "field",)); <sec-count-the-birds>`,
		`== #org-todo("DONE", done: true); Find binoculars`,
		`#org-planning("DEADLINE", "<2026-10-20 Tue>");`,
		// Inline markup.
		"#strong[bold]", "#emph[italic]", "#underline[underline]", "#strike[strike]",
		`#raw("verbatim")`, `#raw("code")`, "H#sub[2]O", "mc#super[2]",
		"α → – and —",
		"Hello, world!",
		// Hostile text survives.
		`a \# hash, \$5 or so, \@someone, \<angle\> \[brackets\], C:\\temp`,
		`path\//to and c\/\*x`,
		`under\_score\_name, star\*s`,
		// Links.
		"#link(<intro>)[the intro]",
		"#link(<intro>)[intro]",
		"#link(<intro>)[by id]",
		"#link(<tbl-counts>)[tbl-counts]",
		"#metadata(none)<target-here>",
		"#link(<target-here>)[back to target]",
		`#link("https://example.com/a?b=1&c=2")[Example site]`,
		`#link("https://typst.app")`,
		`#link("other.pdf")[other notes]`,
		// Footnotes.
		"#footnote[The first footnote, with a #link(\"https://example.com\")[link]\\.]<fn-1>",
		"#footnote[Inline #emph[footnote] text.]",
		// Checkboxes and statistics.
		`#org-stat("1/3")`, "#org-check(true); draft", "#org-check(none); figures", "#org-check(false); review",
		// Table as a figure, with its header and rule.
		"#figure(table(",
		"table.header(org-th[Species], org-th[Count], org-th[Notes], table.hline())",
		"align: (left, right, left,)",
		"table.hline()",
		"[\\$ & \\#]",
		"caption: [Counts by #emph[species]]) <tbl-counts>",
		// A table with no rule has no header.
		"#table(\n  columns: 3,",
		// Pictures.
		`#figure(image("chart.png", width: 40%), caption: [A chart]) <fig-chart>`,
		`#align(center, image("./chart.png"))`,
		// Code and results; :exports none leaves nothing.
		"```python\ndef f(x):",
		"#org-example(\"9\")",
		"#org-example(\"literal *text* #here\")",
		"#org-example(\"fixed width line with $ and #\")",
		"#quote(block: true)[To be or not to be, #strong[that] is the question.]",
		"Roses are red \\\n",
		"#align(center)[Centred text]",
		`#org-special("note")[A special #strong[note] block.]`,
		"#text(fill: red)[raw typst passes through]",
		"#v(0.5em)",
		"#org-hr()",
		// Maths through mitex.
		`#mi("a^2 + b^2 = c^2")`,
		`#mi("\\frac{1}{2}\\alpha")`,
		`#mitex(" \\sum_{i=1}^{n} i = \\frac{n(n+1)}{2} ")`,
		`#mitex("\\begin{align}`,
		// Lists.
		"#enum(\n  [first],",
		"#list(\n  [nested #strong[bullet]],",
		"#enum(\n  numbering: \"a)\",\n  [alpha]",
		"terms.item[Term one][the first description]",
		// An archived heading keeps its title, not its body.
		"= Archived",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, not := range []string{
		"must not appear", "Hidden body", "rm -rf", "Commented", ":PROPERTIES:", "6f1c8d2e-intro\"",
	} {
		if strings.Contains(src, not) {
			t.Errorf("should not contain %q", not)
		}
	}
	if t.Failed() {
		t.Logf("output:\n%s", src)
	}
}

func TestTypstMath(t *testing.T) {
	d := parse(t, "#+TYPST_MATH: typst\n\nInline $a^2$ and \\(\\weird{x}\\).\n\n\\[ \\frac{a}{b} \\]\n", "/tmp/m.org")
	src := export(t, d, nil)
	for _, want := range []string{"$a^(2)$", `#mi("\\weird{x}")`, "$ frac(a, b) $", MitexImport} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in\n%s", want, src)
		}
	}
	d = parse(t, "#+TYPST_MATH: typst\n\nOnly $x$ here.\n", "/tmp/m.org")
	if src := export(t, d, nil); strings.Contains(src, "mitex") {
		t.Errorf("no formula needed mitex, but it was imported:\n%s", src)
	}
}

func TestOptions(t *testing.T) {
	d := parse(t, "#+OPTIONS: toc:nil num:nil todo:nil tags:nil pri:nil H:1\n* TODO [#B] Top :x:\n** Deep\ntext\n", "/tmp/o.org")
	src := export(t, d, nil)
	for _, want := range []string{"toc: false", "numbered: false", "= Top <sec-top>", "#strong[Deep] <sec-deep>"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in\n%s", want, src)
		}
	}
	if strings.Contains(src, "#org-todo(\"") || strings.Contains(src, "#org-tags((") || strings.Contains(src, "#org-priority(\"") {
		t.Errorf("todo/pri/tags should be off:\n%s", src)
	}
}

func TestDuplicateLabels(t *testing.T) {
	d := parse(t, "* Same\n* Same\n* Same\n", "/tmp/d.org")
	src := export(t, d, nil)
	for _, want := range []string{"<sec-same>", "<sec-same-2>", "<sec-same-3>"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestExpressionNotContinued(t *testing.T) {
	d := parse(t, "A *b*.c and [[https://a.b][d]](e).\n", "/tmp/e.org")
	src := export(t, d, nil)
	if !strings.Contains(src, `#strong[b]\.c`) || !strings.Contains(src, `#link("https://a.b")[d]\(e)`) {
		t.Errorf("an expression would swallow what follows:\n%s", src)
	}
}

func TestTemplatesSettings(t *testing.T) {
	d := parse(t, "#+TYPST_TEMPLATE: report\n#+LATEX_CLASS_OPTIONS: [letterpaper]\n* A\n", "/tmp/s.org")
	set := (&Exporter{}).settingsFor(d, map[string]string{})
	if set.template != "report" || set.paper != "us-letter" {
		t.Errorf("settings = %+v", set)
	}
	set = (&Exporter{Template: "notes"}).settingsFor(d, map[string]string{"theme": "article", "paper": "a5"})
	if set.template != "article" || set.paper != "a5" {
		t.Errorf("a request should win: %+v", set)
	}
}

// TestCompile compiles the sample with every template when typst is
// installed, and leaves the pdfs in $TYPST_OUT (if set) to look at.
func TestCompile(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst is not installed")
	}
	if testing.Short() {
		t.Skip("short")
	}
	hpath, _ := filepath.Abs("testdata/hostile.org")
	hb, _ := os.ReadFile(hpath)
	if _, err := (&Exporter{}).PDF(parse(t, string(hb), hpath), map[string]string{}); err != nil {
		t.Errorf("hostile: %v", err)
	}
	d := sample(t)
	for _, tpl := range []string{"article", "report", "notes"} {
		props := map[string]string{"template": tpl}
		if tpl == "notes" {
			props["columns"] = "2"
		}
		pdf, err := (&Exporter{}).PDF(d, props)
		if err != nil {
			t.Errorf("%s: %v", tpl, err)
			continue
		}
		if len(pdf) < 1000 || !strings.HasPrefix(string(pdf), "%PDF") {
			t.Errorf("%s: not a pdf", tpl)
		}
		if out := os.Getenv("TYPST_OUT"); out != "" {
			os.WriteFile(filepath.Join(out, tpl+".pdf"), pdf, 0o644)
			src, _, _ := (&Exporter{}).Document(d, props, nil)
			os.WriteFile(filepath.Join(out, tpl+".typ"), []byte(src), 0o644)
		}
	}
}

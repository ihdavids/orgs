package orgs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// A one pixel png.
const tinyPng = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

type nbm = map[string]interface{}

func md(src ...string) nbm { return nbm{"cell_type": "markdown", "metadata": nbm{}, "source": src} }
func code(src string, outputs ...nbm) nbm {
	if outputs == nil {
		outputs = []nbm{}
	}
	return nbm{"cell_type": "code", "metadata": nbm{}, "execution_count": 1, "source": src, "outputs": outputs}
}
func stream(text ...string) nbm { return nbm{"output_type": "stream", "name": "stdout", "text": text} }
func result(data nbm) nbm {
	return nbm{"output_type": "execute_result", "execution_count": 1, "metadata": nbm{}, "data": data}
}
func display(data nbm) nbm { return nbm{"output_type": "display_data", "metadata": nbm{}, "data": data} }

func nb(meta nbm, cells ...nbm) []byte {
	if meta == nil {
		meta = nbm{"kernelspec": nbm{"name": "python3", "language": "python", "display_name": "Python 3"}}
	}
	b, _ := json.Marshal(nbm{"nbformat": 4, "nbformat_minor": 5, "metadata": meta, "cells": cells})
	return b
}

var importDay = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

func convert(t *testing.T, data []byte) *ipynbConversion {
	t.Helper()
	c, err := convertIpynb(data, ipynbOptions{Base: "penguins", Source: "penguins.ipynb", Now: importDay})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestANotebookBecomesAnOrgFile(t *testing.T) {
	c := convert(t, nb(nil,
		md("# Penguin measurements\n", "\n", "A look at *three* species.\n", "\n", "## Loading\n"),
		code("import statistics\nxs = [3, 1, 4]\nprint(len(xs))", stream("3\n")),
		code("statistics.mean(xs)", result(nbm{"text/plain": []string{"2.6666666666666665"}})),
		md("## Plotting\n", "\n", "- one\n", "- two\n", "  1. nested\n"),
		code("plt.plot(xs)", stream("drawing\n"), display(nbm{"image/png": tinyPng, "text/plain": "<Figure size 640x480 with 1 Axes>"})),
		code("1/0", nbm{"output_type": "error", "ename": "ZeroDivisionError", "evalue": "division by zero",
			"traceback": []string{"\x1b[0;31m---------------------------------------------------------------------------\x1b[0m", "\x1b[0;31mZeroDivisionError\x1b[0m: division by zero"}}),
		code(""),
	))
	want := `#+TITLE: Penguin measurements
#+PROPERTY: header-args:python :session penguins :results output
#+STARTUP: inlineimages
# Imported from penguins.ipynb (nbformat 4.5) on 2026-10-10.

A look at /three/ species.

* Loading

#+BEGIN_SRC python
import statistics
xs = [3, 1, 4]
print(len(xs))
#+END_SRC

#+RESULTS:
: 3

#+BEGIN_SRC python
statistics.mean(xs)
#+END_SRC

#+RESULTS:
: 2.6666666666666665

* Plotting

- one
- two
  1. nested

#+BEGIN_SRC python :results graphics file :file penguins_files/cell-3.png
plt.plot(xs)
#+END_SRC

#+RESULTS:
:results:
: drawing
[[file:penguins_files/cell-3.png]]
:end:

#+BEGIN_SRC python
1/0
#+END_SRC

#+RESULTS:
: ---------------------------------------------------------------------------
: ZeroDivisionError: division by zero
`
	if c.Org != want {
		t.Errorf("got\n%s\nwant\n%s", c.Org, want)
	}
	if c.Title != "Penguin measurements" || c.Session != "penguins" || c.Language != "python" {
		t.Errorf("title %q session %q language %q", c.Title, c.Session, c.Language)
	}
	if c.Cells != 7 || c.Code != 5 || c.Markdown != 2 || c.Blocks != 4 || c.Outputs != 4 || c.Images != 1 || c.Errors != 1 {
		t.Errorf("counts %+v", c.IpynbImported)
	}
	if b, ok := c.Assets["penguins_files/cell-3.png"]; !ok || !strings.HasPrefix(string(b), "\x89PNG") {
		t.Errorf("the picture was not decoded: %v", c.Assets)
	}
}

func TestMarkdownToOrg(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"emphasis", "**bold**, *it*, _also_, ~~gone~~ and `x*y`", "*bold*, /it/, /also/, +gone+ and ~x*y~"},
		{"snake case is not emphasis", "use my_var_name here", "use my_var_name here"},
		{"maths is left alone", "where $a_i * b_i$ and $$\\sum_i x_i$$", "where $a_i * b_i$ and $$\\sum_i x_i$$"},
		{"links", "[docs](https://pandas.pydata.org) and [next](part2.ipynb) and <https://x.org>",
			"[[https://pandas.pydata.org][docs]] and [[file:part2.org][next]] and [[https://x.org]]"},
		{"pictures", "![a plot](figs/plot.png) <img src=\"logo.svg\" width=40>", "[[file:figs/plot.png]] [[file:logo.svg]]"},
		{"star bullets are not headings", "* one\n* two", "- one\n- two"},
		{"numbered", "1. first\n2) second", "1. first\n2. second"},
		{"fenced code", "```python\nx = 1\n* not a heading\n```", "#+BEGIN_SRC python\nx = 1\n,* not a heading\n#+END_SRC"},
		{"fenced plain", "```\nsome output\n```", "#+BEGIN_EXAMPLE\nsome output\n#+END_EXAMPLE"},
		{"quote", "> a *quoted*\n> line", "#+BEGIN_QUOTE\na /quoted/\nline\n#+END_QUOTE"},
		{"table", "| a | b |\n|:--|--:|\n| 1 | `x|y` |", "| a | b           |\n|---+-------------|\n| 1 | ~x\\vert{}y~ |"},
		{"display maths", "$$\n\\int_0^1 x\\,dx\n$$", "$$\n\\int_0^1 x\\,dx\n$$"},
		{"environment", "\\begin{align}\na &= b \\\\\n\\end{align}", "\\begin{align}\na &= b \\\\\n\\end{align}"},
		{"rule", "---", "-----"},
		{"setext", "Title\n=====\n\ntext", "* Title\n\ntext"},
		{"html block", "<div class=\"alert\">\n<b>Note</b> careful\n</div>", "#+BEGIN_EXPORT html\n<div class=\"alert\">\n<b>Note</b> careful\n</div>\n#+END_EXPORT"},
		{"dressed html", "<font color=red><b>Careful</b></font>", "*Careful*"},
		{"anchors go", "## <a id=\"top\"></a>Setup", "** Setup"},
		{"indented code", "text\n\n    x = 1\n    y = 2", "text\n\n#+BEGIN_EXAMPLE\nx = 1\ny = 2\n#+END_EXAMPLE"},
	}
	for _, c := range cases {
		got := strings.Join(markdownToOrg(c.in, mdContext{}), "\n")
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestADataframeIsAnOrgTable(t *testing.T) {
	df := `<div>
<style scoped>
    .dataframe tbody tr th:only-of-type { vertical-align: middle; }
</style>
<table border="1" class="dataframe">
  <thead>
    <tr style="text-align: right;"><th></th><th>species</th><th>mass</th></tr>
    <tr><th>id</th><th></th><th></th></tr>
  </thead>
  <tbody>
    <tr><th>0</th><td>Adelie</td><td>3750.0</td></tr>
    <tr><th>1</th><td>Gentoo &amp; co</td><td>5000.0</td></tr>
  </tbody>
</table>
<p>2 rows × 2 columns</p>
</div>`
	c := convert(t, nb(nil, code("df.head()", result(nbm{"text/html": df, "text/plain": "   species  mass\n0  Adelie  3750.0"}))))
	want := `#+RESULTS:
| id | species     | mass   |
|----+-------------+--------|
| 0  | Adelie      | 3750.0 |
| 1  | Gentoo & co | 5000.0 |
`
	if !strings.HasSuffix(c.Org, want) {
		t.Errorf("got\n%s", c.Org)
	}
	// Html that is not one plain table falls back to its text.
	c = convert(t, nb(nil, code("x", result(nbm{"text/html": "<b>hi</b><table><tr><td>1</td></tr></table>", "text/plain": "hi 1"}))))
	if !strings.HasSuffix(c.Org, "#+RESULTS:\n: hi 1\n") {
		t.Errorf("got\n%s", c.Org)
	}
}

func TestNotebookAttachmentsAndRawCells(t *testing.T) {
	cell := md("Here: ![diagram](attachment:image.png)")
	cell["attachments"] = nbm{"image.png": nbm{"image/png": tinyPng}}
	raw := nbm{"cell_type": "raw", "metadata": nbm{"format": "text/html"}, "source": "<hr/>"}
	c := convert(t, nb(nbm{"title": "From metadata"}, cell, raw, code("%%bash\necho hi", stream("hi\n"))))
	for _, w := range []string{
		"#+TITLE: From metadata",
		"Here: [[file:penguins_files/md-1-image.png]]",
		"#+BEGIN_EXPORT html\n<hr/>\n#+END_EXPORT",
		"#+BEGIN_SRC sh\necho hi\n#+END_SRC\n\n#+RESULTS:\n: hi",
	} {
		if !strings.Contains(c.Org, w) {
			t.Errorf("missing %q in\n%s", w, c.Org)
		}
	}
	if _, ok := c.Assets["penguins_files/md-1-image.png"]; !ok {
		t.Errorf("attachment not saved: %v", c.Assets)
	}
}

func TestNotebookVersions(t *testing.T) {
	v3 := `{"nbformat": 3, "nbformat_minor": 0, "metadata": {"name": "old one"},
	 "worksheets": [{"cells": [
	  {"cell_type": "heading", "level": 2, "source": ["Setup"], "metadata": {}},
	  {"cell_type": "code", "language": "python", "input": ["1 + 1"], "prompt_number": 1, "metadata": {},
	   "outputs": [{"output_type": "pyout", "prompt_number": 1, "text": ["2"], "metadata": {}}]}
	 ]}]}`
	c := convert(t, []byte(v3))
	for _, w := range []string{"#+TITLE: old one", "* Setup", "#+BEGIN_SRC python\n1 + 1\n#+END_SRC\n\n#+RESULTS:\n: 2"} {
		if !strings.Contains(c.Org, w) {
			t.Errorf("missing %q in\n%s", w, c.Org)
		}
	}
	if _, err := convertIpynb([]byte(`{"nbformat": 2, "worksheets": []}`), ipynbOptions{}); err == nil || !strings.Contains(err.Error(), "nbformat 2") {
		t.Errorf("nbformat 2 should be refused clearly: %v", err)
	}
	if _, err := convertIpynb([]byte(`not json`), ipynbOptions{}); err == nil {
		t.Errorf("not json should be refused")
	}
	// An R notebook is written as R, with no picture header (only python has
	// sessions that save figures).
	c = convert(t, nb(nbm{"kernelspec": nbm{"name": "ir", "language": "R"}},
		code("plot(1:10)", display(nbm{"image/png": tinyPng}))))
	if !strings.Contains(c.Org, "#+PROPERTY: header-args:R :session penguins") || !strings.Contains(c.Org, "#+BEGIN_SRC R\nplot") {
		t.Errorf("got\n%s", c.Org)
	}
}

func TestImportingWritesTheFileAndRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	if Conf().Server == nil {
		t.Skip("no server settings")
	}
	old := Conf().Server.OrgDirs
	Conf().Server.OrgDirs = []string{dir}
	t.Cleanup(func() { Conf().Server.OrgDirs = old })

	data := string(nb(nil, md("# Title"), code("print(1)", stream("1\n"), display(nbm{"image/png": tinyPng}))))
	req := &common.IpynbImport{Notebook: data, Name: "My Analysis.ipynb", Filename: "sci/analysis"}
	res, status := ImportIpynb(req)
	if !res.Ok || status != 200 {
		t.Fatalf("%d %+v", status, res)
	}
	if res.Filename != filepath.Join(dir, "sci", "analysis.org") || len(res.Files) != 1 {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "sci", "analysis_files", "cell-1.png")); err != nil {
		t.Errorf("picture: %v", err)
	}
	if f := GetDb().FindByFile(res.Filename); f == nil {
		t.Errorf("the file was not loaded")
	}
	if res, status := ImportIpynb(req); res.Ok || status != 409 {
		t.Errorf("a second import should be refused: %d %+v", status, res)
	}
	req.Overwrite = true
	if res, _ := ImportIpynb(req); !res.Ok {
		t.Errorf("overwrite: %+v", res)
	}
	for _, bad := range []string{"../outside.org", "/etc/x.org", "notes.txt"} {
		if res, status := ImportIpynb(&common.IpynbImport{Notebook: data, Filename: bad}); res.Ok || status != 400 {
			t.Errorf("%s should be refused: %d %+v", bad, status, res)
		}
	}
	dry, _ := ImportIpynb(&common.IpynbImport{Notebook: data, Filename: "dry.org", DryRun: true})
	if !dry.Ok || dry.Org == "" {
		t.Errorf("dry run: %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(dir, "dry.org")); err == nil {
		t.Errorf("a dry run wrote the file")
	}
}

// The point of writing results the way babel writes them: running an
// imported notebook replaces its outputs instead of adding to them.
func TestAnImportedNotebookRunsAndItsResultsAreReplaced(t *testing.T) {
	sessionsOn(t)
	c := convert(t, nb(nil,
		md("## Data\n"),
		code("xs = [3, 1, 4]\nprint(len(xs))", stream("3\n")),
		code("sum(xs)", result(nbm{"text/plain": "8"})),
		code("def f(**kw):\n    \"\"\"\n* a star line in a docstring\n#+ and a hash plus\n\"\"\"\n    return kw\nf(a=1)", result(nbm{"text/plain": "{'a': 1}"})),
	))
	paths := loadOrg(t, map[string]string{"penguins.org": c.Org})
	res := runAll(t, paths["penguins.org"])
	if !res.Ok {
		t.Fatalf("%s %+v", res.Msg, res.Ran)
	}
	got := readOrg(t, paths["penguins.org"])
	// Imported, the sum was the stale 8; run, it is 8 again and the rest is
	// as imported - nothing doubled.
	if strings.TrimSpace(got) != strings.TrimSpace(c.Org) {
		t.Errorf("running changed the file:\n%s\nimported:\n%s", got, c.Org)
	}
}
